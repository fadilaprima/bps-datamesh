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

	"energi/internal/app"
	"energi/models"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/xitongsys/parquet-go-source/local"
	"github.com/xitongsys/parquet-go/reader"
)

type EnergiHandler struct {
	Service app.EnergiService
}

// ==========================================
// 1. METADATA & SCHEMA MANAGEMENT
// ==========================================
func (h *EnergiHandler) CreateSchemaHandler(c *fiber.Ctx) error {
	var input models.Schema
	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Payload JSON tidak valid"})
	}

	domain := c.Params("domain", "energi")
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
		return c.Status(500).JSON(fiber.Map{"error": "Gagal simpan skema energi"})
	}
	return c.Status(201).JSON(input)
}

func (h *EnergiHandler) GetLatestSchemaHandler(c *fiber.Ctx) error {
	var schema models.Schema
	if err := h.Service.Storage.DB.Where("domain = ? AND status = ?", "energi", "ACTIVE").Order("version desc").First(&schema).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Skema energi aktif tidak ditemukan"})
	}
	return c.JSON(schema)
}

// ==========================================
// 2. DATA INGESTION PIPELINE (HYBRID)
// ==========================================
func (h *EnergiHandler) IngestData(c *fiber.Ctx) error {
	var activeSchema models.Schema
	if err := h.Service.Storage.DB.Where("domain = ? AND status = ?", "energi", "ACTIVE").Order("version desc").First(&activeSchema).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Metadata belum siap, ingest ditolak"})
	}

	fileHeader, err := c.FormFile("document")
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "File dokumen tidak ditemukan"})
	}

	sourceIDStr := c.FormValue("source_id", "4")
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

	var dataList []models.RekamEnergi
	filename := strings.ToLower(fileHeader.Filename)

	if strings.HasSuffix(filename, ".csv") {
		// [PARSER 1] CSV Handling
		r := csv.NewReader(file)
		records, _ := r.ReadAll()
		if len(records) < 2 { return c.Status(400).JSON(fiber.Map{"error": "CSV kosong"}) }

		headers := records[0]
		eType := reflect.TypeOf(models.RekamEnergi{})

		for i, rec := range records {
			if i == 0 { continue }
			extraData := make(map[string]interface{})
			p := models.RekamEnergi{ReferenceDate: refDate}
			pValue := reflect.ValueOf(&p).Elem()

			for idx, val := range rec {
				if idx >= len(headers) { continue }
				key := strings.ToLower(headers[idx])
				found := false

				// Alias mapping cerdas
				if key == "no_kk" || key == "nkk" { key = "nomor_kartu_keluarga" }
				if key == "id_pelanggan" || key == "id_pln" { key = "id_pelanggan_pln" }
				if key == "daya_listrik" { key = "daya_terpasang" }

				for fIdx := 0; fIdx < eType.NumField(); fIdx++ {
					field := eType.Field(fIdx)
					jsonTag := strings.Split(field.Tag.Get("json"), ",")[0]
					if jsonTag == key {
						found = true
						fieldVal := pValue.Field(fIdx)
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
			p.AdditionalInfo, _ = json.Marshal(extraData)
			dataList = append(dataList, p)
		}
	} else if strings.HasSuffix(filename, ".parquet") {
		// [PARSER 2] Parquet Handling
		tmpPath := "temp_enr_" + uuid.New().String() + ".parquet"
		fw, _ := os.Create(tmpPath)
		io.Copy(fw, file)
		fw.Close()

		// PERBAIKAN: Mencegah storage leak
		defer os.Remove(tmpPath)

		fr, _ := local.NewLocalFileReader(tmpPath)
		pr, errP := reader.NewParquetReader(fr, new(models.RekamEnergi), 4)
		if errP == nil {
			num := int(pr.GetNumRows())
			res := make([]models.RekamEnergi, num)
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
		
		// PERBAIKAN: Paksa set ReferenceDate agar data tidak ditolak karena "usang"
		for i := range dataList {
			if dataList[i].ReferenceDate.IsZero() {
				dataList[i].ReferenceDate = refDate
			}
		}
	}

	// --- Eksekusi Validasi & Simpan ---
	success, fail := 0, 0
	var errorLogs []string

	for i := range dataList {
		dataList[i].SourceID = sourceName
		dataList[i].IsWaliData = isWali
		dataList[i].AuditStatus = "PENDING"
		dataList[i].SchemaVersion = fmt.Sprintf("v%d", activeSchema.Version)
		
		if dataList[i].TrustScore == 0 { dataList[i].TrustScore = trustScore }

		if ok, msg := h.Service.ValidateEnergiMetadata(dataList[i], activeSchema.Definition); !ok {
			fail++
			errorLogs = append(errorLogs, fmt.Sprintf("NoKK %s: %s", dataList[i].NoKK, msg))
			continue
		}

		// [HARD FAIL] Jika tertangkap error logika, status gagal dan tampilkan pesan ke user
		if _, err := h.Service.ProcessIngestion(dataList[i]); err != nil {
			fail++
			errorLogs = append(errorLogs, fmt.Sprintf("NoKK %s: %v", dataList[i].NoKK, err))
		} else {
			success++
		}
	}

	return c.JSON(fiber.Map{
		"status": "Ingestion Finished",
		"domain": "energi",
		"schema": activeSchema.Name + " v" + fmt.Sprint(activeSchema.Version),
		"stats":  fiber.Map{"total": len(dataList), "success": success, "fail": fail},
		"errors": errorLogs,
	})
}

// ==========================================
// 3. MONITORING & TRACKING
// ==========================================
func (h *EnergiHandler) GetValidationStatus(c *fiber.Ctx) error {
	result, err := h.Service.Storage.GetLatestByNoKK(c.Params("nokk"))
	status := "PENDING"
	if err == nil && result != nil { status = result.AuditStatus }
	return c.JSON(fiber.Map{"nokk": c.Params("nokk"), "status": status, "schema": "DTSEN-ENERGI-ACTIVE"})
}

func (h *EnergiHandler) GetScoring(c *fiber.Ctx) error {
	result, err := h.Service.Storage.GetLatestByNoKK(c.Params("nokk"))
	if err != nil || result == nil { return c.Status(404).JSON(fiber.Map{"error": "Data energi tidak ditemukan"}) }
	return c.JSON(fiber.Map{
		"no_kk": result.NoKK, "trust_score": result.TrustScore,
		"is_walidata": result.IsWaliData, "audit_status": result.AuditStatus,
		"quality_label": "Kalkulasi: 60(Sistem) + 20(Sumber) + 20(Audit)",
	})
}

func (h *EnergiHandler) GetProgress(c *fiber.Ctx) error {
	count, err := h.Service.Storage.CountByNoKK(c.Params("nokk"))
	status := "NOT_FOUND"
	if err == nil && count > 0 { status = "COMPLETED_IN_MESH" }
	return c.JSON(fiber.Map{"nokk": c.Params("nokk"), "status": status, "progress": "100%"})
}

// ==========================================
// 4. DATASET MAINTENANCE (CRUD)
// ==========================================
func (h *EnergiHandler) GetAllDatasets(c *fiber.Ctx) error {
	fields := c.Query("fields")
	var fieldList []string
	if fields != "" { fieldList = strings.Split(fields, ",") }
	results, err := h.Service.Storage.GetFetchWithFields(fieldList)
	if err != nil { return c.Status(500).JSON(fiber.Map{"error": "Gagal mengambil data"}) }
	return c.JSON(results)
}

func (h *EnergiHandler) GetDatasetDetail(c *fiber.Ctx) error {
	fields := c.Query("fields")
	var fieldList []string
	if fields != "" { fieldList = strings.Split(fields, ",") }
	result, err := h.Service.Storage.GetDetailWithFields(c.Params("nokk"), fieldList)
	if err != nil { return c.Status(404).JSON(fiber.Map{"error": "Nomor KK tidak ditemukan"}) }
	return c.JSON(result)
}

func (h *EnergiHandler) UpdateDataset(c *fiber.Ctx) error {
	var payload models.RekamEnergi
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Payload tidak valid"})
	}
	version, err := h.Service.ProcessManualUpdate(c.Params("nokk"), payload)
	if err != nil { return c.Status(500).JSON(fiber.Map{"error": err.Error()}) }
	return c.JSON(fiber.Map{"message": "Versi baru energi dibuat", "version": version})
}

func (h *EnergiHandler) SoftDeleteDataset(c *fiber.Ctx) error {
	identifier := c.Params("nokk")
	if identifier == "" {
		identifier = c.Params("id")
	}

	if err := h.Service.Storage.SoftDelete(identifier); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Gagal dinonaktifkan"})
	}
	return c.JSON(fiber.Map{"message": "Data energi dinonaktifkan"})
}

// ==========================================
// 5. GOVERNANCE & AUDIT LOGIC
// ==========================================
func (h *EnergiHandler) GetAuditSamples(c *fiber.Ctx) error {
	limit, err := strconv.Atoi(c.Query("limit", "10"))
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Limit harus berupa angka"})
	}

	results, err := h.Service.Storage.GetAuditSamples(limit)
	if err != nil { return c.Status(500).JSON(fiber.Map{"error": "Gagal mengambil data audit"}) }
	
	if len(results) == 0 { return c.JSON(fiber.Map{"message": "Tidak ada data energi terbaru yang perlu diaudit."}) }
	
	return c.JSON(results)
}

func (h *EnergiHandler) SubmitAuditDecision(c *fiber.Ctx) error {
	var input struct {
		NoKK    []string `json:"nomor_kartu_keluarga"`
		Verdict int      `json:"verdict"`
	}
	if err := c.BodyParser(&input); err != nil { return c.Status(400).JSON(fiber.Map{"error": "Payload JSON tidak valid"}) }
	if len(input.NoKK) == 0 { return c.Status(400).JSON(fiber.Map{"error": "Daftar NoKK wajib diisi"}) }

	verdictText, err := h.Service.ProcessAuditDecision(input.NoKK, input.Verdict)
	if err != nil { return c.Status(400).JSON(fiber.Map{"error": err.Error()}) }
	return c.JSON(fiber.Map{"message": fmt.Sprintf("Audit energi selesai. %d KK diubah menjadi %s", len(input.NoKK), verdictText)})
}