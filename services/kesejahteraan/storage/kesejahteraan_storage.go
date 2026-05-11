package storage

import (
	"kesejahteraan/models"

	"gorm.io/gorm"
)

type KesejahteraanStorage struct {
	DB *gorm.DB
}

// GetLatestByNoKK: Mengambil record terbaru berdasarkan NoKK (Keluarga)
// Fungsi ini menggantikan GetLatestByNIK karena domain Kemensos berbasis KK
func (s *KesejahteraanStorage) GetLatestByNoKK(noKK string) (*models.RekamKesejahteraan, error) {
	var rk models.RekamKesejahteraan
	// Query mencari berdasarkan kolom nomor_kartu_keluarga
	err := s.DB.Where("nomor_kartu_keluarga = ?", noKK).Order("version desc").First(&rk).Error
	return &rk, err
}

// Create menyimpan record baru (SCD Type 2) ke tabel rekam_kesejahteraans
func (s *KesejahteraanStorage) Create(rk *models.RekamKesejahteraan) error {
	return s.DB.Create(rk).Error
}

// GetBySubmission mengambil data berdasarkan Source ID (Misal: KEMENSOS, DINSOS)
func (s *KesejahteraanStorage) GetBySubmission(subID string) ([]models.RekamKesejahteraan, error) {
	var results []models.RekamKesejahteraan
	err := s.DB.Where("source_id = ?", subID).Find(&results).Error
	return results, err
}

// UpdateAuditStatus menyimpan keputusan VALID/INVALID hasil audit
func (s *KesejahteraanStorage) UpdateAuditStatus(id string, status string) error {
	return s.DB.Model(&models.RekamKesejahteraan{}).Where("id = ?", id).Update("audit_status", status).Error
}

// GetSample mengambil data acak untuk keperluan audit manual
func (s *KesejahteraanStorage) GetSample(limit int) ([]models.RekamKesejahteraan, error) {
	var samples []models.RekamKesejahteraan
	err := s.DB.Limit(limit).Order("RANDOM()").Find(&samples).Error
	return samples, err
}
