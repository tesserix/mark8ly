package audit

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ListFilter narrows an audit log query. Empty fields are ignored.
// TenantID + StoreID are required and enforced by the handler — the
// repository will refuse a query that's missing either.
type ListFilter struct {
	TenantID     uuid.UUID
	StoreID      uuid.UUID
	UserEmail    string    // partial match on actor_email (case-insensitive)
	Action       string    // exact match
	ResourceType string    // exact match
	Severity     string    // exact match
	DateFrom     time.Time // inclusive lower bound on created_at
	DateTo       time.Time // inclusive upper bound on created_at
	Search       string    // free-text across action, resource_type, resource_id, actor_email
	Page         int       // 1-based; defaults to 1
	PageSize     int       // defaults to 25, capped at 100
}

// ListResult is a page of audit entries plus the unpaginated total.
type ListResult struct {
	Entries []Entry
	Total   int64
}

// Repository is the data-access surface for audit_logs.
type Repository interface {
	// List returns a page of entries matching f, plus the total count
	// before pagination. Always tenant+store scoped.
	List(ctx context.Context, db *gorm.DB, f ListFilter) (ListResult, error)

	// Create inserts a single audit row. Used by the Emitter worker.
	Create(ctx context.Context, db *gorm.DB, e *Entry) error

	// Stream calls fn for every entry matching f, in created_at DESC
	// order, ignoring pagination. Used by ExportCSV. Caller's fn must
	// not retain the *Entry across calls.
	Stream(ctx context.Context, db *gorm.DB, f ListFilter, fn func(*Entry) error) error
}

type gormRepository struct{}

// NewRepository constructs a stateless GORM-backed repository.
func NewRepository() Repository { return &gormRepository{} }

// applyScope enforces tenant + store isolation on every query. Returns
// an error if either ID is the zero UUID — defense in depth against a
// caller forgetting to scope.
func applyScope(q *gorm.DB, f ListFilter) (*gorm.DB, error) {
	if f.TenantID == uuid.Nil || f.StoreID == uuid.Nil {
		return nil, fmt.Errorf("audit list: tenant_id and store_id are required")
	}
	q = q.Where("tenant_id = ? AND store_id = ?", f.TenantID, f.StoreID)
	if f.UserEmail != "" {
		q = q.Where("actor_email ILIKE ?", "%"+f.UserEmail+"%")
	}
	if f.Action != "" {
		q = q.Where("action = ?", f.Action)
	}
	if f.ResourceType != "" {
		q = q.Where("resource_type = ?", f.ResourceType)
	}
	if f.Severity != "" {
		q = q.Where("severity = ?", f.Severity)
	}
	if !f.DateFrom.IsZero() {
		q = q.Where("created_at >= ?", f.DateFrom)
	}
	if !f.DateTo.IsZero() {
		q = q.Where("created_at <= ?", f.DateTo)
	}
	if s := strings.TrimSpace(f.Search); s != "" {
		like := "%" + s + "%"
		q = q.Where(
			"action ILIKE ? OR resource_type ILIKE ? OR COALESCE(resource_id,'') ILIKE ? OR COALESCE(actor_email,'') ILIKE ?",
			like, like, like, like,
		)
	}
	return q, nil
}

func (gormRepository) List(ctx context.Context, db *gorm.DB, f ListFilter) (ListResult, error) {
	var result ListResult
	q, err := applyScope(db.WithContext(ctx).Model(&Entry{}), f)
	if err != nil {
		return result, err
	}

	if err := q.Count(&result.Total).Error; err != nil {
		return result, fmt.Errorf("audit list count: %w", err)
	}

	page := max(f.Page, 1)
	pageSize := f.PageSize
	switch {
	case pageSize <= 0:
		pageSize = 25
	case pageSize > 100:
		pageSize = 100
	}
	offset := (page - 1) * pageSize

	if err := q.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&result.Entries).Error; err != nil {
		return result, fmt.Errorf("audit list: %w", err)
	}
	return result, nil
}

func (gormRepository) Create(ctx context.Context, db *gorm.DB, e *Entry) error {
	if e.TenantID == uuid.Nil {
		return fmt.Errorf("audit create: tenant_id is required")
	}
	if e.StoreID != nil && *e.StoreID == uuid.Nil {
		return fmt.Errorf("audit create: store_id must be nil or a real store")
	}
	if err := db.WithContext(ctx).Create(e).Error; err != nil {
		return fmt.Errorf("audit create: %w", err)
	}
	return nil
}

func (gormRepository) Stream(ctx context.Context, db *gorm.DB, f ListFilter, fn func(*Entry) error) error {
	q, err := applyScope(db.WithContext(ctx).Model(&Entry{}), f)
	if err != nil {
		return err
	}
	rows, err := q.Order("created_at DESC").Rows()
	if err != nil {
		return fmt.Errorf("audit stream: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var e Entry
		if err := db.ScanRows(rows, &e); err != nil {
			return fmt.Errorf("audit stream scan: %w", err)
		}
		if err := fn(&e); err != nil {
			return err
		}
	}
	return rows.Err()
}
