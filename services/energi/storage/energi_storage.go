package storage

import (
	"energi/models"

	"gorm.io/gorm"
)

type EnergiStorage struct {
	DB *gorm.DB
}

// GetLatestByNoKK mengambil record terbaru berdasarkan NoKK (Identik dengan GetLatestByNIK)
func (s *EnergiStorage) GetLatestByNoKK(noKK string) (*models.RekamEnergi, error) {
	var re models.RekamEnergi
	// Mengambil versi terbaru untuk NoKK tersebut
	err := s.DB.Where("nomor_kartu_keluarga = ?", noKK).Order("version desc").First(&re).Error
	return &re, err
}

// Create menyimpan record baru ke dalam tabel rekam_energis
func (s *EnergiStorage) Create(re *models.RekamEnergi) error {
	return s.DB.Create(re).Error
}

// GetBySubmission mengambil data berdasarkan ID pengiriman (Identik dengan Pendidikan)
func (s *EnergiStorage) GetBySubmission(subID string) ([]models.RekamEnergi, error) {
	var results []models.RekamEnergi
	err := s.DB.Where("source_id = ?", subID).Find(&results).Error
	return results, err
}

// UpdateAuditStatus menyimpan keputusan Approved/Rejected (Identik dengan Pendidikan)
func (s *EnergiStorage) UpdateAuditStatus(id string, status string) error {
	return s.DB.Model(&models.RekamEnergi{}).Where("id = ?", id).Update("audit_status", status).Error
}

// GetSample mengambil data acak untuk keperluan audit (Identik dengan Pendidikan)
func (s *EnergiStorage) GetSample(limit int) ([]models.RekamEnergi, error) {
	var samples []models.RekamEnergi
	err := s.DB.Limit(limit).Order("RANDOM()").Find(&samples).Error
	return samples, err
}