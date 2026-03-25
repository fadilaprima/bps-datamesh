package app

import (
	"fmt"
	"pendidikan/models"
	"pendidikan/storage"
	"strconv"
	
)

type PendidikanService struct {
	Storage storage.PendidikanStorage
}

var PendidikanSourceRegistry = map[string]struct {
	IsWali     bool
	TrustScore float64
}{
	"KEMENDIKBUD": {IsWali: true, TrustScore: 1.0},
	"KEMENAG":     {IsWali: false, TrustScore: 0.9},
	"BPS":         {IsWali: false, TrustScore: 0.8},
}

// --- 2. METADATA VALIDATOR (KODE ASLI ARSITEK) ---
func (s *PendidikanService) ValidatePendidikanMetadata(p models.RiwayatPendidikan) (bool, string) {
	if len(p.NIK) != 16 { return false, "nomor_induk_kependudukan harus 16 digit" }

	if p.Partisipasi != "" {
		if len(p.Partisipasi) > 2 { return false, "partisipasi_sekolah maksimal 2 digit" }
		if p.Partisipasi != "1" && p.Partisipasi != "2" && p.Partisipasi != "3" {
			return false, "partisipasi_sekolah harus kode 1, 2, atau 3"
		}
	}

	if p.Jenjang != "" {
		val, _ := strconv.Atoi(p.Jenjang)
		if val < 1 || val > 22 { return false, "jenjang_tertinggi harus kode 1 s.d 22" }
	}

	if p.Kelas != "" {
		val, _ := strconv.Atoi(p.Kelas)
		if val < 1 || val > 8 { return false, "kelas_tertinggi harus kode 1 s.d 8" }
	}

	if p.Ijazah != "" {
		val, _ := strconv.Atoi(p.Ijazah)
		if val < 1 || val > 23 { return false, "ijazah_tertinggi harus kode 01 s.d 23" }
	}
	return true, ""
}

// --- 3. LOGIKA INGESTI & RULE-BASED MERGE (SCD TYPE 2) ---
func (s *PendidikanService) ProcessIngestion(p models.RiwayatPendidikan) (string, error) {
	lastVersion, err := s.Storage.GetLatestByNIK(p.NIK)

	if err != nil {
		p.Version = 1
		return "Sukses v1", s.Storage.Create(&p)
	}

	// Hukum Anti-Regresi: Tahun Terbaru Tetap Masuk
	isNewerData := p.ReferenceDate.After(lastVersion.ReferenceDate)
	isSameDate := p.ReferenceDate.Equal(lastVersion.ReferenceDate)
	isHigherAuthority := p.IsWaliData && !lastVersion.IsWaliData
	isHigherScore := p.TrustScore > lastVersion.TrustScore

	canCreateNewVersion := isNewerData || (isSameDate && (isHigherAuthority || isHigherScore))

	if canCreateNewVersion {
		newVersion := *lastVersion
		newVersion.ID = 0
		newVersion.Version = lastVersion.Version + 1
		newVersion.ReferenceDate = p.ReferenceDate
		newVersion.SourceID = p.SourceID
		newVersion.TrustScore = p.TrustScore
		newVersion.IsWaliData = p.IsWaliData

		// RULE-BASED MERGE: Jahitan Sesuai Otoritas Sumber
		switch p.SourceID {
		case "KEMENDIKBUD", "KEMENAG":
			newVersion.Partisipasi = p.Partisipasi
			newVersion.Jenjang = p.Jenjang
			newVersion.Kelas = p.Kelas
			newVersion.Ijazah = p.Ijazah
		case "BPS":
			newVersion.Partisipasi = p.Partisipasi
			newVersion.Kelas = p.Kelas
		default:
			newVersion.Partisipasi = p.Partisipasi
		}

		return fmt.Sprintf("Sukses v%d", newVersion.Version), s.Storage.Create(&newVersion)
	}
	return "Abaikan: Data Outdated", fmt.Errorf("outdated")
}