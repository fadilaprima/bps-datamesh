package app

import (
	"encoding/json"
	"fmt"
	"ketenagakerjaan/models"
	"ketenagakerjaan/storage"

	"gorm.io/datatypes"
)

type KetenagakerjaanService struct {
	Storage storage.KetenagakerjaanStorage
}

var KetenagakerjaanSourceRegistry = map[string]struct {
	IsWali bool
}{
	"BPJS_KETENAGAKERJAAN": {IsWali: true},
	"BPS":                  {IsWali: false},
}

func (s *KetenagakerjaanService) ValidateKetenagakerjaanMetadata(p models.RekamKetenagakerjaan, definition datatypes.JSON) (bool, string) {
	var rulesMap map[string]interface{}
	json.Unmarshal(definition, &rulesMap)
	rules, hasRules := rulesMap["rules"].(map[string]interface{})
	if !hasRules {
		return true, ""
	}

	if r, ok := rules["nik"].(map[string]interface{}); ok {
		if lengthVal, ok := r["length"].(float64); ok {
			if len(p.NIK) != int(lengthVal) {
				return false, fmt.Sprintf("NIK harus %d digit", int(lengthVal))
			}
		}
	}

	var extra map[string]interface{}
	json.Unmarshal(p.AdditionalInfo, &extra)

	for field, rule := range rules {
		r, ok := rule.(map[string]interface{})
		if !ok || r["required"] != true {
			continue
		}

		isFixedColumn := (field == "nik" || field == "status_bekerja" || field == "lapangan_usaha_dari_pekerjaan_utama" ||
			field == "status_dalam_pekerjaan_utama" || field == "kepemilikan_usaha" || field == "jumlah_usaha" ||
			field == "lapangan_usaha_dari_usaha_utama" || field == "jumlah_pekerja_yang_dibayar_dari_usaha_utama" ||
			field == "jumlah_pekerja_yang_tidak_dibayar_dari_usaha_utama" || field == "omzet_usaha_utama")

		if !isFixedColumn {
			if val, exists := extra[field]; !exists || val == "" {
				return false, fmt.Sprintf("Atribut tambahan '%s' wajib diisi", field)
			}
		}
	}
	return true, ""
}

func (s *KetenagakerjaanService) ProcessIngestion(p models.RekamKetenagakerjaan) (string, error) {
	last, err := s.Storage.GetLatestByNIK(p.NIK)
	if err != nil {
		p.Version = 1
		p.AuditStatus = "PENDING"
		s.Storage.Create(&p)
		return "Sukses v1", nil
	}
	if p.ReferenceDate.After(last.ReferenceDate) || (p.ReferenceDate.Equal(last.ReferenceDate) && p.IsWaliData && !last.IsWaliData) {
		p.ID = 0
		p.Version = last.Version + 1
		p.AuditStatus = "PENDING"
		s.Storage.Create(&p)
		return fmt.Sprintf("Sukses v%d", p.Version), nil
	}
	return "Abaikan", fmt.Errorf("data usang")
}
