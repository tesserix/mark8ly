//go:build integration

package onboarding

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/mark8ly/platform-api/internal/tenant"
)

// TestIntegration_Funnel_ExternalView pins #992's contract: the all-sessions
// counts are unchanged by classification, the external view excludes test,
// internal and demo sessions, and completed merchants are counted once per
// email however many sessions they started.
func TestIntegration_Funnel_ExternalView(t *testing.T) {
	repo, db := setupFunnelTest(t)
	ctx := context.Background()
	asOf := time.Now()
	created := asOf.Add(-time.Hour)

	// Two external merchants: one completed on the second attempt (two
	// sessions, one completion), one still in flight.
	seedCompletedSession(t, repo, db, "retry@real-shop.com", created, 120)
	seedInFlightOrAbandoned(t, repo, "retry@real-shop.com", StatusInProgress, created.Add(-time.Minute), created.Add(-time.Minute))
	seedInFlightOrAbandoned(t, repo, "fresh@real-shop.com", StatusInProgress, created, created)

	// Sessions the external view must ignore.
	for _, c := range []struct{ email, class string }{
		{"e2e-1@example.com", ClassificationTest},
		{"ops@tesserix.app", ClassificationInternal},
		{"bondi@mark8ly.com", ClassificationDemo},
	} {
		sess := &Session{
			Email:          c.email,
			Status:         StatusInProgress,
			Classification: c.class,
			CreatedAt:      created,
			LastActivityAt: created,
		}
		if err := repo.Create(ctx, sess); err != nil {
			t.Fatalf("seed %s: %v", c.email, err)
		}
	}

	filter := FunnelFilter{
		CreatedFrom: asOf.Add(-24 * time.Hour),
		CreatedTo:   asOf.Add(time.Hour),
		AsOf:        asOf,
	}
	stats, err := repo.GetFunnel(ctx, filter)
	if err != nil {
		t.Fatalf("GetFunnel: %v", err)
	}

	// Operational totals count every session regardless of class.
	if stats.Started != 6 {
		t.Errorf("started = %d, want 6 (all sessions)", stats.Started)
	}
	if stats.Completed != 1 {
		t.Errorf("completed = %d, want 1", stats.Completed)
	}

	// The external view drops the three fixtures.
	if stats.External.Started != 3 {
		t.Errorf("external.started = %d, want 3", stats.External.Started)
	}
	if stats.External.Completed != 1 {
		t.Errorf("external.completed = %d, want 1", stats.External.Completed)
	}
	if stats.External.InFlight != 2 {
		t.Errorf("external.in_flight = %d, want 2", stats.External.InFlight)
	}
	if stats.External.CompletedMerchants != 1 {
		t.Errorf("external.completed_merchants = %d, want 1", stats.External.CompletedMerchants)
	}

	// The sessions list filters by classification and reports it per row.
	filter.Classification = ClassificationTest
	rows, total, err := repo.ListSessions(ctx, filter)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if total != 1 || len(rows) != 1 {
		t.Fatalf("test-class list: total=%d rows=%d, want 1/1", total, len(rows))
	}
	if rows[0].Email != "e2e-1@example.com" || rows[0].Classification != ClassificationTest {
		t.Errorf("unexpected row %+v", rows[0])
	}
}

// TestIntegration_Create_StoresSanitisedAcquisitionAndClassifies walks the
// service path a real signup takes: the raw browser record goes in, the
// sanitised one comes out, and the classification is decided server-side.
func TestIntegration_Create_StoresSanitisedAcquisitionAndClassifies(t *testing.T) {
	repo, db := setupFunnelTest(t)
	svc := NewService(Config{
		DB:         db,
		Repo:       repo,
		TenantRepo: tenant.NewRepository(db),
		Classifier: NewClassifier([]string{"tesserix.app"}, nil, nil),
	})
	ctx := context.Background()

	raw := json.RawMessage(`{"first":{"utm_source":"ig","utm_campaign":"launch","landing_path":"/?utm_source=ig&token=nope","referrer":"https://instagram.com/p/1?x=1","captured_at":"2026-10-05T10:00:00Z","password":"leak"}}`)

	sess, err := svc.Create(ctx, CreateRequest{Email: "Founder@Real-Shop.com", Acquisition: raw})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if sess.Classification != ClassificationExternal {
		t.Errorf("classification = %q, want external", sess.Classification)
	}
	stored, err := repo.GetByID(ctx, sess.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	var got Acquisition
	if err := json.Unmarshal(stored.Acquisition, &got); err != nil {
		t.Fatalf("stored acquisition is not JSON: %v (%s)", err, stored.Acquisition)
	}
	if got.First == nil || got.First.Campaign != "launch" || got.First.LandingPath != "/" || got.First.Referrer != "https://instagram.com/p/1" {
		t.Errorf("stored first touch = %+v", got.First)
	}
	for _, leak := range []string{"token", "leak", "password"} {
		if strings.Contains(string(stored.Acquisition), leak) {
			t.Errorf("stored acquisition leaks %q: %s", leak, stored.Acquisition)
		}
	}

	internal, err := svc.Create(ctx, CreateRequest{Email: "ops@tesserix.app", Acquisition: json.RawMessage(`not json`)})
	if err != nil {
		t.Fatalf("Create internal: %v", err)
	}
	if internal.Classification != ClassificationInternal {
		t.Errorf("internal classification = %q", internal.Classification)
	}
	if internal.Acquisition != nil {
		t.Errorf("malformed acquisition should be dropped, got %s", internal.Acquisition)
	}

	latest, err := repo.LatestByEmail(ctx, "founder@real-shop.com")
	if err != nil {
		t.Fatalf("LatestByEmail: %v", err)
	}
	if latest.ID != sess.ID {
		t.Errorf("LatestByEmail returned %s, want %s", latest.ID, sess.ID)
	}
}
