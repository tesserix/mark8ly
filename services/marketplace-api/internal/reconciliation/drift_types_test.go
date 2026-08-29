package reconciliation

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	billingstripe "github.com/mark8ly/marketplace-api/internal/billing/stripe"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// stripeStub is a minimal Stripe API stand-in covering the three endpoints
// reconciliation touches: subscription retrieve, subscription list, price
// list. It records which paths were hit so a test can assert a lookup was
// NOT made.
type stripeStub struct {
	// retrieve is returned from GET /v1/subscriptions/{id}.
	retrieve map[string]any
	// list is returned as the data array of GET /v1/subscriptions.
	list []map[string]any
	// priceID is returned as the single result of GET /v1/prices.
	priceID string

	seenPaths []string
}

func (s *stripeStub) client(t *testing.T) *billingstripe.Client {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.seenPaths = append(s.seenPaths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Request-Id", "req_test")

		switch {
		case r.URL.Path == "/v1/prices":
			data := []map[string]any{}
			if s.priceID != "" {
				data = append(data, map[string]any{"id": s.priceID, "object": "price"})
			}
			writeList(t, w, data)
		case r.URL.Path == "/v1/subscriptions":
			writeList(t, w, s.list)
		case strings.HasPrefix(r.URL.Path, "/v1/subscriptions/"):
			require.NotNil(t, s.retrieve, "unexpected subscription retrieve")
			require.NoError(t, json.NewEncoder(w).Encode(s.retrieve))
		default:
			t.Errorf("stripeStub: unexpected path %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	// Deliberately not shaped like a real Stripe secret key: gitleaks'
	// stripe-access-token rule matches that prefix on sight, and a fake key
	// in a test is indistinguishable to it from a real one. The stub server
	// below never inspects the value.
	c := billingstripe.New("reconciliation-fake-key")
	c.SetBaseURLForTesting(srv.URL)
	return c
}

func writeList(t *testing.T, w http.ResponseWriter, data []map[string]any) {
	t.Helper()
	if data == nil {
		data = []map[string]any{}
	}
	require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
		"object":   "list",
		"url":      "/v1/x",
		"has_more": false,
		"data":     data,
	}))
}

// stripeSubJSON builds a subscription payload carrying one item on priceID.
func stripeSubJSON(id, status, priceID string, metadata map[string]string) map[string]any {
	return map[string]any{
		"id":       id,
		"object":   "subscription",
		"status":   status,
		"customer": "cus_reconcile",
		"metadata": metadata,
		"items": map[string]any{
			"object": "list",
			"data": []map[string]any{
				{"id": "si_1", "object": "subscription_item", "price": map[string]any{"id": priceID, "object": "price"}},
			},
		},
	}
}

func activeRow(storeID uuid.UUID, stripeSubID string) row {
	return row{
		StoreID:              storeID,
		TenantID:             uuid.New(),
		LocalStatus:          subscription.StatusActive,
		StripeCustomerID:     "cus_reconcile",
		StripeSubscriptionID: stripeSubID,
		Plan:                 subscription.PlanStarter,
		SubscriptionPeriod:   subscription.PeriodMonthly,
		BillingCurrency:      "USD",
		PriceTier:            subscription.PriceTierDeveloped,
	}
}

func driftCount(t *testing.T, driftType string) float64 {
	t.Helper()
	return testutil.ToFloat64(driftTotal.WithLabelValues(driftType))
}

// --- plan_mismatch ----------------------------------------------------------

// TestCheckOne_PlanMismatch_DetectedWhenStripePriceDiffers is the #425
// upgrade case: Stripe accepted the price swap, the local transaction rolled
// back, so the local plan resolves to a different price than the one Stripe
// is billing.
func TestCheckOne_PlanMismatch_DetectedWhenStripePriceDiffers(t *testing.T) {
	stub := &stripeStub{
		priceID:  "price_starter_monthly",
		retrieve: stripeSubJSON("sub_drift", "active", "price_pro_monthly", nil),
	}
	r := New(nil, stub.client(t), nil, nil)

	before := driftCount(t, DriftTypePlanMismatch)
	drift, err := r.checkOne(context.Background(), activeRow(uuid.New(), "sub_drift"))
	require.NoError(t, err)

	assert.Equal(t, DriftTypePlanMismatch, drift)
	assert.Equal(t, before+1, driftCount(t, DriftTypePlanMismatch),
		"plan_mismatch must increment the drift counter alerts are wired to")
}

// TestCheckOne_PlanMatch_NoDrift guards against the counter firing on every
// healthy subscription.
func TestCheckOne_PlanMatch_NoDrift(t *testing.T) {
	stub := &stripeStub{
		priceID:  "price_starter_monthly",
		retrieve: stripeSubJSON("sub_ok", "active", "price_starter_monthly", nil),
	}
	r := New(nil, stub.client(t), nil, nil)

	drift, err := r.checkOne(context.Background(), activeRow(uuid.New(), "sub_ok"))
	require.NoError(t, err)
	assert.Equal(t, "", drift)
}

// TestCheckOne_NonBillablePlan_SkipsPriceLookup pins the guard that keeps a
// signup row on the default `trial` plan from reaching
// pricing.MustGetDescriptor, which panics for plans with no Price object.
func TestCheckOne_NonBillablePlan_SkipsPriceLookup(t *testing.T) {
	stub := &stripeStub{
		priceID:  "price_starter_monthly",
		retrieve: stripeSubJSON("sub_trial", "active", "price_anything", nil),
	}
	r := New(nil, stub.client(t), nil, nil)

	sub := activeRow(uuid.New(), "sub_trial")
	sub.Plan = subscription.PlanTrial

	drift, err := r.checkOne(context.Background(), sub)
	require.NoError(t, err)
	assert.Equal(t, "", drift)
	assert.NotContains(t, stub.seenPaths, "/v1/prices",
		"a non-billable plan must never reach the catalog price lookup")
}

// --- locally_missing --------------------------------------------------------

// TestCheckOne_LocallyMissing_DetectsOrphanedStripeSubscription is the #425
// initial-subscription case: Stripe created and is billing a subscription
// tagged with our store id, and the local row records no id for it.
func TestCheckOne_LocallyMissing_DetectsOrphanedStripeSubscription(t *testing.T) {
	storeID := uuid.New()
	stub := &stripeStub{
		list: []map[string]any{
			stripeSubJSON("sub_orphan", "trialing", "price_starter_monthly",
				map[string]string{"mark8ly_store_id": storeID.String()}),
		},
	}
	r := New(nil, stub.client(t), nil, nil)

	sub := activeRow(storeID, "")
	sub.LocalStatus = subscription.StatusSignup

	before := driftCount(t, DriftTypeLocallyMissing)
	drift, err := r.checkOne(context.Background(), sub)
	require.NoError(t, err)

	assert.Equal(t, DriftTypeLocallyMissing, drift)
	assert.Equal(t, before+1, driftCount(t, DriftTypeLocallyMissing))
}

// TestCheckOne_LocallyMissing_IgnoresSubscriptionForAnotherStore verifies the
// match is attributed by metadata, not by "the customer has any subscription".
func TestCheckOne_LocallyMissing_IgnoresSubscriptionForAnotherStore(t *testing.T) {
	stub := &stripeStub{
		list: []map[string]any{
			stripeSubJSON("sub_other", "active", "price_starter_monthly",
				map[string]string{"mark8ly_store_id": uuid.NewString()}),
		},
	}
	r := New(nil, stub.client(t), nil, nil)

	sub := activeRow(uuid.New(), "")
	sub.LocalStatus = subscription.StatusSignup

	drift, err := r.checkOne(context.Background(), sub)
	require.NoError(t, err)
	assert.Equal(t, "", drift)
}

// TestCheckOne_LocallyMissing_NoCustomerIsNotDrift: a row we never bootstrapped
// at Stripe has nothing to diverge from.
func TestCheckOne_LocallyMissing_NoCustomerIsNotDrift(t *testing.T) {
	stub := &stripeStub{}
	r := New(nil, stub.client(t), nil, nil)

	sub := activeRow(uuid.New(), "")
	sub.StripeCustomerID = ""
	sub.LocalStatus = subscription.StatusSignup

	drift, err := r.checkOne(context.Background(), sub)
	require.NoError(t, err)
	assert.Equal(t, "", drift)
	assert.Empty(t, stub.seenPaths, "no Stripe call should be made without a customer id")
}

// TestNewDriftTypeConstants pins the label values the Prometheus alert rules
// and dashboards will match on.
func TestNewDriftTypeConstants(t *testing.T) {
	assert.Equal(t, "locally_missing", DriftTypeLocallyMissing)
	assert.Equal(t, "plan_mismatch", DriftTypePlanMismatch)
}
