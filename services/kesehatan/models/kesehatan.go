package models

import (
	"time"

	"gorm.io/datatypes"
)

type RekamKesehatan struct {
	ID                        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	NIK                       string    `gorm:"index;column:nomor_induk_kependudukan" json:"nomor_induk_kependudukan" parquet:"name=nomor_induk_kependudukan, type=UTF8"`
	
	// Variabel Observasi Kesehatan (Sesuai List Kamu)
	PbiNas                    string    `gorm:"column:pbi_nas" json:"pbi_nas" parquet:"name=pbi_nas, type=UTF8"`
	PbiPemda                  string    `gorm:"column:pbi_pemda" json:"pbi_pemda" parquet:"name=pbi_pemda, type=UTF8"`
	KondisiGizi               string    `gorm:"column:kondisi_gizi" json:"kondisi_gizi" parquet:"name=kondisi_gizi, type=UTF8"`
	Penglihatan               string    `gorm:"column:penglihatan" json:"penglihatan" parquet:"name=penglihatan, type=UTF8"`
	Pendengaran               string    `gorm:"column:pendengaran" json:"pendengaran" parquet:"name=pendengaran, type=UTF8"`
	BerjalanNaikTangga        string    `gorm:"column:berjalan_atau_naik_tangga" json:"berjalan_atau_naik_tangga" parquet:"name=berjalan_atau_naik_tangga, type=UTF8"`
	MenggunakanTanganJari     string    `gorm:"column:menggunakan_tangan_jari" json:"menggunakan_tangan_jari" parquet:"name=menggunakan_tangan_jari, type=UTF8"`
	BelajarIntelektual        string    `gorm:"column:belajar_kemampuan_intelektual" json:"belajar_kemampuan_intelektual" parquet:"name=belajar_kemampuan_intelektual, type=UTF8"`
	PengendalianPerilaku      string    `gorm:"column:pengendalian_perilaku" json:"pengendalian_perilaku" parquet:"name=pengendalian_perilaku, type=UTF8"`
	BerbicaraKomunikasi       string    `gorm:"column:berbicara_komunikasi" json:"berbicara_komunikasi" parquet:"name=berbicara_komunikasi, type=UTF8"`
	MengurusDiri              string    `gorm:"column:mengurus_diri" json:"mengurus_diri" parquet:"name=mengurus_diri, type=UTF8"`
	MengingatBerkonsentrasi   string    `gorm:"column:mengingat_berkonsentrasi" json:"mengingat_berkonsentrasi" parquet:"name=mengingat_berkonsentrasi, type=UTF8"`
	KesedihanDepresi          string    `gorm:"column:kesedihan_depresi" json:"kesedihan_depresi" parquet:"name=kesedihan_depresi, type=UTF8"`
	PenyakitKronis            string    `gorm:"column:penyakit_kronis" json:"penyakit_kronis" parquet:"name=penyakit_kronis, type=UTF8"`

	// Variabel Sakti untuk Data Tak Terstruktur (Jangan Sampai Kelewat!)
	AdditionalInfo            datatypes.JSON `gorm:"column:additional_info" json:"additional_info"`

	// Metadata Versioning & Data Mesh Lifecycle (Identik dengan Pendidikan)
	Version       int       `gorm:"column:version" json:"version"`
	SourceID      string    `gorm:"column:source_id" json:"source_id" parquet:"name=source_id, type=UTF8"`
	IsWaliData    bool      `gorm:"column:is_wali_data" json:"is_wali_data" parquet:"name=is_wali_data, type=BOOLEAN"`
	TrustScore    float64   `gorm:"column:trust_score" json:"trust_score" parquet:"name=trust_score, type=DOUBLE"`
	ReferenceDate time.Time `gorm:"column:reference_date" json:"reference_date" parquet:"name=reference_date, type=INT64, convertedtype=TIMESTAMP_MILLIS"`

	// Tambahan untuk 13 Endpoints (Audit Trail)
	IsDeleted     bool      `gorm:"column:is_deleted;default:false" json:"is_deleted"`
	AuditStatus   string    `gorm:"column:audit_status;default:'PENDING'" json:"audit_status"`
	UpdatedAt     time.Time `gorm:"autoUpdateTime;column:updated_at" json:"updated_at"`
}