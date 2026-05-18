package models

import (
	"time"
	"gorm.io/datatypes"
)

type RekamKesejahteraan struct {
	ID                              uint           `gorm:"primaryKey;autoIncrement" json:"id"`
	// Kunci Logis (No. KK)
	NoKK                            string         `gorm:"index;column:nomor_kartu_keluarga" json:"nomor_kartu_keluarga" parquet:"name=nomor_kartu_keluarga, type=UTF8"`
	
	// Variabel Observasi (INT untuk skor/jumlah, VARCHAR untuk kode)
	DesilNasional                   int            `gorm:"column:desil_nasional" json:"desil_nasional" parquet:"name=desil_nasional, type=INT32"`
	BahanBakarMemasak               string         `gorm:"column:bahan_bakar_utama_memasak" json:"bahan_bakar_utama_memasak" parquet:"name=bahan_bakar_utama_memasak, type=UTF8"`
	KepemilikanAset                 int            `gorm:"column:kepemilikan_aset" json:"kepemilikan_aset" parquet:"name=kepemilikan_aset, type=INT32"`
	AsetGas                         int            `gorm:"column:aset_bergerak_tabung_gas" json:"aset_bergerak_tabung_gas" parquet:"name=aset_bergerak_tabung_gas, type=INT32"`
	AsetKulkas                      int            `gorm:"column:aset_bergerak_lemari_es" json:"aset_bergerak_lemari_es" parquet:"name=aset_bergerak_lemari_es, type=INT32"`
	AsetAC                          int            `gorm:"column:aset_bergerak_ac" json:"aset_bergerak_ac" parquet:"name=aset_bergerak_ac, type=INT32"`
	AsetPemanasAir                  int            `gorm:"column:aset_bergerak_pemanas_air" json:"aset_bergerak_pemanas_air" parquet:"name=aset_bergerak_pemanas_air, type=INT32"`
	AsetTelepon                     int            `gorm:"column:aset_bergerak_telepon_rumah" json:"aset_bergerak_telepon_rumah" parquet:"name=aset_bergerak_telepon_rumah, type=INT32"`
	AsetTV                          int            `gorm:"column:aset_bergerak_tv_datar" json:"aset_bergerak_tv_datar" parquet:"name=aset_bergerak_tv_datar, type=INT32"`
	AsetEmas                        int            `gorm:"column:aset_bergerak_emas_perhiasan" json:"aset_bergerak_emas_perhiasan" parquet:"name=aset_bergerak_emas_perhiasan, type=INT32"`
	AsetLaptop                      int            `gorm:"column:aset_bergerak_komputer_laptop_tablet" json:"aset_bergerak_komputer_laptop_tablet" parquet:"name=aset_bergerak_komputer_laptop_tablet, type=INT32"`
	AsetMotor                       int            `gorm:"column:aset_bergerak_sepeda_motor" json:"aset_bergerak_sepeda_motor" parquet:"name=aset_bergerak_sepeda_motor, type=INT32"`
	AsetSepeda                      int            `gorm:"column:aset_bergerak_sepeda" json:"aset_bergerak_sepeda" parquet:"name=aset_bergerak_sepeda, type=INT32"`
	AsetMobil                       int            `gorm:"column:aset_bergerak_mobil" json:"aset_bergerak_mobil" parquet:"name=aset_bergerak_mobil, type=INT32"`
	AsetPerahu                      int            `gorm:"column:aset_bergerak_perahu" json:"aset_bergerak_perahu" parquet:"name=aset_bergerak_perahu, type=INT32"`
	AsetPerahuMotor                 int            `gorm:"column:aset_bergerak_kapal_perahu_motor" json:"aset_bergerak_kapal_perahu_motor" parquet:"name=aset_bergerak_kapal_perahu_motor, type=INT32"`
	AsetSmartphone                  int            `gorm:"column:aset_bergerak_smartphone" json:"aset_bergerak_smartphone" parquet:"name=aset_bergerak_smartphone, type=INT32"`
	AsetLahanLain                   int            `gorm:"column:aset_tidak_bergerak_lahan_lainnya" json:"aset_tidak_bergerak_lahan_lainnya" parquet:"name=aset_tidak_bergerak_lahan_lainnya, type=INT32"`
	AsetRumahLain                   int            `gorm:"column:aset_tidak_bergerak_rumah_lainnya" json:"aset_tidak_bergerak_rumah_lainnya" parquet:"name=aset_tidak_bergerak_rumah_lainnya, type=INT32"`
	TernakSapi                      int            `gorm:"column:jumlah_ternak_sapi" json:"jumlah_ternak_sapi" parquet:"name=jumlah_ternak_sapi, type=INT32"`
	TernakKerbau                    int            `gorm:"column:jumlah_ternak_kerbau" json:"jumlah_ternak_kerbau" parquet:"name=jumlah_ternak_kerbau, type=INT32"`
	TernakKuda                      int            `gorm:"column:jumlah_ternak_kuda" json:"jumlah_ternak_kuda" parquet:"name=jumlah_ternak_kuda, type=INT32"`
	TernakBabi                      int            `gorm:"column:jumlah_ternak_babi" json:"jumlah_ternak_babi" parquet:"name=jumlah_ternak_babi, type=INT32"`
	TernakKambing                   int            `gorm:"column:jumlah_ternak_kambing_domba" json:"jumlah_ternak_kambing_domba" parquet:"name=jumlah_ternak_kambing_domba, type=INT32"`

	// Metadata & Lifecycle
	AdditionalInfo                  datatypes.JSON `gorm:"column:additional_info" json:"additional_info"`
	Version                         int            `gorm:"column:version" json:"version"`
	SourceID                        string         `gorm:"column:source_id" json:"source_id" parquet:"name=source_id, type=UTF8"`
	IsWaliData                      bool           `gorm:"column:is_wali_data" json:"is_wali_data" parquet:"name=is_wali_data, type=BOOLEAN"`
	TrustScore                      float64        `gorm:"column:trust_score" json:"trust_score" parquet:"name=trust_score, type=DOUBLE"`
	ReferenceDate                   time.Time      `gorm:"column:reference_date" json:"reference_date" parquet:"name=reference_date, type=INT64, convertedtype=TIMESTAMP_MILLIS"`
	IsDeleted                       bool           `gorm:"column:is_deleted;default:false" json:"is_deleted"`
	AuditStatus                     string         `gorm:"column:audit_status;default:'PENDING'" json:"audit_status"`
	UpdatedAt                       time.Time      `gorm:"autoUpdateTime;column:updated_at" json:"updated_at"`
	SchemaVersion                   string        `json:"schema_version"`
}