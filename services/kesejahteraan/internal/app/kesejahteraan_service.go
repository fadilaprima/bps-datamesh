package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"time"

	"kesejahteraan/models"
	"kesejahteraan/storage"

	"gorm.io/datatypes"
)

// KesejahteraanService mengelola seluruh logika bisnis domain kesejahteraan dengan arsitektur Data Mesh
type KesejahteraanService struct {
	Storage storage.KesejahteraanStorage
}

// 1. DOMAIN OWNER
type SourceConfig struct {
	Name   string
	IsWali bool
}

// Map Angka -> Konfigurasi Source
var SourceMap = map[int]SourceConfig{
	1: {Name: "BPS", IsWali: true},
	2: {Name: "KEMENSOS", IsWali: true}, // Wali Data Kesejahteraan (Regsosek/DTKS)
	3: {Name: "LAINNYA", IsWali: false},
}

// Map Angka -> Teks Verdict
var AuditMap = map[int]string{
	1: "VALID",
	2: "INVALID",
}

// ==========================================
// 1. FUNGSI VALIDASI INTERNAL (INTRA-DOMAIN)
// ==========================================
func (s *KesejahteraanService) ValidateInternalKesejahteraan(k models.RekamKesejahteraan) error {
	// Rule: Jumlah ternak tidak boleh negatif atau > 999 (standar DTSEN)
	if k.TernakSapi < 0 || k.TernakSapi > 999 ||
		k.TernakKerbau < 0 || k.TernakKerbau > 999 ||
		k.TernakKuda < 0 || k.TernakKuda > 999 ||
		k.TernakBabi < 0 || k.TernakBabi > 999 ||
		k.TernakKambing < 0 || k.TernakKambing > 999 {
		return fmt.Errorf("gagal validasi internal: jumlah ternak tidak berada dalam rentang wajar (0 s.d. 999)")
	}

	return nil
}

// ==========================================
// 2. FUNGSI VALIDASI SILANG (CROSS-DOMAIN API)
// ==========================================
func (s *KesejahteraanService) ValidateCrossDomainAPI(k models.RekamKesejahteraan) error {
	client := &http.Client{Timeout: 3 * time.Second}

	// --- A. Cek ke Domain Hunian (Port 8087) ---
	// Validasi: Aset Listrik (AC/Kulkas) vs Sumber Penerangan "Bukan Listrik"
	respHunian, err := client.Get(fmt.Sprintf("http://host.docker.internal:8087/api/v1/domains/hunian/datasets/%s", k.NoKK))
	
	if err != nil {
		return fmt.Errorf("gagal menghubungi service hunian untuk validasi silang (pastikan service menyala): %v", err)
	}
	defer respHunian.Body.Close()

	if respHunian.StatusCode == 200 {
		body, _ := io.ReadAll(respHunian.Body)

		var result map[string]interface{}
		if errJson := json.Unmarshal(body, &result); errJson == nil {
			sumberPenerangan := fmt.Sprintf("%v", result["sumber_penerangan_utama"])

			if sumberPenerangan == "Bukan Listrik" && (k.AsetAC > 0 || k.AsetKulkas > 0) {
				return fmt.Errorf("gagal validasi lintas domain (hunian): sumber penerangan utama rumah tangga tercatat 'Bukan Listrik' tetapi memiliki aset AC atau Lemari Es")
			}
		} else {
			return fmt.Errorf("gagal parsing JSON dari service hunian (Format Beda): %v", errJson)
		}
	}

	// --- B. Cek ke Domain Ketenagakerjaan (Port 8085) ---
	// Validasi: Kepemilikan Lahan vs Lapangan Usaha Pertanian/Kehutanan Skala Besar
	respKerja, err := client.Get(fmt.Sprintf("http://host.docker.internal:8085/api/v1/domains/ketenagakerjaan/datasets/%s", k.NoKK))
	
	if err != nil {
		return fmt.Errorf("gagal menghubungi service ketenagakerjaan untuk validasi silang (pastikan service menyala): %v", err)
	}
	defer respKerja.Body.Close()

	if respKerja.StatusCode == 200 {
		body, _ := io.ReadAll(respKerja.Body)

		var result map[string]interface{}
		if errJson := json.Unmarshal(body, &result); errJson == nil {
			lapanganUsaha := fmt.Sprintf("%v", result["lapangan_usaha_dari_usaha_utama"])

			// Jika tidak punya aset lahan (0 atau Tidak Ada) tetapi mengelola usaha tani skala besar
			if k.AsetLahanLain == 0 && (lapanganUsaha == "Pertanian tanaman pangan dan palawija" || lapanganUsaha == "Perkebunan" || lapanganUsaha == "Kehutanan & pertanian lainnya") {
				return fmt.Errorf("gagal validasi lintas domain (ketenagakerjaan): mengelola usaha sektor pertanian/kehutanan tetapi tidak memiliki kepemilikan aset tidak bergerak (lahan)")
			}
		} else {
			return fmt.Errorf("gagal parsing JSON dari service ketenagakerjaan (Format Beda): %v", errJson)
		}
	}

	return nil
}

// ==========================================
// 3. METADATA VALIDATOR (DYNAMIC SCHEMA VALIDATION WITH REFLECT)
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
		if jsonTag == "" || jsonTag == "-" {
			continue
		}

		fixedFields[jsonTag] = true
		fieldValue := fmt.Sprintf("%v", val.Field(i).Interface())

		if r, ok := rules[jsonTag].(map[string]interface{}); ok {
			// A. Cek Mandatory (Required)
			if r["required"] == true && strings.TrimSpace(fieldValue) == "" {
				return false, fmt.Sprintf("Atribut kesejahteraan '%s' wajib diisi (Mandatory)", jsonTag)
			}

			// B. Cek Panjang Karakter (Length) - Berguna untuk Nomor KK (16 digit)
			if lengthVal, ok := r["length"].(float64); ok {
				if fieldValue != "" && len(fieldValue) != int(lengthVal) {
					return false, fmt.Sprintf("Atribut '%s' tidak valid, harus %d digit sesuai standar", jsonTag, int(lengthVal))
				}
			}
		}
	}

	// 3. VALIDASI ATRIBUT TAMBAHAN (AdditionalInfo)
	var extra map[string]interface{}
	json.Unmarshal(k.AdditionalInfo, &extra)

	for field, rule := range rules {
		r, ok := rule.(map[string]interface{})
		if !ok {
			continue
		}

		if r["required"] == true {
			if !fixedFields[field] {
				if valData, exists := extra[field]; !exists || fmt.Sprintf("%v", valData) == "" {
					return false, fmt.Sprintf("Atribut tambahan kesejahteraan '%s' wajib diisi sesuai standar Metadata Mesh", field)
				}
			}
		}
	}

	return true, ""
}

// ==========================================
// 4. INGESTION PIPELINE (SCD TYPE 2)
// ==========================================
func (s *KesejahteraanService) ProcessIngestion(k models.RekamKesejahteraan) (string, error) {
	// --- EKSEKUSI BLOK VALIDASI SEBELUM MASUK DATABASE ---
	if err := s.ValidateInternalKesejahteraan(k); err != nil {
		return "Error Validation", err
	}

	if err := s.ValidateCrossDomainAPI(k); err != nil {
		return "Error Cross-Validation", err
	}
	// --- AKHIR BLOK VALIDASI ---

	last, err := s.Storage.GetLatestByNoKK(k.NoKK)

	// Skenario A: Data Kesejahteraan Baru (First Entry)
	if err != nil {
		k.Version = 1
		k.AuditStatus = "PENDING"
		if errCreate := s.Storage.Create(&k); errCreate != nil {
			return "Error", errCreate
		}
		return "Sukses v1 (Initial Entry)", nil
	}

	// Skenario B: Update Data (SCD Type 2)
	isNewer := k.ReferenceDate.After(last.ReferenceDate)
	isHigherAuthority := k.ReferenceDate.Equal(last.ReferenceDate) && k.IsWaliData && !last.IsWaliData

	if isNewer || isHigherAuthority {
		k.ID = 0
		k.Version = last.Version + 1
		k.AuditStatus = "PENDING"

		if errCreate := s.Storage.Create(&k); errCreate != nil {
			return "Error", errCreate
		}
		return fmt.Sprintf("Sukses v%d (Kesejahteraan Updated)", k.Version), nil
	}

	return "Abaikan", fmt.Errorf("data kesejahteraan yang dikirim lebih usang dibandingkan data di mesh")
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
		if verdict == 1 {
			bonus = 20.0
		}
	} else {
		return "", fmt.Errorf("Verdict tidak valid")
	}

	err := s.Storage.UpdateBulkAuditDecision(nokkList, verdictText, bonus)
	return verdictText, err
}