package handler

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"wilayah/internal/app"
	"wilayah/models"

	"github.com/gofiber/fiber/v2"
	"github.com/xitongsys/parquet-go-source/local"
	"github.com/xitongsys/parquet-go/reader"
)

// WilayahHandler mengelola request masuk via HTTP Fiber
type WilayahHandler struct {
	Service app.WilayahService
}

// Mendukung interoperabilitas format data (CSV, JSON, Parquet) sesuai prinsip Data Mesh
func (h *WilayahHandler) IngestData(c *fiber.Ctx) error {
	// 1. Ambil File dari Form
	fileHeader, err := c.FormFile("document")
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "File tidak ditemukan"})
	}

	// 2. Mapping Metadata Pengirim (Source Registry Logic)
	sourceID := strings.ToUpper(c.FormValue("source_id", "UNKNOWN"))
	var isWali bool
	var trustScore float64

	if reg, exists := app.SourceRegistry[sourceID]; exists {
		isWali = reg.IsWali
		trustScore = reg.TrustScore
	} else {
		isWali = false
		trustScore = 0.8 // Default score untuk sumber non-registrasi
	}

	refDateStr := c.FormValue("reference_date", time.Now().Format("2006-01-02"))
	refDate, _ := time.Parse("2006-01-02", refDateStr)

	file, _ := fileHeader.Open()
	defer file.Close()
	filename := strings.ToLower(fileHeader.Filename)

	var dataList []models.MasterWilayah

	// 3. PARSING LOGIC (CSV, JSON, PARQUET) - Dipindahkan dari main lama
	if strings.HasSuffix(filename, ".csv") {
		r := csv.NewReader(file)
		records, _ := r.ReadAll()
		for i, rec := range records {
			if i == 0 { continue } // Skip header
			dataList = append(dataList, models.MasterWilayah{
				KodeProv: rec[0], Provinsi: rec[1], KodeKab: rec[2], Kabupaten: rec[3],
				KodeKec: rec[4], Kecamatan: rec[5], KodeDesa: rec[6], Desa: rec[7],
				SourceID: sourceID, IsWaliData: isWali, TrustScore: trustScore, ReferenceDate: refDate,
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

	for _, w := range dataList {
		// Panggil Validasi dari Service
		if ok, msg := h.Service.ValidateWilayahMetadata(w); !ok {
			fail++
			errorLogs = append(errorLogs, fmt.Sprintf("Kode %s: %s", w.KodeDesa, msg))
			continue
		}

		// Panggil Logika Ingesti (SCD Type 2 & Conflict Resolution) dari Service
		_, err := h.Service.ProcessIngestion(w)
		if err != nil {
			fail++
			errorLogs = append(errorLogs, fmt.Sprintf("Kode %s: %v", w.KodeDesa, err))
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