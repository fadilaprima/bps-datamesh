package database

import (
	"energi/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// InitDB menginisialisasi koneksi ke database PostgreSQL untuk Domain Energi
func InitDB() *gorm.DB {
	// DSN menggunakan Port 5435 sesuai spesifikasi Domain Energi
	dsn := "host=127.0.0.1 user=postgres password=admin dbname=postgres port=5435 sslmode=disable"
	
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		panic("Gagal koneksi database Energi")
	}
	
	// AutoMigrate secara otomatis menyesuaikan struktur tabel di DB dengan struct RekamEnergi
	db.AutoMigrate(&models.RekamEnergi{})
	
	return db
}