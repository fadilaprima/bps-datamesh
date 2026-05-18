package database

import (
	"hunian/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// InitDB menginisialisasi koneksi ke database PostgreSQL untuk Domain Hunian
func InitDB() *gorm.DB {
	// DSN menggunakan Port 5434 sesuai spesifikasi Domain Hunian
	dsn := "host=127.0.0.1 user=postgres password=admin dbname=postgres port=5434 sslmode=disable"
	
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		panic("Gagal koneksi database Hunian")
	}
	
	db.AutoMigrate(&models.RekamHunian{})
	
	return db
}