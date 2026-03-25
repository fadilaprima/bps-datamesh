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
)

// 1. Registry: Peta Kekuatan Data Mesh
type DomainInfo struct {
	Name    string `json:"name"`
	BaseURL string `json:"base_url"`
	Owner   string `json:"owner"`
}

var meshRegistry = []DomainInfo{
	{Name: "Kependudukan", BaseURL: "http://localhost:8081", Owner: "DUKCAPIL"},
	{Name: "Pendidikan", BaseURL: "http://localhost:8082", Owner: "KEMENDIKBUD"},
	{Name: "Wilayah", BaseURL: "http://localhost:8083", Owner: "BPS"},
}

// 2. Mesin Stitching: Helper Fetch Data antar Domain
func fetchFromDomain(url string, target interface{}) error {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil { return err }
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK { return fmt.Errorf("domain unreachable") }
	body, _ := io.ReadAll(resp.Body)
	return json.Unmarshal(body, target)
}

// 3. Mesin Dinamis: Filter Variabel (Field Selection)
func applyDynamicFields(data map[string]interface{}, fields string) map[string]interface{} {
	if fields == "" { return data }
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
		AppName: "Domain Core - Data Sosial Ekonomi v1.0",
	})
	app.Use(logger.New())

	api := app.Group("/api/v1/social-economy")

	// ============================================================
	// 1. GET ALL RECORD (Stitched & Dynamic Variables)
	// ============================================================
	api.Get("/datasets", func(c *fiber.Ctx) error {
		fields := c.Query("fields")
		var rawList []map[string]interface{}

		// Ambil list dasar dari Kependudukan
		urlPdk := fmt.Sprintf("%s/api/v1/domains/dukcapil/datasets", meshRegistry[0].BaseURL)
		if err := fetchFromDomain(urlPdk, &rawList); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "Gagal mengambil basis data kependudukan"})
		}

		var stitchedList []map[string]interface{}
		for _, person := range rawList {
			nik, _ := person["nomor_induk_kependudukan"].(string)
			
			// Jahit dengan Pendidikan (Opsional)
			var edu map[string]interface{}
			urlEdu := fmt.Sprintf("%s/api/v1/domains/pendidikan/datasets/%s", meshRegistry[1].BaseURL, nik)
			_ = fetchFromDomain(urlEdu, &edu)
			for k, v := range edu { person[k] = v } // Merge fields

			// Filter variabel sesuai request user
			stitchedList = append(stitchedList, applyDynamicFields(person, fields))
		}

		return c.JSON(stitchedList)
	})

	// ============================================================
	// 2. GET SPESIFIK NIK (Stitched & Dynamic Variables)
	// ============================================================
	api.Get("/datasets/:nik", func(c *fiber.Ctx) error {
		nik := c.Params("nik")
		fields := c.Query("fields")

		var profile = make(map[string]interface{})
		var edu = make(map[string]interface{})
		var loc = make(map[string]interface{})

		// Stitching Step 1: Kependudukan
		urlPdk := fmt.Sprintf("%s/api/v1/domains/dukcapil/datasets/%s", meshRegistry[0].BaseURL, nik)
		if err := fetchFromDomain(urlPdk, &profile); err != nil {
			return c.Status(404).JSON(fiber.Map{"error": "NIK tidak ditemukan"})
		}

		// Stitching Step 2: Pendidikan
		urlEdu := fmt.Sprintf("%s/api/v1/domains/pendidikan/datasets/%s", meshRegistry[1].BaseURL, nik)
		_ = fetchFromDomain(urlEdu, &edu)

		// Stitching Step 3: Wilayah (Berdasarkan Kode Desa di profil penduduk)
		if kode, ok := profile["kode_kelurahan_desa"].(string); ok {
			urlWil := fmt.Sprintf("%s/api/v1/domains/wilayah/datasets/%s", meshRegistry[2].BaseURL, kode)
			_ = fetchFromDomain(urlWil, &loc)
		}

		// Merge semua ke satu map besar
		for k, v := range edu { profile[k] = v }
		for k, v := range loc { profile[k] = v }

		return c.JSON(applyDynamicFields(profile, fields))
	})

	// ============================================================
	// 3. ENDPOINT DAFTAR DOMAIN (Mesh Registry Management)
	// ============================================================
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
	fmt.Println("🚀 Core Domain: DATA SOSIAL EKONOMI is running")
	fmt.Println("🌐 Stitcher & Registry | Port: 8000")
	fmt.Println("---------------------------------------------------------")

	app.Listen(":8000")
}