package storage

import (
	"wilayah/models"
	"gorm.io/gorm"
)


type WilayahStorage struct {
	DB *gorm.DB
}

// GetLatestByKode mengambil record terbaru berdasarkan Kode Desa/Kelurahan
func (s *WilayahStorage) GetLatestByKode(kode string) (*models.MasterWilayah, error) {
	var w models.MasterWilayah
	err := s.DB.Where("kode_kelurahan_desa = ?", kode).Order("version desc").First(&w).Error
	return &w, err
}

// Create menyimpan record baru ke dalam tabel master_wilayah
func (s *WilayahStorage) Create(w *models.MasterWilayah) error {
	return s.DB.Create(w).Error
}

// Get data spesifik
func (s *WilayahStorage) GetBySubmission(subID string) ([]models.MasterWilayah, error) {
	var results []models.MasterWilayah
	err := s.DB.Where("source_id = ?", subID).Find(&results).Error
	return results, err
}

//Update status audit
func (s *WilayahStorage) UpdateAuditStatus(id string, status string) error {
	return s.DB.Model(&models.MasterWilayah{}).Where("id = ?", id).Update("audit_status", status).Error
}

//Get sample data
func (s *WilayahStorage) GetSample(limit int) ([]models.MasterWilayah, error) {
	var samples []models.MasterWilayah
	err := s.DB.Limit(limit).Order("RANDOM()").Find(&samples).Error
	return samples, err
}