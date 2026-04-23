package handler

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"energi/internal/app"
	"energi/models"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/xitongsys/parquet-go-source/local"
	"github.com/xitongsys/parquet-go/reader"
)

type EnergiHandler struct {
	Service app.EnergiService
}

// ============================================================
// B. METADATA & SCHEMA MANAGEMENT (IDENTIK)
// ============================================================

func (h *EnergiHandler) CreateSchemaHandler(c *fiber.Ctx) error {
	var input models.Schema
	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Payload JSON tidak valid"})
	}

	domain := c.Params("domain", "energi")

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
		return c.Status(500).JSON(fiber.Map{"error": "Gagal simpan skema energi"})
	}
	return c.Status(201).JSON(input)
}

func (h *EnergiHandler) GetLatestSchemaHandler(c *fiber.Ctx) error {
	var schema models.Schema
	domain := c.Params("domain", "energi")
	if err := h.Service.Storage.DB.Where("domain = ? AND status = ?", domain, "ACTIVE").Order("version desc").First(&schema).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Skema aktif tidak ditemukan"})
	}
	return c.JSON(schema)
}

// ============================================================
// A. DATA INGESTION (HYBRID DYNAMIC - VARIABEL ENERGI)
// ============================================================

func (h *EnergiHandler) IngestData(c *fiber.Ctx) error {
	var activeSchema models.Schema
	if err := h.Service.Storage.DB.Where("domain = ? AND status = ?", "energi", "ACTIVE").Order("version desc").First(&activeSchema).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Metadata belum siap, ingest ditolak"})
	}

	fileHeader, err := c.FormFile("document")
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "File tidak ditemukan"})
	}

	sourceID := strings.ToUpper(c.FormValue("source_id", "UNKNOWN"))
	isWali := false
	trustScore := 60.0
	if reg, exists := app.EnergiSourceRegistry[sourceID]; exists {
		isWali = reg.IsWali
		if isWali {
			trustScore += 20.0
		}
	}

	refDate, _ := time.Parse("2006-01-02", c.FormValue("reference_date", time.Now().Format("2006-01-02")))
	file, _ := fileHeader.Open()
	defer file.Close()

	var dataList []models.RekamEnergi
	filename := strings.ToLower(fileHeader.Filename)

	// 3. PARSING LOGIC (CSV, PARQUET, JSON)
	if strings.HasSuffix(filename, ".csv") {
		r := csv.NewReader(file)
		records, _ := r.ReadAll()
		if len(records) < 2 {
			return c.Status(400).JSON(fiber.Map{"error": "CSV kosong"})
		}

		headers := records[0]
		for i, rec := range records {
			if i == 0 {
				continue
			}

			extraData := make(map[string]interface{})
			p := models.RekamEnergi{
				SourceID: sourceID, IsWaliData: isWali, TrustScore: trustScore,
				ReferenceDate: refDate, AuditStatus: "PENDING",
			}

			for idx, val := range rec {
				key := strings.ToLower(headers[idx])
				switch key {
				case "no_kk", "nomor_kartu_keluarga":
					p.NoKK = val
				case "id_pelanggan_pln":
					p.IDPelangganPLN = val
				case "daya_terpasang":
					p.DayaTerpasang = val
				default:
					extraData[key] = val
				}
			}
			p.AdditionalInfo, _ = json.Marshal(extraData)
			dataList = append(dataList, p)
		}
	} else if strings.HasSuffix(filename, ".parquet") {
		tmpPath := "temp_enr_" + uuid.New().String() + ".parquet"
		fw, _ := os.Create(tmpPath)
		io.Copy(fw, file)
		fw.Close()

		fr, _ := local.NewLocalFileReader(tmpPath)
		pr, errP := reader.NewParquetReader(fr, new(models.RekamEnergi), 4)
		if errP == nil {
			num := int(pr.GetNumRows())
			res := make([]models.RekamEnergi, num)
			pr.Read(&res)
			pr.ReadStop()
			fr.Close()
			os.Remove(tmpPath)
			dataList = res
		}
	} else {
		body, _ := io.ReadAll(file)
		json.Unmarshal(body, &dataList)
	}

	// 4. VALIDASI & PROSES (Identik)
	success, fail := 0, 0
	var errorLogs []string

	for _, p := range dataList {
		if ok, msg := h.Service.ValidateEnergiMetadata(p, activeSchema.Definition); !ok {
			fail++
			errorLogs = append(errorLogs, fmt.Sprintf("NoKK %s: %s", p.NoKK, msg))
			continue
		}

		if _, err := h.Service.ProcessIngestion(p); err != nil {
			fail++
			errorLogs = append(errorLogs, fmt.Sprintf("NoKK %s: %v", p.NoKK, err))
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