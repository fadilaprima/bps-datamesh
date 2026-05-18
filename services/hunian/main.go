package main

import (
	"fmt"
	"strings"
	"time"

	"hunian/database"
	"hunian/internal/app"
	"hunian/internal/handler"
	"hunian/models"
	"hunian/storage"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"gorm.io/gorm"
)

func main() {
	// 1. Inisialisasi Koneksi Database (Domain Hunian)
	db := database.InitDB()

	// AUTOMIGRATE: Sinkronisasi tabel Metadata (Schema) dan Data (RekamHunian)
	db.AutoMigrate(&models.Schema{}, &models.RekamHunian{})

	// 2. Inisialisasi Layer Architecture (Dependency Injection)
	hunianRepo := storage.HunianStorage{DB: db}
	hunianService := app.HunianService{Storage: hunianRepo}
	hunianHandler := handler.HunianHandler{Service: hunianService}

	// 3. Setup Fiber Framework
	appFiber := fiber.New(fiber.Config{
		AppName: "BPS Data Mesh - Domain Hunian v1.0",
	})

	// Middleware: Logging & Crash Recovery
	appFiber.Use(logger.New())
	appFiber.Use(recover.New())

	// 4. ROUTING (13 ENDPOINTS DATA MESH)
	api := appFiber.Group("/api/v1/domains/hunian")

	// A. DATA INGESTION & MONITORING (4 Endpoints)
	ingestion := api.Group("/submissions")
	{
		// 1. Ingestion Utama
		ingestion.Post("/", hunianHandler.IngestData)

		// 2. Cek Validasi Format & Status Terakhir (Berdasarkan NoKK)
		ingestion.Get("/:nokk/validation", func(c *fiber.Ctx) error {
			var result models.RekamHunian
			db.Where("nomor_kartu_keluarga = ?", c.Params("nokk")).Order("version desc").First(&result)
			return c.JSON(fiber.Map{
				"nokk":   c.Params("nokk"),
				"status": result.AuditStatus,
				"schema": "DTSEN-HUN-ACTIVE",
			})
		})

		// 3. Cek Laporan Kualitas & Skoring
		ingestion.Get("/:nokk/scoring", func(c *fiber.Ctx) error {
			var result models.RekamHunian
			if err := db.Where("nomor_kartu_keluarga = ?", c.Params("nokk")).Order("version desc").First(&result).Error; err != nil {
				return c.Status(404).JSON(fiber.Map{"error": "Data hunian tidak ditemukan"})
			}
			return c.JSON(fiber.Map{
				"no_kk":         result.NoKK,
				"trust_score":   result.TrustScore,
				"is_walidata":   result.IsWaliData,
				"audit_status":  result.AuditStatus,
				"quality_label": "Kalkulasi: 60(Sistem) + 20(Sumber) + 20(Audit)",
			})
		})

		// 4. Cek Status Progres
		ingestion.Get("/:nokk/progress", func(c *fiber.Ctx) error {
			var count int64
			db.Model(&models.RekamHunian{}).Where("nomor_kartu_keluarga = ?", c.Params("nokk")).Count(&count)
			status := "NOT_FOUND"
			if count > 0 {
				status = "COMPLETED_IN_MESH"
			}
			return c.JSON(fiber.Map{"nokk": c.Params("nokk"), "status": status, "progress": "100%"})
		})
	}

	// B. METADATA & SCHEMA MANAGEMENT (3 Endpoints)
	schemas := api.Group("/schemas")
	{
		schemas.Post("/", hunianHandler.CreateSchemaHandler)
		schemas.Patch("/", hunianHandler.CreateSchemaHandler)
		schemas.Get("/latest", hunianHandler.GetLatestSchemaHandler)
	}

	// C. DATASET MAINTENANCE & DISCOVERY (3 Endpoints)
	datasets := api.Group("/datasets")
	{
		// 8. GET: Data Keseluruhan (Golden Record)
		datasets.Get("/", func(c *fiber.Ctx) error {
			fields := c.Query("fields")
			var results []models.RekamHunian

			subQuery := db.Model(&models.RekamHunian{}).Select("MAX(id)").Group("nomor_kartu_keluarga")
			query := db.Where("id IN (?) AND is_deleted = ?", subQuery, false)

			if fields != "" {
				query = query.Select(strings.Split(fields, ","))
			}

			query.Find(&results)
			return c.JSON(results)
		})

		// 9. GET: Detail NoKK
		datasets.Get("/:nokk", func(c *fiber.Ctx) error {
			fields := c.Query("fields")
			var result models.RekamHunian

			query := db.Model(&models.RekamHunian{}).Where("nomor_kartu_keluarga = ?", c.Params("nokk"))
			if fields != "" {
				query = query.Select(strings.Split(fields, ","))
			}

			if err := query.Order("version desc").First(&result).Error; err != nil {
				return c.Status(404).JSON(fiber.Map{"error": "Nomor KK tidak ditemukan"})
			}
			return c.JSON(result)
		})

		// 10. PUT: Koreksi Nilai (SCD Type 2)
		datasets.Put("/:nokk", func(c *fiber.Ctx) error {
			var oldData models.RekamHunian
			if err := db.Where("nomor_kartu_keluarga = ?", c.Params("nokk")).Order("version desc").First(&oldData).Error; err != nil {
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

			if newData.IsWaliData {
				newData.TrustScore = 80.0
			} else {
				newData.TrustScore = 60.0
			}

			db.Create(&newData)
			return c.JSON(fiber.Map{"message": "Versi baru hunian dibuat", "version": newData.Version})
		})
	}

	// D. GOVERNANCE & LIFECYCLE (3 Endpoints)
	governance := api.Group("/")
	{
		// 11. DELETE: Soft Delete
		governance.Delete("/datasets/:id", func(c *fiber.Ctx) error {
			db.Model(&models.RekamHunian{}).Where("id = ?", c.Params("id")).Update("is_deleted", true)
			return c.JSON(fiber.Map{"message": "Data hunian dinonaktifkan"})
		})

		// 12. GET: Ambil Sample Data untuk Diaudit (Hanya Versi Tertinggi)
		governance.Get("/audit/samples", func(c *fiber.Ctx) error {
			var results []models.RekamHunian
			query := `
				SELECT k.* FROM rekam_hunian k
				INNER JOIN (
					SELECT nomor_kartu_keluarga, MAX(version) as max_ver
					FROM rekam_hunian
					GROUP BY nomor_kartu_keluarga
				) grouped_k 
				ON k.nomor_kartu_keluarga = grouped_k.nomor_kartu_keluarga 
				AND k.version = grouped_k.max_ver
				WHERE k.audit_status = 'PENDING'
			`

			if err := db.Raw(query).Scan(&results).Error; err != nil {
				return c.Status(500).JSON(fiber.Map{"error": "Gagal mengambil data audit hunian"})
			}

			if len(results) == 0 {
				return c.JSON(fiber.Map{"message": "Tidak ada data hunian terbaru yang perlu diaudit."})
			}

			return c.JSON(results)
		})

		// 13. POST: Keputusan Audit (Final 20 Poin Trust Score)
		governance.Post("/audit/decision", func(c *fiber.Ctx) error {
			var input struct {
				NoKK    []string `json:"nomor_kartu_keluarga"`
				Verdict int      `json:"verdict"` // 1 = VALID, 2 = INVALID
			}

			if err := c.BodyParser(&input); err != nil {
				return c.Status(400).JSON(fiber.Map{"error": "Payload JSON tidak valid"})
			}

			if len(input.NoKK) == 0 {
				return c.Status(400).JSON(fiber.Map{"error": "Daftar nomor_kartu_keluarga tidak boleh kosong. Harus tahu pasti data mana yang diaudit."})
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

			// Update ke Database menggunakan teks hasil terjemahan secara Massal (Bulk Update)
			err := db.Model(&models.RekamHunian{}).
				Where("nomor_kartu_keluarga IN ? AND audit_status = ?", input.NoKK, "PENDING").
				Updates(map[string]interface{}{
					"audit_status": verdictText,
					"trust_score":  gorm.Expr("trust_score + ?", bonus),
				}).Error

			if err != nil {
				return c.Status(500).JSON(fiber.Map{"error": "Gagal update audit hunian massal"})
			}

			pesan := fmt.Sprintf("Audit hunian selesai. %d KK telah diubah statusnya menjadi %s", len(input.NoKK), verdictText)
			return c.JSON(fiber.Map{"message": pesan})
		})
	}

	// 5. Run Server pada Port 8086
	fmt.Println("---------------------------------------------------------")
	fmt.Println(" BPS DATA MESH: DOMAIN HUNIAN RUNNING")
	fmt.Println(" Port: 8086")
	fmt.Println("---------------------------------------------------------")
	appFiber.Listen(":8086")
}
