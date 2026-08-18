package handler

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
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
		if isWali { trustScore += 20.0 }
	} else {
		return c.Status(400).JSON(fiber.Map{"error": "source_id tidak valid. Gunakan: 1 (BPS), 2 (KEMENKES), 3 (BPJS_KESEHATAN), 4 (DINKES), 5 (LAINNYA)"})
	}

	refDate, _ := time.Parse("2006-01-02", c.FormValue("reference_date", time.Now().Format("2006-01-02")))
	file, _ := fileHeader.Open()
	defer file.Close()

	var dataList []models.RekamKesehatan
	filename := strings.ToLower(fileHeader.Filename)

	// 3. PARSING LOGIC DENGAN REFLECT ENGINE
	if strings.HasSuffix(filename, ".csv") {
		r := csv.NewReader(file)
		records, _ := r.ReadAll()
		if len(records) < 2 {
			return c.Status(400).JSON(fiber.Map{"error": "CSV kosong"})
		}

		headers := records[0]
		kesehatanType := reflect.TypeOf(models.RekamKesehatan{})

		for i, rec := range records {
			if i == 0 { continue }
			extraData := make(map[string]interface{})
			k := models.RekamKesehatan{ReferenceDate: refDate}
			kesehatanValue := reflect.ValueOf(&k).Elem()

			for idx, val := range rec {
				if idx >= len(headers) { continue }
				key := strings.ToLower(headers[idx])
				found := false

				for fIdx := 0; fIdx < kesehatanType.NumField(); fIdx++ {
					field := kesehatanType.Field(fIdx)
					jsonTag := strings.Split(field.Tag.Get("json"), ",")[0]
					
					// Toleransi typo dari kodingan lawas
					if key == "pendengeran" { key = "pendengaran" }

					if jsonTag == key {
						found = true
						fieldVal := kesehatanValue.Field(fIdx)
						if !fieldVal.CanSet() { continue }
						switch fieldVal.Kind() {
						case reflect.String: fieldVal.SetString(val)
						case reflect.Int, reflect.Int32, reflect.Int64:
							var intVal int64
							if val != "" { fmt.Sscanf(val, "%d", &intVal) }
							fieldVal.SetInt(intVal)
						case reflect.Float32, reflect.Float64:
							var floatVal float64
							if val != "" { fmt.Sscanf(val, "%f", &floatVal) }
							fieldVal.SetFloat(floatVal)
						}
						break
					}
				}
				if !found && key != "" { extraData[key] = val }
			}
			k.AdditionalInfo, _ = json.Marshal(extraData)
			dataList = append(dataList, k)
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

		if dataList[i].TrustScore == 0 { dataList[i].TrustScore = trustScore }

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

func (h *KesehatanHandler) GetValidationStatus(c *fiber.Ctx) error {
	result, err := h.Service.Storage.GetLatestByNIK(c.Params("nik"))
	status := "PENDING"
	if err == nil && result != nil { status = result.AuditStatus }
	return c.JSON(fiber.Map{"nik": c.Params("nik"), "status": status, "schema": "DTSEN-KES-ACTIVE"}) // Ubah label ke KES (Kesehatan)
}

func (h *KesehatanHandler) GetScoring(c *fiber.Ctx) error {
	result, err := h.Service.Storage.GetLatestByNIK(c.Params("nik"))
	if err != nil || result == nil { return c.Status(404).JSON(fiber.Map{"error": "Data tidak ditemukan"}) }
	return c.JSON(fiber.Map{
		"nik": result.NIK, "trust_score": result.TrustScore,
		"is_walidata": result.IsWaliData, "audit_status": result.AuditStatus,
		"quality_label": "Kalkulasi: 60(Sistem) + 20(Sumber) + 20(Audit)",
	})
}

func (h *KesehatanHandler) GetProgress(c *fiber.Ctx) error {
	count, err := h.Service.Storage.CountByNIK(c.Params("nik"))
	status := "NOT_FOUND"
	if err == nil && count > 0 { status = "COMPLETED_IN_MESH" }
	return c.JSON(fiber.Map{"nik": c.Params("nik"), "status": status, "progress": "100%"})
}

func (h *KesehatanHandler) GetAllDatasets(c *fiber.Ctx) error {
	fields := c.Query("fields")
	var fieldList []string
	if fields != "" { fieldList = strings.Split(fields, ",") }
	results, err := h.Service.Storage.GetFetchWithFields(fieldList)
	if err != nil { return c.Status(500).JSON(fiber.Map{"error": "Gagal mengambil data"}) }
	return c.JSON(results)
}

func (h *KesehatanHandler) GetDatasetDetail(c *fiber.Ctx) error {
	fields := c.Query("fields")
	var fieldList []string
	if fields != "" { fieldList = strings.Split(fields, ",") }
	result, err := h.Service.Storage.GetDetailWithFields(c.Params("nik"), fieldList)
	if err != nil { return c.Status(404).JSON(fiber.Map{"error": "NIK tidak ditemukan"}) }
	return c.JSON(result)
}

func (h *KesehatanHandler) UpdateDataset(c *fiber.Ctx) error {
	var payload models.RekamKesehatan
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Payload tidak valid"})
	}
	version, err := h.Service.ProcessManualUpdate(c.Params("nik"), payload)
	if err != nil { return c.Status(500).JSON(fiber.Map{"error": err.Error()}) }
	return c.JSON(fiber.Map{"message": "Versi baru dibuat, status kembali PENDING", "version": version})
}

func (h *KesehatanHandler) SoftDeleteDataset(c *fiber.Ctx) error {
	if err := h.Service.Storage.SoftDelete(c.Params("id")); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Gagal dinonaktifkan (Soft Delete)"})
	}
	return c.JSON(fiber.Map{"message": "Data dinonaktifkan (Soft Delete)"})
}

func (h *KesehatanHandler) GetAuditSamples(c *fiber.Ctx) error {
	results, err := h.Service.Storage.GetAuditSamples()
	if err != nil { return c.Status(500).JSON(fiber.Map{"error": "Gagal mengambil data audit kesehatan"}) }
	if len(results) == 0 { return c.JSON(fiber.Map{"message": "Tidak ada data kesehatan terbaru yang perlu diaudit."}) }
	return c.JSON(results)
}

func (h *KesehatanHandler) SubmitAuditDecision(c *fiber.Ctx) error {
	var input struct {
		NIK     []string `json:"nik"`
		Verdict int      `json:"verdict"` // 1 = VALID, 2 = INVALID
	}
	if err := c.BodyParser(&input); err != nil { return c.Status(400).JSON(fiber.Map{"error": "Payload JSON tidak valid"}) }
	if len(input.NIK) == 0 { return c.Status(400).JSON(fiber.Map{"error": "Daftar NIK tidak boleh kosong. Harus tahu pasti data mana yang diaudit."}) }

	verdictText, err := h.Service.ProcessAuditDecision(input.NIK, input.Verdict)
	if err != nil { return c.Status(400).JSON(fiber.Map{"error": err.Error()}) }
	
	pesan := fmt.Sprintf("Audit kesehatan selesai. %d NIK telah diubah statusnya menjadi %s", len(input.NIK), verdictText)
	return c.JSON(fiber.Map{"message": pesan})
}