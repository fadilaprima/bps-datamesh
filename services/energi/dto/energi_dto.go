package dto

import "time"

type EnergiIngestRequest struct {
	// Kunci Logis (Rumah Tangga)
	NoKK              string    `json:"nomor_kartu_keluarga"`

	// Variabel Inti Energi
	IDPelangganPLN    string    `json:"id_pelanggan_pln"`
	DayaTerpasang     string    `json:"daya_terpasang"`
	
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

type EnergiResponse struct {
	Status  string      `json:"status"`
	Message string      `json:"message"`
	Stats   interface{} `json:"stats,omitempty"`
	Errors  []string    `json:"errors,omitempty"`
}