package models

import (
	"time"
	"gorm.io/datatypes"
)

type RekamKetenagakerjaan struct {
	ID                                         uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	NIK                                        string    `gorm:"index;column:nomor_induk_kependudukan" json:"nomor_induk_kependudukan" parquet:"name=nomor_induk_kependudukan, type=UTF8"`
	
	// Variabel Inti Ketenagakerjaan (Sesuai List Dila)
	StatusBekerja                              string    `gorm:"column:status_bekerja" json:"status_bekerja" parquet:"name=status_bekerja, type=UTF8"`
	LapanganUsahaUtama                         string    `gorm:"column:lapangan_usaha_dari_pekerjaan_utama" json:"lapangan_usaha_dari_pekerjaan_utama" parquet:"name=lapangan_usaha_dari_pekerjaan_utama, type=UTF8"`
	StatusDalamPekerjaanUtama                  string    `gorm:"column:status_dalam_pekerjaan_utama" json:"status_dalam_pekerjaan_utama" parquet:"name=status_dalam_pekerjaan_utama, type=UTF8"`
	KepemilikanUsaha                           string    `gorm:"column:kepemilikan_usaha" json:"kepemilikan_usaha" parquet:"name=kepemilikan_usaha, type=UTF8"`
	JumlahUsaha                                int       `gorm:"column:jumlah_usaha" json:"jumlah_usaha" parquet:"name=jumlah_usaha, type=INT32"`
	LapanganUsahaPekerjaanUtama                string    `gorm:"column:lapangan_usaha_dari_usaha_utama" json:"lapangan_usaha_dari_usaha_utama" parquet:"name=lapangan_usaha_dari_usaha_utama, type=UTF8"`
	JumlahPekerjaDibayar                       int       `gorm:"column:jumlah_pekerja_yang_dibayar_dari_usaha_utama" json:"jumlah_pekerja_yang_dibayar_dari_usaha_utama" parquet:"name=jumlah_pekerja_yang_dibayar_dari_usaha_utama, type=INT32"`
	JumlahPekerjaTidakDibayar                  int       `gorm:"column:jumlah_pekerja_yang_tidak_dibayar_dari_usaha_utama" json:"jumlah_pekerja_yang_tidak_dibayar_dari_usaha_utama" parquet:"name=jumlah_pekerja_yang_tidak_dibayar_dari_usaha_utama, type=INT32"`
	OmzetUsahaUtama                            string    `gorm:"column:omzet_usaha_utama" json:"omzet_usaha_utama" parquet:"name=omzet_usaha_utama, type=UTF8"`

	// Metadata & Data Mesh Lifecycle
	AdditionalInfo datatypes.JSON `gorm:"column:additional_info" json:"additional_info"`
	Version        int            `gorm:"column:version" json:"version"`
	SourceID       string         `gorm:"column:source_id" json:"source_id" parquet:"name=source_id, type=UTF8"`
	IsWaliData     bool           `gorm:"column:is_wali_data" json:"is_wali_data" parquet:"name=is_wali_data, type=BOOLEAN"`
	TrustScore     float64        `gorm:"column:trust_score" json:"trust_score" parquet:"name=trust_score, type=DOUBLE"`
	ReferenceDate  time.Time      `gorm:"column:reference_date" json:"reference_date" parquet:"name=reference_date, type=INT64, convertedtype=TIMESTAMP_MILLIS"`
	IsDeleted      bool           `gorm:"column:is_deleted;default:false" json:"is_deleted"`
	AuditStatus    string         `gorm:"column:audit_status;default:'PENDING'" json:"audit_status"`
	UpdatedAt      time.Time      `gorm:"autoUpdateTime;column:updated_at" json:"updated_at"`
	SchemaVersion  string `json:"schema_version" gorm:"type:varchar(50)"`
}

