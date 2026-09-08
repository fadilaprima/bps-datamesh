package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
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
	2: {Name: "KEMENDAGRI", IsWali: true},
	3: {Name: "LAINNYA", IsWali: false},
}

var AuditMap = map[int]string{
	1: "VALID",
	2: "INVALID",
}

func hitungUmur(tanggalLahir string) int {
	if t, err := time.Parse("2006-01-02", tanggalLahir); err == nil {
		return int(time.Since(t).Hours() / 24 / 365.25)
	}
	return -1
}

// ==========================================
// 1. FUNGSI VALIDASI INTERNAL (HARD FAIL)
// ==========================================
func (s *PendudukService) ValidateInternalKependudukan(p *models.Penduduk) error {
	umur := hitungUmur(p.TglLahir)

	if umur >= 0 && umur < 12 && (p.StatusKawin == "Kawin" || p.StatusKawin == "Cerai Hidup" || p.StatusKawin == "Cerai Mati") {
		return fmt.Errorf("umur di bawah 12 tahun tidak boleh berstatus Kawin/Cerai")
	}
	if umur >= 0 && umur < 10 && p.StatusHubungan == "Kepala Keluarga" {
		return fmt.Errorf("umur di bawah 10 tahun tidak boleh berstatus Kepala Keluarga")
	}
	if (p.StatusHubungan == "Suami" && p.JenisKelamin == "2") || (p.StatusHubungan == "Istri" && p.JenisKelamin == "1") {
		return fmt.Errorf("status hubungan keluarga tidak logis dengan jenis kelamin")
	}
	return nil
}

// ==========================================
// 2. FUNGSI VALIDASI SILANG (HYBRID)
// ==========================================
func (s *PendudukService) ValidateCrossDomainAPI(p *models.Penduduk) error {
	umur := hitungUmur(p.TglLahir)
	client := &http.Client{Timeout: 3 * time.Second}

	// A. Domain Pendidikan 
	if umur >= 0 && umur < 18 {
		baseURLPendidikan := os.Getenv("URL_PENDIDIKAN")
		if baseURLPendidikan != "" {
			targetURL := fmt.Sprintf("%s/api/v1/domains/pendidikan/datasets/%s", baseURLPendidikan, p.NIK)
			resp, err := client.Get(targetURL)
			
			if err != nil {
				// Kurangi skor jika servis mati/timeout. 
				fmt.Printf("[WARNING] Servis Pendidikan down, NIK %s tidak tervalidasi. Trust Score -10\n", p.NIK)
				p.TrustScore -= 10.0
			} else {
				defer resp.Body.Close()
				if resp.StatusCode == 200 {
					body, _ := io.ReadAll(resp.Body)
					var result map[string]interface{}
					if errJson := json.Unmarshal(body, &result); errJson == nil {
						partisipasi := fmt.Sprintf("%v", result["partisipasi_sekolah"])
						jenjang := fmt.Sprintf("%v", result["jenjang_tertinggi_yang_diduduki"])
						ijazah := fmt.Sprintf("%v", result["ijazah_tertinggi_yang_dimiliki"])

						// Servis nyala, tapi data terbukti jelek kualitasnya
						if umur < 5 && (partisipasi == "Masih Sekolah" || (jenjang != "TK/Belum sekolah" && jenjang != "" && jenjang != "null")) {
							return fmt.Errorf("ditolak: umur < 5 tahun tidak wajar untuk partisipasi sekolah")
						}
						if umur < 18 && (strings.Contains(ijazah, "S1") || strings.Contains(ijazah, "S2") || strings.Contains(ijazah, "S3")) {
							return fmt.Errorf("ditolak: umur < 18 tahun tidak wajar memiliki ijazah S1 ke atas")
						}
					}
				}
			}
		}
	}

	// B. Domain Ketenagakerjaan 
	if umur >= 0 && umur < 10 {
		baseURLKetenagakerjaan := os.Getenv("URL_KETENAGAKERJAAN")
		if baseURLKetenagakerjaan != "" {
			targetURL := fmt.Sprintf("%s/api/v1/domains/ketenagakerjaan/datasets/%s", baseURLKetenagakerjaan, p.NIK)
			resp, err := client.Get(targetURL)
			
			if err != nil {
				fmt.Printf("[WARNING] Servis Ketenagakerjaan down, NIK %s tidak tervalidasi. Trust Score -10\n", p.NIK)
				p.TrustScore -= 10.0
			} else {
				defer resp.Body.Close()
				if resp.StatusCode == 200 {
					body, _ := io.ReadAll(resp.Body)
					var result map[string]interface{}
					if errJson := json.Unmarshal(body, &result); errJson == nil {
						statusBekerja := fmt.Sprintf("%v", result["status_bekerja"])
						kepemilikanUsaha := fmt.Sprintf("%v", result["kepemilikan_usaha"])
						if statusBekerja == "Ya" || kepemilikanUsaha == "Ya" {
							return fmt.Errorf("ditolak: umur < 10 tahun tidak boleh berstatus Bekerja atau Memiliki Usaha")
						}
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
func (s *PendudukService) ValidatePendudukMetadata(p models.Penduduk, definition datatypes.JSON) (bool, string) {
	var schemaMap map[string]interface{}
	if err := json.Unmarshal(definition, &schemaMap); err != nil {
		return false, "Gagal membaca aturan metadata kependudukan"
	}

	rules, ok := schemaMap["definition"].(map[string]interface{})
	if !ok {
		return true, ""
	}

	val := reflect.ValueOf(p)
	typ := reflect.TypeOf(p)
	fixedFields := make(map[string]bool)

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		jsonTag := strings.Split(field.Tag.Get("json"), ",")[0]
		if jsonTag == "" || jsonTag == "-" {
			continue
		}

		fixedFields[jsonTag] = true
		fieldValue := fmt.Sprintf("%v", val.Field(i).Interface())

		if r, ok := rules[jsonTag].(map[string]interface{}); ok {
			if r["required"] == true && strings.TrimSpace(fieldValue) == "" {
				return false, fmt.Sprintf("Atribut '%s' wajib diisi (Mandatory)", jsonTag)
			}
			if lengthVal, ok := r["length"].(float64); ok {
				if fieldValue != "" && len(fieldValue) != int(lengthVal) {
					return false, fmt.Sprintf("Atribut '%s' tidak valid, harus %d digit", jsonTag, int(lengthVal))
				}
			}
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
					if !isValid {
						return false, "Jenis Kelamin tidak valid"
					}
				}
			}
			if jsonTag == "jumlah_anggota_keluarga" || jsonTag == "jml_anggota" {
				if p.JmlAnggota < 0 {
					return false, "Jumlah anggota keluarga negatif"
				}
				if minVal, ok := r["min"].(float64); ok {
					if float64(p.JmlAnggota) < minVal {
						return false, fmt.Sprintf("Jumlah anggota keluarga minimal %d", int(minVal))
					}
				}
			}
		}
	}

	var extra map[string]interface{}
	json.Unmarshal(p.AdditionalInfo, &extra)
	for field, rule := range rules {
		r, ok := rule.(map[string]interface{})
		if !ok {
			continue
		}
		aliasField := field
		if field == "jml_anggota" {
			aliasField = "jumlah_anggota_keluarga"
		}
		if r["required"] == true && !fixedFields[aliasField] {
			if val, exists := extra[field]; !exists || fmt.Sprintf("%v", val) == "" {
				return false, fmt.Sprintf("Atribut tambahan '%s' wajib diisi", field)
			}
		}
	}
	return true, ""
}

// ==========================================
// 4. INGESTION PIPELINE
// ==========================================
func (s *PendudukService) ProcessIngestion(p models.Penduduk) (string, error) {
	// Jika melanggar logika internal, LANGSUNG TOLAK dan lempar pesan error ke user
	if err := s.ValidateInternalKependudukan(&p); err != nil {
		return "Error Internal Logic", err
	}

	if err := s.ValidateCrossDomainAPI(&p); err != nil {
		return "Error Cross-Domain Logic", err
	}

	last, err := s.Storage.GetLatestByNIK(p.NIK)

	// Skenario A: Data Baru
	if err != nil {
		p.Version = 1
		p.AuditStatus = "PENDING"
		if errCreate := s.Storage.Create(&p); errCreate != nil {
			return "Error Database", errCreate
		}
		return "Sukses v1 (Masuk Data Mesh)", nil
	}

	// Skenario B: Update Data
	isNewer := p.ReferenceDate.After(last.ReferenceDate)
	isHigherAuthority := p.ReferenceDate.Equal(last.ReferenceDate) && p.IsWaliData && !last.IsWaliData

	if isNewer || isHigherAuthority {
		p.ID = 0
		p.Version = last.Version + 1
		p.AuditStatus = "PENDING"

		if errCreate := s.Storage.Create(&p); errCreate != nil {
			return "Error Database", errCreate
		}
		return fmt.Sprintf("Sukses v%d (Update)", p.Version), nil
	}
	
	return "Abaikan", fmt.Errorf("data usang atau otoritas lebih rendah")
}

// ==========================================
// 5. UPDATE & AUDIT LOGIC
// ==========================================
func (s *PendudukService) ProcessManualUpdate(nik string, newData models.Penduduk) (int, error) {
	oldData, err := s.Storage.GetLatestByNIK(nik)
	if err != nil || oldData == nil {
		return 0, fmt.Errorf("data asli tidak ditemukan")
	}
	newData.ID = 0
	newData.Version = oldData.Version + 1
	newData.AuditStatus = "PENDING"
	newData.UpdatedAt = time.Now()
	newData.TrustScore = 60.0
	if newData.IsWaliData {
		newData.TrustScore = 80.0
	}
	errCreate := s.Storage.Create(&newData)
	return newData.Version, errCreate
}

func (s *PendudukService) ProcessAuditDecision(nikList []string, verdict int) (string, error) {
	verdictText := "INVALID"
	bonus := 0.0
	if text, exists := AuditMap[verdict]; exists {
		verdictText = text
		if verdict == 1 {
			bonus = 20.0
		}
	} else {
		return "", fmt.Errorf("verdict tidak valid")
	}
	err := s.Storage.UpdateBulkAuditDecision(nikList, verdictText, bonus)
	return verdictText, err
}