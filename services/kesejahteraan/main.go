package main

import (
	"fmt"
	"kesejahteraan/database"
	"kesejahteraan/internal/app"
	"kesejahteraan/internal/handler"
	"kesejahteraan/models"
	"kesejahteraan/storage"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
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
		ingestion.Get("/:nokk/validation", kesejahteraanHandler.GetValidationStatus)
		ingestion.Get("/:nokk/scoring", kesejahteraanHandler.GetScoring)
		ingestion.Get("/:nokk/progress", kesejahteraanHandler.GetProgress)
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
		datasets.Get("/", kesejahteraanHandler.GetAllDatasets)
		datasets.Get("/:nokk", kesejahteraanHandler.GetDatasetDetail)
		datasets.Put("/:nokk", kesejahteraanHandler.UpdateDataset)
	}

	// D. GOVERNANCE & AUDIT (Port 8086)
	governance := api.Group("/")
	{
		governance.Delete("/datasets/:id", kesejahteraanHandler.SoftDeleteDataset)
		governance.Get("/audit/samples", kesejahteraanHandler.GetAuditSamples)
		governance.Post("/audit/decision", kesejahteraanHandler.SubmitAuditDecision)
	}

	// 5. Run Server pada Port 8086 (Karena 8085 udah dipakai Ketenagakerjaan)
	fmt.Println("---------------------------------------------------------")
	fmt.Println(" BPS DATA MESH: DOMAIN KESEJAHTERAAN RUNNING")
	fmt.Println(" Port: 8086")
	fmt.Println("---------------------------------------------------------")
	appFiber.Listen(":8086")
}