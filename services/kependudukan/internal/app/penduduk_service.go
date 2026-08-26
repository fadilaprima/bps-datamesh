package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

// ==========================================
// 1. FUNGSI HELPER MENGHITUNG UMUR
// ==========================================
func hitungUmur(tanggalLahir string) int {
	if t, err := time.Parse("2006-01-02", tanggalLahir); err == nil {
		return int(time.Since(t).Hours() / 24 / 365.25)
	}
	return -1 // Kembalikan -1 jika format tanggal salah atau kosong
}

// ==========================================
// 2. FUNGSI VALIDASI INTERNAL (INTRA-DOMAIN)
// ==========================================
func (s *PendudukService) ValidateInternalKependudukan(p models.Penduduk) error {
	umur := hitungUmur(p.TglLahir)

	// Rule: Umur < 12 Tahun dan status_kawin = Kawin/Cerai
	if umur >= 0 && umur < 12 && (p.StatusKawin == "Kawin" || p.StatusKawin == "Cerai Hidup" || p.StatusKawin == "Cerai Mati") {
		return fmt.Errorf("gagal validasi internal: umur di bawah 12 tahun tidak boleh berstatus Kawin/Cerai")
	}

	// Rule: Umur < 10 Tahun dan status_hubungan_keluarga = Kepala Keluarga
	if umur >= 0 && umur < 10 && p.StatusHubungan == "Kepala Keluarga" {
		return fmt.Errorf("gagal validasi internal: umur di bawah 10 tahun tidak boleh berstatus Kepala Keluarga")
	}

	// Rule: Status hubungan keluarga (Suami/Istri) harus sesuai Jenis Kelamin
	// Sesuai metadata, asumsikan kode jenis kelamin: 1 = Laki-laki, 2 = Perempuan
	if (p.StatusHubungan == "Suami" && p.JenisKelamin == "2") || (p.StatusHubungan == "Istri" && p.JenisKelamin == "1") {
		return fmt.Errorf("gagal validasi internal: status hubungan keluarga tidak logis dengan jenis kelamin")
	}

	return nil
}

// ==========================================
// 3. FUNGSI VALIDASI SILANG (CROSS-DOMAIN API)
// ==========================================
func (s *PendudukService) ValidateCrossDomainAPI(p models.Penduduk) error {
	umur := hitungUmur(p.TglLahir)

	// Agar request tidak menggantung selamanya jika service lain mati
	client := &http.Client{Timeout: 3 * time.Second}

	// --- A. Cek ke Domain Pendidikan (Port 8082) ---
	if umur >= 0 && umur < 18 {
		resp, err := client.Get(fmt.Sprintf("http://host.docker.internal:8082/api/v1/domains/pendidikan/datasets/%s", p.NIK))
		if err != nil {
			return fmt.Errorf("gagal menghubungi service pendidikan: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode == 200 {
			body, _ := io.ReadAll(resp.Body)

			// PERBAIKAN: Gunakan Object tunggal, bukan Array (hapus kurung siku [])
			var result map[string]interface{}
			if errJson := json.Unmarshal(body, &result); errJson == nil {
				// Ambil datanya langsung dari result
				partisipasi := fmt.Sprintf("%v", result["partisipasi_sekolah"])
				jenjang := fmt.Sprintf("%v", result["jenjang_tertinggi_yang_diduduki"])
				ijazah := fmt.Sprintf("%v", result["ijazah_tertinggi_yang_dimiliki"])

				// Rule: Umur < 5 Tahun
				if umur < 5 && (partisipasi == "Masih Sekolah" || (jenjang != "TK/Belum sekolah" && jenjang != "" && jenjang != "null")) {
					return fmt.Errorf("gagal validasi lintas domain (pendidikan): umur di bawah 5 tahun tidak wajar untuk partisipasi atau jenjang tersebut")
				}

				// Rule: Umur < 18 Tahun dan ijazah >= S1
				if umur < 18 && (strings.Contains(ijazah, "S1") || strings.Contains(ijazah, "S2") || strings.Contains(ijazah, "S3")) {
					return fmt.Errorf("gagal validasi lintas domain (pendidikan): umur di bawah 18 tahun tidak wajar memiliki ijazah setingkat S1 ke atas")
				}
			} else {
				// ANTI SILENT-FAILURE: Berteriak jika format JSON tidak sesuai!
				return fmt.Errorf("gagal parsing JSON dari service pendidikan (Format Beda): %v", errJson)
			}
		}
	}

	// --- B. Cek ke Domain Ketenagakerjaan (Port 8085) ---
	if umur >= 0 && umur < 10 {
		resp, err := client.Get(fmt.Sprintf("http://host.docker.internal:8085/api/v1/domains/ketenagakerjaan/datasets/%s", p.NIK))
		if err != nil {
			return fmt.Errorf("gagal menghubungi service ketenagakerjaan: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode == 200 {
			body, _ := io.ReadAll(resp.Body)

			// PERBAIKAN: Gunakan Object tunggal
			var result map[string]interface{}
			if errJson := json.Unmarshal(body, &result); errJson == nil {
				statusBekerja := fmt.Sprintf("%v", result["status_bekerja"])
				kepemilikanUsaha := fmt.Sprintf("%v", result["kepemilikan_usaha"])

				if statusBekerja == "Ya" || kepemilikanUsaha == "Ya" {
					return fmt.Errorf("gagal validasi lintas domain (ketenagakerjaan): umur di bawah 10 tahun tidak boleh berstatus Bekerja atau Memiliki Usaha")
				}
			} else {
				// ANTI SILENT-FAILURE
				return fmt.Errorf("gagal parsing JSON dari service ketenagakerjaan (Format Beda): %v", errJson)
			}
		}
	}

	return nil
}

// ==========================================
// 4. KODE BAWAAN METADATA VALIDATION
// ==========================================
// ValidatePendudukMetadata melakukan validasi isi data kependudukan secara dinamis
func (s *PendudukService) ValidatePendudukMetadata(p models.Penduduk, definition datatypes.JSON) (bool, string) {
	var schemaMap map[string]interface{}
	if err := json.Unmarshal(definition, &schemaMap); err != nil {
		return false, "Gagal membaca aturan metadata kependudukan"
	}

	rules, ok := schemaMap["definition"].(map[string]interface{})
	if !ok {
		return true, ""
	}

	// LOGIKA VALIDASI HIRARKI & KATEGORIKAL (DENGAN REFLECT ENGINE)
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
					if !isValid {
						return false, "Jenis Kelamin tidak valid (Gunakan kode 1 untuk L atau 2 untuk P)"
					}
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

	// VALIDASI ATRIBUT TAMBAHAN DI KANTONG AJAIB
	var extra map[string]interface{}
	json.Unmarshal(p.AdditionalInfo, &extra)

	for field, rule := range rules {
		r, ok := rule.(map[string]interface{})
		if !ok {
			continue
		}

		// Sinkronisasi alias khusus dari kodingan awal
		aliasField := field
		if field == "jml_anggota" {
			aliasField = "jumlah_anggota_keluarga"
		}

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

// ==========================================
// 5. INGESTION PIPELINE (SCD Type 2)
// ==========================================
// ProcessIngestion mengelola alur SCD Type 2 (Versioning) untuk Domain Penduduk
func (s *PendudukService) ProcessIngestion(p models.Penduduk) (string, error) {
	// --- EKSEKUSI BLOK VALIDASI SEBELUM MASUK DATABASE ---

	// Tahap 1: Validasi Logika Internal
	if err := s.ValidateInternalKependudukan(p); err != nil {
		return "Error Validation", err
	}

	// Tahap 2: Validasi Logika Lintas Domain (API Call)
	if err := s.ValidateCrossDomainAPI(p); err != nil {
		return "Error Cross-Validation", err
	}

	// --- AKHIR BLOK VALIDASI ---

	last, err := s.Storage.GetLatestByNIK(p.NIK)

	// Skenario A: Data Baru
	if err != nil {
		p.Version = 1
		p.AuditStatus = "PENDING"
		if errCreate := s.Storage.Create(&p); errCreate != nil {
			return "Error", errCreate
		}
		return "Sukses v1 (Initial Entry)", nil
	}

	// Skenario B: Update Data
	isNewer := p.ReferenceDate.After(last.ReferenceDate)
	isHigherAuthority := p.ReferenceDate.Equal(last.ReferenceDate) && p.IsWaliData && !last.IsWaliData

	if isNewer || isHigherAuthority {
		p.ID = 0
		p.Version = last.Version + 1
		p.AuditStatus = "PENDING"

		if errCreate := s.Storage.Create(&p); errCreate != nil {
			return "Error", errCreate
		}
		return fmt.Sprintf("Sukses v%d (Data Updated)", p.Version), nil
	}
	return "Abaikan", fmt.Errorf("data yang dikirim lebih usang")
}

// ==========================================
// 6. UPDATE & AUDIT LOGIC
// ==========================================
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

	if newData.IsWaliData {
		newData.TrustScore = 80.0
	} else {
		newData.TrustScore = 60.0
	}

	errCreate := s.Storage.Create(&newData)
	return newData.Version, errCreate
}

// Logika Bisnis Audit
func (s *PendudukService) ProcessAuditDecision(nikList []string, verdict int) (string, error) {
	verdictText := "INVALID"
	bonus := 0.0

	if text, exists := AuditMap[verdict]; exists {
		verdictText = text
		if verdict == 1 {
			bonus = 20.0
		}
	} else {
		return "", fmt.Errorf("Verdict tidak valid. Gunakan 1 (VALID) atau 2 (INVALID)")
	}

	err := s.Storage.UpdateBulkAuditDecision(nikList, verdictText, bonus)
	return verdictText, err
}
