package storage

import (
	"ketenagakerjaan/models"
	"gorm.io/gorm"
)

type KetenagakerjaanStorage struct {
	DB *gorm.DB
}

func (s *KetenagakerjaanStorage) GetLatestByNIK(nik string) (*models.RekamKetenagakerjaan, error) {
	var rk models.RekamKetenagakerjaan
	err := s.DB.Where("nomor_induk_kependudukan = ?", nik).Order("version desc").First(&rk).Error
	return &rk, err
}

func (s *KetenagakerjaanStorage) Create(rk *models.RekamKetenagakerjaan) error {
	return s.DB.Create(rk).Error
}