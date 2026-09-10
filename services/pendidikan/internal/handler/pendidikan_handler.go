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

// ==========================================
// 1. METADATA & SCHEMA MANAGEMENT
// ==========================================
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

// ==========================================
// 2. DATA INGESTION PIPELINE (HYBRID)
// ==========================================
func (h *PendidikanHandler) IngestData(c *fiber.Ctx) error {
	var activeSchema models.Schema
	if err := h.Service.Storage.DB.Where("domain = ? AND status = ?", "pendidikan", "ACTIVE").Order("version desc").First(&activeSchema).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Metadata belum siap, ingest ditolak"})
	}

	fileHeader, err := c.FormFile("document")
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "File tidak ditemukan"})
	}

	sourceIDStr := c.FormValue("source_id", "3")
	sourceIDInt, _ := strconv.Atoi(sourceIDStr)
	sourceName := "LAINNYA"
	isWali := false
	trustScore := 60.0

	if config, exists := app.SourceMap[sourceIDInt]; exists {
		sourceName = config.Name
		isWali = config.IsWali
		if isWali { trustScore += 20.0 }
	} else {
		return c.Status(400).JSON(fiber.Map{"error": "source_id tidak valid"})
	}

	refDate, _ := time.Parse("2006-01-02", c.FormValue("reference_date", time.Now().Format("2006-01-02")))
	file, _ := fileHeader.Open()
	defer file.Close()

	var dataList []models.RiwayatPendidikan
	filename := strings.ToLower(fileHeader.Filename)

	if strings.HasSuffix(filename, ".csv") {
		// [PARSER 1] CSV Handling dengan Reflect Engine
		r := csv.NewReader(file)
		records, _ := r.ReadAll()
		if len(records) < 2 {
			return c.Status(400).JSON(fiber.Map{"error": "CSV kosong"})
		}

		headers := records[0]
		pendType := reflect.TypeOf(models.RiwayatPendidikan{})

		for i, rec := range records {
			if i == 0 { continue }
			extraData := make(map[string]interface{})
			w := models.RiwayatPendidikan{ReferenceDate: refDate}
			pendValue := reflect.ValueOf(&w).Elem()

			for idx, val := range rec {
				if idx >= len(headers) { continue }
				key := strings.ToLower(headers[idx])
				found := false

				for fIdx := 0; fIdx < pendType.NumField(); fIdx++ {
					field := pendType.Field(fIdx)
					jsonTag := strings.Split(field.Tag.Get("json"), ",")[0]
					if jsonTag == key {
						found = true
						fieldVal := pendValue.Field(fIdx)
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
			w.AdditionalInfo, _ = json.Marshal(extraData)
			dataList = append(dataList, w)
		}
	} else if strings.HasSuffix(filename, ".parquet") {
		// [PARSER 2] Parquet Handling
		tmpPath := "temp_pend_" + uuid.New().String() + ".parquet"
		fw, _ := os.Create(tmpPath)
		io.Copy(fw, file)
		fw.Close()

		defer os.Remove(tmpPath)

		fr, _ := local.NewLocalFileReader(tmpPath)
		pr, errP := reader.NewParquetReader(fr, new(models.RiwayatPendidikan), 4)
		if errP == nil {
			num := int(pr.GetNumRows())
			res := make([]models.RiwayatPendidikan, num)
			pr.Read(&res)
			pr.ReadStop()
			fr.Close()
			dataList = res
		}
	} else {
		// [PARSER 3] JSON Handling
		body, _ := io.ReadAll(file)
		if errJson := json.Unmarshal(body, &dataList); errJson != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Format JSON gagal diparsing"})
		}
		
		for i := range dataList {
			if dataList[i].ReferenceDate.IsZero() {
				dataList[i].ReferenceDate = refDate
			}
		}
	}

	success, fail := 0, 0
	var errorLogs []string

	for i := range dataList {
		dataList[i].SourceID = sourceName
		dataList[i].IsWaliData = isWali
		dataList[i].AuditStatus = "PENDING"
		dataList[i].SchemaVersion = fmt.Sprintf("v%d", activeSchema.Version)

		if dataList[i].TrustScore == 0 { dataList[i].TrustScore = trustScore }

		if ok, msg := h.Service.ValidatePendidikanMetadata(dataList[i], activeSchema.Definition); !ok {
			fail++
			errorLogs = append(errorLogs, fmt.Sprintf("NIK %s: %s", dataList[i].NIK, msg))
			continue
		}

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

// ==========================================
// 3. MONITORING & TRACKING
// ==========================================
func (h *PendidikanHandler) GetValidationStatus(c *fiber.Ctx) error {
	result, err := h.Service.Storage.GetLatestByNIK(c.Params("nik"))
	status := "PENDING"
	if err == nil && result != nil { status = result.AuditStatus }
	return c.JSON(fiber.Map{"nik": c.Params("nik"), "status": status, "schema": "DTSEN-PEND-ACTIVE"})
}

func (h *PendidikanHandler) GetScoring(c *fiber.Ctx) error {
	result, err := h.Service.Storage.GetLatestByNIK(c.Params("nik"))
	if err != nil || result == nil { return c.Status(404).JSON(fiber.Map{"error": "Data tidak ditemukan"}) }
	return c.JSON(fiber.Map{
		"nik": result.NIK, "trust_score": result.TrustScore,
		"is_walidata": result.IsWaliData, "audit_status": result.AuditStatus,
		"quality_label": "Kalkulasi: 60(Sistem) + 20(Sumber) + 20(Audit)",
	})
}

func (h *PendidikanHandler) GetProgress(c *fiber.Ctx) error {
	count, err := h.Service.Storage.CountByNIK(c.Params("nik"))
	status := "NOT_FOUND"
	if err == nil && count > 0 { status = "COMPLETED_IN_MESH" }
	return c.JSON(fiber.Map{"nik": c.Params("nik"), "status": status, "progress": "100%"})
}

// ==========================================
// 4. DATASET MAINTENANCE (CRUD)
// ==========================================
func (h *PendidikanHandler) GetAllDatasets(c *fiber.Ctx) error {
	fields := c.Query("fields")
	var fieldList []string
	if fields != "" { fieldList = strings.Split(fields, ",") }
	results, err := h.Service.Storage.GetFetchWithFields(fieldList)
	if err != nil { return c.Status(500).JSON(fiber.Map{"error": "Gagal mengambil data"}) }
	return c.JSON(results)
}

func (h *PendidikanHandler) GetDatasetDetail(c *fiber.Ctx) error {
	fields := c.Query("fields")
	var fieldList []string
	if fields != "" { fieldList = strings.Split(fields, ",") }
	result, err := h.Service.Storage.GetDetailWithFields(c.Params("nik"), fieldList)
	if err != nil { return c.Status(404).JSON(fiber.Map{"error": "NIK tidak ditemukan"}) }
	return c.JSON(result)
}

func (h *PendidikanHandler) UpdateDataset(c *fiber.Ctx) error {
	var payload struct {
		Data           models.RiwayatPendidikan `json:"data"` 
		CorrectionNote string                   `json:"correction_note"`
	}

	if err := c.BodyParser(&payload); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Payload tidak valid"})
	}

	version, err := h.Service.ProcessManualUpdate(c.Params("nik"), payload.Data)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"message": "Versi baru dibuat, status kembali PENDING",
		"version": version,
		"note":    payload.CorrectionNote,
	})
}

func (h *PendidikanHandler) SoftDeleteDataset(c *fiber.Ctx) error {
	identifier := c.Params("nik")
	if identifier == "" {
		identifier = c.Params("id")
	}

	if identifier == "" {
		return c.Status(400).JSON(fiber.Map{"error": "Parameter NIK atau ID wajib diisi"})
	}

	if err := h.Service.Storage.SoftDelete(identifier); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Gagal menonaktifkan data"})
	}

	return c.JSON(fiber.Map{"message": "Data berhasil dinonaktifkan (Soft Delete)"})
}

// ==========================================
// 5. GOVERNANCE & AUDIT LOGIC
// ==========================================
func (h *PendidikanHandler) GetAuditSamples(c *fiber.Ctx) error {
	limit, err := strconv.Atoi(c.Query("limit", "10"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Limit harus berupa angka"})
	}

	results, err := h.Service.Storage.GetAuditSamples(limit) 
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Gagal mengambil data audit"})
	}
	
	if len(results) == 0 {
		return c.JSON(fiber.Map{"message": "Tidak ada data kependudukan terbaru yang perlu diaudit."})
	}
	
	return c.JSON(results)
}

func (h *PendidikanHandler) SubmitAuditDecision(c *fiber.Ctx) error {
	var input struct {
		NIK     []string `json:"nik"`
		Verdict int      `json:"verdict"`
	}
	if err := c.BodyParser(&input); err != nil { return c.Status(400).JSON(fiber.Map{"error": "Payload JSON tidak valid"}) }
	if len(input.NIK) == 0 { return c.Status(400).JSON(fiber.Map{"error": "List NIK tidak boleh kosong"}) }

	verdictText, err := h.Service.ProcessAuditDecision(input.NIK, input.Verdict)
	if err != nil { return c.Status(400).JSON(fiber.Map{"error": err.Error()}) }
	return c.JSON(fiber.Map{"message": fmt.Sprintf("Berhasil memproses %d NIK menjadi %s", len(input.NIK), verdictText)})
}