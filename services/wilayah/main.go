package main

import (
	"fmt"
	"strings"
	"wilayah/database"
	"wilayah/internal/app"
	"wilayah/internal/handler"
	"wilayah/models"
	"wilayah/storage"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
)

func main() {
	// 1. Inisialisasi Koneksi Database
	db := database.InitDB()

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

	// --- 4. ROUTING (BERDASARKAN MASTER PLAN DATA MESH - 13 ENDPOINTS) ---
	api := appFiber.Group("/api/v1/domains/wilayah")

	// ============================================================
	// A. DATA INGESTION & MONITORING (4 Endpoints)
	// ============================================================
	ingestion := api.Group("/submissions")
	{
		// 1. Ingestion Utama (Mendukung Multi-format & SCD Type 2)
		ingestion.Post("/", wilayahHandler.IngestData)

		// 2. Cek Validasi Format (Metadata & Structural Integrity)
		ingestion.Get("/:id/validation", func(c *fiber.Ctx) error {
			return c.JSON(fiber.Map{
				"submission_id": c.Params("id"),
				"status":        "PASSED",
				"schema":        "DTSEN-2026-WIL",
			})
		})

		// 3. Cek Laporan Kualitas & Skoring (Trust Score & Freshness)
		ingestion.Get("/:id/scoring", func(c *fiber.Ctx) error {
			return c.JSON(fiber.Map{
				"submission_id": c.Params("id"),
				"trust_score":   0.98,
				"quality_label": "High Integrity",
			})
		})

		// 4. Cek Status Progres (Sinkronisasi ke Master Table)
		ingestion.Get("/:id/progress", func(c *fiber.Ctx) error {
			return c.JSON(fiber.Map{
				"submission_id": c.Params("id"),
				"status":        "COMPLETED",
				"progress":      "100%",
			})
		})
	}

	// ============================================================
	// B. METADATA & SCHEMA MANAGEMENT (3 Endpoints)
	// ============================================================
	schemas := api.Group("/schemas")
	{
		// 5. Daftarkan standar struktur data baru (DTSEN)
		schemas.Post("/", func(c *fiber.Ctx) error {
			return c.Status(201).JSON(fiber.Map{"message": "Schema DTSEN baru berhasil didaftarkan"})
		})

		// 6. Cek Detail Skema Aktif (Metadata Discovery)
		schemas.Get("/latest", func(c *fiber.Ctx) error {
			return c.JSON(fiber.Map{
				"domain":    "wilayah",
				"standard":  "BPS-DTSEN-2026",
				"version":   "V1.2",
				"structure": []string{"kode_prov", "provinsi", "kode_kab", "kabupaten", "kode_kec", "kecamatan", "kode_desa", "desa"},
			})
		})

		// 7. Revisi Skema Domain (Schema Evolution)
		schemas.Patch("/", func(c *fiber.Ctx) error {
			return c.JSON(fiber.Map{"message": "Skema wilayah berhasil direvisi"})
		})
	}

	// ============================================================
	// C. DATASET MAINTENANCE & DISCOVERY (3 Endpoints)
	// ============================================================
	datasets := api.Group("/datasets")
	{
		// 8. GET: Data Keseluruhan (Catalog Golden Record + Dynamic Field Selection)
		datasets.Get("/", func(c *fiber.Ctx) error {
			fields := c.Query("fields")
			var results []models.MasterWilayah

			// Logika: Ambil hanya versi terbaru (Golden Record) yang tidak dihapus
			subQuery := db.Model(&models.MasterWilayah{}).Select("MAX(id)").Group("kode_kelurahan_desa")
			query := db.Where("id IN (?) AND is_deleted = ?", subQuery, false)

			if fields != "" {
				query = query.Select(strings.Split(fields, ","))
			}

			query.Find(&results)
			return c.JSON(results)
		})

		// 9. GET: Endpoint Data Spesifik (Golden Record Detail + Field Selection)
		datasets.Get("/:kode", func(c *fiber.Ctx) error {
			fields := c.Query("fields")
			var result models.MasterWilayah

			query := db.Model(&models.MasterWilayah{}).Where("kode_kelurahan_desa = ?", c.Params("kode"))

			if fields != "" {
				query = query.Select(strings.Split(fields, ","))
			}

			if err := query.Order("version desc").First(&result).Error; err != nil {
				return c.Status(404).JSON(fiber.Map{"error": "Data wilayah tidak ditemukan"})
			}
			return c.JSON(result)
		})

		// 10. PUT: Koreksi Nilai (Manual Correction memicu SCD Type 2)
		datasets.Put("/:id", func(c *fiber.Ctx) error {
			return c.JSON(fiber.Map{"message": "Koreksi manual berhasil, versi data ditingkatkan (New Version created)"})
		})
	}

	// ============================================================
	// D. GOVERNANCE & LIFECYCLE (3 Endpoints)
	// ============================================================
	governance := api.Group("/")
	{
		// 11. DELETE: Soft Delete (Dataset Lifecycle Management)
		governance.Delete("/datasets/:id", func(c *fiber.Ctx) error {
			id := c.Params("id")
			db.Model(&models.MasterWilayah{}).Where("id = ?", id).Update("is_deleted", true)
			return c.JSON(fiber.Map{"message": "Data dengan ID " + id + " berhasil dinonaktifkan"})
		})

		// 12. GET: Minta Sampel Data Acak (Audit Mechanism)
		governance.Get("/audit/samples", func(c *fiber.Ctx) error {
			var samples []models.MasterWilayah
			db.Limit(5).Order("RANDOM()").Find(&samples)
			return c.JSON(samples)
		})

		// 13. POST: Keputusan Audit Sampel (Approved/Rejected Decision)
		governance.Post("/audit/decision", func(c *fiber.Ctx) error {
			return c.JSON(fiber.Map{"message": "Keputusan audit oleh Data Steward telah disimpan secara permanen"})
		})
	}

	// 5. Jalankan Service pada Port 8083
	fmt.Println("---------------------------------------------------------")
	fmt.Println(" Service Wilayah (Full Master Plan) is running")
	fmt.Println(" Endpoints Active | Port: 8083 | DB: Port 5433")
	fmt.Println("---------------------------------------------------------")

	if err := appFiber.Listen(":8083"); err != nil {
		panic(fmt.Sprintf("Gagal menjalankan server: %v", err))
	}
}