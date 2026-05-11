package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type Schema struct {
	ID         uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	Domain     string         `gorm:"index" json:"domain"`      
	Name       string         `json:"name"`                
	Version    int            `gorm:"default:1" json:"version"`
	Definition datatypes.JSON `json:"definition"`           
	Status     string         `gorm:"default:'ACTIVE'" json:"status"` 
	CreatedAt  time.Time      `json:"created_at"`
}