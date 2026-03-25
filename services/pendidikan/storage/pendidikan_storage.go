package storage

import (
	"pendidikan/models"
	"gorm.io/gorm"
)

type PendidikanStorage struct {
	DB *gorm.DB
}

// GetLatestByNIK mengambil record terbaru berdasarkan NIK (Identik dengan GetLatestByKode)
func (s *PendidikanStorage) GetLatestByNIK(nik string) (*models.RiwayatPendidikan, error) {
	var rp models.RiwayatPendidikan
	// Mengambil versi terbaru untuk NIK tersebut
	err := s.DB.Where("nomor_induk_kependudukan = ?", nik).Order("version desc").First(&rp).Error
	return &rp, err
}

// Create menyimpan record baru ke dalam tabel riwayat_pendidikan
func (s *PendidikanStorage) Create(rp *models.RiwayatPendidikan) error {
	return s.DB.Create(rp).Error
}

// GetBySubmission mengambil data berdasarkan ID pengiriman (Identik dengan Wilayah)
func (s *PendidikanStorage) GetBySubmission(subID string) ([]models.RiwayatPendidikan, error) {
	var results []models.RiwayatPendidikan
	err := s.DB.Where("source_id = ?", subID).Find(&results).Error
	return results, err
}

// UpdateAuditStatus menyimpan keputusan Approved/Rejected (Identik dengan Wilayah)
func (s *PendidikanStorage) UpdateAuditStatus(id string, status string) error {
	return s.DB.Model(&models.RiwayatPendidikan{}).Where("id = ?", id).Update("audit_status", status).Error
}

// GetSample mengambil data acak untuk keperluan audit (Identik dengan Wilayah)
func (s *PendidikanStorage) GetSample(limit int) ([]models.RiwayatPendidikan, error) {
	var samples []models.RiwayatPendidikan
	err := s.DB.Limit(limit).Order("RANDOM()").Find(&samples).Error
	return samples, err
}