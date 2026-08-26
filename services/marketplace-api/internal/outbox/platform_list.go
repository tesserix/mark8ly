// Package outbox: the cross-tenant platform read (#331). Deliberately a
// separate file from repository.go, which owns the publisher's write path —
// this is a read for the platform console and shares nothing with it but
// the table.
package outbox

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Derived status values. These are computed in SQL, not stored: the issue
// requires the console not reimplement the null-check, and deriving it
// server-side is what keeps one definition of "pending" in the estate.
const (
	StatusPending   = "pending"
	StatusFailed    = "failed"
	StatusPublished = "published"
)

// Page bounds, matching notification and ticket exactly so the platform
// surface behaves identically across its list endpoints.
const (
	DefaultPlatformPageSize = 50
	MaxPlatformPageSize     = 500
)

// PlatformListFilter narrows the cross-tenant read. Every field is
// optional, and an unrecognised value narrows NOTHING rather than erroring
// — the established contract across this surface.
//
// TenantID NARROWS rather than scopes: this endpoint is cross-tenant by
// design, and the console uses it to answer estate-wide questions.
type PlatformListFilter struct {
	TenantID  *uuid.UUID
	Status    string // StatusPending | StatusFailed | StatusPublished | "" (any)
	EventType string
	// OlderThanMinutes, when > 0, narrows to UNPUBLISHED rows at least that
	// old. It deliberately does NOT match published rows: this filter
	// answers "what is stuck", and a published row is settled however old
	// it is. Same reasoning as AgeSeconds being nil for published rows.
	OlderThanMinutes int
	From             *time.Time
	To               *time.Time
	Page             int
	Limit            int
}

// PlatformRow is one row of the platform read.
//
// There is no Payload field, and that is the point: the projection is
// field-by-field, so a column added to OutboxEvent tomorrow cannot leak
// through this surface. outbox_events.payload is arbitrary JSONB that may
// carry customer data, and a governance surface listing stuck events does
// not need it to be useful.
type PlatformRow struct {
	ID          string
	TenantID    string
	Aggregate   string
	AggregateID string
	EventType   string
	Status      string
	CreatedAt   time.Time
	PublishedAt *time.Time
	Error       *string
	// AgeSeconds is how long an UNPUBLISHED row has been waiting, measured
	// from the caller's asOf. It is nil for a published row: that row is
	// settled, so it has no waiting time, and a number that grew forever
	// there would read as "stuck" beside a genuinely stuck row.
	AgeSeconds *int64
}

// PlatformListResult carries the page plus the FULL match count.
type PlatformListResult struct {
	Rows  []PlatformRow
	Total int64
}

// ListPlatform returns a filtered, paginated, CROSS-TENANT page of outbox
// events for the platform console (#331).
//
// asOf is the instant both AgeSeconds and OlderThanMinutes are measured
// from. It is a parameter rather than time.Now() so the two can never
// disagree — a console that displayed an age computed at a different
// instant from the filter that selected the row would be quietly wrong —
// and so a test can pin an exact age.
func ListPlatform(ctx context.Context, db *gorm.DB, f PlatformListFilter,
	asOf time.Time) (PlatformListResult, error) {
	var result PlatformListResult

	q := db.WithContext(ctx).Model(&OutboxEvent{})

	if f.TenantID != nil {
		q = q.Where("tenant_id = ?", *f.TenantID)
	}
	if f.EventType != "" {
		q = q.Where("event_type = ?", f.EventType)
	}
	// An unrecognised status narrows nothing, matching how every other
	// unknown parameter on this surface behaves.
	switch f.Status {
	case StatusPending:
		q = q.Where("published_at IS NULL AND error IS NULL")
	case StatusFailed:
		q = q.Where("published_at IS NULL AND error IS NOT NULL")
	case StatusPublished:
		q = q.Where("published_at IS NOT NULL")
	}
	if f.OlderThanMinutes > 0 {
		cutoff := asOf.Add(-time.Duration(f.OlderThanMinutes) * time.Minute)
		q = q.Where("published_at IS NULL AND created_at <= ?", cutoff)
	}
	if f.From != nil {
		q = q.Where("created_at >= ?", *f.From)
	}
	if f.To != nil {
		q = q.Where("created_at <= ?", *f.To)
	}

	// Count BEFORE Select: the page below adds computed columns, and Total
	// must be the full match count, not the page size.
	if err := q.Count(&result.Total).Error; err != nil {
		return result, fmt.Errorf("outbox platform list count: %w", err)
	}

	limit := f.Limit
	if limit <= 0 {
		limit = DefaultPlatformPageSize
	}
	if limit > MaxPlatformPageSize {
		limit = MaxPlatformPageSize
	}
	page := f.Page
	if page < 1 {
		page = 1
	}

	// status and age_seconds are derived HERE, in SQL, so there is exactly
	// one definition of each in the estate. age_seconds is NULL for a
	// published row by the same CASE that makes its status 'published'.
	if err := q.
		Select(`id, tenant_id, aggregate, aggregate_id, event_type, created_at, published_at, error,
			CASE
				WHEN published_at IS NOT NULL THEN ?
				WHEN error IS NOT NULL        THEN ?
				ELSE                               ?
			END AS status,
			CASE
				WHEN published_at IS NULL
				THEN EXTRACT(EPOCH FROM (? - created_at))::bigint
				ELSE NULL
			END AS age_seconds`,
			StatusPublished, StatusFailed, StatusPending, asOf).
		Order("created_at DESC").
		Limit(limit).Offset((page - 1) * limit).
		Scan(&result.Rows).Error; err != nil {
		return result, fmt.Errorf("outbox platform list: %w", err)
	}
	return result, nil
}
