package handler

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"pendidikan/internal/app"
	"pendidikan/models"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/xitongsys/parquet-go-source/local"
	"github.com/xitongsys/parquet-go/reader"
)

type PendidikanHandler struct {
	Service app.PendidikanService
}

// 1. METADATA & SCHEMA MANAGEMENT
func (h *PendidikanHandler) CreateSchemaHandler(c *fiber.Ctx) error {
	var input models.Schema
	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Payload JSON tidak valid"})
	}

	domain := c.Params("domain", "pendidikan")

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

// 2. DATA INGESTION (HYBRID DYNAMIC - WITH ADDITIONAL INFO)
func (h *PendidikanHandler) IngestData(c *fiber.Ctx) error {
	var activeSchema models.Schema
	if err := h.Service.Storage.DB.Where("domain = ? AND status = ?", "pendidikan", "ACTIVE").Order("version desc").First(&activeSchema).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Metadata belum siap, ingest ditolak"})
	}

	fileHeader, err := c.FormFile("document")
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "File tidak ditemukan"})
	}

	sourceIDStr := c.FormValue("source_id", "3") // Default: 3 (LAINNYA)
	sourceIDInt, _ := strconv.Atoi(sourceIDStr)

	sourceName := "LAINNYA"
	isWali := false
	trustScore := 60.0

	// Cocokkan angka dengan kamus di Service
	if config, exists := app.SourceMap[sourceIDInt]; exists {
		sourceName = config.Name
		isWali = config.IsWali
		if isWali {
			trustScore += 20.0
		}
	} else {
		return c.Status(400).JSON(fiber.Map{"error": "source_id tidak valid. Gunakan: 1 (BPS), 2 (KEMENDAGRI), 3 (LAINNYA)"})
	}

	refDate, _ := time.Parse("2006-01-02", c.FormValue("reference_date", time.Now().Format("2006-01-02")))
	file, _ := fileHeader.Open()
	defer file.Close()

	var dataList []models.RiwayatPendidikan
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
			w := models.RiwayatPendidikan{
				ReferenceDate: refDate,
			}

			for idx, val := range rec {
				key := strings.ToLower(headers[idx])
				switch key {
				case "nomor_induk_kependudukan":
					w.NIK = val
				case "partisipasi_sekolah":
					w.Partisipasi = val
				case "jenjang_tertinggi":
					w.Jenjang = val
				case "kelas_tertinggi":
					w.Kelas = val
				case "ijazah_tertinggi":
					w.Ijazah = val
				default:
					extraData[key] = val
				}
			}
			w.AdditionalInfo, _ = json.Marshal(extraData)
			dataList = append(dataList, w)
		}
	} else if strings.HasSuffix(filename, ".parquet") {
		//LOGIKA PARQUET
		tmpPath := "temp_pend_" + uuid.New().String() + ".parquet"
		fw, _ := os.Create(tmpPath)
		io.Copy(fw, file)
		fw.Close()

		fr, _ := local.NewLocalFileReader(tmpPath)
		pr, errP := reader.NewParquetReader(fr, new(models.RiwayatPendidikan), 4)
		if errP == nil {
			num := int(pr.GetNumRows())
			res := make([]models.RiwayatPendidikan, num)
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

	// 4. VALIDASI & PROSES (SCD TYPE 2)
	success, fail := 0, 0
	var errorLogs []string

	for i := range dataList {
		// Paksa masuk aturan Governance
		dataList[i].SourceID = sourceName
		dataList[i].IsWaliData = isWali
		dataList[i].AuditStatus = "PENDING"
		dataList[i].SchemaVersion = fmt.Sprintf("v%d", activeSchema.Version)

		if dataList[i].TrustScore == 0 {
			dataList[i].TrustScore = trustScore
		}

		// Validasi Metadata
		if ok, msg := h.Service.ValidatePendidikanMetadata(dataList[i], activeSchema.Definition); !ok {
			fail++
			errorLogs = append(errorLogs, fmt.Sprintf("NIK %s: %s", dataList[i].NIK, msg))
			continue
		}

		// Simpan ke DB
		if _, err := h.Service.ProcessIngestion(dataList[i]); err != nil {
			fail++
			errorLogs = append(errorLogs, fmt.Sprintf("NIK %s: %v", dataList[i].NIK, err))
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
