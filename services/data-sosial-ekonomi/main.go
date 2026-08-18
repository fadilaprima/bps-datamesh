package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/cors"
)

type DomainInfo struct {
	Name    string `json:"name"`
	BaseURL string `json:"base_url"`
	Owner   string `json:"owner"`
	Path    string `json:"path"`
	JoinKey string `json:"join_key"`
}

// Daftar 8 Domain BPS yang terhubung di Data Mesh
var meshRegistry = []DomainInfo{
	{Name: "Kependudukan", BaseURL: "http://localhost:8081", Owner: "BPS_DUKCAPIL", Path: "penduduk", JoinKey: "NIK"},
	{Name: "Pendidikan", BaseURL: "http://localhost:8082", Owner: "KEMENDIKBUD", Path: "pendidikan", JoinKey: "NIK"},
	{Name: "Wilayah", BaseURL: "http://localhost:8083", Owner: "BPS", Path: "wilayah", JoinKey: "KODE"},
	{Name: "Kesehatan", BaseURL: "http://localhost:8084", Owner: "KEMENKES", Path: "kesehatan", JoinKey: "NIK"},
	{Name: "Ketenagakerjaan", BaseURL: "http://localhost:8085", Owner: "BPJS_TK", Path: "ketenagakerjaan", JoinKey: "NIK"},
	{Name: "Kesejahteraan", BaseURL: "http://localhost:8086", Owner: "KEMENSOS", Path: "kesejahteraan", JoinKey: "NoKK"},
	{Name: "Hunian", BaseURL: "http://localhost:8087", Owner: "PUPR", Path: "hunian", JoinKey: "NoKK"},
	{Name: "Energi", BaseURL: "http://localhost:8088", Owner: "ESDM", Path: "energi", JoinKey: "NoKK"},
}

// 2. Stitching Data
func fetchFromDomain(url string, target interface{}) error {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Domain tidak dapat diakses/ data tidak ditemukan")
	}

	body, _ := io.ReadAll(resp.Body)
	return json.Unmarshal(body, target)
}

// 3. Mesin Dinamis: Filter Variabel
func applyDynamicFields(data map[string]interface{}, fields string) map[string]interface{} {
	if fields == "" {
		return data
	}
	selected := make(map[string]interface{})
	targetFields := strings.Split(fields, ",")
	for _, f := range targetFields {
		f = strings.TrimSpace(f)
		if val, ok := data[f]; ok {
			selected[f] = val
		}
	}
	return selected
}

func main() {
	app := fiber.New(fiber.Config{
		AppName: "Domain Core - Data Sosial Ekonomi",
	})
	app.Use(logger.New())

	api := app.Group("/api/v1/social-economy")

	// 1. GET ALL RECORD (Stitched & Dynamic Variables)
	api.Get("/datasets", func(c *fiber.Ctx) error {
		fields := c.Query("fields")
		var rawList []map[string]interface{}

		// Ambil list dasar dari Kependudukan
		urlPdk := fmt.Sprintf("%s/api/v1/domains/%s/datasets", meshRegistry[0].BaseURL, meshRegistry[0].Path)
		if err := fetchFromDomain(urlPdk, &rawList); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Gagal mengambil basis data kependudukan. Pastikan domain port 8081 menyala."})
		}

		var stitchedList []map[string]interface{}

		for _, person := range rawList {
			nik, _ := person["nomor_induk_kependudukan"].(string)
			nokk, _ := person["nomor_kartu_keluarga"].(string)
			kodeWil, _ := person["kode_kelurahan_desa"].(string)

			// Gabung dengan 7 domain lainnya
			for i := 1; i < len(meshRegistry); i++ {
				domain := meshRegistry[i]
				var sub map[string]interface{}
				var url string

				// Penentuan URL Dinamis berdasarkan jenis Key (NIK / NoKK / Kode)
				if domain.JoinKey == "NIK" && nik != "" {
					url = fmt.Sprintf("%s/api/v1/domains/%s/datasets/%s", domain.BaseURL, domain.Path, nik)
				} else if domain.JoinKey == "NoKK" && nokk != "" {
					url = fmt.Sprintf("%s/api/v1/domains/%s/datasets/%s", domain.BaseURL, domain.Path, nokk)
				} else if domain.JoinKey == "KODE" && kodeWil != "" {
					url = fmt.Sprintf("%s/api/v1/domains/%s/datasets/%s", domain.BaseURL, domain.Path, kodeWil)
				} else {
					continue // Jika kunci kosong, lewati domain ini
				}

				if err := fetchFromDomain(url, &sub); err == nil {
					for k, v := range sub {
						person[k] = v
					}
				}
			}

			// Filter variabel sesuai request user
			stitchedList = append(stitchedList, applyDynamicFields(person, fields))
		}

		return c.JSON(stitchedList)
	})

	// 2. GET SPESIFIK NIK (Stitched & Dynamic Variables)
	api.Get("/datasets/:nik", func(c *fiber.Ctx) error {
		nik := c.Params("nik")
		fields := c.Query("fields")

		var profile = make(map[string]interface{})

		// Stitching Step 1: Ambil Profil Kependudukan Utama
		urlPdk := fmt.Sprintf("%s/api/v1/domains/%s/datasets/%s", meshRegistry[0].BaseURL, meshRegistry[0].Path, nik)
		if err := fetchFromDomain(urlPdk, &profile); err != nil {
			return c.Status(404).JSON(fiber.Map{"error": "NIK tidak ditemukan di basis data kependudukan"})
		}

		// Ekstrak Kunci Logis untuk domain lain
		nokk, _ := profile["nomor_kartu_keluarga"].(string)
		kodeWil, _ := profile["kode_kelurahan_desa"].(string)

		// Stitching Step 2: Menjahit semua domain secara otomatis
		for i := 1; i < len(meshRegistry); i++ {
			domain := meshRegistry[i]
			var sub map[string]interface{}
			var url string

			if domain.JoinKey == "NIK" && nik != "" {
				url = fmt.Sprintf("%s/api/v1/domains/%s/datasets/%s", domain.BaseURL, domain.Path, nik)
			} else if domain.JoinKey == "NoKK" && nokk != "" {
				url = fmt.Sprintf("%s/api/v1/domains/%s/datasets/%s", domain.BaseURL, domain.Path, nokk)
			} else if domain.JoinKey == "KODE" && kodeWil != "" {
				url = fmt.Sprintf("%s/api/v1/domains/%s/datasets/%s", domain.BaseURL, domain.Path, kodeWil)
			} else {
				continue
			}

			// Gabungkan data jika berhasil ditarik
			if err := fetchFromDomain(url, &sub); err == nil {
				for k, v := range sub {
					profile[k] = v
				}
			}
		}

		return c.JSON(applyDynamicFields(profile, fields))
	})

	// 3. ENDPOINT DAFTAR DOMAIN (Mesh Registry Management)
	api.Get("/registry", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"mesh_status":    "Active",
			"total_domains":  len(meshRegistry),
			"active_domains": meshRegistry,
		})
	})

	// Tambah Domain Baru ke Mesh secara Dinamis
	api.Post("/registry", func(c *fiber.Ctx) error {
		var newDomain DomainInfo
		if err := c.BodyParser(&newDomain); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Format domain tidak valid"})
		}
		meshRegistry = append(meshRegistry, newDomain)
		return c.Status(201).JSON(fiber.Map{"message": "Domain " + newDomain.Name + " berhasil didaftarkan ke Mesh"})
	})

	fmt.Println("---------------------------------------------------------")
	fmt.Println("Core Domain: DATA SOSIAL EKONOMI is running")
	fmt.Println("Port: 8000")
	fmt.Println("---------------------------------------------------------")

	app.Listen(":8000")
}
