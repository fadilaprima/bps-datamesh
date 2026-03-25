<<<<<<< HEAD
package dto

import "time"

type PendidikanIngestRequest struct {
	NIK         string    `json:"nomor_induk_kependudukan"`
	Partisipasi string    `json:"partisipasi_sekolah"`
	Jenjang     string    `json:"jenjang_tertinggi_yang_diduduki"`
	Kelas       string    `json:"kelas_tertinggi_yang_diduduki"`
	Ijazah      string    `json:"ijazah_tertinggi_yang_dimiliki"`
	
	// Metadata Data Mesh (Wajib Identik)
	SourceID      string    `json:"source_id"`
	ReferenceDate time.Time `json:"reference_date"`
}

type PendidikanResponse struct {
	Status  string      `json:"status"`
	Message string      `json:"message"`
	Stats   interface{} `json:"stats,omitempty"`
	Errors  []string    `json:"errors,omitempty"`
=======
package dto

import "time"

type PendidikanIngestRequest struct {
	NIK         string    `json:"nomor_induk_kependudukan"`
	Partisipasi string    `json:"partisipasi_sekolah"`
	Jenjang     string    `json:"jenjang_tertinggi_yang_diduduki"`
	Kelas       string    `json:"kelas_tertinggi_yang_diduduki"`
	Ijazah      string    `json:"ijazah_tertinggi_yang_dimiliki"`
	
	// Metadata Data Mesh (Wajib Identik)
	SourceID      string    `json:"source_id"`
	ReferenceDate time.Time `json:"reference_date"`
}

type PendidikanResponse struct {
	Status  string      `json:"status"`
	Message string      `json:"message"`
	Stats   interface{} `json:"stats,omitempty"`
	Errors  []string    `json:"errors,omitempty"`
>>>>>>> a935de74a96c3dd35516385ff2e46b34b9d60ef1
}