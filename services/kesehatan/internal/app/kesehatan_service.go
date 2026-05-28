package app

import (
	"encoding/json"
	"fmt"
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
// KesehatanSourceRegistry
type SourceConfig struct {
	Name   string
	IsWali bool
}

// Map Angka -> Konfigurasi Source (Sesuaikan dengan Handler nanti)
var SourceMap = map[int]SourceConfig{
	1: {Name: "BPS", IsWali: true},
	2: {Name: "KEMENKES", IsWali: true},       // Wali Data
	3: {Name: "BPJS_KESEHATAN", IsWali: true}, // Wali Data
	4: {Name: "DINKES", IsWali: true},         // Kepanjangan Wali Data
	5: {Name: "LAINNYA", IsWali: false},
}

// Map Angka -> Teks Verdict
var AuditMap = map[int]string{
	1: "VALID",
	2: "INVALID",
}

// 2. METADATA VALIDATOR (DYNAMIC SCHEMA VALIDATION WITH REFLECT)
func (s *KesehatanService) ValidateKesehatanMetadata(k models.RekamKesehatan, definition datatypes.JSON) (bool, string) {
	// 1. Parsing Aturan dari Skema Aktif di Database
	var schemaMap map[string]interface{}
	if err := json.Unmarshal(definition, &schemaMap); err != nil {
		return false, "Gagal membaca aturan metadata kesehatan"
	}

	rules, ok := schemaMap["definition"].(map[string]interface{})
	if !ok {
		return true, ""
	}

	// 2. LOGIKA VALIDASI FIELD KESEHATAN (Dinamis pakai Reflect)
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
			// Jika diwajibkan tapi tidak ada di kolom fisik, cari di Additional Info
			if !fixedFields[field] {
				if valData, exists := extra[field]; !exists || fmt.Sprintf("%v", valData) == "" {
					return false, fmt.Sprintf("Atribut tambahan kesehatan '%s' wajib diisi sesuai standar Metadata Mesh", field)
				}
			}
		}
	}

	return true, ""
}

// 3. CONFLICT RESOLUTION & SCD TYPE 2 (VERSIONING)
// ProcessIngestion mengelola alur SCD Type 2 untuk Domain Kesehatan
func (s *KesehatanService) ProcessIngestion(k models.RekamKesehatan) (string, error) {
	// 1. Ambil versi terakhir berdasarkan NIK (Natural Key)
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
	// Logika: Diterima jika ReferenceDate lebih baru ATAU (Tanggal sama tapi dari Wali Data)
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

// Logika Bisnis untuk Manual Update dari PUT Dataset
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

// Logika Bisnis Audit
func (s *KesehatanService) ProcessAuditDecision(nikList []string, verdict int) (string, error) {
	verdictText := "INVALID"
	bonus := 0.0

	// Penerjemah Angka ke Teks & Logika Bonus
	if text, exists := AuditMap[verdict]; exists {
		verdictText = text
		if verdict == 1 { bonus = 20.0 }
	} else {
		return "", fmt.Errorf("Verdict tidak valid. Gunakan 1 (VALID) atau 2 (INVALID)")
	}

	err := s.Storage.UpdateBulkAuditDecision(nikList, verdictText, bonus)
	return verdictText, err
}