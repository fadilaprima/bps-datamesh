package app

import (
	"encoding/json"
	"fmt"
	"ketenagakerjaan/models"
	"ketenagakerjaan/storage"
	"strings"

	"gorm.io/datatypes"
)

// KetenagakerjaanService mengelola seluruh logika bisnis domain ketenagakerjaan dengan arsitektur Data Mesh
type KetenagakerjaanService struct {
	Storage storage.KetenagakerjaanStorage
}

// 1. DOMAIN OWNER
// KetenagakerjaanSourceRegistry
type SourceConfig struct {
	Name   string
	IsWali bool
}

// Map Angka -> Konfigurasi Source
var SourceMap = map[int]SourceConfig{
	1: {Name: "BPS", IsWali: true},
	2: {Name: "BPJS_KETENAGAKERJAAN", IsWali: true}, // Wali Data Ketenagakerjaan
	3: {Name: "LAINNYA", IsWali: false},
}

// Map Angka -> Teks Verdict
var AuditMap = map[int]string{
	1: "VALID",
	2: "INVALID",
}

// 2. METADATA VALIDATOR (DYNAMIC SCHEMA VALIDATION)
// ValidateKetenagakerjaanMetadata melakukan validasi isi data secara dinamis berdasarkan skema aktif
func (s *KetenagakerjaanService) ValidateKetenagakerjaanMetadata(k models.RekamKetenagakerjaan, definition datatypes.JSON) (bool, string) {
	// 1. Parsing Aturan dari Skema Aktif di Database (Pakai key "definition" seperti Wilayah)
	var schemaMap map[string]interface{}
	if err := json.Unmarshal(definition, &schemaMap); err != nil {
		return false, "Gagal membaca aturan metadata ketenagakerjaan"
	}

	rules, ok := schemaMap["definition"].(map[string]interface{})
	if !ok {
		return true, ""
	}

	// 2. LOGIKA VALIDASI FIELD KETENAGAKERJAAN
	// Mapping field struct ke key di JSON Metadata
	checkList := []struct {
		FieldName string
		Value     string
	}{
		{"nomor_induk_kependudukan", k.NIK},
		{"status_bekerja", k.StatusBekerja},
		{"lapangan_usaha_dari_pekerjaan_utama", k.LapanganUsahaUtama},
		{"status_dalam_pekerjaan_utama", k.StatusDalamPekerjaanUtama},
		{"kepemilikan_usaha", k.KepemilikanUsaha},
		{"jumlah_usaha", fmt.Sprint(k.JumlahUsaha)},
		{"lapangan_usaha_dari_usaha_utama", k.LapanganUsahaPekerjaanUtama},
		{"jumlah_pekerja_yang_dibayar_dari_usaha_utama", fmt.Sprint(k.JumlahPekerjaDibayar)},
		{"jumlah_pekerja_yang_tidak_dibayar_dari_usaha_utama", fmt.Sprint(k.JumlahPekerjaTidakDibayar)},
		{"omzet_usaha_utama", fmt.Sprintf("%.0f", k.OmzetUsahaUtama)},
	}

	for _, item := range checkList {
		if r, ok := rules[item.FieldName].(map[string]interface{}); ok {
			// A. Cek Mandatory (Required)
			// (Abaikan string "0" dari konversi numerik jika memang kosong/null di logicmu)
			if r["required"] == true && strings.TrimSpace(item.Value) == "" {
				return false, fmt.Sprintf("Atribut ketenagakerjaan '%s' wajib diisi (Mandatory)", item.FieldName)
			}

			// B. Cek Panjang Karakter (Length) - Khususnya untuk NIK (16 digit)
			if lengthVal, ok := r["length"].(float64); ok {
				if item.Value != "" && item.Value != "0" && len(item.Value) != int(lengthVal) {
					return false, fmt.Sprintf("Atribut '%s' tidak valid, harus %d digit sesuai standar", item.FieldName, int(lengthVal))
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
					return false, fmt.Sprintf("Atribut tambahan ketenagakerjaan '%s' wajib diisi sesuai standar Metadata Mesh", field)
				}
			}
		}
	}

	return true, ""
}

// 3. CONFLICT RESOLUTION & SCD TYPE 2 (VERSIONING)
// ProcessIngestion mengelola alur SCD Type 2 (Versioning) untuk Domain Ketenagakerjaan
func (s *KetenagakerjaanService) ProcessIngestion(k models.RekamKetenagakerjaan) (string, error) {
	// 1. Ambil versi terakhir berdasarkan NIK (Natural Key)
	last, err := s.Storage.GetLatestByNIK(k.NIK)

	// Skenario A: Data Ketenagakerjaan Baru (First Entry)
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
		return fmt.Sprintf("Sukses v%d (Ketenagakerjaan Updated)", k.Version), nil
	}

	return "Abaikan", fmt.Errorf("data ketenagakerjaan yang dikirim lebih usang dibandingkan data di mesh")
}
