<<<<<<< HEAD
package database

import (
	"pendidikan/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// InitDB menginisialisasi koneksi ke database PostgreSQL untuk Domain Pendidikan
func InitDB() *gorm.DB {
	// DSN menggunakan Port 5434 sesuai spesifikasi Domain Pendidikan
	dsn := "host=127.0.0.1 user=postgres password=admin dbname=postgres port=5434 sslmode=disable"
	
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		panic("Gagal koneksi database Pendidikan")
	}
	
	// AutoMigrate secara otomatis menyesuaikan struktur tabel di DB dengan struct RiwayatPendidikan
	// Mencakup field lifecycle: IsDeleted, AuditStatus, Version, dll.
	db.AutoMigrate(&models.RiwayatPendidikan{})
	
	return db
=======
package database

import (
	"pendidikan/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// InitDB menginisialisasi koneksi ke database PostgreSQL untuk Domain Pendidikan
func InitDB() *gorm.DB {
	// DSN menggunakan Port 5434 sesuai spesifikasi Domain Pendidikan
	dsn := "host=127.0.0.1 user=postgres password=admin dbname=postgres port=5434 sslmode=disable"
	
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		panic("Gagal koneksi database Pendidikan")
	}
	
	// AutoMigrate secara otomatis menyesuaikan struktur tabel di DB dengan struct RiwayatPendidikan
	// Mencakup field lifecycle: IsDeleted, AuditStatus, Version, dll.
	db.AutoMigrate(&models.RiwayatPendidikan{})
	
	return db
>>>>>>> a935de74a96c3dd35516385ff2e46b34b9d60ef1
}