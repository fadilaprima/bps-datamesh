package models

import "time"

type RiwayatPendidikan struct {
	ID          uint   `gorm:"primaryKey;autoIncrement" json:"id"`
	NIK         string `gorm:"index;column:nomor_induk_kependudukan" json:"nomor_induk_kependudukan" parquet:"name=nomor_induk_kependudukan, type=UTF8"`
	Partisipasi string `gorm:"column:partisipasi_sekolah" json:"partisipasi_sekolah" parquet:"name=partisipasi_sekolah, type=UTF8"`
	Jenjang     string `gorm:"column:jenjang_tertinggi_yang_diduduki" json:"jenjang_tertinggi_yang_diduduki" parquet:"name=jenjang_tertinggi_yang_diduduki, type=UTF8"`
	Kelas       string `gorm:"column:kelas_tertinggi_yang_diduduki" json:"kelas_tertinggi_yang_diduduki" parquet:"name=kelas_tertinggi_yang_diduduki, type=UTF8"`
	Ijazah      string `gorm:"column:ijazah_tertinggi_yang_dimiliki" json:"ijazah_tertinggi_yang_dimiliki" parquet:"name=ijazah_tertinggi_yang_dimiliki, type=UTF8"`

	// Metadata Versioning & Data Mesh Lifecycle
	Version       int       `gorm:"column:version" json:"version"`
	SourceID      string    `gorm:"column:source_id" json:"source_id" parquet:"name=source_id, type=UTF8"`
	IsWaliData    bool      `gorm:"column:is_wali_data" json:"is_wali_data" parquet:"name=is_wali_data, type=BOOLEAN"`
	TrustScore    float64   `gorm:"column:trust_score" json:"trust_score" parquet:"name=trust_score, type=DOUBLE"`
	ReferenceDate time.Time `gorm:"column:reference_date" json:"reference_date" parquet:"name=reference_date, type=INT64, convertedtype=TIMESTAMP_MILLIS"`
	
	// Tambahan untuk 13 Endpoints
	IsDeleted     bool      `gorm:"column:is_deleted;default:false" json:"is_deleted"`
	AuditStatus   string    `gorm:"column:audit_status;default:'PENDING'" json:"audit_status"`
	UpdatedAt     time.Time `gorm:"autoUpdateTime;column:updated_at" json:"updated_at"`

}