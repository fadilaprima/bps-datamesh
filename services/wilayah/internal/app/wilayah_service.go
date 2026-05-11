package app

import (
	"encoding/json"
	"fmt"
	"strings"
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
var WilayahSourceRegistry = map[string]struct {
	IsWali bool
}{
	"BPS":        {IsWali: true},  // Wali Data Statistik (MFD)
	"KEMENDAGRI": {IsWali: true},  // Wali Data Administrasi (Kode & Data Wilayah)
	"BIG":        {IsWali: false}, // Sumber Data Geospasial
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
	if !ok {
		return true, "" // Lolos jika definisi skema kosong
	}

	// 2. LOGIKA VALIDASI HIRARKI (Prov, Kab, Kec, Desa)
	// Mapping field struct ke key di JSON Metadata
	checkList := []struct {
		FieldName string
		Value     string
	}{
		{"kode_prov", w.KodeProv}, // 2 digit
		{"kode_kab", w.KodeKab},   // 4 digit
		{"kode_kec", w.KodeKec},   // 7 digit
		{"kode_desa", w.KodeDesa}, // 10 digit (Natural Key)
		{"provinsi", w.Provinsi},
		{"kabupaten", w.Kabupaten},
		{"kecamatan", w.Kecamatan},
		{"desa", w.Desa},
	}

	for _, item := range checkList {
		if r, ok := rules[item.FieldName].(map[string]interface{}); ok {
			// A. Cek Mandatory (Required)
			if r["required"] == true && strings.TrimSpace(item.Value) == "" {
				return false, fmt.Sprintf("Atribut wilayah '%s' wajib diisi (Mandatory)", item.FieldName)
			}

			// B. Cek Panjang Karakter (Length) - Dinamis menggantikan Hardcode 2, 4, 7, 10
			if lengthVal, ok := r["length"].(float64); ok {
				if item.Value != "" && len(item.Value) != int(lengthVal) {
					return false, fmt.Sprintf("Atribut '%s' tidak valid, harus %d digit sesuai standar MFD BPS", item.FieldName, int(lengthVal))
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

		if r["required"] == true {
			// Cek apakah field ini termasuk kolom fisik tetap (fixed columns)
			isFixed := false
			for _, item := range checkList {
				if item.FieldName == field { isFixed = true; break }
			}

			// Jika diwajibkan tapi tidak ada di kolom fisik, cari di Additional Info
			if !isFixed {
				if val, exists := extra[field]; !exists || val == "" {
					return false, fmt.Sprintf("Atribut tambahan wilayah '%s' wajib diisi sesuai standar Metadata Mesh", field)
				}
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
		if errCreate := s.Storage.Create(&w); errCreate != nil {
			return "Error", errCreate
		}
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
		
		if errCreate := s.Storage.Create(&w); errCreate != nil {
			return "Error", errCreate
		}
		return fmt.Sprintf("Sukses v%d (Wilayah Updated)", w.Version), nil
	}

	return "Abaikan", fmt.Errorf("data wilayah yang dikirim lebih usang dibandingkan data di mesh")
}