package storage

import (
	"strings"

	"pendidikan/models"
	"gorm.io/gorm"
)

type PendidikanStorage struct {
	DB *gorm.DB
}

// GetLatestByNIK mengambil record terbaru berdasarkan NIK
func (s *PendidikanStorage) GetLatestByNIK(nik string) (*models.RiwayatPendidikan, error) {
	var rp models.RiwayatPendidikan
	err := s.DB.Where("nomor_induk_kependudukan = ? AND is_deleted = ?", nik, false).
		Order("version desc").
		First(&rp).Error
	return &rp, err
}

// Create menyimpan record baru ke dalam tabel riwayat_pendidikan
func (s *PendidikanStorage) Create(rp *models.RiwayatPendidikan) error {
	return s.DB.Create(rp).Error
}

func (s *PendidikanStorage) CountByNIK(nik string) (int64, error) {
	var count int64
	err := s.DB.Model(&models.RiwayatPendidikan{}).Where("nomor_induk_kependudukan = ?", nik).Count(&count).Error
	return count, err
}

func (s *PendidikanStorage) GetBySubmission(subID string) ([]models.RiwayatPendidikan, error) {
	var results []models.RiwayatPendidikan
	err := s.DB.Where("source_id = ? AND is_deleted = ?", subID, false).Find(&results).Error
	return results, err
}

// SoftDelete bisa menerima NIK (16 digit) untuk hapus semua versi, atau ID spesifik
func (s *PendidikanStorage) SoftDelete(identifier string) error {
	cleanID := strings.TrimSpace(identifier)

	if len(cleanID) == 16 {
		return s.DB.Model(&models.RiwayatPendidikan{}).
			Where("nomor_induk_kependudukan = ?", cleanID).
			Update("is_deleted", true).Error
	}
	
	// Jika bukan 16 digit, eksekusi hapus berdasarkan ID absolut
	return s.DB.Model(&models.RiwayatPendidikan{}).
		Where("id = ?", cleanID).
		Update("is_deleted", true).Error
}

func (s *PendidikanStorage) GetFetchWithFields(fields []string) ([]models.RiwayatPendidikan, error) {
	var results []models.RiwayatPendidikan
	
	// Cari absolute ID tertinggi dari tiap NIK
	subQuery := s.DB.Model(&models.RiwayatPendidikan{}).
		Select("MAX(id)").
		Where("is_deleted = ?", false).
		Group("nomor_induk_kependudukan")
	
	// Filter ID tertinggi tersebut. Kalau dia terhapus, NIK-nya tidak akan tampil sama sekali
	query := s.DB.Where("id IN (?)", subQuery)

	if len(fields) > 0 && fields[0] != "" { query = query.Select(fields) }
	err := query.Find(&results).Error
	return results, err
}

func (s *PendidikanStorage) GetDetailWithFields(nik string, fields []string) (*models.RiwayatPendidikan, error) {
	var result models.RiwayatPendidikan
	query := s.DB.Model(&models.RiwayatPendidikan{}).
		Where("nomor_induk_kependudukan = ? AND is_deleted = ?", nik, false)

	if len(fields) > 0 && fields[0] != "" { query = query.Select(fields) }
	err := query.Order("version desc").First(&result).Error
	return &result, err
}

func (s *PendidikanStorage) UpdateAuditStatus(id string, status string) error {
	return s.DB.Model(&models.RiwayatPendidikan{}).Where("id = ?", id).Update("audit_status", status).Error
}

func (s *PendidikanStorage) GetAuditSamples(limit int) ([]models.RiwayatPendidikan, error) {
	var results []models.RiwayatPendidikan
	
	query := `
		SELECT w.* FROM riwayat_pendidikans w
		INNER JOIN (
			SELECT nomor_induk_kependudukan, MAX(version) as max_ver
			FROM riwayat_pendidikans
			WHERE is_deleted = false
			GROUP BY nomor_induk_kependudukan
		) grouped_w 
		ON w.nomor_induk_kependudukan = grouped_w.nomor_induk_kependudukan 
		AND w.version = grouped_w.max_ver
		WHERE w.audit_status = 'PENDING'
		ORDER BY RANDOM()
		LIMIT ?
	`
	
	err := s.DB.Raw(query, limit).Scan(&results).Error
	return results, err
}

func (s *PendidikanStorage) UpdateBulkAuditDecision(nikList []string, verdictText string, bonus float64) error {
	return s.DB.Model(&models.RiwayatPendidikan{}).
		Where("nomor_induk_kependudukan IN ? AND audit_status = ?", nikList, "PENDING").
		Updates(map[string]interface{}{
			"audit_status": verdictText,
			"trust_score":  gorm.Expr("trust_score + ?", bonus),
		}).Error
}