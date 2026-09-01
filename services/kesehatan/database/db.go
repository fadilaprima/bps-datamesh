package database

import (
	"fmt"
	"os"

	"kesehatan/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func InitDB() *gorm.DB {
	dsn := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=disable",
		os.Getenv("DB_HOST_KESEHATAN"),
		os.Getenv("DB_USER_KESEHATAN"),
		os.Getenv("DB_PASS_KESEHATAN"),
		os.Getenv("DB_NAME_KESEHATAN"),
		os.Getenv("DB_PORT_KESEHATAN"),
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		panic("Gagal koneksi database Kesehatan: " + err.Error())
	}

	db.AutoMigrate(&models.RekamKesehatan{})
	fmt.Println("Database Kesehatan Sinkron & Terkoneksi")
	return db
}