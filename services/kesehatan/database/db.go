package database

import (
	"kesehatan/models"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// InitDB menginisialisasi koneksi ke database PostgreSQL untuk Domain kesehatan
func InitDB() *gorm.DB {
	dsn := "host=127.0.0.1 user=postgres password=admin dbname=postgres port=5434 sslmode=disable"

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		panic("Gagal koneksi database kesehatan")
	}

	db.AutoMigrate(&models.RekamKesehatan{})

	return db
}
