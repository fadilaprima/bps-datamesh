package main

import (
	"fmt"
	"hunian/database"
	"hunian/internal/app"
	"hunian/internal/handler"
	"hunian/models"
	"hunian/storage"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/fiber/v2/middleware/cors"
)

func main() {
	// 1. Inisialisasi Koneksi Database (Domain Hunian)
	db := database.InitDB()

	// AUTOMIGRATE: Sinkronisasi tabel Metadata (Schema) dan Data (RekamHunian)
	db.AutoMigrate(&models.Schema{}, &models.RekamHunian{})

	// 2. Inisialisasi Layer Architecture (Dependency Injection)
	hunianRepo := storage.HunianStorage{DB: db}
	hunianService := app.HunianService{Storage: hunianRepo}
	hunianHandler := handler.HunianHandler{Service: hunianService}

	// 3. Setup Fiber Framework
	appFiber := fiber.New(fiber.Config{
		AppName: "BPS Data Mesh - Domain Hunian v1.0",
	})

	appFiber.Use(logger.New())
	appFiber.Use(recover.New())

	// 4. ROUTING (13 ENDPOINTS DATA MESH)
	api := appFiber.Group("/api/v1/domains/hunian")

	// A. DATA INGESTION & MONITORING (4 Endpoints)
	ingestion := api.Group("/submissions")
	{
		ingestion.Post("/", hunianHandler.IngestData)
		ingestion.Get("/:nokk/validation", hunianHandler.GetValidationStatus)
		ingestion.Get("/:nokk/scoring", hunianHandler.GetScoring)
		ingestion.Get("/:nokk/progress", hunianHandler.GetProgress)
	}

	// B. METADATA & SCHEMA MANAGEMENT (3 Endpoints)
	schemas := api.Group("/schemas")
	{
		schemas.Post("/", hunianHandler.CreateSchemaHandler)
		schemas.Patch("/", hunianHandler.CreateSchemaHandler)
		schemas.Get("/latest", hunianHandler.GetLatestSchemaHandler)
	}

	// C. DATASET MAINTENANCE & DISCOVERY (3 Endpoints)
	datasets := api.Group("/datasets")
	{
		datasets.Get("/", hunianHandler.GetAllDatasets)
		datasets.Get("/:nokk", hunianHandler.GetDatasetDetail)
		datasets.Put("/:nokk", hunianHandler.UpdateDataset)
	}

	// D. GOVERNANCE & LIFECYCLE (3 Endpoints)
	governance := api.Group("/")
	{
		governance.Delete("/datasets/:id", hunianHandler.SoftDeleteDataset)
		governance.Get("/audit/samples", hunianHandler.GetAuditSamples)
		governance.Post("/audit/decision", hunianHandler.SubmitAuditDecision)
	}

	// 5. Run Server pada Port 8087 (8086 sudah dipakai Kesejahteraan)
	fmt.Println("---------------------------------------------------------")
	fmt.Println(" BPS DATA MESH: DOMAIN HUNIAN RUNNING")
	fmt.Println(" Port: 8087")
	fmt.Println("---------------------------------------------------------")
	appFiber.Listen(":8087")
}