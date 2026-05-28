package main

import (
	"fmt"
	"strings"
	"time"

	"kependudukan/database"
	"kependudukan/internal/app"
	"kependudukan/internal/handler"
	"kependudukan/models"
	"kependudukan/storage"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"gorm.io/gorm"
)

func main() {
	// 1. Inisialisasi Koneksi Database (Domain Kependudukan)
	db := database.InitDB()

	// AUTOMIGRATE: Sinkronisasi tabel Metadata (Schema) dan Data (Penduduk)
	db.AutoMigrate(&models.Schema{}, &models.Penduduk{})

	// 2. Inisialisasi Layer Architecture (Dependency Injection)
	pendudukRepo := storage.PendudukStorage{DB: db}
	pendudukService := app.PendudukService{Storage: pendudukRepo}
	pendudukHandler := handler.PendudukHandler{Service: pendudukService}

	// 3. Setup Fiber Framework
	appFiber := fiber.New(fiber.Config{
		AppName: "BPS Data Mesh - Domain Kependudukan v1.0",
	})

	// Middleware: Logging & Crash Recovery
	appFiber.Use(logger.New())
	appFiber.Use(recover.New())

	// 4. ROUTING (13 ENDPOINTS DATA MESH - IDENTIK PENDIDIKAN) 
	api := appFiber.Group("/api/v1/domains/penduduk")

	
	// A. DATA INGESTION & MONITORING (4 Endpoints)
	ingestion := api.Group("/submissions")
	{
		// 1. Ingestion Utama (Mendukung Multi-format CSV/JSON/Parquet & SCD Type 2)
		ingestion.Post("/", pendudukHandler.IngestData)

		// 2. Cek Validasi Format & Status Terakhir
		ingestion.Get("/:nik/validation", func(c *fiber.Ctx) error {
			var result models.Penduduk
			db.Where("nomor_induk_kependudukan = ?", c.Params("nik")).Order("version desc").First(&result)
			return c.JSON(fiber.Map{
				"nik":    c.Params("nik"),
				"status": result.AuditStatus, 
				"schema": "DTSEN-PENDDK-ACTIVE",
			})
		})

		// 3. Cek Laporan Kualitas & Skoring (Trust Score Real-time)
		ingestion.Get("/:nik/scoring", func(c *fiber.Ctx) error {
			var result models.Penduduk
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
			db.Model(&models.Penduduk{}).Where("nomor_induk_kependudukan = ?", c.Params("nik")).Count(&count)
			status := "NOT_FOUND"
			if count > 0 { status = "COMPLETED_IN_MESH" }
			return c.JSON(fiber.Map{"nik": c.Params("nik"), "status": status, "progress": "100%"})
		})
	}

	
	// B. METADATA & SCHEMA MANAGEMENT (3 Endpoints - DINAMIS)
	schemas := api.Group("/schemas")
	{
		// 5 & 7. Daftar & Revisi Skema (Mendukung Validasi Dinamis NIK/Wilayah)
		schemas.Post("/", pendudukHandler.CreateSchemaHandler)
		schemas.Patch("/", pendudukHandler.CreateSchemaHandler)

		// 6. Cek Detail Skema Aktif (Kiblat Aturan Metadata Penduduk)
		schemas.Get("/latest", pendudukHandler.GetLatestSchemaHandler)
	}

	
	// C. DATASET MAINTENANCE & DISCOVERY (3 Endpoints)
	datasets := api.Group("/datasets")
	{
		// 8. GET: Data Keseluruhan (Golden Record + Dynamic Field Selection)
		datasets.Get("/", func(c *fiber.Ctx) error {
			fields := c.Query("fields")
			var results []models.Penduduk

			subQuery := db.Model(&models.Penduduk{}).Select("MAX(id)").Group("nomor_induk_kependudukan")
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
			var result models.Penduduk

			query := db.Model(&models.Penduduk{}).Where("nomor_induk_kependudukan = ?", c.Params("nik"))
			if fields != "" {
				query = query.Select(strings.Split(fields, ","))
			}

			if err := query.Order("version desc").First(&result).Error; err != nil {
				return c.Status(404).JSON(fiber.Map{"error": "NIK tidak ditemukan"})
			}
			return c.JSON(result)
		})

		// 10. PUT: Koreksi Nilai (SCD Type 2: Atribut Fisik Tetap Terjaga)
		datasets.Put("/:nik", func(c *fiber.Ctx) error {
			var oldData models.Penduduk
			if err := db.Where("nomor_induk_kependudukan = ?", c.Params("nik")).Order("version desc").First(&oldData).Error; err != nil {
				return c.Status(404).JSON(fiber.Map{"error": "Data asli tidak ditemukan"})
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
			return c.JSON(fiber.Map{"message": "Versi baru penduduk dibuat, status kembali PENDING", "version": newData.Version})
		})
	}

	
	// D. GOVERNANCE & LIFECYCLE (3 Endpoints)
	governance := api.Group("/")
	{
		// 11. DELETE: Soft Delete (Sesuai Storage Penduduk)
		governance.Delete("/datasets/:id", func(c *fiber.Ctx) error {
			db.Model(&models.Penduduk{}).Where("id = ?", c.Params("id")).Update("is_deleted", true)
			return c.JSON(fiber.Map{"message": "Data penduduk berhasil dinonaktifkan (Soft Delete)"})
		})

		// 12. GET: Sampel Data Acak untuk Audit Kependudukan
		governance.Get("/audit/samples", func(c *fiber.Ctx) error {
			var results []models.Penduduk

			query := `
				SELECT k.* FROM Penduduks k
				INNER JOIN (
					SELECT nomor_induk_kependudukan, MAX(version) as max_ver
					FROM Penduduks
					GROUP BY nomor_induk_kependudukan
				) grouped_k 
				ON k.nomor_induk_kependudukan = grouped_k.nomor_induk_kependudukan
				AND k.version = grouped_k.max_ver
				WHERE k.audit_status = 'PENDING'
			`

			if err := db.Raw(query).Scan(&results).Error; err != nil {
				return c.Status(500).JSON(fiber.Map{"error": "Gagal mengambil data audit kependudukan"})
			}

			if len(results) == 0 {
				return c.JSON(fiber.Map{"message": "Tidak ada data kependudukan terbaru yang perlu diaudit."})
			}
			return c.JSON(results)
		})

		// 13. POST: Keputusan Audit (Final 20 Poin Trust Score)
		governance.Post("/audit/decision", func(c *fiber.Ctx) error {
			var input struct {
				NIK     []string `json:"nik"`
				Verdict int      `json:"verdict"`
			}

			if err := c.BodyParser(&input); err != nil {
				return c.Status(400).JSON(fiber.Map{"error": "Payload JSON tidak valid"})
			}
			if len(input.NIK) == 0 {
				return c.Status(400).JSON(fiber.Map{"error": "List NIK tidak boleh kosong"})
			}

			verdictText := "INVALID"
			bonus := 0.0

			if input.Verdict == 1 {
				verdictText = "VALID"
				bonus = 20.0
			} else if input.Verdict != 2 {
				return c.Status(400).JSON(fiber.Map{"error": "Verdict tidak valid. Gunakan 1 (VALID) atau 2 (INVALID)"})
			}

			// GORM Update berdasarkan Array NIK
			err := db.Model(&models.Penduduk{}).
				Where("nomor_induk_kependudukan IN ? AND audit_status = ?", input.NIK, "PENDING").
				Updates(map[string]interface{}{
					"audit_status": verdictText,
					"trust_score":  gorm.Expr("trust_score + ?", bonus),
					"updated_at":   time.Now(),
				}).Error

			if err != nil {
				return c.Status(500).JSON(fiber.Map{"error": "Gagal memproses keputusan audit"})
			}

			return c.JSON(fiber.Map{
				"message": fmt.Sprintf("Berhasil memproses %d NIK menjadi %s", len(input.NIK), verdictText),
			})
		})
	}

	// 5. Run Server pada Port 8081 (Sesuai Master Plan)
	fmt.Println("---------------------------------------------------------")
	fmt.Println(" BPS DATA MESH: DOMAIN KEPENDUDUKAN RUNNING")
	fmt.Println(" Port: 8081")
	fmt.Println("---------------------------------------------------------")
	appFiber.Listen(":8081")
}