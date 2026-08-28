package emaillog

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Page sizing, mirroring the outbox platform read (#331).
const (
	DefaultPlatformPageSize = 50
	MaxPlatformPageSize     = 200
)

// Status values a row can hold. Kept in step with the CHECK constraint in
// migration 000109.
const (
	StatusSending    = "sending"
	StatusSent       = "sent"
	StatusFailed     = "failed"
	StatusDelivered  = "delivered"
	StatusBounced    = "bounced"
	StatusComplained = "complained"
)

// PlatformListFilter narrows the cross-tenant send log (#348D).
type PlatformListFilter struct {
	TenantID *uuid.UUID
	// Status matches exactly. An unrecognised value narrows nothing,
	// matching how every other unknown parameter on this surface behaves.
	Status string
	Kind   string
	// StuckMinutes, when > 0, narrows to rows still at `sending` for at
	// least that long.
	//
	// Deliberately restricted to `sending`: this filter answers "what never
	// completed", and a row that reached any other status is settled however
	// old it is. Pairing it with an explicit Status of anything else is
	// therefore a contradiction that returns zero rows — intended, not a bug
	// to investigate. Same reasoning as OlderThanMinutes in the outbox read.
	StuckMinutes int
	From         *time.Time
	To           *time.Time
	Page         int
	Limit        int
}

// PlatformRow is one send in the cross-tenant view.
//
// No subject and no body, by construction — migration 000108 does not store
// them. Three prior endpoints on this surface excluded the same class of
// interpolated customer content: message (#332), description (#329), payload
// (#331).
type PlatformRow struct {
	ID        uuid.UUID  `json:"id"`
	TenantID  *uuid.UUID `json:"tenant_id"`
	StoreID   *uuid.UUID `json:"store_id"`
	Recipient string     `json:"recipient"`
	Kind      string     `json:"kind"`
	Status    string     `json:"status"`
	Error     *string    `json:"error"`
	CreatedAt time.Time  `json:"created_at"`
	SentAt    *time.Time `json:"sent_at"`
	EventAt   *time.Time `json:"event_at"`
	// AgeSeconds is how long a row has been stuck at `sending`, and is NULL
	// for anything that reached a further state — by the same CASE that
	// makes it settled. Derived in SQL so there is one definition of "stuck"
	// in the estate.
	AgeSeconds *int64 `json:"age_seconds"`
}

// PlatformListResult is a page plus the unpaginated total.
type PlatformListResult struct {
	Sends []PlatformRow
	Total int64
}

// ListPlatform serves the platform console's cross-tenant send log (#348D).
//
// A package function rather than a method, so it can be wired through a
// ...Func adapter in main.go exactly as outbox.ListPlatform is.
func ListPlatform(ctx context.Context, db *gorm.DB, f PlatformListFilter,
	asOf time.Time) (PlatformListResult, error) {
	result := PlatformListResult{Sends: make([]PlatformRow, 0)}

	q := db.WithContext(ctx).Table("email_sends")

	if f.TenantID != nil {
		q = q.Where("tenant_id = ?", *f.TenantID)
	}
	if f.Kind != "" {
		q = q.Where("kind = ?", f.Kind)
	}
	switch f.Status {
	case StatusSending, StatusSent, StatusFailed,
		StatusDelivered, StatusBounced, StatusComplained:
		q = q.Where("status = ?", f.Status)
	}
	if f.StuckMinutes > 0 {
		cutoff := asOf.Add(-time.Duration(f.StuckMinutes) * time.Minute)
		q = q.Where("status = ? AND created_at <= ?", StatusSending, cutoff)
	}
	if f.From != nil {
		q = q.Where("created_at >= ?", *f.From)
	}
	if f.To != nil {
		q = q.Where("created_at <= ?", *f.To)
	}

	// Count BEFORE Select: the page below adds a computed column, and Total
	// must be the full match count, not the page size.
	if err := q.Count(&result.Total).Error; err != nil {
		return result, fmt.Errorf("emaillog platform list count: %w", err)
	}

	limit := f.Limit
	if limit <= 0 {
		limit = DefaultPlatformPageSize
	}
	if limit > MaxPlatformPageSize {
		limit = MaxPlatformPageSize
	}
	page := max(f.Page, 1)

	// id breaks the tie so paging is stable: two sends sharing a timestamp
	// could otherwise swap between pages, showing one twice and another never.
	if err := q.
		Select(`id, tenant_id, store_id, recipient, kind, status, error,
			created_at, sent_at, event_at,
			CASE
				WHEN status = ?
				THEN EXTRACT(EPOCH FROM (? - created_at))::bigint
				ELSE NULL
			END AS age_seconds`, StatusSending, asOf).
		Order("created_at DESC, id DESC").
		Offset((page - 1) * limit).
		Limit(limit).
		Scan(&result.Sends).Error; err != nil {
		return result, fmt.Errorf("emaillog platform list: %w", err)
	}
	return result, nil
}
