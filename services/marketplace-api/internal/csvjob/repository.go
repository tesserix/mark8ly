package csvjob

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// Repository is the persistence contract for CSV import jobs.
type Repository interface {
	Create(ctx context.Context, job *CsvImportJob) error
	GetByID(ctx context.Context, id string) (*CsvImportJob, error)
	ListByStore(ctx context.Context, storeID string, page, pageSize int) ([]CsvImportJob, int64, error)
	UpdateStatus(ctx context.Context, id, status string) error
	UpdateProgress(ctx context.Context, id string, lastRow, successCount, errorCount int) error
	UpdateHeartbeat(ctx context.Context, id string) error
	FindOrphanedJobs(ctx context.Context, staleDuration time.Duration) ([]CsvImportJob, error)
	FindQueuedJobs(ctx context.Context, limit int) ([]CsvImportJob, error)
	FindByContentHash(ctx context.Context, storeID, contentHash string) (*CsvImportJob, error)
	SetStatusFields(ctx context.Context, id string, fields map[string]any) error
}

type gormRepository struct {
	db *gorm.DB
}

// NewRepository constructs a Repository backed by the given *gorm.DB.
func NewRepository(db *gorm.DB) Repository { return &gormRepository{db: db} }

func (r *gormRepository) Create(ctx context.Context, job *CsvImportJob) error {
	if err := r.db.WithContext(ctx).Create(job).Error; err != nil {
		return fmt.Errorf("csvjob: create: %w", err)
	}
	return nil
}

func (r *gormRepository) GetByID(ctx context.Context, id string) (*CsvImportJob, error) {
	var job CsvImportJob
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&job).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperrors.NotFound("csv_import_job")
	}
	if err != nil {
		return nil, fmt.Errorf("csvjob: get by id: %w", err)
	}
	return &job, nil
}

func (r *gormRepository) ListByStore(ctx context.Context, storeID string, page, pageSize int) ([]CsvImportJob, int64, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}

	// store_id is a uuid column — an empty string makes Postgres reject the
	// whole statement with SQLSTATE 22P02, so reject it before it hits SQL.
	if storeID == "" {
		return nil, 0, apperrors.ValidationFailed("store_id", "store id is required")
	}

	base := r.db.WithContext(ctx).Model(&CsvImportJob{}).Where("store_id = ?", storeID)

	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("csvjob: list count: %w", err)
	}

	var jobs []CsvImportJob
	err := base.
		Order("created_at DESC").
		Limit(pageSize).
		Offset((page - 1) * pageSize).
		Find(&jobs).Error
	if err != nil {
		return nil, 0, fmt.Errorf("csvjob: list: %w", err)
	}
	return jobs, total, nil
}

func (r *gormRepository) UpdateStatus(ctx context.Context, id, status string) error {
	res := r.db.WithContext(ctx).Model(&CsvImportJob{}).
		Where("id = ?", id).
		Updates(map[string]any{"status": status, "updated_at": gorm.Expr("now()")})
	if res.Error != nil {
		return fmt.Errorf("csvjob: update status: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return apperrors.NotFound("csv_import_job")
	}
	return nil
}

func (r *gormRepository) UpdateProgress(ctx context.Context, id string, lastRow, successCount, errorCount int) error {
	res := r.db.WithContext(ctx).Model(&CsvImportJob{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"last_processed_row": lastRow,
			"success_count":      successCount,
			"error_count":        errorCount,
			"updated_at":         gorm.Expr("now()"),
		})
	if res.Error != nil {
		return fmt.Errorf("csvjob: update progress: %w", res.Error)
	}
	return nil
}

func (r *gormRepository) UpdateHeartbeat(ctx context.Context, id string) error {
	res := r.db.WithContext(ctx).Model(&CsvImportJob{}).
		Where("id = ?", id).
		Update("heartbeat_at", gorm.Expr("now()"))
	if res.Error != nil {
		return fmt.Errorf("csvjob: update heartbeat: %w", res.Error)
	}
	return nil
}

// FindOrphanedJobs returns jobs with status='running' whose heartbeat is
// older than staleDuration. Uses FOR UPDATE SKIP LOCKED to prevent
// concurrent recovery scans from picking up the same jobs.
func (r *gormRepository) FindOrphanedJobs(ctx context.Context, staleDuration time.Duration) ([]CsvImportJob, error) {
	cutoff := time.Now().Add(-staleDuration)
	var jobs []CsvImportJob
	err := r.db.WithContext(ctx).
		Raw("SELECT * FROM csv_import_jobs WHERE status = ? AND (heartbeat_at IS NULL OR heartbeat_at < ?) FOR UPDATE SKIP LOCKED",
			StatusRunning, cutoff).
		Scan(&jobs).Error
	if err != nil {
		return nil, fmt.Errorf("csvjob: find orphaned: %w", err)
	}
	return jobs, nil
}

// FindQueuedJobs returns up to limit jobs sitting in the queued state,
// across all stores, oldest first. This is the store-agnostic query the
// polling loop needs: jobs left behind by a crash (re-queued by
// RecoverOrphanedJobs or resumed by a merchant) belong to arbitrary stores.
func (r *gormRepository) FindQueuedJobs(ctx context.Context, limit int) ([]CsvImportJob, error) {
	if limit <= 0 {
		limit = 10
	}
	var jobs []CsvImportJob
	err := r.db.WithContext(ctx).
		Where("status = ?", StatusQueued).
		Order("created_at ASC").
		Limit(limit).
		Find(&jobs).Error
	if err != nil {
		return nil, fmt.Errorf("csvjob: find queued: %w", err)
	}
	return jobs, nil
}

// FindByContentHash returns an active (queued/running/paused) job for the
// given store + content hash, or nil if none exists.
func (r *gormRepository) FindByContentHash(ctx context.Context, storeID, contentHash string) (*CsvImportJob, error) {
	var job CsvImportJob
	err := r.db.WithContext(ctx).
		Where("store_id = ? AND content_hash = ? AND status IN (?, ?, ?)",
			storeID, contentHash, StatusQueued, StatusRunning, StatusPaused).
		First(&job).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("csvjob: find by content hash: %w", err)
	}
	return &job, nil
}

func (r *gormRepository) SetStatusFields(ctx context.Context, id string, fields map[string]any) error {
	fields["updated_at"] = gorm.Expr("now()")
	res := r.db.WithContext(ctx).Model(&CsvImportJob{}).
		Where("id = ?", id).
		Updates(fields)
	if res.Error != nil {
		return fmt.Errorf("csvjob: set status fields: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return apperrors.NotFound("csv_import_job")
	}
	return nil
}
