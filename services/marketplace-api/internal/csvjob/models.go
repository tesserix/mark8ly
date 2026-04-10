// Package csvjob manages CSV import job lifecycle: submission, tracking,
// parsing, worker execution, and crash recovery. Each job represents a
// single CSV file uploaded by a merchant for bulk product import.
package csvjob

import "time"

// Status constants match the CHECK constraint in migration 000007.
const (
	StatusQueued    = "queued"
	StatusRunning   = "running"
	StatusPaused    = "paused"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
	StatusCancelled = "cancelled"
)

// CsvImportJob is the GORM model for the csv_import_jobs table.
type CsvImportJob struct {
	ID               string     `gorm:"primaryKey;column:id;type:uuid;default:gen_random_uuid()" json:"id"`
	StoreID          string     `gorm:"column:store_id;type:uuid;not null"                       json:"store_id"`
	UserID           string     `gorm:"column:user_id;type:text;not null"                        json:"user_id"`
	GCSPath          string     `gorm:"column:gcs_path;type:text;not null"                       json:"gcs_path"`
	ContentHash      string     `gorm:"column:content_hash;type:text;not null"                   json:"content_hash"`
	ErrorCSVGCSPath  *string    `gorm:"column:error_csv_gcs_path;type:text"                      json:"error_csv_gcs_path,omitempty"`
	Status           string     `gorm:"column:status;type:text;not null;default:queued"          json:"status"`
	TotalRows        *int       `gorm:"column:total_rows"                                        json:"total_rows,omitempty"`
	LastProcessedRow int        `gorm:"column:last_processed_row;not null;default:0"             json:"last_processed_row"`
	SuccessCount     int        `gorm:"column:success_count;not null;default:0"                  json:"success_count"`
	ErrorCount       int        `gorm:"column:error_count;not null;default:0"                    json:"error_count"`
	HeartbeatAt      *time.Time `gorm:"column:heartbeat_at"                                      json:"heartbeat_at,omitempty"`
	CreatedAt        time.Time  `gorm:"column:created_at;not null;default:now()"                 json:"created_at"`
	UpdatedAt        time.Time  `gorm:"column:updated_at;not null;default:now()"                 json:"updated_at"`
}

func (CsvImportJob) TableName() string { return "csv_import_jobs" }

// IsTerminal reports whether the job is in a final state that cannot transition.
func (j CsvImportJob) IsTerminal() bool {
	return j.Status == StatusCompleted || j.Status == StatusFailed || j.Status == StatusCancelled
}
