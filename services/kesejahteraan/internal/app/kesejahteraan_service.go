package app

import (
	"encoding/json"
	"fmt"
	"kesejahteraan/models"
	"kesejahteraan/storage"
	"strings"

	"gorm.io/datatypes"
)

type KesejahteraanService struct {
	Storage storage.KesejahteraanStorage
}

// KesejahteraanSourceRegistry: Otoritas sumber data (Kemensos sebagai Wali Data)
var KesejahteraanSourceRegistry = map[string]struct {
	IsWali bool
}{
	"KEMENSOS": {IsWali: true},
	"BPS":      {IsWali: false},
}

// ValidateKesejahteraanMetadata: Validasi dinamis 25 variabel Kemensos
func (s *KesejahteraanService) ValidateKesejahteraanMetadata(p models.RekamKesejahteraan, definition datatypes.JSON) (bool, string) {
	var rulesMap map[string]interface{}
	if err := json.Unmarshal(definition, &rulesMap); err != nil {
		return false, "Gagal membaca aturan metadata dari skema"
	}

	rules, hasRules := rulesMap["rules"].(map[string]interface{})
	if !hasRules {
		return true, ""
	}

	// 1. VALIDASI NOMOR KARTU KELUARGA (Length 16)
	if r, ok := rules["nomor_kartu_keluarga"].(map[string]interface{}); ok {
		if lengthVal, ok := r["length"].(float64); ok {
			if len(p.NoKK) != int(lengthVal) {
				return false, fmt.Sprintf("Nomor KK harus %d digit", int(lengthVal))
			}
		}
	}

	// 2. VALIDASI DESIL NASIONAL (Contoh: Max 100)
	if r, ok := rules["desil_nasional"].(map[string]interface{}); ok {
		if maxVal, ok := r["max"].(float64); ok {
			if p.DesilNasional > int(maxVal) {
				return false, fmt.Sprintf("Desil nasional tidak boleh lebih dari %d", int(maxVal))
			}
		}
	}

	// 3. VALIDASI ATRIBUT TAMBAHAN & PENGECEKAN KOLOM TETAP (25 ATRIBUT)
	var extra map[string]interface{}
	json.Unmarshal(p.AdditionalInfo, &extra)

	for field, rule := range rules {
		r, ok := rule.(map[string]interface{})
		if !ok {
			continue
		}

		if r["required"] == true {
			// DAFTAR ATRIBUT LENGKAP: Memastikan 25 variabel ERD tidak dianggap data extra
			isFixedColumn := (field == "nomor_kartu_keluarga" ||
				field == "desil_nasional" ||
				field == "bahan_bakar_utama_memasak" ||
				field == "kepemilikan_aset" ||
				strings.HasPrefix(field, "aset_bergerak_") ||
				strings.HasPrefix(field, "aset_tidak_bergerak_") ||
				strings.HasPrefix(field, "jumlah_ternak_"))

			if !isFixedColumn {
				// Jika field required tapi tidak ada di struct utama, maka harus ada di AdditionalInfo
				if val, exists := extra[field]; !exists || val == "" {
					return false, fmt.Sprintf("Atribut tambahan '%s' wajib diisi sesuai skema", field)
				}
			}
		}
	}

	return true, ""
}

// ProcessIngestion: Logika SCD Type 2 Berbasis Nomor KK
func (s *KesejahteraanService) ProcessIngestion(p models.RekamKesejahteraan) (string, error) {
	// Ambil record terakhir berdasarkan NoKK
	last, err := s.Storage.GetLatestByNoKK(p.NoKK)

	// Jika data v1 (Baru masuk Mesh)
	if err != nil {
		p.Version = 1
		p.AuditStatus = "PENDING"
		if errCreate := s.Storage.Create(&p); errCreate != nil {
			return "Error", errCreate
		}
		return "Sukses v1", nil
	}

	// Logika Anti-Regresi (Hanya terima data yang lebih baru secara waktu atau otoritas)
	isNewer := p.ReferenceDate.After(last.ReferenceDate)
	isHigherAuthority := p.ReferenceDate.Equal(last.ReferenceDate) && p.IsWaliData && !last.IsWaliData

	if isNewer || isHigherAuthority {
		p.ID = 0
		p.Version = last.Version + 1
		p.AuditStatus = "PENDING" // Reset status audit setiap ada versi baru

		if errCreate := s.Storage.Create(&p); errCreate != nil {
			return "Error", errCreate
		}
		return fmt.Sprintf("Sukses v%d", p.Version), nil
	}

	return "Abaikan", fmt.Errorf("data kesejahteraan masuk ditolak: versi %s sudah ada yang lebih baru", p.NoKK)
}
