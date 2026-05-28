package storage

import (
	"pendidikan/models"
	"gorm.io/gorm"
)

type PendidikanStorage struct {
	DB *gorm.DB
}

// GetLatestByNIK mengambil record terbaru berdasarkan NIK
func (s *PendidikanStorage) GetLatestByNIK(nik string) (*models.RiwayatPendidikan, error) {
	var rp models.RiwayatPendidikan
	// Mengambil versi terbaru untuk NIK tersebut
	err := s.DB.Where("nomor_induk_kependudukan = ? AND is_deleted = ?", nik, false).Order("version desc").First(&rp).Error
	return &rp, err
}

// Create menyimpan record baru ke dalam tabel riwayat_pendidikan
func (s *PendidikanStorage) Create(rp *models.RiwayatPendidikan) error {
	return s.DB.Create(rp).Error
}

// CountByNIK menghitung progres masuknya data ke dalam mesh
func (s *PendidikanStorage) CountByNIK(nik string) (int64, error) {
	var count int64
	err := s.DB.Model(&models.RiwayatPendidikan{}).Where("nomor_induk_kependudukan = ?", nik).Count(&count).Error
	return count, err
}

// GetBySubmission mengambil data berdasarkan ID pengiriman
func (s *PendidikanStorage) GetBySubmission(subID string) ([]models.RiwayatPendidikan, error) {
	var results []models.RiwayatPendidikan
	err := s.DB.Where("source_id = ? AND is_deleted = ?", subID, false).Find(&results).Error
	return results, err
}

// SoftDelete menonaktifkan data
func (s *PendidikanStorage) SoftDelete(id string) error {
	return s.DB.Model(&models.RiwayatPendidikan{}).Where("id = ?", id).Update("is_deleted", true).Error
}

// GetFetchWithFields mendukung field selection (Datasets)
func (s *PendidikanStorage) GetFetchWithFields(fields []string) ([]models.RiwayatPendidikan, error) {
	var results []models.RiwayatPendidikan
	subQuery := s.DB.Model(&models.RiwayatPendidikan{}).Select("MAX(id)").Group("nomor_induk_kependudukan")
	query := s.DB.Where("id IN (?) AND is_deleted = ?", subQuery, false)

	if len(fields) > 0 && fields[0] != "" { query = query.Select(fields) }
	err := query.Find(&results).Error
	return results, err
}

// GetDetailWithFields mengambil data spesifik (Datasets)
func (s *PendidikanStorage) GetDetailWithFields(nik string, fields []string) (*models.RiwayatPendidikan, error) {
	var result models.RiwayatPendidikan
	query := s.DB.Model(&models.RiwayatPendidikan{}).Where("nomor_induk_kependudukan = ?", nik)

	if len(fields) > 0 && fields[0] != "" { query = query.Select(fields) }
	err := query.Order("version desc").First(&result).Error
	return &result, err
}

// UpdateAuditStatus menyimpan keputusan Approved/Rejected (Single)
func (s *PendidikanStorage) UpdateAuditStatus(id string, status string) error {
	return s.DB.Model(&models.RiwayatPendidikan{}).Where("id = ?", id).Update("audit_status", status).Error
}

// GetSample mengambil data acak untuk keperluan audit lapangan
func (s *PendidikanStorage) GetSample(limit int) ([]models.RiwayatPendidikan, error) {
	var samples []models.RiwayatPendidikan
	err := s.DB.Where("is_deleted = ?", false).Limit(limit).Order("RANDOM()").Find(&samples).Error
	return samples, err
}

// GetAuditSamples mengambil record paling tinggi yang masih pending
func (s *PendidikanStorage) GetAuditSamples() ([]models.RiwayatPendidikan, error) {
	var results []models.RiwayatPendidikan
	query := `
		SELECT w.* FROM riwayat_pendidikans w
		INNER JOIN (
			SELECT nomor_induk_kependudukan, MAX(version) as max_ver
			FROM riwayat_pendidikans
			GROUP BY nomor_induk_kependudukan
		) grouped_w 
		ON w.nomor_induk_kependudukan = grouped_w.nomor_induk_kependudukan 
		AND w.version = grouped_w.max_ver
		WHERE w.audit_status = 'PENDING'
	`
	err := s.DB.Raw(query).Scan(&results).Error
	return results, err
}

// UpdateBulkAuditDecision eksekusi final 20 poin trust score secara masal
func (s *PendidikanStorage) UpdateBulkAuditDecision(nikList []string, verdictText string, bonus float64) error {
	return s.DB.Model(&models.RiwayatPendidikan{}).
		Where("nomor_induk_kependudukan IN ? AND audit_status = ?", nikList, "PENDING").
		Updates(map[string]interface{}{
			"audit_status": verdictText,
			"trust_score":  gorm.Expr("trust_score + ?", bonus),
		}).Error
}