package database

import (
	"fmt"
	"os"

	"hunian/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func InitDB() *gorm.DB {
	dsn := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=disable",
		os.Getenv("DB_HOST_HUNIAN"),
		os.Getenv("DB_USER_HUNIAN"),
		os.Getenv("DB_PASS_HUNIAN"),
		os.Getenv("DB_NAME_HUNIAN"),
		os.Getenv("DB_PORT_HUNIAN"),
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		panic("Gagal koneksi database Hunian: " + err.Error())
	}

	db.AutoMigrate(&models.RekamHunian{})
	fmt.Println("Database Hunian Sinkron & Terkoneksi")
	return db
}