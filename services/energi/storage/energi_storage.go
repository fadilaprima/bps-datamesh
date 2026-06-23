package storage

import (
	"energi/models"

	"gorm.io/gorm"
)

type EnergiStorage struct {
	DB *gorm.DB
}

func (s *EnergiStorage) GetLatestByNoKK(noKK string) (*models.RekamEnergi, error) {
	var re models.RekamEnergi
	err := s.DB.Where("nomor_kartu_keluarga = ? AND is_deleted = ?", noKK, false).Order("version desc").First(&re).Error
	return &re, err
}

func (s *EnergiStorage) Create(re *models.RekamEnergi) error {
	return s.DB.Create(re).Error
}

func (s *EnergiStorage) CountByNoKK(noKK string) (int64, error) {
	var count int64
	err := s.DB.Model(&models.RekamEnergi{}).Where("nomor_kartu_keluarga = ?", noKK).Count(&count).Error
	return count, err
}

func (s *EnergiStorage) GetBySubmission(subID string) ([]models.RekamEnergi, error) {
	var results []models.RekamEnergi
	err := s.DB.Where("source_id = ? AND is_deleted = ?", subID, false).Find(&results).Error
	return results, err
}

func (s *EnergiStorage) GetFetchWithFields(fields []string) ([]models.RekamEnergi, error) {
	var results []models.RekamEnergi
	subQuery := s.DB.Model(&models.RekamEnergi{}).Select("MAX(id)").Group("nomor_kartu_keluarga")
	query := s.DB.Where("id IN (?) AND is_deleted = ?", subQuery, false)

	if len(fields) > 0 && fields[0] != "" {
		query = query.Select(fields)
	}
	err := query.Find(&results).Error
	return results, err
}

func (s *EnergiStorage) GetDetailWithFields(noKK string, fields []string) (*models.RekamEnergi, error) {
	var result models.RekamEnergi
	query := s.DB.Model(&models.RekamEnergi{}).Where("nomor_kartu_keluarga = ?", noKK)

	if len(fields) > 0 && fields[0] != "" {
		query = query.Select(fields)
	}
	err := query.Order("version desc").First(&result).Error
	return &result, err
}

func (s *EnergiStorage) SoftDelete(id string) error {
	return s.DB.Model(&models.RekamEnergi{}).Where("id = ?", id).Update("is_deleted", true).Error
}

// GetAuditSamples mengambil sampel data versi tertinggi yang berstatus PENDING
func (s *EnergiStorage) GetAuditSamples(limit int) ([]models.RekamEnergi, error) {
    var results []models.RekamEnergi
    
    // Query dengan penambahan RANDOM() dan limit
    query := `
        SELECT k.* FROM rekam_energis k
        INNER JOIN (
            SELECT nomor_kartu_keluarga, MAX(version) as max_ver
            FROM rekam_energis
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

func (s *EnergiStorage) UpdateBulkAuditDecision(nokkList []string, verdictText string, bonus float64) error {
	return s.DB.Model(&models.RekamEnergi{}).
		Where("nomor_kartu_keluarga IN ? AND audit_status = ?", nokkList, "PENDING").
		Updates(map[string]interface{}{
			"audit_status": verdictText,
			"trust_score":  gorm.Expr("trust_score + ?", bonus),
		}).Error
}
