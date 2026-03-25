package storage

import (
	"kependudukan/models"
	"gorm.io/gorm"
)

type PendudukStorage struct {
	DB *gorm.DB
}

// --- 1. CORE OPERATIONS (Versioning) ---
func (s *PendudukStorage) GetLatestByNIK(nik string) (*models.Penduduk, error) {
	var p models.Penduduk
	err := s.DB.Where("nomor_induk_kependudukan = ? AND is_deleted = ?", nik, false).Order("version desc").First(&p).Error
	return &p, err
}

func (s *PendudukStorage) Create(p *models.Penduduk) error {
	return s.DB.Create(p).Error
}

// --- 2. MONITORING & PROGRESS ---
func (s *PendudukStorage) GetSubmissionLog(id string) (map[string]interface{}, error) {
	var total int64
	s.DB.Model(&models.Penduduk{}).Where("source_id = ?", id).Count(&total)
	return map[string]interface{}{
		"submission_id": id,
		"records_found": total,
		"status":        "COMPLETED",
	}, nil
}

// --- 3. MAINTENANCE (Lifecycle) ---
func (s *PendudukStorage) SoftDelete(id string) error {
	return s.DB.Model(&models.Penduduk{}).Where("id = ?", id).Update("is_deleted", true).Error
}

func (s *PendudukStorage) UpdateManual(id string, data map[string]interface{}) error {
	return s.DB.Model(&models.Penduduk{}).Where("id = ?", id).Updates(data).Error
}

// --- 4. GOVERNANCE & AUDIT ---
func (s *PendudukStorage) GetSamples(limit int) ([]models.Penduduk, error) {
	var samples []models.Penduduk
	err := s.DB.Where("is_deleted = ?", false).Limit(limit).Order("RANDOM()").Find(&samples).Error
	return samples, err
}

func (s *PendudukStorage) UpdateAuditStatus(id string, status string) error {
	return s.DB.Model(&models.Penduduk{}).Where("id = ?", id).Update("audit_status", status).Error
}