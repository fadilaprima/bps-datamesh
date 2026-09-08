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

// ==========================================
// 1. FUNGSI VALIDASI INTERNAL (HARD FAIL)
// ==========================================
func (s *HunianService) ValidateInternalHunian(k *models.RekamHunian) error {
	if k.LuasLantai < 0 || k.LuasLantai > 999 {
		return fmt.Errorf("luas lantai tidak valid (harus 0 s.d. 999 m2)")
	}

	if k.FasilitasBAB == "Tidak Ada" && (k.JenisKloset == "Leher Angsa" || k.PembuanganTinja == "Tangki Septik") {
		return fmt.Errorf("fasilitas BAB tidak ada tetapi jenis kloset atau pembuangan tinja menggunakan standar layak")
	}

	return nil
}

// ==========================================
// 2. FUNGSI VALIDASI SILANG (HYBRID)
// ==========================================
func (s *HunianService) ValidateCrossDomainAPI(k *models.RekamHunian) error {
	client := &http.Client{Timeout: 3 * time.Second}

	// A. Domain Energi 
	baseURLEnergi := os.Getenv("URL_ENERGI")
	if baseURLEnergi != "" {
		targetURLEnergi := fmt.Sprintf("%s/api/v1/domains/energi/datasets/%s", baseURLEnergi, k.NoKK)
		respEnergi, err := client.Get(targetURLEnergi)
		
		if err != nil {
			fmt.Printf("[WARNING] Servis Energi down, NoKK %s tidak tervalidasi penuh. Trust Score -10\n", k.NoKK)
			k.TrustScore -= 10.0
		} else {
			defer respEnergi.Body.Close()
			if respEnergi.StatusCode == 200 {
				body, _ := io.ReadAll(respEnergi.Body)
				var result map[string]interface{}
				
				if errJson := json.Unmarshal(body, &result); errJson == nil {
					dayaTerpasang, _ := strconv.Atoi(fmt.Sprintf("%v", result["daya_terpasang"]))

					if k.SumberPenerangan == "Bukan Listrik" && dayaTerpasang > 0 {
						return fmt.Errorf("ditolak: sumber penerangan bukan listrik tetapi terdapat daya terpasang > 0")
					}
				}
			}
		}
	}

	// B. Domain Kesejahteraan 
	baseURLKesejahteraan := os.Getenv("URL_KESEJAHTERAAN")
	if baseURLKesejahteraan != "" {
		targetURLKesejahteraan := fmt.Sprintf("%s/api/v1/domains/kesejahteraan/datasets/%s", baseURLKesejahteraan, k.NoKK)
		respKes, err := client.Get(targetURLKesejahteraan)
		
		if err != nil {
			// [NETWORK ERROR - SOFT FAIL]
			fmt.Printf("[WARNING] Servis Kesejahteraan down, NoKK %s tidak tervalidasi penuh. Trust Score -10\n", k.NoKK)
			k.TrustScore -= 10.0
		} else {
			defer respKes.Body.Close()
			if respKes.StatusCode == 200 {
				body, _ := io.ReadAll(respKes.Body)
				var result map[string]interface{}
				
				if errJson := json.Unmarshal(body, &result); errJson == nil {
					asetAc := fmt.Sprintf("%v", result["aset_bergerak_ac"])
					asetKulkas := fmt.Sprintf("%v", result["aset_bergerak_lemari_es"])

					// [LOGICAL ERROR - HARD FAIL]
					if k.SumberPenerangan == "Bukan Listrik" && (asetAc == "Ya" || asetAc == "1" || asetKulkas == "Ya" || asetKulkas == "1") {
						return fmt.Errorf("ditolak: sumber penerangan utama bukan listrik tetapi memiliki aset AC atau Lemari Es")
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
			if r["required"] == true && strings.TrimSpace(fieldValue) == "" {
				return false, fmt.Sprintf("Atribut '%s' wajib diisi (Mandatory)", jsonTag)
			}

			if lengthVal, ok := r["length"].(float64); ok {
				if fieldValue != "" && len(fieldValue) != int(lengthVal) {
					return false, fmt.Sprintf("Atribut '%s' tidak valid, harus %d digit", jsonTag, int(lengthVal))
				}
			}
			
			if maxVal, ok := r["max"].(float64); ok {
				var intVal int
				fmt.Sscanf(fieldValue, "%d", &intVal)
				if intVal > int(maxVal) {
					return false, fmt.Sprintf("Atribut '%s' tidak boleh lebih dari %d", jsonTag, int(maxVal))
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
func (s *HunianService) ProcessIngestion(k models.RekamHunian) (string, error) {
	if err := s.ValidateInternalHunian(&k); err != nil {
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