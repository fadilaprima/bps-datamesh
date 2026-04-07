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

	// 2. Inisialisasi Layer Architecture (Dependency Injection)
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

	// --- 4. ROUTING (13 ENDPOINTS DATA MESH - IDENTIK) ---
	api := appFiber.Group("/api/v1/domains/wilayah")

	// ============================================================
	// A. DATA INGESTION & MONITORING (4 Endpoints)
	// ============================================================
	ingestion := api.Group("/submissions")
	{
		// 1. Ingestion Utama (Mendukung Multi-format CSV/JSON/Parquet & SCD Type 2)
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

		// 3. Cek Laporan Kualitas & Skoring (Trust Score Real-time)
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
			if count > 0 { status = "COMPLETED_IN_MESH" }
			return c.JSON(fiber.Map{"kode_desa": c.Params("kode"), "status": status, "progress": "100%"})
		})
	}

	// ============================================================
	// B. METADATA & SCHEMA MANAGEMENT (3 Endpoints - DINAMIS)
	// ============================================================
	schemas := api.Group("/schemas")
	{
		// 5 & 7. Daftar & Revisi Skema (Mendukung Validasi Dinamis Kode Wilayah)
		schemas.Post("/", wilayahHandler.CreateSchemaHandler)
		schemas.Patch("/", wilayahHandler.CreateSchemaHandler)

		// 6. Cek Detail Skema Aktif (Kiblat Aturan Metadata Wilayah)
		schemas.Get("/latest", wilayahHandler.GetLatestSchemaHandler)
	}

	// ============================================================
	// C. DATASET MAINTENANCE & DISCOVERY (3 Endpoints)
	// ============================================================
	datasets := api.Group("/datasets")
	{
		// 8. GET: Data Keseluruhan (Golden Record + Dynamic Field Selection)
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

		// 9. GET: Detail Kode Desa (History/Golden Record + Dynamic Field Selection)
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

		// 10. PUT: Koreksi Nilai (SCD Type 2: Status Reset ke PENDING)
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
			if newData.IsWaliData { newData.TrustScore = 80.0 } else { newData.TrustScore = 60.0 }

			db.Create(&newData)
			return c.JSON(fiber.Map{"message": "Versi baru wilayah dibuat, status kembali PENDING", "version": newData.Version})
		})
	}

	// ============================================================
	// D. GOVERNANCE & LIFECYCLE (3 Endpoints)
	// ============================================================
	governance := api.Group("/")
	{
		// 11. DELETE: Soft Delete
		governance.Delete("/datasets/:id", func(c *fiber.Ctx) error {
			db.Model(&models.MasterWilayah{}).Where("id = ?", c.Params("id")).Update("is_deleted", true)
			return c.JSON(fiber.Map{"message": "Data wilayah berhasil dinonaktifkan (Soft Delete)"})
		})

		// 12. GET: Sampel Data Acak untuk Audit Wilayah
		governance.Get("/audit/samples", func(c *fiber.Ctx) error {
			var samples []models.MasterWilayah
			db.Where("audit_status = ?", "PENDING").Order("RANDOM()").Limit(5).Find(&samples)
			return c.JSON(samples)
		})

		// 13. POST: Keputusan Audit (Final 20 Poin Trust Score)
		governance.Post("/audit/decision", func(c *fiber.Ctx) error {
			var input struct {
				KodeDesa string `json:"kode_desa"`
				Verdict  string `json:"verdict"` // VALID / INVALID
			}
			c.BodyParser(&input)

			bonus := 0.0
			if strings.ToUpper(input.Verdict) == "VALID" { bonus = 20.0 }

			err := db.Model(&models.MasterWilayah{}).
				Where("kode_kelurahan_desa = ? AND audit_status = ?", input.KodeDesa, "PENDING").
				Updates(map[string]interface{}{
					"audit_status": strings.ToUpper(input.Verdict),
					"trust_score":  gorm.Expr("trust_score + ?", bonus),
				}).Error

			if err != nil { return c.Status(500).JSON(fiber.Map{"error": "Gagal update audit wilayah"}) }
			return c.JSON(fiber.Map{"message": "Audit wilayah selesai, trust score diperbarui"})
		})
	}

	// 5. Run Server pada Port 8083 (Sesuai Master Plan)
	fmt.Println("---------------------------------------------------------")
	fmt.Println(" BPS DATA MESH: DOMAIN WILAYAH RUNNING")
	fmt.Println(" Port: 8083 | Status: Identik & Dynamic")
	fmt.Println("---------------------------------------------------------")
	appFiber.Listen(":8083")
}