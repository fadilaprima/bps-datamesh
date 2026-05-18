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

	"ketenagakerjaan/internal/app"
	"ketenagakerjaan/models"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/xitongsys/parquet-go-source/local"
	"github.com/xitongsys/parquet-go/reader"
)

type KetenagakerjaanHandler struct {
	Service app.KetenagakerjaanService
}

// A. METADATA & SCHEMA MANAGEMENT
func (h *KetenagakerjaanHandler) CreateSchemaHandler(c *fiber.Ctx) error {
	var input models.Schema
	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Payload JSON tidak valid"})
	}

	domain := c.Params("domain", "ketenagakerjaan")

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
		return c.Status(500).JSON(fiber.Map{"error": "Gagal simpan skema ketenagakerjaan"})
	}
	return c.Status(201).JSON(input)
}

func (h *KetenagakerjaanHandler) GetLatestSchemaHandler(c *fiber.Ctx) error {
	var schema models.Schema
	domain := c.Params("domain", "ketenagakerjaan")
	if err := h.Service.Storage.DB.Where("domain = ? AND status = ?", domain, "ACTIVE").Order("version desc").First(&schema).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Skema ketenagakerjaan aktif tidak ditemukan"})
	}
	return c.JSON(schema)
}

// B. DATA INGESTION
func (h *KetenagakerjaanHandler) IngestData(c *fiber.Ctx) error {
	// 1. Ambil Skema Aktif (Data Mesh Governance)
	var activeSchema models.Schema
	if err := h.Service.Storage.DB.Where("domain = ? AND status = ?", "ketenagakerjaan", "ACTIVE").Order("version desc").First(&activeSchema).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Metadata ketenagakerjaan belum siap, ingest ditolak"})
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

	// Cocokkan angka dengan kamus di Service (BPJS Ketenagakerjaan)
	if config, exists := app.SourceMap[sourceIDInt]; exists {
		sourceName = config.Name
		isWali = config.IsWali
		if isWali {
			trustScore += 20.0
		}
	} else {
		return c.Status(400).JSON(fiber.Map{"error": "source_id tidak valid. Gunakan: 1 (BPS), 2 (BPJS_KETENAGAKERJAAN), 3 (LAINNYA)"})
	}

	refDate, _ := time.Parse("2006-01-02", c.FormValue("reference_date", time.Now().Format("2006-01-02")))
	file, _ := fileHeader.Open()
	defer file.Close()

	var dataList []models.RekamKetenagakerjaan
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
			k := models.RekamKetenagakerjaan{
				ReferenceDate: refDate,
			}

			// Mapping dengan alias cerdas yang disesuaikan dengan struct kamu yang baru
			for idx, val := range rec {
				key := strings.ToLower(headers[idx])
				switch key {
				case "nik", "nomor_induk_kependudukan":
					k.NIK = val
				case "status_bekerja":
					k.StatusBekerja = val
				case "lapangan_usaha_dari_pekerjaan_utama", "lapangan_usaha_pekerjaan":
					k.LapanganUsahaUtama = val
				case "status_dalam_pekerjaan_utama", "kedudukan_pekerjaan":
					k.StatusDalamPekerjaanUtama = val
				case "kepemilikan_usaha", "punya_usaha":
					k.KepemilikanUsaha = val
				case "jumlah_usaha":
					fmt.Sscanf(val, "%d", &k.JumlahUsaha)
				case "lapangan_usaha_dari_usaha_utama", "lapangan_usaha_usaha":
					k.LapanganUsahaPekerjaanUtama = val
				case "jumlah_pekerja_yang_dibayar_dari_usaha_utama", "jml_pekerja_dibayar":
					fmt.Sscanf(val, "%d", &k.JumlahPekerjaDibayar)
				case "jumlah_pekerja_yang_tidak_dibayar_dari_usaha_utama", "jml_pekerja_tak_dibayar":
					fmt.Sscanf(val, "%d", &k.JumlahPekerjaTidakDibayar)
				case "omzet_usaha_utama", "omzet":
					fmt.Sscanf(val, "%f", &k.OmzetUsahaUtama)
				default:
					extraData[key] = val
				}
			}
			k.AdditionalInfo, _ = json.Marshal(extraData)
			dataList = append(dataList, k)
		}
	} else if strings.HasSuffix(filename, ".parquet") {
		tmpPath := "temp_kerja_" + uuid.New().String() + ".parquet"
		fw, _ := os.Create(tmpPath)
		io.Copy(fw, file)
		fw.Close()

		fr, _ := local.NewLocalFileReader(tmpPath)
		pr, errP := reader.NewParquetReader(fr, new(models.RekamKetenagakerjaan), 4)
		if errP == nil {
			num := int(pr.GetNumRows())
			res := make([]models.RekamKetenagakerjaan, num)
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
		if ok, msg := h.Service.ValidateKetenagakerjaanMetadata(dataList[i], activeSchema.Definition); !ok {
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
		"domain": "ketenagakerjaan",
		"schema": activeSchema.Name + " v" + fmt.Sprint(activeSchema.Version),
		"stats":  fiber.Map{"total": len(dataList), "success": success, "fail": fail},
		"errors": errorLogs,
	})
}
