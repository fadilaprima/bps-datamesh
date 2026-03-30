package handler

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"pendidikan/internal/app"
	"pendidikan/models"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type PendidikanHandler struct {
	Service app.PendidikanService
}

// ============================================================
// B. METADATA & SCHEMA MANAGEMENT (DINAMIS)
// ============================================================

func (h *PendidikanHandler) CreateSchemaHandler(c *fiber.Ctx) error {
	var input models.Schema
	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Payload JSON tidak valid"})
	}

	domain := c.Params("domain", "pendidikan")

	// Archive skema lama
	h.Service.Storage.DB.Model(&models.Schema{}).
		Where("domain = ? AND status = ?", domain, "ACTIVE").
		Update("status", "ARCHIVED")

	var lastSchema models.Schema
	h.Service.Storage.DB.Where("domain = ?", domain).Order("version desc").First(&lastSchema)

	input.ID = uuid.New()
	input.Domain = domain
	input.Version = lastSchema.Version + 1
	input.Status = "ACTIVE"
	input.CreatedAt = time.Now()

	if err := h.Service.Storage.DB.Create(&input).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Gagal simpan skema"})
	}
	return c.Status(201).JSON(input)
}

func (h *PendidikanHandler) GetLatestSchemaHandler(c *fiber.Ctx) error {
	var schema models.Schema
	domain := c.Params("domain", "pendidikan")
	if err := h.Service.Storage.DB.Where("domain = ? AND status = ?", domain, "ACTIVE").Order("version desc").First(&schema).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Skema aktif tidak ditemukan"})
	}
	return c.JSON(schema)
}

// ============================================================
// A. DATA INGESTION (HYBRID DYNAMIC - WITH ADDITIONAL INFO)
// ============================================================

func (h *PendidikanHandler) IngestData(c *fiber.Ctx) error {
	// 1. Ambil Skema Aktif sebagai Kiblat Aturan
	var activeSchema models.Schema
	if err := h.Service.Storage.DB.Where("domain = ? AND status = ?", "pendidikan", "ACTIVE").Order("version desc").First(&activeSchema).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Metadata belum siap, ingest ditolak"})
	}

	fileHeader, err := c.FormFile("document")
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "File tidak ditemukan"})
	}

	// 2. Skoring 60+20 (Sistem + Walidata)
	sourceID := strings.ToUpper(c.FormValue("source_id", "UNKNOWN"))
	isWali := false
	trustScore := 60.0
	if reg, exists := app.PendidikanSourceRegistry[sourceID]; exists {
		isWali = reg.IsWali
		if isWali { trustScore += 20.0 }
	}

	refDate, _ := time.Parse("2006-01-02", c.FormValue("reference_date", time.Now().Format("2006-01-02")))
	file, _ := fileHeader.Open()
	defer file.Close()
	
	var dataList []models.RiwayatPendidikan
	filename := strings.ToLower(fileHeader.Filename)

	// 3. PARSING LOGIC (CSV AUTO-DETECTION FOR ADDITIONAL INFO)
	if strings.HasSuffix(filename, ".csv") {
		r := csv.NewReader(file)
		records, _ := r.ReadAll()
		if len(records) < 2 { return c.Status(400).JSON(fiber.Map{"error": "CSV kosong"}) }
		
		headers := records[0]
		for i, rec := range records {
			if i == 0 { continue }
			
			// Misahkan kolom utama dan kolom tambahan (AdditionalInfo)
			extraData := make(map[string]interface{})
			p := models.RiwayatPendidikan{
				SourceID: sourceID, IsWaliData: isWali, TrustScore: trustScore,
				ReferenceDate: refDate, AuditStatus: "PENDING",
			}

			for idx, val := range rec {
				key := strings.ToLower(headers[idx])
				switch key {
				case "nik", "nomor_induk_kependudukan": p.NIK = val
				case "partisipasi", "partisipasi_sekolah": p.Partisipasi = val
				case "jenjang", "jenjang_tertinggi": p.Jenjang = val
				case "kelas", "kelas_tertinggi": p.Kelas = val
				case "ijazah", "ijazah_tertinggi": p.Ijazah = val
				default:
					// Kalau gak ada di kolom utama, masukin kantong ajaib!
					extraData[key] = val
				}
			}
			p.AdditionalInfo, _ = json.Marshal(extraData)
			dataList = append(dataList, p)
		}
	} else {
		// Untuk JSON/Parquet, mapping langsung ke struct (AdditionalInfo tetap didukung jika ada di JSON)
		body, _ := io.ReadAll(file)
		json.Unmarshal(body, &dataList)
	}

	// 4. VALIDASI & PROSES (SCD TYPE 2)
	success, fail := 0, 0
	var errorLogs []string

	for _, p := range dataList {
		// Validasi Konten (NIK 16, dll) berdasarkan Aturan Skema di DB
		if ok, msg := h.Service.ValidatePendidikanMetadata(p, activeSchema.Definition); !ok {
			fail++
			errorLogs = append(errorLogs, fmt.Sprintf("NIK %s: %s", p.NIK, msg))
			continue
		}

		if _, err := h.Service.ProcessIngestion(p); err != nil {
			fail++
			errorLogs = append(errorLogs, fmt.Sprintf("NIK %s: %v", p.NIK, err))
		} else {
			success++
		}
	}

	return c.JSON(fiber.Map{
		"status": "Finished",
		"schema": activeSchema.Name + " v" + fmt.Sprint(activeSchema.Version),
		"stats":  fiber.Map{"total": len(dataList), "success": success, "fail": fail},
		"errors": errorLogs,
	})
}