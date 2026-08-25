package stripe_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	billingstripe "github.com/mark8ly/marketplace-api/internal/billing/stripe"
	"github.com/mark8ly/marketplace-api/internal/subscription"
)

// subJSON returns a minimal Stripe subscription JSON with one item.
// itemID is the subscription item ID (si_xxx); priceID is the price on that item.
func subJSON(subID, itemID, priceID string) string {
	return `{
		"id":"` + subID + `","status":"active","currency":"usd",
		"items":{"data":[{"id":"` + itemID + `","price":{"id":"` + priceID + `","currency":"usd"}}]}
	}`
}

// TestUpdateSubscription_SendsProrationAndIdempotency verifies that
// UpdateSubscription issues a POST to /v1/subscriptions/:id with the correct
// proration_behavior, items[0][id], items[0][price], and metadata fields,
// and returns the mapped Subscription from the response.
func TestUpdateSubscription_SendsProrationAndIdempotency(t *testing.T) {
	const (
		subID    = "sub_x"
		itemID   = "si_xxx"
		oldPrice = "price_old"
		newPrice = "price_new"
	)

	var (
		postBody  url.Values
		getCount  int
		postCount int
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/subscriptions/"+subID:
			getCount++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(subJSON(subID, itemID, oldPrice)))

		case r.Method == http.MethodPost && r.URL.Path == "/v1/subscriptions/"+subID:
			postCount++
			b, _ := io.ReadAll(r.Body)
			postBody, _ = url.ParseQuery(string(b))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(subJSON(subID, itemID, newPrice)))

		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := billingstripe.New("sk_test_x")
	c.SetBaseURLForTesting(srv.URL)

	got, err := billingstripe.UpdateSubscription(context.Background(), c, billingstripe.UpdateSubscriptionParams{
		SubscriptionID:    subID,
		PriceID:           newPrice,
		ProrationBehavior: billingstripe.ProrationAlwaysInvoice,
		IdempotencyKey:    "ikey_abc",
		Metadata:          map[string]string{"plan": "studio", "period": "monthly"},
	})

	require.NoError(t, err)
	require.Equal(t, subID, got.ID)
	require.Equal(t, "active", got.Status)
	require.Equal(t, 1, getCount, "expected one GET to retrieve item ID")
	require.Equal(t, 1, postCount, "expected one POST to update subscription")

	require.Equal(t, "always_invoice", postBody.Get("proration_behavior"))
	require.Equal(t, itemID, postBody.Get("items[0][id]"))
	require.Equal(t, newPrice, postBody.Get("items[0][price]"))
	require.Equal(t, "studio", postBody.Get("metadata[plan]"))
	require.Equal(t, "monthly", postBody.Get("metadata[period]"))
}

// TestUpdateSubscription_FailsWhenNoItems verifies that UpdateSubscription
// returns an error when the fetched subscription has an empty items list.
func TestUpdateSubscription_FailsWhenNoItems(t *testing.T) {
	const subID = "sub_empty"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/subscriptions/"+subID {
			_, _ = w.Write([]byte(`{"id":"` + subID + `","status":"active","items":{"data":[]}}`))
			return
		}
		t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	c := billingstripe.New("sk_test_x")
	c.SetBaseURLForTesting(srv.URL)

	_, err := billingstripe.UpdateSubscription(context.Background(), c, billingstripe.UpdateSubscriptionParams{
		SubscriptionID:    subID,
		PriceID:           "price_new",
		ProrationBehavior: billingstripe.ProrationNone,
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "no items")
}

// TestUpdateSubscription_PropagatesAPIError verifies that a Stripe 402 response
// is surfaced as an *APIError via errors.As.
func TestUpdateSubscription_PropagatesAPIError(t *testing.T) {
	const subID = "sub_y"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/subscriptions/"+subID:
			_, _ = w.Write([]byte(subJSON(subID, "si_yyy", "price_old")))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/subscriptions/"+subID:
			w.Header().Set("Request-Id", "req_err_1")
			w.WriteHeader(http.StatusPaymentRequired)
			_, _ = w.Write([]byte(`{"error":{"type":"card_error","code":"card_declined","message":"card declined"}}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := billingstripe.New("sk_test_x")
	c.SetBaseURLForTesting(srv.URL)

	_, err := billingstripe.UpdateSubscription(context.Background(), c, billingstripe.UpdateSubscriptionParams{
		SubscriptionID:    subID,
		PriceID:           "price_new",
		ProrationBehavior: billingstripe.ProrationAlwaysInvoice,
	})

	require.Error(t, err)
	var apiErr *billingstripe.APIError
	require.True(t, errors.As(err, &apiErr), "expected *APIError, got %T: %v", err, err)
	require.Equal(t, http.StatusPaymentRequired, apiErr.HTTPStatus)
}

// TestPriceIDFor_DevelopedTier_ResolvesByLookupKey verifies that PriceIDFor
// calls FindPriceByLookupKey with the correct developed-tier lookup key and
// returns the price ID from the response.
func TestPriceIDFor_DevelopedTier_ResolvesByLookupKey(t *testing.T) {
	var seenURI string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenURI = r.URL.RequestURI()
		_, _ = w.Write([]byte(`{"data":[{"id":"price_starter_monthly_dev","lookup_key":"mark8ly_starter_monthly_developed_v1","active":true}]}`))
	}))
	defer srv.Close()

	c := billingstripe.New("sk_test_x")
	c.SetBaseURLForTesting(srv.URL)

	id, err := billingstripe.PriceIDFor(
		context.Background(), c,
		subscription.PlanStarter,
		subscription.PeriodMonthly,
		"usd",
		subscription.PriceTierDeveloped,
	)

	require.NoError(t, err)
	require.Equal(t, "price_starter_monthly_dev", id)
	require.Contains(t, seenURI, "mark8ly_starter_monthly_developed_v1")
}

// TestPriceIDFor_PPPTier_EncodesCurrentCurrency verifies that PriceIDFor
// encodes the currency in the PPP lookup key.
func TestPriceIDFor_PPPTier_EncodesCurrentCurrency(t *testing.T) {
	var seenURI string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenURI = r.URL.RequestURI()
		_, _ = w.Write([]byte(`{"data":[{"id":"price_starter_monthly_ppp_inr","lookup_key":"mark8ly_starter_monthly_ppp_inr_v1","active":true}]}`))
	}))
	defer srv.Close()

	c := billingstripe.New("sk_test_x")
	c.SetBaseURLForTesting(srv.URL)

	id, err := billingstripe.PriceIDFor(
		context.Background(), c,
		subscription.PlanStarter,
		subscription.PeriodMonthly,
		"inr",
		subscription.PriceTierPPP,
	)

	require.NoError(t, err)
	require.Equal(t, "price_starter_monthly_ppp_inr", id)
	require.Contains(t, seenURI, "mark8ly_starter_monthly_ppp_inr_v1")
}

// TestPriceIDFor_ReturnsErrNotFoundWhenMissing verifies that PriceIDFor
// surfaces ErrNotFound when the Prices list returns empty data.
func TestPriceIDFor_ReturnsErrNotFoundWhenMissing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	c := billingstripe.New("sk_test_x")
	c.SetBaseURLForTesting(srv.URL)

	_, err := billingstripe.PriceIDFor(
		context.Background(), c,
		subscription.PlanStudio,
		subscription.PeriodAnnual,
		"gbp",
		subscription.PriceTierDeveloped,
	)

	require.True(t, errors.Is(err, billingstripe.ErrNotFound))
}

// subTrialJSON returns a trialing subscription with a trial_end and a
// billing_cycle_anchor, so a caller can read back what Stripe holds.
func subTrialJSON(subID, itemID, priceID string, trialEnd, anchor int64) string {
	return `{
		"id":"` + subID + `","status":"trialing","currency":"usd",
		"trial_end":` + strconv.FormatInt(trialEnd, 10) + `,
		"billing_cycle_anchor":` + strconv.FormatInt(anchor, 10) + `,
		"items":{"data":[{"id":"` + itemID + `","price":{"id":"` + priceID + `","currency":"usd"}}]}
	}`
}

// The acceptance criterion of #358: the EXACT integer must reach Stripe. A
// stub returns the zero value for a field nobody set, so asserting that a
// call happened proves nothing.
func TestUpdateTrialEnd_SendsExactTrialEndAndNoPrice(t *testing.T) {
	const (
		subID     = "sub_trial"
		itemID    = "si_trial"
		priceID   = "price_current"
		newEnd    = int64(1_790_000_000)
		oldEnd    = int64(1_780_000_000)
		oldAnchor = int64(1_780_000_000)
	)

	var postBody url.Values
	var postCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/subscriptions/"+subID:
			postCount++
			b, _ := io.ReadAll(r.Body)
			postBody, _ = url.ParseQuery(string(b))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(subTrialJSON(subID, itemID, priceID, newEnd, newEnd)))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	c := billingstripe.New("sk_test_x")
	c.SetBaseURLForTesting(srv.URL)

	sub, err := billingstripe.UpdateTrialEnd(context.Background(), c, billingstripe.UpdateTrialEndParams{
		SubscriptionID: subID,
		TrialEnd:       newEnd,
		IdempotencyKey: "trial_extend:store-1:abc",
		Metadata:       map[string]string{"mark8ly_reason_code": "goodwill"},
	})
	require.NoError(t, err)
	require.Equal(t, 1, postCount)

	// The exact integer, not "a call happened".
	require.Equal(t, strconv.FormatInt(newEnd, 10), postBody.Get("trial_end"))
	// The price must be untouched: no items key of any kind.
	for k := range postBody {
		require.NotContains(t, k, "items[", "extend must not send items: %s=%s", k, postBody.Get(k))
	}
	require.Equal(t, "none", postBody.Get("proration_behavior"))
	require.Empty(t, postBody.Get("trial_from_plan"), "trial_from_plan is rejected by Stripe alongside trial_end")
	require.Equal(t, "goodwill", postBody.Get("metadata[mark8ly_reason_code]"))

	// The mapped result carries what Stripe now holds.
	require.Equal(t, newEnd, sub.TrialEnd)
	require.Equal(t, newEnd, sub.BillingCycleAnchor)
	_ = oldEnd
	_ = oldAnchor
}

// GetSubscription must expose trial_end and billing_cycle_anchor, or the
// two-year bound cannot be validated and the anchor move cannot be confirmed.
func TestGetSubscription_MapsTrialEndAndAnchor(t *testing.T) {
	const subID = "sub_read"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(subTrialJSON(subID, "si_1", "price_1", 1_781_000_000, 1_779_000_000)))
	}))
	defer srv.Close()

	c := billingstripe.New("sk_test_x")
	c.SetBaseURLForTesting(srv.URL)

	sub, err := billingstripe.GetSubscription(context.Background(), c, subID)
	require.NoError(t, err)
	require.Equal(t, int64(1_781_000_000), sub.TrialEnd)
	require.Equal(t, int64(1_779_000_000), sub.BillingCycleAnchor)
	require.Equal(t, "trialing", sub.Status)
}

// A plan change must STILL swap the price. This is the regression guard for
// making Items conditional — and it is the only one that runs, because the
// planchange integration suite is 9 FAIL on fixture drift and never reaches
// the logic (see the plan's measured baseline).
func TestUpdateSubscription_StillSendsPrice_WhenPriceIDSet(t *testing.T) {
	const (
		subID    = "sub_plan"
		itemID   = "si_plan"
		newPrice = "price_new"
	)
	var postBody url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(subJSON(subID, itemID, "price_old")))
		case r.Method == http.MethodPost:
			b, _ := io.ReadAll(r.Body)
			postBody, _ = url.ParseQuery(string(b))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(subJSON(subID, itemID, newPrice)))
		}
	}))
	defer srv.Close()

	c := billingstripe.New("sk_test_x")
	c.SetBaseURLForTesting(srv.URL)

	_, err := billingstripe.UpdateSubscription(context.Background(), c, billingstripe.UpdateSubscriptionParams{
		SubscriptionID:    subID,
		PriceID:           newPrice,
		ProrationBehavior: billingstripe.ProrationCreateProrations,
	})
	require.NoError(t, err)
	require.Equal(t, itemID, postBody.Get("items[0][id]"))
	require.Equal(t, newPrice, postBody.Get("items[0][price]"))
	require.Empty(t, postBody.Get("trial_end"), "a plan change must not move the trial end")
}

// Neither a price nor a trial end is a no-op update, and a no-op update that
// silently succeeds is how a caller comes to believe something changed.
func TestUpdateSubscription_RejectsEmptyUpdate(t *testing.T) {
	c := billingstripe.New("sk_test_x")
	_, err := billingstripe.UpdateSubscription(context.Background(), c, billingstripe.UpdateSubscriptionParams{
		SubscriptionID: "sub_x",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "price_id or trial_end required")
}
