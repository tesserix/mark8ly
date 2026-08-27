package inbox

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// SEAReviewProvider surfaces sea_manual_review_queue.
//
// This queue matters more than its size suggests: migration
// 000065_sea_manual_review_queue states that any ID entering it immediately
// pauses the 14-day validation clock on the associated subscription, under a
// 5-business-day SLA. Until this endpoint, nothing read the table.
type SEAReviewProvider struct {
	db  *gorm.DB
	now func() time.Time
}

func NewSEAReviewProvider(db *gorm.DB, now func() time.Time) *SEAReviewProvider {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &SEAReviewProvider{db: db, now: now}
}

func (p *SEAReviewProvider) Kind() string { return KindSEAManualReview }

type seaRow struct {
	ID           string
	BusinessName string
	Country      string
	QueueReason  string
	Status       string
	SLADueAt     time.Time
	QueuedAt     time.Time
}

// seaWaitingStatuses is the status set that means a human still owes this row
// a decision. approved and rejected are resolved and must be absent
// entirely, not returned carrying a resolved status.
const seaWaitingStatuses = "('pending','in_review')"

func (p *SEAReviewProvider) List(ctx context.Context, f Filter) ([]Item, error) {
	q := p.db.WithContext(ctx).
		Table("sea_manual_review_queue").
		Select("id::text AS id, business_name, country, queue_reason, status, sla_due_at, queued_at").
		Where("status IN " + seaWaitingStatuses)
	if f.TenantID != "" {
		q = q.Where("tenant_id = ?", f.TenantID)
	}
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	q = q.Order("sla_due_at ASC")
	if f.Limit > 0 {
		q = q.Limit(f.Limit)
	}
	if f.Page > 1 && f.Limit > 0 {
		q = q.Offset((f.Page - 1) * f.Limit)
	}

	var rows []seaRow
	if err := q.Scan(&rows).Error; err != nil {
		return nil, err
	}

	now := p.now()
	items := make([]Item, 0, len(rows))
	for _, r := range rows {
		due := r.SLADueAt
		items = append(items, Item{
			ID:           r.ID,
			Kind:         KindSEAManualReview,
			Title:        r.BusinessName,
			Subtitle:     r.Country + " tax ID " + r.Status,
			WaitingSince: r.QueuedAt,
			DueAt:        &due,
			Severity:     DeriveSeverity(&due, now),
			Href:         "/admin/tax/manual-review/" + r.ID,
			Actions: []Action{
				{ID: "approve", Label: "Approve", Destructive: false},
				{ID: "reject", Label: "Reject", Destructive: true},
			},
		})
	}
	return items, nil
}

func (p *SEAReviewProvider) Count(ctx context.Context, f Filter) (int64, error) {
	q := p.db.WithContext(ctx).
		Table("sea_manual_review_queue").
		Where("status IN " + seaWaitingStatuses)
	if f.TenantID != "" {
		q = q.Where("tenant_id = ?", f.TenantID)
	}
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	var n int64
	return n, q.Count(&n).Error
}
