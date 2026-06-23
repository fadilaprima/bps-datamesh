package storage

import (
	"hunian/models"

	"gorm.io/gorm"
)

type HunianStorage struct {
	DB *gorm.DB
}

func (s *HunianStorage) GetLatestByNoKK(noKK string) (*models.RekamHunian, error) {
	var rh models.RekamHunian
	err := s.DB.Where("nomor_kartu_keluarga = ? AND is_deleted = ?", noKK, false).Order("version desc").First(&rh).Error
	return &rh, err
}

func (s *HunianStorage) Create(rh *models.RekamHunian) error {
	return s.DB.Create(rh).Error
}

func (s *HunianStorage) CountByNoKK(noKK string) (int64, error) {
	var count int64
	err := s.DB.Model(&models.RekamHunian{}).Where("nomor_kartu_keluarga = ?", noKK).Count(&count).Error
	return count, err
}

func (s *HunianStorage) GetBySubmission(subID string) ([]models.RekamHunian, error) {
	var results []models.RekamHunian
	err := s.DB.Where("source_id = ? AND is_deleted = ?", subID, false).Find(&results).Error
	return results, err
}

func (s *HunianStorage) GetFetchWithFields(fields []string) ([]models.RekamHunian, error) {
	var results []models.RekamHunian
	subQuery := s.DB.Model(&models.RekamHunian{}).Select("MAX(id)").Group("nomor_kartu_keluarga")
	query := s.DB.Where("id IN (?) AND is_deleted = ?", subQuery, false)

	if len(fields) > 0 && fields[0] != "" {
		query = query.Select(fields)
	}
	err := query.Find(&results).Error
	return results, err
}

func (s *HunianStorage) GetDetailWithFields(noKK string, fields []string) (*models.RekamHunian, error) {
	var result models.RekamHunian
	query := s.DB.Model(&models.RekamHunian{}).Where("nomor_kartu_keluarga = ?", noKK)

	if len(fields) > 0 && fields[0] != "" {
		query = query.Select(fields)
	}
	err := query.Order("version desc").First(&result).Error
	return &result, err
}

func (s *HunianStorage) SoftDelete(id string) error {
	return s.DB.Model(&models.RekamHunian{}).Where("id = ?", id).Update("is_deleted", true).Error
}

// GetAuditSamples mengambil sampel data versi tertinggi yang berstatus PENDING
func (s *HunianStorage) GetAuditSamples(limit int) ([]models.RekamHunian, error) {
    var results []models.RekamHunian
    
    // Query dengan penambahan RANDOM() dan limit
    query := `
        SELECT k.* FROM rekam_hunians k
        INNER JOIN (
            SELECT nomor_kartu_keluarga, MAX(version) as max_ver
            FROM rekam_hunians
            GROUP BY nomor_kartu_keluarga
        ) grouped_k 
        ON k.nomor_kartu_keluarga = grouped_k.nomor_kartu_keluarga 
        AND k.version = grouped_k.max_ver
        WHERE k.audit_status = 'PENDING'
        ORDER BY RANDOM()
        LIMIT ?
    `
    
    // Oper limit ke Raw query
    err := s.DB.Raw(query, limit).Scan(&results).Error
    return results, err
}

func (s *HunianStorage) UpdateBulkAuditDecision(nokkList []string, verdictText string, bonus float64) error {
	return s.DB.Model(&models.RekamHunian{}).
		Where("nomor_kartu_keluarga IN ? AND audit_status = ?", nokkList, "PENDING").
		Updates(map[string]interface{}{
			"audit_status": verdictText,
			"trust_score":  gorm.Expr("trust_score + ?", bonus),
		}).Error
}
