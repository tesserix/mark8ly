package stripe_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	billingstripe "github.com/mark8ly/marketplace-api/internal/billing/stripe"
)

// TestClient_PostForm_AttachesIdempotencyKey verifies that CreateCustomer (a
// write helper) attaches the deterministic idempotency key as a header.
func TestClient_PostForm_AttachesIdempotencyKey(t *testing.T) {
	var gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("Idempotency-Key")
		w.Header().Set("Request-Id", "req_test")
		_, _ = w.Write([]byte(`{"id":"cus_x","email":""}`))
	}))
	defer srv.Close()

	c := billingstripe.New("sk_test_x")
	c.SetBaseURLForTesting(srv.URL)

	_, err := billingstripe.CreateCustomer(context.Background(), c, billingstripe.CreateCustomerInput{
		StoreID: "store-1", TenantID: "tenant-1",
	})
	require.NoError(t, err)
	require.Equal(t, "customer:store-1", gotKey)
}

// TestClient_PostForm_4xxReturnsAPIError verifies that a 4xx response from the
// Stripe API is translated into our *APIError type by a write helper.
func TestClient_PostForm_4xxReturnsAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Request-Id", "req_err")
		w.WriteHeader(http.StatusPaymentRequired)
		_, _ = io.WriteString(w, `{"error":{"code":"card_declined","type":"card_error","message":"4242 declined"}}`)
	}))
	defer srv.Close()

	c := billingstripe.New("sk_test_x")
	c.SetBaseURLForTesting(srv.URL)

	_, err := billingstripe.CreateCustomer(context.Background(), c, billingstripe.CreateCustomerInput{
		StoreID: "store-1", TenantID: "tenant-1",
	})
	var apiErr *billingstripe.APIError
	require.True(t, errors.As(err, &apiErr))
	require.Equal(t, "card_declined", apiErr.Code)
	require.Equal(t, "card_error", apiErr.Type)
	require.Equal(t, 402, apiErr.HTTPStatus)
}

func TestIdempotencyKey_PortalBucket5Min(t *testing.T) {
	// 1_711_999_900 and 1_711_999_950 both fall in the same 300-second bucket.
	a := billingstripe.PortalIdempotencyKey("store-1", 1_711_999_900)
	b := billingstripe.PortalIdempotencyKey("store-1", 1_711_999_950)
	require.Equal(t, a, b)

	// 1_712_000_100 is in the next bucket.
	c := billingstripe.PortalIdempotencyKey("store-1", 1_712_000_100)
	require.NotEqual(t, a, c)
}

func TestIdempotencyKey_CheckoutDayBucket(t *testing.T) {
	// 1_712_000_000 and 1_712_010_000 both fall in the same 86400-second bucket.
	a := billingstripe.CheckoutIdempotencyKey("store-1", "pro", "annual", 1_712_000_000)
	b := billingstripe.CheckoutIdempotencyKey("store-1", "pro", "annual", 1_712_010_000)
	require.Equal(t, a, b)

	// 1_712_100_000 crosses into the next day bucket.
	c := billingstripe.CheckoutIdempotencyKey("store-1", "pro", "annual", 1_712_100_000)
	require.NotEqual(t, a, c)
}

func TestIdempotencyKey_CustomerDeterministic(t *testing.T) {
	require.Equal(t, "customer:store-1", billingstripe.CustomerIdempotencyKey("store-1"))
}
