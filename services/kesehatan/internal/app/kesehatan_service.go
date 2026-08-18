package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"time"

	"kesehatan/models"
	"kesehatan/storage"

	"gorm.io/datatypes"
)

// KesehatanService mengelola seluruh logika bisnis domain kesehatan dengan arsitektur Data Mesh
type KesehatanService struct {
	Storage storage.KesehatanStorage
}

// 1. DOMAIN OWNER
type SourceConfig struct {
	Name   string
	IsWali bool
}

// Map Angka -> Konfigurasi Source
var SourceMap = map[int]SourceConfig{
	1: {Name: "BPS", IsWali: true},
	2: {Name: "KEMENKES", IsWali: true},      // Wali Data
	3: {Name: "BPJS_KESEHATAN", IsWali: true}, // Wali Data
	4: {Name: "DINKES", IsWali: true},         // Kepanjangan Wali Data
	5: {Name: "LAINNYA", IsWali: false},
}

// Map Angka -> Teks Verdict
var AuditMap = map[int]string{
	1: "VALID",
	2: "INVALID",
}

// ==========================================
// 1. FUNGSI VALIDASI INTERNAL (INTRA-DOMAIN)
// ==========================================
func (s *KesehatanService) ValidateInternalKesehatan(k models.RekamKesehatan) error {
	// Contoh Rule Internal: Kondisi gizi balita/anak tidak boleh berisi teks ngawur atau tidak valid jika diisi
	if k.KondisiGizi != "" && k.KondisiGizi != "Kurang gizi (Wasting)" && k.KondisiGizi != "Kerdil (Stunting)" && k.KondisiGizi != "Tidak ada catatan" && k.KondisiGizi != "Tidak tahu" {
		// Bisa disesuaikan dengan standar isian DTSEN kamu
	}

	return nil
}

// ==========================================
// 2. FUNGSI VALIDASI SILANG (CROSS-DOMAIN API)
// ==========================================
func (s *KesehatanService) ValidateCrossDomainAPI(k models.RekamKesehatan) error {
	client := &http.Client{Timeout: 3 * time.Second}

	// Cek ke Domain Kependudukan (Port 8081) untuk mendapatkan umur / tanggal lahir jika dibutuhkan
	resp, err := client.Get(fmt.Sprintf("http://localhost:8081/api/v1/domains/penduduk/datasets/%s", k.NIK))
	if err == nil && resp.StatusCode == 200 {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)

		var result []map[string]interface{}
		if errJson := json.Unmarshal(body, &result); errJson == nil && len(result) > 0 {
			dataPenduduk := result[0]
			tglLahir := fmt.Sprintf("%v", dataPenduduk["tanggal_lahir"])

			umur := -1
			if t, parseErr := time.Parse("2006-01-02", tglLahir); parseErr == nil {
				umur = int(time.Since(t).Hours() / 24 / 365.25)
			}

			// Contoh Rule Silang: Kondisi gizi atau pemeriksaan balita untuk umur tertentu
			if umur >= 0 && umur > 5 && k.KondisiGizi == "Kerdil (Stunting)" {
				// Validasi tambahan lintas domain jika diperlukan
			}
		}
	}

	return nil
}

// ==========================================
// 3. METADATA VALIDATOR (DYNAMIC SCHEMA)
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
			// A. Cek Mandatory (Required)
			if r["required"] == true && strings.TrimSpace(fieldValue) == "" {
				return false, fmt.Sprintf("Atribut kesehatan '%s' wajib diisi (Mandatory)", jsonTag)
			}

			// B. Cek Panjang Karakter (Length) - Berguna banget buat validasi NIK (16 Digit)
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
		if !ok { continue }

		if r["required"] == true {
			if !fixedFields[field] {
				if valData, exists := extra[field]; !exists || fmt.Sprintf("%v", valData) == "" {
					return false, fmt.Sprintf("Atribut tambahan kesehatan '%s' wajib diisi sesuai standar Metadata Mesh", field)
				}
			}
		}
	}

	return true, ""
}

// ==========================================
// 4. INGESTION PIPELINE (SCD TYPE 2)
// ==========================================
func (s *KesehatanService) ProcessIngestion(k models.RekamKesehatan) (string, error) {
	// --- EKSEKUSI BLOK VALIDASI SEBELUM MASUK DATABASE ---
	if err := s.ValidateInternalKesehatan(k); err != nil {
		return "Error Validation", err
	}

	if err := s.ValidateCrossDomainAPI(k); err != nil {
		return "Error Cross-Validation", err
	}
	// --- AKHIR BLOK VALIDASI ---

	last, err := s.Storage.GetLatestByNIK(k.NIK)

	// Skenario A: Data Kesehatan Baru (First Entry)
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
		k.ID = 0 // Reset ID untuk record baru di database
		k.Version = last.Version + 1
		k.AuditStatus = "PENDING" // Reset audit untuk setiap perubahan data

		if errCreate := s.Storage.Create(&k); errCreate != nil {
			return "Error", errCreate
		}
		return fmt.Sprintf("Sukses v%d (Kesehatan Updated)", k.Version), nil
	}

	return "Abaikan", fmt.Errorf("data kesehatan yang dikirim lebih usang dibandingkan data di mesh")
}

// ==========================================
// 5. UPDATE & AUDIT LOGIC
// ==========================================
func (s *KesehatanService) ProcessManualUpdate(nik string, newData models.RekamKesehatan) (int, error) {
	oldData, err := s.Storage.GetLatestByNIK(nik)
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