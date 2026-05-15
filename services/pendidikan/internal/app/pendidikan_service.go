package app

import (
	"encoding/json"
	"fmt"
	"pendidikan/models"
	"pendidikan/storage"
	"strconv"

	"gorm.io/datatypes"
)

type PendidikanService struct {
	Storage storage.PendidikanStorage
}

// Kamus Sumber Data Khusus Pendidikan
type SourceConfig struct {
	Name   string
	IsWali bool
}

var SourceMap = map[int]SourceConfig{
	1: {Name: "BPS", IsWali: true},
	2: {Name: "KEMENDIKBUD", IsWali: true}, // Wali Data Pendidikan
	3: {Name: "LAINNYA", IsWali: false},
}

// Kamus Audit Decision
var AuditMap = map[int]string{
	1: "VALID",
	2: "INVALID",
}

// ValidatePendidikanMetadata melakukan validasi isi data berdasarkan aturan di database (Dinamis)
func (s *PendidikanService) ValidatePendidikanMetadata(p models.RiwayatPendidikan, definition datatypes.JSON) (bool, string) {
	// 1. Parsing Aturan dari Skema Aktif
	var rulesMap map[string]interface{}
	if err := json.Unmarshal(definition, &rulesMap); err != nil {
		return false, "Gagal membaca aturan metadata dari skema"
	}

	rules, hasRules := rulesMap["rules"].(map[string]interface{})
	if !hasRules {
		return true, "" // Jika tidak ada rules di skema, dianggap lolos validasi teknis
	}

	// 2. VALIDASI NIK (Dinamis sesuai angka 'length' di JSON)
	if r, ok := rules["nik"].(map[string]interface{}); ok {
		if lengthVal, ok := r["length"].(float64); ok {
			if len(p.NIK) != int(lengthVal) {
				return false, fmt.Sprintf("NIK harus %d digit (Aturan Skema Aktif)", int(lengthVal))
			}
		}
	}

	// 3. VALIDASI JENJANG (Dinamis sesuai angka 'max' di JSON)
	if r, ok := rules["jenjang"].(map[string]interface{}); ok {
		if maxVal, ok := r["max"].(float64); ok {
			val, _ := strconv.Atoi(p.Jenjang)
			if val > int(maxVal) {
				return false, fmt.Sprintf("Kode jenjang pendidikan tidak boleh lebih dari %d", int(maxVal))
			}
		}
	}

	// 4. VALIDASI ATRIBUT TAMBAHAN DI KANTONG AJAIB (AdditionalInfo)
	// Kita cek apakah ada atribut di AdditionalInfo yang diwajibkan oleh skema
	var extra map[string]interface{}
	json.Unmarshal(p.AdditionalInfo, &extra)

	for field, rule := range rules {
		r, ok := rule.(map[string]interface{})
		if !ok { continue }

		// Jika di skema bilang field ini "required", tapi di struct utama gak ada (berarti di extra)
		if r["required"] == true {
			// Cek apakah field ini adalah salah satu kolom tetap
			isFixedColumn := (field == "nik" || field == "partisipasi" || field == "jenjang" || field == "kelas" || field == "ijazah")
			
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
func (s *PendidikanService) ProcessIngestion(p models.RiwayatPendidikan) (string, error) {
	// 1. Cari data terakhir di Mesh untuk NIK ini
	last, err := s.Storage.GetLatestByNIK(p.NIK)

	// Jika data belum pernah ada (v1)
	if err != nil {
		p.Version = 1
		p.AuditStatus = "PENDING"
		if errCreate := s.Storage.Create(&p); errCreate != nil {
			return "Error", errCreate
		}
		return "Sukses v1", nil
	}

	// LOGIKA (SCD Type 2)
	// Data baru diterima jika: Tanggal lebih baru ATAU (Tanggal sama tapi pengirim adalah Walidata)
	isNewer := p.ReferenceDate.After(last.ReferenceDate)
	isHigherAuthority := p.ReferenceDate.Equal(last.ReferenceDate) && p.IsWaliData && !last.IsWaliData

	if isNewer || isHigherAuthority {
		// Buat versi baru
		p.ID = 0 // Reset ID agar auto-increment di DB
		p.Version = last.Version + 1
		p.AuditStatus = "PENDING" // Reset audit untuk pemeriksaan data baru
		
		if errCreate := s.Storage.Create(&p); errCreate != nil {
			return "Error", errCreate
		}
		return fmt.Sprintf("Sukses v%d", p.Version), nil
	}

	return "Abaikan", fmt.Errorf("data lebih lama dibandingkan data di database")
}