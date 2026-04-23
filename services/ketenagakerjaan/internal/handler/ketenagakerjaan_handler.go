package handler

import (
	"encoding/csv"
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"time"

	"ketenagakerjaan/internal/app"
	"ketenagakerjaan/models"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type KetenagakerjaanHandler struct {
	Service app.KetenagakerjaanService
}

func (h *KetenagakerjaanHandler) IngestData(c *fiber.Ctx) error {
	var activeSchema models.Schema
	h.Service.Storage.DB.Where("domain = ? AND status = ?", "ketenagakerjaan", "ACTIVE").Order("version desc").First(&activeSchema)

	fileHeader, _ := c.FormFile("document")
	sourceID := strings.ToUpper(c.FormValue("source_id", "UNKNOWN"))
	refDate, _ := time.Parse("2006-01-02", c.FormValue("reference_date", time.Now().Format("2006-01-02")))
	file, _ := fileHeader.Open()
	defer file.Close()

	var dataList []models.RekamKetenagakerjaan
	filename := strings.ToLower(fileHeader.Filename)

	if strings.HasSuffix(filename, ".csv") {
		r := csv.NewReader(file)
		records, _ := r.ReadAll()
		headers := records[0]
		for i, rec := range records {
			if i == 0 { continue }
			p := models.RekamKetenagakerjaan{SourceID: sourceID, ReferenceDate: refDate, AuditStatus: "PENDING"}
			extra := make(map[string]interface{})
			for idx, val := range rec {
				key := strings.ToLower(headers[idx])
				iv, _ := strconv.Atoi(val)
				switch key {
				case "nik", "nomor_induk_kependudukan": p.NIK = val
				case "status_bekerja": p.StatusBekerja = val
				case "lapangan_usaha_dari_pekerjaan_utama": p.LapanganUsahaUtama = val
				case "status_dalam_pekerjaan_utama": p.StatusPekerjaanUtama = val
				case "kepemilikan_usaha": p.KepemilikanUsaha = val
				case "jumlah_usaha": p.JumlahUsaha = iv
				case "lapangan_usaha_dari_usaha_utama": p.LapanganUsahaUsahaUtama = val
				case "jumlah_pekerja_yang_dibayar_dari_usaha_utama": p.JumlahPekerjaDibayar = iv
				case "jumlah_pekerja_yang_tidak_dibayar_dari_usaha_utama": p.JumlahPekerjaTidakDibayar = iv
				case "omzet_usaha_utama": p.OmzetUsahaUtama = val
				default: extra[key] = val
				}
			}
			p.AdditionalInfo, _ = json.Marshal(extra)
			dataList = append(dataList, p)
		}
	} else {
		body, _ := io.ReadAll(file)
		json.Unmarshal(body, &dataList)
	}

	for _, p := range dataList {
		h.Service.ProcessIngestion(p)
	}
	return c.JSON(fiber.Map{"status": "Finished", "total": len(dataList)})
}

func (h *KetenagakerjaanHandler) CreateSchemaHandler(c *fiber.Ctx) error {
	var input models.Schema
	c.BodyParser(&input)
	input.ID = uuid.New()
	input.Domain = "ketenagakerjaan"
	input.Status = "ACTIVE"
	h.Service.Storage.DB.Create(&input)
	return c.Status(201).JSON(input)
}

func (h *KetenagakerjaanHandler) GetLatestSchemaHandler(c *fiber.Ctx) error {
	var schema models.Schema
	h.Service.Storage.DB.Where("domain = ? AND status = ?", "ketenagakerjaan", "ACTIVE").Order("version desc").First(&schema)
	return c.JSON(schema)
}