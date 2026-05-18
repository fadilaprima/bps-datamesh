package app

import (
	"encoding/json"
	"energi/models"
	"energi/storage"
	"fmt"
	"strings"

	"gorm.io/datatypes"
)

// EnergiService mengelola seluruh logika bisnis domain energi dengan arsitektur Data Mesh
type EnergiService struct {
	Storage storage.EnergiStorage
}

// 1. DOMAIN OWNER
// EnergiSourceRegistry
type SourceConfig struct {
	Name   string
	IsWali bool
}

// Map Angka -> Konfigurasi Source
var SourceMap = map[int]SourceConfig{
	1: {Name: "BPS", IsWali: true},
	2: {Name: "PLN", IsWali: true},  // Wali Data Energi
	3: {Name: "ESDM", IsWali: true}, // Wali Data Energi
	4: {Name: "LAINNYA", IsWali: false},
}

// Map Angka -> Teks Verdict
var AuditMap = map[int]string{
	1: "VALID",
	2: "INVALID",
}

// 2. METADATA VALIDATOR (DYNAMIC SCHEMA VALIDATION)
// ValidateEnergiMetadata melakukan validasi isi data secara dinamis berdasarkan skema aktif
func (s *EnergiService) ValidateEnergiMetadata(k models.RekamEnergi, definition datatypes.JSON) (bool, string) {
	// 1. Parsing Aturan dari Skema Aktif di Database
	var schemaMap map[string]interface{}
	if err := json.Unmarshal(definition, &schemaMap); err != nil {
		return false, "Gagal membaca aturan metadata energi"
	}

	rules, ok := schemaMap["definition"].(map[string]interface{})
	if !ok {
		return true, ""
	}

	// 2. LOGIKA VALIDASI FIELD ENERGI
	// Mapping field struct ke key di JSON Metadata
	checkList := []struct {
		FieldName string
		Value     string
	}{
		{"nomor_kartu_keluarga", k.NoKK},
		{"id_pelanggan_pln", k.IDPelangganPLN},
		{"daya_terpasang", k.DayaTerpasang},
	}

	for _, item := range checkList {
		if r, ok := rules[item.FieldName].(map[string]interface{}); ok {
			// A. Cek Mandatory (Required)
			if r["required"] == true && strings.TrimSpace(item.Value) == "" {
				return false, fmt.Sprintf("Atribut energi '%s' wajib diisi (Mandatory)", item.FieldName)
			}

			// B. Cek Panjang Karakter (Length) - Berguna buat Nomor KK
			if lengthVal, ok := r["length"].(float64); ok {
				if item.Value != "" && len(item.Value) != int(lengthVal) {
					return false, fmt.Sprintf("Atribut '%s' tidak valid, harus %d digit sesuai standar", item.FieldName, int(lengthVal))
				}
			}
		}
	}

	// 3. VALIDASI ATRIBUT TAMBAEM (AdditionalInfo)
	var extra map[string]interface{}
	json.Unmarshal(k.AdditionalInfo, &extra)

	for field, rule := range rules {
		r, ok := rule.(map[string]interface{})
		if !ok {
			continue
		}

		if r["required"] == true {
			// Cek apakah field ini termasuk kolom fisik tetap (fixed columns)
			isFixed := false
			for _, item := range checkList {
				if item.FieldName == field {
					isFixed = true
					break
				}
			}

			// Jika diwajibkan tapi tidak ada di kolom fisik, cari di Additional Info
			if !isFixed {
				if val, exists := extra[field]; !exists || val == "" {
					return false, fmt.Sprintf("Atribut tambahan energi '%s' wajib diisi sesuai standar Metadata Mesh", field)
				}
			}
		}
	}

	return true, ""
}

// 3. CONFLICT RESOLUTION & SCD TYPE 2 (VERSIONING)
// ProcessIngestion mengelola alur SCD Type 2 untuk Domain Energi
func (s *EnergiService) ProcessIngestion(k models.RekamEnergi) (string, error) {
	// 1. Ambil versi terakhir berdasarkan NoKK (Natural Key)
	last, err := s.Storage.GetLatestByNoKK(k.NoKK)

	// Skenario A: Data Energi Baru (First Entry)
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
		return fmt.Sprintf("Sukses v%d (Energi Updated)", k.Version), nil
	}

	return "Abaikan", fmt.Errorf("data energi yang dikirim lebih usang dibandingkan data di mesh")
}