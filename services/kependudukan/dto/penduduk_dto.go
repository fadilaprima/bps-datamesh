package dto

import "time"

type PendudukIngestRequest struct {
	// Variabel Identitas Kependudukan 
	NIK            string    `json:"nomor_induk_kependudukan"`
	NoKK           string    `json:"nomor_kartu_keluarga"`
	Nama           string    `json:"nama"`
	NamaAnggota    string    `json:"nama_anggota_keluarga"`
	JmlAnggota     int       `json:"jumlah_anggota_keluarga"`
	TglLahir       string    `json:"tanggal_lahir"`
	JenisKelamin   string    `json:"jenis_kelamin"`
	StatusKawin    string    `json:"status_kawin"`
	StatusHubungan string    `json:"status_hubungan_keluarga"`
	
	// Variabel Hirarki Wilayah Domisili
	Alamat         string    `json:"alamat"`
	KodeProv       string    `json:"kode_provinsi"`
	KodeKab        string    `json:"kode_kabupaten_kota"`
	KodeKec        string    `json:"kode_kecamatan"`
	KodeDesa       string    `json:"kode_kelurahan_desa"`

	// Variabel Hirarki Wilayah KTP 
	AlamatKTP      string    `json:"alamat_ktp"`
	RTKTP          string    `json:"rt_ktp"`
	RWKTP          string    `json:"rw_ktp"`
	DusunKTP       string    `json:"dusun_ktp"`
	KodeProvKTP    string    `json:"kode_provinsi_ktp"`
	KodeKabKTP     string    `json:"kode_kabupaten_kota_ktp"`
	KodeKecKTP     string    `json:"kode_kecamatan_ktp"`
	KodeDesaKTP    string    `json:"kode_kelurahan_desa_ktp"`

	// Metadata untuk Implementasi Data Mesh 
	// Version: Untuk pelacakan versi data 
	Version       int       `json:"version"`
	// SourceID: Identitas Organisasi Pengirim 
	SourceID      int    `json:"source_id"`
	// AuditStatus : Keputusan audit
	AuditStatus   int    `json:"audit_status"` 
	// TrustScore: Skor kepercayaan terhadap kualitas data yang dikirim
	TrustScore    float64   `json:"trust_score"`
	// ReferenceDate: Tanggal referensi data 
	ReferenceDate time.Time `json:"reference_date"`
}

// Respons
type PendudukResponse struct {
	Status  string      `json:"status"`
	Message string      `json:"message"`
	Stats   interface{} `json:"stats,omitempty"`
	Errors  []string    `json:"errors,omitempty"`
}