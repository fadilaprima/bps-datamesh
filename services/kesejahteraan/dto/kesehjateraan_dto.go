package dto

import "time"

type KesehjateraanIngestRequest struct {
	// Kunci Logis (Rumah Tangga)
	NoKK              string `json:"nomor_kartu_keluarga"`
	
	// Variabel Observasi Kesejahteraan
	DesilNasional     int    `json:"desil_nasional"`
	BahanBakarMemasak string `json:"bahan_bakar_utama_memasak"`
	KepemilikanAset   int    `json:"kepemilikan_aset"`
	
	// Aset Bergerak
	AsetGas           int    `json:"aset_bergerak_tabung_gas"`
	AsetKulkas        int    `json:"aset_bergerak_lemari_es"`
	AsetAC            int    `json:"aset_bergerak_ac"`
	AsetPemanasAir    int    `json:"aset_bergerak_pemanas_air"`
	AsetTelepon       int    `json:"aset_bergerak_telepon_rumah"`
	AsetTV            int    `json:"aset_bergerak_tv_datar"`
	AsetEmas          int    `json:"aset_bergerak_emas_perhiasan"`
	AsetLaptop        int    `json:"aset_bergerak_komputer_laptop_tablet"`
	AsetMotor         int    `json:"aset_bergerak_sepeda_motor"`
	AsetSepeda        int    `json:"aset_bergerak_sepeda"`
	AsetMobil         int    `json:"aset_bergerak_mobil"`
	AsetPerahu        int    `json:"aset_bergerak_perahu"`
	AsetPerahuMotor   int    `json:"aset_bergerak_kapal_perahu_motor"`
	AsetSmartphone    int    `json:"aset_bergerak_smartphone"`
	
	// Aset Tidak Bergerak
	AsetLahanLain     int    `json:"aset_tidak_bergerak_lahan_lainnya"`
	AsetRumahLain     int    `json:"aset_tidak_bergerak_rumah_lainnya"`
	
	// Ternak
	TernakSapi        int    `json:"jumlah_ternak_sapi"`
	TernakKerbau      int    `json:"jumlah_ternak_kerbau"`
	TernakKuda        int    `json:"jumlah_ternak_kuda"`
	TernakBabi        int    `json:"jumlah_ternak_babi"`
	TernakKambing     int    `json:"jumlah_ternak_kambing_domba"`

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

type KesejahteraanResponse struct {
	Status  string      `json:"status"`
	Message string      `json:"message"`
	Stats   interface{} `json:"stats,omitempty"`
	Errors  []string    `json:"errors,omitempty"`
}