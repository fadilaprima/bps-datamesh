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

	"hunian/internal/app"
	"hunian/models"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/xitongsys/parquet-go-source/local"
	"github.com/xitongsys/parquet-go/reader"
)

type HunianHandler struct {
	Service app.HunianService
}

// A. METADATA & SCHEMA MANAGEMENT
func (h *HunianHandler) CreateSchemaHandler(c *fiber.Ctx) error {
	var input models.Schema
	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Payload JSON tidak valid"})
	}

	domain := c.Params("domain", "hunian")

	// Archive skema lama agar hanya satu yang aktif
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
		return c.Status(500).JSON(fiber.Map{"error": "Gagal simpan skema hunian"})
	}
	return c.Status(201).JSON(input)
}

func (h *HunianHandler) GetLatestSchemaHandler(c *fiber.Ctx) error {
	var schema models.Schema
	domain := c.Params("domain", "hunian")
	if err := h.Service.Storage.DB.Where("domain = ? AND status = ?", domain, "ACTIVE").Order("version desc").First(&schema).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Skema hunian aktif tidak ditemukan"})
	}
	return c.JSON(schema)
}

// B. DATA INGESTION
func (h *HunianHandler) IngestData(c *fiber.Ctx) error {
	// 1. Ambil Skema Aktif (Data Mesh Governance)
	var activeSchema models.Schema
	if err := h.Service.Storage.DB.Where("domain = ? AND status = ?", "hunian", "ACTIVE").Order("version desc").First(&activeSchema).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Metadata hunian belum siap, ingest ditolak"})
	}

	fileHeader, err := c.FormFile("document")
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "File dokumen (CSV/Parquet) tidak ditemukan"})
	}

	// 2. PENERJEMAH DROPDOWN ANGKA KHUSUS SOURCE ID (FIX SESUAI MAP TERBARU)
	sourceIDStr := c.FormValue("source_id", "5") // Default: 5 (LAINNYA)
	sourceIDInt, _ := strconv.Atoi(sourceIDStr)

	sourceName := "LAINNYA"
	isWali := false
	trustScore := 60.0

	// Cocokkan angka dengan kamus baru (BPS, PUPR, PKP, LAINNYA)
	if config, exists := app.SourceMap[sourceIDInt]; exists {
		sourceName = config.Name
		isWali = config.IsWali
		if isWali {
			trustScore += 20.0
		}
	} else {
		return c.Status(400).JSON(fiber.Map{"error": "source_id tidak valid. Gunakan: 1 (BPS), 2 (PUPR), 3 (PKP), 5 (LAINNYA)"})
	}

	refDate, _ := time.Parse("2006-01-02", c.FormValue("reference_date", time.Now().Format("2006-01-02")))
	file, _ := fileHeader.Open()
	defer file.Close()

	var dataList []models.RekamHunian
	filename := strings.ToLower(fileHeader.Filename)

	// 3. PARSING LOGIC (CSV AUTO-DETECTION & PARQUET SUPPORT)
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
			p := models.RekamHunian{
				ReferenceDate: refDate,
			}

			// Mapping variabel Hunian dengan alias pintar
			for idx, val := range rec {
				key := strings.ToLower(headers[idx])
				switch key {
				case "nomor_kartu_keluarga", "no_kk", "nkk":
					p.NoKK = val
				case "status_kepemilikan_rumah", "status_kepemilikan":
					p.StatusKepemilikan = val
				case "jenis_lantai_terluas", "jenis_lantai":
					p.JenisLantai = val
				case "luas_lantai":
					fmt.Sscanf(val, "%d", &p.LuasLantai)
				case "jenis_dinding_terluas", "jenis_dinding":
					p.JenisDinding = val
				case "jenis_atap_terluas", "jenis_atap":
					p.JenisAtap = val
				case "sumber_air_minum_utama", "sumber_air_minum", "air_minum":
					p.SumberAirMinum = val
				case "sumber_penerangan_utama", "sumber_penerangan", "penerangan":
					p.SumberPenerangan = val
				case "fasilitas_bab", "bab":
					p.FasilitasBAB = val
				case "jenis_kloset", "kloset":
					p.JenisKloset = val
				case "pembuangan_akhir_tinja", "pembuangan_tinja":
					p.PembuanganTinja = val
				default:
					extraData[key] = val
				}
			}
			p.AdditionalInfo, _ = json.Marshal(extraData)
			dataList = append(dataList, p)
		}
	} else if strings.HasSuffix(filename, ".parquet") {
		tmpPath := "temp_hun_" + uuid.New().String() + ".parquet"
		fw, _ := os.Create(tmpPath)
		io.Copy(fw, file)
		fw.Close()

		fr, _ := local.NewLocalFileReader(tmpPath)
		pr, errP := reader.NewParquetReader(fr, new(models.RekamHunian), 4)
		if errP == nil {
			num := int(pr.GetNumRows())
			res := make([]models.RekamHunian, num)
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
		// a. INJEKSI KEAMANAN & GOVERNANCE
		dataList[i].SourceID = sourceName
		dataList[i].IsWaliData = isWali
		dataList[i].AuditStatus = "PENDING"
		dataList[i].SchemaVersion = fmt.Sprintf("v%d", activeSchema.Version)

		if dataList[i].TrustScore == 0 {
			dataList[i].TrustScore = trustScore
		}

		// b. Validasi Dinamis lewat Service
		if ok, msg := h.Service.ValidateHunianMetadata(dataList[i], activeSchema.Definition); !ok {
			fail++
			errorLogs = append(errorLogs, fmt.Sprintf("NoKK %s: %s", dataList[i].NoKK, msg))
			continue
		}

		// c. Simpan ke Database
		if _, err := h.Service.ProcessIngestion(dataList[i]); err != nil {
			fail++
			errorLogs = append(errorLogs, fmt.Sprintf("NoKK %s: %v", dataList[i].NoKK, err))
		} else {
			success++
		}
	}

	return c.JSON(fiber.Map{
		"status": "Ingestion Finished",
		"domain": "hunian",
		"schema": activeSchema.Name + " v" + fmt.Sprint(activeSchema.Version),
		"stats":  fiber.Map{"total": len(dataList), "success": success, "fail": fail},
		"errors": errorLogs,
	})
}