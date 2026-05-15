package main

import (
	"fmt"
	"strings"
	"time"

	"wilayah/database"
	"wilayah/internal/app"
	"wilayah/internal/handler"
	"wilayah/models"
	"wilayah/storage"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"gorm.io/gorm"
)

func main() {
	// 1. Inisialisasi Koneksi Database (Domain Wilayah)
	db := database.InitDB()

	// AUTOMIGRATE: Sinkronisasi tabel Metadata (Schema) dan Data (MasterWilayah)
	db.AutoMigrate(&models.Schema{}, &models.MasterWilayah{})

	// 2. Inisialisasi Layer Architecture
	wilayahRepo := storage.WilayahStorage{DB: db}
	wilayahService := app.WilayahService{Storage: wilayahRepo}
	wilayahHandler := handler.WilayahHandler{Service: wilayahService}

	// 3. Setup Fiber Framework
	appFiber := fiber.New(fiber.Config{
		AppName: "BPS Data Mesh - Domain Wilayah v1.0",
	})

	// Middleware: Logging & Crash Recovery
	appFiber.Use(logger.New())
	appFiber.Use(recover.New())

	// 4. ROUTING (13 ENDPOINTS DATA MESH - IDENTIK)
	api := appFiber.Group("/api/v1/domains/wilayah")

	// A. DATA INGESTION & MONITORING (4 Endpoints)
	ingestion := api.Group("/submissions")
	{
		// 1. Ingestion  (Mendukung Multi-format CSV/JSON/Parquet & SCD Type 2)
		ingestion.Post("/", wilayahHandler.IngestData)

		// 2. Cek Validasi Format & Status Terakhir
		ingestion.Get("/:kode/validation", func(c *fiber.Ctx) error {
			var result models.MasterWilayah
			db.Where("kode_kelurahan_desa = ?", c.Params("kode")).Order("version desc").First(&result)
			return c.JSON(fiber.Map{
				"kode_desa": c.Params("kode"),
				"status":    result.AuditStatus,
				"schema":    "DTSEN-WIL-ACTIVE",
			})
		})

		// 3. Cek Laporan Kualitas & Skoring
		ingestion.Get("/:kode/scoring", func(c *fiber.Ctx) error {
			var result models.MasterWilayah
			if err := db.Where("kode_kelurahan_desa = ?", c.Params("kode")).Order("version desc").First(&result).Error; err != nil {
				return c.Status(404).JSON(fiber.Map{"error": "Data wilayah tidak ditemukan"})
			}
			return c.JSON(fiber.Map{
				"kode_desa":     result.KodeDesa,
				"trust_score":   result.TrustScore,
				"is_walidata":   result.IsWaliData,
				"audit_status":  result.AuditStatus,
				"quality_label": "Kalkulasi: 60(Sistem) + 20(Sumber) + 20(Audit)",
			})
		})

		// 4. Cek Status Progres
		ingestion.Get("/:kode/progress", func(c *fiber.Ctx) error {
			var count int64
			db.Model(&models.MasterWilayah{}).Where("kode_kelurahan_desa = ?", c.Params("kode")).Count(&count)
			status := "NOT_FOUND"
			if count > 0 {
				status = "COMPLETED_IN_MESH"
			}
			return c.JSON(fiber.Map{"kode_desa": c.Params("kode"), "status": status, "progress": "100%"})
		})
	}

	// B. METADATA & SCHEMA MANAGEMENT (3 Endpoints - DINAMIS)
	schemas := api.Group("/schemas")
	{
		// 5 & 7. Daftar & Revisi Skema
		schemas.Post("/", wilayahHandler.CreateSchemaHandler)
		schemas.Patch("/", wilayahHandler.CreateSchemaHandler)

		// 6. Cek Detail Skema Aktif
		schemas.Get("/latest", wilayahHandler.GetLatestSchemaHandler)
	}

	// C. DATASET MAINTENANCE & DISCOVERY (3 Endpoints)
	datasets := api.Group("/datasets")
	{
		// 8. GET: Data Keseluruhan (Dinamis Field Selection)
		datasets.Get("/", func(c *fiber.Ctx) error {
			fields := c.Query("fields")
			var results []models.MasterWilayah

			subQuery := db.Model(&models.MasterWilayah{}).Select("MAX(id)").Group("kode_kelurahan_desa")
			query := db.Where("id IN (?) AND is_deleted = ?", subQuery, false)

			if fields != "" {
				query = query.Select(strings.Split(fields, ","))
			}

			query.Find(&results)
			return c.JSON(results)
		})

		// 9. GET: Detail Kode Desa
		datasets.Get("/:kode", func(c *fiber.Ctx) error {
			fields := c.Query("fields")
			var result models.MasterWilayah

			query := db.Model(&models.MasterWilayah{}).Where("kode_kelurahan_desa = ?", c.Params("kode"))
			if fields != "" {
				query = query.Select(strings.Split(fields, ","))
			}

			if err := query.Order("version desc").First(&result).Error; err != nil {
				return c.Status(404).JSON(fiber.Map{"error": "Kode Desa tidak ditemukan"})
			}
			return c.JSON(result)
		})

		// 10. PUT: Koreksi Nilai
		datasets.Put("/:kode", func(c *fiber.Ctx) error {
			var oldData models.MasterWilayah
			if err := db.Where("kode_kelurahan_desa = ?", c.Params("kode")).Order("version desc").First(&oldData).Error; err != nil {
				return c.Status(404).JSON(fiber.Map{"error": "Data wilayah asli tidak ditemukan"})
			}

			newData := oldData
			if err := c.BodyParser(&newData); err != nil {
				return c.Status(400).JSON(fiber.Map{"error": "Payload tidak valid"})
			}

			// LOGIKA RESET SCD TYPE 2
			newData.ID = 0
			newData.Version = oldData.Version + 1
			newData.AuditStatus = "PENDING"
			newData.UpdatedAt = time.Now()

			// Skor kembali ke base (60 Sistem + 20 Sumber jika Walidata)
			if newData.IsWaliData {
				newData.TrustScore = 80.0
			} else {
				newData.TrustScore = 60.0
			}

			db.Create(&newData)
			return c.JSON(fiber.Map{"message": "Versi baru wilayah dibuat, status kembali PENDING", "version": newData.Version})
		})
	}

	// D. GOVERNANCE & LIFECYCLE (3 Endpoints)
	governance := api.Group("/")
	{
		// 11. DELETE: Soft Delete
		governance.Delete("/datasets/:id", func(c *fiber.Ctx) error {
			db.Model(&models.MasterWilayah{}).Where("id = ?", c.Params("id")).Update("is_deleted", true)
			return c.JSON(fiber.Map{"message": "Data wilayah berhasil dinonaktifkan (Soft Delete)"})
		})

		// 12. GET: Ambil Sample Data untuk Diaudit ( Hanya Versi Tertinggi)
		governance.Get("/audit/samples", func(c *fiber.Ctx) error {
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

			if err := db.Raw(query).Scan(&results).Error; err != nil {
				return c.Status(500).JSON(fiber.Map{"error": "Gagal mengambil data audit"})
			}

			// Tambahan opsional
			if len(results) == 0 {
				return c.JSON(fiber.Map{"message": "Tidak ada data wilayah terbaru yang perlu diaudit."})
			}

			return c.JSON(results)
		})

		// 13. POST: Keputusan Audit (Final 20 Poin Trust Score)
		governance.Post("/audit/decision", func(c *fiber.Ctx) error {
			var input struct {
				KodeDesa []string `json:"kode_kelurahan_desa"`
				Verdict  int      `json:"verdict"` // 1 = VALID, 2 = INVALID
			}

			if err := c.BodyParser(&input); err != nil {
				return c.Status(400).JSON(fiber.Map{"error": "Payload JSON tidak valid"})
			}

			if len(input.KodeDesa) == 0 {
				return c.Status(400).JSON(fiber.Map{"error": "Daftar kode_kelurahan_desa tidak boleh kosong. Harus tahu pasti data mana yang diaudit."})
			}

			// Penerjemah Angka ke Teks & Logika Bonus
			verdictText := "INVALID"
			bonus := 0.0

			if input.Verdict == 1 {
				verdictText = "VALID"
				bonus = 20.0
			} else if input.Verdict != 2 {
				return c.Status(400).JSON(fiber.Map{"error": "Verdict tidak valid. Gunakan 1 (VALID) atau 2 (INVALID)"})
			}

			// Update ke Database menggunakan teks hasil terjemahan
			err := db.Model(&models.MasterWilayah{}).
				Where("kode_kelurahan_desa IN ? AND audit_status = ?", input.KodeDesa, "PENDING").
				Updates(map[string]interface{}{
					"audit_status": verdictText,
					"trust_score":  gorm.Expr("trust_score + ?", bonus),
				}).Error

			if err != nil {
				return c.Status(500).JSON(fiber.Map{"error": "Gagal update audit wilayah massal"})
			}

			pesan := fmt.Sprintf("Audit wilayah selesai. %d desa telah diubah statusnya menjadi %s", len(input.KodeDesa), verdictText)
			return c.JSON(fiber.Map{"message": pesan})
		})
	}

	// 5. Run Server pada Port 8083
	fmt.Println("---------------------------------------------------------")
	fmt.Println(" BPS DATA MESH: DOMAIN WILAYAH RUNNING")
	fmt.Println(" Port: 8083 ")
	fmt.Println("---------------------------------------------------------")
	appFiber.Listen(":8083")
}
