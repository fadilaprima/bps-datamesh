package storage

import (
	"wilayah/models"
	"gorm.io/gorm"
)

type WilayahStorage struct {
	DB *gorm.DB
}

// ============================================================
// 1. CORE OPERATIONS (SCD TYPE 2 & VERSIONING)
// ============================================================

// GetLatestByKode mengambil record terbaru berdasarkan Kode Desa/Kelurahan (Natural Key)
func (s *WilayahStorage) GetLatestByKode(kode string) (*models.MasterWilayah, error) {
	var w models.MasterWilayah
	// Mengambil versi terbaru yang belum dihapus (Soft Delete aware)
	err := s.DB.Where("kode_kelurahan_desa = ? AND is_deleted = ?", kode, false).
		Order("version desc").
		First(&w).Error
	return &w, err
}

// Create menyimpan record baru ke dalam tabel master_wilayah (Snapshot Baru)
func (s *WilayahStorage) Create(w *models.MasterWilayah) error {
	return s.DB.Create(w).Error
}

// ============================================================
// 2. MONITORING & PROGRESS (SOURCE TRACKING)
// ============================================================

// GetBySubmission mengambil data berdasarkan ID pengirim (Identik dengan Pendidikan/Penduduk)
func (s *WilayahStorage) GetBySubmission(sourceID string) ([]models.MasterWilayah, error) {
	var results []models.MasterWilayah
	err := s.DB.Where("source_id = ? AND is_deleted = ?", sourceID, false).
		Find(&results).Error
	return results, err
}

// ============================================================
// 3. MAINTENANCE (LIFECYCLE MANAGEMENT)
// ============================================================

// SoftDelete menandai data wilayah sebagai terhapus tanpa menghilangkan dari database
func (s *WilayahStorage) SoftDelete(id string) error {
	return s.DB.Model(&models.MasterWilayah{}).
		Where("id = ?", id).
		Update("is_deleted", true).Error
}

// UpdateManual melakukan pembaruan parsial jika ada koreksi manual pada atribut wilayah
func (s *WilayahStorage) UpdateManual(id string, data map[string]interface{}) error {
	return s.DB.Model(&models.MasterWilayah{}).
		Where("id = ?", id).
		Updates(data).Error
}

// ============================================================
// 4. GOVERNANCE & AUDIT (QUALITY CONTROL)
// ============================================================

// UpdateAuditStatus menyimpan keputusan Approved/Rejected (Identik dengan Pendidikan/Penduduk)
func (s *WilayahStorage) UpdateAuditStatus(id string, status string) error {
	return s.DB.Model(&models.MasterWilayah{}).
		Where("id = ?", id).
		Update("audit_status", status).Error
}

// GetSample mengambil data acak wilayah untuk keperluan audit lapangan (Identik dengan Pendidikan/Penduduk)
func (s *WilayahStorage) GetSample(limit int) ([]models.MasterWilayah, error) {
	var samples []models.MasterWilayah
	err := s.DB.Where("is_deleted = ?", false).
		Limit(limit).
		Order("RANDOM()").
		Find(&samples).Error
	return samples, err
}