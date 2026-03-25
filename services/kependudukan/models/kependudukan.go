package models

import "time"

type Penduduk struct {
	// 1. IDENTITAS UTAMA (SCD Type 2 Ready)
	// ID adalah Primary Key unik untuk tiap baris (v1, v2 beda ID)
	// NIK adalah Natural Key (Index) untuk melacak sejarah orang yang sama
	ID             uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	NIK            string    `gorm:"index;column:nomor_induk_kependudukan" json:"nomor_induk_kependudukan" parquet:"name=nomor_induk_kependudukan, type=UTF8"`
	NoKK           string    `gorm:"column:nomor_kartu_keluarga" json:"nomor_kartu_keluarga" parquet:"name=nomor_kartu_keluarga, type=UTF8"`
	Nama           string    `gorm:"column:nama" json:"nama" parquet:"name=nama, type=UTF8"`
	NamaAnggota    string    `gorm:"column:nama_anggota_keluarga" json:"nama_anggota_keluarga" parquet:"name=nama_anggota_keluarga, type=UTF8"`
	JmlAnggota     int       `gorm:"column:jumlah_anggota_keluarga" json:"jumlah_anggota_keluarga" parquet:"name=jumlah_anggota_keluarga, type=INT32"`
	
	// 2. DATA DEMOGRAFI & STATUS
	TglLahir       string    `gorm:"column:tanggal_lahir" json:"tanggal_lahir" parquet:"name=tanggal_lahir, type=UTF8"`
	JenisKelamin   string    `gorm:"column:jenis_kelamin" json:"jenis_kelamin" parquet:"name=jenis_kelamin, type=UTF8"`
	StatusKawin    string    `gorm:"column:status_kawin" json:"status_kawin" parquet:"name=status_kawin, type=UTF8"`
	StatusHubungan string    `gorm:"column:status_hubungan_keluarga" json:"status_hubungan_keluarga" parquet:"name=status_hubungan_keluarga, type=UTF8"`
	
	// 3. DATA WILAYAH DOMISILI (Stitching Point)
	Alamat         string    `gorm:"column:alamat" json:"alamat" parquet:"name=alamat, type=UTF8"`
	KodeProv       string    `gorm:"column:kode_provinsi" json:"kode_provinsi" parquet:"name=kode_provinsi, type=UTF8"`
	KodeKab        string    `gorm:"column:kode_kabupaten_kota" json:"kode_kabupaten_kota" parquet:"name=kode_kabupaten_kota, type=UTF8"`
	KodeKec        string    `gorm:"column:kode_kecamatan" json:"kode_kecamatan" parquet:"name=kode_kecamatan, type=UTF8"`
	KodeDesa       string    `gorm:"column:kode_kelurahan_desa" json:"kode_kelurahan_desa" parquet:"name=kode_kelurahan_desa, type=UTF8"`

	// 4. DATA WILAYAH KTP (Standard Administrasi)
	AlamatKTP      string    `gorm:"column:alamat_ktp" json:"alamat_ktp" parquet:"name=alamat_ktp, type=UTF8"`
	RTKTP          string    `gorm:"column:rt_ktp" json:"rt_ktp" parquet:"name=rt_ktp, type=UTF8"`
	RWKTP          string    `gorm:"column:rw_ktp" json:"rw_ktp" parquet:"name=rw_ktp, type=UTF8"`
	DusunKTP       string    `gorm:"column:dusun_ktp" json:"dusun_ktp" parquet:"name=dusun_ktp, type=UTF8"`
	KodeProvKTP    string    `gorm:"column:kode_provinsi_ktp" json:"kode_provinsi_ktp" parquet:"name=kode_provinsi_ktp, type=UTF8"`
	KodeKabKTP     string    `gorm:"column:kode_kabupaten_kota_ktp" json:"kode_kabupaten_kota_ktp" parquet:"name=kode_kabupaten_kota_ktp, type=UTF8"`
	KodeKecKTP     string    `gorm:"column:kode_kecamatan_ktp" json:"kode_kecamatan_ktp" parquet:"name=kode_kecamatan_ktp, type=UTF8"`
	KodeDesaKTP    string    `gorm:"column:kode_kelurahan_desa_ktp" json:"kode_kelurahan_desa_ktp" parquet:"name=kode_kelurahan_desa_ktp, type=UTF8"`

	// 5. METADATA INGESTI & GOVERNANCE (Lifecycle)
	Version        int       `gorm:"column:version" json:"version"`
	SourceID       string    `gorm:"column:source_id" json:"source_id" parquet:"name=source_id, type=UTF8"`
	IsWaliData     bool      `gorm:"column:is_wali_data" json:"is_wali_data" parquet:"name=is_wali_data, type=BOOLEAN"`
	TrustScore     float64   `gorm:"column:trust_score" json:"trust_score" parquet:"name=trust_score, type=DOUBLE"`
	ReferenceDate  time.Time `gorm:"column:reference_date" json:"reference_date" parquet:"name=reference_date, type=INT64, convertedtype=TIMESTAMP_MILLIS"`
	
	// Field Tambahan untuk 13 Endpoints (Lifecycle & Audit)
	IsDeleted      bool      `gorm:"column:is_deleted;default:false" json:"is_deleted"`
	AuditStatus    string    `gorm:"column:audit_status;default:'PENDING'" json:"audit_status"` // PENDING, APPROVED, REJECTED
	SchemaVersion  string    `gorm:"column:schema_version" json:"schema_version"`
	UpdatedAt      time.Time `gorm:"autoUpdateTime;column:updated_at" json:"updated_at"`
}