package app

import (
	"encoding/json"
	"energi/models"
	"energi/storage"
	"fmt"

	"gorm.io/datatypes"
)

type EnergiService struct {
	Storage storage.EnergiStorage
}

// EnergiSourceRegistry menentukan otoritas sumber data (PLN/ESDM)
var EnergiSourceRegistry = map[string]struct {
	IsWali bool
}{
	"PLN":  {IsWali: true},
	"ESDM": {IsWali: true},
	"BPS":  {IsWali: false},
}

// ValidateEnergiMetadata melakukan validasi isi data berdasarkan aturan di database
func (s *EnergiService) ValidateEnergiMetadata(p models.RekamEnergi, definition datatypes.JSON) (bool, string) {
	// 1. Parsing Aturan dari Skema Aktif
	var rulesMap map[string]interface{}
	if err := json.Unmarshal(definition, &rulesMap); err != nil {
		return false, "Gagal membaca aturan metadata dari skema"
	}

	rules, hasRules := rulesMap["rules"].(map[string]interface{})
	if !hasRules {
		return true, ""
	}

	// 2. VALIDASI NOMOR KARTU KELUARGA (Identik dengan NIK)
	if r, ok := rules["nomor_kartu_keluarga"].(map[string]interface{}); ok {
		if lengthVal, ok := r["length"].(float64); ok {
			if len(p.NoKK) != int(lengthVal) {
				return false, fmt.Sprintf("Nomor KK harus %d digit (Aturan Skema Aktif)", int(lengthVal))
			}
		}
	}

	// 3. VALIDASI ATRIBUT TAMBAHAN DI KANTONG AJAIB (AdditionalInfo)
	var extra map[string]interface{}
	json.Unmarshal(p.AdditionalInfo, &extra)

	for field, rule := range rules {
		r, ok := rule.(map[string]interface{})
		if !ok {
			continue
		}

		if r["required"] == true {
			// Daftar variabel tetap di domain Energi
			isFixedColumn := (field == "nomor_kartu_keluarga" || field == "id_pelanggan_pln" || field == "daya_terpasang")

			if !isFixedColumn {
				if val, exists := extra[field]; !exists || val == "" {
					return false, fmt.Sprintf("Atribut tambahan '%s' wajib diisi sesuai skema", field)
				}
			}
		}
	}

	return true, ""
}

// ProcessIngestion mengelola logika SCD Type 2 (Versioning)
func (s *EnergiService) ProcessIngestion(p models.RekamEnergi) (string, error) {
	// 1. Cari data terakhir di Mesh untuk NoKK ini
	last, err := s.Storage.GetLatestByNoKK(p.NoKK)

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

	return "Abaikan", fmt.Errorf("data energi lebih lama dibandingkan data di database")
}
