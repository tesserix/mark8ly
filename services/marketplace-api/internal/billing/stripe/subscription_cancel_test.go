package stripe_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	billingstripe "github.com/mark8ly/marketplace-api/internal/billing/stripe"
)

// cancelSubJSON returns a subscription whose single item carries the billing
// period. CurrentPeriodStart/End live on the item in SDK v82, and the period
// end is the whole point of these calls — it is the date the merchant is told
// their access runs to.
func cancelSubJSON(subID string, cancelAtPeriodEnd bool, periodEnd int64) string {
	flag := "false"
	if cancelAtPeriodEnd {
		flag = "true"
	}
	return `{
		"id":"` + subID + `","status":"active","currency":"usd",
		"cancel_at_period_end":` + flag + `,
		"items":{"data":[{"id":"si_1","price":{"id":"price_1","currency":"usd"},
		"current_period_start":1700000000,"current_period_end":` + strconv.FormatInt(periodEnd, 10) + `}]}
	}`
}

// TestCancelAtPeriodEnd_SendsFlagAndReturnsPeriodEnd is the write that did not
// exist: cancellation was recorded locally and Stripe was never told. It must
// post cancel_at_period_end=true, send no items (nothing may be re-priced on
// the way out), and hand back the period end Stripe holds.
func TestCancelAtPeriodEnd_SendsFlagAndReturnsPeriodEnd(t *testing.T) {
	const subID = "sub_cancel"
	const periodEnd int64 = 1893456000

	var postBody url.Values
	var postCount int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/subscriptions/"+subID {
			postCount++
			b, _ := io.ReadAll(r.Body)
			postBody, _ = url.ParseQuery(string(b))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(cancelSubJSON(subID, true, periodEnd)))
			return
		}
		t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	c := billingstripe.New("sk_test_x")
	c.SetBaseURLForTesting(srv.URL)

	got, err := billingstripe.CancelAtPeriodEnd(context.Background(), c, billingstripe.CancelAtPeriodEndParams{
		SubscriptionID: subID,
		IdempotencyKey: "ikey_cancel",
		Metadata:       map[string]string{"reason": "merchant_cancelled"},
	})

	require.NoError(t, err)
	require.Equal(t, 1, postCount, "expected exactly one POST — no retrieve round-trip is needed")
	require.Equal(t, "true", postBody.Get("cancel_at_period_end"))
	require.Equal(t, "merchant_cancelled", postBody.Get("metadata[reason]"))

	// No items and no trial_end: this call must be structurally incapable of
	// re-pricing or moving the billing anchor on the way out.
	require.Empty(t, postBody.Get("items[0][id]"))
	require.Empty(t, postBody.Get("items[0][price]"))
	require.Empty(t, postBody.Get("trial_end"))

	require.True(t, got.CancelAtPeriodEnd)
	require.Equal(t, periodEnd, got.CurrentPeriodEnd,
		"the period end is what the merchant is told their access runs to")
}

// TestResumeSubscription_ClearsTheFlag is the save offer's other half: a
// merchant who un-cancels must not still be cancelled at Stripe.
func TestResumeSubscription_ClearsTheFlag(t *testing.T) {
	const subID = "sub_resume"

	var postBody url.Values

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/subscriptions/"+subID {
			b, _ := io.ReadAll(r.Body)
			postBody, _ = url.ParseQuery(string(b))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(cancelSubJSON(subID, false, 1893456000)))
			return
		}
		t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	c := billingstripe.New("sk_test_x")
	c.SetBaseURLForTesting(srv.URL)

	got, err := billingstripe.ResumeSubscription(context.Background(), c, billingstripe.ResumeSubscriptionParams{
		SubscriptionID: subID,
	})

	require.NoError(t, err)
	require.Equal(t, "false", postBody.Get("cancel_at_period_end"))
	require.False(t, got.CancelAtPeriodEnd)
}

// TestCancelAtPeriodEnd_RequiresSubscriptionID refuses before any network
// call: an empty id would POST to /v1/subscriptions, which is Create.
func TestCancelAtPeriodEnd_RequiresSubscriptionID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		t.Errorf("no request should be made, got %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()

	c := billingstripe.New("sk_test_x")
	c.SetBaseURLForTesting(srv.URL)

	_, err := billingstripe.CancelAtPeriodEnd(context.Background(), c, billingstripe.CancelAtPeriodEndParams{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "subscription_id required")

	_, err = billingstripe.ResumeSubscription(context.Background(), c, billingstripe.ResumeSubscriptionParams{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "subscription_id required")
}

// TestCancelAtPeriodEnd_PropagatesAPIError — a Stripe failure must surface, so
// the caller can refuse the cancellation rather than record one locally.
func TestCancelAtPeriodEnd_PropagatesAPIError(t *testing.T) {
	const subID = "sub_err"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"type":"api_error","message":"boom"}}`))
	}))
	defer srv.Close()

	c := billingstripe.New("sk_test_x")
	c.SetBaseURLForTesting(srv.URL)

	_, err := billingstripe.CancelAtPeriodEnd(context.Background(), c, billingstripe.CancelAtPeriodEndParams{
		SubscriptionID: subID,
	})
	require.Error(t, err)
}
