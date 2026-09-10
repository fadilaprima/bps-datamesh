package storage

import (
	"strings"

	"ketenagakerjaan/models"
	"gorm.io/gorm"
)

type KetenagakerjaanStorage struct {
	DB *gorm.DB
}

// GetLatestByNIK mengambil record terbaru berdasarkan NIK
func (s *KetenagakerjaanStorage) GetLatestByNIK(nik string) (*models.RekamKetenagakerjaan, error) {
	var rk models.RekamKetenagakerjaan
	err := s.DB.Where("nomor_induk_kependudukan = ? AND is_deleted = ?", nik, false).
		Order("version desc").
		First(&rk).Error
	return &rk, err
}

// Create menyimpan record baru
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
	
	// Cari absolute ID tertinggi dari tiap NIK
	subQuery := s.DB.Model(&models.RekamKetenagakerjaan{}).
		Select("MAX(id)").
		Where("is_deleted = ?", false).
		Group("nomor_induk_kependudukan")
	
	// Filter ID tertinggi tersebut. Kalau dia terhapus, NIK-nya tidak akan tampil sama sekali
	query := s.DB.Where("id IN (?)", subQuery)

	if len(fields) > 0 && fields[0] != "" {
		query = query.Select(fields)
	}
	err := query.Find(&results).Error
	return results, err
}

func (s *KetenagakerjaanStorage) GetDetailWithFields(nik string, fields []string) (*models.RekamKetenagakerjaan, error) {
	var result models.RekamKetenagakerjaan
	query := s.DB.Model(&models.RekamKetenagakerjaan{}).
		Where("nomor_induk_kependudukan = ? AND is_deleted = ?", nik, false)

	if len(fields) > 0 && fields[0] != "" {
		query = query.Select(fields)
	}
	err := query.Order("version desc").First(&result).Error
	return &result, err
}

func (s *KetenagakerjaanStorage) SoftDelete(identifier string) error {
	cleanID := strings.TrimSpace(identifier)

	// Pencarian menggunakan NIK (16 digit)
	if len(cleanID) == 16 {
		return s.DB.Model(&models.RekamKetenagakerjaan{}).
			Where("nomor_induk_kependudukan = ?", cleanID).
			Update("is_deleted", true).Error
	}

	// Jika bukan 16 digit, gunakan ID absolut
	return s.DB.Model(&models.RekamKetenagakerjaan{}).
		Where("id = ?", cleanID).
		Update("is_deleted", true).Error
}

// GetAuditSamples mengambil sampel data versi tertinggi yang berstatus PENDING
func (s *KetenagakerjaanStorage) GetAuditSamples(limit int) ([]models.RekamKetenagakerjaan, error) {
	var results []models.RekamKetenagakerjaan
	
	query := `
		SELECT k.* FROM rekam_ketenagakerjaans k
		INNER JOIN (
			SELECT nomor_induk_kependudukan, MAX(version) as max_ver
			FROM rekam_ketenagakerjaans
			WHERE is_deleted = false
			GROUP BY nomor_induk_kependudukan
		) grouped_k 
		ON k.nomor_induk_kependudukan = grouped_k.nomor_induk_kependudukan 
		AND k.version = grouped_k.max_ver
		WHERE k.audit_status = 'PENDING'
		ORDER BY RANDOM()
		LIMIT ?
	`
	
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