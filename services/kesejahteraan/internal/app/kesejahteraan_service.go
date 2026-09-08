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

	"kesejahteraan/models"
	"kesejahteraan/storage"

	"gorm.io/datatypes"
)

type KesejahteraanService struct {
	Storage storage.KesejahteraanStorage
}

type SourceConfig struct {
	Name   string
	IsWali bool
}

var SourceMap = map[int]SourceConfig{
	1: {Name: "BPS", IsWali: true},
	2: {Name: "KEMENSOS", IsWali: true},
	3: {Name: "LAINNYA", IsWali: false},
}

var AuditMap = map[int]string{
	1: "VALID",
	2: "INVALID",
}

// ==========================================
// 1. FUNGSI VALIDASI INTERNAL (HARD FAIL)
// ==========================================
func (s *KesejahteraanService) ValidateInternalKesejahteraan(k *models.RekamKesejahteraan) error {
	// Rule: Jumlah ternak tidak boleh negatif atau > 999
	if k.TernakSapi < 0 || k.TernakSapi > 999 ||
		k.TernakKerbau < 0 || k.TernakKerbau > 999 ||
		k.TernakKuda < 0 || k.TernakKuda > 999 ||
		k.TernakBabi < 0 || k.TernakBabi > 999 ||
		k.TernakKambing < 0 || k.TernakKambing > 999 {
		return fmt.Errorf("jumlah ternak tidak berada dalam rentang wajar (0 s.d. 999)")
	}

	return nil
}

// ==========================================
// 2. FUNGSI VALIDASI SILANG (HYBRID)
// ==========================================
func (s *KesejahteraanService) ValidateCrossDomainAPI(k *models.RekamKesejahteraan) error {
	client := &http.Client{Timeout: 3 * time.Second}

	// A. Domain Hunian 
	baseURLHunian := os.Getenv("URL_HUNIAN")
	if baseURLHunian != "" {
		targetURLHunian := fmt.Sprintf("%s/api/v1/domains/hunian/datasets/%s", baseURLHunian, k.NoKK)
		respHunian, err := client.Get(targetURLHunian)
		
		if err != nil {
			fmt.Printf("[WARNING] Servis Hunian down, NoKK %s tidak tervalidasi penuh. Trust Score -10\n", k.NoKK)
			k.TrustScore -= 10.0
		} else {
			defer respHunian.Body.Close()
			if respHunian.StatusCode == 200 {
				body, _ := io.ReadAll(respHunian.Body)
				var result map[string]interface{}
				
				if errJson := json.Unmarshal(body, &result); errJson == nil {
					sumberPenerangan := fmt.Sprintf("%v", result["sumber_penerangan_utama"])

					if sumberPenerangan == "Bukan Listrik" && (k.AsetAC > 0 || k.AsetKulkas > 0) {
						return fmt.Errorf("ditolak: sumber penerangan 'Bukan Listrik' tetapi tercatat memiliki AC atau Kulkas")
					}
				}
			}
		}
	}

	// B. Domain Ketenagakerjaan 
	baseURLKetenagakerjaan := os.Getenv("URL_KETENAGAKERJAAN")
	if baseURLKetenagakerjaan != "" {
		targetURLKetenagakerjaan := fmt.Sprintf("%s/api/v1/domains/ketenagakerjaan/datasets/%s", baseURLKetenagakerjaan, k.NoKK)
		respKerja, err := client.Get(targetURLKetenagakerjaan)
		
		if err != nil {
			fmt.Printf("[WARNING] Servis Ketenagakerjaan down, NoKK %s tidak tervalidasi penuh. Trust Score -10\n", k.NoKK)
			k.TrustScore -= 10.0
		} else {
			defer respKerja.Body.Close()
			if respKerja.StatusCode == 200 {
				body, _ := io.ReadAll(respKerja.Body)
				var result map[string]interface{}
				
				if errJson := json.Unmarshal(body, &result); errJson == nil {
					lapanganUsaha := fmt.Sprintf("%v", result["lapangan_usaha_dari_usaha_utama"])
					if k.AsetLahanLain == 0 && (lapanganUsaha == "Pertanian tanaman pangan dan palawija" || lapanganUsaha == "Perkebunan" || lapanganUsaha == "Kehutanan & pertanian lainnya") {
						return fmt.Errorf("ditolak: mengelola usaha tani/hutan tetapi tidak memiliki aset lahan")
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
func (s *KesejahteraanService) ValidateKesejahteraanMetadata(k models.RekamKesejahteraan, definition datatypes.JSON) (bool, string) {
	var schemaMap map[string]interface{}
	if err := json.Unmarshal(definition, &schemaMap); err != nil {
		return false, "Gagal membaca aturan metadata kesejahteraan"
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
func (s *KesejahteraanService) ProcessIngestion(k models.RekamKesejahteraan) (string, error) {
	if err := s.ValidateInternalKesejahteraan(&k); err != nil {
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
func (s *KesejahteraanService) ProcessManualUpdate(nokk string, newData models.RekamKesejahteraan) (int, error) {
	oldData, err := s.Storage.GetLatestByNoKK(nokk)
	if err != nil || oldData == nil {
		return 0, fmt.Errorf("data asli tidak ditemukan")
	}

	newData.ID = 0
	newData.Version = oldData.Version + 1
	newData.AuditStatus = "PENDING"
	newData.UpdatedAt = time.Now()

	if newData.IsWaliData {
		newData.TrustScore = 80.0
	} else {
		newData.TrustScore = 60.0
	}

	errCreate := s.Storage.Create(&newData)
	return newData.Version, errCreate
}

func (s *KesejahteraanService) ProcessAuditDecision(nokkList []string, verdict int) (string, error) {
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