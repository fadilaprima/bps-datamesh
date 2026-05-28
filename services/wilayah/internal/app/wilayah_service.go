package app

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"wilayah/models"
	"wilayah/storage"

	"gorm.io/datatypes"
)

// WilayahService mengelola seluruh logika bisnis domain wilayah dengan arsitektur Data Mesh
type WilayahService struct {
	Storage storage.WilayahStorage
}

// 1. DOMAIN OWNER
// WilayahSourceRegistry
type SourceConfig struct {
	Name   string
	IsWali bool
}

// Map Angka -> Konfigurasi Source
var SourceMap = map[int]SourceConfig{
	1: {Name: "BPS", IsWali: true},
	2: {Name: "KEMENDAGRI", IsWali: true},
	3: {Name: "LAINNYA", IsWali: false},
}

// Map Angka -> Teks Verdict
var AuditMap = map[int]string{
	1: "VALID",
	2: "INVALID",
}

// 2. METADATA VALIDATOR (DYNAMIC SCHEMA VALIDATION)
// ValidateWilayahMetadata melakukan validasi isi data wilayah secara dinamis berdasarkan skema aktif
func (s *WilayahService) ValidateWilayahMetadata(w models.MasterWilayah, definition datatypes.JSON) (bool, string) {
	// 1. Parsing Aturan dari Skema Aktif di Database
	var schemaMap map[string]interface{}
	if err := json.Unmarshal(definition, &schemaMap); err != nil {
		return false, "Gagal membaca aturan metadata wilayah"
	}

	rules, ok := schemaMap["definition"].(map[string]interface{})
	if !ok { return true, "" }

	// 2. LOGIKA VALIDASI HIRARKI (Prov, Kab, Kec, Desa) - Ekstrak Struct Menggunakan Reflect
	val := reflect.ValueOf(w)
	typ := reflect.TypeOf(w)
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
				return false, fmt.Sprintf("Atribut wilayah '%s' wajib diisi (Mandatory)", jsonTag)
			}
			// B. Cek Panjang Karakter (Length)
			if lengthVal, ok := r["length"].(float64); ok {
				if fieldValue != "" && len(fieldValue) != int(lengthVal) {
					return false, fmt.Sprintf("Atribut '%s' tidak valid, harus %d digit sesuai standar MFD BPS", jsonTag, int(lengthVal))
				}
			}
		}
	}

	// 3. VALIDASI ATRIBUT TAMBAHAN (AdditionalInfo)
	var extra map[string]interface{}
	json.Unmarshal(w.AdditionalInfo, &extra)

	for field, rule := range rules {
		r, ok := rule.(map[string]interface{})
		if !ok { continue }
		
		// Jika dia required tapi BUKAN kolom fixed/struct, cari di extra data JSONB
		if r["required"] == true && !fixedFields[field] {
			if valData, exists := extra[field]; !exists || fmt.Sprintf("%v", valData) == "" {
				return false, fmt.Sprintf("Atribut tambahan wilayah '%s' wajib diisi sesuai standar Metadata Mesh", field)
			}
		}
	}

	return true, ""
}

// 3. CONFLICT RESOLUTION & SCD TYPE 2 (VERSIONING)
// ProcessIngestion mengelola alur SCD Type 2 (Versioning) untuk Domain Wilayah
func (s *WilayahService) ProcessIngestion(w models.MasterWilayah) (string, error) {
	// 1. Ambil versi terakhir berdasarkan KodeDesa (Natural Key)
	last, err := s.Storage.GetLatestByKode(w.KodeDesa)

	// Skenario A: Data Wilayah Baru (First Entry)
	if err != nil {
		w.Version = 1
		w.AuditStatus = "PENDING"
		if errCreate := s.Storage.Create(&w); errCreate != nil { return "Error", errCreate }
		return "Sukses v1 (Initial Entry)", nil
	}

	// Skenario B: Update Data (SCD Type 2)
	// Logika: Diterima jika ReferenceDate lebih baru ATAU (Tanggal sama tapi dari Wali Data)
	isNewer := w.ReferenceDate.After(last.ReferenceDate)
	isHigherAuthority := w.ReferenceDate.Equal(last.ReferenceDate) && w.IsWaliData && !last.IsWaliData

	if isNewer || isHigherAuthority {
		w.ID = 0 // Reset ID untuk record baru di database
		w.Version = last.Version + 1
		w.AuditStatus = "PENDING" // Reset audit untuk setiap perubahan data

		if errCreate := s.Storage.Create(&w); errCreate != nil { return "Error", errCreate }
		return fmt.Sprintf("Sukses v%d (Wilayah Updated)", w.Version), nil
	}

	return "Abaikan", fmt.Errorf("data wilayah yang dikirim lebih usang dibandingkan data di mesh")
}

// 4. KOREKSI & AUDIT (Manual Update & Audit Decision)
// Logika Bisnis untuk Koreksi Nilai PUT Dataset
func (s *WilayahService) ProcessManualUpdate(kodeDesa string, newData models.MasterWilayah) (int, error) {
	oldData, err := s.Storage.GetLatestByKode(kodeDesa)
	if err != nil || oldData == nil {
		return 0, fmt.Errorf("data asli tidak ditemukan")
	}

	// LOGIKA RESET SCD TYPE 2
	newData.ID = 0
	newData.Version = oldData.Version + 1
	newData.AuditStatus = "PENDING"
	newData.UpdatedAt = time.Now()

	// Skor kembali ke base (60 Sistem + 20 Sumber jika Walidata)
	if newData.IsWaliData {
		newData.TrustScore = 80.0
	} else {
		newData.TrustScore = 60.0
	}

	errCreate := s.Storage.Create(&newData)
	return newData.Version, errCreate
}

// ProcessAuditDecision memproses logika bisnis penentuan status validasi silang & bonus score
func (s *WilayahService) ProcessAuditDecision(kodeDesa []string, verdict int) (string, error) {
	verdictText := "INVALID"
	bonus := 0.0

	// Penerjemah Angka ke Teks & Logika Bonus
	if text, exists := AuditMap[verdict]; exists {
		verdictText = text
		if verdict == 1 { bonus = 20.0 }
	} else {
		return "", fmt.Errorf("verdict tidak valid. Gunakan 1 (VALID) atau 2 (INVALID)")
	}

	// Update ke Database
	err := s.Storage.UpdateBulkAuditDecision(kodeDesa, verdictText, bonus)
	return verdictText, err
}