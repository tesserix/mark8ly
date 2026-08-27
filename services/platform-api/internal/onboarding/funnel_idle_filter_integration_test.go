//go:build integration

package onboarding

import (
	"context"
	"testing"
	"time"

	"github.com/mark8ly/platform-api/pkg/testdb"
)

// #406, second half: the inbox applied its idle-hours threshold CLIENT-side,
// after upstream had already paged. That made pagination.total wrong (it
// counted every abandoned session, not the ones past the threshold) and forced
// the consumer to fetch up to MaxAggregateItems rows just to count them, which
// saturated at that bound.
//
// Filtering server-side makes the SAME filter apply to the count query and the
// page query — they share applySessionFilter — so the total is exact and no
// paging-to-count is needed.
func TestIntegration_ListSessions_IdleHoursMinFiltersCountAndPage(t *testing.T) {
	db := testdb.NewTx(t)
	repo := NewRepository(db)
	now := time.Now().UTC()

	// AsOf pins "now" so the idle-hours arithmetic is exact rather than racing
	// the wall clock between seeding and querying.
	asOf := now

	// Idle ~100h: past both the abandoned cutoff and a 72h threshold.
	seedInFlightOrAbandoned(t, repo, "very-stale@example.com", "in_progress",
		now.Add(-200*time.Hour), now.Add(-100*time.Hour))
	// Idle ~50h: abandoned, but BELOW a 72h threshold.
	seedInFlightOrAbandoned(t, repo, "mildly-stale@example.com", "in_progress",
		now.Add(-200*time.Hour), now.Add(-50*time.Hour))

	minIdle := 72.0
	abandoned := true
	rows, total, err := repo.ListSessions(context.Background(), FunnelFilter{
		Abandoned:    &abandoned,
		IdleHoursMin: &minIdle,
		AsOf:         asOf,
		Page:         1, Limit: 50,
	})
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}

	if total != 1 {
		t.Fatalf("total must count only rows past the threshold, want 1 got %d", total)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rows))
	}
	if rows[0].Email != "very-stale@example.com" {
		t.Fatalf("want the >72h idle session, got %q", rows[0].Email)
	}

	// Without the threshold both abandoned rows are counted — proving the
	// assertion above is about the filter and not about the fixture.
	_, allTotal, err := repo.ListSessions(context.Background(), FunnelFilter{
		Abandoned: &abandoned,
		AsOf:      asOf,
		Page:      1, Limit: 50,
	})
	if err != nil {
		t.Fatalf("list sessions unfiltered: %v", err)
	}
	if allTotal != 2 {
		t.Fatalf("want 2 abandoned rows without the threshold, got %d", allTotal)
	}
}
