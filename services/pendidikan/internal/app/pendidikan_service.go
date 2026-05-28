package app

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"pendidikan/models"
	"pendidikan/storage"

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

	// 4. VALIDASI ATRIBUT TAMBAHAN DI KANTONG AJAIB (AdditionalInfo) DENGAN REFLECT
	// Ambil daftar tag JSON dari struct fisik
	typ := reflect.TypeOf(models.RiwayatPendidikan{})
	fixedFields := make(map[string]bool)
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		jsonTag := strings.Split(field.Tag.Get("json"), ",")[0]
		if jsonTag != "" && jsonTag != "-" {
			fixedFields[jsonTag] = true
		}
	}

	// Kita cek apakah ada atribut di AdditionalInfo yang diwajibkan oleh skema
	var extra map[string]interface{}
	json.Unmarshal(p.AdditionalInfo, &extra)

	for field, rule := range rules {
		r, ok := rule.(map[string]interface{})
		if !ok { continue }

		// Jika di skema bilang field ini "required", tapi di struct utama gak ada (berarti di extra)
		if r["required"] == true {
			if !fixedFields[field] { // Cek dinamis, menggantikan isFixedColumn manual
				if val, exists := extra[field]; !exists || fmt.Sprintf("%v", val) == "" {
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

// Logika Bisnis untuk Manual Update dari PUT Dataset
func (s *PendidikanService) ProcessManualUpdate(nik string, newData models.RiwayatPendidikan) (int, error) {
	oldData, err := s.Storage.GetLatestByNIK(nik)
	if err != nil || oldData == nil {
		return 0, fmt.Errorf("data asli tidak ditemukan")
	}

	// LOGIKA RESET: Jika data berubah, harus audit ulang (Score -20)
	newData.ID = 0
	newData.Version = oldData.Version + 1
	newData.AuditStatus = "PENDING"
	newData.UpdatedAt = time.Now()

	// Skor kembali ke base (Sistem 60 + Sumber 20/0)
	if newData.IsWaliData {
		newData.TrustScore = 80.0
	} else {
		newData.TrustScore = 60.0
	}

	errCreate := s.Storage.Create(&newData)
	return newData.Version, errCreate
}

// Logika Bisnis Audit
func (s *PendidikanService) ProcessAuditDecision(nikList []string, verdict int) (string, error) {
	verdictText := "INVALID"
	bonus := 0.0

	if text, exists := AuditMap[verdict]; exists {
		verdictText = text
		if verdict == 1 { bonus = 20.0 }
	} else {
		return "", fmt.Errorf("Verdict tidak valid. Gunakan 1 (VALID) atau 2 (INVALID)")
	}

	err := s.Storage.UpdateBulkAuditDecision(nikList, verdictText, bonus)
	return verdictText, err
}