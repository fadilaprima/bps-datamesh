package database

import (
	"fmt"
	"os"

	"wilayah/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func InitDB() *gorm.DB {
	dsn := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=disable",
		os.Getenv("DB_HOST_WILAYAH"),
		os.Getenv("DB_USER_WILAYAH"),
		os.Getenv("DB_PASS_WILAYAH"),
		os.Getenv("DB_NAME_WILAYAH"),
		os.Getenv("DB_PORT_WILAYAH"),
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		panic("Gagal koneksi database Wilayah: " + err.Error())
	}

	db.AutoMigrate(&models.MasterWilayah{})
	fmt.Println("Database Wilayah Sinkron & Terkoneksi")
	return db
}