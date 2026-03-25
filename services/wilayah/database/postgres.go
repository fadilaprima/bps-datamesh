package database

import (
   "wilayah/models"
    "gorm.io/driver/postgres"
    "gorm.io/gorm"
)

// Menginisialisasi koneksi ke database PostgreSQL
func InitDB() *gorm.DB {
	dsn := "host=127.0.0.1 user=postgres password=admin dbname=postgres port=5433 sslmode=disable"
	
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		panic("Gagal koneksi database Wilayah")
	}
	
	// AutoMigrate secara otomatis menyesuaikan struktur tabel di DB dengan struct MasterWilayah
	db.AutoMigrate(&models.MasterWilayah{})
	
	return db
}