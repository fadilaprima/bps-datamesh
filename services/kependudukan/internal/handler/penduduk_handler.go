package handler

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
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


// A. METADATA & SCHEMA MANAGEMENT (DINAMIS)
func (h *PendudukHandler) CreateSchemaHandler(c *fiber.Ctx) error {
	var input models.Schema
	if err := c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Payload JSON tidak valid"})
	}

	domain := c.Params("domain", "penduduk")

	// Archive skema lama
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

func (h *PendudukHandler) GetLatestSchemaHandler(c *fiber.Ctx) error {
	var schema models.Schema
	domain := c.Params("domain", "penduduk")
	if err := h.Service.Storage.DB.Where("domain = ? AND status = ?", domain, "ACTIVE").Order("version desc").First(&schema).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Skema aktif tidak ditemukan"})
	}
	return c.JSON(schema)
}


// B. DATA INGESTION (HYBRID DYNAMIC - WITH ADDITIONAL INFO)
func (h *PendudukHandler) IngestData(c *fiber.Ctx) error {
	// 1. Ambil Skema Aktif sebagai Kiblat Aturan
	var activeSchema models.Schema
	if err := h.Service.Storage.DB.Where("domain = ? AND status = ?", "penduduk", "ACTIVE").Order("version desc").First(&activeSchema).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Metadata belum siap, ingest ditolak"})
	}

	fileHeader, err := c.FormFile("document")
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "File tidak ditemukan"})
	}

	// 2. Skoring 60+20 (Sistem + Walidata)
	sourceID := strings.ToUpper(c.FormValue("source_id", "UNKNOWN"))
	isWali := false
	trustScore := 60.0
	if reg, exists := app.PendudukSourceRegistry[sourceID]; exists {
		isWali = reg.IsWali
		if isWali { trustScore += 20.0 }
	}

	refDate, _ := time.Parse("2006-01-02", c.FormValue("reference_date", time.Now().Format("2006-01-02")))
	file, _ := fileHeader.Open()
	defer file.Close()
	
	var dataList []models.Penduduk
	filename := strings.ToLower(fileHeader.Filename)

	// 3. PARSING LOGIC (CSV, PARQUET, JSON)
	if strings.HasSuffix(filename, ".csv") {
		r := csv.NewReader(file)
		records, _ := r.ReadAll()
		if len(records) < 2 { return c.Status(400).JSON(fiber.Map{"error": "CSV kosong"}) }
		
		headers := records[0]
		for i, rec := range records {
			if i == 0 { continue }
			
			// Misahkan kolom utama dan kolom tambahan (AdditionalInfo)
			extraData := make(map[string]interface{})
			p := models.Penduduk{
				SourceID: sourceID, IsWaliData: isWali, TrustScore: trustScore,
				ReferenceDate: refDate, AuditStatus: "PENDING",
			}

			for idx, val := range rec {
				key := strings.ToLower(headers[idx])
				switch key {
				// Atribut dari kode lama (Identik)
				case "nokk": p.NoKK = val
				case "nama_anggota": p.NamaAnggota = val
				case "nik": p.NIK = val
				case "nama": p.Nama = val
				case "jml_anggota": fmt.Sscanf(val, "%d", &p.JmlAnggota)
				case "tgl_lahir": p.TglLahir = val
				case "jenis_kelamin": p.JenisKelamin = val
				case "status_kawin": p.StatusKawin = val
				case "status_hubungan": p.StatusHubungan = val
				case "alamat": p.Alamat = val
				case "kode_prov": p.KodeProv = val
				case "kode_kab": p.KodeKab = val
				case "kode_kec": p.KodeKec = val
				case "kode_desa": p.KodeDesa = val
				case "alamat_ktp": p.AlamatKTP = val
				case "rt_ktp": p.RTKTP = val
				case "rw_ktp": p.RWKTP = val
				case "dusun_ktp": p.DusunKTP = val
				case "kode_prov_ktp": p.KodeProvKTP = val
				case "kode_kab_ktp": p.KodeKabKTP = val
				case "kode_kec_ktp": p.KodeKecKTP = val
				case "kode_desa_ktp": p.KodeDesaKTP = val
				default:
					// JSONB
					extraData[key] = val
				}
			}
			p.AdditionalInfo, _ = json.Marshal(extraData)
			dataList = append(dataList, p)
		}
	} else if strings.HasSuffix(filename, ".parquet") {
		// LOGIKA PARQUET (Identik dengan Pendidikan)
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
		// Logika JSON
		body, _ := io.ReadAll(file)
		json.Unmarshal(body, &dataList)
	}

	// 4. VALIDASI & PROSES (SCD TYPE 2)
	success, fail := 0, 0
	var errorLogs []string

	for _, p := range dataList {
		// Validasi Dinamis lewat Service
		if ok, msg := h.Service.ValidatePendudukMetadata(p, activeSchema.Definition); !ok {
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
		"status": "Finished",
		"schema": activeSchema.Name + " v" + fmt.Sprint(activeSchema.Version),
		"stats":  fiber.Map{"total": len(dataList), "success": success, "fail": fail},
		"errors": errorLogs,
	})
}