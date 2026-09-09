package app

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"wilayah/models"
	"wilayah/storage"

	"gorm.io/datatypes"
)

type WilayahService struct {
	Storage storage.WilayahStorage
}

type SourceConfig struct {
	Name   string
	IsWali bool
}

var SourceMap = map[int]SourceConfig{
	1: {Name: "BPS", IsWali: true},
	2: {Name: "KEMENDAGRI", IsWali: true},
	3: {Name: "LAINNYA", IsWali: false},
}

var AuditMap = map[int]string{
	1: "VALID",
	2: "INVALID",
}

// ==========================================
// 1. METADATA VALIDATOR (DYNAMIC SCHEMA)
// ==========================================
func (s *WilayahService) ValidateWilayahMetadata(w models.MasterWilayah, definition datatypes.JSON) (bool, string) {
	var schemaMap map[string]interface{}
	if err := json.Unmarshal(definition, &schemaMap); err != nil {
		return false, "Gagal membaca aturan metadata wilayah"
	}

	rules, ok := schemaMap["definition"].(map[string]interface{})
	if !ok { return true, "" }

	val := reflect.ValueOf(w)
	typ := reflect.TypeOf(w)
	fixedFields := make(map[string]bool)

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		jsonTag := strings.Split(field.Tag.Get("json"), ",")[0]
		
		if jsonTag == "" || jsonTag == "-" { continue }
		fixedFields[jsonTag] = true
		fieldValue := fmt.Sprintf("%v", val.Field(i).Interface()) 

		if r, ok := rules[jsonTag].(map[string]interface{}); ok {
			if r["required"] == true && strings.TrimSpace(fieldValue) == "" {
				return false, fmt.Sprintf("Atribut '%s' wajib diisi (Mandatory)", jsonTag)
			}
			if lengthVal, ok := r["length"].(float64); ok {
				if fieldValue != "" && len(fieldValue) != int(lengthVal) {
					return false, fmt.Sprintf("Atribut '%s' tidak valid, harus %d digit sesuai standar MFD BPS", jsonTag, int(lengthVal))
				}
			}
		}
	}

	var extra map[string]interface{}
	json.Unmarshal(w.AdditionalInfo, &extra)

	for field, rule := range rules {
		r, ok := rule.(map[string]interface{})
		if !ok { continue }
		
		if r["required"] == true && !fixedFields[field] {
			if valData, exists := extra[field]; !exists || fmt.Sprintf("%v", valData) == "" {
				return false, fmt.Sprintf("Atribut tambahan '%s' wajib diisi", field)
			}
		}
	}

	return true, ""
}

// ==========================================
// 2. INGESTION PIPELINE (SCD TYPE 2)
// ==========================================
func (s *WilayahService) ProcessIngestion(w models.MasterWilayah) (string, error) {

	last, err := s.Storage.GetLatestByKode(w.KodeDesa)

	// Skenario A: Data Baru
	if err != nil {
		w.Version = 1
		w.AuditStatus = "PENDING"
		if errCreate := s.Storage.Create(&w); errCreate != nil { return "Error Database", errCreate }
		return "Sukses v1 (Masuk Data Mesh)", nil
	}

	// Skenario B: Update Data 
	isNewer := w.ReferenceDate.After(last.ReferenceDate)
	isHigherAuthority := w.ReferenceDate.Equal(last.ReferenceDate) && w.IsWaliData && !last.IsWaliData

	if isNewer || isHigherAuthority {
		w.ID = 0 
		w.Version = last.Version + 1
		w.AuditStatus = "PENDING"

		if errCreate := s.Storage.Create(&w); errCreate != nil { return "Error Database", errCreate }
		return fmt.Sprintf("Sukses v%d (Wilayah Updated)", w.Version), nil
	}

	return "Abaikan", fmt.Errorf("data usang atau otoritas lebih rendah")
}

// ==========================================
// 3. UPDATE & AUDIT LOGIC
// ==========================================
func (s *WilayahService) ProcessManualUpdate(kodeDesa string, newData models.MasterWilayah) (int, error) {
	oldData, err := s.Storage.GetLatestByKode(kodeDesa)
	if err != nil || oldData == nil {
		return 0, fmt.Errorf("data asli tidak ditemukan")
	}

	// LOGIKA RESET SCD TYPE 2
	newData.ID = 0
	newData.Version = oldData.Version + 1
	newData.AuditStatus = "PENDING"
	newData.UpdatedAt = time.Now()

	// Skor kembali ke base (60 Sistem + 20 Sumber jika Walidata)
	if newData.IsWaliData {
		newData.TrustScore = 80.0
	} else {
		newData.TrustScore = 60.0
	}

	errCreate := s.Storage.Create(&newData)
	return newData.Version, errCreate
}

func (s *WilayahService) ProcessAuditDecision(kodeDesa []string, verdict int) (string, error) {
	verdictText := "INVALID"
	bonus := 0.0

	if text, exists := AuditMap[verdict]; exists {
		verdictText = text
		if verdict == 1 { bonus = 20.0 }
	} else {
		return "", fmt.Errorf("verdict tidak valid. Gunakan 1 (VALID) atau 2 (INVALID)")
	}

	err := s.Storage.UpdateBulkAuditDecision(kodeDesa, verdictText, bonus)
	return verdictText, err
}