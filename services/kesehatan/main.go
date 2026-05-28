package main

import (
	"fmt"
	"kesehatan/database"
	"kesehatan/internal/app"
	"kesehatan/internal/handler"
	"kesehatan/models"
	"kesehatan/storage"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
)

func main() {
	// 1. Inisialisasi Koneksi Database (Domain kesehatan)
	db := database.InitDB()

	// AUTOMIGRATE: Pastikan nama struct benar (K-nya besar)
	db.AutoMigrate(&models.Schema{}, &models.RekamKesehatan{})

	// 2. Inisialisasi Layer Architecture (Dependency Injection)
	// Gunakan K-besar (Exported) agar bisa diakses antar folder
	kesehatanRepo := storage.KesehatanStorage{DB: db}
	kesehatanService := app.KesehatanService{Storage: kesehatanRepo}
	kesehatanHandler := handler.KesehatanHandler{Service: kesehatanService}

	// 3. Setup Fiber Framework
	appFiber := fiber.New(fiber.Config{
		AppName: "BPS Data Mesh - Domain Kesehatan v1.0",
	})

	appFiber.Use(logger.New())
	appFiber.Use(recover.New())

	// 4. ROUTING (13 ENDPOINTS DATA MESH)
	api := appFiber.Group("/api/v1/domains/kesehatan")

	// A. DATA INGESTION & MONITORING (4 Endpoints)
	ingestion := api.Group("/submissions")
	{
		ingestion.Post("/", kesehatanHandler.IngestData)
		ingestion.Get("/:nik/validation", kesehatanHandler.GetValidationStatus)
		ingestion.Get("/:nik/scoring", kesehatanHandler.GetScoring)
		ingestion.Get("/:nik/progress", kesehatanHandler.GetProgress)
	}

	// B. METADATA & SCHEMA MANAGEMENT (3 Endpoints)
	schemas := api.Group("/schemas")
	{
		schemas.Post("/", kesehatanHandler.CreateSchemaHandler)
		schemas.Patch("/", kesehatanHandler.CreateSchemaHandler)
		schemas.Get("/latest", kesehatanHandler.GetLatestSchemaHandler)
	}

	// C. DATASET MAINTENANCE & DISCOVERY (3 Endpoints)
	datasets := api.Group("/datasets")
	{
		datasets.Get("/", kesehatanHandler.GetAllDatasets)
		datasets.Get("/:nik", kesehatanHandler.GetDatasetDetail)
		datasets.Put("/:nik", kesehatanHandler.UpdateDataset)
	}

	// D. GOVERNANCE & LIFECYCLE (3 Endpoints)
	governance := api.Group("/")
	{
		governance.Delete("/datasets/:id", kesehatanHandler.SoftDeleteDataset)
		governance.Get("/audit/samples", kesehatanHandler.GetAuditSamples)
		governance.Post("/audit/decision", kesehatanHandler.SubmitAuditDecision)
	}

	// 5. Run Server pada Port 8084
	fmt.Println("---------------------------------------------------------")
	fmt.Println(" BPS DATA MESH: DOMAIN KESEHATAN RUNNING")
	fmt.Println(" Port: 8084")
	fmt.Println("---------------------------------------------------------")
	appFiber.Listen(":8084")
}