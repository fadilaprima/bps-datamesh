package dto

import "time"

type PendudukIngestRequest struct {
	// --- Variabel Identitas Kependudukan (Sesuai Kode Lama & Metadata) ---
	NIK            string    `json:"nomor_induk_kependudukan"`
	NoKK           string    `json:"nomor_kartu_keluarga"`
	Nama           string    `json:"nama"`
	NamaAnggota    string    `json:"nama_anggota_keluarga"`
	JmlAnggota     int       `json:"jumlah_anggota_keluarga"`
	TglLahir       string    `json:"tanggal_lahir"`
	JenisKelamin   string    `json:"jenis_kelamin"`
	StatusKawin    string    `json:"status_kawin"`
	StatusHubungan string    `json:"status_hubungan_keluarga"`
	
	// --- Variabel Hirarki Wilayah Domisili ---
	Alamat         string    `json:"alamat"`
	KodeProv       string    `json:"kode_provinsi"`
	KodeKab        string    `json:"kode_kabupaten_kota"`
	KodeKec        string    `json:"kode_kecamatan"`
	KodeDesa       string    `json:"kode_kelurahan_desa"`

	// --- Variabel Hirarki Wilayah KTP ---
	AlamatKTP      string    `json:"alamat_ktp"`
	RTKTP          string    `json:"rt_ktp"`
	RWKTP          string    `json:"rw_ktp"`
	DusunKTP       string    `json:"dusun_ktp"`
	KodeProvKTP    string    `json:"kode_provinsi_ktp"`
	KodeKabKTP     string    `json:"kode_kabupaten_kota_ktp"`
	KodeKecKTP     string    `json:"kode_kecamatan_ktp"`
	KodeDesaKTP    string    `json:"kode_kelurahan_desa_ktp"`

	// --- Metadata untuk Data Mesh & Governance (SCD Type 2) ---
	Version        int       `json:"version"`
	SourceID       string    `json:"source_id"`
	IsWaliData     bool      `json:"is_wali_data"`
	TrustScore     float64   `json:"trust_score"`
	ReferenceDate  time.Time `json:"reference_date"`
}

// PendudukResponse: Standarisasi Output API agar seragam dengan Domain Wilayah
type PendudukResponse struct {
	Status  string      `json:"status"`
	Message string      `json:"message"`
	Stats   interface{} `json:"stats,omitempty"`
	Errors  []string    `json:"errors,omitempty"`
}