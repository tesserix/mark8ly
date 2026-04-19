package seaqueue

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Repository is the persistence boundary for sea_manual_review_queue.
type Repository struct {
	db *gorm.DB
}

func New(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Enqueue inserts a pending entry for the (tenant, store, country) tuple, or
// returns the existing entry if one already exists. Caller is the tax service
// orchestrator on a manual-review verdict.
func (r *Repository) Enqueue(ctx context.Context, e Entry) (Entry, error) {
	now := time.Now().UTC()
	if e.QueuedAt.IsZero() {
		e.QueuedAt = now
	}
	if e.SLADueAt.IsZero() {
		e.SLADueAt = AddBusinessDays(e.QueuedAt, 5)
	}
	if e.Status == "" {
		e.Status = StatusPending
	}

	err := r.db.WithContext(ctx).Exec(`
		INSERT INTO sea_manual_review_queue
			(tenant_id, store_id, country, tax_id, business_name, queue_reason, status, sla_due_at, queued_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (tenant_id, store_id, country) DO NOTHING
	`, e.TenantID, e.StoreID, e.Country, e.TaxID, e.BusinessName, e.QueueReason, e.Status, e.SLADueAt, e.QueuedAt).Error
	if err != nil {
		return Entry{}, fmt.Errorf("seaqueue: insert: %w", err)
	}

	var out Entry
	if err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND store_id = ? AND country = ?", e.TenantID, e.StoreID, e.Country).
		First(&out).Error; err != nil {
		return Entry{}, fmt.Errorf("seaqueue: read-back: %w", err)
	}
	return out, nil
}

// Resolve marks an entry approved or rejected and stamps resolved_at. Only
// pending/in_review rows can be resolved; resolving an already-final row is
// a no-op (no error).
func (r *Repository) Resolve(ctx context.Context, id, reviewerID uuid.UUID, approved bool, notes string) error {
	status := StatusRejected
	if approved {
		status = StatusApproved
	}
	return r.db.WithContext(ctx).Exec(`
		UPDATE sea_manual_review_queue
		   SET status         = ?,
		       reviewer_id    = ?,
		       reviewer_notes = ?,
		       resolved_at    = now()
		 WHERE id = ?
		   AND status IN ('pending', 'in_review')
	`, status, reviewerID, notes, id).Error
}

// FindByStore returns the active (pending or in_review) entry for a store, or
// nil if none exists.
func (r *Repository) FindByStore(ctx context.Context, storeID uuid.UUID) (*Entry, error) {
	var out Entry
	err := r.db.WithContext(ctx).
		Where("store_id = ? AND status IN ('pending', 'in_review')", storeID).
		Order("queued_at DESC").
		First(&out).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// CountThisWeek returns the number of entries queued in the last 7 days. The
// 30/week capacity alert (§19.3) compares this against a threshold.
func (r *Repository) CountThisWeek(ctx context.Context) (int, error) {
	var n int64
	err := r.db.WithContext(ctx).Raw(`
		SELECT COUNT(*) FROM sea_manual_review_queue
		 WHERE queued_at > now() - INTERVAL '7 days'
	`).Row().Scan(&n)
	return int(n), err
}

// AddBusinessDays advances a wall clock by n weekdays (Mon-Fri), preserving
// the time-of-day component. Used to compute the 5-business-day SLA deadline.
func AddBusinessDays(start time.Time, n int) time.Time {
	t := start
	added := 0
	for added < n {
		t = t.Add(24 * time.Hour)
		switch t.Weekday() {
		case time.Saturday, time.Sunday:
			continue
		default:
			added++
		}
	}
	return t
}
