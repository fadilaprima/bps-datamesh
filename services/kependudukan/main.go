package main

import (
	"fmt"
	"kependudukan/database"
	"kependudukan/internal/app"
	"kependudukan/internal/handler"
	"kependudukan/models"
	"kependudukan/storage"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
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
		ingestion.Get("/:nik/validation", pendudukHandler.GetValidationStatus)
		// 3. Cek Laporan Kualitas & Skoring (Trust Score Real-time)
		ingestion.Get("/:nik/scoring", pendudukHandler.GetScoring)
		// 4. Cek Status Progres
		ingestion.Get("/:nik/progress", pendudukHandler.GetProgress)
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
		datasets.Get("/", pendudukHandler.GetAllDatasets)
		// 9. GET: Detail NIK (History/Golden Record + Dynamic Field Selection)
		datasets.Get("/:nik", pendudukHandler.GetDatasetDetail)
		// 10. PUT: Koreksi Nilai (SCD Type 2: Atribut Fisik Tetap Terjaga)
		datasets.Put("/:nik", pendudukHandler.UpdateDataset)
	}

	// D. GOVERNANCE & LIFECYCLE (3 Endpoints)
	governance := api.Group("/")
	{
		// 11. DELETE: Soft Delete (Sesuai Storage Penduduk)
		governance.Delete("/datasets/:id", pendudukHandler.SoftDeleteDataset)
		// 12. GET: Sampel Data Acak untuk Audit Kependudukan
		governance.Get("/audit/samples", pendudukHandler.GetAuditSamples)
		// 13. POST: Keputusan Audit (Final 20 Poin Trust Score)
		governance.Post("/audit/decision", pendudukHandler.SubmitAuditDecision)
	}

	// 5. Run Server pada Port 8081 (Sesuai Master Plan)
	fmt.Println("---------------------------------------------------------")
	fmt.Println(" BPS DATA MESH: DOMAIN KEPENDUDUKAN RUNNING")
	fmt.Println(" Port: 8081")
	fmt.Println("---------------------------------------------------------")
	appFiber.Listen(":8081")
}