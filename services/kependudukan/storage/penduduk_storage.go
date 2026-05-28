package storage

import (
	"kependudukan/models"
	"gorm.io/gorm"
)

type PendudukStorage struct {
	DB *gorm.DB
}

// GetLatestByNIK mengambil record terbaru berdasarkan NIK 
func (s *PendudukStorage) GetLatestByNIK(nik string) (*models.Penduduk, error) {
	var p models.Penduduk
	err := s.DB.Where("nomor_induk_kependudukan = ? AND is_deleted = ?", nik, false).
		Order("version desc").
		First(&p).Error
	return &p, err
}

// Create menyimpan record baru ke dalam tabel penduduk 
func (s *PendudukStorage) Create(p *models.Penduduk) error {
	return s.DB.Create(p).Error
}

// CountByNIK menghitung progres
func (s *PendudukStorage) CountByNIK(nik string) (int64, error) {
	var count int64
	err := s.DB.Model(&models.Penduduk{}).Where("nomor_induk_kependudukan = ?", nik).Count(&count).Error
	return count, err
}

// GetBySubmission mengambil data berdasarkan ID pengirim 
func (s *PendudukStorage) GetBySubmission(sourceID string) ([]models.Penduduk, error) {
	var results []models.Penduduk
	err := s.DB.Where("source_id = ? AND is_deleted = ?", sourceID, false).Find(&results).Error
	return results, err
}

// SoftDelete menandai data sebagai terhapus
func (s *PendudukStorage) SoftDelete(id string) error {
	return s.DB.Model(&models.Penduduk{}).Where("id = ?", id).Update("is_deleted", true).Error
}

// GetFetchWithFields mengambil dataset keseluruhan
func (s *PendudukStorage) GetFetchWithFields(fields []string) ([]models.Penduduk, error) {
	var results []models.Penduduk
	subQuery := s.DB.Model(&models.Penduduk{}).Select("MAX(id)").Group("nomor_induk_kependudukan")
	query := s.DB.Where("id IN (?) AND is_deleted = ?", subQuery, false)

	if len(fields) > 0 && fields[0] != "" { query = query.Select(fields) }
	err := query.Find(&results).Error
	return results, err
}

// GetDetailWithFields mengambil data spesifik
func (s *PendudukStorage) GetDetailWithFields(nik string, fields []string) (*models.Penduduk, error) {
	var result models.Penduduk
	query := s.DB.Model(&models.Penduduk{}).Where("nomor_induk_kependudukan = ?", nik)

	if len(fields) > 0 && fields[0] != "" { query = query.Select(fields) }
	err := query.Order("version desc").First(&result).Error
	return &result, err
}

// UpdateManual melakukan pembaruan parsial jika ada koreksi manual
func (s *PendudukStorage) UpdateManual(id string, data map[string]interface{}) error {
	return s.DB.Model(&models.Penduduk{}).Where("id = ?", id).Updates(data).Error
}

// UpdateAuditStatus menyimpan keputusan audir
func (s *PendudukStorage) UpdateAuditStatus(id string, status string) error {
	return s.DB.Model(&models.Penduduk{}).Where("id = ?", id).Update("audit_status", status).Error
}

// GetSample mengambil data acak untuk keperluan audit lapangan 
func (s *PendudukStorage) GetSample(limit int) ([]models.Penduduk, error) {
	var samples []models.Penduduk
	err := s.DB.Where("is_deleted = ?", false).Limit(limit).Order("RANDOM()").Find(&samples).Error
	return samples, err
}

// GetAuditSamples mengambil data versi tertinggi yang berstatus PENDING
func (s *PendudukStorage) GetAuditSamples() ([]models.Penduduk, error) {
	var results []models.Penduduk
	query := `
		SELECT k.* FROM penduduks k
		INNER JOIN (
			SELECT nomor_induk_kependudukan, MAX(version) as max_ver
			FROM penduduks
			GROUP BY nomor_induk_kependudukan
		) grouped_k 
		ON k.nomor_induk_kependudukan = grouped_k.nomor_induk_kependudukan
		AND k.version = grouped_k.max_ver
		WHERE k.audit_status = 'PENDING'
	`
	err := s.DB.Raw(query).Scan(&results).Error
	return results, err
}

// UpdateBulkAuditDecision update audit kependudukan massal
func (s *PendudukStorage) UpdateBulkAuditDecision(nikList []string, verdictText string, bonus float64) error {
	return s.DB.Model(&models.Penduduk{}).
		Where("nomor_induk_kependudukan IN ? AND audit_status = ?", nikList, "PENDING").
		Updates(map[string]interface{}{
			"audit_status": verdictText,
			"trust_score":  gorm.Expr("trust_score + ?", bonus),
		}).Error
}