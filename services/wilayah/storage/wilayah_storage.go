package storage

import (
	"strings"
	"wilayah/models"

	"gorm.io/gorm"
)

type WilayahStorage struct {
	DB *gorm.DB
}

// 1. CORE OPERATIONS (SCD TYPE 2 & VERSIONING)
func (s *WilayahStorage) GetLatestByKode(kode string) (*models.MasterWilayah, error) {
	var w models.MasterWilayah
	err := s.DB.Where("kode_kelurahan_desa = ? AND is_deleted = ?", kode, false).
		Order("version desc").
		First(&w).Error
	return &w, err
}

func (s *WilayahStorage) Create(w *models.MasterWilayah) error {
	return s.DB.Create(w).Error
}

func (s *WilayahStorage) CountByKode(kode string) (int64, error) {
	var count int64
	err := s.DB.Model(&models.MasterWilayah{}).Where("kode_kelurahan_desa = ?", kode).Count(&count).Error
	return count, err
}

// 2. MONITORING & PROGRESS (SOURCE TRACKING)
func (s *WilayahStorage) GetFetchWithFields(fields []string) ([]models.MasterWilayah, error) {
	var results []models.MasterWilayah
	
	// Cari absolute ID tertinggi dari tiap Kode Desa
	subQuery := s.DB.Model(&models.MasterWilayah{}).
		Select("MAX(id)").
		Where("is_deleted = ?", false).
		Group("kode_kelurahan_desa")
	
	// Filter ID tertinggi tersebut
	query := s.DB.Where("id IN (?)", subQuery)

	if len(fields) > 0 && fields[0] != "" {
		query = query.Select(fields)
	}
	err := query.Find(&results).Error
	return results, err
}

func (s *WilayahStorage) GetDetailWithFields(kode string, fields []string) (*models.MasterWilayah, error) {
	var result models.MasterWilayah
	query := s.DB.Model(&models.MasterWilayah{}).
		Where("kode_kelurahan_desa = ? AND is_deleted = ?", kode, false)

	if len(fields) > 0 && fields[0] != "" {
		query = query.Select(fields)
	}
	err := query.Order("version desc").First(&result).Error
	return &result, err
}

// 3. MAINTENANCE (LIFECYCLE MANAGEMENT)
func (s *WilayahStorage) SoftDelete(identifier string) error {
	cleanID := strings.TrimSpace(identifier)

	//cek digit 
	if len(cleanID) == 10 { 
		return s.DB.Model(&models.MasterWilayah{}).
			Where("kode_kelurahan_desa = ?", cleanID).
			Update("is_deleted", true).Error
	}
	
	// Jika bukan 10 digit, eksekusi hapus berdasarkan ID absolut
	return s.DB.Model(&models.MasterWilayah{}).
		Where("id = ?", cleanID).
		Update("is_deleted", true).Error
}

// 4. GOVERNANCE & AUDIT (QUALITY CONTROL)
func (s *WilayahStorage) GetAuditSamples(limit int) ([]models.MasterWilayah, error) {
	var results []models.MasterWilayah
	
	query := `
		SELECT m.* FROM master_wilayah m
		INNER JOIN (
			SELECT kode_kelurahan_desa, MAX(version) as max_ver
			FROM master_wilayah
			WHERE is_deleted = false
			GROUP BY kode_kelurahan_desa
		) grouped_m 
		ON m.kode_kelurahan_desa = grouped_m.kode_kelurahan_desa 
		AND m.version = grouped_m.max_ver
		WHERE m.audit_status = 'PENDING'
		ORDER BY RANDOM()
		LIMIT ?
	`
	
	err := s.DB.Raw(query, limit).Scan(&results).Error
	return results, err
}

func (s *WilayahStorage) UpdateBulkAuditDecision(kodeDesa []string, verdictText string, bonus float64) error {
	return s.DB.Model(&models.MasterWilayah{}).
		Where("kode_kelurahan_desa IN ? AND audit_status = ?", kodeDesa, "PENDING").
		Updates(map[string]interface{}{
			"audit_status": verdictText,
			"trust_score":  gorm.Expr("trust_score + ?", bonus),
		}).Error
}