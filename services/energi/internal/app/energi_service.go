package app

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"energi/models"
	"energi/storage"

	"gorm.io/datatypes"
)

type EnergiService struct {
	Storage storage.EnergiStorage
}

type SourceConfig struct {
	Name   string
	IsWali bool
}

var SourceMap = map[int]SourceConfig{
	1: {Name: "BPS", IsWali: true},
	2: {Name: "PLN", IsWali: true},
	3: {Name: "ESDM", IsWali: true},
	4: {Name: "LAINNYA", IsWali: false},
}

var AuditMap = map[int]string{
	1: "VALID",
	2: "INVALID",
}

// 2. METADATA VALIDATOR (DYNAMIC SCHEMA VALIDATION DENGAN REFLECT)
func (s *EnergiService) ValidateEnergiMetadata(k models.RekamEnergi, definition datatypes.JSON) (bool, string) {
	var schemaMap map[string]interface{}
	if err := json.Unmarshal(definition, &schemaMap); err != nil {
		return false, "Gagal membaca aturan metadata energi"
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
				return false, fmt.Sprintf("Atribut energi '%s' wajib diisi (Mandatory)", jsonTag)
			}

			// B. Cek Panjang Karakter (Length) - Khusus Nomor KK atau ID PLN
			if lengthVal, ok := r["length"].(float64); ok {
				if fieldValue != "" && len(fieldValue) != int(lengthVal) {
					return false, fmt.Sprintf("Atribut '%s' tidak valid, harus %d digit", jsonTag, int(lengthVal))
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
					return false, fmt.Sprintf("Atribut tambahan energi '%s' wajib diisi", field)
				}
			}
		}
	}

	return true, ""
}

// 3. CONFLICT RESOLUTION & SCD TYPE 2
func (s *EnergiService) ProcessIngestion(k models.RekamEnergi) (string, error) {
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
		return fmt.Sprintf("Sukses v%d (Energi Updated)", k.Version), nil
	}

	return "Abaikan", fmt.Errorf("data energi yang dikirim usang")
}

func (s *EnergiService) ProcessManualUpdate(nokk string, newData models.RekamEnergi) (int, error) {
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

func (s *EnergiService) ProcessAuditDecision(nokkList []string, verdict int) (string, error) {
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