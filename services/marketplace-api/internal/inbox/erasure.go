package inbox

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// ErasureProvider surfaces customer_erasure_requests that no one has acted on.
//
// The table is append-only and, before this endpoint, had no reader at all —
// see #259. Items carry no DueAt: GDPR's 30-day window is real, but the table
// has no due column and deriving a statutory deadline in a read endpoint would
// be inventing policy in the wrong place.
type ErasureProvider struct{ db *gorm.DB }

func NewErasureProvider(db *gorm.DB) *ErasureProvider { return &ErasureProvider{db: db} }

func (p *ErasureProvider) Kind() string { return KindErasureRequest }

type erasureRow struct {
	ID            string
	CustomerEmail string
	RequestedAt   time.Time
}

func (p *ErasureProvider) List(ctx context.Context, f Filter) ([]Item, error) {
	if f.Status != "" && f.Status != "pending" {
		return []Item{}, nil
	}
	q := p.db.WithContext(ctx).
		Table("customer_erasure_requests").
		Select("id::text AS id, customer_email, requested_at").
		Where("status = ?", "pending")
	if f.TenantID != "" {
		q = q.Where("tenant_id = ?", f.TenantID)
	}
	q = q.Order("requested_at ASC")
	if f.Limit > 0 {
		q = q.Limit(f.Limit)
	}
	if f.Page > 1 && f.Limit > 0 {
		q = q.Offset((f.Page - 1) * f.Limit)
	}

	var rows []erasureRow
	if err := q.Scan(&rows).Error; err != nil {
		return nil, err
	}
	items := make([]Item, 0, len(rows))
	for _, r := range rows {
		items = append(items, Item{
			ID:           r.ID,
			Kind:         KindErasureRequest,
			Title:        r.CustomerEmail,
			Subtitle:     "Erasure requested",
			WaitingSince: r.RequestedAt,
			Severity:     SeverityNormal,
			Href:         "/admin/erasure/" + r.ID,
			Actions: []Action{
				{ID: "process", Label: "Process erasure", Destructive: true},
				{ID: "reject", Label: "Reject", Destructive: false},
			},
		})
	}
	return items, nil
}

func (p *ErasureProvider) Count(ctx context.Context, f Filter) (int64, error) {
	if f.Status != "" && f.Status != "pending" {
		return 0, nil
	}
	q := p.db.WithContext(ctx).Table("customer_erasure_requests").Where("status = ?", "pending")
	if f.TenantID != "" {
		q = q.Where("tenant_id = ?", f.TenantID)
	}
	var n int64
	return n, q.Count(&n).Error
}
