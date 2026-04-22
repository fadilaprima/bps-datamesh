package models

import (
	"time"
	"gorm.io/datatypes"
)

type RekamHunian struct {
	ID                        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	NoKK                      string    `gorm:"index;column:nomor_kartu_keluarga" json:"nomor_kartu_keluarga" parquet:"name=nomor_kartu_keluarga, type=UTF8"`
	
	// Variabel Observasi Hunian (Sesuai List Dila)
	StatusKepemilikan         string    `gorm:"column:status_kepemilikan_rumah" json:"status_kepemilikan_rumah" parquet:"name=status_kepemilikan_rumah, type=UTF8"`
	JenisLantai               string    `gorm:"column:jenis_lantai_terluas" json:"jenis_lantai_terluas" parquet:"name=jenis_lantai_terluas, type=UTF8"`
	LuasLantai                int       `gorm:"column:luas_lantai" json:"luas_lantai" parquet:"name=luas_lantai, type=INT32"`
	JenisDinding              string    `gorm:"column:jenis_dinding_terluas" json:"jenis_dinding_terluas" parquet:"name=jenis_dinding_terluas, type=UTF8"`
	JenisAtap                 string    `gorm:"column:jenis_atap_terluas" json:"jenis_atap_terluas" parquet:"name=jenis_atap_terluas, type=UTF8"`
	SumberAirMinum            string    `gorm:"column:sumber_air_minum_utama" json:"sumber_air_minum_utama" parquet:"name=sumber_air_minum_utama, type=UTF8"`
	SumberPenerangan          string    `gorm:"column:sumber_penerangan_utama" json:"sumber_penerangan_utama" parquet:"name=sumber_penerangan_utama, type=UTF8"`
	FasilitasBAB              string    `gorm:"column:fasilitas_bab" json:"fasilitas_bab" parquet:"name=fasilitas_bab, type=UTF8"`
	JenisKloset               string    `gorm:"column:jenis_kloset" json:"jenis_kloset" parquet:"name=jenis_kloset, type=UTF8"`
	PembuanganTinja           string    `gorm:"column:pembuangan_akhir_tinja" json:"pembuangan_akhir_tinja" parquet:"name=pembuangan_akhir_tinja, type=UTF8"`

	// Variabel Sakti & Metadata (Wajib Identik)
	AdditionalInfo datatypes.JSON `gorm:"column:additional_info" json:"additional_info"`
	Version        int            `gorm:"column:version" json:"version"`
	SourceID       string         `gorm:"column:source_id" json:"source_id" parquet:"name=source_id, type=UTF8"`
	IsWaliData     bool           `gorm:"column:is_wali_data" json:"is_wali_data" parquet:"name=is_wali_data, type=BOOLEAN"`
	TrustScore     float64        `gorm:"column:trust_score" json:"trust_score" parquet:"name=trust_score, type=DOUBLE"`
	ReferenceDate  time.Time      `gorm:"column:reference_date" json:"reference_date" parquet:"name=reference_date, type=INT64, convertedtype=TIMESTAMP_MILLIS"`
	IsDeleted      bool           `gorm:"column:is_deleted;default:false" json:"is_deleted"`
	AuditStatus    string         `gorm:"column:audit_status;default:'PENDING'" json:"audit_status"`
	UpdatedAt      time.Time      `gorm:"autoUpdateTime;column:updated_at" json:"updated_at"`
}