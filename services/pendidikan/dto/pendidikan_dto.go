package dto

import "time"

type PendidikanIngestRequest struct {
	NIK         string    `json:"nomor_induk_kependudukan"`
	Partisipasi string    `json:"partisipasi_sekolah"`
	Jenjang     string    `json:"jenjang_tertinggi_yang_diduduki"`
	Kelas       string    `json:"kelas_tertinggi_yang_diduduki"`
	Ijazah      string    `json:"ijazah_tertinggi_yang_dimiliki"`
	
	// Metadata untuk Implementasi Data Mesh & Governance 
	// Version: Untuk pelacakan versi data 
	Version       int       `json:"version"`
	// SourceID: Identitas Organisasi Pengirim 
	SourceID      string    `json:"source_id"`
	// IsWaliData: Flag otoritas sumber data 
	IsWaliData    bool      `json:"is_wali_data"`
	// TrustScore: Skor kepercayaan terhadap kualitas data yang dikirim
	TrustScore    float64   `json:"trust_score"`
	// ReferenceDate: Tanggal referensi data 
	ReferenceDate time.Time `json:"reference_date"`
}

type PendidikanResponse struct {
	Status  string      `json:"status"`
	Message string      `json:"message"`
	Stats   interface{} `json:"stats,omitempty"`
	Errors  []string    `json:"errors,omitempty"`
}