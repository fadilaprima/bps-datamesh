package storage

import (
	"strings"

	"kesejahteraan/models"
	"gorm.io/gorm"
)

type KesejahteraanStorage struct {
	DB *gorm.DB
}

// GetLatestByNoKK mengambil record terbaru berdasarkan NoKK (Keluarga)
func (s *KesejahteraanStorage) GetLatestByNoKK(noKK string) (*models.RekamKesejahteraan, error) {
	var rk models.RekamKesejahteraan
	err := s.DB.Where("nomor_kartu_keluarga = ? AND is_deleted = ?", noKK, false).
		Order("version desc").
		First(&rk).Error
	return &rk, err
}

// Create menyimpan record baru (SCD Type 2)
func (s *KesejahteraanStorage) Create(rk *models.RekamKesejahteraan) error {
	return s.DB.Create(rk).Error
}

func (s *KesejahteraanStorage) CountByNoKK(noKK string) (int64, error) {
	var count int64
	err := s.DB.Model(&models.RekamKesejahteraan{}).Where("nomor_kartu_keluarga = ?", noKK).Count(&count).Error
	return count, err
}

func (s *KesejahteraanStorage) GetBySubmission(subID string) ([]models.RekamKesejahteraan, error) {
	var results []models.RekamKesejahteraan
	err := s.DB.Where("source_id = ? AND is_deleted = ?", subID, false).Find(&results).Error
	return results, err
}

func (s *KesejahteraanStorage) GetFetchWithFields(fields []string) ([]models.RekamKesejahteraan, error) {
	var results []models.RekamKesejahteraan
	
	subQuery := s.DB.Model(&models.RekamKesejahteraan{}).
		Select("MAX(id)").
		Where("is_deleted = ?", false).
		Group("nomor_kartu_keluarga")
	
	query := s.DB.Where("id IN (?)", subQuery)

	if len(fields) > 0 && fields[0] != "" {
		query = query.Select(fields)
	}
	err := query.Find(&results).Error
	return results, err
}

func (s *KesejahteraanStorage) GetDetailWithFields(noKK string, fields []string) (*models.RekamKesejahteraan, error) {
	var result models.RekamKesejahteraan
	query := s.DB.Model(&models.RekamKesejahteraan{}).
		Where("nomor_kartu_keluarga = ? AND is_deleted = ?", noKK, false)

	if len(fields) > 0 && fields[0] != "" {
		query = query.Select(fields)
	}
	err := query.Order("version desc").First(&result).Error
	return &result, err
}

func (s *KesejahteraanStorage) SoftDelete(identifier string) error {
	cleanID := strings.TrimSpace(identifier)

	if len(cleanID) == 16 {
		return s.DB.Model(&models.RekamKesejahteraan{}).
			Where("nomor_kartu_keluarga = ?", cleanID).
			Update("is_deleted", true).Error
	}
	
	// Jika bukan 16 digit, eksekusi hapus berdasarkan ID absolut
	return s.DB.Model(&models.RekamKesejahteraan{}).
		Where("id = ?", cleanID).
		Update("is_deleted", true).Error
}

func (s *KesejahteraanStorage) UpdateAuditStatus(id string, status string) error {
	return s.DB.Model(&models.RekamKesejahteraan{}).Where("id = ?", id).Update("audit_status", status).Error
}

func (s *KesejahteraanStorage) GetSample(limit int) ([]models.RekamKesejahteraan, error) {
	var samples []models.RekamKesejahteraan
	err := s.DB.Where("is_deleted = ?", false).Limit(limit).Order("RANDOM()").Find(&samples).Error
	return samples, err
}

// GetAuditSamples mengambil sampel data versi tertinggi yang berstatus PENDING
func (s *KesejahteraanStorage) GetAuditSamples(limit int) ([]models.RekamKesejahteraan, error) {
	var results []models.RekamKesejahteraan
	
	query := `
		SELECT k.* FROM rekam_kesejahteraans k
		INNER JOIN (
			SELECT nomor_kartu_keluarga, MAX(version) as max_ver
			FROM rekam_kesejahteraans
			WHERE is_deleted = false
			GROUP BY nomor_kartu_keluarga
		) grouped_k 
		ON k.nomor_kartu_keluarga = grouped_k.nomor_kartu_keluarga 
		AND k.version = grouped_k.max_ver
		WHERE k.audit_status = 'PENDING'
		ORDER BY RANDOM()
		LIMIT ?
	`
	
	err := s.DB.Raw(query, limit).Scan(&results).Error
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