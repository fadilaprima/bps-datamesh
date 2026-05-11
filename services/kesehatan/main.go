package main

import (
	"fmt"
	"strings"
	"time"

	"kesehatan/database"
	"kesehatan/internal/app"   
	"kesehatan/internal/handler" 
	"kesehatan/storage" 
	"kesehatan/models"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"gorm.io/gorm"
)

func main() {
	// 1. Inisialisasi Koneksi Database (Domain kesehatan)
	db := database.InitDB()

	// AUTOMIGRATE: Pastikan nama struct benar (K-nya besar)
	db.AutoMigrate(&models.Schema{}, &models.RekamKesehatan{})

	// 2. Inisialisasi Layer Architecture (Dependency Injection)
	// Gunakan K-besar (Exported) agar bisa diakses antar folder
	kesehatanRepo := storage.KesehatanStorage{DB: db}
	kesehatanService := app.KesehatanService{Storage: kesehatanRepo}
	kesehatanHandler := handler.KesehatanHandler{Service: kesehatanService}

	// 3. Setup Fiber Framework
	appFiber := fiber.New(fiber.Config{
		AppName: "BPS Data Mesh - Domain Kesehatan v1.0",
	})

	appFiber.Use(logger.New())
	appFiber.Use(recover.New())

	// 4. ROUTING (13 ENDPOINTS DATA MESH) 
	api := appFiber.Group("/api/v1/domains/kesehatan")

	
	// A. DATA INGESTION & MONITORING (4 Endpoints)
	ingestion := api.Group("/submissions")
	{
		ingestion.Post("/", kesehatanHandler.IngestData)

		ingestion.Get("/:nik/validation", func(c *fiber.Ctx) error {
			var result models.RekamKesehatan
			db.Where("nomor_induk_kependudukan = ?", c.Params("nik")).Order("version desc").First(&result)
			return c.JSON(fiber.Map{
				"nik":    c.Params("nik"),
				"status": result.AuditStatus,
				"schema": "DTSEN-KES-ACTIVE", // Ubah label ke KES (Kesehatan)
			})
		})

		ingestion.Get("/:nik/scoring", func(c *fiber.Ctx) error {
			var result models.RekamKesehatan
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

		ingestion.Get("/:nik/progress", func(c *fiber.Ctx) error {
			var count int64
			db.Model(&models.RekamKesehatan{}).Where("nomor_induk_kependudukan = ?", c.Params("nik")).Count(&count)
			status := "NOT_FOUND"
			if count > 0 { status = "COMPLETED_IN_MESH" }
			return c.JSON(fiber.Map{"nik": c.Params("nik"), "status": status, "progress": "100%"})
		})
	}

	
	// B. METADATA & SCHEMA MANAGEMENT (3 Endpoints)
	schemas := api.Group("/schemas")
	{
		schemas.Post("/", kesehatanHandler.CreateSchemaHandler)
		schemas.Patch("/", kesehatanHandler.CreateSchemaHandler)
		schemas.Get("/latest", kesehatanHandler.GetLatestSchemaHandler)
	}

	
	// C. DATASET MAINTENANCE & DISCOVERY (3 Endpoints)
	datasets := api.Group("/datasets")
	{
		datasets.Get("/", func(c *fiber.Ctx) error {
			fields := c.Query("fields")
			var results []models.RekamKesehatan

			subQuery := db.Model(&models.RekamKesehatan{}).Select("MAX(id)").Group("nomor_induk_kependudukan")
			query := db.Where("id IN (?) AND is_deleted = ?", subQuery, false)

			if fields != "" {
				query = query.Select(strings.Split(fields, ","))
			}

			query.Find(&results)
			return c.JSON(results)
		})

		datasets.Get("/:nik", func(c *fiber.Ctx) error {
			fields := c.Query("fields")
			var result models.RekamKesehatan

			query := db.Model(&models.RekamKesehatan{}).Where("nomor_induk_kependudukan = ?", c.Params("nik"))
			if fields != "" {
				query = query.Select(strings.Split(fields, ","))
			}

			if err := query.Order("version desc").First(&result).Error; err != nil {
				return c.Status(404).JSON(fiber.Map{"error": "NIK tidak ditemukan"})
			}
			return c.JSON(result)
		})

		datasets.Put("/:nik", func(c *fiber.Ctx) error {
			var oldData models.RekamKesehatan
			if err := db.Where("nomor_induk_kependudukan = ?", c.Params("nik")).Order("version desc").First(&oldData).Error; err != nil {
				return c.Status(404).JSON(fiber.Map{"error": "Data asli tidak ditemukan"})
			}

			newData := oldData
			if err := c.BodyParser(&newData); err != nil {
				return c.Status(400).JSON(fiber.Map{"error": "Payload tidak valid"})
			}

			newData.ID = 0
			newData.Version = oldData.Version + 1
			newData.AuditStatus = "PENDING"
			newData.UpdatedAt = time.Now()

			if newData.IsWaliData { newData.TrustScore = 80.0 } else { newData.TrustScore = 60.0 }

			db.Create(&newData)
			return c.JSON(fiber.Map{"message": "Versi baru dibuat, status kembali PENDING", "version": newData.Version})
		})
	}

	
	// D. GOVERNANCE & LIFECYCLE (3 Endpoints)
	governance := api.Group("/")
	{
		governance.Delete("/datasets/:id", func(c *fiber.Ctx) error {
			db.Model(&models.RekamKesehatan{}).Where("id = ?", c.Params("id")).Update("is_deleted", true)
			return c.JSON(fiber.Map{"message": "Data dinonaktifkan (Soft Delete)"})
		})

		governance.Get("/audit/samples", func(c *fiber.Ctx) error {
			var samples []models.RekamKesehatan
			db.Where("audit_status = ?", "PENDING").Order("RANDOM()").Limit(5).Find(&samples)
			return c.JSON(samples)
		})

		governance.Post("/audit/decision", func(c *fiber.Ctx) error {
			var input struct {
				NIK     string `json:"nik"`
				Verdict string `json:"verdict"`
			}
			c.BodyParser(&input)

			bonus := 0.0
			if strings.ToUpper(input.Verdict) == "VALID" { bonus = 20.0 }

			err := db.Model(&models.RekamKesehatan{}).
				Where("nomor_induk_kependudukan = ? AND audit_status = ?", input.NIK, "PENDING").
				Updates(map[string]interface{}{
					"audit_status": strings.ToUpper(input.Verdict),
					"trust_score":  gorm.Expr("trust_score + ?", bonus),
				}).Error

			if err != nil { return c.Status(500).JSON(fiber.Map{"error": "Gagal update audit"}) }
			return c.JSON(fiber.Map{"message": "Audit selesai, skor diperbarui"})
		})
	}

	// 5. Run Server pada Port 8084
	fmt.Println("---------------------------------------------------------")
	fmt.Println(" BPS DATA MESH: DOMAIN KESEHATAN RUNNING")
	fmt.Println(" Port: 8084")
	fmt.Println("---------------------------------------------------------")
	appFiber.Listen(":8084")
}