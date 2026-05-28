package storage

import (
	"wilayah/models"

	"gorm.io/gorm"
)

type WilayahStorage struct {
	DB *gorm.DB
}

// 1. CORE OPERATIONS (SCD TYPE 2 & VERSIONING)
// GetLatestByKode mengambil record terbaru berdasarkan Kode Desa/Kelurahan (Natural Key)
func (s *WilayahStorage) GetLatestByKode(kode string) (*models.MasterWilayah, error) {
	var w models.MasterWilayah
	// Mengambil versi terbaru yang belum dihapus (Soft Delete)
	err := s.DB.Where("kode_kelurahan_desa = ? AND is_deleted = ?", kode, false).
		Order("version desc").
		First(&w).Error
	return &w, err
}

// Create menyimpan record baru ke dalam tabel master_wilayah
func (s *WilayahStorage) Create(w *models.MasterWilayah) error {
	return s.DB.Create(w).Error
}

// CountByKode menghitung jumlah record berdasarkan kode untuk keperluan cek progres
func (s *WilayahStorage) CountByKode(kode string) (int64, error) {
	var count int64
	err := s.DB.Model(&models.MasterWilayah{}).Where("kode_kelurahan_desa = ?", kode).Count(&count).Error
	return count, err
}

// 2. MONITORING & PROGRESS (SOURCE TRACKING)
// GetFetchWithFields mengambil semua data aktif dengan pemilihan field dinamis
func (s *WilayahStorage) GetFetchWithFields(fields []string) ([]models.MasterWilayah, error) {
	var results []models.MasterWilayah
	subQuery := s.DB.Model(&models.MasterWilayah{}).Select("MAX(id)").Group("kode_kelurahan_desa")
	query := s.DB.Where("id IN (?) AND is_deleted = ?", subQuery, false)

	if len(fields) > 0 && fields[0] != "" {
		query = query.Select(fields)
	}
	err := query.Find(&results).Error
	return results, err
}

// GetDetailWithFields mengambil satu data desa berdasarkan kode dengan pemilihan field dinamis
func (s *WilayahStorage) GetDetailWithFields(kode string, fields []string) (*models.MasterWilayah, error) {
	var result models.MasterWilayah
	query := s.DB.Model(&models.MasterWilayah{}).Where("kode_kelurahan_desa = ?", kode)

	if len(fields) > 0 && fields[0] != "" {
		query = query.Select(fields)
	}
	err := query.Order("version desc").First(&result).Error
	return &result, err
}

// 3. MAINTENANCE (LIFECYCLE MANAGEMENT)
// SoftDelete menandai data wilayah sebagai terhapus tanpa menghilangkan dari database
func (s *WilayahStorage) SoftDelete(id string) error {
	return s.DB.Model(&models.MasterWilayah{}).
		Where("id = ?", id).
		Update("is_deleted", true).Error
}

// 4. GOVERNANCE & AUDIT (QUALITY CONTROL)
// GetAuditSamples mengambil data versi tertinggi yang berstatus PENDING
func (s *WilayahStorage) GetAuditSamples() ([]models.MasterWilayah, error) {
	var results []models.MasterWilayah
	// Mengambil data pending yang merupakan versi paling mutakhir (tertinggi)
	query := `
		SELECT m.* FROM master_wilayah m
		INNER JOIN (
			SELECT kode_kelurahan_desa, MAX(version) as max_ver
			FROM master_wilayah
			GROUP BY kode_kelurahan_desa
		) grouped_m 
		ON m.kode_kelurahan_desa = grouped_m.kode_kelurahan_desa 
		AND m.version = grouped_m.max_ver
		WHERE m.audit_status = 'PENDING'
	`
	err := s.DB.Raw(query).Scan(&results).Error
	return results, err
}

// UpdateBulkAuditDecision menyimpan keputusan Approved/Rejected beserta Trust Score
func (s *WilayahStorage) UpdateBulkAuditDecision(kodeDesa []string, verdictText string, bonus float64) error {
	return s.DB.Model(&models.MasterWilayah{}).
		Where("kode_kelurahan_desa IN ? AND audit_status = ?", kodeDesa, "PENDING").
		Updates(map[string]interface{}{
			"audit_status": verdictText,
			"trust_score":  gorm.Expr("trust_score + ?", bonus),
		}).Error
}
