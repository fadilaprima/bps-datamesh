package database

import (
	"fmt"
	"os"

	"ketenagakerjaan/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func InitDB() *gorm.DB {
	dsn := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=disable",
		os.Getenv("DB_HOST_KETENAGAKERJAAN"),
		os.Getenv("DB_USER_KETENAGAKERJAAN"),
		os.Getenv("DB_PASS_KETENAGAKERJAAN"),
		os.Getenv("DB_NAME_KETENAGAKERJAAN"),
		os.Getenv("DB_PORT_KETENAGAKERJAAN"),
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		panic("Gagal koneksi database Ketenagakerjaan: " + err.Error())
	}

	db.AutoMigrate(&models.RekamKetenagakerjaan{})
	fmt.Println("Database Ketenagakerjaan Sinkron & Terkoneksi")
	return db
}