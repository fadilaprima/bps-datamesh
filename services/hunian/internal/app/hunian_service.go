package app

import (
	"encoding/json"
	"fmt"
	"strings"
	"hunian/models"
	"hunian/storage"

	"gorm.io/datatypes"
)

// HunianService mengelola seluruh logika bisnis domain hunian dengan arsitektur Data Mesh
type HunianService struct {
	Storage storage.HunianStorage
}

// 1. DOMAIN OWNER
// HunianSourceRegistry
type SourceConfig struct {
	Name   string
	IsWali bool
}

// Map Angka -> Konfigurasi Source
var SourceMap = map[int]SourceConfig{
	1: {Name: "BPS", IsWali: true},
	2: {Name: "PUPR", IsWali: true},        
	3: {Name: "PKP", IsWali: true},         
	5: {Name: "LAINNYA", IsWali: false},
}

// Map Angka -> Teks Verdict
var AuditMap = map[int]string{
	1: "VALID",
	2: "INVALID",
}

// 2. METADATA VALIDATOR (DYNAMIC SCHEMA VALIDATION)
// ValidateHunianMetadata melakukan validasi isi data secara dinamis berdasarkan skema aktif
func (s *HunianService) ValidateHunianMetadata(k models.RekamHunian, definition datatypes.JSON) (bool, string) {
	// 1. Parsing Aturan dari Skema Aktif di Database
	var schemaMap map[string]interface{}
	if err := json.Unmarshal(definition, &schemaMap); err != nil {
		return false, "Gagal membaca aturan metadata hunian"
	}

	rules, ok := schemaMap["definition"].(map[string]interface{})
	if !ok {
		return true, ""
	}

	// 2. LOGIKA VALIDASI FIELD HUNIAN
	// Mapping field struct ke key di JSON Metadata
	checkList := []struct {
		FieldName string
		Value     string
	}{
		{"nomor_kartu_keluarga", k.NoKK},
		{"status_kepemilikan_rumah", k.StatusKepemilikan},
		{"jenis_lantai_terluas", k.JenisLantai},
		{"luas_lantai", fmt.Sprint(k.LuasLantai)}, // Konversi int ke string
		{"jenis_dinding_terluas", k.JenisDinding},
		{"jenis_atap_terluas", k.JenisAtap},
		{"sumber_air_minum_utama", k.SumberAirMinum},
		{"sumber_penerangan_utama", k.SumberPenerangan},
		{"fasilitas_bab", k.FasilitasBAB},
		{"jenis_kloset", k.JenisKloset},
		{"pembuangan_akhir_tinja", k.PembuanganTinja},
	}

	for _, item := range checkList {
		if r, ok := rules[item.FieldName].(map[string]interface{}); ok {
			// A. Cek Mandatory (Required)
			if r["required"] == true && strings.TrimSpace(item.Value) == "" {
				return false, fmt.Sprintf("Atribut hunian '%s' wajib diisi (Mandatory)", item.FieldName)
			}

			// B. Cek Panjang Karakter (Length) - Berguna buat Nomor KK (16 digit)
			if lengthVal, ok := r["length"].(float64); ok {
				if item.Value != "" && len(item.Value) != int(lengthVal) {
					return false, fmt.Sprintf("Atribut '%s' tidak valid, harus %d digit sesuai standar", item.FieldName, int(lengthVal))
				}
			}
			
			// C. Cek Batas Maksimum (Max) - Khusus untuk Luas Lantai
			if maxVal, ok := r["max"].(float64); ok {
				var intVal int
				fmt.Sscanf(item.Value, "%d", &intVal)
				if intVal > int(maxVal) {
					return false, fmt.Sprintf("Atribut '%s' tidak boleh lebih dari %d", item.FieldName, int(maxVal))
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
					return false, fmt.Sprintf("Atribut tambahan hunian '%s' wajib diisi sesuai standar Metadata Mesh", field)
				}
			}
		}
	}

	return true, ""
}

// 3. CONFLICT RESOLUTION & SCD TYPE 2 (VERSIONING)
// ProcessIngestion mengelola alur SCD Type 2 untuk Domain Hunian
func (s *HunianService) ProcessIngestion(k models.RekamHunian) (string, error) {
	// 1. Ambil versi terakhir berdasarkan NoKK (Natural Key)
	last, err := s.Storage.GetLatestByNoKK(k.NoKK)

	// Skenario A: Data Hunian Baru (First Entry)
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
		return fmt.Sprintf("Sukses v%d (Hunian Updated)", k.Version), nil
	}

	return "Abaikan", fmt.Errorf("data hunian yang dikirim lebih usang dibandingkan data di mesh")
}