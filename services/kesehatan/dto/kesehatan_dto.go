package dto

import "time"

type KesehatanIngestRequest struct {
	// Identitas Individu
	NIK                     string    `json:"nomor_induk_kependudukan"`
	
	// Variabel Observasi Kesehatan
	PbiNas                  string    `json:"pbi_nas"`
	PbiPemda                string    `json:"pbi_pemda"`
	KondisiGizi             string    `json:"kondisi_gizi"`
	Penglihatan             string    `json:"penglihatan"`
	Pendengaran             string    `json:"pendengaran"`
	BerjalanNaikTangga      string    `json:"berjalan_atau_naik_tangga"`
	MenggunakanTanganJari   string    `json:"menggunakan_tangan_jari"`
	BelajarIntelektual      string    `json:"belajar_kemampuan_intelektual"`
	PengendalianPerilaku    string    `json:"pengendalian_perilaku"`
	BerbicaraKomunikasi     string    `json:"berbicara_komunikasi"`
	MengurusDiri            string    `json:"mengurus_diri"`
	MengingatBerkonsentrasi string    `json:"mengingat_berkonsentrasi"`
	KesedihanDepresi        string    `json:"kesedihan_depresi"`
	PenyakitKronis          string    `json:"penyakit_kronis"`
	
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

type KesehatanResponse struct {
	Status  string      `json:"status"`
	Message string      `json:"message"`
	Stats   interface{} `json:"stats,omitempty"`
	Errors  []string    `json:"errors,omitempty"`
}
