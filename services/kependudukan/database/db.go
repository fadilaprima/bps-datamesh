package database

import (
	"fmt"
	"log"
	"os"

	"kependudukan/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func InitDB() *gorm.DB {
	dsn := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=disable",
		os.Getenv("DB_HOST_KEPENDUDUKAN"),
		os.Getenv("DB_USER_KEPENDUDUKAN"),
		os.Getenv("DB_PASS_KEPENDUDUKAN"),
		os.Getenv("DB_NAME_KEPENDUDUKAN"),
		os.Getenv("DB_PORT_KEPENDUDUKAN"),
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("Gagal koneksi database Kependudukan: %v", err)
	}

	err = db.AutoMigrate(&models.Penduduk{})
	if err != nil {
		log.Fatalf("Gagal migrasi database Kependudukan: %v", err)
	}

	fmt.Println("Database Kependudukan Sinkron & Terkoneksi")
	return db
}