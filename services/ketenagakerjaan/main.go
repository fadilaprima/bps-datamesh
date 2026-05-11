package main

import (
	"fmt"
	"strings"
	"time"

	"ketenagakerjaan/database"
	"ketenagakerjaan/internal/app"
	"ketenagakerjaan/internal/handler"
	"ketenagakerjaan/models"
	"ketenagakerjaan/storage"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"gorm.io/gorm"
)

func main() {
	// 1. Inisialisasi Koneksi Database (Domain Ketenagakerjaan)
	db := database.InitDB()

	// AUTOMIGRATE: Sinkronisasi tabel Metadata (Schema) dan Data (RekamKetenagakerjaan)
	db.AutoMigrate(&models.Schema{}, &models.RekamKetenagakerjaan{})

	// 2. Inisialisasi Layer Architecture (Dependency Injection)
	ketenagakerjaanRepo := storage.KetenagakerjaanStorage{DB: db}
	ketenagakerjaanService := app.KetenagakerjaanService{Storage: ketenagakerjaanRepo}
	ketenagakerjaanHandler := handler.KetenagakerjaanHandler{Service: ketenagakerjaanService}

	// 3. Setup Fiber Framework
	appFiber := fiber.New(fiber.Config{
		AppName: "BPS Data Mesh - Domain Ketenagakerjaan v1.0",
	})

	// Middleware: Logging & Crash Recovery
	appFiber.Use(logger.New())
	appFiber.Use(recover.New())

	// ROUTING (13 ENDPOINTS DATA MESH) ---
	api := appFiber.Group("/api/v1/domains/ketenagakerjaan")

	
	// A. DATA INGESTION & MONITORING (4 Endpoints)
	ingestion := api.Group("/submissions")
	{
		// 1. Ingestion Utama (Mendukung Multi-format & Skoring Biner 60+20)
		ingestion.Post("/", ketenagakerjaanHandler.IngestData)

		// 2. Cek Validasi Format & Status Terakhir
		ingestion.Get("/:nik/validation", func(c *fiber.Ctx) error {
			var result models.RekamKetenagakerjaan
			db.Where("nomor_induk_kependudukan = ?", c.Params("nik")).Order("version desc").First(&result)
			return c.JSON(fiber.Map{
				"nik":    c.Params("nik"),
				"status": result.AuditStatus, // PENDING / VALID / INVALID
				"schema": "DTSEN-NAKER-ACTIVE",
			})
		})

		// 3. Cek Laporan Kualitas & Skoring (Trust Score)
		ingestion.Get("/:nik/scoring", func(c *fiber.Ctx) error {
			var result models.RekamKetenagakerjaan
			if err := db.Where("nomor_induk_kependudukan = ?", c.Params("nik")).Order("version desc").First(&result).Error; err != nil {
				return c.Status(404).JSON(fiber.Map{"error": "Data tidak ditemukan"})
			}
			return c.JSON(fiber.Map{
				"nik":           result.NIK,
				"trust_score":   result.TrustScore,
				"is_walidata":   result.IsWaliData,
				"audit_status":  result.AuditStatus,
				"quality_label": "Kalkulasi: 60(Sistem) + 20(Sumber) + 20(Audit)",
			})
		})

		// 4. Cek Status Progres
		ingestion.Get("/:nik/progress", func(c *fiber.Ctx) error {
			var count int64
			db.Model(&models.RekamKetenagakerjaan{}).Where("nomor_induk_kependudukan = ?", c.Params("nik")).Count(&count)
			status := "NOT_FOUND"
			if count > 0 { status = "COMPLETED_IN_MESH" }
			return c.JSON(fiber.Map{"nik": c.Params("nik"), "status": status, "progress": "100%"})
		})
	}

	
	// B. METADATA & SCHEMA MANAGEMENT (3 Endpoints - DINAMIS)
	schemas := api.Group("/schemas")
	{
		// 5 & 7. Daftar & Revisi Skema (Mendukung Tambah/Kurang Kolom via JSON Definition)
		schemas.Post("/", ketenagakerjaanHandler.CreateSchemaHandler)
		schemas.Patch("/", ketenagakerjaanHandler.CreateSchemaHandler)

		// 6. Cek Detail Skema Aktif (Kiblat Aturan Metadata)
		schemas.Get("/latest", ketenagakerjaanHandler.GetLatestSchemaHandler)
	}

	
	// C. DATASET MAINTENANCE & DISCOVERY (3 Endpoints)
	datasets := api.Group("/datasets")
	{
		// 8. GET: Data Keseluruhan (Golden Record + Dynamic Field Selection)
		datasets.Get("/", func(c *fiber.Ctx) error {
			fields := c.Query("fields")
			var results []models.RekamKetenagakerjaan

			subQuery := db.Model(&models.RekamKetenagakerjaan{}).Select("MAX(id)").Group("nomor_induk_kependudukan")
			query := db.Where("id IN (?) AND is_deleted = ?", subQuery, false)

			if fields != "" {
				query = query.Select(strings.Split(fields, ","))
			}

			query.Find(&results)
			return c.JSON(results)
		})

		// 9. GET: Detail NIK (History/Golden Record + Dynamic Field Selection)
		datasets.Get("/:nik", func(c *fiber.Ctx) error {
			fields := c.Query("fields")
			var result models.RekamKetenagakerjaan

			query := db.Model(&models.RekamKetenagakerjaan{}).Where("nomor_induk_kependudukan = ?", c.Params("nik"))
			if fields != "" {
				query = query.Select(strings.Split(fields, ","))
			}

			if err := query.Order("version desc").First(&result).Error; err != nil {
				return c.Status(404).JSON(fiber.Map{"error": "NIK tidak ditemukan"})
			}
			return c.JSON(result)
		})

		// 10. PUT: Koreksi Nilai (SCD Type 2: Status Reset ke PENDING)
		datasets.Put("/:nik", func(c *fiber.Ctx) error {
			var oldData models.RekamKetenagakerjaan
			if err := db.Where("nomor_induk_kependudukan = ?", c.Params("nik")).Order("version desc").First(&oldData).Error; err != nil {
				return c.Status(404).JSON(fiber.Map{"error": "Data asli tidak ditemukan"})
			}

			newData := oldData
			if err := c.BodyParser(&newData); err != nil {
				return c.Status(400).JSON(fiber.Map{"error": "Payload tidak valid"})
			}

			// LOGIKA RESET: Jika data berubah, harus audit ulang (Score -20)
			newData.ID = 0 
			newData.Version = oldData.Version + 1
			newData.AuditStatus = "PENDING"
			newData.UpdatedAt = time.Now()
			
			// Skor kembali ke base (Sistem 60 + Sumber 20/0)
			if newData.IsWaliData { newData.TrustScore = 80.0 } else { newData.TrustScore = 60.0 }

			db.Create(&newData)
			return c.JSON(fiber.Map{"message": "Versi baru dibuat, status kembali PENDING", "version": newData.Version})
		})
	}

	
	// D. GOVERNANCE & LIFECYCLE (3 Endpoints - AUDIT REAL)
	governance := api.Group("/")
	{
		// 11. DELETE: Soft Delete
		governance.Delete("/datasets/:id", func(c *fiber.Ctx) error {
			db.Model(&models.RekamKetenagakerjaan{}).Where("id = ?", c.Params("id")).Update("is_deleted", true)
			return c.JSON(fiber.Map{"message": "Data dinonaktifkan (Soft Delete)"})
		})

		// 12. GET: Sampel Data Acak untuk Audit
		governance.Get("/audit/samples", func(c *fiber.Ctx) error {
			var samples []models.RekamKetenagakerjaan
			db.Where("audit_status = ?", "PENDING").Order("RANDOM()").Limit(5).Find(&samples)
			return c.JSON(samples)
		})

		// 13. POST: Keputusan Audit (Final 20 Poin)
		governance.Post("/audit/decision", func(c *fiber.Ctx) error {
			var input struct {
				NIK     string `json:"nik"`
				Verdict string `json:"verdict"` // VALID / INVALID
			}
			c.BodyParser(&input)

			bonus := 0.0
			if strings.ToUpper(input.Verdict) == "VALID" { bonus = 20.0 }

			err := db.Model(&models.RekamKetenagakerjaan{}).
				Where("nomor_induk_kependudukan = ? AND audit_status = ?", input.NIK, "PENDING").
				Updates(map[string]interface{}{
					"audit_status": strings.ToUpper(input.Verdict),
					"trust_score":  gorm.Expr("trust_score + ?", bonus),
				}).Error

			if err != nil { return c.Status(500).JSON(fiber.Map{"error": "Gagal update audit"}) }
			return c.JSON(fiber.Map{"message": "Audit selesai, skor diperbarui"})
		})
	}

	// 5. Run Server pada Port 8088
	fmt.Println("---------------------------------------------------------")
	fmt.Println(" BPS DATA MESH: DOMAIN KETENAGAKERJAAN RUNNING")
	fmt.Println(" Port: 8088")
	fmt.Println("---------------------------------------------------------")
	appFiber.Listen(":8088")
}