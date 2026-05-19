package database

import (
	"kesejahteraan/models" 
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// InitDB menginisialisasi koneksi ke database PostgreSQL untuk Domain Kesejahteraan
func InitDB() *gorm.DB {
	
	dsn := "host=127.0.0.1 user=postgres password=admin dbname=db_kesejahteraan port=5436 sslmode=disable"
	
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		panic("Gagal koneksi database Kesejahteraan: " + err.Error())
	}
	
	db.AutoMigrate(&models.RekamKesejahteraan{}, &models.Schema{})
	
	return db
}