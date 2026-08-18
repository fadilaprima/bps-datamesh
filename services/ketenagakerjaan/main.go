package main

import (
	"fmt"
	"ketenagakerjaan/database"
	"ketenagakerjaan/internal/app"
	"ketenagakerjaan/internal/handler"
	"ketenagakerjaan/models"
	"ketenagakerjaan/storage"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/fiber/v2/middleware/cors"
)

func main() {
	// 1. Inisialisasi Koneksi Database (Domain Ketenagakerjaan)
	db := database.InitDB()

	// AUTOMIGRATE: Sinkronisasi tabel Metadata (Schema) dan Data
	db.AutoMigrate(&models.Schema{}, &models.RekamKetenagakerjaan{})

	// 2. Inisialisasi Layer Architecture (Dependency Injection)
	ketenagakerjaanRepo := storage.KetenagakerjaanStorage{DB: db}
	ketenagakerjaanService := app.KetenagakerjaanService{Storage: ketenagakerjaanRepo}
	ketenagakerjaanHandler := handler.KetenagakerjaanHandler{Service: ketenagakerjaanService}

	// 3. Setup Fiber Framework
	appFiber := fiber.New(fiber.Config{
		AppName: "BPS Data Mesh - Domain Ketenagakerjaan v1.0",
	})

	appFiber.Use(logger.New())
	appFiber.Use(recover.New())
	appFiber.Use(cors.New())

	// 4. ROUTING (13 ENDPOINTS DATA MESH)
	api := appFiber.Group("/api/v1/domains/ketenagakerjaan")

	// A. DATA INGESTION & MONITORING (4 Endpoints)
	ingestion := api.Group("/submissions")
	{
		ingestion.Post("/", ketenagakerjaanHandler.IngestData)
		ingestion.Get("/:nik/validation", ketenagakerjaanHandler.GetValidationStatus)
		ingestion.Get("/:nik/scoring", ketenagakerjaanHandler.GetScoring)
		ingestion.Get("/:nik/progress", ketenagakerjaanHandler.GetProgress)
	}

	// B. METADATA & SCHEMA MANAGEMENT (3 Endpoints)
	schemas := api.Group("/schemas")
	{
		schemas.Post("/", ketenagakerjaanHandler.CreateSchemaHandler)
		schemas.Patch("/", ketenagakerjaanHandler.CreateSchemaHandler)
		schemas.Get("/latest", ketenagakerjaanHandler.GetLatestSchemaHandler)
	}

	// C. DATASET MAINTENANCE & DISCOVERY (3 Endpoints)
	datasets := api.Group("/datasets")
	{
		datasets.Get("/", ketenagakerjaanHandler.GetAllDatasets)
		datasets.Get("/:nik", ketenagakerjaanHandler.GetDatasetDetail)
		datasets.Put("/:nik", ketenagakerjaanHandler.UpdateDataset)
	}

	// D. GOVERNANCE & LIFECYCLE (3 Endpoints)
	governance := api.Group("/")
	{
		governance.Delete("/datasets/:id", ketenagakerjaanHandler.SoftDeleteDataset)
		governance.Get("/audit/samples", ketenagakerjaanHandler.GetAuditSamples)
		governance.Post("/audit/decision", ketenagakerjaanHandler.SubmitAuditDecision)
	}

	// 5. Run Server pada Port 8085
	fmt.Println("---------------------------------------------------------")
	fmt.Println(" BPS DATA MESH: DOMAIN KETENAGAKERJAAN RUNNING")
	fmt.Println(" Port: 8085")
	fmt.Println("---------------------------------------------------------")
	appFiber.Listen(":8085")
}
