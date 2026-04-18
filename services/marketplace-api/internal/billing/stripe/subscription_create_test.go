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
)

func TestCreateSubscription_SendsTrialEndAndIdempotencyKey(t *testing.T) {
	const trialEnd int64 = 1_784_140_800 // 2026-07-17 UTC, arbitrary future unix
	var seenKey string
	var seenForm url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/v1/subscriptions", r.URL.Path)
		seenKey = r.Header.Get("Idempotency-Key")
		body, _ := io.ReadAll(r.Body)
		seenForm, _ = url.ParseQuery(string(body))
		w.Header().Set("Request-Id", "req_test")
		_, _ = w.Write([]byte(`{"id":"sub_created","status":"trialing","customer":"cus_123"}`))
	}))
	defer srv.Close()

	c := billingstripe.New("sk_test_x")
	c.SetBaseURLForTesting(srv.URL)

	sub, err := billingstripe.CreateSubscription(context.Background(), c, billingstripe.CreateSubscriptionInput{
		StoreID:    "store_abc",
		Plan:       "starter",
		Period:     "monthly",
		CustomerID: "cus_123",
		PriceID:    "price_starter_monthly_gbp",
		TrialEnd:   trialEnd,
	})
	require.NoError(t, err)
	require.Equal(t, "sub_created", sub.ID)

	require.Equal(t, "subscription:store_abc:starter:monthly", seenKey)
	require.Equal(t, "cus_123", seenForm.Get("customer"))
	require.Equal(t, "price_starter_monthly_gbp", seenForm.Get("items[0][price]"))
	require.Equal(t, "none", seenForm.Get("proration_behavior"))
	require.Equal(t, strconv.FormatInt(trialEnd, 10), seenForm.Get("trial_end"))
	require.Equal(t, "store_abc", seenForm.Get("metadata[mark8ly_store_id]"))
	require.Equal(t, "starter", seenForm.Get("metadata[mark8ly_plan]"))
	require.Equal(t, "monthly", seenForm.Get("metadata[mark8ly_period]"))
}

func TestCreateSubscription_IdempotencyKeyStableAcrossCalls(t *testing.T) {
	keys := []string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		keys = append(keys, r.Header.Get("Idempotency-Key"))
		_, _ = w.Write([]byte(`{"id":"sub_x"}`))
	}))
	defer srv.Close()

	c := billingstripe.New("sk_test_x")
	c.SetBaseURLForTesting(srv.URL)

	in := billingstripe.CreateSubscriptionInput{
		StoreID: "store_abc", Plan: "starter", Period: "monthly",
		CustomerID: "cus_1", PriceID: "price_x", TrialEnd: 1_784_140_800,
	}
	_, err := billingstripe.CreateSubscription(context.Background(), c, in)
	require.NoError(t, err)
	_, err = billingstripe.CreateSubscription(context.Background(), c, in)
	require.NoError(t, err)

	require.Len(t, keys, 2)
	require.Equal(t, keys[0], keys[1], "same (store, plan, period) must produce the same idempotency key")
}

func TestCreateSubscription_RejectsMissingCustomerOrPrice(t *testing.T) {
	c := billingstripe.New("sk_test_x")

	_, err := billingstripe.CreateSubscription(context.Background(), c, billingstripe.CreateSubscriptionInput{
		Plan: "starter", Period: "monthly", PriceID: "price_x", TrialEnd: 1_784_140_800,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "customer + price required")

	_, err = billingstripe.CreateSubscription(context.Background(), c, billingstripe.CreateSubscriptionInput{
		Plan: "starter", Period: "monthly", CustomerID: "cus_x", TrialEnd: 1_784_140_800,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "customer + price required")
}

func TestCreateSubscription_RejectsMissingTrialEnd(t *testing.T) {
	c := billingstripe.New("sk_test_x")
	_, err := billingstripe.CreateSubscription(context.Background(), c, billingstripe.CreateSubscriptionInput{
		StoreID: "s", Plan: "starter", Period: "monthly",
		CustomerID: "cus_x", PriceID: "price_x",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "trial_end required")
}

func TestCreateSubscription_PropagatesAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Request-Id", "req_err")
		w.WriteHeader(http.StatusPaymentRequired)
		_, _ = io.WriteString(w, `{"error":{"code":"card_declined","type":"card_error","message":"declined"}}`)
	}))
	defer srv.Close()

	c := billingstripe.New("sk_test_x")
	c.SetBaseURLForTesting(srv.URL)

	_, err := billingstripe.CreateSubscription(context.Background(), c, billingstripe.CreateSubscriptionInput{
		StoreID: "s", Plan: "starter", Period: "monthly",
		CustomerID: "cus_x", PriceID: "price_x", TrialEnd: 1_784_140_800,
	})
	require.Error(t, err)
	var apiErr *billingstripe.APIError
	require.True(t, errors.As(err, &apiErr))
	require.Equal(t, "card_declined", apiErr.Code)
	require.Equal(t, 402, apiErr.HTTPStatus)
}
