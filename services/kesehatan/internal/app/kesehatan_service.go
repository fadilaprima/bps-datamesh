package app

import (
	"encoding/json"
	"fmt"
	"kesehatan/models"
	"kesehatan/storage" 
	"strconv"

	"gorm.io/datatypes"
)


type KesehatanService struct {
	Storage storage.KesehatanStorage
}


var KesehatanSourceRegistry = map[string]struct {
	IsWali bool
}{
	"BPJS KESEHATAN": {IsWali: true},
	"KEMENKES":      {IsWali: true},
	"DINKES":        {IsWali: true},
	"BPS":           {IsWali: false},
}

// ValidateKesehatanMetadata Identik dengan Pendidikan
func (s *KesehatanService) ValidateKesehatanMetadata(p models.RekamKesehatan, definition datatypes.JSON) (bool, string) {
	var rulesMap map[string]interface{}
	if err := json.Unmarshal(definition, &rulesMap); err != nil {
		return false, "Gagal membaca aturan metadata dari skema"
	}

	rules, hasRules := rulesMap["rules"].(map[string]interface{})
	if !hasRules {
		return true, "" 
	}

	// 2. VALIDASI NIK
	if r, ok := rules["nik"].(map[string]interface{}); ok {
		if lengthVal, ok := r["length"].(float64); ok {
			if len(p.NIK) != int(lengthVal) {
				return false, fmt.Sprintf("NIK harus %d digit (Aturan Skema Aktif)", int(lengthVal))
			}
		}
	}

	// 3. VALIDASI KONDISI GIZI 
	if r, ok := rules["kondisi_gizi"].(map[string]interface{}); ok {
		if maxVal, ok := r["max"].(float64); ok {
			val, _ := strconv.Atoi(p.KondisiGizi)
			if val > int(maxVal) {
				return false, fmt.Sprintf("Kode kondisi gizi tidak boleh lebih dari %d", int(maxVal))
			}
		}
	}

	// 4. VALIDASI ATRIBUT TAMBAHAN DI AdditionalInfo
	var extra map[string]interface{}
	json.Unmarshal(p.AdditionalInfo, &extra)

	for field, rule := range rules {
		r, ok := rule.(map[string]interface{})
		if !ok { continue }

		if r["required"] == true {
			// Daftar seluruh variabel tetap di domain Kesehatan (Sync dengan model)
			isFixedColumn := (field == "nik" || field == "pbi_nas" || field == "pbi_pemda" || 
				field == "kondisi_gizi" || field == "penglihatan" || field == "pendengaran" || 
				field == "berjalan_atau_naik_tangga" || field == "menggunakan_tangan_jari" || 
				field == "belajar_kemampuan_intelektual" || field == "pengendalian_perilaku" || 
				field == "berbicara_komunikasi" || field == "mengurus_diri" || 
				field == "mengingat_berkonsentrasi" || field == "kesedihan_depresi" || 
				field == "penyakit_kronis")
			
			if !isFixedColumn {
				if val, exists := extra[field]; !exists || val == "" {
					return false, fmt.Sprintf("Atribut tambahan '%s' wajib diisi sesuai skema", field)
				}
			}
		}
	}

	return true, ""
}

// ProcessIngestion Identik dengan Pendidikan
func (s *KesehatanService) ProcessIngestion(p models.RekamKesehatan) (string, error) {
	last, err := s.Storage.GetLatestByNIK(p.NIK)

	if err != nil {
		p.Version = 1
		p.AuditStatus = "PENDING"
		if errCreate := s.Storage.Create(&p); errCreate != nil {
			return "Error", errCreate
		}
		return "Sukses v1", nil
	}

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