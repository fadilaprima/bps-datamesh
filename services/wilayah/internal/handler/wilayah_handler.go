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

// A. METADATA & SCHEMA MANAGEMENT
func (h *WilayahHandler) CreateSchemaHandler(c *fiber.Ctx) error {
	var input models.Schema
	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Payload JSON tidak valid"})
	}

	domain := c.Params("domain", "wilayah")

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
		return c.Status(500).JSON(fiber.Map{"error": "Gagal simpan skema wilayah"})
	}
	return c.Status(201).JSON(input)
}

func (h *WilayahHandler) GetLatestSchemaHandler(c *fiber.Ctx) error {
	var schema models.Schema
	if err := h.Service.Storage.DB.Where("domain = ? AND status = ?", "wilayah", "ACTIVE").Order("version desc").First(&schema).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Skema wilayah aktif tidak ditemukan"})
	}
	return c.JSON(schema)
}

// B. DATA INGESTION
func (h *WilayahHandler) IngestData(c *fiber.Ctx) error {
	// 1. Ambil Skema Aktif (Data Mesh Governance)
	var activeSchema models.Schema
	if err := h.Service.Storage.DB.Where("domain = ? AND status = ?", "wilayah", "ACTIVE").Order("version desc").First(&activeSchema).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Metadata belum siap, ingest ditolak"})
	}

	fileHeader, err := c.FormFile("document")
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "File dokumen tidak ditemukan"})
	}

	// 2. PENERJEMAH DROPDOWN ANGKA KHUSUS SOURCE ID
	sourceIDStr := c.FormValue("source_id", "3")
	sourceIDInt, _ := strconv.Atoi(sourceIDStr)
	sourceName := "LAINNYA"
	isWali := false
	trustScore := 60.0

	// Cocokkan angka dengan kamus di Service
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

	var dataList []models.MasterWilayah
	filename := strings.ToLower(fileHeader.Filename)

	// 3. PARSING LOGIC (CSV AUTO-DETECTION & PARQUET SUPPORT - REFLECT ENGINE)
	if strings.HasSuffix(filename, ".csv") {
		r := csv.NewReader(file)
		records, _ := r.ReadAll()
		if len(records) < 2 {
			return c.Status(400).JSON(fiber.Map{"error": "CSV kosong"})
		}
		headers := records[0]
		wilayahType := reflect.TypeOf(models.MasterWilayah{})

		for i, rec := range records {
			if i == 0 {
				continue
			}
			extraData := make(map[string]interface{})
			w := models.MasterWilayah{ReferenceDate: refDate}
			wilayahValue := reflect.ValueOf(&w).Elem()

			for idx, val := range rec {
				if idx >= len(headers) {
					continue
				}
				key := strings.ToLower(headers[idx])
				found := false

				for fIdx := 0; fIdx < wilayahType.NumField(); fIdx++ {
					field := wilayahType.Field(fIdx)
					jsonTag := strings.Split(field.Tag.Get("json"), ",")[0]
					if jsonTag == key {
						found = true
						fieldVal := wilayahValue.Field(fIdx)
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
			w.AdditionalInfo, _ = json.Marshal(extraData)
			dataList = append(dataList, w)
		}
	} else if strings.HasSuffix(filename, ".parquet") {
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
		if ok, msg := h.Service.ValidateWilayahMetadata(dataList[i], activeSchema.Definition); !ok {
			fail++
			errorLogs = append(errorLogs, fmt.Sprintf("KodeDesa %s: %s", dataList[i].KodeDesa, msg))
			continue
		}

		// c. Simpan ke Database
		if _, err := h.Service.ProcessIngestion(dataList[i]); err != nil {
			fail++
			errorLogs = append(errorLogs, fmt.Sprintf("KodeDesa %s: %v", dataList[i].KodeDesa, err))
		} else {
			success++
		}
	}

	return c.JSON(fiber.Map{
		"status": "Ingestion Finished",
		"schema": activeSchema.Name,
		"stats":  fiber.Map{"total": len(dataList), "success": success, "fail": fail},
		"errors": errorLogs,
	})
}

func (h *WilayahHandler) GetValidationStatus(c *fiber.Ctx) error {
	result, err := h.Service.Storage.GetLatestByKode(c.Params("kode"))
	status := "PENDING"
	if err == nil && result != nil {
		status = result.AuditStatus
	}
	return c.JSON(fiber.Map{"kode_desa": c.Params("kode"), "status": status})
}

func (h *WilayahHandler) GetScoring(c *fiber.Ctx) error {
	result, err := h.Service.Storage.GetLatestByKode(c.Params("kode"))
	if err != nil || result == nil {
		return c.Status(404).JSON(fiber.Map{"error": "Data wilayah tidak ditemukan"})
	}
	return c.JSON(fiber.Map{
		"kode_desa": result.KodeDesa, "trust_score": result.TrustScore,
		"is_walidata": result.IsWaliData, "audit_status": result.AuditStatus,
		"quality_label": "Kalkulasi: 60(Sistem) + 20(Sumber) + 20(Audit)",
	})
}

func (h *WilayahHandler) GetProgress(c *fiber.Ctx) error {
	count, err := h.Service.Storage.CountByKode(c.Params("kode"))
	status := "NOT_FOUND"
	if err == nil && count > 0 {
		status = "COMPLETED_IN_MESH"
	}
	return c.JSON(fiber.Map{"kode_desa": c.Params("kode"), "status": status, "progress": "100%"})
}

func (h *WilayahHandler) GetAllDatasets(c *fiber.Ctx) error {
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

func (h *WilayahHandler) GetDatasetDetail(c *fiber.Ctx) error {
	fields := c.Query("fields")
	var fieldList []string
	if fields != "" {
		fieldList = strings.Split(fields, ",")
	}
	result, err := h.Service.Storage.GetDetailWithFields(c.Params("kode"), fieldList)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Data tidak ditemukan"})
	}
	return c.JSON(result)
}

func (h *WilayahHandler) UpdateDataset(c *fiber.Ctx) error {
	var payload models.MasterWilayah
	if err := c.BodyParser(&payload); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Payload tidak valid"})
	}

	version, err := h.Service.ProcessManualUpdate(c.Params("kode"), payload)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": "Versi baru dibuat (Koreksi)", "version": version})
}

func (h *WilayahHandler) SoftDeleteDataset(c *fiber.Ctx) error {
	if err := h.Service.Storage.SoftDelete(c.Params("id")); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Gagal nonaktifkan data"})
	}
	return c.JSON(fiber.Map{"message": "Soft delete berhasil"})
}

func (h *WilayahHandler) GetAuditSamples(c *fiber.Ctx) error {
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

func (h *WilayahHandler) SubmitAuditDecision(c *fiber.Ctx) error {
	var input struct {
		KodeDesa []string `json:"kode_kelurahan_desa"`
		Verdict  int      `json:"verdict"`
	}
	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Payload tidak valid"})
	}
	if len(input.KodeDesa) == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "Kode desa wajib diisi"})
	}

	verdictText, err := h.Service.ProcessAuditDecision(input.KodeDesa, input.Verdict)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"message": fmt.Sprintf("Audit %d desa selesai: %s", len(input.KodeDesa), verdictText)})
}
