package dto

import "time"

type WilayahIngestRequest struct {
	// --- Variabel Hirarki Wilayah (Sesuai Spesifikasi Metadata PDF) ---
	
	// KodeProv: Kode Provinsi standar 2 digit
	KodeProv  string `json:"kode_provinsi"`
	// Provinsi: Nama Provinsi
	Provinsi  string `json:"provinsi"`
	// KodeKab: Kode Kabupaten/Kota standar 4 digit
	KodeKab   string `json:"kode_kabupaten_kota"`
	// Kabupaten: Nama Kabupaten/Kota
	Kabupaten string `json:"kabupaten_kota"`
	// KodeKec: Kode Kecamatan standar 7 digit
	KodeKec   string `json:"kode_kecamatan"`
	// Kecamatan: Nama Kecamatan
	Kecamatan string `json:"kecamatan"`
	// KodeDesa: Kode Kelurahan/Desa standar 10 digit (Natural Key)
	KodeDesa  string `json:"kode_kelurahan_desa"`
	// Desa: Nama Kelurahan/Desa
	Desa      string `json:"kelurahan_desa"`

	// --- Metadata untuk Implementasi Data Mesh & Governance ---
	
	// Version: Untuk pelacakan versi data (SCD Type 2)
	Version       int       `json:"version"`
	// SourceID: Identitas Organisasi Pengirim (Contoh: BPS, KEMENDAGRI)
	SourceID      string    `json:"source_id"`
	// IsWaliData: Flag otoritas sumber data sesuai Inpres 4/2025
	IsWaliData    bool      `json:"is_wali_data"`
	// TrustScore: Skor kepercayaan terhadap kualitas data yang dikirim
	TrustScore    float64   `json:"trust_score"`
	// ReferenceDate: Tanggal referensi data (Kapan data tersebut diambil/valid)
	ReferenceDate time.Time `json:"reference_date"`
}

// WilayahResponse digunakan untuk standarisasi output API kepada pengguna.
type WilayahResponse struct {
	Status  string      `json:"status"`
	Message string      `json:"message"`
	Stats   interface{} `json:"stats,omitempty"`
	Errors  []string    `json:"errors,omitempty"`
}