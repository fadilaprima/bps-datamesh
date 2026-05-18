package main

import (
	"fmt"
	"strings"
	"time"

	"kesejahteraan/database"
	"kesejahteraan/internal/app"
	"kesejahteraan/internal/handler"
	"kesejahteraan/models"
	"kesejahteraan/storage"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"gorm.io/gorm"
)

func main() {
	// 1. Inisialisasi Database
	db := database.InitDB()

	// AUTOMIGRATE: Pastikan struct RekamKesejahteraan yang dipanggil
	db.AutoMigrate(&models.Schema{}, &models.RekamKesejahteraan{})

	// 2. Inisialisasi Layer (Dependency Injection)
	kesejahteraanRepo := storage.KesejahteraanStorage{DB: db}
	kesejahteraanService := app.KesejahteraanService{Storage: kesejahteraanRepo}
	kesejahteraanHandler := handler.KesejahteraanHandler{Service: kesejahteraanService}

	// 3. Setup Fiber Framework
	appFiber := fiber.New(fiber.Config{
		AppName: "BPS Data Mesh - Domain Kesejahteraan v1.0",
	})

	appFiber.Use(logger.New())
	appFiber.Use(recover.New())

	// --- 4. ROUTING (Domain Kesejahteraan) ---
	api := appFiber.Group("/api/v1/domains/kesejahteraan")

	// A. DATA INGESTION & MONITORING (Fokus NoKK)
	ingestion := api.Group("/submissions")
	{
		ingestion.Post("/", kesejahteraanHandler.IngestData)

		// Cek Validasi berdasarkan Nomor KK
		ingestion.Get("/:nokk/validation", func(c *fiber.Ctx) error {
			var result models.RekamKesejahteraan
			db.Where("nomor_kartu_keluarga = ?", c.Params("nokk")).Order("version desc").First(&result)
			return c.JSON(fiber.Map{
				"nomor_kartu_keluarga": c.Params("nokk"),
				"status":               result.AuditStatus,
				"schema":               "DTSEN-KESJ-ACTIVE",
			})
		})

		ingestion.Get("/:nokk/scoring", func(c *fiber.Ctx) error {
			var result models.RekamKesejahteraan
			if err := db.Where("nomor_kartu_keluarga = ?", c.Params("nokk")).Order("version desc").First(&result).Error; err != nil {
				return c.Status(404).JSON(fiber.Map{"error": "Data tidak ditemukan"})
			}
			return c.JSON(fiber.Map{
				"no_kk":         result.NoKK,
				"trust_score":   result.TrustScore,
				"audit_status":  result.AuditStatus,
				"quality_label": "Kalkulasi: 60(Sistem) + 20(Sumber) + 20(Audit)",
			})
		})
	}

	// B. METADATA & SCHEMA MANAGEMENT
	schemas := api.Group("/schemas")
	{
		schemas.Post("/", kesejahteraanHandler.CreateSchemaHandler)
		schemas.Patch("/", kesejahteraanHandler.CreateSchemaHandler)
		schemas.Get("/latest", kesejahteraanHandler.GetLatestSchemaHandler)
	}

	// C. DATASET MAINTENANCE (Fokus NoKK & Golden Record)
	datasets := api.Group("/datasets")
	{
		datasets.Get("/", func(c *fiber.Ctx) error {
			fields := c.Query("fields")
			var results []models.RekamKesejahteraan
			// Ambil versi terbaru per NoKK (Golden Record)
			subQuery := db.Model(&models.RekamKesejahteraan{}).Select("MAX(id)").Group("nomor_kartu_keluarga")
			query := db.Where("id IN (?) AND is_deleted = ?", subQuery, false)
			if fields != "" {
				query = query.Select(strings.Split(fields, ","))
			}
			query.Find(&results)
			return c.JSON(results)
		})

		datasets.Get("/:nokk", func(c *fiber.Ctx) error {
			fields := c.Query("fields")
			var result models.RekamKesejahteraan
			query := db.Model(&models.RekamKesejahteraan{}).Where("nomor_kartu_keluarga = ?", c.Params("nokk"))
			if fields != "" {
				query = query.Select(strings.Split(fields, ","))
			}
			if err := query.Order("version desc").First(&result).Error; err != nil {
				return c.Status(404).JSON(fiber.Map{"error": "Nomor KK tidak ditemukan"})
			}
			return c.JSON(result)
		})

		datasets.Put("/:nokk", func(c *fiber.Ctx) error {
			var oldData models.RekamKesejahteraan
			if err := db.Where("nomor_kartu_keluarga = ?", c.Params("nokk")).Order("version desc").First(&oldData).Error; err != nil {
				return c.Status(404).JSON(fiber.Map{"error": "Data KK tidak ditemukan"})
			}

			newData := oldData
			c.BodyParser(&newData)
			newData.ID = 0
			newData.Version = oldData.Version + 1
			newData.AuditStatus = "PENDING"
			newData.UpdatedAt = time.Now()

			db.Create(&newData)
			return c.JSON(fiber.Map{"message": "Versi baru dibuat", "version": newData.Version})
		})
	}

	// D. GOVERNANCE & AUDIT (Port 8085)
	governance := api.Group("/")
	{
		governance.Delete("/datasets/:id", func(c *fiber.Ctx) error {
			db.Model(&models.RekamKesejahteraan{}).Where("id = ?", c.Params("id")).Update("is_deleted", true)
			return c.JSON(fiber.Map{"message": "Data diarsipkan"})
		})

		
		governance.Get("/audit/samples", func(c *fiber.Ctx) error {
			var results []models.RekamKesejahteraan

			query := `
				SELECT k.* FROM rekam_kesejahteraan k
				INNER JOIN (
					SELECT nomor_kartu_keluarga, MAX(version) as max_ver
					FROM rekam_kesejahteraan
					GROUP BY nomor_kartu_keluarga
				) grouped_k 
				ON k.nomor_kartu_keluarga = grouped_k.nomor_kartu_keluarga 
				AND k.version = grouped_k.max_ver
				WHERE k.audit_status = 'PENDING'
			`

			if err := db.Raw(query).Scan(&results).Error; err != nil {
				return c.Status(500).JSON(fiber.Map{"error": "Gagal mengambil data audit kesejahteraan"})
			}

			if len(results) == 0 {
				return c.JSON(fiber.Map{"message": "Tidak ada data kesejahteraan terbaru yang perlu diaudit."})
			}

			return c.JSON(results)
		})

		
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
			err := db.Model(&models.RekamKesejahteraan{}).
				Where("nomor_kartu_keluarga IN ? AND audit_status = ?", input.NoKK, "PENDING").
				Updates(map[string]interface{}{
					"audit_status": verdictText,
					"trust_score":  gorm.Expr("trust_score + ?", bonus),
				}).Error

			if err != nil {
				return c.Status(500).JSON(fiber.Map{"error": "Gagal update audit kesejahteraan massal"})
			}

			pesan := fmt.Sprintf("Audit kesejahteraan selesai. %d KK telah diubah statusnya menjadi %s", len(input.NoKK), verdictText)
			return c.JSON(fiber.Map{"message": pesan})
		})
	}
	
	// 5. Run Server pada Port 8085
	fmt.Println("---------------------------------------------------------")
	fmt.Println(" BPS DATA MESH: DOMAIN KESEJAHTERAAN RUNNING")
	fmt.Println(" Port: 8085")
	fmt.Println("---------------------------------------------------------")
	appFiber.Listen(":8085")
}
