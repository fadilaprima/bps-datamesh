package app

import (
	"fmt"
	"strings"
	"wilayah/models"
	"wilayah/storage"
)

// WilayahService mengelola seluruh logika bisnis domain wilayah
type WilayahService struct {
	Storage storage.WilayahStorage
}

// --- 1. SOURCE REGISTRY (Pusat Otoritas & Skoring Trust) ---
var SourceRegistry = map[string]struct {
	IsWali     bool
	TrustScore float64
}{
	"BPS":        {IsWali: true, TrustScore: 1.0},  // Wali Data Utama (MFD)
	"KEMENDAGRI": {IsWali: false, TrustScore: 0.9}, // Administrasi Kewilayahan
}

// --- 2. METADATA VALIDATOR (Automated Quality Guardrail) ---
func (s *WilayahService) ValidateWilayahMetadata(w models.MasterWilayah) (bool, string) {
	// A. VALIDASI MANDATORY & PANJANG KARAKTER
	if strings.TrimSpace(w.KodeDesa) == "" || len(w.KodeDesa) != 10 {
		return false, "kode_kelurahan_desa wajib 10 digit (Natural Key)"
	}
	if strings.TrimSpace(w.Desa) == "" {
		return false, "atribut nama kelurahan_desa tidak boleh kosong"
	}

	// B. VALIDASI HIRARKI WILAYAH (Spatial Hierarchy Validation)
	if w.KodeProv == "" || len(w.KodeProv) != 2 {
		return false, "kode_provinsi tidak valid (harus 2 digit)"
	}
	if w.KodeKab == "" || len(w.KodeKab) != 4 {
		return false, "kode_kabupaten_kota tidak valid (harus 4 digit)"
	}
	if w.KodeKec == "" || len(w.KodeKec) != 7 {
		return false, "kode_kecamatan tidak valid (harus 7 digit)"
	}

	return true, ""
}

// --- 3. CONFLICT RESOLUTION & SCD TYPE 2 LOGIC (Anti-Regression Policy) ---
func (s *WilayahService) ProcessIngestion(w models.MasterWilayah) (string, error) {
	// A. Identifikasi data existing untuk kebutuhan Versioning (SCD Type 2)
	lastVersion, err := s.Storage.GetLatestByKode(w.KodeDesa)

	// B. Skenario: Data Belum Terdaftar (Initial Ingestion)
	if err != nil {
		w.Version = 1
		errCreate := s.Storage.Create(&w)
		return "Sukses: Data awal berhasil didaftarkan (v1)", errCreate
	}

	// C. EVALUASI KONFLIK DATA (Conflict Resolution Strategy)
	// 1. Cek Mutlak: Apakah data baru secara waktu memang lebih mutakhir
	isNewerData := w.ReferenceDate.After(lastVersion.ReferenceDate)

	// 2. Cek Pendukung: Kondisi jika tanggal referensi sama
	isSameDate := w.ReferenceDate.Equal(lastVersion.ReferenceDate)
	isHigherAuthority := w.IsWaliData && !lastVersion.IsWaliData
	isHigherScore := w.TrustScore > lastVersion.TrustScore

	// --- LOGIKA ANTI-REGRESI (PENGUATAN) ---
	// Data baru diterima (v2, v3, dst) HANYA JIKA:
	// - Memiliki tanggal referensi yang lebih baru (Temporal Priority)
	// - ATAU Tanggal sama, namun memiliki otoritas/skor kepercayaan lebih tinggi
	canCreateNewVersion := isNewerData || (isSameDate && (isHigherAuthority || isHigherScore))

	if canCreateNewVersion {
		// D. IMPLEMENTASI VERSIONING (SCD Type 2)
		newVersion := *lastVersion
		newVersion.ID = 0 // Memastikan pembuatan record baru (bukan update row lama)
		newVersion.Version = lastVersion.Version + 1

		// Map Metadata Baru ke Record Aktif
		newVersion.SourceID = w.SourceID
		newVersion.TrustScore = w.TrustScore
		newVersion.ReferenceDate = w.ReferenceDate
		newVersion.IsWaliData = w.IsWaliData

		// E. RULE-BASED MERGE (Content Update)
		newVersion.Provinsi = w.Provinsi
		newVersion.Kabupaten = w.Kabupaten
		newVersion.Kecamatan = w.Kecamatan
		newVersion.Desa = w.Desa
		newVersion.KodeProv = w.KodeProv
		newVersion.KodeKab = w.KodeKab
		newVersion.KodeKec = w.KodeKec

		errCreate := s.Storage.Create(&newVersion)
		if errCreate != nil {
			return "Gagal", fmt.Errorf("database error: %v", errCreate)
		}
		return fmt.Sprintf("Sukses: Versi %d berhasil dibuat (Anti-Regression Update)", newVersion.Version), nil
	}

	// F. Penolakan Data (Data Regression Prevention)
	return fmt.Sprintf("Abaikan: Kode %s ditolak (Data existing lebih baru/setara)", w.KodeDesa), fmt.Errorf("data outdated")
}
