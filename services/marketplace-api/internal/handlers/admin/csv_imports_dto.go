package admin

import (
	"time"

	"github.com/mark8ly/marketplace-api/internal/csvjob"
)

// CSVImportJobResponse is the wire DTO for a CSV import job.
type CSVImportJobResponse struct {
	ID               string     `json:"id"`
	StoreID          string     `json:"store_id"`
	UserID           string     `json:"user_id"`
	GCSPath          string     `json:"gcs_path"`
	ContentHash      string     `json:"content_hash"`
	ErrorCSVGCSPath  *string    `json:"error_csv_gcs_path,omitempty"`
	Status           string     `json:"status"`
	TotalRows        *int       `json:"total_rows,omitempty"`
	LastProcessedRow int        `json:"last_processed_row"`
	SuccessCount     int        `json:"success_count"`
	ErrorCount       int        `json:"error_count"`
	HeartbeatAt      *time.Time `json:"heartbeat_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// ToCSVImportJobResponse converts the domain model to its wire DTO.
func ToCSVImportJobResponse(j *csvjob.CsvImportJob) CSVImportJobResponse {
	return CSVImportJobResponse{
		ID:               j.ID,
		StoreID:          j.StoreID,
		UserID:           j.UserID,
		GCSPath:          j.GCSPath,
		ContentHash:      j.ContentHash,
		ErrorCSVGCSPath:  j.ErrorCSVGCSPath,
		Status:           j.Status,
		TotalRows:        j.TotalRows,
		LastProcessedRow: j.LastProcessedRow,
		SuccessCount:     j.SuccessCount,
		ErrorCount:       j.ErrorCount,
		HeartbeatAt:      j.HeartbeatAt,
		CreatedAt:        j.CreatedAt,
		UpdatedAt:        j.UpdatedAt,
	}
}

// ListCSVImportsQuery is the query params for GET /csv-imports.
type ListCSVImportsQuery struct {
	Page     int `form:"page"`
	PageSize int `form:"page_size"`
}

// Defaults fills in zero-value fields with sensible defaults.
func (q *ListCSVImportsQuery) Defaults() {
	if q.Page <= 0 {
		q.Page = 1
	}
	if q.PageSize <= 0 {
		q.PageSize = 20
	}
}
