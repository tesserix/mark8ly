//go:build integration

package trial_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/billing/trial"
	"github.com/mark8ly/marketplace-api/internal/subscription"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// The scopes are SQL restatements of EndsAt, and the two-branch predicates
// are duplicated across three helpers. This test is what stops them drifting:
// for a matrix of extended and unextended rows placed on and around each
// boundary, every scope must return EXACTLY the rows EndsAt says it should.
//
// Delete either branch of any scope and this fails.
func TestScopesAgreeWithEndsAt(t *testing.T) {
	db := testdb.NewDB(t, "store_subscriptions", "stores")

	anchor := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)

	// Each row is built so its EFFECTIVE end sits at a named instant, via
	// both routes: unextended rows get created_at = end - 90d (via
	// seedExpiringRow's default), extended rows get created_at far away (200
	// days back) so the derived date is nowhere near the stored one. An
	// implementation that ignored trial_ends_at would place the extended rows
	// ~110 days in the past and fail every case.
	ends := []time.Time{
		anchor.Add(-48 * time.Hour),     // well before
		anchor,                          // exactly on the anchor
		anchor.Add(72 * time.Hour),      // inside a 7-day window
		anchor.Add(30 * 24 * time.Hour), // well after
	}

	var all []subscription.StoreSubscription
	for _, e := range ends {
		all = append(all, seedExpiringRow(t, db, e, nil)) // unextended
		stored := e
		all = append(all, seedExpiringRow(t, db, e, func(r *subscription.StoreSubscription) {
			r.CreatedAt = anchor.Add(-200 * 24 * time.Hour)
			r.TrialEndsAt = &stored
		})) // extended
	}

	// Sanity: EndsAt agrees with where we intended to put each row. Without
	// this, a bug in the fixture could make the comparisons below vacuous.
	for i, sub := range all {
		require.Equal(t, ends[i/2].UTC(), trial.EndsAt(sub),
			"fixture row %d is not where the test thinks it is", i)
	}

	idsFrom := func(rows []subscription.StoreSubscription) map[uuid.UUID]bool {
		m := map[uuid.UUID]bool{}
		for _, r := range rows {
			m[r.ID] = true
		}
		return m
	}
	// expected computes the answer in GO, from EndsAt — never from the SQL
	// under test.
	expected := func(pred func(time.Time) bool) map[uuid.UUID]bool {
		m := map[uuid.UUID]bool{}
		for _, s := range all {
			if pred(trial.EndsAt(s)) {
				m[s.ID] = true
			}
		}
		return m
	}
	query := func(scope func(*gorm.DB) *gorm.DB) map[uuid.UUID]bool {
		var got []subscription.StoreSubscription
		require.NoError(t, scope(db.Model(&subscription.StoreSubscription{})).Find(&got).Error)
		return idsFrom(got)
	}

	t.Run("EndedBeforeScope", func(t *testing.T) {
		got := query(func(d *gorm.DB) *gorm.DB { return trial.EndedBeforeScope(d, anchor) })
		want := expected(func(e time.Time) bool { return e.Before(anchor) })
		require.Equal(t, want, got)
		require.NotEmpty(t, want, "the predicate must match something, or this proves nothing")
	})

	t.Run("EndsBetweenScope is half-open left, inclusive right", func(t *testing.T) {
		hi := anchor.Add(7 * 24 * time.Hour)
		got := query(func(d *gorm.DB) *gorm.DB { return trial.EndsBetweenScope(d, anchor, hi) })
		want := expected(func(e time.Time) bool { return e.After(anchor) && !e.After(hi) })
		require.Equal(t, want, got)
		// The row sitting exactly on `anchor` must be EXCLUDED, and that is
		// the case a `>=` implementation would get wrong.
		require.NotEmpty(t, want)
	})

	t.Run("EndsBetweenScope right edge is inclusive", func(t *testing.T) {
		hi := anchor.Add(72 * time.Hour) // a row ends exactly here
		got := query(func(d *gorm.DB) *gorm.DB { return trial.EndsBetweenScope(d, anchor, hi) })
		want := expected(func(e time.Time) bool { return e.After(anchor) && !e.After(hi) })
		require.Equal(t, want, got)
		require.Len(t, want, 2, "both the extended and unextended row on that instant must be included")
	})

	t.Run("EndsWithinDayScope is inclusive left, exclusive right", func(t *testing.T) {
		dayStart := anchor // a row ends exactly at dayStart
		got := query(func(d *gorm.DB) *gorm.DB { return trial.EndsWithinDayScope(d, dayStart) })
		want := expected(func(e time.Time) bool {
			return !e.Before(dayStart) && e.Before(dayStart.Add(24*time.Hour))
		})
		require.Equal(t, want, got)
		require.Len(t, want, 2, "the row exactly on dayStart must be INCLUDED — that is the bracket that differs from EndsBetweenScope")
	})
}
