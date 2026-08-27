//go:build integration

package onboarding

import (
	"context"
	"testing"
	"time"

	"github.com/mark8ly/platform-api/pkg/testdb"
)

// #406: ListSessions hardcoded ORDER BY created_at DESC, so the one consumer
// that needs the OLDEST rows — mark8ly's /admin/inbox, which surfaces stalled
// onboarding as work waiting on a human — could not reach them. Asking for
// page 1 at limit N returned the N NEWEST sessions, and the genuinely stalled
// ones are by definition the least recently active.
//
// The ordering is applied by the database, not by the caller, so it holds for
// rows beyond the first page. That is the whole point: a client-side sort can
// only reorder what it was already given.
func TestIntegration_ListSessions_OrderByLastActivityAsc(t *testing.T) {
	db := testdb.NewTx(t)
	repo := NewRepository(db)
	now := time.Now().UTC()

	// Deliberately seeded so created_at DESC and last_activity_at ASC give
	// DIFFERENT orders — otherwise the test passes under the old behaviour.
	seedInFlightOrAbandoned(t, repo, "newest-created@example.com", "in_progress",
		now.Add(-2*time.Hour), now.Add(-1*time.Hour))
	seedInFlightOrAbandoned(t, repo, "stalest@example.com", "in_progress",
		now.Add(-72*time.Hour), now.Add(-70*time.Hour))
	seedInFlightOrAbandoned(t, repo, "middle@example.com", "in_progress",
		now.Add(-48*time.Hour), now.Add(-10*time.Hour))

	ctx := context.Background()

	rows, _, err := repo.ListSessions(ctx, FunnelFilter{
		Order: SessionOrderLastActivityAsc,
		Page:  1, Limit: 10,
	})
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(rows) < 3 {
		t.Fatalf("want at least 3 rows, got %d", len(rows))
	}
	for i := 1; i < len(rows); i++ {
		if rows[i].LastActivityAt.Before(rows[i-1].LastActivityAt) {
			t.Fatalf("row %d (%s, %s) is older than row %d (%s, %s); want last_activity_at ASC",
				i, rows[i].Email, rows[i].LastActivityAt,
				i-1, rows[i-1].Email, rows[i-1].LastActivityAt)
		}
	}
	if rows[0].Email != "stalest@example.com" {
		t.Fatalf("want the least recently active session first, got %q", rows[0].Email)
	}

	// The default must not move: every existing consumer reads newest-first.
	def, _, err := repo.ListSessions(ctx, FunnelFilter{Page: 1, Limit: 10})
	if err != nil {
		t.Fatalf("list sessions default: %v", err)
	}
	for i := 1; i < len(def); i++ {
		if def[i].CreatedAt.After(def[i-1].CreatedAt) {
			t.Fatalf("default order is not created_at DESC at row %d", i)
		}
	}
	if def[0].Email != "newest-created@example.com" {
		t.Fatalf("default order must stay created_at DESC, got %q first", def[0].Email)
	}
}

// An unrecognised order falls back to the default rather than erroring or —
// far worse — reaching the query builder as raw text. Matches the convention
// in parseFunnelFilter, where a malformed parameter takes the default.
func TestIntegration_ListSessions_UnknownOrderFallsBackToDefault(t *testing.T) {
	db := testdb.NewTx(t)
	repo := NewRepository(db)
	now := time.Now().UTC()

	seedInFlightOrAbandoned(t, repo, "a-newer@example.com", "in_progress",
		now.Add(-1*time.Hour), now.Add(-1*time.Hour))
	seedInFlightOrAbandoned(t, repo, "b-older@example.com", "in_progress",
		now.Add(-9*time.Hour), now.Add(-9*time.Hour))

	rows, _, err := repo.ListSessions(context.Background(), FunnelFilter{
		Order: SessionOrder("last_activity_at asc; drop table onboarding_sessions --"),
		Page:  1, Limit: 10,
	})
	if err != nil {
		t.Fatalf("unknown order must not error: %v", err)
	}
	if len(rows) < 2 {
		t.Fatalf("want at least 2 rows, got %d", len(rows))
	}
	if rows[0].Email != "a-newer@example.com" {
		t.Fatalf("unknown order must fall back to created_at DESC, got %q first", rows[0].Email)
	}
}
