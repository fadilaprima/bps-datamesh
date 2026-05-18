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

// A. METADATA & SCHEMA MANAGEMENT
func (h *KesejahteraanHandler) CreateSchemaHandler(c *fiber.Ctx) error {
	var input models.Schema
	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Payload JSON tidak valid"})
	}

	domain := c.Params("domain", "kesejahteraan")

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
		return c.Status(500).JSON(fiber.Map{"error": "Gagal simpan skema kesejahteraan"})
	}
	return c.Status(201).JSON(input)
}

func (h *KesejahteraanHandler) GetLatestSchemaHandler(c *fiber.Ctx) error {
	var schema models.Schema
	domain := c.Params("domain", "kesejahteraan")
	if err := h.Service.Storage.DB.Where("domain = ? AND status = ?", domain, "ACTIVE").Order("version desc").First(&schema).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Skema kesejahteraan aktif tidak ditemukan"})
	}
	return c.JSON(schema)
}

// B. DATA INGESTION
func (h *KesejahteraanHandler) IngestData(c *fiber.Ctx) error {
	// 1. Ambil Skema Aktif (Data Mesh Governance)
	var activeSchema models.Schema
	if err := h.Service.Storage.DB.Where("domain = ? AND status = ?", "kesejahteraan", "ACTIVE").Order("version desc").First(&activeSchema).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Metadata kesejahteraan belum siap, ingest ditolak"})
	}

	fileHeader, err := c.FormFile("document")
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "File dokumen (CSV/Parquet) tidak ditemukan"})
	}

	// 2. PENERJEMAH DROPDOWN ANGKA KHUSUS SOURCE ID
	sourceIDStr := c.FormValue("source_id", "3") // Default: 3 (LAINNYA)
	sourceIDInt, _ := strconv.Atoi(sourceIDStr)

	sourceName := "LAINNYA"
	isWali := false
	trustScore := 60.0

	// Cocokkan angka dengan kamus di Service (KEMENSOS)
	if config, exists := app.SourceMap[sourceIDInt]; exists {
		sourceName = config.Name
		isWali = config.IsWali
		if isWali {
			trustScore += 20.0
		}
	} else {
		return c.Status(400).JSON(fiber.Map{"error": "source_id tidak valid. Gunakan: 1 (BPS), 2 (KEMENSOS), 3 (LAINNYA)"})
	}

	refDate, _ := time.Parse("2006-01-02", c.FormValue("reference_date", time.Now().Format("2006-01-02")))
	file, _ := fileHeader.Open()
	defer file.Close()

	var dataList []models.RekamKesejahteraan
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

			// Inisialisasi bersih
			extraData := make(map[string]interface{})
			p := models.RekamKesejahteraan{
				ReferenceDate: refDate,
			}

			// Mapping variabel Kesejahteraan / Kemensos Regsosek
			for idx, val := range rec {
				key := strings.ToLower(headers[idx])
				intVal, _ := strconv.Atoi(val)

				switch key {
				case "nomor_kartu_keluarga":
					p.NoKK = val
				case "desil_nasional":
					p.DesilNasional = intVal
				case "bahan_bakar_utama_memasak":
					p.BahanBakarMemasak = val
				case "kepemilikan_aset":
					p.KepemilikanAset = intVal
				case "aset_bergerak_tabung_gas":
					p.AsetGas = intVal
				case "aset_bergerak_lemari_es":
					p.AsetKulkas = intVal
				case "aset_bergerak_ac":
					p.AsetAC = intVal
				case "aset_bergerak_pemanas_air":
					p.AsetPemanasAir = intVal
				case "aset_bergerak_telepon_rumah":
					p.AsetTelepon = intVal
				case "aset_bergerak_tv_datar":
					p.AsetTV = intVal
				case "aset_bergerak_emas_perhiasan":
					p.AsetEmas = intVal
				case "aset_bergerak_komputer_laptop_tablet":
					p.AsetLaptop = intVal
				case "aset_bergerak_sepeda_motor":
					p.AsetMotor = intVal
				case "aset_bergerak_sepeda":
					p.AsetSepeda = intVal
				case "aset_bergerak_mobil":
					p.AsetMobil = intVal
				case "aset_bergerak_perahu":
					p.AsetPerahu = intVal
				case "aset_bergerak_kapal_perahu_motor":
					p.AsetPerahuMotor = intVal
				case "aset_bergerak_smartphone":
					p.AsetSmartphone = intVal
				case "aset_tidak_bergerak_lahan_lainnya":
					p.AsetLahanLain = intVal
				case "aset_tidak_bergerak_rumah_lainnya":
					p.AsetRumahLain = intVal
				case "jumlah_ternak_sapi":
					p.TernakSapi = intVal
				case "jumlah_ternak_kerbau":
					p.TernakKerbau = intVal
				case "jumlah_ternak_kuda":
					p.TernakKuda = intVal
				case "jumlah_ternak_babi":
					p.TernakBabi = intVal
				case "jumlah_ternak_kambing_domba":
					p.TernakKambing = intVal
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

	// 4. VALIDASI & PROSES (SCD TYPE 2)
	success, fail := 0, 0
	var errorLogs []string

	// Lakukan injeksi data otoritas & validasi ke semua baris data
	for i := range dataList {
		// a. INJEKSI KEAMANAN & GOVERNANCE (Sistem Memaksa Nilai Ini)
		dataList[i].SourceID = sourceName
		dataList[i].IsWaliData = isWali
		dataList[i].AuditStatus = "PENDING"
		dataList[i].SchemaVersion = fmt.Sprintf("v%d", activeSchema.Version)

		if dataList[i].TrustScore == 0 {
			dataList[i].TrustScore = trustScore
		}

		// b. Validasi Dinamis lewat Service
		if ok, msg := h.Service.ValidateKesejahteraanMetadata(dataList[i], activeSchema.Definition); !ok {
			fail++
			errorLogs = append(errorLogs, fmt.Sprintf("NoKK %s: %s", dataList[i].NoKK, msg))
			continue // Skip ke baris berikutnya jika gagal validasi
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
		"domain": "kesejahteraan",
		"schema": activeSchema.Name + " v" + fmt.Sprint(activeSchema.Version),
		"stats":  fiber.Map{"total": len(dataList), "success": success, "fail": fail},
		"errors": errorLogs,
	})
}