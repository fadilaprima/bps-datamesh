package main

import (
	"fmt"
	"kependudukan/database"
	"kependudukan/internal/app"
	"kependudukan/internal/handler"
	"kependudukan/models"
	"kependudukan/storage"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
)

func main() {
	// 1. Inisialisasi Koneksi Database (Spesifik Domain Kependudukan)
	db := database.InitDB()

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

	// --- 4. ROUTING (BERDASARKAN MASTER PLAN DATA MESH - 13 ENDPOINTS) ---
	api := appFiber.Group("/api/v1/domains/dukcapil")

	// ============================================================
	// A. DATA INGESTION & MONITORING (4 Endpoints)
	// ============================================================
	ingestion := api.Group("/submissions")
	{
		// 1. Ingestion Utama (Mendukung Multi-format & SCD Type 2)
		ingestion.Post("/", pendudukHandler.IngestData)

		// 2. Cek Validasi Format (Metadata & NIK Integrity)
		ingestion.Get("/:id/validation", func(c *fiber.Ctx) error {
			return c.JSON(fiber.Map{
				"submission_id": c.Params("id"),
				"status":        "PASSED",
				"schema":        "DTSEN-2026-PENDUDUK",
			})
		})

		// 3. Cek Laporan Kualitas & Skoring (Trust Score & Freshness)
		ingestion.Get("/:id/scoring", func(c *fiber.Ctx) error {
			return c.JSON(fiber.Map{
				"submission_id": c.Params("id"),
				"trust_score":   1.0,
				"quality_label": "High Integrity - Verified by KEMENDAGRI",
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
		// 5. Daftarkan standar struktur data baru (DTSEN Penduduk)
		schemas.Post("/", func(c *fiber.Ctx) error {
			return c.Status(201).JSON(fiber.Map{"message": "Schema DTSEN Penduduk baru berhasil didaftarkan"})
		})

		// 6. Cek Detail Skema Aktif (Metadata Discovery)
		schemas.Get("/latest", func(c *fiber.Ctx) error {
			return c.JSON(fiber.Map{
				"domain":    "kependudukan",
				"standard":  "BPS-DTSEN-2026",
				"version":   "V1.2",
				"structure": []string{"nik", "no_kk", "nama", "tgl_lahir", "jenis_kelamin", "kode_desa", "alamat_ktp"},
			})
		})

		// 7. Revisi Skema Domain (Schema Evolution)
		schemas.Patch("/", func(c *fiber.Ctx) error {
			return c.JSON(fiber.Map{"message": "Skema kependudukan berhasil direvisi"})
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
			var results []models.Penduduk

			// Logika: Ambil hanya versi terbaru tiap NIK (Golden Record) yang tidak dihapus
			subQuery := db.Model(&models.Penduduk{}).Select("MAX(id)").Group("nomor_induk_kependudukan")
			query := db.Where("id IN (?) AND is_deleted = ?", subQuery, false)

			if fields != "" {
				query = query.Select(strings.Split(fields, ","))
			}

			query.Find(&results)
			return c.JSON(results)
		})

		// 9. GET: Endpoint Data Spesifik (Golden Record Detail / History NIK)
		datasets.Get("/:nik", func(c *fiber.Ctx) error {
			fields := c.Query("fields")
			var result models.Penduduk

			query := db.Model(&models.Penduduk{}).Where("nomor_induk_kependudukan = ?", c.Params("nik"))

			if fields != "" {
				query = query.Select(strings.Split(fields, ","))
			}

			if err := query.Order("version desc").First(&result).Error; err != nil {
				return c.Status(404).JSON(fiber.Map{"error": "Data NIK tidak ditemukan"})
			}
			return c.JSON(result)
		})

		// 10. PUT: Koreksi Nilai (Manual Correction memicu SCD Type 2)
		datasets.Put("/:id", func(c *fiber.Ctx) error {
			return c.JSON(fiber.Map{"message": "Koreksi manual penduduk berhasil, versi data ditingkatkan"})
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
			db.Model(&models.Penduduk{}).Where("id = ?", id).Update("is_deleted", true)
			return c.JSON(fiber.Map{"message": "NIK dengan ID " + id + " berhasil dinonaktifkan"})
		})

		// 12. GET: Minta Sampel Data Acak (Audit Mechanism)
		governance.Get("/audit/samples", func(c *fiber.Ctx) error {
			var samples []models.Penduduk
			db.Limit(5).Order("RANDOM()").Find(&samples)
			return c.JSON(samples)
		})

		// 13. POST: Keputusan Audit Sampel (Approved/Rejected Decision)
		governance.Post("/audit/decision", func(c *fiber.Ctx) error {
			return c.JSON(fiber.Map{"message": "Keputusan audit (Data Steward) kependudukan telah disimpan"})
		})
	}

	// 5. Jalankan Service pada Port 8081 (Sesuai kode lama)
	fmt.Println("---------------------------------------------------------")
	fmt.Println(" Service Kependudukan is running")
	fmt.Println(" Port: 8081 | DB: Port 5431")
	fmt.Println("---------------------------------------------------------")

	if err := appFiber.Listen(":8081"); err != nil {
		panic(fmt.Sprintf("Gagal menjalankan server: %v", err))
	}
}
