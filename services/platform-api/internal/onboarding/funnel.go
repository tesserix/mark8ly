package onboarding

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// AbandonedAfter is how long a non-completed session may sit idle before the
// funnel counts it abandoned.
//
// Derived, not stored: StatusAbandoned exists as a constant and in migration
// 0004's CHECK constraint, but nothing ever writes it and there is no gc
// (#322). Querying status = 'abandoned' returns zero forever.
//
// 24h because onboarding is normally one sitting; the only legitimate long
// pause is waiting on the verification email, which is minutes to hours.
const AbandonedAfter = 24 * time.Hour

// abandonedExpr is the SQL for "not completed and idle past the cutoff".
// Shared by the funnel aggregate and the sessions list so the two cannot
// disagree about which sessions are abandoned. Exactly AbandonedAfter idle
// counts as abandoned, hence <=.
//
// The interval is built from AbandonedAfter (via make_interval, in hours)
// rather than hardcoded as a second literal — a test changing the constant
// changes the query, since there is only one place the cutoff is written.
//
// asOfExpr is the SQL expression to evaluate "now" against — normally the
// literal "now()", but a test can pass a bound parameter placeholder (see
// FunnelFilter.AsOf) to pin the evaluation instant exactly.
func abandonedExpr(asOfExpr string) string {
	return fmt.Sprintf(
		"(onboarding_sessions.status <> 'completed' AND onboarding_sessions.last_activity_at <= %s - make_interval(hours => %d))",
		asOfExpr,
		int(AbandonedAfter.Hours()),
	)
}

// asOfExpr renders the SQL expression the funnel queries use in place of
// now(). When f.AsOf is the zero value it returns the literal "now()", so
// production behaviour is unchanged. When AsOf is set, it renders a
// timestamptz literal instead, so a test can pin the evaluation instant
// exactly and place a fixture precisely on the abandoned-cutoff boundary.
//
// AsOf is formatted from a Go time.Time via RFC3339Nano, never from
// unsanitized external input, so building the literal by fmt.Sprintf here
// carries no injection risk.
func asOfExpr(f FunnelFilter) string {
	if f.AsOf.IsZero() {
		return "now()"
	}
	return fmt.Sprintf("'%s'::timestamptz", f.AsOf.UTC().Format(time.RFC3339Nano))
}

// DefaultFunnelPageSize applies when the caller sends no limit.
const DefaultFunnelPageSize = 50

// MaxFunnelPageSize caps a page, mirroring the tenant directory's ceiling.
const MaxFunnelPageSize = 500

// FunnelFilter narrows the onboarding funnel to a window and, for
// ListSessions, a page. CreatedFrom/CreatedTo bound onboarding_sessions
// created_at; Status and Abandoned further narrow ListSessions only.
type FunnelFilter struct {
	CreatedFrom time.Time
	CreatedTo   time.Time
	Status      string
	// Abandoned, when non-nil, filters ListSessions rows to only
	// abandoned (true) or only non-abandoned (false) sessions. GetFunnel
	// ignores this field — it always returns all three buckets.
	Abandoned *bool
	Page      int
	Limit     int
	// AsOf pins the instant the abandoned/in-flight predicates and the
	// last_24h window evaluate "now" against. Zero value (the default)
	// means "use the database's now() exactly as before" — production
	// behaviour is unchanged.
	//
	// Internal only. This field exists so tests can freeze time and place
	// a fixture precisely on the AbandonedAfter boundary; it must NEVER be
	// exposed as an HTTP query parameter by Task 2's handlers — a console
	// caller able to set it could time-travel the funnel.
	AsOf time.Time
}

// FunnelCounts is one window's worth of funnel aggregates. Completed +
// InFlight + Abandoned == Started, exactly, for any window — that
// invariant is the whole point of computing all four in one query.
// EmailVerified is a subset counter cutting across the other three, not a
// fourth bucket to add into the sum.
type FunnelCounts struct {
	Started       int64 `json:"started"`
	EmailVerified int64 `json:"email_verified"`
	Completed     int64 `json:"completed"`
	InFlight      int64 `json:"in_flight"`
	Abandoned     int64 `json:"abandoned"`
}

// FunnelStats is the full funnel response: the windowed counts, the
// median completion time within that window, and a Last24h count that
// always covers the trailing 24 hours regardless of the requested window.
type FunnelStats struct {
	FunnelCounts
	// MedianCompletionSeconds is nil when no session in the window
	// completed — NULL must survive the SQL round trip as Go nil, not
	// silently become 0 ("instant completion").
	MedianCompletionSeconds *float64     `json:"median_completion_seconds"`
	Last24h                 FunnelCounts `json:"last_24h"`
}

// SessionRow is one row of the onboarding sessions list, with Abandoned
// computed by the same predicate GetFunnel uses so the two endpoints can
// never disagree about which sessions are abandoned.
type SessionRow struct {
	ID              string     `json:"id"`
	Email           string     `json:"email"`
	Status          string     `json:"status"`
	EmailVerifiedAt *time.Time `json:"email_verified_at,omitempty"`
	TenantID        *string    `json:"tenant_id,omitempty"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
	LastActivityAt  time.Time  `json:"last_activity_at"`
	CreatedAt       time.Time  `json:"created_at"`
	Abandoned       bool       `json:"abandoned"`
}

// applyFunnelWindow scopes a query to onboarding_sessions.created_at within
// [CreatedFrom, CreatedTo]. Shared by GetFunnel's windowed aggregate and
// ListSessions, so a session outside the window can never appear in one
// but not the other.
func applyFunnelWindow(q *gorm.DB, f FunnelFilter) *gorm.DB {
	if !f.CreatedFrom.IsZero() {
		q = q.Where("onboarding_sessions.created_at >= ?", f.CreatedFrom)
	}
	if !f.CreatedTo.IsZero() {
		q = q.Where("onboarding_sessions.created_at <= ?", f.CreatedTo)
	}
	return q
}

// applySessionFilter narrows ListSessions further, on top of the window:
// by status, and by the shared abandoned predicate.
func applySessionFilter(q *gorm.DB, f FunnelFilter) *gorm.DB {
	q = applyFunnelWindow(q, f)
	if f.Status != "" {
		q = q.Where("onboarding_sessions.status = ?", f.Status)
	}
	if f.Abandoned != nil {
		abandoned := abandonedExpr(asOfExpr(f))
		if *f.Abandoned {
			q = q.Where(abandoned)
		} else {
			q = q.Where("NOT " + abandoned)
		}
	}
	return q
}

// funnelAggregateRow is the raw scan target for GetFunnel's single
// aggregate query. MedianCompletionSeconds is a *float64 so SQL NULL
// (no completions in the window) survives as Go nil rather than silently
// becoming 0.
type funnelAggregateRow struct {
	Started                 int64
	EmailVerified           int64
	Completed               int64
	Abandoned               int64
	InFlight                int64
	MedianCompletionSeconds *float64
}

// GetFunnel returns the funnel counters for the given window: started,
// email_verified, completed, in_flight, abandoned (all from ONE query with
// FILTER (WHERE …) aggregates, so every count observes the same database
// state), the median completion time, and last_24h (a second, independent
// query that always covers the trailing 24 hours regardless of the
// requested window).
func (r *gormRepository) GetFunnel(ctx context.Context, f FunnelFilter) (*FunnelStats, error) {
	asOf := asOfExpr(f)
	abandoned := abandonedExpr(asOf)

	var row funnelAggregateRow
	q := applyFunnelWindow(r.db.WithContext(ctx).Table("onboarding_sessions"), f)
	err := q.Select(
		"COUNT(*) AS started",
		"COUNT(*) FILTER (WHERE email_verified_at IS NOT NULL) AS email_verified",
		"COUNT(*) FILTER (WHERE status = 'completed') AS completed",
		fmt.Sprintf("COUNT(*) FILTER (WHERE %s) AS abandoned", abandoned),
		fmt.Sprintf("COUNT(*) FILTER (WHERE status <> 'completed' AND NOT (%s)) AS in_flight", abandoned),
		"percentile_cont(0.5) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM completed_at - created_at)) FILTER (WHERE status = 'completed') AS median_completion_seconds",
	).Scan(&row).Error
	if err != nil {
		return nil, fmt.Errorf("onboarding: get funnel: %w", err)
	}

	var last24h funnelAggregateRow
	err = r.db.WithContext(ctx).Table("onboarding_sessions").
		Where(fmt.Sprintf("created_at > %s - INTERVAL '24 hours'", asOf)).
		Select(
			"COUNT(*) AS started",
			"COUNT(*) FILTER (WHERE email_verified_at IS NOT NULL) AS email_verified",
			"COUNT(*) FILTER (WHERE status = 'completed') AS completed",
			fmt.Sprintf("COUNT(*) FILTER (WHERE %s) AS abandoned", abandoned),
			fmt.Sprintf("COUNT(*) FILTER (WHERE status <> 'completed' AND NOT (%s)) AS in_flight", abandoned),
		).Scan(&last24h).Error
	if err != nil {
		return nil, fmt.Errorf("onboarding: get funnel last_24h: %w", err)
	}

	return &FunnelStats{
		FunnelCounts: FunnelCounts{
			Started:       row.Started,
			EmailVerified: row.EmailVerified,
			Completed:     row.Completed,
			InFlight:      row.InFlight,
			Abandoned:     row.Abandoned,
		},
		MedianCompletionSeconds: row.MedianCompletionSeconds,
		Last24h: FunnelCounts{
			Started:       last24h.Started,
			EmailVerified: last24h.EmailVerified,
			Completed:     last24h.Completed,
			InFlight:      last24h.InFlight,
			Abandoned:     last24h.Abandoned,
		},
	}, nil
}

// sessionRowScan is the raw scan target for one ListSessions row.
type sessionRowScan struct {
	ID              string
	Email           string
	Status          string
	EmailVerifiedAt *time.Time
	TenantID        *string
	CompletedAt     *time.Time
	LastActivityAt  time.Time
	CreatedAt       time.Time
	Abandoned       bool
}

// ListSessions returns a page of onboarding sessions plus the unpaginated
// total, ordered created_at DESC, sharing applyFunnelWindow/applySessionFilter
// with GetFunnel so the two queries can never drift apart on which sessions
// are in scope or which are abandoned.
func (r *gormRepository) ListSessions(ctx context.Context, f FunnelFilter) ([]SessionRow, int64, error) {
	abandoned := abandonedExpr(asOfExpr(f))

	countQ := applySessionFilter(r.db.WithContext(ctx).Table("onboarding_sessions"), f)
	var total int64
	if err := countQ.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("onboarding: list sessions count: %w", err)
	}

	page := max(f.Page, 1)
	limit := f.Limit
	switch {
	case limit <= 0:
		limit = DefaultFunnelPageSize
	case limit > MaxFunnelPageSize:
		limit = MaxFunnelPageSize
	}

	// Allocate before Scan: a nil slice marshals to {} downstream, which
	// defeats a caller's `?? []`.
	rawRows := make([]sessionRowScan, 0, limit)

	pageQ := applySessionFilter(r.db.WithContext(ctx).Table("onboarding_sessions"), f)
	err := pageQ.
		Select(
			"id", "email", "status", "email_verified_at", "tenant_id",
			"completed_at", "last_activity_at", "created_at",
			fmt.Sprintf("(%s) AS abandoned", abandoned),
		).
		Order("created_at DESC").
		Offset((page - 1) * limit).
		Limit(limit).
		Scan(&rawRows).Error
	if err != nil {
		return nil, 0, fmt.Errorf("onboarding: list sessions: %w", err)
	}

	rows := make([]SessionRow, 0, len(rawRows))
	for _, raw := range rawRows {
		rows = append(rows, SessionRow{
			ID:              raw.ID,
			Email:           raw.Email,
			Status:          raw.Status,
			EmailVerifiedAt: raw.EmailVerifiedAt,
			TenantID:        raw.TenantID,
			CompletedAt:     raw.CompletedAt,
			LastActivityAt:  raw.LastActivityAt,
			CreatedAt:       raw.CreatedAt,
			Abandoned:       raw.Abandoned,
		})
	}
	return rows, total, nil
}
