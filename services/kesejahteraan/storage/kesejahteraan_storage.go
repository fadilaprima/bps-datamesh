package storage

import (
	"kesejahteraan/models"

	"gorm.io/gorm"
)

type KesejahteraanStorage struct {
	DB *gorm.DB
}

// GetLatestByNoKK: Mengambil record terbaru berdasarkan NoKK (Keluarga)
func (s *KesejahteraanStorage) GetLatestByNoKK(noKK string) (*models.RekamKesejahteraan, error) {
	var rk models.RekamKesejahteraan
	// Query mencari berdasarkan kolom nomor_kartu_keluarga
	err := s.DB.Where("nomor_kartu_keluarga = ? AND is_deleted = ?", noKK, false).Order("version desc").First(&rk).Error
	return &rk, err
}

// Create menyimpan record baru (SCD Type 2) ke tabel rekam_kesejahteraans
func (s *KesejahteraanStorage) Create(rk *models.RekamKesejahteraan) error {
	return s.DB.Create(rk).Error
}

func (s *KesejahteraanStorage) CountByNoKK(noKK string) (int64, error) {
	var count int64
	err := s.DB.Model(&models.RekamKesejahteraan{}).Where("nomor_kartu_keluarga = ?", noKK).Count(&count).Error
	return count, err
}

// GetBySubmission mengambil data berdasarkan Source ID
func (s *KesejahteraanStorage) GetBySubmission(subID string) ([]models.RekamKesejahteraan, error) {
	var results []models.RekamKesejahteraan
	err := s.DB.Where("source_id = ? AND is_deleted = ?", subID, false).Find(&results).Error
	return results, err
}

func (s *KesejahteraanStorage) GetFetchWithFields(fields []string) ([]models.RekamKesejahteraan, error) {
	var results []models.RekamKesejahteraan
	subQuery := s.DB.Model(&models.RekamKesejahteraan{}).Select("MAX(id)").Group("nomor_kartu_keluarga")
	query := s.DB.Where("id IN (?) AND is_deleted = ?", subQuery, false)

	if len(fields) > 0 && fields[0] != "" {
		query = query.Select(fields)
	}
	err := query.Find(&results).Error
	return results, err
}

func (s *KesejahteraanStorage) GetDetailWithFields(noKK string, fields []string) (*models.RekamKesejahteraan, error) {
	var result models.RekamKesejahteraan
	query := s.DB.Model(&models.RekamKesejahteraan{}).Where("nomor_kartu_keluarga = ?", noKK)

	if len(fields) > 0 && fields[0] != "" {
		query = query.Select(fields)
	}
	err := query.Order("version desc").First(&result).Error
	return &result, err
}

func (s *KesejahteraanStorage) SoftDelete(id string) error {
	return s.DB.Model(&models.RekamKesejahteraan{}).Where("id = ?", id).Update("is_deleted", true).Error
}

// UpdateAuditStatus menyimpan keputusan audit
func (s *KesejahteraanStorage) UpdateAuditStatus(id string, status string) error {
	return s.DB.Model(&models.RekamKesejahteraan{}).Where("id = ?", id).Update("audit_status", status).Error
}

// GetSample mengambil data acak untuk keperluan audit manual
func (s *KesejahteraanStorage) GetSample(limit int) ([]models.RekamKesejahteraan, error) {
	var samples []models.RekamKesejahteraan
	err := s.DB.Where("is_deleted = ?", false).Limit(limit).Order("RANDOM()").Find(&samples).Error
	return samples, err
}

func (s *KesejahteraanStorage) GetAuditSamples() ([]models.RekamKesejahteraan, error) {
	var results []models.RekamKesejahteraan
	query := `
		SELECT k.* FROM rekam_kesejahteraans k
		INNER JOIN (
			SELECT nomor_kartu_keluarga, MAX(version) as max_ver
			FROM rekam_kesejahteraans
			GROUP BY nomor_kartu_keluarga
		) grouped_k 
		ON k.nomor_kartu_keluarga = grouped_k.nomor_kartu_keluarga 
		AND k.version = grouped_k.max_ver
		WHERE k.audit_status = 'PENDING'
	`
	err := s.DB.Raw(query).Scan(&results).Error
	return results, err
}

func (s *KesejahteraanStorage) UpdateBulkAuditDecision(nokkList []string, verdictText string, bonus float64) error {
	return s.DB.Model(&models.RekamKesejahteraan{}).
		Where("nomor_kartu_keluarga IN ? AND audit_status = ?", nokkList, "PENDING").
		Updates(map[string]interface{}{
			"audit_status": verdictText,
			"trust_score":  gorm.Expr("trust_score + ?", bonus),
		}).Error
}
