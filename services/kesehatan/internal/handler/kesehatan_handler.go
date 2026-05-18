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

// A. METADATA & SCHEMA MANAGEMENT
func (h *KesehatanHandler) CreateSchemaHandler(c *fiber.Ctx) error {
	var input models.Schema
	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Payload JSON tidak valid"})
	}

	domain := c.Params("domain", "kesehatan")

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

// B. DATA INGESTION
func (h *KesehatanHandler) IngestData(c *fiber.Ctx) error {
	// 1. Ambil Skema Aktif (Data Mesh Governance)
	var activeSchema models.Schema
	if err := h.Service.Storage.DB.Where("domain = ? AND status = ?", "kesehatan", "ACTIVE").Order("version desc").First(&activeSchema).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Metadata kesehatan belum siap, ingest ditolak"})
	}

	fileHeader, err := c.FormFile("document")
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "File dokumen (CSV/Parquet) tidak ditemukan"})
	}

	// 2. PENERJEMAH DROPDOWN ANGKA KHUSUS SOURCE ID
	sourceIDStr := c.FormValue("source_id", "5") // Default: 5 (LAINNYA)
	sourceIDInt, _ := strconv.Atoi(sourceIDStr)

	sourceName := "LAINNYA"
	isWali := false
	trustScore := 60.0

	// Cocokkan angka dengan kamus di Service (KEMENKES / BPJS / DINKES)
	if config, exists := app.SourceMap[sourceIDInt]; exists {
		sourceName = config.Name
		isWali = config.IsWali
		if isWali {
			trustScore += 20.0
		}
	} else {
		return c.Status(400).JSON(fiber.Map{"error": "source_id tidak valid. Gunakan: 1 (BPS), 2 (KEMENKES), 3 (BPJS_KESEHATAN), 4 (DINKES), 5 (LAINNYA)"})
	}

	refDate, _ := time.Parse("2006-01-02", c.FormValue("reference_date", time.Now().Format("2006-01-02")))
	file, _ := fileHeader.Open()
	defer file.Close()

	var dataList []models.RekamKesehatan
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
			p := models.RekamKesehatan{
				ReferenceDate: refDate,
			}

			// Mapping variabel Kesehatan
			for idx, val := range rec {
				key := strings.ToLower(headers[idx])
				switch key {
				case "nomor_induk_kependudukan":
					p.NIK = val
				case "pbi_nas":
					p.PbiNas = val
				case "pbi_pemda":
					p.PbiPemda = val
				case "kondisi_gizi":
					p.KondisiGizi = val
				case "penglihatan":
					p.Penglihatan = val
				case "pendengaran", "pendengeran":
					p.Pendengaran = val
				case "berjalan_atau_naik_tangga":
					p.BerjalanNaikTangga = val
				case "menggunakan_tangan_jari":
					p.MenggunakanTanganJari = val
				case "belajar_kemampuan_intelektual":
					p.BelajarIntelektual = val
				case "pengendalian_perilaku":
					p.PengendalianPerilaku = val
				case "berbicara_komunikasi":
					p.BerbicaraKomunikasi = val
				case "mengurus_diri":
					p.MengurusDiri = val
				case "mengingat_berkonsentrasi":
					p.MengingatBerkonsentrasi = val
				case "kesedihan_depresi":
					p.KesedihanDepresi = val
				case "penyakit_kronis":
					p.PenyakitKronis = val
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

	// Lakukan injeksi data otoritas & validasi ke semua baris data
	for i := range dataList {
		// a. INJEKSI KEAMANAN & GOVERNANCE (Sistem Memaksa Nilai Ini)
		dataList[i].SourceID = sourceName
		dataList[i].IsWaliData = isWali
		dataList[i].AuditStatus = "PENDING"
		
		// TAMBAHAN WAJIB: Pastikan struct RekamKesehatan di models.go punya field SchemaVersion string ya!
		dataList[i].SchemaVersion = fmt.Sprintf("v%d", activeSchema.Version)

		if dataList[i].TrustScore == 0 {
			dataList[i].TrustScore = trustScore
		}

		// b. Validasi Dinamis lewat Service
		if ok, msg := h.Service.ValidateKesehatanMetadata(dataList[i], activeSchema.Definition); !ok {
			fail++
			errorLogs = append(errorLogs, fmt.Sprintf("NIK %s: %s", dataList[i].NIK, msg))
			continue // Skip ke baris berikutnya jika gagal validasi
		}

		// c. Simpan ke Database
		if _, err := h.Service.ProcessIngestion(dataList[i]); err != nil {
			fail++
			errorLogs = append(errorLogs, fmt.Sprintf("NIK %s: %v", dataList[i].NIK, err))
		} else {
			success++
		}
	}

	return c.JSON(fiber.Map{
		"status": "Ingestion Finished",
		"domain": "kesehatan",
		"schema": activeSchema.Name + " v" + fmt.Sprint(activeSchema.Version),
		"stats":  fiber.Map{"total": len(dataList), "success": success, "fail": fail},
		"errors": errorLogs,
	})
}