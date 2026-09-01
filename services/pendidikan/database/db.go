package database

import (
	"fmt"
	"os"

	"pendidikan/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func InitDB() *gorm.DB {
	dsn := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=disable",
		os.Getenv("DB_HOST_PENDIDIKAN"),
		os.Getenv("DB_USER_PENDIDIKAN"),
		os.Getenv("DB_PASS_PENDIDIKAN"),
		os.Getenv("DB_NAME_PENDIDIKAN"),
		os.Getenv("DB_PORT_PENDIDIKAN"),
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		panic("Gagal koneksi database Pendidikan: " + err.Error())
	}

	db.AutoMigrate(&models.RiwayatPendidikan{})
	fmt.Println("Database Pendidikan Sinkron & Terkoneksi")
	return db
}