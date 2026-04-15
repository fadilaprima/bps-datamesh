package models

import (
	"time"
	"gorm.io/datatypes"
)

// MasterWilayah mewakili entitas data wilayah di level Desa/Kelurahan
// Struktur ini mengikuti standar variabel Metadata DTSEN (Data Terpadu Sosial Ekonomi Nasional)
type MasterWilayah struct {
	ID            uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	
	// Variabel Hirarki Wilayah sesuai spesifikasi Metadata PDF
	KodeProv      string    `gorm:"column:kode_provinsi" json:"kode_provinsi"`           // Standar 2 digit
	Provinsi      string    `gorm:"column:provinsi" json:"provinsi"`
	KodeKab       string    `gorm:"column:kode_kabupaten_kota" json:"kode_kabupaten_kota"` // Standar 4 digit
	Kabupaten     string    `gorm:"column:kabupaten_kota" json:"kabupaten_kota"`
	KodeKec       string    `gorm:"column:kode_kecamatan" json:"kode_kecamatan"`          // Standar 7 digit
	Kecamatan     string    `gorm:"column:kecamatan" json:"kecamatan"`
	KodeDesa      string    `gorm:"index;column:kode_kelurahan_desa" json:"kode_kelurahan_desa"` // Standar 10 digit (Index Utama)
	Desa          string    `gorm:"column:kelurahan_desa" json:"kelurahan_desa"`
	AdditionalInfo datatypes.JSON `gorm:"column:additional_info" json:"additional_info"`

	// Metadata untuk Implementasi Data Mesh & Governance
	Version       int       `gorm:"column:version" json:"version"`                       
	SourceID      string    `gorm:"column:source_id" json:"source_id"`                   // Identitas Organisasi Pengirim (BPS/Kemendagri)
	IsWaliData    bool      `gorm:"column:is_wali_data" json:"is_wali_data"`             // Flag Otoritas (True jika dari BPS)
	TrustScore    float64   `gorm:"column:trust_score" json:"trust_score"`               // Skor Kepercayaan Sumber Data
	ReferenceDate time.Time `gorm:"column:reference_date" json:"reference_date"`         // Tanggal referensi data
	UpdatedAt     time.Time `gorm:"autoUpdateTime;column:updated_at" json:"updated_at"` // Timestamp pembaruan record
	IsDeleted     bool      `gorm:"default:false" json:"is_deleted"`
	AuditStatus   string    `gorm:"default:'PENDING'" json:"audit_status"` // PENDING, APPROVED, REJECTED
	SchemaVersion string    `json:"schema_version"`
}

// TableName menentukan nama tabel spesifik di database agar tidak plural (default GORM)
func (MasterWilayah) TableName() string { return "master_wilayah" }