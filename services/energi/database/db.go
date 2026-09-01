package database

import (
	"fmt"
	"os"

	"energi/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func InitDB() *gorm.DB {
	dsn := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=disable",
		os.Getenv("DB_HOST_ENERGI"),
		os.Getenv("DB_USER_ENERGI"),
		os.Getenv("DB_PASS_ENERGI"),
		os.Getenv("DB_NAME_ENERGI"),
		os.Getenv("DB_PORT_ENERGI"),
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		panic("Gagal koneksi database Energi: " + err.Error())
	}

	db.AutoMigrate(&models.RekamEnergi{})
	fmt.Println("Database Energi Sinkron & Terkoneksi")
	return db
}