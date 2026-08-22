package stripe_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	billingstripe "github.com/mark8ly/marketplace-api/internal/billing/stripe"
)

func TestCreateCheckoutSession_PassesCurrencyAndIdempotency(t *testing.T) {
	seen := url.Values{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		v, _ := url.ParseQuery(string(b))
		for k, vs := range v {
			seen[k] = vs
		}
		_, _ = w.Write([]byte(`{"id":"cs_x","url":"https://checkout.stripe.com/cs_x"}`))
	}))
	defer srv.Close()

	c := billingstripe.New("sk_test_x")
	c.SetBaseURLForTesting(srv.URL)

	sess, err := billingstripe.CreateCheckoutSession(context.Background(), c, billingstripe.CheckoutInput{
		StoreID: "store-1", TenantID: "tenant-1", CustomerID: "cus_1",
		PriceID:  "price_starter_monthly",
		Currency: "AUD", Plan: "starter", Period: "monthly",
		SuccessURL: "https://admin.example/success",
		CancelURL:  "https://admin.example/cancel",
		Now:        time.Unix(1_712_000_000, 0),
	})
	require.NoError(t, err)
	require.Equal(t, "cs_x", sess.ID)
	require.Equal(t, "aud", seen.Get("currency"))
	require.Equal(t, "subscription", seen.Get("mode"))
	require.Equal(t, "price_starter_monthly", seen.Get("line_items[0][price]"))
	require.Equal(t, "1", seen.Get("line_items[0][quantity]"))
	require.Contains(t, sess.IdempotencyKey, "checkout:store-1:starter:monthly:")
	require.Equal(t, "store-1", seen.Get("subscription_data[metadata][store_id]"))
}
