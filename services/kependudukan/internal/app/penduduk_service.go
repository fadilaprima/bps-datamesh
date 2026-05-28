package app

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"kependudukan/models"
	"kependudukan/storage"

	"gorm.io/datatypes"
)

type PendudukService struct {
	Storage storage.PendudukStorage
}

type SourceConfig struct {
	Name   string
	IsWali bool
}

var SourceMap = map[int]SourceConfig{
	1: {Name: "BPS", IsWali: true},
	2: {Name: "KEMENDAGRI", IsWali: true}, // Wali Data Kependudukan
	3: {Name: "LAINNYA", IsWali: false},
}

var AuditMap = map[int]string{
	1: "VALID",
	2: "INVALID",
}

// ValidatePendudukMetadata melakukan validasi isi data kependudukan secara dinamis
func (s *PendudukService) ValidatePendudukMetadata(p models.Penduduk, definition datatypes.JSON) (bool, string) {
	var schemaMap map[string]interface{}
	if err := json.Unmarshal(definition, &schemaMap); err != nil {
		return false, "Gagal membaca aturan metadata kependudukan"
	}

	rules, ok := schemaMap["definition"].(map[string]interface{})
	if !ok { return true, "" }

	// 2. LOGIKA VALIDASI HIRARKI & KATEGORIKAL (DENGAN REFLECT ENGINE)
	val := reflect.ValueOf(p)
	typ := reflect.TypeOf(p)
	fixedFields := make(map[string]bool)

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		jsonTag := strings.Split(field.Tag.Get("json"), ",")[0]
		if jsonTag == "" || jsonTag == "-" { continue }
		
		fixedFields[jsonTag] = true
		fieldValue := fmt.Sprintf("%v", val.Field(i).Interface()) 

		if r, ok := rules[jsonTag].(map[string]interface{}); ok {
			// A. Cek Mandatory (Required)
			if r["required"] == true && strings.TrimSpace(fieldValue) == "" {
				return false, fmt.Sprintf("Atribut '%s' wajib diisi (Mandatory)", jsonTag)
			}

			// B. Cek Panjang Karakter (Length) 
			if lengthVal, ok := r["length"].(float64); ok {
				if fieldValue != "" && len(fieldValue) != int(lengthVal) {
					return false, fmt.Sprintf("Atribut '%s' tidak valid, harus %d digit sesuai standar BPS", jsonTag, int(lengthVal))
				}
			}

			// C. Cek Enum (Khusus Jenis Kelamin)
			if jsonTag == "jenis_kelamin" && fieldValue != "" {
				if options, ok := r["options"].([]interface{}); ok {
					isValid := false
					for _, opt := range options {
						m := opt.(map[string]interface{})
						if fmt.Sprintf("%v", m["code"]) == fieldValue {
							isValid = true
							break
						}
					}
					if !isValid { return false, "Jenis Kelamin tidak valid (Gunakan kode 1 untuk L atau 2 untuk P)" }
				}
			}

			// D. Cek Numerik Khusus (Jumlah Anggota Keluarga)
			if jsonTag == "jumlah_anggota_keluarga" || jsonTag == "jml_anggota" {
				if p.JmlAnggota < 0 {
					return false, "Jumlah anggota keluarga tidak logis (Nilai negatif)"
				}
				if minVal, ok := r["min"].(float64); ok {
					if float64(p.JmlAnggota) < minVal {
						return false, fmt.Sprintf("Jumlah anggota keluarga minimal adalah %d", int(minVal))
					}
				}
			}
		}
	}

	// 4. VALIDASI ATRIBUT TAMBAHAN DI KANTONG AJAIB 
	var extra map[string]interface{}
	json.Unmarshal(p.AdditionalInfo, &extra)

	for field, rule := range rules {
		r, ok := rule.(map[string]interface{})
		if !ok { continue }

		// Sinkronisasi alias khusus dari kodingan awal
		aliasField := field
		if field == "jml_anggota" { aliasField = "jumlah_anggota_keluarga" }

		if r["required"] == true {
			// Jika diwajibkan tapi tidak ada di kolom fisik
			if !fixedFields[aliasField] {
				if val, exists := extra[field]; !exists || fmt.Sprintf("%v", val) == "" {
					return false, fmt.Sprintf("Atribut tambahan '%s' wajib diisi sesuai standar Metadata Mesh", field)
				}
			}
		}
	}

	return true, ""
}

// ProcessIngestion mengelola alur SCD Type 2 (Versioning) untuk Domain Penduduk
func (s *PendudukService) ProcessIngestion(p models.Penduduk) (string, error) {
	last, err := s.Storage.GetLatestByNIK(p.NIK)

	// Skenario A: Data Baru
	if err != nil {
		p.Version = 1
		p.AuditStatus = "PENDING"
		if errCreate := s.Storage.Create(&p); errCreate != nil { return "Error", errCreate }
		return "Sukses v1 (Initial Entry)", nil
	}

	// Skenario B: Update Data
	isNewer := p.ReferenceDate.After(last.ReferenceDate)
	isHigherAuthority := p.ReferenceDate.Equal(last.ReferenceDate) && p.IsWaliData && !last.IsWaliData

	if isNewer || isHigherAuthority {
		p.ID = 0
		p.Version = last.Version + 1
		p.AuditStatus = "PENDING" 

		if errCreate := s.Storage.Create(&p); errCreate != nil { return "Error", errCreate }
		return fmt.Sprintf("Sukses v%d (Data Updated)", p.Version), nil
	}
	return "Abaikan", fmt.Errorf("data yang dikirim lebih usang")
}

// Logika Bisnis untuk Manual Update dari PUT Dataset
func (s *PendudukService) ProcessManualUpdate(nik string, newData models.Penduduk) (int, error) {
	oldData, err := s.Storage.GetLatestByNIK(nik)
	if err != nil || oldData == nil {
		return 0, fmt.Errorf("data asli tidak ditemukan")
	}

	newData.ID = 0
	newData.Version = oldData.Version + 1
	newData.AuditStatus = "PENDING"
	newData.UpdatedAt = time.Now()

	if newData.IsWaliData { newData.TrustScore = 80.0 } else { newData.TrustScore = 60.0 }

	errCreate := s.Storage.Create(&newData)
	return newData.Version, errCreate
}

// Logika Bisnis Audit
func (s *PendudukService) ProcessAuditDecision(nikList []string, verdict int) (string, error) {
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