package main

import (
	"fmt"
	"wilayah/database"
	"wilayah/internal/app"
	"wilayah/internal/handler"
	"wilayah/models"
	"wilayah/storage"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/fiber/v2/middleware/cors"
)

func main() {
	// 1. Inisialisasi Koneksi Database (Domain Wilayah)
	db := database.InitDB()

	// AUTOMIGRATE: Sinkronisasi tabel Metadata (Schema) dan Data (MasterWilayah)
	db.AutoMigrate(&models.Schema{}, &models.MasterWilayah{})

	// 2. Inisialisasi Layer Architecture Sesuai Otonomi Data Mesh
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

	// 4. ROUTING (13 ENDPOINTS DATA MESH - IDENTIK)
	api := appFiber.Group("/api/v1/domains/wilayah")

	// A. DATA INGESTION & MONITORING (4 Endpoints)
	ingestion := api.Group("/submissions")
	{
		// 1. Ingestion  (Mendukung Multi-format CSV/JSON/Parquet & SCD Type 2)
		ingestion.Post("/", wilayahHandler.IngestData)
		// 2. Cek Validasi Format & Status Terakhir
		ingestion.Get("/:kode/validation", wilayahHandler.GetValidationStatus)
		// 3. Cek Laporan Kualitas & Skoring
		ingestion.Get("/:kode/scoring", wilayahHandler.GetScoring)
		// 4. Cek Status Progres
		ingestion.Get("/:kode/progress", wilayahHandler.GetProgress)
	}

	// B. METADATA & SCHEMA MANAGEMENT (3 Endpoints - DINAMIS)
	schemas := api.Group("/schemas")
	{
		// 5 & 7. Daftar & Revisi Skema
		schemas.Post("/", wilayahHandler.CreateSchemaHandler)
		schemas.Patch("/", wilayahHandler.CreateSchemaHandler)
		// 6. Cek Detail Skema Aktif
		schemas.Get("/latest", wilayahHandler.GetLatestSchemaHandler)
	}

	// C. DATASET MAINTENANCE & DISCOVERY (3 Endpoints)
	datasets := api.Group("/datasets")
	{
		// 8. GET: Data Keseluruhan (Dinamis Field Selection)
		datasets.Get("/", wilayahHandler.GetAllDatasets)
		// 9. GET: Detail Kode Desa
		datasets.Get("/:kode", wilayahHandler.GetDatasetDetail)
		// 10. PUT: Koreksi Nilai
		datasets.Put("/:kode", wilayahHandler.UpdateDataset)
	}

	// D. GOVERNANCE & LIFECYCLE (3 Endpoints)
	governance := api.Group("/")
	{
		// 11. DELETE: Soft Delete
		governance.Delete("/datasets/:id", wilayahHandler.SoftDeleteDataset)
		// 12. GET: Ambil Sample Data untuk Diaudit ( Hanya Versi Tertinggi)
		governance.Get("/audit/samples", wilayahHandler.GetAuditSamples)
		// 13. POST: Keputusan Audit (Final 20 Poin Trust Score)
		governance.Post("/audit/decision", wilayahHandler.SubmitAuditDecision)
	}

	// 5. Run Server pada Port 8083
	fmt.Println("---------------------------------------------------------")
	fmt.Println(" BPS DATA MESH: DOMAIN WILAYAH RUNNING")
	fmt.Println(" Port: 8083 ")
	fmt.Println("---------------------------------------------------------")
	appFiber.Listen(":8083")
}
