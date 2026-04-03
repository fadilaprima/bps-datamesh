package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type Schema struct {
	ID         uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	Domain     string         `gorm:"index" json:"domain"`      // Isinya: "wilayah"
	Name       string         `json:"name"`                // Isinya: "master_wilayah"
	Version    int            `gorm:"default:1" json:"version"`
	Definition datatypes.JSON `json:"definition"`           // Aturan dinamis (length kode wilayah, dll)
	Status     string         `gorm:"default:'ACTIVE'" json:"status"` 
	CreatedAt  time.Time      `json:"created_at"`
}