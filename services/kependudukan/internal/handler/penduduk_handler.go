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

	"kependudukan/internal/app"
	"kependudukan/models"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/xitongsys/parquet-go-source/local"
	"github.com/xitongsys/parquet-go/reader"
)

type PendudukHandler struct {
	Service app.PendudukService
}

// A. METADATA & SCHEMA MANAGEMENT
func (h *PendudukHandler) CreateSchemaHandler(c *fiber.Ctx) error {
	var input models.Schema
	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Payload JSON tidak valid"})
	}

	domain := c.Params("domain", "penduduk")

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
		return c.Status(500).JSON(fiber.Map{"error": "Gagal simpan skema kependudukan"})
	}
	return c.Status(201).JSON(input)
}

func (h *PendudukHandler) GetLatestSchemaHandler(c *fiber.Ctx) error {
	var schema models.Schema
	domain := c.Params("domain", "penduduk")
	if err := h.Service.Storage.DB.Where("domain = ? AND status = ?", domain, "ACTIVE").Order("version desc").First(&schema).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Skema kependudukan aktif tidak ditemukan"})
	}
	return c.JSON(schema)
}

// B. DATA INGESTION (HYBRID DYNAMIC - REFLECT ENGINE)
func (h *PendudukHandler) IngestData(c *fiber.Ctx) error {
	var activeSchema models.Schema
	if err := h.Service.Storage.DB.Where("domain = ? AND status = ?", "penduduk", "ACTIVE").Order("version desc").First(&activeSchema).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Metadata kependudukan belum siap, ingest ditolak"})
	}

	fileHeader, err := c.FormFile("document")
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "File dokumen tidak ditemukan"})
	}

	sourceIDStr := c.FormValue("source_id", "3") // Default: 3 (LAINNYA)
	sourceIDInt, _ := strconv.Atoi(sourceIDStr)

	sourceName := "LAINNYA"
	isWali := false
	trustScore := 60.0

	if config, exists := app.SourceMap[sourceIDInt]; exists {
		sourceName = config.Name
		isWali = config.IsWali
		if isWali {
			trustScore += 20.0
		}
	} else {
		return c.Status(400).JSON(fiber.Map{"error": "source_id tidak valid. Gunakan: 1 (BPS), 2 (KEMENDAGRI), 3 (LAINNYA)"})
	}

	refDate, _ := time.Parse("2006-01-02", c.FormValue("reference_date", time.Now().Format("2006-01-02")))
	file, _ := fileHeader.Open()
	defer file.Close()

	var dataList []models.Penduduk
	filename := strings.ToLower(fileHeader.Filename)

	// 3. PARSING LOGIC DENGAN REFLECT
	if strings.HasSuffix(filename, ".csv") {
		r := csv.NewReader(file)
		records, _ := r.ReadAll()
		if len(records) < 2 {
			return c.Status(400).JSON(fiber.Map{"error": "CSV kosong"})
		}

		headers := records[0]
		pendudukType := reflect.TypeOf(models.Penduduk{})

		for i, rec := range records {
			if i == 0 {
				continue
			}
			extraData := make(map[string]interface{})
			p := models.Penduduk{ReferenceDate: refDate}
			pendudukValue := reflect.ValueOf(&p).Elem()

			for idx, val := range rec {
				if idx >= len(headers) {
					continue
				}
				key := strings.ToLower(headers[idx])
				found := false

				for fIdx := 0; fIdx < pendudukType.NumField(); fIdx++ {
					field := pendudukType.Field(fIdx)
					jsonTag := strings.Split(field.Tag.Get("json"), ",")[0]

					// Toleransi typo legacy (jika headers 'nik' atau 'jml_anggota')
					if key == "nik" {
						key = "nomor_induk_kependudukan"
					}
					if key == "jml_anggota" {
						key = "jumlah_anggota_keluarga"
					}

					if jsonTag == key {
						found = true
						fieldVal := pendudukValue.Field(fIdx)
						if !fieldVal.CanSet() {
							continue
						}
						switch fieldVal.Kind() {
						case reflect.String:
							fieldVal.SetString(val)
						case reflect.Int, reflect.Int32, reflect.Int64:
							var intVal int64
							if val != "" {
								fmt.Sscanf(val, "%d", &intVal)
							}
							fieldVal.SetInt(intVal)
						case reflect.Float32, reflect.Float64:
							var floatVal float64
							if val != "" {
								fmt.Sscanf(val, "%f", &floatVal)
							}
							fieldVal.SetFloat(floatVal)
						}
						break
					}
				}
				if !found && key != "" {
					extraData[key] = val
				}
			}
			p.AdditionalInfo, _ = json.Marshal(extraData)
			dataList = append(dataList, p)
		}
	} else if strings.HasSuffix(filename, ".parquet") {
		tmpPath := "temp_penduduk_" + uuid.New().String() + ".parquet"
		fw, _ := os.Create(tmpPath)
		io.Copy(fw, file)
		fw.Close()

		fr, _ := local.NewLocalFileReader(tmpPath)
		pr, errP := reader.NewParquetReader(fr, new(models.Penduduk), 4)
		if errP == nil {
			num := int(pr.GetNumRows())
			res := make([]models.Penduduk, num)
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

	for i := range dataList {
		dataList[i].SourceID = sourceName
		dataList[i].IsWaliData = isWali
		dataList[i].AuditStatus = "PENDING"
		dataList[i].SchemaVersion = fmt.Sprintf("v%d", activeSchema.Version)

		if dataList[i].TrustScore == 0 {
			dataList[i].TrustScore = trustScore
		}

		if ok, msg := h.Service.ValidatePendudukMetadata(dataList[i], activeSchema.Definition); !ok {
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
		"status": "Ingestion Finished",
		"domain": "penduduk",
		"schema": activeSchema.Name + " v" + fmt.Sprint(activeSchema.Version),
		"stats":  fiber.Map{"total": len(dataList), "success": success, "fail": fail},
		"errors": errorLogs,
	})
}

func (h *PendudukHandler) GetValidationStatus(c *fiber.Ctx) error {
	result, err := h.Service.Storage.GetLatestByNIK(c.Params("nik"))
	status := "PENDING"
	if err == nil && result != nil {
		status = result.AuditStatus
	}
	return c.JSON(fiber.Map{"nik": c.Params("nik"), "status": status, "schema": "DTSEN-PENDDK-ACTIVE"})
}

func (h *PendudukHandler) GetScoring(c *fiber.Ctx) error {
	result, err := h.Service.Storage.GetLatestByNIK(c.Params("nik"))
	if err != nil || result == nil {
		return c.Status(404).JSON(fiber.Map{"error": "Data tidak ditemukan"})
	}
	return c.JSON(fiber.Map{
		"nik": result.NIK, "trust_score": result.TrustScore,
		"is_walidata": result.IsWaliData, "audit_status": result.AuditStatus,
		"quality_label": "Kalkulasi: 60(Sistem) + 20(Sumber) + 20(Audit)",
	})
}

func (h *PendudukHandler) GetProgress(c *fiber.Ctx) error {
	count, err := h.Service.Storage.CountByNIK(c.Params("nik"))
	status := "NOT_FOUND"
	if err == nil && count > 0 {
		status = "COMPLETED_IN_MESH"
	}
	return c.JSON(fiber.Map{"nik": c.Params("nik"), "status": status, "progress": "100%"})
}

func (h *PendudukHandler) GetAllDatasets(c *fiber.Ctx) error {
	fields := c.Query("fields")
	var fieldList []string
	if fields != "" {
		fieldList = strings.Split(fields, ",")
	}
	results, err := h.Service.Storage.GetFetchWithFields(fieldList)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Gagal mengambil data"})
	}
	return c.JSON(results)
}

func (h *PendudukHandler) GetDatasetDetail(c *fiber.Ctx) error {
	fields := c.Query("fields")
	var fieldList []string
	if fields != "" {
		fieldList = strings.Split(fields, ",")
	}
	result, err := h.Service.Storage.GetDetailWithFields(c.Params("nik"), fieldList)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Data tidak ditemukan"})
	}
	return c.JSON(result)
}

func (h *PendudukHandler) UpdateDataset(c *fiber.Ctx) error {
	var payload models.Penduduk
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Payload tidak valid"})
	}
	version, err := h.Service.ProcessManualUpdate(c.Params("nik"), payload)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "Versi baru dibuat, status kembali PENDING", "version": version})
}

func (h *PendudukHandler) SoftDeleteDataset(c *fiber.Ctx) error {
	if err := h.Service.Storage.SoftDelete(c.Params("id")); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Gagal nonaktifkan data"})
	}
	return c.JSON(fiber.Map{"message": "Data penduduk berhasil dinonaktifkan (Soft Delete)"})
}

func (h *PendudukHandler) GetAuditSamples(c *fiber.Ctx) error {
    // 1. Ambil limit dari query param, default ke 10 jika tidak diisi
    limit, err := strconv.Atoi(c.Query("limit", "10"))
    if err != nil {
        return c.Status(400).JSON(fiber.Map{"error": "Limit harus berupa angka"})
    }

    // 2. Oper 'limit' ke Service
    results, err := h.Service.Storage.GetAuditSamples(limit) 
    if err != nil {
        return c.Status(500).JSON(fiber.Map{"error": "Gagal mengambil data audit"})
    }
    
    if len(results) == 0 {
        return c.JSON(fiber.Map{"message": "Tidak ada data kependudukan terbaru yang perlu diaudit."})
    }
    
    return c.JSON(results)
}

func (h *PendudukHandler) SubmitAuditDecision(c *fiber.Ctx) error {
	var input struct {
		NIK     []string `json:"nik"`
		Verdict int      `json:"verdict"`
	}
	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Payload JSON tidak valid"})
	}
	if len(input.NIK) == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "List NIK tidak boleh kosong"})
	}

	verdictText, err := h.Service.ProcessAuditDecision(input.NIK, input.Verdict)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": fmt.Sprintf("Berhasil memproses %d NIK menjadi %s", len(input.NIK), verdictText)})
}
