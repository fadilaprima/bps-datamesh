package database

import (
	"fmt"
	"os"

	"kesejahteraan/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func InitDB() *gorm.DB {
	dsn := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=disable",
		os.Getenv("DB_HOST_KESEJAHTERAAN"),
		os.Getenv("DB_USER_KESEJAHTERAAN"),
		os.Getenv("DB_PASS_KESEJAHTERAAN"),
		os.Getenv("DB_NAME_KESEJAHTERAAN"),
		os.Getenv("DB_PORT_KESEJAHTERAAN"),
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		panic("Gagal koneksi database Kesejahteraan: " + err.Error())
	}

	db.AutoMigrate(&models.RekamKesejahteraan{}, &models.Schema{})
	fmt.Println("Database Kesejahteraan Sinkron & Terkoneksi")
	return db
}