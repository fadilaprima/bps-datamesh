package handler

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os" 
	"strings"
	"time"

	"kesehatan/internal/app"
	"kesehatan/models"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/xitongsys/parquet-go-source/local" 
	"github.com/xitongsys/parquet-go/reader" 	   
)

type KesehatanHandler struct {
	Service app.KesehatanService
}

// ============================================================
// B. METADATA & SCHEMA MANAGEMENT (IDENTIK TOTAL)
// ============================================================

func (h *KesehatanHandler) CreateSchemaHandler(c *fiber.Ctx) error {
	var input models.Schema
	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Payload JSON tidak valid"})
	}

	domain := c.Params("domain", "kesehatan")

	// Arsipkan skema lama agar hanya satu yang ACTIVE
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
		return c.Status(500).JSON(fiber.Map{"error": "Gagal simpan skema kesehatan"})
	}
	return c.Status(201).JSON(input)
}

func (h *KesehatanHandler) GetLatestSchemaHandler(c *fiber.Ctx) error {
	var schema models.Schema
	domain := c.Params("domain", "kesehatan")
	if err := h.Service.Storage.DB.Where("domain = ? AND status = ?", domain, "ACTIVE").Order("version desc").First(&schema).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Skema kesehatan aktif tidak ditemukan"})
	}
	return c.JSON(schema)
}

// ============================================================
// A. DATA INGESTION (HYBRID DYNAMIC - VARIABEL KESEHATAN LENGKAP)
// ============================================================

func (h *KesehatanHandler) IngestData(c *fiber.Ctx) error {
	// 1. Cek Skema Aktif
	var activeSchema models.Schema
	if err := h.Service.Storage.DB.Where("domain = ? AND status = ?", "kesehatan", "ACTIVE").Order("version desc").First(&activeSchema).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Metadata kesehatan belum siap, ingest ditolak"})
	}

	// 2. Ambil File Dokumen (Identik pakai key 'document')
	fileHeader, err := c.FormFile("document")
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "File tidak ditemukan"})
	}

	// Otoritas & Trust Score (Identik)
	sourceID := strings.ToUpper(c.FormValue("source_id", "UNKNOWN"))
	isWali := false
	trustScore := 60.0
	if reg, exists := app.KesehatanSourceRegistry[sourceID]; exists {
		isWali = reg.IsWali
		if isWali { trustScore += 20.0 }
	}

	refDate, _ := time.Parse("2006-01-02", c.FormValue("reference_date", time.Now().Format("2006-01-02")))
	file, _ := fileHeader.Open()
	defer file.Close()
	
	var dataList []models.RekamKesehatan
	filename := strings.ToLower(fileHeader.Filename)

	// 3. PARSING LOGIC (CSV, PARQUET, JSON)
	if strings.HasSuffix(filename, ".csv") {
		r := csv.NewReader(file)
		records, _ := r.ReadAll()
		if len(records) < 2 { return c.Status(400).JSON(fiber.Map{"error": "CSV kosong"}) }
		
		headers := records[0]
		for i, rec := range records {
			if i == 0 { continue }
			
			extraData := make(map[string]interface{})
			p := models.RekamKesehatan{
				SourceID: sourceID, IsWaliData: isWali, TrustScore: trustScore,
				ReferenceDate: refDate, AuditStatus: "PENDING",
			}

			for idx, val := range rec {
				key := strings.ToLower(headers[idx])
				switch key {
				case "nik", "nomor_induk_kependudukan": p.NIK = val
				case "pbi_nas": p.PbiNas = val
				case "pbi_pemda": p.PbiPemda = val
				case "kondisi_gizi": p.KondisiGizi = val
				case "penglihatan": p.Penglihatan = val
				case "pendengaran", "pendengeran": p.Pendengaran = val
				case "berjalan_atau_naik_tangga": p.BerjalanNaikTangga = val
				case "menggunakan_tangan_jari": p.MenggunakanTanganJari = val
				case "belajar_kemampuan_intelektual": p.BelajarIntelektual = val
				case "pengendalian_perilaku": p.PengendalianPerilaku = val
				case "berbicara_komunikasi": p.BerbicaraKomunikasi = val
				case "mengurus_diri": p.MengurusDiri = val
				case "mengingat_berkonsentrasi": p.MengingatBerkonsentrasi = val
				case "kesedihan_depresi": p.KesedihanDepresi = val
				case "penyakit_kronis": p.PenyakitKronis = val
				default:
					extraData[key] = val
				}
			}
			p.AdditionalInfo, _ = json.Marshal(extraData)
			dataList = append(dataList, p)
		}
	} else if strings.HasSuffix(filename, ".parquet") {
		tmpPath := "temp_kes_" + uuid.New().String() + ".parquet"
		fw, _ := os.Create(tmpPath)
		io.Copy(fw, file)
		fw.Close()

		fr, _ := local.NewLocalFileReader(tmpPath)
		pr, errP := reader.NewParquetReader(fr, new(models.RekamKesehatan), 4)
		if errP == nil {
			num := int(pr.GetNumRows())
			res := make([]models.RekamKesehatan, num)
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

	for _, p := range dataList {
		if ok, msg := h.Service.ValidateKesehatanMetadata(p, activeSchema.Definition); !ok {
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