package app

import (
	"encoding/json"
	"fmt"
	"hunian/models"
	"hunian/storage"

	"gorm.io/datatypes"
)

type HunianService struct {
	Storage storage.HunianStorage
}

// HunianSourceRegistry menentukan otoritas sumber data (Identik)
var HunianSourceRegistry = map[string]struct {
	IsWali bool
}{
	"PUPR":        {IsWali: true},
	"PLN":         {IsWali: true},
	"BPS":         {IsWali: false},
	"NGANJUK_KAB": {IsWali: true},
}

// ValidateHunianMetadata melakukan validasi isi data berdasarkan aturan di database (Identik)
func (s *HunianService) ValidateHunianMetadata(p models.RekamHunian, definition datatypes.JSON) (bool, string) {
	// 1. Parsing Aturan dari Skema Aktif
	var rulesMap map[string]interface{}
	if err := json.Unmarshal(definition, &rulesMap); err != nil {
		return false, "Gagal membaca aturan metadata dari skema"
	}

	rules, hasRules := rulesMap["rules"].(map[string]interface{})
	if !hasRules {
		return true, ""
	}

	// 2. VALIDASI NOMOR KARTU KELUARGA (Identik dengan Logika NIK)
	if r, ok := rules["nomor_kartu_keluarga"].(map[string]interface{}); ok {
		if lengthVal, ok := r["length"].(float64); ok {
			if len(p.NoKK) != int(lengthVal) {
				return false, fmt.Sprintf("Nomor KK harus %d digit (Aturan Skema Aktif)", int(lengthVal))
			}
		}
	}

	// 3. VALIDASI LUAS LANTAI (Identik dengan Logika Jenjang/Max)
	if r, ok := rules["luas_lantai"].(map[string]interface{}); ok {
		if maxVal, ok := r["max"].(float64); ok {
			if p.LuasLantai > int(maxVal) {
				return false, fmt.Sprintf("Luas lantai hunian tidak boleh lebih dari %d", int(maxVal))
			}
		}
	}

	// 4. VALIDASI ATRIBUT TAMBAHAN DI KANTONG AJAIB (Identik)
	var extra map[string]interface{}
	json.Unmarshal(p.AdditionalInfo, &extra)

	for field, rule := range rules {
		r, ok := rule.(map[string]interface{})
		if !ok {
			continue
		}

		if r["required"] == true {
			// Daftar seluruh variabel tetap di domain Hunian agar tidak dianggap extra data
			isFixedColumn := (field == "nomor_kartu_keluarga" || field == "status_kepemilikan_rumah" ||
				field == "jenis_lantai_terluas" || field == "luas_lantai" || field == "jenis_dinding_terluas" ||
				field == "jenis_atap_terluas" || field == "sumber_air_minum_utama" ||
				field == "sumber_penerangan_utama" || field == "fasilitas_bab" ||
				field == "jenis_kloset" || field == "pembuangan_akhir_tinja")

			if !isFixedColumn {
				if val, exists := extra[field]; !exists || val == "" {
					return false, fmt.Sprintf("Atribut tambahan '%s' wajib diisi sesuai skema", field)
				}
			}
		}
	}

	return true, ""
}

// ProcessIngestion mengelola logika SCD Type 2 (Identik)
func (s *HunianService) ProcessIngestion(p models.RekamHunian) (string, error) {
	// 1. Cari data terakhir di Mesh untuk NoKK ini
	last, err := s.Storage.GetLatestByNoKK(p.NoKK)

	// Jika data belum pernah ada (v1)
	if err != nil {
		p.Version = 1
		p.AuditStatus = "PENDING"
		if errCreate := s.Storage.Create(&p); errCreate != nil {
			return "Error", errCreate
		}
		return "Sukses v1", nil
	}

	// 2. LOGIKA ANTI-REGRESI (SCD Type 2)
	isNewer := p.ReferenceDate.After(last.ReferenceDate)
	isHigherAuthority := p.ReferenceDate.Equal(last.ReferenceDate) && p.IsWaliData && !last.IsWaliData

	if isNewer || isHigherAuthority {
		p.ID = 0
		p.Version = last.Version + 1
		p.AuditStatus = "PENDING"

		if errCreate := s.Storage.Create(&p); errCreate != nil {
			return "Error", errCreate
		}
		return fmt.Sprintf("Sukses v%d", p.Version), nil
	}

	return "Abaikan", fmt.Errorf("data lebih lama dibandingkan data di database")
}
