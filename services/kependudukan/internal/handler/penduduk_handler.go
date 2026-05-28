package handler

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
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

// B. DATA INGESTION
func (h *PendudukHandler) IngestData(c *fiber.Ctx) error {
	// 1. Ambil Skema Aktif (Data Mesh Governance)
	var activeSchema models.Schema
	if err := h.Service.Storage.DB.Where("domain = ? AND status = ?", "penduduk", "ACTIVE").Order("version desc").First(&activeSchema).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Metadata kependudukan belum siap, ingest ditolak"})
	}

	fileHeader, err := c.FormFile("document")
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "File dokumen (CSV/Parquet) tidak ditemukan"})
	}

	// 2. PENERJEMAH DROPDOWN ANGKA KHUSUS SOURCE ID
	sourceIDStr := c.FormValue("source_id", "3") // Default: 3 (LAINNYA)
	sourceIDInt, _ := strconv.Atoi(sourceIDStr)

	sourceName := "LAINNYA"
	isWali := false
	trustScore := 60.0

	// Cocokkan angka dengan kamus di Service (Kependudukan)
	if config, exists := app.SourceMap[sourceIDInt]; exists {
		sourceName = config.Name
		isWali = config.IsWali
		if isWali {
			trustScore += 20.0
		}
	} else {
		// Pesan error diubah merujuk ke wali data kependudukan
		return c.Status(400).JSON(fiber.Map{"error": "source_id tidak valid. Gunakan: 1 (BPS), 2 (KEMENDGRI), 3 (LAINNYA)"})
	}

	refDate, _ := time.Parse("2006-01-02", c.FormValue("reference_date", time.Now().Format("2006-01-02")))
	file, _ := fileHeader.Open()
	defer file.Close()

	var dataList []models.Penduduk
	filename := strings.ToLower(fileHeader.Filename)

	// 3. PARSING LOGIC (CSV AUTO-DETECTION & PARQUET SUPPORT)
	if strings.HasSuffix(filename, ".csv") {
		r := csv.NewReader(file)
		records, _ := r.ReadAll()
		if len(records) < 2 {
			return c.Status(400).JSON(fiber.Map{"error": "CSV kosong"})
		}

		headers := records[0]
		for i, rec := range records {
			if i == 0 {
				continue
			}

			// Inisialisasi bersih, sama seperti di wilayah
			extraData := make(map[string]interface{})
			p := models.Penduduk{
				ReferenceDate: refDate,
			}

			// Mapping agar tidak nyasar ke JSONB
			for idx, val := range rec {
				key := strings.ToLower(headers[idx])
				switch key {

				// 1. DATA IDENTITAS
				case "nomor_induk_kependudukan":
					p.NIK = val
				case "nomor_kartu_keluarga":
					p.NoKK = val
				case "nama":
					p.Nama = val
				case "nama_anggota_keluarga":
					p.NamaAnggota = val
				case "jumlah_anggota_keluarga":
					fmt.Sscanf(val, "%d", &p.JmlAnggota)

				// 2. DATA DEMOGRAFI & STATUS
				case "tanggal_lahir":
					p.TglLahir = val
				case "jenis_kelamin":
					p.JenisKelamin = val
				case "status_kawin":
					p.StatusKawin = val
				case "status_hubungan_keluarga":
					p.StatusHubungan = val

				// 3. DATA WILAYAH DOMISILI
				case "alamat":
					p.Alamat = val
				case "kode_provinsi":
					p.KodeProv = val
				case "kode_kabupaten_kota":
					p.KodeKab = val
				case "kode_kecamatan":
					p.KodeKec = val
				case "kode_kelurahan_desa":
					p.KodeDesa = val

				// 4. DATA WILAYAH KTP
				case "alamat_ktp":
					p.AlamatKTP = val
				case "rt_ktp":
					p.RTKTP = val
				case "rw_ktp":
					p.RWKTP = val
				case "dusun_ktp":
					p.DusunKTP = val
				case "kode_provinsi_ktp":
					p.KodeProvKTP = val
				case "kode_kabupaten_kota_ktp":
					p.KodeKabKTP = val
				case "kode_kec_ktp", "kode_kecamatan_ktp":
					p.KodeKecKTP = val
				case "kode_kelurahan_desa_ktp":
					p.KodeDesaKTP = val
				default:
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

	// 4. VALIDASI & PROSES (SCD TYPE 2)
	success, fail := 0, 0
	var errorLogs []string

	// Lakukan injeksi data otoritas & validasi ke semua baris data
	for i := range dataList {
		// a. INJEKSI KEAMANAN & GOVERNANCE
		dataList[i].SourceID = sourceName
		dataList[i].IsWaliData = isWali
		dataList[i].AuditStatus = "PENDING"
		dataList[i].SchemaVersion = fmt.Sprintf("v%d", activeSchema.Version)

		if dataList[i].TrustScore == 0 {
			dataList[i].TrustScore = trustScore
		}

		// b. Validasi Dinamis lewat Service
		if ok, msg := h.Service.ValidatePendudukMetadata(dataList[i], activeSchema.Definition); !ok {
			fail++
			errorLogs = append(errorLogs, fmt.Sprintf("NIK %s: %s", dataList[i].NIK, msg))
			continue
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
		"status":   "Ingestion Finished",
		"domain":   "penduduk",
		"schema":   activeSchema.Name + " v" + fmt.Sprint(activeSchema.Version),
		"stats":    fiber.Map{"total": len(dataList), "success": success, "fail": fail},
		"errors":   errorLogs,
	})
}