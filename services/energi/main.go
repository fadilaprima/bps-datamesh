package main

import (
	"fmt"
	"energi/database"
	"energi/internal/app"
	"energi/internal/handler"
	"energi/models"
	"energi/storage"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/fiber/v2/middleware/cors"
)

func main() {
	// 1. Inisialisasi Koneksi Database (Domain Energi)
	db := database.InitDB()

	// AUTOMIGRATE: Sinkronisasi tabel Metadata (Schema) dan Data (RekamEnergi)
	db.AutoMigrate(&models.Schema{}, &models.RekamEnergi{})

	// 2. Inisialisasi Layer Architecture (Dependency Injection)
	energiRepo := storage.EnergiStorage{DB: db}
	energiService := app.EnergiService{Storage: energiRepo}
	energiHandler := handler.EnergiHandler{Service: energiService}

	// 3. Setup Fiber Framework
	appFiber := fiber.New(fiber.Config{
		AppName: "BPS Data Mesh - Domain Energi v1.0",
	})

	// Middleware: Logging & Crash Recovery
	appFiber.Use(logger.New())
	appFiber.Use(recover.New())

	// 4. ROUTING (13 ENDPOINTS DATA MESH) 
	api := appFiber.Group("/api/v1/domains/energi")

	// A. DATA INGESTION & MONITORING (4 Endpoints)
	ingestion := api.Group("/submissions")
	{
		ingestion.Post("/", energiHandler.IngestData)
		ingestion.Get("/:nokk/validation", energiHandler.GetValidationStatus)
		ingestion.Get("/:nokk/scoring", energiHandler.GetScoring)
		ingestion.Get("/:nokk/progress", energiHandler.GetProgress)
	}

	// B. METADATA & SCHEMA MANAGEMENT (3 Endpoints)
	schemas := api.Group("/schemas")
	{
		schemas.Post("/", energiHandler.CreateSchemaHandler)
		schemas.Patch("/", energiHandler.CreateSchemaHandler)
		schemas.Get("/latest", energiHandler.GetLatestSchemaHandler)
	}

	// C. DATASET MAINTENANCE & DISCOVERY (3 Endpoints)
	datasets := api.Group("/datasets")
	{
		datasets.Get("/", energiHandler.GetAllDatasets)
		datasets.Get("/:nokk", energiHandler.GetDatasetDetail)
		datasets.Put("/:nokk", energiHandler.UpdateDataset)
	}

	// D. GOVERNANCE & LIFECYCLE (3 Endpoints)
	governance := api.Group("/")
	{
		governance.Delete("/datasets/:id", energiHandler.SoftDeleteDataset)
		governance.Get("/audit/samples", energiHandler.GetAuditSamples)
		governance.Post("/audit/decision", energiHandler.SubmitAuditDecision)
	}

	// 5. Run Server pada Port 8088 (8087 sudah dipakai Hunian)
	fmt.Println("---------------------------------------------------------")
	fmt.Println(" BPS DATA MESH: DOMAIN ENERGI RUNNING")
	fmt.Println(" Port: 8088")
	fmt.Println("---------------------------------------------------------")
	appFiber.Listen(":8088")
}