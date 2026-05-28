package app

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

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

// 2. METADATA VALIDATOR (DYNAMIC SCHEMA VALIDATION WITH REFLECT)
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

	// 2. LOGIKA VALIDASI FIELD KESEJAHTERAAN (25 Variabel Utama Regsosek via Reflect)
	val := reflect.ValueOf(k)
	typ := reflect.TypeOf(k)
	fixedFields := make(map[string]bool)

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		jsonTag := strings.Split(field.Tag.Get("json"), ",")[0]
		if jsonTag == "" || jsonTag == "-" { continue }
		
		fixedFields[jsonTag] = true
		
		// Eksekusi logika emas: Jika integer 0, sprintf merubahnya jadi "0" (Lolos Mandatory)
		fieldValue := fmt.Sprintf("%v", val.Field(i).Interface()) 

		if r, ok := rules[jsonTag].(map[string]interface{}); ok {
			// A. Cek Mandatory (Required)
			// Catatan: Jika field integer bernilai 0, Sprint menjadikannya "0" sehingga lolos dari cek kosong "".
			// Ini aman untuk field aset/ternak karena "0" adalah jawaban valid (tidak punya).
			if r["required"] == true && strings.TrimSpace(fieldValue) == "" {
				return false, fmt.Sprintf("Atribut kesejahteraan '%s' wajib diisi (Mandatory)", jsonTag)
			}

			// B. Cek Panjang Karakter (Length) - Berguna untuk Nomor KK (16 digit)
			if lengthVal, ok := r["length"].(float64); ok {
				if fieldValue != "" && len(fieldValue) != int(lengthVal) {
					return false, fmt.Sprintf("Atribut '%s' tidak valid, harus %d digit sesuai standar", jsonTag, int(lengthVal))
				}
			}
		}
	}

	// 3. VALIDASI ATRIBUT TAMBAHAN (AdditionalInfo)
	var extra map[string]interface{}
	json.Unmarshal(k.AdditionalInfo, &extra)

	for field, rule := range rules {
		r, ok := rule.(map[string]interface{})
		if !ok { continue }

		if r["required"] == true {
			// Jika diwajibkan tapi tidak ada di kolom fisik (fixed columns), cari di Additional Info
			if !fixedFields[field] {
				if valData, exists := extra[field]; !exists || fmt.Sprintf("%v", valData) == "" {
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

func (s *KesejahteraanService) ProcessManualUpdate(nokk string, newData models.RekamKesejahteraan) (int, error) {
	oldData, err := s.Storage.GetLatestByNoKK(nokk)
	if err != nil || oldData == nil {
		return 0, fmt.Errorf("data asli tidak ditemukan")
	}

	newData.ID = 0
	newData.Version = oldData.Version + 1
	newData.AuditStatus = "PENDING"
	newData.UpdatedAt = time.Now()

	errCreate := s.Storage.Create(&newData)
	return newData.Version, errCreate
}

func (s *KesejahteraanService) ProcessAuditDecision(nokkList []string, verdict int) (string, error) {
	verdictText := "INVALID"
	bonus := 0.0

	if text, exists := AuditMap[verdict]; exists {
		verdictText = text
		if verdict == 1 { bonus = 20.0 }
	} else {
		return "", fmt.Errorf("Verdict tidak valid")
	}

	err := s.Storage.UpdateBulkAuditDecision(nokkList, verdictText, bonus)
	return verdictText, err
}