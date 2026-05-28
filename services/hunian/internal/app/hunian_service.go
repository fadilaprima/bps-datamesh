package app

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"hunian/models"
	"hunian/storage"

	"gorm.io/datatypes"
)

type HunianService struct {
	Storage storage.HunianStorage
}

type SourceConfig struct {
	Name   string
	IsWali bool
}

var SourceMap = map[int]SourceConfig{
	1: {Name: "BPS", IsWali: true},
	2: {Name: "PUPR", IsWali: true},
	3: {Name: "PKP", IsWali: true},
	5: {Name: "LAINNYA", IsWali: false},
}

var AuditMap = map[int]string{
	1: "VALID",
	2: "INVALID",
}

// 2. METADATA VALIDATOR (DYNAMIC SCHEMA VALIDATION DENGAN REFLECT)
func (s *HunianService) ValidateHunianMetadata(k models.RekamHunian, definition datatypes.JSON) (bool, string) {
	var schemaMap map[string]interface{}
	if err := json.Unmarshal(definition, &schemaMap); err != nil {
		return false, "Gagal membaca aturan metadata hunian"
	}

	rules, ok := schemaMap["definition"].(map[string]interface{})
	if !ok { return true, "" }

	val := reflect.ValueOf(k)
	typ := reflect.TypeOf(k)
	fixedFields := make(map[string]bool)

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		jsonTag := strings.Split(field.Tag.Get("json"), ",")[0]
		if jsonTag == "" || jsonTag == "-" { continue }
		
		fixedFields[jsonTag] = true
		fieldValue := fmt.Sprintf("%v", val.Field(i).Interface()) 

		if r, ok := rules[jsonTag].(map[string]interface{}); ok {
			// A. Cek Mandatory (Required)
			if r["required"] == true && strings.TrimSpace(fieldValue) == "" {
				return false, fmt.Sprintf("Atribut hunian '%s' wajib diisi (Mandatory)", jsonTag)
			}

			// B. Cek Panjang Karakter (Length) - Berguna buat Nomor KK (16 digit)
			if lengthVal, ok := r["length"].(float64); ok {
				if fieldValue != "" && len(fieldValue) != int(lengthVal) {
					return false, fmt.Sprintf("Atribut '%s' tidak valid, harus %d digit", jsonTag, int(lengthVal))
				}
			}
			
			// C. Cek Batas Maksimum (Max) - Khusus untuk Luas Lantai
			if maxVal, ok := r["max"].(float64); ok {
				var intVal int
				fmt.Sscanf(fieldValue, "%d", &intVal)
				if intVal > int(maxVal) {
					return false, fmt.Sprintf("Atribut '%s' tidak boleh lebih dari %d", jsonTag, int(maxVal))
				}
			}
		}
	}

	// 3. VALIDASI ATRIBUT TAMBAHAN (AdditionalInfo)
	var extra map[string]interface{}
	json.Unmarshal(k.AdditionalInfo, &extra)

	for field, rule := range rules {
		r, ok := rule.(map[string]interface{})
		if !ok { continue }

		if r["required"] == true {
			if !fixedFields[field] {
				if valData, exists := extra[field]; !exists || fmt.Sprintf("%v", valData) == "" {
					return false, fmt.Sprintf("Atribut tambahan hunian '%s' wajib diisi", field)
				}
			}
		}
	}

	return true, ""
}

// 3. CONFLICT RESOLUTION & SCD TYPE 2
func (s *HunianService) ProcessIngestion(k models.RekamHunian) (string, error) {
	last, err := s.Storage.GetLatestByNoKK(k.NoKK)

	if err != nil {
		k.Version = 1
		k.AuditStatus = "PENDING"
		if errCreate := s.Storage.Create(&k); errCreate != nil { return "Error", errCreate }
		return "Sukses v1 (Initial Entry)", nil
	}

	isNewer := k.ReferenceDate.After(last.ReferenceDate)
	isHigherAuthority := k.ReferenceDate.Equal(last.ReferenceDate) && k.IsWaliData && !last.IsWaliData

	if isNewer || isHigherAuthority {
		k.ID = 0 
		k.Version = last.Version + 1
		k.AuditStatus = "PENDING" 

		if errCreate := s.Storage.Create(&k); errCreate != nil { return "Error", errCreate }
		return fmt.Sprintf("Sukses v%d (Hunian Updated)", k.Version), nil
	}

	return "Abaikan", fmt.Errorf("data hunian yang dikirim usang")
}

func (s *HunianService) ProcessManualUpdate(nokk string, newData models.RekamHunian) (int, error) {
	oldData, err := s.Storage.GetLatestByNoKK(nokk)
	if err != nil || oldData == nil {
		return 0, fmt.Errorf("data asli tidak ditemukan")
	}

	newData.ID = 0
	newData.Version = oldData.Version + 1
	newData.AuditStatus = "PENDING"
	newData.UpdatedAt = time.Now()

	if newData.IsWaliData { newData.TrustScore = 80.0 } else { newData.TrustScore = 60.0 }

	errCreate := s.Storage.Create(&newData)
	return newData.Version, errCreate
}

func (s *HunianService) ProcessAuditDecision(nokkList []string, verdict int) (string, error) {
	verdictText := "INVALID"
	bonus := 0.0

	if text, exists := AuditMap[verdict]; exists {
		verdictText = text
		if verdict == 1 { bonus = 20.0 }
	} else {
		return "", fmt.Errorf("Verdict tidak valid")
	}

	err := s.Storage.UpdateBulkAuditDecision(nokkList, verdictText, bonus)
	return verdictText, err
}