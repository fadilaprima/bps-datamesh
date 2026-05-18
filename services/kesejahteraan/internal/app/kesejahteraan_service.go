package app

import (
	"encoding/json"
	"fmt"
	"strings"
	"kesejahteraan/models"
	"kesejahteraan/storage"

	"gorm.io/datatypes"
)

// KesejahteraanService mengelola seluruh logika bisnis domain kesejahteraan dengan arsitektur Data Mesh
type KesejahteraanService struct {
	Storage storage.KesejahteraanStorage
}

// 1. DOMAIN OWNER
// KesejahteraanSourceRegistry
type SourceConfig struct {
	Name   string
	IsWali bool
}

// Map Angka -> Konfigurasi Source
var SourceMap = map[int]SourceConfig{
	1: {Name: "BPS", IsWali: true},
	2: {Name: "KEMENSOS", IsWali: true}, // Wali Data Kesejahteraan (Regsosek/DTKS)
	3: {Name: "LAINNYA", IsWali: false},
}

// Map Angka -> Teks Verdict
var AuditMap = map[int]string{
	1: "VALID",
	2: "INVALID",
}

// 2. METADATA VALIDATOR (DYNAMIC SCHEMA VALIDATION)
// ValidateKesejahteraanMetadata melakukan validasi isi data secara dinamis berdasarkan skema aktif
func (s *KesejahteraanService) ValidateKesejahteraanMetadata(k models.RekamKesejahteraan, definition datatypes.JSON) (bool, string) {
	// 1. Parsing Aturan dari Skema Aktif di Database
	var schemaMap map[string]interface{}
	if err := json.Unmarshal(definition, &schemaMap); err != nil {
		return false, "Gagal membaca aturan metadata kesejahteraan"
	}

	rules, ok := schemaMap["definition"].(map[string]interface{})
	if !ok {
		return true, "" 
	}

	// 2. LOGIKA VALIDASI FIELD KESEJAHTERAAN (25 Variabel Utama)
	// Mapping field struct ke key di JSON Metadata
	checkList := []struct {
		FieldName string
		Value     string
	}{
		{"nomor_kartu_keluarga", k.NoKK},
		{"desil_nasional", fmt.Sprint(k.DesilNasional)},
		{"bahan_bakar_utama_memasak", k.BahanBakarMemasak},
		{"kepemilikan_aset", fmt.Sprint(k.KepemilikanAset)},
		{"aset_bergerak_tabung_gas", fmt.Sprint(k.AsetGas)},
		{"aset_bergerak_lemari_es", fmt.Sprint(k.AsetKulkas)},
		{"aset_bergerak_ac", fmt.Sprint(k.AsetAC)},
		{"aset_bergerak_pemanas_air", fmt.Sprint(k.AsetPemanasAir)},
		{"aset_bergerak_telepon_rumah", fmt.Sprint(k.AsetTelepon)},
		{"aset_bergerak_tv_datar", fmt.Sprint(k.AsetTV)},
		{"aset_bergerak_emas_perhiasan", fmt.Sprint(k.AsetEmas)},
		{"aset_bergerak_komputer_laptop_tablet", fmt.Sprint(k.AsetLaptop)},
		{"aset_bergerak_sepeda_motor", fmt.Sprint(k.AsetMotor)},
		{"aset_bergerak_sepeda", fmt.Sprint(k.AsetSepeda)},
		{"aset_bergerak_mobil", fmt.Sprint(k.AsetMobil)},
		{"aset_bergerak_perahu", fmt.Sprint(k.AsetPerahu)},
		{"aset_bergerak_kapal_perahu_motor", fmt.Sprint(k.AsetPerahuMotor)},
		{"aset_bergerak_smartphone", fmt.Sprint(k.AsetSmartphone)},
		{"aset_tidak_bergerak_lahan_lainnya", fmt.Sprint(k.AsetLahanLain)},
		{"aset_tidak_bergerak_rumah_lainnya", fmt.Sprint(k.AsetRumahLain)},
		{"jumlah_ternak_sapi", fmt.Sprint(k.TernakSapi)},
		{"jumlah_ternak_kerbau", fmt.Sprint(k.TernakKerbau)},
		{"jumlah_ternak_kuda", fmt.Sprint(k.TernakKuda)},
		{"jumlah_ternak_babi", fmt.Sprint(k.TernakBabi)},
		{"jumlah_ternak_kambing_domba", fmt.Sprint(k.TernakKambing)},
	}

	for _, item := range checkList {
		if r, ok := rules[item.FieldName].(map[string]interface{}); ok {
			// A. Cek Mandatory (Required)
			// Catatan: Jika field integer bernilai 0, Sprint menjadikannya "0" sehingga lolos dari cek kosong "".
			// Ini aman untuk field aset/ternak karena "0" adalah jawaban valid (tidak punya).
			if r["required"] == true && strings.TrimSpace(item.Value) == "" {
				return false, fmt.Sprintf("Atribut kesejahteraan '%s' wajib diisi (Mandatory)", item.FieldName)
			}

			// B. Cek Panjang Karakter (Length) - Berguna untuk Nomor KK (16 digit)
			if lengthVal, ok := r["length"].(float64); ok {
				if item.Value != "" && len(item.Value) != int(lengthVal) {
					return false, fmt.Sprintf("Atribut '%s' tidak valid, harus %d digit sesuai standar", item.FieldName, int(lengthVal))
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
					return false, fmt.Sprintf("Atribut tambahan kesejahteraan '%s' wajib diisi sesuai standar Metadata Mesh", field)
				}
			}
		}
	}

	return true, ""
}

// 3. CONFLICT RESOLUTION & SCD TYPE 2 (VERSIONING)
// ProcessIngestion mengelola alur SCD Type 2 untuk Domain Kesejahteraan
func (s *KesejahteraanService) ProcessIngestion(k models.RekamKesejahteraan) (string, error) {
	// 1. Ambil versi terakhir berdasarkan NoKK (Natural Key)
	last, err := s.Storage.GetLatestByNoKK(k.NoKK)

	// Skenario A: Data Kesejahteraan Baru (First Entry)
	if err != nil {
		k.Version = 1
		k.AuditStatus = "PENDING"
		if errCreate := s.Storage.Create(&k); errCreate != nil {
			return "Error", errCreate
		}
		return "Sukses v1 (Initial Entry)", nil
	}

	// Skenario B: Update Data (SCD Type 2)
	// Logika: Diterima jika ReferenceDate lebih baru ATAU (Tanggal sama tapi dari Wali Data)
	isNewer := k.ReferenceDate.After(last.ReferenceDate)
	isHigherAuthority := k.ReferenceDate.Equal(last.ReferenceDate) && k.IsWaliData && !last.IsWaliData

	if isNewer || isHigherAuthority {
		k.ID = 0 // Reset ID untuk record baru di database
		k.Version = last.Version + 1
		k.AuditStatus = "PENDING" // Reset audit untuk setiap perubahan data

		if errCreate := s.Storage.Create(&k); errCreate != nil {
			return "Error", errCreate
		}
		return fmt.Sprintf("Sukses v%d (Kesejahteraan Updated)", k.Version), nil
	}

	return "Abaikan", fmt.Errorf("data kesejahteraan yang dikirim lebih usang dibandingkan data di mesh")
}