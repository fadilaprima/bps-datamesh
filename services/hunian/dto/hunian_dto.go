package dto

import "time"

type HunianIngestRequest struct {
	// Kunci Logis (Rumah Tangga)
	NoKK              string `json:"nomor_kartu_keluarga"`

	// Variabel Observasi Hunian
	StatusKepemilikan string `json:"status_kepemilikan_rumah"`
	JenisLantai       string `json:"jenis_lantai_terluas"`
	LuasLantai        int    `json:"luas_lantai"`
	JenisDinding      string `json:"jenis_dinding_terluas"`
	JenisAtap         string `json:"jenis_atap_terluas"`
	SumberAirMinum    string `json:"sumber_air_minum_utama"`
	SumberPenerangan  string `json:"sumber_penerangan_utama"`
	FasilitasBAB      string `json:"fasilitas_bab"`
	JenisKloset       string `json:"jenis_kloset"`
	PembuanganTinja   string `json:"pembuangan_akhir_tinja"`

	// Metadata untuk Implementasi Data Mesh & Governance 
	// Version: Untuk pelacakan versi data (SCD Type 2)
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

type HunianResponse struct {
	Status  string      `json:"status"`
	Message string      `json:"message"`
	Stats   interface{} `json:"stats,omitempty"`
	Errors  []string    `json:"errors,omitempty"`
}