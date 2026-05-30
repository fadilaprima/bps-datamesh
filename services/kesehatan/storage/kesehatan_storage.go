package storage

import (
	"kesehatan/models"

	"gorm.io/gorm"
)

type KesehatanStorage struct {
	DB *gorm.DB
}

// GetLatestByNIK mengambil record terbaru berdasarkan NIK
func (s *KesehatanStorage) GetLatestByNIK(nik string) (*models.RekamKesehatan, error) {
	var rp models.RekamKesehatan
	// Mengambil versi terbaru untuk NIK tersebut
	err := s.DB.Where("nomor_induk_kependudukan = ? AND is_deleted = ?", nik, false).Order("version desc").First(&rp).Error
	return &rp, err
}

// Create menyimpan record baru ke dalam tabel Rekam_kesehatan
func (s *KesehatanStorage) Create(rp *models.RekamKesehatan) error {
	return s.DB.Create(rp).Error
}

// CountByNIK menghitung progres masuknya data ke dalam mesh
func (s *KesehatanStorage) CountByNIK(nik string) (int64, error) {
	var count int64
	err := s.DB.Model(&models.RekamKesehatan{}).Where("nomor_induk_kependudukan = ?", nik).Count(&count).Error
	return count, err
}

// GetBySubmission mengambil data berdasarkan ID pengiriman
func (s *KesehatanStorage) GetBySubmission(subID string) ([]models.RekamKesehatan, error) {
	var results []models.RekamKesehatan
	err := s.DB.Where("source_id = ? AND is_deleted = ?", subID, false).Find(&results).Error
	return results, err
}

// SoftDelete menonaktifkan data
func (s *KesehatanStorage) SoftDelete(id string) error {
	return s.DB.Model(&models.RekamKesehatan{}).Where("id = ?", id).Update("is_deleted", true).Error
}

// GetFetchWithFields mendukung field selection (Datasets)
func (s *KesehatanStorage) GetFetchWithFields(fields []string) ([]models.RekamKesehatan, error) {
	var results []models.RekamKesehatan
	subQuery := s.DB.Model(&models.RekamKesehatan{}).Select("MAX(id)").Group("nomor_induk_kependudukan")
	query := s.DB.Where("id IN (?) AND is_deleted = ?", subQuery, false)

	if len(fields) > 0 && fields[0] != "" {
		query = query.Select(fields)
	}
	err := query.Find(&results).Error
	return results, err
}

// GetDetailWithFields mengambil data spesifik (Datasets)
func (s *KesehatanStorage) GetDetailWithFields(nik string, fields []string) (*models.RekamKesehatan, error) {
	var result models.RekamKesehatan
	query := s.DB.Model(&models.RekamKesehatan{}).Where("nomor_induk_kependudukan = ?", nik)

	if len(fields) > 0 && fields[0] != "" {
		query = query.Select(fields)
	}
	err := query.Order("version desc").First(&result).Error
	return &result, err
}

// UpdateAuditStatus menyimpan keputusan Approved/Rejected (Single)
func (s *KesehatanStorage) UpdateAuditStatus(id string, status string) error {
	return s.DB.Model(&models.RekamKesehatan{}).Where("id = ?", id).Update("audit_status", status).Error
}

// GetSample mengambil data acak untuk keperluan audit
func (s *KesehatanStorage) GetSample(limit int) ([]models.RekamKesehatan, error) {
	var samples []models.RekamKesehatan
	err := s.DB.Where("is_deleted = ?", false).Limit(limit).Order("RANDOM()").Find(&samples).Error
	return samples, err
}

// GetAuditSamples mengambil data pending yang merupakan versi paling mutakhir (tertinggi)
func (s *KesehatanStorage) GetAuditSamples() ([]models.RekamKesehatan, error) {
	var results []models.RekamKesehatan
	query := `
		SELECT k.* FROM rekam_kesehatans k
		INNER JOIN (
			SELECT nomor_induk_kependudukan, MAX(version) as max_ver
			FROM rekam_kesehatans
			GROUP BY nomor_induk_kependudukan
		) grouped_k 
		ON k.nomor_induk_kependudukan = grouped_k.nomor_induk_kependudukan 
		AND k.version = grouped_k.max_ver
		WHERE k.audit_status = 'PENDING'
	`
	err := s.DB.Raw(query).Scan(&results).Error
	return results, err
}

// UpdateBulkAuditDecision update audit kesehatan massal
func (s *KesehatanStorage) UpdateBulkAuditDecision(nikList []string, verdictText string, bonus float64) error {
	return s.DB.Model(&models.RekamKesehatan{}).
		Where("nomor_induk_kependudukan IN ? AND audit_status = ?", nikList, "PENDING").
		Updates(map[string]interface{}{
			"audit_status": verdictText,
			"trust_score":  gorm.Expr("trust_score + ?", bonus),
		}).Error
}
