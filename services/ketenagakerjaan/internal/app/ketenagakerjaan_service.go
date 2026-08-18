package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	"ketenagakerjaan/models"
	"ketenagakerjaan/storage"

	"gorm.io/datatypes"
)

type KetenagakerjaanService struct {
	Storage storage.KetenagakerjaanStorage
}

type SourceConfig struct {
	Name   string
	IsWali bool
}

var SourceMap = map[int]SourceConfig{
	1: {Name: "BPS", IsWali: true},
	2: {Name: "BPJS_KETENAGAKERJAAN", IsWali: true},
	3: {Name: "LAINNYA", IsWali: false},
}

var AuditMap = map[int]string{
	1: "VALID",
	2: "INVALID",
}

// ==========================================
// 1. FUNGSI VALIDASI INTERNAL (INTRA-DOMAIN)
// ==========================================
func (s *KetenagakerjaanService) ValidateInternalKetenagakerjaan(k models.RekamKetenagakerjaan) error {
	// Konversi tipe data numerik jika diperlukan dari struct
	jumlahUsaha, _ := strconv.Atoi(fmt.Sprintf("%v", k.JumlahUsaha))
	pekerjaDibayar, _ := strconv.Atoi(fmt.Sprintf("%v", k.JumlahPekerjaDibayar))
	omzet, _ := strconv.ParseFloat(fmt.Sprintf("%v", k.OmzetUsahaUtama), 64)

	// Rule: Status bekerja "Tidak" tetapi lapangan usaha utama terisi valid
	if k.StatusBekerja == "Tidak" && strings.TrimSpace(k.LapanganUsahaPekerjaanUtama) != "" && k.LapanganUsahaPekerjaanUtama != "0" {
		return fmt.Errorf("gagal validasi internal: status bekerja 'Tidak' tetapi lapangan usaha utama terisi")
	}

	// Rule: Kepemilikan usaha "Tidak" tetapi jumlah usaha > 0 atau omzet > 0
	if k.KepemilikanUsaha == "Tidak" && (jumlahUsaha > 0 || omzet > 0) {
		return fmt.Errorf("gagal validasi internal: kepemilikan usaha 'Tidak' tetapi jumlah usaha atau omzet > 0")
	}

	// Rule: Omzet < Rp 500.000 tetapi jumlah pekerja yang dibayar > 10 orang
	if omzet > 0 && omzet < 500000 && pekerjaDibayar > 10 {
		return fmt.Errorf("gagal validasi internal: omzet di bawah Rp 500.000 tetapi mempekerjakan lebih dari 10 orang")
	}

	// Rule: Status dalam pekerjaan utama = "Berusaha dibantu buruh dibayar", tetapi jumlah pekerja yang dibayar = 0
	if k.StatusDalamPekerjaanUtama == "Berusaha dibantu buruh dibayar" && pekerjaDibayar == 0 {
		return fmt.Errorf("gagal validasi internal: status berusaha dibantu buruh dibayar, tetapi jumlah pekerja yang dibayar 0")
	}

	// Rule: Status dalam pekerjaan utama = "Pekerja keluarga/tak dibayar", tetapi kepemilikan usaha = "Ya"
	if k.StatusDalamPekerjaanUtama == "Pekerja keluarga/tak dibayar" && k.KepemilikanUsaha == "Ya" {
		return fmt.Errorf("gagal validasi internal: pekerja tak dibayar tidak boleh berstatus memiliki usaha sendiri")
	}

	return nil
}

// ==========================================
// 2. FUNGSI VALIDASI SILANG (CROSS-DOMAIN API)
// ==========================================
func (s *KetenagakerjaanService) ValidateCrossDomainAPI(k models.RekamKetenagakerjaan) error {
	client := &http.Client{Timeout: 3 * time.Second}

	// Cek ke Domain Kependudukan (Port 8081) untuk mendapatkan Tanggal Lahir / Umur
	resp, err := client.Get(fmt.Sprintf("http://localhost:8081/api/v1/domains/penduduk/datasets/%s", k.NIK))
	if err == nil && resp.StatusCode == 200 {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)

		var result []map[string]interface{}
		if errJson := json.Unmarshal(body, &result); errJson == nil && len(result) > 0 {
			dataPenduduk := result[0]
			tglLahir := fmt.Sprintf("%v", dataPenduduk["tanggal_lahir"])

			umur := -1
			if t, parseErr := time.Parse("2006-01-02", tglLahir); parseErr == nil {
				umur = int(time.Since(t).Hours() / 24 / 365.25)
			}

			// Rule: Umur < 10 Tahun dan (status_bekerja = Ya ATAU kepemilikan_usaha = Ya)
			if umur >= 0 && umur < 10 && (k.StatusBekerja == "Ya" || k.KepemilikanUsaha == "Ya") {
				return fmt.Errorf("gagal validasi lintas domain (kependudukan): umur di bawah 10 tahun tidak boleh berstatus Bekerja atau Memiliki Usaha")
			}
		}
	}

	return nil
}

// ValidateKetenagakerjaanMetadata melakukan validasi isi data menggunakan Reflect Engine
func (s *KetenagakerjaanService) ValidateKetenagakerjaanMetadata(k models.RekamKetenagakerjaan, definition datatypes.JSON) (bool, string) {
	var schemaMap map[string]interface{}
	if err := json.Unmarshal(definition, &schemaMap); err != nil {
		return false, "Gagal membaca aturan metadata ketenagakerjaan"
	}

	rules, ok := schemaMap["definition"].(map[string]interface{})
	if !ok { return true, "" }

	val := reflect.ValueOf(k)
	typ := reflect.TypeOf(k)
	fixedFields := make(map[string]bool)

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		jsonTag := strings.Split(field.Tag.Get("json"), ",")[0]
		if jsonTag == "" || jsonTag == "-" { continue }
		
		fixedFields[jsonTag] = true
		fieldValue := fmt.Sprintf("%v", val.Field(i).Interface()) 

		if r, ok := rules[jsonTag].(map[string]interface{}); ok {
			// A. Cek Mandatory
			if r["required"] == true && strings.TrimSpace(fieldValue) == "" {
				return false, fmt.Sprintf("Atribut ketenagakerjaan '%s' wajib diisi (Mandatory)", jsonTag)
			}

			// B. Cek Panjang Karakter (Length) 
			if lengthVal, ok := r["length"].(float64); ok {
				if fieldValue != "" && fieldValue != "0" && len(fieldValue) != int(lengthVal) {
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
			if !fixedFields[field] {
				if valData, exists := extra[field]; !exists || fmt.Sprintf("%v", valData) == "" {
					return false, fmt.Sprintf("Atribut tambahan ketenagakerjaan '%s' wajib diisi", field)
				}
			}
		}
	}

	return true, ""
}

// ProcessIngestion mengelola alur SCD Type 2
func (s *KetenagakerjaanService) ProcessIngestion(k models.RekamKetenagakerjaan) (string, error) {
	// --- EKSEKUSI BLOK VALIDASI SEBELUM MASUK DATABASE ---
	
	// Tahap 1: Validasi Logika Internal
	if err := s.ValidateInternalKetenagakerjaan(k); err != nil {
		return "Error Validation", err
	}

	// Tahap 2: Validasi Logika Lintas Domain (API Call ke Kependudukan)
	if err := s.ValidateCrossDomainAPI(k); err != nil {
		return "Error Cross-Validation", err
	}

	// --- AKHIR BLOK VALIDASI ---

	last, err := s.Storage.GetLatestByNIK(k.NIK)

	if err != nil {
		k.Version = 1
		k.AuditStatus = "PENDING"
		if errCreate := s.Storage.Create(&k); errCreate != nil { return "Error", errCreate }
		return "Sukses v1 (Initial Entry)", nil
	}

	isNewer := k.ReferenceDate.After(last.ReferenceDate)
	isHigherAuthority := k.ReferenceDate.Equal(last.ReferenceDate) && k.IsWaliData && !last.IsWaliData

	if isNewer || isHigherAuthority {
		k.ID = 0
		k.Version = last.Version + 1
		k.AuditStatus = "PENDING" 

		if errCreate := s.Storage.Create(&k); errCreate != nil { return "Error", errCreate }
		return fmt.Sprintf("Sukses v%d (Data Updated)", k.Version), nil
	}

	return "Abaikan", fmt.Errorf("data ketenagakerjaan yang dikirim usang")
}

func (s *KetenagakerjaanService) ProcessManualUpdate(nik string, newData models.RekamKetenagakerjaan) (int, error) {
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

func (s *KetenagakerjaanService) ProcessAuditDecision(nikList []string, verdict int) (string, error) {
	verdictText := "INVALID"
	bonus := 0.0

	if text, exists := AuditMap[verdict]; exists {
		verdictText = text
		if verdict == 1 { bonus = 20.0 }
	} else {
		return "", fmt.Errorf("Verdict tidak valid")
	}

	err := s.Storage.UpdateBulkAuditDecision(nikList, verdictText, bonus)
	return verdictText, err
}