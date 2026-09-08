package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
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
// 1. FUNGSI VALIDASI INTERNAL (HARD FAIL)
// ==========================================
func (s *KetenagakerjaanService) ValidateInternalKetenagakerjaan(k *models.RekamKetenagakerjaan) error {
	jumlahUsaha, _ := strconv.Atoi(fmt.Sprintf("%v", k.JumlahUsaha))
	pekerjaDibayar, _ := strconv.Atoi(fmt.Sprintf("%v", k.JumlahPekerjaDibayar))
	omzet, _ := strconv.ParseFloat(fmt.Sprintf("%v", k.OmzetUsahaUtama), 64)

	// Rule 1: Status bekerja "Tidak" tetapi lapangan usaha utama terisi valid
	if k.StatusBekerja == "Tidak" && strings.TrimSpace(k.LapanganUsahaPekerjaanUtama) != "" && k.LapanganUsahaPekerjaanUtama != "0" {
		return fmt.Errorf("status bekerja 'Tidak' tetapi lapangan usaha utama terisi")
	}

	// Rule 2: Kepemilikan usaha "Tidak" tetapi jumlah usaha > 0 atau omzet > 0
	if k.KepemilikanUsaha == "Tidak" && (jumlahUsaha > 0 || omzet > 0) {
		return fmt.Errorf("kepemilikan usaha 'Tidak' tetapi jumlah usaha atau omzet > 0")
	}

	// Rule 3: Omzet < Rp 500.000 tetapi mempekerjakan > 10 orang
	if omzet > 0 && omzet < 500000 && pekerjaDibayar > 10 {
		return fmt.Errorf("omzet di bawah Rp 500.000 tetapi mempekerjakan lebih dari 10 orang")
	}

	// Rule 4: Status berusaha dibantu buruh dibayar, tetapi jumlah pekerja dibayar = 0
	if k.StatusDalamPekerjaanUtama == "Berusaha dibantu buruh dibayar" && pekerjaDibayar == 0 {
		return fmt.Errorf("status berusaha dibantu buruh dibayar, tetapi jumlah pekerja yang dibayar 0")
	}

	// Rule 5: Status pekerja tak dibayar, tetapi memiliki usaha
	if k.StatusDalamPekerjaanUtama == "Pekerja keluarga/tak dibayar" && k.KepemilikanUsaha == "Ya" {
		return fmt.Errorf("pekerja tak dibayar tidak boleh berstatus memiliki usaha sendiri")
	}

	return nil
}

// ==========================================
// 2. FUNGSI VALIDASI SILANG (HYBRID)
// ==========================================
func (s *KetenagakerjaanService) ValidateCrossDomainAPI(k *models.RekamKetenagakerjaan) error {
	client := &http.Client{Timeout: 3 * time.Second}

	// A. Domain Kependudukan 
	baseURLKependudukan := os.Getenv("URL_KEPENDUDUKAN")
	if baseURLKependudukan != "" {
		targetURL := fmt.Sprintf("%s/api/v1/domains/penduduk/datasets/%s", baseURLKependudukan, k.NIK)
		resp, err := client.Get(targetURL)
		
		if err != nil {
			fmt.Printf("[WARNING] Servis Kependudukan down, NIK %s tidak tervalidasi penuh. Trust Score -10\n", k.NIK)
			k.TrustScore -= 10.0
		} else {
			defer resp.Body.Close()
			if resp.StatusCode == 200 {
				body, _ := io.ReadAll(resp.Body)
				var result map[string]interface{}
				
				if errJson := json.Unmarshal(body, &result); errJson == nil {
					tglLahir := fmt.Sprintf("%v", result["tanggal_lahir"])

					umur := -1
					if t, parseErr := time.Parse("2006-01-02", tglLahir); parseErr == nil {
						umur = int(time.Since(t).Hours() / 24 / 365.25)
					}

					if umur >= 0 && umur < 10 && (k.StatusBekerja == "Ya" || k.KepemilikanUsaha == "Ya") {
						return fmt.Errorf("ditolak: umur di bawah 10 tahun tidak boleh berstatus Bekerja atau Memiliki Usaha")
					}
				}
			}
		}
	}

	return nil
}

// ==========================================
// 3. METADATA VALIDATION (HARD FAIL)
// ==========================================
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
			if r["required"] == true && strings.TrimSpace(fieldValue) == "" {
				return false, fmt.Sprintf("Atribut '%s' wajib diisi (Mandatory)", jsonTag)
			}
			if lengthVal, ok := r["length"].(float64); ok {
				if fieldValue != "" && fieldValue != "0" && len(fieldValue) != int(lengthVal) {
					return false, fmt.Sprintf("Atribut '%s' tidak valid, harus %d digit", jsonTag, int(lengthVal))
				}
			}
		}
	}

	var extra map[string]interface{}
	json.Unmarshal(k.AdditionalInfo, &extra)

	for field, rule := range rules {
		r, ok := rule.(map[string]interface{})
		if !ok { continue }

		if r["required"] == true {
			if !fixedFields[field] {
				if valData, exists := extra[field]; !exists || fmt.Sprintf("%v", valData) == "" {
					return false, fmt.Sprintf("Atribut tambahan '%s' wajib diisi", field)
				}
			}
		}
	}

	return true, ""
}

// ==========================================
// 4. INGESTION PIPELINE (SCD Type 2)
// ==========================================
func (s *KetenagakerjaanService) ProcessIngestion(k models.RekamKetenagakerjaan) (string, error) {
	if err := s.ValidateInternalKetenagakerjaan(&k); err != nil {
		return "Error Internal Logic", err
	}

	if err := s.ValidateCrossDomainAPI(&k); err != nil {
		return "Error Cross-Domain Logic", err
	}

	last, err := s.Storage.GetLatestByNIK(k.NIK)

	// Skenario A: Data Baru
	if err != nil {
		k.Version = 1
		k.AuditStatus = "PENDING"
		if errCreate := s.Storage.Create(&k); errCreate != nil {
			return "Error Database", errCreate
		}
		return "Sukses v1 (Masuk Data Mesh)", nil
	}

	// Skenario B: Update Data
	isNewer := k.ReferenceDate.After(last.ReferenceDate)
	isHigherAuthority := k.ReferenceDate.Equal(last.ReferenceDate) && k.IsWaliData && !last.IsWaliData

	if isNewer || isHigherAuthority {
		k.ID = 0
		k.Version = last.Version + 1
		k.AuditStatus = "PENDING" 

		if errCreate := s.Storage.Create(&k); errCreate != nil {
			return "Error Database", errCreate
		}
		return fmt.Sprintf("Sukses v%d (Update)", k.Version), nil
	}

	return "Abaikan", fmt.Errorf("data usang atau otoritas lebih rendah")
}

// ==========================================
// 5. UPDATE & AUDIT LOGIC
// ==========================================
func (s *KetenagakerjaanService) ProcessManualUpdate(nik string, newData models.RekamKetenagakerjaan) (int, error) {
	oldData, err := s.Storage.GetLatestByNIK(nik)
	if err != nil || oldData == nil {
		return 0, fmt.Errorf("data asli tidak ditemukan")
	}

	newData.ID = 0
	newData.Version = oldData.Version + 1
	newData.AuditStatus = "PENDING"
	newData.UpdatedAt = time.Now()

	if newData.IsWaliData {
		newData.TrustScore = 80.0
	} else {
		newData.TrustScore = 60.0
	}

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
		return "", fmt.Errorf("Verdict tidak valid. Gunakan 1 (VALID) atau 2 (INVALID)")
	}

	err := s.Storage.UpdateBulkAuditDecision(nikList, verdictText, bonus)
	return verdictText, err
}