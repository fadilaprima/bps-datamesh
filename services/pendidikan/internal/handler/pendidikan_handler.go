package handler

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"pendidikan/internal/app"
	"pendidikan/models"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/xitongsys/parquet-go-source/local"
	"github.com/xitongsys/parquet-go/reader"
)

// PendidikanHandler mengelola request masuk via HTTP Fiber
type PendidikanHandler struct {
	Service app.PendidikanService
}

// ============================================================
// METADATA & SCHEMA MANAGEMENT (DINAMIS & VERSIONED)
// ============================================================

// CreateSchemaHandler handles POST & PATCH /api/v1/domains/pendidikan/schemas
// Digunakan untuk pendaftaran awal, penambahan, maupun pengurangan kolom metadata.
func (h *PendidikanHandler) CreateSchemaHandler(c *fiber.Ctx) error {
	var input models.Schema
	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Payload JSON tidak valid atau kosong"})
	}

	domain := c.Params("domain", "pendidikan")

	// 1. Archive skema lama yang sedang ACTIVE agar tidak ada dualisme standar
	h.Service.Storage.DB.Model(&models.Schema{}).
		Where("domain = ? AND status = ?", domain, "ACTIVE").
		Update("status", "ARCHIVED")

	// 2. Cari versi terakhir untuk auto-increment versioning
	var lastSchema models.Schema
	h.Service.Storage.DB.Where("domain = ?", domain).Order("version desc").First(&lastSchema)

	// 3. Setup Metadata Baru
	input.ID = uuid.New()
	input.Domain = domain
	input.Version = lastSchema.Version + 1
	input.Status = "ACTIVE"
	input.CreatedAt = time.Now()

	if err := h.Service.Storage.DB.Create(&input).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Gagal menyimpan evolusi skema ke database"})
	}

	return c.Status(201).JSON(fiber.Map{
		"message": "Schema Evolution Berhasil",
		"info":    fmt.Sprintf("Domain %s kini menggunakan Versi %d", domain, input.Version),
		"data":    input,
	})
}

// GetLatestSchemaHandler handles GET /api/v1/domains/pendidikan/schemas/latest
// Menampilkan 'Kiblat' aturan metadata yang sedang aktif.
func (h *PendidikanHandler) GetLatestSchemaHandler(c *fiber.Ctx) error {
	var schema models.Schema
	domain := c.Params("domain", "pendidikan")

	if err := h.Service.Storage.DB.Where("domain = ? AND status = ?", domain, "ACTIVE").
		Order("version desc").First(&schema).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{
			"error": "Skema aktif tidak ditemukan",
			"hint":  "Silahkan daftar skema dulu di POST /api/v1/domains/pendidikan/schemas",
		})
	}

	return c.JSON(fiber.Map{
		"domain":         schema.Domain,
		"schema_name":    schema.Name,
		"version":        schema.Version,
		"status":         schema.Status,
		"last_updated":   schema.CreatedAt,
		"metadata_rules": schema.Definition, // Menunjukkan list kolom/rules secara dinamis
	})
}

// ============================================================
// DATA INGESTION (60% SISTEM + 20% SUMBER + 20% AUDIT)
// ============================================================

func (h *PendidikanHandler) IngestData(c *fiber.Ctx) error {
	// 1. CEK SKEMA AKTIF (Wajib ada sebagai validasi dasar)
	var activeSchema models.Schema
	if err := h.Service.Storage.DB.Where("domain = ? AND status = ?", "pendidikan", "ACTIVE").
		Order("version desc").First(&activeSchema).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Metadata aktif belum terdaftar. Ingesti ditolak."})
	}

	// 2. TERIMA FILE
	fileHeader, err := c.FormFile("document")
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "File document tidak ditemukan pada form-data"})
	}

	// 3. LOGIKA SKORING BINER (60 Sistem + 20 Sumber)
	// Skor 60 diberikan otomatis karena lolos validasi skema aktif (Hard-gate)
	sourceID := strings.ToUpper(c.FormValue("source_id", "UNKNOWN"))
	isWali := false
	trustScore := 60.0 

	if reg, exists := app.PendidikanSourceRegistry[sourceID]; exists {
		isWali = reg.IsWali
		if isWali {
			trustScore += 20.0 // Tambah 20 poin jika pengirim adalah Walidata (Dinas)
		}
	}

	refDateStr := c.FormValue("reference_date", time.Now().Format("2006-01-02"))
	refDate, _ := time.Parse("2006-01-02", refDateStr)

	file, _ := fileHeader.Open()
	defer file.Close()
	filename := strings.ToLower(fileHeader.Filename)

	var dataList []models.RiwayatPendidikan

	// 4. PARSING LOGIC (CSV, JSON, PARQUET)
	if strings.HasSuffix(filename, ".csv") {
		r := csv.NewReader(file)
		records, _ := r.ReadAll()
		for i, rec := range records {
			if i == 0 { continue } // Skip Header
			if len(rec) < 5 { continue } // Safety check agar tidak index out of range

			dataList = append(dataList, models.RiwayatPendidikan{
				NIK:           rec[0],
				Partisipasi:   rec[1],
				Jenjang:       rec[2],
				Kelas:         rec[3],
				Ijazah:        rec[4],
				SourceID:      sourceID,
				IsWaliData:    isWali,
				TrustScore:    trustScore,
				ReferenceDate: refDate,
				AuditStatus:   "PENDING", // Menunggu 20 poin sisa dari Admin BPS
			})
		}
	} else if strings.HasSuffix(filename, ".json") {
		body, _ := io.ReadAll(file)
		if err := json.Unmarshal(body, &dataList); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Format JSON data tidak sesuai"})
		}
	} else if strings.HasSuffix(filename, ".parquet") {
		tmpPath := fmt.Sprintf("temp_%d.parquet", time.Now().UnixNano())
		fw, _ := os.Create(tmpPath)
		io.Copy(fw, file)
		fw.Close()

		fr, _ := local.NewLocalFileReader(tmpPath)
		pr, errP := reader.NewParquetReader(fr, new(models.RiwayatPendidikan), 4)
		if errP == nil {
			num := int(pr.GetNumRows())
			res := make([]models.RiwayatPendidikan, num)
			pr.Read(&res)
			pr.ReadStop()
			fr.Close()
			os.Remove(tmpPath)
			dataList = res
		}
	}

	// 5. INJECT METADATA UNTUK NON-CSV
	if !strings.HasSuffix(filename, ".csv") {
		for i := range dataList {
			dataList[i].SourceID = sourceID
			dataList[i].IsWaliData = isWali
			dataList[i].TrustScore = trustScore
			dataList[i].ReferenceDate = refDate
			dataList[i].AuditStatus = "PENDING"
		}
	}

	// 6. VALIDATION & PROCESSING (SCD TYPE 2)
	success, fail := 0, 0
	var errorLogs []string

	for _, p := range dataList {
		if ok, msg := h.Service.ValidatePendidikanMetadata(p); !ok {
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
		"status":      "Ingestion Finished",
		"schema_used": activeSchema.Name + " (v" + fmt.Sprint(activeSchema.Version) + ")",
		"stats":       fiber.Map{"total": len(dataList), "success": success, "fail": fail},
		"base_score":  trustScore,
		"next_step":   "Silahkan lakukan audit di BPS Governance untuk mendapatkan sisa 20 poin",
		"errors":      errorLogs,
	})
}