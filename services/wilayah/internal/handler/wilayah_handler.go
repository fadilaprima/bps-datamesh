package handler

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"wilayah/internal/app"
	"wilayah/models"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/xitongsys/parquet-go-source/local"
	"github.com/xitongsys/parquet-go/reader"
)

type WilayahHandler struct {
	Service app.WilayahService
}

// ============================================================
// B. METADATA & SCHEMA MANAGEMENT (IDENTIK PENDIDIKAN/PENDUDUK)
// ============================================================

func (h *WilayahHandler) CreateSchemaHandler(c *fiber.Ctx) error {
	var input models.Schema
	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Payload JSON tidak valid"})
	}

	domain := c.Params("domain", "wilayah")

	// Archive skema lama agar hanya satu yang ACTIVE
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
		return c.Status(500).JSON(fiber.Map{"error": "Gagal simpan skema wilayah"})
	}
	return c.Status(201).JSON(input)
}

func (h *WilayahHandler) GetLatestSchemaHandler(c *fiber.Ctx) error {
	var schema models.Schema
	domain := c.Params("domain", "wilayah")
	if err := h.Service.Storage.DB.Where("domain = ? AND status = ?", domain, "ACTIVE").Order("version desc").First(&schema).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Skema wilayah aktif tidak ditemukan"})
	}
	return c.JSON(schema)
}

// ============================================================
// A. DATA INGESTION (HYBRID DYNAMIC - WITH ADDITIONAL INFO)
// ============================================================

func (h *WilayahHandler) IngestData(c *fiber.Ctx) error {
	// 1. Ambil Skema Aktif sebagai Kiblat Aturan (Data Mesh Governance)
	var activeSchema models.Schema
	if err := h.Service.Storage.DB.Where("domain = ? AND status = ?", "wilayah", "ACTIVE").Order("version desc").First(&activeSchema).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Metadata wilayah belum siap, ingest ditolak"})
	}

	fileHeader, err := c.FormFile("document")
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "File tidak ditemukan"})
	}

	// 2. Skoring & Otoritas (60 Base + 20 Wali)
	sourceID := strings.ToUpper(c.FormValue("source_id", "UNKNOWN"))
	isWali := false
	trustScore := 60.0
	if reg, exists := app.WilayahSourceRegistry[sourceID]; exists {
		isWali = reg.IsWali
		if isWali { trustScore += 20.0 }
	}

	refDate, _ := time.Parse("2006-01-02", c.FormValue("reference_date", time.Now().Format("2006-01-02")))
	file, _ := fileHeader.Open()
	defer file.Close()
	
	var dataList []models.MasterWilayah
	filename := strings.ToLower(fileHeader.Filename)

	// 3. PARSING LOGIC (CSV AUTO-DETECTION & PARQUET SUPPORT)
	if strings.HasSuffix(filename, ".csv") {
		r := csv.NewReader(file)
		records, _ := r.ReadAll()
		if len(records) < 2 { return c.Status(400).JSON(fiber.Map{"error": "CSV kosong"}) }
		
		headers := records[0]
		for i, rec := range records {
			if i == 0 { continue }
			
			extraData := make(map[string]interface{})
			w := models.MasterWilayah{
				SourceID: sourceID, IsWaliData: isWali, TrustScore: trustScore,
				ReferenceDate: refDate, AuditStatus: "PENDING",
			}

			for idx, val := range rec {
				key := strings.ToLower(headers[idx])
				switch key {
				case "kode_prov": w.KodeProv = val
				case "provinsi": w.Provinsi = val
				case "kode_kab": w.KodeKab = val
				case "kabupaten": w.Kabupaten = val
				case "kode_kec": w.KodeKec = val
				case "kecamatan": w.Kecamatan = val
				case "kode_desa": w.KodeDesa = val
				case "desa": w.Desa = val
				default:
					extraData[key] = val
				}
			}
			w.AdditionalInfo, _ = json.Marshal(extraData)
			dataList = append(dataList, w)
		}
	} else if strings.HasSuffix(filename, ".parquet") {
		// Logika Parquet (Menggunakan file temporary)
		tmpPath := "temp_wil_" + uuid.New().String() + ".parquet"
		fw, _ := os.Create(tmpPath)
		io.Copy(fw, file)
		fw.Close()

		fr, _ := local.NewLocalFileReader(tmpPath)
		pr, errP := reader.NewParquetReader(fr, new(models.MasterWilayah), 4)
		if errP == nil {
			num := int(pr.GetNumRows())
			res := make([]models.MasterWilayah, num)
			pr.Read(&res)
			pr.ReadStop()
			fr.Close()
			os.Remove(tmpPath)
			dataList = res
		}
	} else {
		// Logika JSON
		body, _ := io.ReadAll(file)
		json.Unmarshal(body, &dataList)
	}

	// 4. VALIDASI & PROSES (SCD TYPE 2)
	success, fail := 0, 0
	var errorLogs []string

	for _, w := range dataList {
		// Validasi Dinamis lewat Service (Kiblat ke activeSchema.Definition)
		if ok, msg := h.Service.ValidateWilayahMetadata(w, activeSchema.Definition); !ok {
			fail++
			errorLogs = append(errorLogs, fmt.Sprintf("KodeDesa %s: %s", w.KodeDesa, msg))
			continue
		}

		if _, err := h.Service.ProcessIngestion(w); err != nil {
			fail++
			errorLogs = append(errorLogs, fmt.Sprintf("KodeDesa %s: %v", w.KodeDesa, err))
		} else {
			success++
		}
	}

	return c.JSON(fiber.Map{
		"status": "Ingestion Finished",
		"domain": "wilayah",
		"schema": activeSchema.Name + " v" + fmt.Sprint(activeSchema.Version),
		"stats":  fiber.Map{"total": len(dataList), "success": success, "fail": fail},
		"errors": errorLogs,
	})
}