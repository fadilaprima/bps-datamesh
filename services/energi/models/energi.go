package models

import (
	"time"

	"gorm.io/datatypes"
)

type RekamEnergi struct {
	ID uint `gorm:"primaryKey;autoIncrement" json:"id"`
	// Menggunakan NoKK sebagai kunci logis keluarga (FK Logis)
	NoKK string `gorm:"index;column:nomor_kartu_keluarga" json:"nomor_kartu_keluarga" parquet:"name=nomor_kartu_keluarga, type=UTF8"`

	// Variabel Inti Energi (Sesuai list Dila)
	IDPelangganPLN string         `gorm:"column:id_pelanggan_pln" json:"id_pelanggan_pln" parquet:"name=id_pelanggan_pln, type=UTF8"`
	DayaTerpasang  string         `gorm:"column:daya_terpasang" json:"daya_terpasang" parquet:"name=daya_terpasang, type=UTF8"`
	AdditionalInfo datatypes.JSON `gorm:"column:additional_info" json:"additional_info"`

	// Metadata Versioning & Data Mesh Lifecycle (Identik)
	Version       int       `gorm:"column:version" json:"version"`
	SourceID      string    `gorm:"column:source_id" json:"source_id" parquet:"name=source_id, type=UTF8"`
	IsWaliData    bool      `gorm:"column:is_wali_data" json:"is_wali_data" parquet:"name=is_wali_data, type=BOOLEAN"`
	TrustScore    float64   `gorm:"column:trust_score" json:"trust_score" parquet:"name=trust_score, type=DOUBLE"`
	ReferenceDate time.Time `gorm:"column:reference_date" json:"reference_date" parquet:"name=reference_date, type=INT64, convertedtype=TIMESTAMP_MILLIS"`

	// Lifecycle Endpoints (Identik)
	IsDeleted   bool      `gorm:"column:is_deleted;default:false" json:"is_deleted"`
	AuditStatus string    `gorm:"column:audit_status;default:'PENDING'" json:"audit_status"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime;column:updated_at" json:"updated_at"`
}
