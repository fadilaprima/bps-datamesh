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

// ==========================================
// 1. FUNGSI VALIDASI INTERNAL (INTRA-DOMAIN)
// ==========================================
func (s *PendidikanService) ValidateInternalPendidikan(p models.RiwayatPendidikan) error {
	// Konversi p.Kelas dari string ke int untuk pengecekan batas kelas
	kelasInt, _ := strconv.Atoi(p.Kelas)

	// Mapping hierarki jenjang untuk mempermudah logika "Lebih dari / Kurang dari"
	jenjangRank := map[string]int{
		"TK/Belum sekolah": 0,
		"SD":               1,
		"SMP":              2,
		"SMA":              3,
		"D1":               4,
		"D2":               5,
		"D3":               6,
		"D4":               7,
		"S1":               7,
		"S2":               8,
		"S3":               9,
	}

	jRank, jExists := jenjangRank[p.Jenjang]

	if jExists {
		// Rule 1: Ijazah vs Jenjang
		if p.Ijazah == "S1" && jRank <= jenjangRank["SMA"] {
			return fmt.Errorf("gagal validasi internal: memiliki ijazah S1 tetapi jenjang tertinggi <= SMA")
		}
		if p.Ijazah == "SMA" && jRank <= jenjangRank["SMP"] {
			return fmt.Errorf("gagal validasi internal: memiliki ijazah SMA tetapi jenjang tertinggi <= SMP")
		}
		if p.Ijazah == "SMP" && jRank <= jenjangRank["SD"] {
			return fmt.Errorf("gagal validasi internal: memiliki ijazah SMP tetapi jenjang tertinggi <= SD")
		}
		if p.Ijazah == "SD" && jRank <= jenjangRank["TK/Belum sekolah"] {
			return fmt.Errorf("gagal validasi internal: memiliki ijazah SD tetapi jenjang tertinggi <= TK/Belum sekolah")
		}

		// Rule 2: Jenjang vs Kelas Tertinggi
		if p.Jenjang == "SD" && kelasInt > 6 {
			return fmt.Errorf("gagal validasi internal: jenjang SD tetapi kelas yang diduduki > 6")
		}
		if (p.Jenjang == "SMP" || p.Jenjang == "SMA" || strings.HasPrefix(p.Jenjang, "D1") || strings.HasPrefix(p.Jenjang, "D2") || strings.HasPrefix(p.Jenjang, "D3")) && kelasInt > 3 {
			return fmt.Errorf("gagal validasi internal: jenjang %s tetapi kelas yang diduduki > 3", p.Jenjang)
		}
		if (p.Jenjang == "D4" || p.Jenjang == "S1" || p.Jenjang == "S3") && kelasInt > 4 {
			return fmt.Errorf("gagal validasi internal: jenjang %s tetapi kelas yang diduduki > 4", p.Jenjang)
		}
		if p.Jenjang == "S2" && kelasInt > 2 {
			return fmt.Errorf("gagal validasi internal: jenjang S2 tetapi kelas yang diduduki > 2")
		}
	}

	// Rule 3: Partisipasi Sekolah vs Kelas
	if p.Partisipasi == "Tidak/Belum Pernah Sekolah" && kelasInt > 0 {
		return fmt.Errorf("gagal validasi internal: status tidak/belum pernah sekolah tetapi kelas tertinggi > 0")
	}

	return nil
}

// ==========================================
// 2. FUNGSI VALIDASI SILANG (CROSS-DOMAIN API)
// ==========================================
func (s *PendidikanService) ValidateCrossDomainAPI(p models.RiwayatPendidikan) error {
	client := &http.Client{Timeout: 3 * time.Second}

	// Cek ke Domain Kependudukan untuk mendapatkan Umur
	baseURLKependudukan := os.Getenv("URL_KEPENDUDUKAN")
	targetURL := fmt.Sprintf("%s/api/v1/domains/penduduk/datasets/%s", baseURLKependudukan, p.NIK)
	
	resp, err := client.Get(targetURL)
	
	// PENAMBAHAN: Blok fail-closed jika koneksi ke domain lain gagal
	if err != nil {
		return fmt.Errorf("gagal menghubungi service kependudukan untuk validasi silang (pastikan service menyala): %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		body, _ := io.ReadAll(resp.Body)

		// PERBAIKAN: Gunakan Object tunggal, bukan Array
		var result map[string]interface{}
		if errJson := json.Unmarshal(body, &result); errJson == nil {
			tglLahir := fmt.Sprintf("%v", result["tanggal_lahir"])

			umur := -1
			if t, parseErr := time.Parse("2006-01-02", tglLahir); parseErr == nil {
				umur = int(time.Since(t).Hours() / 24 / 365.25)
			}

			if umur >= 0 {
				// Cek kelayakan Jenjang SD ke atas
				jenjangRank := map[string]int{"TK/Belum sekolah": 0, "SD": 1, "SMP": 2, "SMA": 3}
				isJenjangSDKeAtas := false
				if rank, exists := jenjangRank[p.Jenjang]; exists && rank >= 1 {
					isJenjangSDKeAtas = true
				} else if p.Jenjang != "TK/Belum sekolah" && p.Jenjang != "" {
					isJenjangSDKeAtas = true
				}

				// Rule: Umur < 5 Tahun dan (partisipasi = Masih Sekolah ATAU jenjang >= SD)
				if umur < 5 && (p.Partisipasi == "Masih Sekolah" || isJenjangSDKeAtas) {
					return fmt.Errorf("gagal validasi lintas domain (kependudukan): umur di bawah 5 tahun tidak wajar untuk partisipasi Masih Sekolah atau menduduki jenjang SD ke atas")
				}

				// Rule: Umur < 18 Tahun dan ijazah >= S1
				if umur < 18 && (strings.Contains(p.Ijazah, "S1") || strings.Contains(p.Ijazah, "S2") || strings.Contains(p.Ijazah, "S3")) {
					return fmt.Errorf("gagal validasi lintas domain (kependudukan): umur di bawah 18 tahun tidak wajar memiliki ijazah setingkat S1 ke atas")
				}
			}
		} else {
			// ANTI SILENT-FAILURE: Berteriak jika format JSON tidak sesuai!
			return fmt.Errorf("gagal parsing JSON dari service kependudukan (Format Beda): %v", errJson)
		}
	}

	return nil
}

// ==========================================
// 3. KODE BAWAAN METADATA VALIDATION
// ==========================================
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

	// 3. VALIDASI JENJANG (Pengecekan Panjang Karakter sesuai Varchar)
	if r, ok := rules["jenjang"].(map[string]interface{}); ok {
		if lengthVal, ok := r["length"].(float64); ok {
			if p.Jenjang != "" && len(p.Jenjang) > int(lengthVal) {
				return false, fmt.Sprintf("Jenjang pendidikan melebihi batas panjang karakter (%d digit)", int(lengthVal))
			}
		}
	}

	// 4. VALIDASI ATRIBUT TAMBAHAN DI KANTONG AJAIB (AdditionalInfo) DENGAN REFLECT
	typ := reflect.TypeOf(models.RiwayatPendidikan{})
	fixedFields := make(map[string]bool)
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		jsonTag := strings.Split(field.Tag.Get("json"), ",")[0]
		if jsonTag != "" && jsonTag != "-" {
			fixedFields[jsonTag] = true
		}
	}

	var extra map[string]interface{}
	json.Unmarshal(p.AdditionalInfo, &extra)

	for field, rule := range rules {
		r, ok := rule.(map[string]interface{})
		if !ok { continue }

		if r["required"] == true {
			if !fixedFields[field] {
				if val, exists := extra[field]; !exists || fmt.Sprintf("%v", val) == "" {
					return false, fmt.Sprintf("Atribut tambahan '%s' wajib diisi sesuai skema", field)
				}
			}
		}
	}

	return true, ""
}

// ==========================================
// 4. INGESTION PIPELINE (SCD Type 2)
// ==========================================
// ProcessIngestion mengelola logika SCD Type 2 (Versioning)
func (s *PendidikanService) ProcessIngestion(p models.RiwayatPendidikan) (string, error) {
	// --- EKSEKUSI BLOK VALIDASI SEBELUM MASUK DATABASE ---
	
	// Tahap 1: Validasi Logika Internal
	if err := s.ValidateInternalPendidikan(p); err != nil {
		return "Error Validation", err
	}

	// Tahap 2: Validasi Logika Lintas Domain (API Call ke Kependudukan)
	if err := s.ValidateCrossDomainAPI(p); err != nil {
		return "Error Cross-Validation", err
	}

	// --- AKHIR BLOK VALIDASI ---

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
	isNewer := p.ReferenceDate.After(last.ReferenceDate)
	isHigherAuthority := p.ReferenceDate.Equal(last.ReferenceDate) && p.IsWaliData && !last.IsWaliData

	if isNewer || isHigherAuthority {
		p.ID = 0
		p.Version = last.Version + 1
		p.AuditStatus = "PENDING" 
		
		if errCreate := s.Storage.Create(&p); errCreate != nil {
			return "Error", errCreate
		}
		return fmt.Sprintf("Sukses v%d", p.Version), nil
	}

	return "Abaikan", fmt.Errorf("data lebih lama dibandingkan data di database")
}

// ==========================================
// 5. UPDATE & AUDIT LOGIC
// ==========================================
// Logika Bisnis untuk Manual Update dari PUT Dataset
func (s *PendidikanService) ProcessManualUpdate(nik string, newData models.RiwayatPendidikan) (int, error) {
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