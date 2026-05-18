package storage

import (
	"hunian/models"

	"gorm.io/gorm"
)

type HunianStorage struct {
	DB *gorm.DB
}

// GetLatestByNoKK mengambil record terbaru berdasarkan NoKK 
func (s *HunianStorage) GetLatestByNoKK(noKK string) (*models.RekamHunian, error) {
	var rh models.RekamHunian
	// Mengambil versi terbaru untuk NoKK tersebut
	err := s.DB.Where("nomor_kartu_keluarga = ?", noKK).Order("version desc").First(&rh).Error
	return &rh, err
}

// Create menyimpan record baru ke dalam tabel rekam_hunian
func (s *HunianStorage) Create(rh *models.RekamHunian) error {
	return s.DB.Create(rh).Error
}

// GetBySubmission mengambil data berdasarkan ID pengiriman 
func (s *HunianStorage) GetBySubmission(subID string) ([]models.RekamHunian, error) {
	var results []models.RekamHunian
	err := s.DB.Where("source_id = ?", subID).Find(&results).Error
	return results, err
}

// UpdateAuditStatus menyimpan keputusan Approved/Rejected 
func (s *HunianStorage) UpdateAuditStatus(id string, status string) error {
	return s.DB.Model(&models.RekamHunian{}).Where("id = ?", id).Update("audit_status", status).Error
}

// GetSample mengambil data acak untuk keperluan audit 
func (s *HunianStorage) GetSample(limit int) ([]models.RekamHunian, error) {
	var samples []models.RekamHunian
	err := s.DB.Limit(limit).Order("RANDOM()").Find(&samples).Error
	return samples, err
}