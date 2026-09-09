package storage

import (
	"strings"

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

func (s *PendudukStorage) CountByNIK(nik string) (int64, error) {
	var count int64
	err := s.DB.Model(&models.Penduduk{}).Where("nomor_induk_kependudukan = ?", nik).Count(&count).Error
	return count, err
}

func (s *PendudukStorage) GetBySubmission(sourceID string) ([]models.Penduduk, error) {
	var results []models.Penduduk
	err := s.DB.Where("source_id = ? AND is_deleted = ?", sourceID, false).Find(&results).Error
	return results, err
}

// SoftDelete bisa menerima NIK (16 digit) untuk hapus semua versi, atau ID spesifik
func (s *PendudukStorage) SoftDelete(identifier string) error {
	// PERBAIKAN: Sapu bersih spasi gaib dari Postman
	cleanID := strings.TrimSpace(identifier)

	if len(cleanID) == 16 {
		return s.DB.Model(&models.Penduduk{}).Where("nomor_induk_kependudukan = ?", cleanID).Update("is_deleted", true).Error
	}
	return s.DB.Model(&models.Penduduk{}).Where("id = ?", cleanID).Update("is_deleted", true).Error
}

func (s *PendudukStorage) GetFetchWithFields(fields []string) ([]models.Penduduk, error) {
	var results []models.Penduduk
	
	// Cari absolute ID tertinggi dari tiap NIK
	subQuery := s.DB.Model(&models.Penduduk{}).Select("MAX(id)").Group("nomor_induk_kependudukan")
	
	// Filter ID tertinggi tersebut.Jika (is_deleted = true), NIK-nya tidak akan tampil sama sekali
	query := s.DB.Where("id IN (?) AND is_deleted = ?", subQuery, false)

	if len(fields) > 0 && fields[0] != "" {
		query = query.Select(fields)
	}
	err := query.Find(&results).Error
	return results, err
}

func (s *PendudukStorage) GetDetailWithFields(nik string, fields []string) (*models.Penduduk, error) {
	var result models.Penduduk
	query := s.DB.Model(&models.Penduduk{}).
		Where("nomor_induk_kependudukan = ? AND is_deleted = ?", nik, false)

	if len(fields) > 0 && fields[0] != "" {
		query = query.Select(fields)
	}
	err := query.Order("version desc").First(&result).Error
	return &result, err
}

func (s *PendudukStorage) UpdateManual(id string, data map[string]interface{}) error {
	return s.DB.Model(&models.Penduduk{}).Where("id = ?", id).Updates(data).Error
}

func (s *PendudukStorage) UpdateAuditStatus(id string, status string) error {
	return s.DB.Model(&models.Penduduk{}).Where("id = ?", id).Update("audit_status", status).Error
}

func (s *PendudukStorage) GetAuditSamples(limit int) ([]models.Penduduk, error) {
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
		WHERE k.audit_status = 'PENDING' AND k.is_deleted = false
		ORDER BY RANDOM()
		LIMIT ?
	`
	err := s.DB.Raw(query, limit).Scan(&results).Error
	return results, err
}

func (s *PendudukStorage) UpdateBulkAuditDecision(nikList []string, verdictText string, bonus float64) error {
	return s.DB.Model(&models.Penduduk{}).
		Where("nomor_induk_kependudukan IN ? AND audit_status = ?", nikList, "PENDING").
		Updates(map[string]interface{}{
			"audit_status": verdictText,
			"trust_score":  gorm.Expr("trust_score + ?", bonus),
		}).Error
}