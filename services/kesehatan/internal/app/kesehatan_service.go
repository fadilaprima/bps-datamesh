package app

import (
	"encoding/json"
	"fmt"
	"strings"
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

// 2. METADATA VALIDATOR (DYNAMIC SCHEMA VALIDATION)
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

	// 2. LOGIKA VALIDASI FIELD KESEHATAN
	// Mapping field struct ke key di JSON Metadata
	checkList := []struct {
		FieldName string
		Value     string
	}{
		{"nomor_induk_kependudukan", k.NIK},
		{"pbi_nas", k.PbiNas},
		{"pbi_pemda", k.PbiPemda},
		{"kondisi_gizi", k.KondisiGizi},
		{"penglihatan", k.Penglihatan},
		{"pendengaran", k.Pendengaran},
		{"berjalan_atau_naik_tangga", k.BerjalanNaikTangga},
		{"menggunakan_tangan_jari", k.MenggunakanTanganJari},
		{"belajar_kemampuan_intelektual", k.BelajarIntelektual},
		{"pengendalian_perilaku", k.PengendalianPerilaku},
		{"berbicara_komunikasi", k.BerbicaraKomunikasi},
		{"mengurus_diri", k.MengurusDiri},
		{"mengingat_berkonsentrasi", k.MengingatBerkonsentrasi},
		{"kesedihan_depresi", k.KesedihanDepresi},
		{"penyakit_kronis", k.PenyakitKronis},
	}

	for _, item := range checkList {
		if r, ok := rules[item.FieldName].(map[string]interface{}); ok {
			// A. Cek Mandatory (Required)
			if r["required"] == true && strings.TrimSpace(item.Value) == "" {
				return false, fmt.Sprintf("Atribut kesehatan '%s' wajib diisi (Mandatory)", item.FieldName)
			}

			// B. Cek Panjang Karakter (Length) - Berguna banget buat validasi NIK (16 Digit)
			if lengthVal, ok := r["length"].(float64); ok {
				if item.Value != "" && len(item.Value) != int(lengthVal) {
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