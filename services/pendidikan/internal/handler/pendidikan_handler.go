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
	"github.com/xitongsys/parquet-go-source/local"
	"github.com/xitongsys/parquet-go/reader"
)

// PendidikanHandler mengelola request masuk via HTTP Fiber
type PendidikanHandler struct {
	Service app.PendidikanService
}

// IngestData handles POST /api/v1/domains/pendidikan/submissions
// Mendukung interoperabilitas format data (CSV, JSON, Parquet) s
func (h *PendidikanHandler) IngestData(c *fiber.Ctx) error {
	// 1. Ambil File dari Form
	fileHeader, err := c.FormFile("document")
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "File tidak ditemukan"})
	}

	// 2. Mapping Metadata Pengirim (Source Registry Logic)
	sourceID := strings.ToUpper(c.FormValue("source_id", "UNKNOWN"))
	var isWali bool
	var trustScore float64

	// Mengacu pada PendidikanSourceRegistry di Service Pendidikan
	if reg, exists := app.PendidikanSourceRegistry[sourceID]; exists {
		isWali = reg.IsWali
		trustScore = reg.TrustScore
	} else {
		isWali = false
		trustScore = 0.7 // Default score sesuai kode lama untuk sumber non-registrasi
	}

	refDateStr := c.FormValue("reference_date", time.Now().Format("2006-01-02"))
	refDate, _ := time.Parse("2006-01-02", refDateStr)

	file, _ := fileHeader.Open()
	defer file.Close()
	filename := strings.ToLower(fileHeader.Filename)

	var dataList []models.RiwayatPendidikan

	// 3. PARSING LOGIC (CSV, JSON, PARQUET)
	if strings.HasSuffix(filename, ".csv") {
		r := csv.NewReader(file)
		records, _ := r.ReadAll()
		for i, rec := range records {
			if i == 0 { continue } // Skip header
			dataList = append(dataList, models.RiwayatPendidikan{
				// Mapping variabel sesuai Metadata Riwayat Pendidikan
				NIK:         rec[0],
				Partisipasi: rec[1],
				Jenjang:     rec[2],
				Kelas:       rec[3],
				Ijazah:      rec[4],
				SourceID:    sourceID,
				IsWaliData:  isWali,
				TrustScore:  trustScore,
				ReferenceDate: refDate,
			})
		}
	} else if strings.HasSuffix(filename, ".json") {
		body, _ := io.ReadAll(file)
		json.Unmarshal(body, &dataList)
	} else if strings.HasSuffix(filename, ".parquet") {
		tmpPath := "temp_" + filename
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

	// Inject Metadata untuk non-CSV
	if !strings.HasSuffix(filename, ".csv") {
		for i := range dataList {
			dataList[i].SourceID = sourceID
			dataList[i].IsWaliData = isWali
			dataList[i].TrustScore = trustScore
			dataList[i].ReferenceDate = refDate
		}
	}

	// 4. VALIDATION & PROCESSING LOOP
	success, fail := 0, 0
	var errorLogs []string

	for _, p := range dataList {
		// Panggil Validasi dari Service Pendidikan
		if ok, msg := h.Service.ValidatePendidikanMetadata(p); !ok {
			fail++
			errorLogs = append(errorLogs, fmt.Sprintf("NIK %s: %s", p.NIK, msg))
			continue
		}

		// Panggil Logika Ingesti (SCD Type 2 & Rule-Based Merge) dari Service Pendidikan
		_, err := h.Service.ProcessIngestion(p)
		if err != nil {
			fail++
			errorLogs = append(errorLogs, fmt.Sprintf("NIK %s: %v", p.NIK, err))
		} else {
			success++
		}
	}

	return c.JSON(fiber.Map{
		"status": "Ingestion Finished",
		"stats":  fiber.Map{"total": len(dataList), "success": success, "fail": fail},
		"errors": errorLogs,
	})
}