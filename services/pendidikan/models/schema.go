package models

import (
    "time"
    "github.com/google/uuid"
    "gorm.io/datatypes"
)

type Schema struct {
    ID         uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
    Domain     string         `gorm:"index" json:"domain"`      // Isi: "pendidikan"
    Name       string         `json:"name"`                     // Isi: "riwayat_pendidikan"
    Version    int            `gorm:"default:1" json:"version"`
    Definition datatypes.JSON `json:"definition"`               // Isi: aturan kolom dinamis
    Status     string         `gorm:"default:'ACTIVE'" json:"status"` 
    CreatedAt  time.Time      `json:"created_at"`
}