package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"strconv"
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

// ==========================================
// 1. FUNGSI VALIDASI INTERNAL (HARD FAIL)
// ==========================================
func (s *EnergiService) ValidateInternalEnergi(k *models.RekamEnergi) error {
	if strings.TrimSpace(k.IDPelangganPLN) != "" && strings.TrimSpace(k.DayaTerpasang) == "" {
		return fmt.Errorf("memiliki ID pelanggan PLN tetapi daya terpasang kosong")
	}

	return nil
}

// ==========================================
// 2. FUNGSI VALIDASI SILANG (HYBRID)
// ==========================================
func (s *EnergiService) ValidateCrossDomainAPI(k *models.RekamEnergi) error {
	client := &http.Client{Timeout: 3 * time.Second}

	// A. Domain Hunian 
	baseURLHunian := os.Getenv("URL_HUNIAN")
	if baseURLHunian != "" {
		targetURL := fmt.Sprintf("%s/api/v1/domains/hunian/datasets/%s", baseURLHunian, k.NoKK)
		resp, err := client.Get(targetURL)
		
		if err != nil {
			fmt.Printf("[WARNING] Servis Hunian down, NoKK %s tidak tervalidasi penuh. Trust Score -10\n", k.NoKK)
			k.TrustScore -= 10.0
		} else {
			defer resp.Body.Close()
			if resp.StatusCode == 200 {
				body, _ := io.ReadAll(resp.Body)
				var result map[string]interface{}
				
				if errJson := json.Unmarshal(body, &result); errJson == nil {
					sumberPenerangan := fmt.Sprintf("%v", result["sumber_penerangan_utama"])
					dayaInt, _ := strconv.Atoi(strings.TrimSuffix(strings.ReplaceAll(k.DayaTerpasang, " watt", ""), " watt"))
					if sumberPenerangan == "Bukan Listrik" && dayaInt > 0 {
						return fmt.Errorf("ditolak: sumber penerangan utama rumah tangga tercatat 'Bukan Listrik' tetapi memiliki daya terpasang > 0")
					}
				}
			}
		}
	}

	return nil
}

// ==========================================
// 3. METADATA VALIDATION (HARD FAIL)
// ==========================================
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
			if r["required"] == true && strings.TrimSpace(fieldValue) == "" {
				return false, fmt.Sprintf("Atribut '%s' wajib diisi (Mandatory)", jsonTag)
			}
			if lengthVal, ok := r["length"].(float64); ok {
				if fieldValue != "" && len(fieldValue) != int(lengthVal) {
					return false, fmt.Sprintf("Atribut '%s' tidak valid, harus %d digit", jsonTag, int(lengthVal))
				}
			}
		}
	}

	var extra map[string]interface{}
	json.Unmarshal(k.AdditionalInfo, &extra)

	for field, rule := range rules {
		r, ok := rule.(map[string]interface{})
		if !ok { continue }

		if r["required"] == true {
			if !fixedFields[field] {
				if valData, exists := extra[field]; !exists || fmt.Sprintf("%v", valData) == "" {
					return false, fmt.Sprintf("Atribut tambahan '%s' wajib diisi", field)
				}
			}
		}
	}

	return true, ""
}

// ==========================================
// 4. INGESTION PIPELINE (SCD Type 2)
// ==========================================
func (s *EnergiService) ProcessIngestion(k models.RekamEnergi) (string, error) {
	if err := s.ValidateInternalEnergi(&k); err != nil {
		return "Error Internal Logic", err
	}

	if err := s.ValidateCrossDomainAPI(&k); err != nil {
		return "Error Cross-Domain Logic", err
	}

	last, err := s.Storage.GetLatestByNoKK(k.NoKK)

	// Skenario A: Data Baru
	if err != nil {
		k.Version = 1
		k.AuditStatus = "PENDING"
		if errCreate := s.Storage.Create(&k); errCreate != nil {
			return "Error Database", errCreate
		}
		return "Sukses v1 (Masuk Data Mesh)", nil
	}

	// Skenario B: Update Data
	isNewer := k.ReferenceDate.After(last.ReferenceDate)
	isHigherAuthority := k.ReferenceDate.Equal(last.ReferenceDate) && k.IsWaliData && !last.IsWaliData

	if isNewer || isHigherAuthority {
		k.ID = 0
		k.Version = last.Version + 1
		k.AuditStatus = "PENDING"

		if errCreate := s.Storage.Create(&k); errCreate != nil {
			return "Error Database", errCreate
		}
		return fmt.Sprintf("Sukses v%d (Update)", k.Version), nil
	}

	return "Abaikan", fmt.Errorf("data usang atau otoritas lebih rendah")
}

// ==========================================
// 5. UPDATE & AUDIT LOGIC
// ==========================================
func (s *EnergiService) ProcessManualUpdate(nokk string, incomingData models.RekamEnergi) (int, error) {
	oldData, err := s.Storage.GetLatestByNoKK(nokk)
	if err != nil || oldData == nil {
		return 0, fmt.Errorf("data asli tidak ditemukan")
	}

	newRecord := *oldData
	newRecord.ID = 0
	newRecord.Version = oldData.Version + 1
	newRecord.AuditStatus = "PENDING"
	newRecord.UpdatedAt = time.Now()

	newRecord.TrustScore = oldData.TrustScore - 20.0
	if newRecord.TrustScore < 0 {
		newRecord.TrustScore = 0
	}

	// TIMPA PARSIAL OTOMATIS
	valIncoming := reflect.ValueOf(incomingData)
	valNew := reflect.ValueOf(&newRecord).Elem()

	for i := 0; i < valIncoming.NumField(); i++ {
		fieldName := valIncoming.Type().Field(i).Name
		
		if fieldName == "ID" || fieldName == "Version" || fieldName == "AuditStatus" ||
			fieldName == "UpdatedAt" || fieldName == "TrustScore" || fieldName == "ReferenceDate" || fieldName == "CreatedAt" {
			continue
		}
		
		incomingField := valIncoming.Field(i)
		newField := valNew.Field(i)
		
		if newField.CanSet() && !incomingField.IsZero() {
			newField.Set(incomingField)
		}
	}

	if errCreate := s.Storage.Create(&newRecord); errCreate != nil {
		return 0, fmt.Errorf("gagal menyimpan data: %v", errCreate)
	}

	_ = s.Storage.SoftDelete(fmt.Sprintf("%d", oldData.ID))
	return newRecord.Version, nil
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