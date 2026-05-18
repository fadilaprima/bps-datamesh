package app

import (
	"encoding/json"
	"fmt"
	"kependudukan/models"
	"kependudukan/storage"
	"strings"

	"gorm.io/datatypes"
)

type PendudukService struct {
	Storage storage.PendudukStorage
}

// Kamus Sumber Data Khusus Kependudukan
type SourceConfig struct {
	Name   string
	IsWali bool
}

var SourceMap = map[int]SourceConfig{
	1: {Name: "BPS", IsWali: true},
	2: {Name: "KEMENDAGRI", IsWali: true}, // Wali Data Kependudukan
	3: {Name: "LAINNYA", IsWali: false},
}

// Kamus Audit Decision
var AuditMap = map[int]string{
	1: "VALID",
	2: "INVALID",
}

// ValidatePendudukMetadata melakukan validasi isi data kependudukan secara dinamis
func (s *PendudukService) ValidatePendudukMetadata(p models.Penduduk, definition datatypes.JSON) (bool, string) {
	// 1. Parsing Aturan dari Skema Aktif di Database
	var schemaMap map[string]interface{}
	if err := json.Unmarshal(definition, &schemaMap); err != nil {
		return false, "Gagal membaca aturan metadata kependudukan"
	}

	rules, ok := schemaMap["definition"].(map[string]interface{})
	if !ok {
		return true, "" // Lolos jika definisi skema kosong
	}

	// 2. LOGIKA VALIDASI HIRARKI & KATEGORIKAL (NIK, NAMA, WILAYAH, JK)
	// Mapping field struct ke key di JSON Metadata
	checkList := []struct {
		FieldName string
		Value     string
	}{
		{"nomor_induk_kependudukan", p.NIK},
		{"nama", p.Nama},
		{"kode_provinsi", p.KodeProv},
		{"kode_kabupaten_kota", p.KodeKab},
		{"kode_kecamatan", p.KodeKec},
		{"kode_kelurahan_desa", p.KodeDesa},
		{"kode_provinsi_ktp", p.KodeProvKTP},
		{"kode_kabupaten_kota_ktp", p.KodeKabKTP},
		{"kode_kecamatan_ktp", p.KodeKecKTP},
		{"kode_kelurahan_desa_ktp", p.KodeDesaKTP},
		{"jenis_kelamin", p.JenisKelamin},
	}

	for _, item := range checkList {
		if r, ok := rules[item.FieldName].(map[string]interface{}); ok {
			// A. Cek Mandatory (Required)
			if r["required"] == true && strings.TrimSpace(item.Value) == "" {
				return false, fmt.Sprintf("Atribut '%s' wajib diisi (Mandatory)", item.FieldName)
			}

			// B. Cek Panjang Karakter (Length) 
			if lengthVal, ok := r["length"].(float64); ok {
				if item.Value != "" && len(item.Value) != int(lengthVal) {
					return false, fmt.Sprintf("Atribut '%s' tidak valid, harus %d digit sesuai standar BPS", item.FieldName, int(lengthVal))
				}
			}

			// C. Cek Enum 
			if item.FieldName == "jenis_kelamin" && item.Value != "" {
				if options, ok := r["options"].([]interface{}); ok {
					isValid := false
					for _, opt := range options {
						m := opt.(map[string]interface{})
						if m["code"] == item.Value {
							isValid = true
							break
						}
					}
					if !isValid {
						return false, "Jenis Kelamin tidak valid (Gunakan kode 1 untuk L atau 2 untuk P)"
					}
				}
			}
		}
	}

	// 3. VALIDASI NUMERIK (JUMLAH ANGGOTA KELUARGA)
	if r, ok := rules["jml_anggota"].(map[string]interface{}); ok {
		// Validasi Dasar: Tidak boleh negatif (Logika Kode Lama)
		if p.JmlAnggota < 0 {
			return false, "Jumlah anggota keluarga tidak logis (Nilai negatif)"
		}

		// Validasi Dinamis: Cek batas minimal jika ada di skema
		if minVal, ok := r["min"].(float64); ok {
			if float64(p.JmlAnggota) < minVal {
				return false, fmt.Sprintf("Jumlah anggota keluarga minimal adalah %d", int(minVal))
			}
		}
	}

	// 4. VALIDASI ATRIBUT TAMBAHAN DI KANTONG AJAIB 
	var extra map[string]interface{}
	json.Unmarshal(p.AdditionalInfo, &extra)

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
			if field == "jml_anggota" || field == "alamat" {
				isFixed = true
			}

			// Jika diwajibkan tapi tidak ada di kolom fisik
			if !isFixed {
				if val, exists := extra[field]; !exists || val == "" {
					return false, fmt.Sprintf("Atribut tambahan '%s' wajib diisi sesuai standar Metadata Mesh", field)
				}
			}
		}
	}

	return true, ""
}

// ProcessIngestion mengelola alur SCD Type 2 (Versioning) untuk Domain Penduduk
func (s *PendudukService) ProcessIngestion(p models.Penduduk) (string, error) {
	// 1. Ambil versi terakhir berdasarkan NIK
	last, err := s.Storage.GetLatestByNIK(p.NIK)

	// Skenario A: Data Baru (First Entry)
	if err != nil {
		p.Version = 1
		p.AuditStatus = "PENDING"
		if errCreate := s.Storage.Create(&p); errCreate != nil {
			return "Error", errCreate
		}
		return "Sukses v1 (Initial Entry)", nil
	}

	// Skenario B: Update Data (SCD Type 2)
	// Logika: Diterima jika ReferenceDate lebih baru ATAU (Tanggal sama tapi dari Wali Data)
	isNewer := p.ReferenceDate.After(last.ReferenceDate)
	isHigherAuthority := p.ReferenceDate.Equal(last.ReferenceDate) && p.IsWaliData && !last.IsWaliData

	if isNewer || isHigherAuthority {
		p.ID = 0 // Reset ID untuk record baru di database
		p.Version = last.Version + 1
		p.AuditStatus = "PENDING" // Reset audit untuk setiap perubahan data

		if errCreate := s.Storage.Create(&p); errCreate != nil {
			return "Error", errCreate
		}
		return fmt.Sprintf("Sukses v%d (Data Updated)", p.Version), nil
	}

	return "Abaikan", fmt.Errorf("data yang dikirim lebih usang dibandingkan data di mesh")
}
