package platformadmin_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
)

// stubHealthSource returns canned measurements. Every value asserted
// against is DISTINCT and NON-ZERO so an assertion cannot pass on a
// fabricated zero produced by a missing map key — the corollary to trap 6
// that bit twice. The one deliberate exception is
// StripeWebhooksHealth.ManualReviewRequired, which healthFixture sets to 0
// so that stripe_webhooks reports `ok` in the golden fixture (Task 5),
// giving that file a mix of statuses rather than uniform degradation.
type stubHealthSource struct {
	outbox    platformadmin.OutboxHealth
	csv       platformadmin.CSVJobsHealth
	campaign  platformadmin.CampaignSendsHealth
	stripe    platformadmin.StripeWebhooksHealth
	outboxErr error
}

func (s *stubHealthSource) Outbox(context.Context, time.Time) (platformadmin.OutboxHealth, error) {
	return s.outbox, s.outboxErr
}
func (s *stubHealthSource) CSVJobs(context.Context, time.Time) (platformadmin.CSVJobsHealth, error) {
	return s.csv, nil
}
func (s *stubHealthSource) CampaignSends(context.Context, time.Time) (platformadmin.CampaignSendsHealth, error) {
	return s.campaign, nil
}
func (s *stubHealthSource) StripeWebhooks(context.Context, time.Time) (platformadmin.StripeWebhooksHealth, error) {
	return s.stripe, nil
}

// healthFixture is the one shared stub set, so the golden fixture in Task 5
// and the assertions here cannot drift apart.
func healthFixture() *stubHealthSource {
	return &stubHealthSource{
		outbox:   platformadmin.OutboxHealth{Pending: 7, OldestPendingAgeSeconds: 400},
		csv:      platformadmin.CSVJobsHealth{Queued: 5, RunningStaleHeartbeat: 2},
		campaign: platformadmin.CampaignSendsHealth{Sending: 9, SendingStaleHeartbeat: 4},
		stripe: platformadmin.StripeWebhooksHealth{
			Unprocessed: 6, OldestUnprocessedAgeSeconds: 62, ManualReviewRequired: 0,
		},
	}
}

func healthRouter(t *testing.T, src platformadmin.HealthSource) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	platformadmin.NewHealthHandler(src, nil).Register(r.Group(""))
	return r
}

type healthBody struct {
	Data struct {
		CheckedAt    string `json:"checked_at"`
		Dependencies []struct {
			Name    string           `json:"name"`
			Status  string           `json:"status"`
			Metrics map[string]int64 `json:"metrics"`
		} `json:"dependencies"`
	} `json:"data"`
}

func getHealth(t *testing.T, src platformadmin.HealthSource) (*httptest.ResponseRecorder, healthBody) {
	t.Helper()
	rec := httptest.NewRecorder()
	healthRouter(t, src).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/health", nil))
	var body healthBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	return rec, body
}

// Every registry entry must appear, in registry order. The registry's whole
// purpose is that a dependency cannot silently fall out of the response.
func TestHealthReportsEveryRegistryEntryInOrder(t *testing.T) {
	_, body := getHealth(t, healthFixture())

	// The nine names are written out literally, NOT derived from
	// DependencyRegistry. Deriving them would make this test self-referential:
	// deleting a registry entry would shrink the response and the expectation
	// together and the test would still pass, despite its name. If you are here
	// because you added a dependency, adding it to this list is the deliberate
	// friction — the console's contract changed.
	wantNames := []string{
		"outbox", "csv_import_jobs", "campaign_sends", "stripe_webhooks",
		"scheduled_jobs", "platform_api", "stripe_api", "email_delivery", "object_storage",
	}
	require.Len(t, body.Data.Dependencies, len(wantNames))
	for i, want := range wantNames {
		require.Equal(t, want, body.Data.Dependencies[i].Name, "dependency %d out of contract order", i)
	}

	// Cross-check against the registry too, so the two sources of truth are
	// pinned to each other as well as to the literal contract above.
	require.Len(t, body.Data.Dependencies, len(platformadmin.DependencyRegistry))
	for i, want := range platformadmin.DependencyRegistry {
		require.Equal(t, want.Name, body.Data.Dependencies[i].Name,
			"dependency %d out of registry order", i)
	}
}

// An uninstrumented dependency carries NO metrics key — not {}, not zeroes.
// A zeroed metrics block is indistinguishable from a healthy one.
func TestHealthUninstrumentedEntriesHaveNoMetricsKey(t *testing.T) {
	rec, body := getHealth(t, healthFixture())
	require.Equal(t, http.StatusOK, rec.Code)

	var raw struct {
		Data struct {
			Dependencies []map[string]json.RawMessage `json:"dependencies"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &raw))

	seen := 0
	for i, dep := range body.Data.Dependencies {
		if dep.Status != platformadmin.StatusNotInstrumented {
			continue
		}
		seen++
		_, present := raw.Data.Dependencies[i]["metrics"]
		require.False(t, present, "%s is not_instrumented and must omit metrics entirely", dep.Name)
	}
	require.Equal(t, 5, seen, "expected five uninstrumented dependencies")
}

// A failed check is `unknown`, never `ok`, and never fails the endpoint —
// acceptance criterion 2. The error text must not reach the caller.
func TestHealthFailedCheckIsUnknownAndDoesNotFailTheEndpoint(t *testing.T) {
	src := healthFixture()
	src.outboxErr = errors.New("pq: password authentication failed for user \"dev\"")

	rec, body := getHealth(t, src)
	require.Equal(t, http.StatusOK, rec.Code, "a degraded dependency must not fail the endpoint")
	require.NotContains(t, rec.Body.String(), "password authentication",
		"driver error text must be logged server-side, never echoed")

	for _, dep := range body.Data.Dependencies {
		if dep.Name == "outbox" {
			require.Equal(t, platformadmin.StatusUnknown, dep.Status)
			require.Nil(t, dep.Metrics, "an unmeasured dependency must not ship fabricated zeroes")
		}
	}
	// The other checks still report.
	for _, dep := range body.Data.Dependencies {
		if dep.Name == "csv_import_jobs" {
			require.Equal(t, platformadmin.StatusDegraded, dep.Status)
		}
	}
}

// Thresholds: each instrumented dependency is degraded on its own rule.
func TestHealthStatusPerThreshold(t *testing.T) {
	src := &stubHealthSource{
		// Pending but young: ok.
		outbox:   platformadmin.OutboxHealth{Pending: 4, OldestPendingAgeSeconds: 1},
		csv:      platformadmin.CSVJobsHealth{Queued: 3, RunningStaleHeartbeat: 0},
		campaign: platformadmin.CampaignSendsHealth{Sending: 2, SendingStaleHeartbeat: 0},
		stripe:   platformadmin.StripeWebhooksHealth{Unprocessed: 1, OldestUnprocessedAgeSeconds: 1},
	}
	_, body := getHealth(t, src)
	for _, dep := range body.Data.Dependencies {
		switch dep.Name {
		case "outbox", "csv_import_jobs", "campaign_sends", "stripe_webhooks":
			require.Equal(t, platformadmin.StatusOK, dep.Status, "%s should be ok", dep.Name)
		}
	}

	// Outbox degrades on age alone. The fixture sits ON the threshold
	// instant: exactly OutboxPendingThreshold is degraded (age >= window).
	src.outbox = platformadmin.OutboxHealth{
		Pending: 1, OldestPendingAgeSeconds: int64(platformadmin.OutboxPendingThreshold / time.Second),
	}
	_, body = getHealth(t, src)
	for _, dep := range body.Data.Dependencies {
		if dep.Name == "outbox" {
			require.Equal(t, platformadmin.StatusDegraded, dep.Status,
				"an age exactly equal to the threshold is degraded")
		}
	}

	// One second under the threshold is ok — this pins the boundary from
	// the other side, so `>=` cannot silently become `>`.
	src.outbox = platformadmin.OutboxHealth{
		Pending: 1, OldestPendingAgeSeconds: int64(platformadmin.OutboxPendingThreshold/time.Second) - 1,
	}
	_, body = getHealth(t, src)
	for _, dep := range body.Data.Dependencies {
		if dep.Name == "outbox" {
			require.Equal(t, platformadmin.StatusOK, dep.Status,
				"one second under the threshold is ok")
		}
	}

	// manual_review_required is the system's own "a human must look" flag.
	src.outbox = platformadmin.OutboxHealth{Pending: 0, OldestPendingAgeSeconds: 0}
	src.stripe = platformadmin.StripeWebhooksHealth{Unprocessed: 1, OldestUnprocessedAgeSeconds: 1, ManualReviewRequired: 1}
	_, body = getHealth(t, src)
	for _, dep := range body.Data.Dependencies {
		if dep.Name == "stripe_webhooks" {
			require.Equal(t, platformadmin.StatusDegraded, dep.Status)
		}
	}

	// campaign_sends degrades on a stale heartbeat alone. Use a distinct,
	// non-zero value (3) so it cannot be confused with another stub's field.
	src.stripe = platformadmin.StripeWebhooksHealth{Unprocessed: 1, OldestUnprocessedAgeSeconds: 1}
	src.campaign = platformadmin.CampaignSendsHealth{Sending: 2, SendingStaleHeartbeat: 3}
	_, body = getHealth(t, src)
	for _, dep := range body.Data.Dependencies {
		if dep.Name == "campaign_sends" {
			require.Equal(t, platformadmin.StatusDegraded, dep.Status,
				"a stale campaign_sends heartbeat is degraded")
		}
	}
	src.campaign = platformadmin.CampaignSendsHealth{Sending: 2, SendingStaleHeartbeat: 0}

	// stripe_webhooks age threshold, pinned from both sides with
	// ManualReviewRequired: 0 throughout so the age disjunct alone is under
	// test, mirroring how the outbox age threshold is pinned above.
	src.stripe = platformadmin.StripeWebhooksHealth{
		Unprocessed:                 1,
		OldestUnprocessedAgeSeconds: int64(platformadmin.StripeUnprocessedThreshold / time.Second),
		ManualReviewRequired:        0,
	}
	_, body = getHealth(t, src)
	for _, dep := range body.Data.Dependencies {
		if dep.Name == "stripe_webhooks" {
			require.Equal(t, platformadmin.StatusDegraded, dep.Status,
				"a stripe age exactly equal to the threshold is degraded")
		}
	}

	src.stripe = platformadmin.StripeWebhooksHealth{
		Unprocessed:                 1,
		OldestUnprocessedAgeSeconds: int64(platformadmin.StripeUnprocessedThreshold/time.Second) - 1,
		ManualReviewRequired:        0,
	}
	_, body = getHealth(t, src)
	for _, dep := range body.Data.Dependencies {
		if dep.Name == "stripe_webhooks" {
			require.Equal(t, platformadmin.StatusOK, dep.Status,
				"one second under the stripe age threshold is ok")
		}
	}
}
