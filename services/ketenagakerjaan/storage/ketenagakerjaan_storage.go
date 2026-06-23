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
	err := s.DB.Where("nomor_induk_kependudukan = ? AND is_deleted = ?", nik, false).Order("version desc").First(&rk).Error
	return &rk, err
}

func (s *KetenagakerjaanStorage) Create(rk *models.RekamKetenagakerjaan) error {
	return s.DB.Create(rk).Error
}

func (s *KetenagakerjaanStorage) CountByNIK(nik string) (int64, error) {
	var count int64
	err := s.DB.Model(&models.RekamKetenagakerjaan{}).Where("nomor_induk_kependudukan = ?", nik).Count(&count).Error
	return count, err
}

func (s *KetenagakerjaanStorage) GetFetchWithFields(fields []string) ([]models.RekamKetenagakerjaan, error) {
	var results []models.RekamKetenagakerjaan
	subQuery := s.DB.Model(&models.RekamKetenagakerjaan{}).Select("MAX(id)").Group("nomor_induk_kependudukan")
	query := s.DB.Where("id IN (?) AND is_deleted = ?", subQuery, false)

	if len(fields) > 0 && fields[0] != "" {
		query = query.Select(fields)
	}
	err := query.Find(&results).Error
	return results, err
}

func (s *KetenagakerjaanStorage) GetDetailWithFields(nik string, fields []string) (*models.RekamKetenagakerjaan, error) {
	var result models.RekamKetenagakerjaan
	query := s.DB.Model(&models.RekamKetenagakerjaan{}).Where("nomor_induk_kependudukan = ?", nik)

	if len(fields) > 0 && fields[0] != "" {
		query = query.Select(fields)
	}
	err := query.Order("version desc").First(&result).Error
	return &result, err
}

func (s *KetenagakerjaanStorage) SoftDelete(id string) error {
	return s.DB.Model(&models.RekamKetenagakerjaan{}).Where("id = ?", id).Update("is_deleted", true).Error
}

// GetAuditSamples mengambil sampel data versi tertinggi yang berstatus PENDING
func (s *KetenagakerjaanStorage) GetAuditSamples(limit int) ([]models.RekamKetenagakerjaan, error) {
    var results []models.RekamKetenagakerjaan
    
    // Query dengan penambahan RANDOM() dan limit
    query := `
        SELECT k.* FROM rekam_ketenagakerjaans k
        INNER JOIN (
            SELECT nomor_induk_kependudukan, MAX(version) as max_ver
            FROM rekam_ketenagakerjaans
            GROUP BY nomor_induk_kependudukan
        ) grouped_k 
        ON k.nomor_induk_kependudukan = grouped_k.nomor_induk_kependudukan 
        AND k.version = grouped_k.max_ver
        WHERE k.audit_status = 'PENDING'
        ORDER BY RANDOM()
        LIMIT ?
    `
    
    // Oper limit ke Raw query
    err := s.DB.Raw(query, limit).Scan(&results).Error
    return results, err
}

func (s *KetenagakerjaanStorage) UpdateBulkAuditDecision(nikList []string, verdictText string, bonus float64) error {
	return s.DB.Model(&models.RekamKetenagakerjaan{}).
		Where("nomor_induk_kependudukan IN ? AND audit_status = ?", nikList, "PENDING").
		Updates(map[string]interface{}{
			"audit_status": verdictText,
			"trust_score":  gorm.Expr("trust_score + ?", bonus),
		}).Error
}
