package main

import (
	"fmt"
	"pendidikan/database"
	"pendidikan/internal/app"
	"pendidikan/internal/handler"
	"pendidikan/models"
	"pendidikan/storage"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/fiber/v2/middleware/cors"
)

func main() {
	// 1. Inisialisasi Koneksi Database (Domain Pendidikan)
	db := database.InitDB()

	// AUTOMIGRATE: Sinkronisasi tabel Metadata (Schema) dan Data (RiwayatPendidikan)
	db.AutoMigrate(&models.Schema{}, &models.RiwayatPendidikan{})

	// 2. Inisialisasi Layer Architecture
	pendidikanRepo := storage.PendidikanStorage{DB: db}
	pendidikanService := app.PendidikanService{Storage: pendidikanRepo}
	pendidikanHandler := handler.PendidikanHandler{Service: pendidikanService}

	// 3. Setup Fiber Framework
	appFiber := fiber.New(fiber.Config{
		AppName: "BPS Data Mesh - Domain Pendidikan v1.0",
	})

	// Middleware: Logging & Crash Recovery
	appFiber.Use(logger.New())
	appFiber.Use(recover.New())
	appFiber.Use(cors.New())

	// 4. ROUTING (13 ENDPOINTS DATA MESH)
	api := appFiber.Group("/api/v1/domains/pendidikan")

	// A. DATA INGESTION & MONITORING (4 Endpoints)
	ingestion := api.Group("/submissions")
	{
		// 1. Ingestion Utama
		ingestion.Post("/", pendidikanHandler.IngestData)
		// 2. Cek Validasi Format & Status Terakhir
		ingestion.Get("/:nik/validation", pendidikanHandler.GetValidationStatus)
		// 3. Cek Laporan Kualitas & Skoring
		ingestion.Get("/:nik/scoring", pendidikanHandler.GetScoring)
		// 4. Cek Status Progres
		ingestion.Get("/:nik/progress", pendidikanHandler.GetProgress)
	}

	// B. METADATA & SCHEMA MANAGEMENT (3 Endpoints - DINAMIS)
	schemas := api.Group("/schemas")
	{
		// 5 & 7. Daftar & Revisi Skema (Mendukung Tambah/Kurang Kolom via JSON Definition)
		schemas.Post("/", pendidikanHandler.CreateSchemaHandler)
		schemas.Patch("/", pendidikanHandler.CreateSchemaHandler)
		// 6. Cek Detail Skema Aktif (Kiblat Aturan Metadata)
		schemas.Get("/latest", pendidikanHandler.GetLatestSchemaHandler)
	}

	// C. DATASET MAINTENANCE & DISCOVERY (3 Endpoints)
	datasets := api.Group("/datasets")
	{
		// 8. GET: Data Keseluruhan (Golden Record + Dynamic Field Selection)
		datasets.Get("/", pendidikanHandler.GetAllDatasets)
		// 9. GET: Detail NIK (History/Golden Record + Dynamic Field Selection)
		datasets.Get("/:nik", pendidikanHandler.GetDatasetDetail)
		// 10. PUT: Koreksi Nilai (SCD Type 2: Status Reset ke PENDING)
		datasets.Put("/:nik", pendidikanHandler.UpdateDataset)
	}

	// D. GOVERNANCE & LIFECYCLE (3 Endpoints)
	governance := api.Group("/")
	{
		// 11. DELETE: Soft Delete
		governance.Delete("/datasets/:id", pendidikanHandler.SoftDeleteDataset)
		// 12. GET: Sampel Data Acak untuk Audit (A. GET SAMPLES: Hanya ambil versi tertinggi yang PENDING)
		governance.Get("/audit/samples", pendidikanHandler.GetAuditSamples)
		// 13. POST: Keputusan Audit (Final 20 Poin) - B. POST DECISION: Bulk Update menggunakan Array NIK
		governance.Post("/audit/decision", pendidikanHandler.SubmitAuditDecision)
	}

	// 5. Run Server pada Port 8082
	fmt.Println("---------------------------------------------------------")
	fmt.Println(" BPS DATA MESH: DOMAIN PENDIDIKAN RUNNING")
	fmt.Println(" Port: 8082 ")
	fmt.Println("---------------------------------------------------------")
	appFiber.Listen(":8082")
}