package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
// 1. FUNGSI VALIDASI INTERNAL (INTRA-DOMAIN)
// ==========================================
func (s *HunianService) ValidateInternalHunian(k models.RekamHunian) error {
	// Rule: Luas lantai maksimum sesuai standar DTSEN adalah 999 m2
	if k.LuasLantai < 0 || k.LuasLantai > 999 {
		return fmt.Errorf("gagal validasi internal: luas lantai tidak valid (harus 0 s.d. 999 m2)")
	}

	// Rule: FasilitasBAB "Tidak Ada" tetapi jenis kloset Leher Angsa atau pembuangan tinja Tangki Septik
	if k.FasilitasBAB == "Tidak Ada" && (k.JenisKloset == "Leher Angsa" || k.PembuanganTinja == "Tangki Septik") {
		return fmt.Errorf("gagal validasi internal: fasilitas BAB tidak ada tetapi jenis kloset atau pembuangan tinja menggunakan standar layak")
	}

	return nil
}

// ==========================================
// 2. FUNGSI VALIDASI SILANG (CROSS-DOMAIN API)
// ==========================================
func (s *HunianService) ValidateCrossDomainAPI(k models.RekamHunian) error {
	client := &http.Client{Timeout: 3 * time.Second}

	// Cek ke Domain Energi (Port 8088) berdasarkan NoKK
	respEnergi, err := client.Get(fmt.Sprintf("http://localhost:8088/api/v1/domains/energi/datasets/%s", k.NoKK))
	if err == nil && respEnergi.StatusCode == 200 {
		defer respEnergi.Body.Close()
		body, _ := io.ReadAll(respEnergi.Body)

		var result []map[string]interface{}
		if errJson := json.Unmarshal(body, &result); errJson == nil && len(result) > 0 {
			dataEnergi := result[0]
			dayaTerpasang, _ := strconv.Atoi(fmt.Sprintf("%v", dataEnergi["daya_terpasang"]))

			// Rule: SumberPenerangan = Bukan Listrik dan daya_terpasang > 0
			if k.SumberPenerangan == "Bukan Listrik" && dayaTerpasang > 0 {
				return fmt.Errorf("gagal validasi lintas domain (energi): sumber penerangan bukan listrik tetapi terdapat daya terpasang > 0")
			}
		}
	}

	// Cek ke Domain Kesejahteraan (Port 8086) berdasarkan NoKK
	respKes, err := client.Get(fmt.Sprintf("http://localhost:8086/api/v1/domains/kesejahteraan/datasets/%s", k.NoKK))
	if err == nil && respKes.StatusCode == 200 {
		defer respKes.Body.Close()
		body, _ := io.ReadAll(respKes.Body)

		var result []map[string]interface{}
		if errJson := json.Unmarshal(body, &result); errJson == nil && len(result) > 0 {
			dataKes := result[0]
			asetAc := fmt.Sprintf("%v", dataKes["aset_bergerak_ac"])
			asetKulkas := fmt.Sprintf("%v", dataKes["aset_bergerak_lemari_es"])

			// Rule: SumberPenerangan = Bukan Listrik dan (aset_bergerak_ac = Ya ATAU aset_bergerak_lemari_es = Ya)
			if k.SumberPenerangan == "Bukan Listrik" && (asetAc == "Ya" || asetAc == "1" || asetKulkas == "Ya" || asetKulkas == "1") {
				return fmt.Errorf("gagal validasi lintas domain (kesejahteraan): sumber penerangan utama bukan listrik tetapi memiliki aset AC atau Lemari Es")
			}
		}
	}

	return nil
}

// ==========================================
// 3. METADATA VALIDATOR (DYNAMIC SCHEMA)
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

	// VALIDASI ATRIBUT TAMBAHAN (AdditionalInfo)
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

// ==========================================
// 4. INGESTION PIPELINE (SCD TYPE 2)
// ==========================================
func (s *HunianService) ProcessIngestion(k models.RekamHunian) (string, error) {
	// --- EKSEKUSI BLOK VALIDASI SEBELUM MASUK DATABASE ---
	if err := s.ValidateInternalHunian(k); err != nil {
		return "Error Validation", err
	}

	if err := s.ValidateCrossDomainAPI(k); err != nil {
		return "Error Cross-Validation", err
	}
	// --- AKHIR BLOK VALIDASI ---

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