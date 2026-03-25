<<<<<<< HEAD
package handler

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"kependudukan/internal/app"
	"kependudukan/models"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/xitongsys/parquet-go-source/local"
	"github.com/xitongsys/parquet-go/reader"
)

// PendudukHandler mengelola request masuk via HTTP Fiber
type PendudukHandler struct {
	Service app.PendudukService
}

// Mendukung interoperabilitas format data (CSV, JSON, Parquet) sesuai prinsip Data Mesh
func (h *PendudukHandler) IngestData(c *fiber.Ctx) error {
	// 1. Ambil File dari Form
	fileHeader, err := c.FormFile("document")
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "File tidak ditemukan"})
	}

	// 2. Mapping Metadata Pengirim (Source Registry Logic)
	sourceID := strings.ToUpper(c.FormValue("source_id", "UNKNOWN"))
	var isWali bool
	var trustScore float64

	// Mengacu pada PendudukSourceRegistry di Service
	if reg, exists := app.PendudukSourceRegistry[sourceID]; exists {
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

	var dataList []models.Penduduk

	// 3. PARSING LOGIC (CSV, JSON, PARQUET)
	if strings.HasSuffix(filename, ".csv") {
		r := csv.NewReader(file)
		records, _ := r.ReadAll()
		for i, rec := range records {
			if i == 0 { continue } // Skip header
			
			jml, _ := strconv.Atoi(rec[4]) // jumlah_anggota_keluarga
			
			dataList = append(dataList, models.Penduduk{
				// Mapping Identitas & KK
				NoKK: rec[0], NamaAnggota: rec[1], NIK: rec[2], Nama: rec[3], JmlAnggota: jml,
				TglLahir: rec[5], JenisKelamin: rec[6], StatusKawin: rec[7], StatusHubungan: rec[8],
				
				// Mapping Wilayah Domisili
				Alamat: rec[9], KodeProv: rec[10], KodeKab: rec[11], KodeKec: rec[12], KodeDesa: rec[13],
				
				// Mapping Wilayah KTP
				AlamatKTP: rec[14], RTKTP: rec[15], RWKTP: rec[16], DusunKTP: rec[17],
				KodeProvKTP: rec[18], KodeKabKTP: rec[19], KodeKecKTP: rec[20], KodeDesaKTP: rec[21],
				
				// Metadata Ingesti
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
	}

	// Inject Metadata untuk non-CSV agar tetap patuh pada Governance
	if !strings.HasSuffix(filename, ".csv") {
		for i := range dataList {
			dataList[i].SourceID = sourceID
			dataList[i].IsWaliData = isWali
			dataList[i].TrustScore = trustScore
			dataList[i].ReferenceDate = refDate
		}
	}

	// 4. VALIDATION & PROCESSING LOOP (Menggunakan Service)
	success, fail := 0, 0
	var errorLogs []string

	for _, p := range dataList {
		// A. Panggil Validasi Metadata dari Service (13+ Variabel)
		if ok, msg := h.Service.ValidatePendudukMetadata(p); !ok {
			fail++
			errorLogs = append(errorLogs, fmt.Sprintf("NIK %s: %s", p.NIK, msg))
			continue
		}

		// B. Panggil Logika Ingesti (SCD Type 2 & Rule-Based Merge)
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
=======
package handler

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"kependudukan/internal/app"
	"kependudukan/models"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/xitongsys/parquet-go-source/local"
	"github.com/xitongsys/parquet-go/reader"
)

// PendudukHandler mengelola request masuk via HTTP Fiber
type PendudukHandler struct {
	Service app.PendudukService
}

// Mendukung interoperabilitas format data (CSV, JSON, Parquet) sesuai prinsip Data Mesh
func (h *PendudukHandler) IngestData(c *fiber.Ctx) error {
	// 1. Ambil File dari Form
	fileHeader, err := c.FormFile("document")
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "File tidak ditemukan"})
	}

	// 2. Mapping Metadata Pengirim (Source Registry Logic)
	sourceID := strings.ToUpper(c.FormValue("source_id", "UNKNOWN"))
	var isWali bool
	var trustScore float64

	// Mengacu pada PendudukSourceRegistry di Service
	if reg, exists := app.PendudukSourceRegistry[sourceID]; exists {
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

	var dataList []models.Penduduk

	// 3. PARSING LOGIC (CSV, JSON, PARQUET)
	if strings.HasSuffix(filename, ".csv") {
		r := csv.NewReader(file)
		records, _ := r.ReadAll()
		for i, rec := range records {
			if i == 0 { continue } // Skip header
			
			jml, _ := strconv.Atoi(rec[4]) // jumlah_anggota_keluarga
			
			dataList = append(dataList, models.Penduduk{
				// Mapping Identitas & KK
				NoKK: rec[0], NamaAnggota: rec[1], NIK: rec[2], Nama: rec[3], JmlAnggota: jml,
				TglLahir: rec[5], JenisKelamin: rec[6], StatusKawin: rec[7], StatusHubungan: rec[8],
				
				// Mapping Wilayah Domisili
				Alamat: rec[9], KodeProv: rec[10], KodeKab: rec[11], KodeKec: rec[12], KodeDesa: rec[13],
				
				// Mapping Wilayah KTP
				AlamatKTP: rec[14], RTKTP: rec[15], RWKTP: rec[16], DusunKTP: rec[17],
				KodeProvKTP: rec[18], KodeKabKTP: rec[19], KodeKecKTP: rec[20], KodeDesaKTP: rec[21],
				
				// Metadata Ingesti
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
	}

	// Inject Metadata untuk non-CSV agar tetap patuh pada Governance
	if !strings.HasSuffix(filename, ".csv") {
		for i := range dataList {
			dataList[i].SourceID = sourceID
			dataList[i].IsWaliData = isWali
			dataList[i].TrustScore = trustScore
			dataList[i].ReferenceDate = refDate
		}
	}

	// 4. VALIDATION & PROCESSING LOOP (Menggunakan Service)
	success, fail := 0, 0
	var errorLogs []string

	for _, p := range dataList {
		// A. Panggil Validasi Metadata dari Service (13+ Variabel)
		if ok, msg := h.Service.ValidatePendudukMetadata(p); !ok {
			fail++
			errorLogs = append(errorLogs, fmt.Sprintf("NIK %s: %s", p.NIK, msg))
			continue
		}

		// B. Panggil Logika Ingesti (SCD Type 2 & Rule-Based Merge)
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
>>>>>>> a935de74a96c3dd35516385ff2e46b34b9d60ef1
}