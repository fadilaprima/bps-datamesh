package database

import (
	"ketenagakerjaan/models"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func InitDB() *gorm.DB {
	dsn := "host=127.0.0.1 user=postgres password=admin dbname=postgres port=5439 sslmode=disable"
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		panic("Gagal koneksi database Ketenagakerjaan")
	}
	db.AutoMigrate(&models.RekamKetenagakerjaan{})
	return db
}
