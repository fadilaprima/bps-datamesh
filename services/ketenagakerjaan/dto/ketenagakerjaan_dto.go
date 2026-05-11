package dto

import "time"

type KetenagakerjaanIngestRequest struct {
	NIK                       string    `json:"nomor_induk_kependudukan"`
	StatusBekerja             string    `json:"status_bekerja"`
	LapanganUsahaUtama        string    `json:"lapangan_usaha_dari_pekerjaan_utama"`
	StatusPekerjaanUtama      string    `json:"status_dalam_pekerjaan_utama"`
	KepemilikanUsaha          string    `json:"kepemilikan_usaha"`
	JumlahUsaha               int       `json:"jumlah_usaha"`
	LapanganUsahaUsahaUtama   string    `json:"lapangan_usaha_dari_usaha_utama"`
	JumlahPekerjaDibayar      int       `json:"jumlah_pekerja_yang_dibayar_dari_usaha_utama"`
	JumlahPekerjaTidakDibayar int       `json:"jumlah_pekerja_yang_tidak_dibayar_dari_usaha_utama"`
	OmzetUsahaUtama           string    `json:"omzet_usaha_utama"`

	// Metadata untuk Implementasi Data Mesh & Governance 
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
type KetenagakerjaanResponse struct {
	Status  string      `json:"status"`
	Message string      `json:"message"`
	Stats   interface{} `json:"stats,omitempty"`
	Errors  []string    `json:"errors,omitempty"`
}