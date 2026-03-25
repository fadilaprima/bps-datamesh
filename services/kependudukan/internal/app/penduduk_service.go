<<<<<<< HEAD
package app

import (
	"fmt"
	"kependudukan/models"
	"kependudukan/storage"
	"strings"
)

type PendudukService struct {
	Storage storage.PendudukStorage
}

// --- 1. SOURCE REGISTRY (Pusat Otoritas Berdasarkan Inpres 4/2025) ---
var PendudukSourceRegistry = map[string]struct {
	IsWali     bool
	TrustScore float64
}{
	"KEMENDAGRI": {IsWali: true, TrustScore: 1.0},  // Wali Data Identitas
	"BPS":        {IsWali: false, TrustScore: 0.9}, // Aggregator Utama
}

// --- 2. METADATA VALIDATOR (Versi Lengkap Pilihan Arsitek) ---
// Melakukan pengecekan menyeluruh terhadap integritas setiap variabel kependudukan
func (s *PendudukService) ValidatePendudukMetadata(p models.Penduduk) (bool, string) {
	// A. VALIDASI (MANDATORY)
	if len(p.NIK) != 16 {
		return false, "NIK harus 16 digit"
	}
	if strings.TrimSpace(p.Nama) == "" {
		return false, "Nama tidak boleh kosong"
	}

	// B. VALIDASI HIRARKI WILAYAH DOMISILI (OPTIONAL BUT STRUCTURED)
	if p.KodeProv != "" && len(p.KodeProv) != 2 {
		return false, "Kode Provinsi harus 2 digit"
	}
	if p.KodeKab != "" && len(p.KodeKab) != 4 {
		return false, "Kode KabKot harus 4 digit"
	}
	if p.KodeKec != "" && len(p.KodeKec) != 7 {
		return false, "Kode Kecamatan harus 7 digit"
	}
	if p.KodeDesa != "" && len(p.KodeDesa) != 10 {
		return false, "Kode Desa harus 10 digit"
	}

	// C. VALIDASI HIRARKI WILAYAH KTP (OPTIONAL BUT STRUCTURED)
	if p.KodeProvKTP != "" && len(p.KodeProvKTP) != 2 {
		return false, "Kode Provinsi KTP harus 2 digit"
	}
	if p.KodeKabKTP != "" && len(p.KodeKabKTP) != 4 {
		return false, "Kode KabKot KTP harus 4 digit"
	}
	if p.KodeKecKTP != "" && len(p.KodeKecKTP) != 7 {
		return false, "Kode Kecamatan KTP harus 7 digit"
	}
	if p.KodeDesaKTP != "" && len(p.KodeDesaKTP) != 10 {
		return false, "Kode Desa KTP harus 10 digit"
	}

	// D. VALIDASI KATEGORIKAL
	if p.JenisKelamin != "" && (p.JenisKelamin != "1" && p.JenisKelamin != "2") {
		return false, "Jenis Kelamin tidak valid (Gunakan 1 atau 2)"
	}
	if p.JmlAnggota < 0 {
		return false, "Jumlah anggota keluarga tidak logis"
	}

	return true, ""
}

// --- 3. CONFLICT RESOLUTION & RULE-BASED MERGE (SCD TYPE 2) ---
func (s *PendudukService) ProcessIngestion(p models.Penduduk) (string, error) {
	// A. Identifikasi data existing (Snapshot versi terakhir)
	lastVersion, err := s.Storage.GetLatestByNIK(p.NIK)

	// B. Skenario: Data Belum Terdaftar (Initial Entry)
	if err != nil {
		p.Version = 1
		errCreate := s.Storage.Create(&p)
		return "Sukses v1: Data awal didaftarkan", errCreate
	}

	// C. EVALUASI KONFLIK (HUKUM UTAMA: TEMPORAL PRIORITY)
	// 1. Apakah data baru memiliki tanggal referensi yang lebih baru?
	isNewerData := p.ReferenceDate.After(lastVersion.ReferenceDate)

	// 2. Tie-Breaker (Jika tanggal sama, cek Otoritas/TrustScore)
	isSameDate := p.ReferenceDate.Equal(lastVersion.ReferenceDate)
	isHigherAuthority := p.IsWaliData && !lastVersion.IsWaliData
	isHigherScore := p.TrustScore > lastVersion.TrustScore

	// SYARAT PEMBUATAN VERSI BARU
	canCreateNewVersion := isNewerData || (isSameDate && (isHigherAuthority || isHigherScore))

	if canCreateNewVersion {
		// D. IMPLEMENTASI VERSIONING (Snapshot Merge)
		newVersion := *lastVersion
		newVersion.ID = 0
		newVersion.Version = lastVersion.Version + 1

		// Metadata Ingesti
		newVersion.SourceID = p.SourceID
		newVersion.TrustScore = p.TrustScore
		newVersion.ReferenceDate = p.ReferenceDate
		newVersion.IsWaliData = p.IsWaliData

		// E. RULE-BASED MERGE (Content Update Berdasarkan Otoritas Sumber)
		// Meskipun data lebih baru, update field dilakukan secara selektif
		switch p.SourceID {
		case "KEMENDAGRI":
			// Wali Data Identitas: Berhak update data Legal & Domisili
			newVersion.NoKK = p.NoKK
			newVersion.NamaAnggota = p.NamaAnggota
			newVersion.JmlAnggota = p.JmlAnggota
			newVersion.Nama = p.Nama
			newVersion.TglLahir = p.TglLahir
			newVersion.JenisKelamin = p.JenisKelamin
			newVersion.StatusKawin = p.StatusKawin
			newVersion.StatusHubungan = p.StatusHubungan
			newVersion.Alamat = p.Alamat
			newVersion.KodeProv = p.KodeProv
			newVersion.KodeKab = p.KodeKab
			newVersion.KodeKec = p.KodeKec
			newVersion.KodeDesa = p.KodeDesa
			newVersion.AlamatKTP = p.AlamatKTP
			newVersion.RTKTP = p.RTKTP
			newVersion.RWKTP = p.RWKTP
			newVersion.DusunKTP = p.DusunKTP
			newVersion.KodeProvKTP = p.KodeProvKTP
			newVersion.KodeKabKTP = p.KodeKabKTP
			newVersion.KodeKecKTP = p.KodeKecKTP
			newVersion.KodeDesaKTP = p.KodeDesaKTP

		case "BPS":
			// Wali Data Lapangan: Hanya update data atribut riil/domisili
			newVersion.Alamat = p.Alamat
			newVersion.JmlAnggota = p.JmlAnggota
			newVersion.RTKTP = p.RTKTP
			newVersion.RWKTP = p.RWKTP
			newVersion.KodeDesa = p.KodeDesa
			// Identitas Legal tetap menggunakan versi Dukcapil sebelumnya

		default:
			// Instansi Lain: Hanya update info tambahan/domisili
			newVersion.Alamat = p.Alamat
			newVersion.JmlAnggota = p.JmlAnggota
		}

		errCreate := s.Storage.Create(&newVersion)
		if errCreate != nil {
			return "Gagal", fmt.Errorf("database error: %v", errCreate)
		}
		return fmt.Sprintf("Sukses v%d: Data diperbarui", newVersion.Version), nil
	}

	// F. PENOLAKAN DATA (Regresi Terdeteksi)
	return fmt.Sprintf("Abaikan: NIK %s ditolak (Data existing lebih mutakhir)", p.NIK), fmt.Errorf("data outdated")
=======
package app

import (
	"fmt"
	"kependudukan/models"
	"kependudukan/storage"
	"strings"
)

type PendudukService struct {
	Storage storage.PendudukStorage
}

// --- 1. SOURCE REGISTRY (Pusat Otoritas Berdasarkan Inpres 4/2025) ---
var PendudukSourceRegistry = map[string]struct {
	IsWali     bool
	TrustScore float64
}{
	"KEMENDAGRI": {IsWali: true, TrustScore: 1.0},  // Wali Data Identitas
	"BPS":        {IsWali: false, TrustScore: 0.9}, // Aggregator Utama
}

// --- 2. METADATA VALIDATOR (Versi Lengkap Pilihan Arsitek) ---
// Melakukan pengecekan menyeluruh terhadap integritas setiap variabel kependudukan
func (s *PendudukService) ValidatePendudukMetadata(p models.Penduduk) (bool, string) {
	// A. VALIDASI (MANDATORY)
	if len(p.NIK) != 16 {
		return false, "NIK harus 16 digit"
	}
	if strings.TrimSpace(p.Nama) == "" {
		return false, "Nama tidak boleh kosong"
	}

	// B. VALIDASI HIRARKI WILAYAH DOMISILI (OPTIONAL BUT STRUCTURED)
	if p.KodeProv != "" && len(p.KodeProv) != 2 {
		return false, "Kode Provinsi harus 2 digit"
	}
	if p.KodeKab != "" && len(p.KodeKab) != 4 {
		return false, "Kode KabKot harus 4 digit"
	}
	if p.KodeKec != "" && len(p.KodeKec) != 7 {
		return false, "Kode Kecamatan harus 7 digit"
	}
	if p.KodeDesa != "" && len(p.KodeDesa) != 10 {
		return false, "Kode Desa harus 10 digit"
	}

	// C. VALIDASI HIRARKI WILAYAH KTP (OPTIONAL BUT STRUCTURED)
	if p.KodeProvKTP != "" && len(p.KodeProvKTP) != 2 {
		return false, "Kode Provinsi KTP harus 2 digit"
	}
	if p.KodeKabKTP != "" && len(p.KodeKabKTP) != 4 {
		return false, "Kode KabKot KTP harus 4 digit"
	}
	if p.KodeKecKTP != "" && len(p.KodeKecKTP) != 7 {
		return false, "Kode Kecamatan KTP harus 7 digit"
	}
	if p.KodeDesaKTP != "" && len(p.KodeDesaKTP) != 10 {
		return false, "Kode Desa KTP harus 10 digit"
	}

	// D. VALIDASI KATEGORIKAL
	if p.JenisKelamin != "" && (p.JenisKelamin != "1" && p.JenisKelamin != "2") {
		return false, "Jenis Kelamin tidak valid (Gunakan 1 atau 2)"
	}
	if p.JmlAnggota < 0 {
		return false, "Jumlah anggota keluarga tidak logis"
	}

	return true, ""
}

// --- 3. CONFLICT RESOLUTION & RULE-BASED MERGE (SCD TYPE 2) ---
func (s *PendudukService) ProcessIngestion(p models.Penduduk) (string, error) {
	// A. Identifikasi data existing (Snapshot versi terakhir)
	lastVersion, err := s.Storage.GetLatestByNIK(p.NIK)

	// B. Skenario: Data Belum Terdaftar (Initial Entry)
	if err != nil {
		p.Version = 1
		errCreate := s.Storage.Create(&p)
		return "Sukses v1: Data awal didaftarkan", errCreate
	}

	// C. EVALUASI KONFLIK (HUKUM UTAMA: TEMPORAL PRIORITY)
	// 1. Apakah data baru memiliki tanggal referensi yang lebih baru?
	isNewerData := p.ReferenceDate.After(lastVersion.ReferenceDate)

	// 2. Tie-Breaker (Jika tanggal sama, cek Otoritas/TrustScore)
	isSameDate := p.ReferenceDate.Equal(lastVersion.ReferenceDate)
	isHigherAuthority := p.IsWaliData && !lastVersion.IsWaliData
	isHigherScore := p.TrustScore > lastVersion.TrustScore

	// SYARAT PEMBUATAN VERSI BARU
	canCreateNewVersion := isNewerData || (isSameDate && (isHigherAuthority || isHigherScore))

	if canCreateNewVersion {
		// D. IMPLEMENTASI VERSIONING (Snapshot Merge)
		newVersion := *lastVersion
		newVersion.ID = 0
		newVersion.Version = lastVersion.Version + 1

		// Metadata Ingesti
		newVersion.SourceID = p.SourceID
		newVersion.TrustScore = p.TrustScore
		newVersion.ReferenceDate = p.ReferenceDate
		newVersion.IsWaliData = p.IsWaliData

		// E. RULE-BASED MERGE (Content Update Berdasarkan Otoritas Sumber)
		// Meskipun data lebih baru, update field dilakukan secara selektif
		switch p.SourceID {
		case "KEMENDAGRI":
			// Wali Data Identitas: Berhak update data Legal & Domisili
			newVersion.NoKK = p.NoKK
			newVersion.NamaAnggota = p.NamaAnggota
			newVersion.JmlAnggota = p.JmlAnggota
			newVersion.Nama = p.Nama
			newVersion.TglLahir = p.TglLahir
			newVersion.JenisKelamin = p.JenisKelamin
			newVersion.StatusKawin = p.StatusKawin
			newVersion.StatusHubungan = p.StatusHubungan
			newVersion.Alamat = p.Alamat
			newVersion.KodeProv = p.KodeProv
			newVersion.KodeKab = p.KodeKab
			newVersion.KodeKec = p.KodeKec
			newVersion.KodeDesa = p.KodeDesa
			newVersion.AlamatKTP = p.AlamatKTP
			newVersion.RTKTP = p.RTKTP
			newVersion.RWKTP = p.RWKTP
			newVersion.DusunKTP = p.DusunKTP
			newVersion.KodeProvKTP = p.KodeProvKTP
			newVersion.KodeKabKTP = p.KodeKabKTP
			newVersion.KodeKecKTP = p.KodeKecKTP
			newVersion.KodeDesaKTP = p.KodeDesaKTP

		case "BPS":
			// Wali Data Lapangan: Hanya update data atribut riil/domisili
			newVersion.Alamat = p.Alamat
			newVersion.JmlAnggota = p.JmlAnggota
			newVersion.RTKTP = p.RTKTP
			newVersion.RWKTP = p.RWKTP
			newVersion.KodeDesa = p.KodeDesa
			// Identitas Legal tetap menggunakan versi Dukcapil sebelumnya

		default:
			// Instansi Lain: Hanya update info tambahan/domisili
			newVersion.Alamat = p.Alamat
			newVersion.JmlAnggota = p.JmlAnggota
		}

		errCreate := s.Storage.Create(&newVersion)
		if errCreate != nil {
			return "Gagal", fmt.Errorf("database error: %v", errCreate)
		}
		return fmt.Sprintf("Sukses v%d: Data diperbarui", newVersion.Version), nil
	}

	// F. PENOLAKAN DATA (Regresi Terdeteksi)
	return fmt.Sprintf("Abaikan: NIK %s ditolak (Data existing lebih mutakhir)", p.NIK), fmt.Errorf("data outdated")
>>>>>>> a935de74a96c3dd35516385ff2e46b34b9d60ef1
}