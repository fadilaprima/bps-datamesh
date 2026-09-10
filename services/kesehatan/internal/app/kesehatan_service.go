package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"strings"
	"time"
	"strconv"

	"kesehatan/models"
	"kesehatan/storage"

	"gorm.io/datatypes"
)

type KesehatanService struct {
	Storage storage.KesehatanStorage
}

type SourceConfig struct {
	Name   string
	IsWali bool
}

var SourceMap = map[int]SourceConfig{
	1: {Name: "BPS", IsWali: true},
	2: {Name: "KEMENKES", IsWali: true},
	3: {Name: "BPJS_KESEHATAN", IsWali: true},
	4: {Name: "DINKES", IsWali: true},
	5: {Name: "LAINNYA", IsWali: false},
}

var AuditMap = map[int]string{
	1: "VALID",
	2: "INVALID",
}

// ==========================================
// 1. FUNGSI VALIDASI INTERNAL (HARD FAIL)
// ==========================================
func (s *KesehatanService) ValidateInternalKesehatan(k *models.RekamKesehatan) error {
	if k.KondisiGizi != "" && k.KondisiGizi != "Kurang gizi (Wasting)" && k.KondisiGizi != "Kerdil (Stunting)" && k.KondisiGizi != "Tidak ada catatan" && k.KondisiGizi != "Tidak tahu" {
		return fmt.Errorf("kondisi gizi diisi dengan nilai yang tidak dikenali")
	}

	return nil
}

// ==========================================
// 2. FUNGSI VALIDASI SILANG (HYBRID)
// ==========================================
func (s *KesehatanService) ValidateCrossDomainAPI(k *models.RekamKesehatan) error {
	client := &http.Client{Timeout: 3 * time.Second}

	// A. Domain Kependudukan 
	baseURLKependudukan := os.Getenv("URL_KEPENDUDUKAN")
	if baseURLKependudukan != "" {
		targetURL := fmt.Sprintf("%s/api/v1/domains/penduduk/datasets/%s", baseURLKependudukan, k.NIK)
		resp, err := client.Get(targetURL)
		
		if err != nil {
			fmt.Printf("[WARNING] Servis Kependudukan down, NIK %s tidak tervalidasi penuh. Trust Score -10\n", k.NIK)
			k.TrustScore -= 10.0
		} else {
			defer resp.Body.Close()
			if resp.StatusCode == 200 {
				body, _ := io.ReadAll(resp.Body)
				var result map[string]interface{}
				
				if errJson := json.Unmarshal(body, &result); errJson == nil {
					tglLahir := fmt.Sprintf("%v", result["tanggal_lahir"])

					umur := -1
					if t, parseErr := time.Parse("2006-01-02", tglLahir); parseErr == nil {
						umur = int(time.Since(t).Hours() / 24 / 365.25)
					}

					if umur >= 0 && umur > 5 && k.KondisiGizi == "Kerdil (Stunting)" {
						return fmt.Errorf("ditolak: kondisi gizi 'Kerdil (Stunting)' tidak wajar untuk umur di atas 5 tahun")
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
func (s *KesehatanService) ValidateKesehatanMetadata(k models.RekamKesehatan, definition datatypes.JSON) (bool, string) {
	var schemaMap map[string]interface{}
	if err := json.Unmarshal(definition, &schemaMap); err != nil {
		return false, "Gagal membaca aturan metadata kesehatan"
	}

	rules, ok := schemaMap["definition"].(map[string]interface{})
	if !ok {
		return true, ""
	}

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
func (s *KesehatanService) ProcessIngestion(k models.RekamKesehatan) (string, error) {
	if err := s.ValidateInternalKesehatan(&k); err != nil {
		return "Error Internal Logic", err
	}

	if err := s.ValidateCrossDomainAPI(&k); err != nil {
		return "Error Cross-Domain Logic", err
	}

	last, err := s.Storage.GetLatestByNIK(k.NIK)

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
func (s *KesehatanService) ProcessManualUpdate(nik string, incomingData models.RekamKesehatan) (int, error) {
	oldData, err := s.Storage.GetLatestByNIK(nik)
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

func (s *KesehatanService) ProcessAuditDecision(nikList []string, verdict int) (string, error) {
	verdictText := "INVALID"
	bonus := 0.0

	if text, exists := AuditMap[verdict]; exists {
		verdictText = text
		if verdict == 1 { bonus = 20.0 }
	} else {
		return "", fmt.Errorf("Verdict tidak valid. Gunakan 1 (VALID) atau 2 (INVALID)")
	}

	err := s.Storage.UpdateBulkAuditDecision(nikList, verdictText, bonus)
	return verdictText, err
}