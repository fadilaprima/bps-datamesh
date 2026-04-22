package storage

import (
	"kesehatan/models"
	"gorm.io/gorm"
)

type KesehatanStorage struct {
	DB *gorm.DB
}

// GetLatestByNIK mengambil record terbaru berdasarkan NIK (Identik dengan GetLatestByKode)
func (s *KesehatanStorage) GetLatestByNIK(nik string) (*models.RekamKesehatan, error) {
	var rp models.RekamKesehatan
	// Mengambil versi terbaru untuk NIK tersebut
	err := s.DB.Where("nomor_induk_kependudukan = ?", nik).Order("version desc").First(&rp).Error
	return &rp, err
}

// Create menyimpan record baru ke dalam tabel Rekam_kesehatan
func (s *KesehatanStorage) Create(rp *models.RekamKesehatan) error {
	return s.DB.Create(rp).Error
}

// GetBySubmission mengambil data berdasarkan ID pengiriman (Identik dengan Wilayah)
func (s *KesehatanStorage) GetBySubmission(subID string) ([]models.RekamKesehatan, error) {
	var results []models.RekamKesehatan
	err := s.DB.Where("source_id = ?", subID).Find(&results).Error
	return results, err
}

// UpdateAuditStatus menyimpan keputusan Approved/Rejected (Identik dengan Wilayah)
func (s *KesehatanStorage) UpdateAuditStatus(id string, status string) error {
	return s.DB.Model(&models.RekamKesehatan{}).Where("id = ?", id).Update("audit_status", status).Error
}

// GetSample mengambil data acak untuk keperluan audit (Identik dengan Wilayah)
func (s *KesehatanStorage) GetSample(limit int) ([]models.RekamKesehatan, error) {
	var samples []models.RekamKesehatan
	err := s.DB.Limit(limit).Order("RANDOM()").Find(&samples).Error
	return samples, err
}
