package models

import "time"
import "gorm.io/datatypes"


type RiwayatPendidikan struct {
	ID          uint   `gorm:"primaryKey;autoIncrement" json:"id"`
	NIK         string `gorm:"index;column:nomor_induk_kependudukan" json:"nomor_induk_kependudukan" parquet:"name=nomor_induk_kependudukan, type=UTF8"`
	Partisipasi string `gorm:"column:partisipasi_sekolah" json:"partisipasi_sekolah" parquet:"name=partisipasi_sekolah, type=UTF8"`
	Jenjang     string `gorm:"column:jenjang_tertinggi_yang_diduduki" json:"jenjang_tertinggi_yang_diduduki" parquet:"name=jenjang_tertinggi_yang_diduduki, type=UTF8"`
	Kelas       string `gorm:"column:kelas_tertinggi_yang_diduduki" json:"kelas_tertinggi_yang_diduduki" parquet:"name=kelas_tertinggi_yang_diduduki, type=UTF8"`
	Ijazah      string `gorm:"column:ijazah_tertinggi_yang_dimiliki" json:"ijazah_tertinggi_yang_dimiliki" parquet:"name=ijazah_tertinggi_yang_dimiliki, type=UTF8"`

	AdditionalInfo datatypes.JSON `gorm:"column:additional_info" json:"additional_info"`
	// Metadata untuk Implementasi Data Mesh & Governance
	Version       int       `gorm:"column:version" json:"version"`                       
	SourceID      string    `gorm:"column:source_id" json:"source_id"`                   // Identitas Organisasi Pengirim
	IsWaliData    bool      `gorm:"column:is_wali_data" json:"is_wali_data"`             // Flag Otoritas 
	TrustScore    float64   `gorm:"column:trust_score" json:"trust_score"`               // Skor Kepercayaan Sumber Data
	ReferenceDate time.Time `gorm:"column:reference_date" json:"reference_date"`         // Tanggal referensi data
	UpdatedAt     time.Time `gorm:"autoUpdateTime;column:updated_at" json:"updated_at"` // Timestamp pembaruan record
	IsDeleted     bool      `gorm:"default:false" json:"is_deleted"`
	AuditStatus   string    `gorm:"default:'PENDING'" json:"audit_status"` // PENDING, VALID, INVALID
	SchemaVersion string    `json:"schema_version"`
	
}