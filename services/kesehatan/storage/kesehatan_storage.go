package storage

import (
	"strings"

	"kesehatan/models"
	"gorm.io/gorm"
)

type KesehatanStorage struct {
	DB *gorm.DB
}

// GetLatestByNIK mengambil record terbaru berdasarkan NIK
func (s *KesehatanStorage) GetLatestByNIK(nik string) (*models.RekamKesehatan, error) {
	var rp models.RekamKesehatan
	err := s.DB.Where("nomor_induk_kependudukan = ? AND is_deleted = ?", nik, false).
		Order("version desc").
		First(&rp).Error
	return &rp, err
}

// Create menyimpan record baru (SCD Type 2)
func (s *KesehatanStorage) Create(rp *models.RekamKesehatan) error {
	return s.DB.Create(rp).Error
}

func (s *KesehatanStorage) CountByNIK(nik string) (int64, error) {
	var count int64
	err := s.DB.Model(&models.RekamKesehatan{}).Where("nomor_induk_kependudukan = ?", nik).Count(&count).Error
	return count, err
}

func (s *KesehatanStorage) GetBySubmission(subID string) ([]models.RekamKesehatan, error) {
	var results []models.RekamKesehatan
	err := s.DB.Where("source_id = ? AND is_deleted = ?", subID, false).Find(&results).Error
	return results, err
}

func (s *KesehatanStorage) GetFetchWithFields(fields []string) ([]models.RekamKesehatan, error) {
	var results []models.RekamKesehatan
	
	// Cari absolute ID tertinggi dari tiap NIK
	subQuery := s.DB.Model(&models.RekamKesehatan{}).
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

func (s *KesehatanStorage) GetDetailWithFields(nik string, fields []string) (*models.RekamKesehatan, error) {
	var result models.RekamKesehatan
	query := s.DB.Model(&models.RekamKesehatan{}).
		Where("nomor_induk_kependudukan = ? AND is_deleted = ?", nik, false)

	if len(fields) > 0 && fields[0] != "" {
		query = query.Select(fields)
	}
	err := query.Order("version desc").First(&result).Error
	return &result, err
}

func (s *KesehatanStorage) SoftDelete(identifier string) error {
	cleanID := strings.TrimSpace(identifier)

	if len(cleanID) == 16 {
		return s.DB.Model(&models.RekamKesehatan{}).Where("nomor_induk_kependudukan = ?", cleanID).Update("is_deleted", true).Error
	}
	
	// Jika bukan 16 digit, eksekusi hapus berdasarkan ID absolut
	return s.DB.Model(&models.RekamKesehatan{}).Where("id = ?", cleanID).Update("is_deleted", true).Error
}

func (s *KesehatanStorage) UpdateAuditStatus(id string, status string) error {
	return s.DB.Model(&models.RekamKesehatan{}).Where("id = ?", id).Update("audit_status", status).Error
}

func (s *KesehatanStorage) GetSample(limit int) ([]models.RekamKesehatan, error) {
	var samples []models.RekamKesehatan
	err := s.DB.Where("is_deleted = ?", false).Limit(limit).Order("RANDOM()").Find(&samples).Error
	return samples, err
}

// GetAuditSamples mengambil sampel data versi tertinggi yang berstatus PENDING
func (s *KesehatanStorage) GetAuditSamples(limit int) ([]models.RekamKesehatan, error) {
	var results []models.RekamKesehatan
	
	query := `
		SELECT k.* FROM rekam_kesehatans k
		INNER JOIN (
			SELECT nomor_induk_kependudukan, MAX(version) as max_ver
			FROM rekam_kesehatans
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

func (s *KesehatanStorage) UpdateBulkAuditDecision(nikList []string, verdictText string, bonus float64) error {
	return s.DB.Model(&models.RekamKesehatan{}).
		Where("nomor_induk_kependudukan IN ? AND audit_status = ?", nikList, "PENDING").
		Updates(map[string]interface{}{
			"audit_status": verdictText,
			"trust_score":  gorm.Expr("trust_score + ?", bonus),
		}).Error
}