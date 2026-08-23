//go:build integration

package onboarding

import (
	"context"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/mark8ly/platform-api/internal/tenant"
	"github.com/mark8ly/platform-api/pkg/testdb"
)

// seedInFlightOrAbandoned inserts a non-completed session with an explicit
// created_at / last_activity_at. Fixtures are always relative to now() (via
// the caller) rather than absolute clock times, and last_activity_at is
// always set explicitly rather than left to the column default — the whole
// point of these tests is pinning behaviour around that column.
func seedInFlightOrAbandoned(t *testing.T, repo Repository, email, status string, createdAt, lastActivityAt time.Time) *Session {
	t.Helper()
	sess := &Session{
		Email:          email,
		Status:         status,
		CreatedAt:      createdAt,
		LastActivityAt: lastActivityAt,
	}
	if err := repo.Create(context.Background(), sess); err != nil {
		t.Fatalf("seed session %s: %v", email, err)
	}
	return sess
}

// seedCompletedSession creates a tenant row (required by the FK and the
// completed_consistency CHECK on onboarding_sessions) plus a completed
// session that took completionSeconds to finish from createdAt.
func seedCompletedSession(t *testing.T, repo Repository, db *gorm.DB, email string, createdAt time.Time, completionSeconds float64) *Session {
	t.Helper()
	ctx := context.Background()

	tn := &tenant.Tenant{
		Name:        "Funnel Test Co " + email,
		OwnerUserID: "owner-" + email,
		OwnerEmail:  email,
		Status:      tenant.StatusActive,
	}
	if err := db.WithContext(ctx).Create(tn).Error; err != nil {
		t.Fatalf("seed tenant for %s: %v", email, err)
	}

	completedAt := createdAt.Add(time.Duration(completionSeconds * float64(time.Second)))
	sess := &Session{
		Email:          email,
		Status:         StatusCompleted,
		CreatedAt:      createdAt,
		LastActivityAt: completedAt,
		TenantID:       &tn.ID,
		CompletedAt:    &completedAt,
	}
	if err := repo.Create(ctx, sess); err != nil {
		t.Fatalf("seed completed session %s: %v", email, err)
	}
	return sess
}

func setupFunnelTest(t *testing.T) (Repository, *gorm.DB) {
	t.Helper()
	db := testdb.NewDB(t,
		"outbox_events",
		"verification_tokens",
		"onboarding_sessions",
		"stores",
		"tenants",
	)
	return NewRepository(db), db
}

// TestIntegration_Funnel_BoundaryAbandonment pins the exactly-24h-idle rule:
// a session idle for AbandonedAfter minus a small delta is in flight, one
// idle for exactly AbandonedAfter is abandoned. This is arbitrary but
// decided, and must not silently drift.
//
// This pins the rule genuinely, not just apparently: FunnelFilter.AsOf
// freezes the instant the query evaluates "now" against, so the fixture at
// exactly asOf-AbandonedAfter lands on the true knife-edge in DB time
// rather than drifting past it during the query round trip (as it would if
// the fixture were built from Go's now() and the query used Postgres's
// independently-evaluated now()).
//
// Verified by mutation: changing abandonedExpr's <= to < makes this test
// fail (the at-cutoff session flips to in-flight) — see task-1-report.md.
func TestIntegration_Funnel_BoundaryAbandonment(t *testing.T) {
	repo, _ := setupFunnelTest(t)
	ctx := context.Background()
	asOf := time.Now()

	justUnderCutoff := asOf.Add(-AbandonedAfter + time.Minute)
	atCutoff := asOf.Add(-AbandonedAfter)

	seedInFlightOrAbandoned(t, repo, "in-flight-just-under@boundary.local", StatusInProgress, justUnderCutoff, justUnderCutoff)
	seedInFlightOrAbandoned(t, repo, "abandoned-at-cutoff@boundary.local", StatusInProgress, atCutoff, atCutoff)

	filter := FunnelFilter{
		CreatedFrom: asOf.Add(-48 * time.Hour),
		CreatedTo:   asOf.Add(time.Hour),
		AsOf:        asOf,
	}

	stats, err := repo.GetFunnel(ctx, filter)
	if err != nil {
		t.Fatalf("GetFunnel: %v", err)
	}
	if stats.Started != 2 {
		t.Fatalf("Started = %d, want 2", stats.Started)
	}
	if stats.InFlight != 1 {
		t.Errorf("InFlight = %d, want 1 (idle AbandonedAfter minus a minute)", stats.InFlight)
	}
	if stats.Abandoned != 1 {
		t.Errorf("Abandoned = %d, want 1 (idle exactly AbandonedAfter)", stats.Abandoned)
	}

	rows, _, err := repo.ListSessions(ctx, FunnelFilter{
		CreatedFrom: filter.CreatedFrom,
		CreatedTo:   filter.CreatedTo,
		AsOf:        asOf,
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	abandonedCount := 0
	for _, r := range rows {
		if r.Email == "abandoned-at-cutoff@boundary.local" && !r.Abandoned {
			t.Errorf("session idle exactly AbandonedAfter must be flagged abandoned")
		}
		if r.Email == "in-flight-just-under@boundary.local" && r.Abandoned {
			t.Errorf("session idle just under AbandonedAfter must NOT be flagged abandoned")
		}
		if r.Abandoned {
			abandonedCount++
		}
	}
	if abandonedCount != 1 {
		t.Errorf("ListSessions abandoned rows = %d, want 1", abandonedCount)
	}
}

// TestIntegration_Funnel_IdleHoursAgreesWithAbandoned proves idle_hours and
// abandoned can never contradict each other on the same row, because both
// are computed upstream from the same asOfExpr(f) — never from a caller's
// own clock. A session idle by exactly AbandonedAfter relative to a fixed
// AsOf must show abandoned == true AND idle_hours == 24 (within a tiny
// float epsilon); one idle by AbandonedAfter minus a small delta must show
// abandoned == false AND idle_hours just under 24.
//
// This assertion was impossible before this fix: idle_hours did not exist
// on SessionRow at all, so there was nothing to freeze against AsOf or
// compare against abandoned.
func TestIntegration_Funnel_IdleHoursAgreesWithAbandoned(t *testing.T) {
	repo, _ := setupFunnelTest(t)
	ctx := context.Background()
	// asOf is pinned to a clearly historical instant (not time.Now()) so
	// that a mutation using an independent now() for idle_hours can't hide
	// behind the sub-second gap between Go's clock and Postgres's own now()
	// at query time.
	asOf := time.Now().Add(-6 * time.Hour)

	justUnderCutoff := asOf.Add(-AbandonedAfter + time.Minute)
	atCutoff := asOf.Add(-AbandonedAfter)

	seedInFlightOrAbandoned(t, repo, "idle-just-under@idlehours.local", StatusInProgress, justUnderCutoff, justUnderCutoff)
	seedInFlightOrAbandoned(t, repo, "idle-at-cutoff@idlehours.local", StatusInProgress, atCutoff, atCutoff)

	rows, _, err := repo.ListSessions(ctx, FunnelFilter{
		CreatedFrom: asOf.Add(-48 * time.Hour),
		CreatedTo:   asOf.Add(time.Hour),
		AsOf:        asOf,
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}

	const epsilon = 0.001
	found := 0
	for _, r := range rows {
		switch r.Email {
		case "idle-at-cutoff@idlehours.local":
			found++
			if !r.Abandoned {
				t.Errorf("session idle exactly AbandonedAfter must be abandoned")
			}
			if diff := r.IdleHours - 24; diff < -epsilon || diff > epsilon {
				t.Errorf("idle_hours = %v, want 24 (within epsilon)", r.IdleHours)
			}
		case "idle-just-under@idlehours.local":
			found++
			if r.Abandoned {
				t.Errorf("session idle just under AbandonedAfter must NOT be abandoned")
			}
			if r.IdleHours >= 24 {
				t.Errorf("idle_hours = %v, want just under 24", r.IdleHours)
			}
		}
	}
	if found != 2 {
		t.Fatalf("expected to find both fixtures in ListSessions rows, found %d", found)
	}
}

// TestIntegration_Funnel_Partition proves completed + in_flight + abandoned
// == started exactly, over a mixed fixture. email_verified is a subset
// counter and must never be added into this sum.
func TestIntegration_Funnel_Partition(t *testing.T) {
	repo, db := setupFunnelTest(t)
	ctx := context.Background()
	now := time.Now()

	seedInFlightOrAbandoned(t, repo, "p-inflight-1@partition.local", StatusInProgress, now.Add(-2*time.Hour), now.Add(-2*time.Hour))
	seedInFlightOrAbandoned(t, repo, "p-inflight-2@partition.local", StatusVerifying, now.Add(-1*time.Hour), now.Add(-1*time.Hour))
	seedInFlightOrAbandoned(t, repo, "p-abandoned-1@partition.local", StatusInProgress, now.Add(-30*time.Hour), now.Add(-30*time.Hour))
	seedInFlightOrAbandoned(t, repo, "p-abandoned-2@partition.local", StatusVerifying, now.Add(-48*time.Hour), now.Add(-26*time.Hour))
	seedCompletedSession(t, repo, db, "p-completed-1@partition.local", now.Add(-3*time.Hour), 10)
	seedCompletedSession(t, repo, db, "p-completed-2@partition.local", now.Add(-4*time.Hour), 20)

	stats, err := repo.GetFunnel(ctx, FunnelFilter{
		CreatedFrom: now.Add(-72 * time.Hour),
		CreatedTo:   now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("GetFunnel: %v", err)
	}
	if stats.Started != 6 {
		t.Fatalf("Started = %d, want 6", stats.Started)
	}
	sum := stats.Completed + stats.InFlight + stats.Abandoned
	if sum != stats.Started {
		t.Errorf("completed(%d) + in_flight(%d) + abandoned(%d) = %d, want started(%d)",
			stats.Completed, stats.InFlight, stats.Abandoned, sum, stats.Started)
	}
}

// TestIntegration_Funnel_CrossEndpointAgreement proves the funnel's
// abandoned count and ListSessions' abandoned-flagged row count agree over
// the same window — the point of sharing one predicate.
//
// The fixture deliberately includes a session idle ~24.5h — squarely in
// the 24-25h band. Without a row in that band, GetFunnel's abandoned
// predicate (built from AbandonedAfter == 24h) and a hand-copied 25h
// predicate in ListSessions would classify every fixture identically and
// this test would pass even with the two predicates drifted apart, which
// is exactly what happened once: a reviewer hand-copied a 25h interval
// into ListSessions while GetFunnel kept 24h, and this test still passed
// because the old fixtures idled at 5h/40h/30h/completed — nowhere near
// the 24-25h band the drift moves. See task-4-report.md for the mutation
// that proves this row closes the gap.
func TestIntegration_Funnel_CrossEndpointAgreement(t *testing.T) {
	repo, db := setupFunnelTest(t)
	ctx := context.Background()
	now := time.Now()

	seedInFlightOrAbandoned(t, repo, "x-inflight@cross.local", StatusInProgress, now.Add(-5*time.Hour), now.Add(-5*time.Hour))
	seedInFlightOrAbandoned(t, repo, "x-abandoned-1@cross.local", StatusInProgress, now.Add(-40*time.Hour), now.Add(-40*time.Hour))
	seedInFlightOrAbandoned(t, repo, "x-abandoned-2@cross.local", StatusVerifying, now.Add(-90*time.Hour), now.Add(-30*time.Hour))
	seedInFlightOrAbandoned(t, repo, "x-abandoned-3-band@cross.local", StatusInProgress, now.Add(-24*time.Hour-30*time.Minute), now.Add(-24*time.Hour-30*time.Minute))
	seedCompletedSession(t, repo, db, "x-completed@cross.local", now.Add(-6*time.Hour), 15)

	filter := FunnelFilter{
		CreatedFrom: now.Add(-100 * time.Hour),
		CreatedTo:   now.Add(time.Hour),
		Limit:       500,
	}

	stats, err := repo.GetFunnel(ctx, filter)
	if err != nil {
		t.Fatalf("GetFunnel: %v", err)
	}

	rows, total, err := repo.ListSessions(ctx, filter)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if total != stats.Started {
		t.Fatalf("ListSessions total = %d, GetFunnel started = %d, want equal", total, stats.Started)
	}

	rowAbandoned := int64(0)
	for _, r := range rows {
		if r.Abandoned {
			rowAbandoned++
		}
	}
	if rowAbandoned != stats.Abandoned {
		t.Errorf("ListSessions abandoned rows = %d, GetFunnel.Abandoned = %d, want equal", rowAbandoned, stats.Abandoned)
	}
}

// TestIntegration_Funnel_MedianOddCount verifies percentile_cont(0.5) picks
// the middle value for an odd number of completions: 10s, 20s, 60s -> 20.
func TestIntegration_Funnel_MedianOddCount(t *testing.T) {
	repo, db := setupFunnelTest(t)
	ctx := context.Background()
	now := time.Now()

	seedCompletedSession(t, repo, db, "median-odd-1@median.local", now.Add(-1*time.Hour), 10)
	seedCompletedSession(t, repo, db, "median-odd-2@median.local", now.Add(-1*time.Hour), 20)
	seedCompletedSession(t, repo, db, "median-odd-3@median.local", now.Add(-1*time.Hour), 60)

	stats, err := repo.GetFunnel(ctx, FunnelFilter{
		CreatedFrom: now.Add(-24 * time.Hour),
		CreatedTo:   now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("GetFunnel: %v", err)
	}
	if stats.MedianCompletionSeconds == nil {
		t.Fatal("MedianCompletionSeconds is nil, want 20")
	}
	if got := *stats.MedianCompletionSeconds; got < 19.9 || got > 20.1 {
		t.Errorf("MedianCompletionSeconds = %v, want ~20", got)
	}
}

// TestIntegration_Funnel_MedianEvenCount verifies the even-count case, where
// a wrong percentile implementation (e.g. plain MEDIAN via a naive middle
// row pick) tends to show up: 10s and 20s -> 15.
func TestIntegration_Funnel_MedianEvenCount(t *testing.T) {
	repo, db := setupFunnelTest(t)
	ctx := context.Background()
	now := time.Now()

	seedCompletedSession(t, repo, db, "median-even-1@median.local", now.Add(-1*time.Hour), 10)
	seedCompletedSession(t, repo, db, "median-even-2@median.local", now.Add(-1*time.Hour), 20)

	stats, err := repo.GetFunnel(ctx, FunnelFilter{
		CreatedFrom: now.Add(-24 * time.Hour),
		CreatedTo:   now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("GetFunnel: %v", err)
	}
	if stats.MedianCompletionSeconds == nil {
		t.Fatal("MedianCompletionSeconds is nil, want 15")
	}
	if got := *stats.MedianCompletionSeconds; got < 14.9 || got > 15.1 {
		t.Errorf("MedianCompletionSeconds = %v, want ~15", got)
	}
}

// TestIntegration_Funnel_MedianZeroCompletions verifies that with no
// completions in the window the median scans as nil, not 0 — scanning into
// a plain float64 would silently turn "nothing completed" into "instant
// completion".
func TestIntegration_Funnel_MedianZeroCompletions(t *testing.T) {
	repo, _ := setupFunnelTest(t)
	ctx := context.Background()
	now := time.Now()

	seedInFlightOrAbandoned(t, repo, "no-completions@median.local", StatusInProgress, now.Add(-1*time.Hour), now.Add(-1*time.Hour))

	stats, err := repo.GetFunnel(ctx, FunnelFilter{
		CreatedFrom: now.Add(-24 * time.Hour),
		CreatedTo:   now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("GetFunnel: %v", err)
	}
	if stats.MedianCompletionSeconds != nil {
		t.Errorf("MedianCompletionSeconds = %v, want nil", *stats.MedianCompletionSeconds)
	}
}

// TestIntegration_Funnel_WindowExcludesOutsideSessions verifies a session
// created outside [from, to] appears in neither the counters nor the rows.
func TestIntegration_Funnel_WindowExcludesOutsideSessions(t *testing.T) {
	repo, _ := setupFunnelTest(t)
	ctx := context.Background()
	now := time.Now()

	// Inside the window.
	seedInFlightOrAbandoned(t, repo, "inside-window@window.local", StatusInProgress, now.Add(-10*time.Hour), now.Add(-10*time.Hour))
	// Outside the window (created well before `from`).
	seedInFlightOrAbandoned(t, repo, "outside-window@window.local", StatusInProgress, now.Add(-200*time.Hour), now.Add(-200*time.Hour))

	filter := FunnelFilter{
		CreatedFrom: now.Add(-48 * time.Hour),
		CreatedTo:   now.Add(time.Hour),
		Limit:       500,
	}

	stats, err := repo.GetFunnel(ctx, filter)
	if err != nil {
		t.Fatalf("GetFunnel: %v", err)
	}
	if stats.Started != 1 {
		t.Fatalf("Started = %d, want 1 (only the in-window session)", stats.Started)
	}

	rows, total, err := repo.ListSessions(ctx, filter)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if total != 1 {
		t.Fatalf("ListSessions total = %d, want 1", total)
	}
	for _, r := range rows {
		if r.Email == "outside-window@window.local" {
			t.Error("ListSessions returned a session created outside the window")
		}
	}
}

// TestIntegration_Funnel_WindowKeyReflectsEffectiveBounds proves GetFunnel
// populates the "window" field with the effective bounds it actually
// filtered on, RFC3339 in UTC — the field platform-api's real upstream
// response was missing entirely (marketplace-api's client, wire type and
// golden fixture all assumed it existed, but nothing populated it, so the
// console would always have received {"from":"","to":""} in production).
//
// Also pins the no-bounds case: when the caller supplies neither
// CreatedFrom nor CreatedTo, GetFunnel applies no window constraint at
// all (applyFunnelWindow adds nothing), so the effective bound on that
// side is "unbounded" and Window renders "" for it — not a computed
// default timestamp. That is the existing behaviour, described here
// truthfully rather than inventing new defaulting.
func TestIntegration_Funnel_WindowKeyReflectsEffectiveBounds(t *testing.T) {
	repo, _ := setupFunnelTest(t)
	ctx := context.Background()
	now := time.Now()

	seedInFlightOrAbandoned(t, repo, "window-key@window.local", StatusInProgress, now.Add(-1*time.Hour), now.Add(-1*time.Hour))

	from := now.Add(-48 * time.Hour)
	to := now.Add(time.Hour)

	stats, err := repo.GetFunnel(ctx, FunnelFilter{
		CreatedFrom: from,
		CreatedTo:   to,
	})
	if err != nil {
		t.Fatalf("GetFunnel: %v", err)
	}
	if got, want := stats.Window.From, from.UTC().Format(time.RFC3339); got != want {
		t.Errorf("Window.From = %q, want %q", got, want)
	}
	if got, want := stats.Window.To, to.UTC().Format(time.RFC3339); got != want {
		t.Errorf("Window.To = %q, want %q", got, want)
	}

	// No bounds supplied at all: the effective window is unbounded on
	// both sides, and the field must say so rather than fabricate a
	// default.
	unboundedStats, err := repo.GetFunnel(ctx, FunnelFilter{})
	if err != nil {
		t.Fatalf("GetFunnel (unbounded): %v", err)
	}
	if unboundedStats.Window.From != "" {
		t.Errorf("Window.From = %q with no CreatedFrom supplied, want \"\"", unboundedStats.Window.From)
	}
	if unboundedStats.Window.To != "" {
		t.Errorf("Window.To = %q with no CreatedTo supplied, want \"\"", unboundedStats.Window.To)
	}
}

// TestIntegration_Funnel_Last24hIgnoresWindow verifies last_24h is always
// computed over the trailing 24h regardless of the requested window — with
// a window entirely in the past, last_24h.started still counts a session
// created minutes ago.
func TestIntegration_Funnel_Last24hIgnoresWindow(t *testing.T) {
	repo, _ := setupFunnelTest(t)
	ctx := context.Background()
	now := time.Now()

	// Created just now — outside a window that's entirely in the past.
	seedInFlightOrAbandoned(t, repo, "recent@last24h.local", StatusInProgress, now.Add(-5*time.Minute), now.Add(-5*time.Minute))
	// Also seed a session inside the requested (past) window, to make sure
	// the two aggregates are actually independent rather than one just
	// echoing the other.
	seedInFlightOrAbandoned(t, repo, "in-past-window@last24h.local", StatusInProgress, now.Add(-200*time.Hour), now.Add(-200*time.Hour))

	stats, err := repo.GetFunnel(ctx, FunnelFilter{
		CreatedFrom: now.Add(-300 * time.Hour),
		CreatedTo:   now.Add(-100 * time.Hour),
	})
	if err != nil {
		t.Fatalf("GetFunnel: %v", err)
	}
	if stats.Started != 1 {
		t.Fatalf("windowed Started = %d, want 1 (only the session inside the requested window)", stats.Started)
	}
	if stats.Last24h.Started < 1 {
		t.Errorf("Last24h.Started = %d, want >= 1 (the session created 5 minutes ago)", stats.Last24h.Started)
	}
}

// TestIntegration_Funnel_Last24hPinnedToAsOf proves last_24h is computed
// relative to FunnelFilter.AsOf, not an independently-evaluated now() —
// the same defect class fc8b4198 fixed for idle_hours, in the last_24h
// clock instead. Before this test, replacing the shared asOf - INTERVAL
// '24 hours' expression with a literal now() in GetFunnel's last_24h query
// survived the entire suite, because every other last_24h assertion used
// real wall-clock fixtures where a leaked now() looks identical to the
// pinned AsOf.
//
// AsOf is pinned 10 days in the past — clearly historical, and far enough
// from real time.Now() that every fixture below sits nowhere near a real
// trailing-24h window. That distance is deliberate: it is what makes the
// primary assertion (Last24h.Started == 1) actually discriminate against a
// mutated, independently-evaluated now() rather than passing by
// coincidence. A 6h-in-the-past AsOf was tried first and rejected — a
// fixture placed just after such a near AsOf can still land inside the
// real last-24-real-hours window, so a now()-mutation would count it too
// and the test would pass either way.
//
// Also pins the upper bound: last_24h had no created_at <= asOf clause,
// so with AsOf pinned in the past a session created between AsOf and the
// real now() (i.e. "in the future" relative to AsOf) would still be
// counted. That session must NOT appear in Last24h.Started.
func TestIntegration_Funnel_Last24hPinnedToAsOf(t *testing.T) {
	repo, _ := setupFunnelTest(t)
	ctx := context.Background()
	asOf := time.Now().Add(-10 * 24 * time.Hour)

	// Within the trailing 24h of asOf: must count. Correct impl: yes
	// (asOf-23h is within (asOf-24h, asOf]). Mutated impl (real now()):
	// no — asOf-23h is ~10 days before real now(), nowhere near it.
	seedInFlightOrAbandoned(t, repo, "within-24h-of-asof@last24hpin.local", StatusInProgress,
		asOf.Add(-23*time.Hour), asOf.Add(-23*time.Hour))
	// More than 24h before asOf: must not count under either impl.
	seedInFlightOrAbandoned(t, repo, "before-24h-of-asof@last24hpin.local", StatusInProgress,
		asOf.Add(-25*time.Hour), asOf.Add(-25*time.Hour))
	// Created after asOf (but ~10 days before real now()): must not count
	// under either impl — this pins the missing-upper-bound nit
	// independently of the now()-mutation this test is primarily for.
	seedInFlightOrAbandoned(t, repo, "after-asof@last24hpin.local", StatusInProgress,
		asOf.Add(1*time.Hour), asOf.Add(1*time.Hour))

	stats, err := repo.GetFunnel(ctx, FunnelFilter{
		CreatedFrom: asOf.Add(-72 * time.Hour),
		CreatedTo:   asOf.Add(72 * time.Hour),
		AsOf:        asOf,
	})
	if err != nil {
		t.Fatalf("GetFunnel: %v", err)
	}
	if stats.Last24h.Started != 1 {
		t.Errorf("Last24h.Started = %d, want 1 (only the session within 24h before AsOf)", stats.Last24h.Started)
	}
}

// TestIntegration_Funnel_CompletedNeverAbandoned verifies a completed
// session is never flagged abandoned, however old and idle its
// last_activity_at is.
func TestIntegration_Funnel_CompletedNeverAbandoned(t *testing.T) {
	repo, db := setupFunnelTest(t)
	ctx := context.Background()
	now := time.Now()

	// completed session whose last_activity_at (== completed_at here) is
	// far in the past — would be "abandoned" by the raw idle check if the
	// completed-status exclusion weren't applied.
	seedCompletedSession(t, repo, db, "old-completed@never-abandoned.local", now.Add(-500*time.Hour), 30)

	stats, err := repo.GetFunnel(ctx, FunnelFilter{
		CreatedFrom: now.Add(-600 * time.Hour),
		CreatedTo:   now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("GetFunnel: %v", err)
	}
	if stats.Abandoned != 0 {
		t.Errorf("Abandoned = %d, want 0 (the only session is completed)", stats.Abandoned)
	}
	if stats.Completed != 1 {
		t.Errorf("Completed = %d, want 1", stats.Completed)
	}

	rows, _, err := repo.ListSessions(ctx, FunnelFilter{
		CreatedFrom: now.Add(-600 * time.Hour),
		CreatedTo:   now.Add(time.Hour),
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	for _, r := range rows {
		if r.Email == "old-completed@never-abandoned.local" && r.Abandoned {
			t.Error("completed session was flagged abandoned")
		}
	}
}

// TestIntegration_Funnel_AbandonedAfterConstantDrivesQuery proves the SQL
// predicate is built from the AbandonedAfter constant rather than a second,
// independently hardcoded interval: a session idle for exactly
// AbandonedAfter is abandoned, one idle for AbandonedAfter minus a minute
// is not. If the query hardcoded a different interval than the Go
// constant, this test would catch the drift without needing to touch the
// constant itself.
//
// Uses a fixed AsOf for the same reason TestIntegration_Funnel_
// BoundaryAbandonment does: without pinning the evaluation instant, the
// "exactly AbandonedAfter" fixture is evaluated against Postgres's own
// now(), which has already moved past the fixture's timestamp by the time
// the query runs, so the boundary is never actually exercised.
func TestIntegration_Funnel_AbandonedAfterConstantDrivesQuery(t *testing.T) {
	repo, _ := setupFunnelTest(t)
	ctx := context.Background()
	asOf := time.Now()

	seedInFlightOrAbandoned(t, repo, "just-under-cutoff@constant.local", StatusInProgress,
		asOf.Add(-AbandonedAfter+time.Minute), asOf.Add(-AbandonedAfter+time.Minute))
	seedInFlightOrAbandoned(t, repo, "at-cutoff@constant.local", StatusInProgress,
		asOf.Add(-AbandonedAfter), asOf.Add(-AbandonedAfter))

	stats, err := repo.GetFunnel(ctx, FunnelFilter{
		CreatedFrom: asOf.Add(-2 * AbandonedAfter),
		CreatedTo:   asOf.Add(time.Hour),
		AsOf:        asOf,
	})
	if err != nil {
		t.Fatalf("GetFunnel: %v", err)
	}
	if stats.InFlight != 1 {
		t.Errorf("InFlight = %d, want 1 (just under the AbandonedAfter cutoff)", stats.InFlight)
	}
	if stats.Abandoned != 1 {
		t.Errorf("Abandoned = %d, want 1 (exactly at the AbandonedAfter cutoff)", stats.Abandoned)
	}
}
