package database

import (
	"fmt"
	"kependudukan/models"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func InitDB() *gorm.DB {
	// Port 5431 sesuai kode lama kamu
	dsn := "host=127.0.0.1 user=postgres password=admin dbname=postgres port=5431 sslmode=disable"
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		panic("Gagal koneksi database kependudukan")
	}

	// Auto Migration: Menambah field lifecycle secara otomatis
	db.AutoMigrate(&models.Penduduk{})
	fmt.Println("Database Kependudukan Sinkron & Terkoneksi")
	return db
}
