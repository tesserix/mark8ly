package billingarchive

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	// sweepBatchSize is the number of expired rows to delete per RunOnce call.
	// SKIP LOCKED ensures concurrent sweeper instances never contend.
	sweepBatchSize = 100
)

// Sweeper deletes billing_archive rows whose archive_expires_at has passed.
// It is designed to be called from a daily cron (§23.2 retention expiry).
type Sweeper struct {
	db     *gorm.DB
	logger *slog.Logger
}

// NewSweeper constructs a Sweeper.
func NewSweeper(db *gorm.DB, logger *slog.Logger) *Sweeper {
	if logger == nil {
		logger = slog.Default()
	}
	return &Sweeper{db: db, logger: logger}
}

// RunOnce deletes up to sweepBatchSize expired billing_archive rows.
// It uses SELECT … FOR UPDATE SKIP LOCKED so multiple pod instances can
// safely call RunOnce concurrently without double-deleting.
//
// Returns the number of rows deleted and any database error.
func (s *Sweeper) RunOnce(ctx context.Context) (int64, error) {
	now := time.Now().UTC()

	// Collect IDs of expired rows using SKIP LOCKED so concurrent sweepers
	// each claim a non-overlapping batch.
	var ids []string
	if err := s.db.WithContext(ctx).
		Model(&BillingArchive{}).
		Select("id").
		Where("archive_expires_at <= ?", now).
		Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
		Limit(sweepBatchSize).
		Pluck("id", &ids).Error; err != nil {
		return 0, fmt.Errorf("billingarchive sweeper: select expired ids: %w", err)
	}

	if len(ids) == 0 {
		s.logger.Debug("billingarchive sweeper: no expired rows", "checked_at", now)
		return 0, nil
	}

	result := s.db.WithContext(ctx).
		Where("id IN ?", ids).
		Delete(&BillingArchive{})
	if result.Error != nil {
		return 0, fmt.Errorf("billingarchive sweeper: delete expired rows: %w", result.Error)
	}

	deleted := result.RowsAffected
	s.logger.Info("billingarchive sweeper: deleted expired rows",
		"count", deleted,
		"checked_at", now)

	return deleted, nil
}
