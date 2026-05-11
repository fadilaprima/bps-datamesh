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

	"kesejahteraan/internal/app"
	"kesejahteraan/models"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/xitongsys/parquet-go-source/local"
	"github.com/xitongsys/parquet-go/reader"
)

type KesejahteraanHandler struct {
	Service app.KesejahteraanService
}

// A. METADATA & SCHEMA MANAGEMENT (DINAMIS - KESEJAHTERAAN)
func (h *KesejahteraanHandler) CreateSchemaHandler(c *fiber.Ctx) error {
	var input models.Schema
	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Payload JSON tidak valid"})
	}

	domain := c.Params("domain", "kesejahteraan")

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
		return c.Status(500).JSON(fiber.Map{"error": "Gagal simpan skema kesejahteraan"})
	}
	return c.Status(201).JSON(input)
}

func (h *KesejahteraanHandler) GetLatestSchemaHandler(c *fiber.Ctx) error {
	var schema models.Schema
	domain := c.Params("domain", "kesejahteraan")
	if err := h.Service.Storage.DB.Where("domain = ? AND status = ?", domain, "ACTIVE").Order("version desc").First(&schema).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Skema aktif tidak ditemukan"})
	}
	return c.JSON(schema)
}


// B. DATA INGESTION (HYBRID DYNAMIC - VARIABEL KEMENSOS LENGKAP)
func (h *KesejahteraanHandler) IngestData(c *fiber.Ctx) error {
	var activeSchema models.Schema
	if err := h.Service.Storage.DB.Where("domain = ? AND status = ?", "kesejahteraan", "ACTIVE").Order("version desc").First(&activeSchema).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Metadata belum siap, ingest ditolak"})
	}

	fileHeader, err := c.FormFile("document")
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "File tidak ditemukan"})
	}

	sourceID := strings.ToUpper(c.FormValue("source_id", "UNKNOWN"))
	isWali := false
	trustScore := 60.0
	if reg, exists := app.KesejahteraanSourceRegistry[sourceID]; exists {
		isWali = reg.IsWali
		if isWali { trustScore += 20.0 }
	}

	refDate, _ := time.Parse("2006-01-02", c.FormValue("reference_date", time.Now().Format("2006-01-02")))
	file, _ := fileHeader.Open()
	defer file.Close()

	var dataList []models.RekamKesejahteraan
	filename := strings.ToLower(fileHeader.Filename)

	if strings.HasSuffix(filename, ".csv") {
		r := csv.NewReader(file)
		records, _ := r.ReadAll()
		if len(records) < 2 { return c.Status(400).JSON(fiber.Map{"error": "CSV kosong"}) }

		headers := records[0]
		for i, rec := range records {
			if i == 0 { continue }

			extraData := make(map[string]interface{})
			p := models.RekamKesejahteraan{
				SourceID: sourceID, IsWaliData: isWali, TrustScore: trustScore,
				ReferenceDate: refDate, AuditStatus: "PENDING",
			}

			for idx, val := range rec {
				key := strings.ToLower(headers[idx])
				intVal, _ := strconv.Atoi(val) 

				switch key {
				case "nomor_kartu_keluarga", "no_kk": p.NoKK = val
				case "desil_nasional": p.DesilNasional = intVal
				case "bahan_bakar_utama_memasak": p.BahanBakarMemasak = val
				case "kepemilikan_aset": p.KepemilikanAset = intVal
				case "aset_bergerak_tabung_gas": p.AsetGas = intVal
				case "aset_bergerak_lemari_es": p.AsetKulkas = intVal
				case "aset_bergerak_ac": p.AsetAC = intVal
				case "aset_bergerak_pemanas_air": p.AsetPemanasAir = intVal
				case "aset_bergerak_telepon_rumah": p.AsetTelepon = intVal
				case "aset_bergerak_tv_datar": p.AsetTV = intVal
				case "aset_bergerak_emas_perhiasan": p.AsetEmas = intVal
				case "aset_bergerak_komputer_laptop_tablet": p.AsetLaptop = intVal
				case "aset_bergerak_sepeda_motor": p.AsetMotor = intVal
				case "aset_bergerak_sepeda": p.AsetSepeda = intVal
				case "aset_bergerak_mobil": p.AsetMobil = intVal
				case "aset_bergerak_perahu": p.AsetPerahu = intVal
				case "aset_bergerak_kapal_perahu_motor": p.AsetPerahuMotor = intVal
				case "aset_bergerak_smartphone": p.AsetSmartphone = intVal
				case "aset_tidak_bergerak_lahan_lainnya": p.AsetLahanLain = intVal
				case "aset_tidak_bergerak_rumah_lainnya": p.AsetRumahLain = intVal
				case "jumlah_ternak_sapi": p.TernakSapi = intVal
				case "jumlah_ternak_kerbau": p.TernakKerbau = intVal
				case "jumlah_ternak_kuda": p.TernakKuda = intVal
				case "jumlah_ternak_babi": p.TernakBabi = intVal
				case "jumlah_ternak_kambing_domba": p.TernakKambing = intVal
				default:
					extraData[key] = val
				}
			}
			p.AdditionalInfo, _ = json.Marshal(extraData)
			dataList = append(dataList, p)
		}
	} else if strings.HasSuffix(filename, ".parquet") {
		tmpPath := "temp_kesj_" + uuid.New().String() + ".parquet"
		fw, _ := os.Create(tmpPath)
		io.Copy(fw, file)
		fw.Close()

		fr, _ := local.NewLocalFileReader(tmpPath)
		pr, errP := reader.NewParquetReader(fr, new(models.RekamKesejahteraan), 4)
		if errP == nil {
			num := int(pr.GetNumRows())
			res := make([]models.RekamKesejahteraan, num)
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

	success, fail := 0, 0
	var errorLogs []string

	for _, p := range dataList {
		if ok, msg := h.Service.ValidateKesejahteraanMetadata(p, activeSchema.Definition); !ok {
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