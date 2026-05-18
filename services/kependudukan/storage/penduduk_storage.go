package storage

import (
	"kependudukan/models"
	"gorm.io/gorm"
)

type PendudukStorage struct {
	DB *gorm.DB
}

// GetLatestByNIK mengambil record terbaru berdasarkan NIK 
func (s *PendudukStorage) GetLatestByNIK(nik string) (*models.Penduduk, error) {
	var p models.Penduduk
	// Mengambil versi terbaru yang belum dihapus 
	err := s.DB.Where("nomor_induk_kependudukan = ? AND is_deleted = ?", nik, false).
		Order("version desc").
		First(&p).Error
	return &p, err
}

// Create menyimpan record baru ke dalam tabel penduduk 
func (s *PendudukStorage) Create(p *models.Penduduk) error {
	return s.DB.Create(p).Error
}


// GetBySubmission mengambil data berdasarkan ID pengirim 
func (s *PendudukStorage) GetBySubmission(sourceID string) ([]models.Penduduk, error) {
	var results []models.Penduduk
	err := s.DB.Where("source_id = ? AND is_deleted = ?", sourceID, false).
		Find(&results).Error
	return results, err
}


// SoftDelete menandai data sebagai terhapus tanpa menghilangkan dari database
func (s *PendudukStorage) SoftDelete(id string) error {
	return s.DB.Model(&models.Penduduk{}).
		Where("id = ?", id).
		Update("is_deleted", true).Error
}

// UpdateManual melakukan pembaruan parsial jika ada koreksi manual
func (s *PendudukStorage) UpdateManual(id string, data map[string]interface{}) error {
	return s.DB.Model(&models.Penduduk{}).
		Where("id = ?", id).
		Updates(data).Error
}


// UpdateAuditStatus menyimpan keputusan audir
func (s *PendudukStorage) UpdateAuditStatus(id string, status string) error {
	return s.DB.Model(&models.Penduduk{}).
		Where("id = ?", id).
		Update("audit_status", status).Error
}

// GetSample mengambil data acak untuk keperluan audit lapangan 
func (s *PendudukStorage) GetSample(limit int) ([]models.Penduduk, error) {
	var samples []models.Penduduk
	err := s.DB.Where("is_deleted = ?", false).
		Limit(limit).
		Order("RANDOM()").
		Find(&samples).Error
	return samples, err
}