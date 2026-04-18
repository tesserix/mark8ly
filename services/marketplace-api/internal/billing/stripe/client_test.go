package stripe_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	billingstripe "github.com/mark8ly/marketplace-api/internal/billing/stripe"
)

func TestClient_PostForm_AttachesIdempotencyKey(t *testing.T) {
	var gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("Idempotency-Key")
		w.Header().Set("Request-Id", "req_test")
		_, _ = w.Write([]byte(`{"id":"obj_x"}`))
	}))
	defer srv.Close()

	c := billingstripe.New("sk_test_x")
	c.SetBaseURLForTesting(srv.URL)

	_, err := c.PostForm(context.Background(), "/v1/customers", "customer:store-1", url.Values{"name": []string{"Acme"}})
	require.NoError(t, err)
	require.Equal(t, "customer:store-1", gotKey)
}

func TestClient_PostForm_MissingIdempotencyKeyReturnsError(t *testing.T) {
	c := billingstripe.New("sk_test_x")
	_, err := c.PostForm(context.Background(), "/v1/customers", "", url.Values{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "idempotency key required")
}

func TestClient_PostForm_4xxReturnsAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Request-Id", "req_err")
		w.WriteHeader(http.StatusPaymentRequired)
		_, _ = io.WriteString(w, `{"error":{"code":"card_declined","type":"card_error","message":"4242 declined"}}`)
	}))
	defer srv.Close()

	c := billingstripe.New("sk_test_x")
	c.SetBaseURLForTesting(srv.URL)

	_, err := c.PostForm(context.Background(), "/v1/customers", "key-1", url.Values{})
	var apiErr *billingstripe.APIError
	require.True(t, errors.As(err, &apiErr))
	require.Equal(t, "card_declined", apiErr.Code)
	require.Equal(t, "card_error", apiErr.Type)
	require.Equal(t, 402, apiErr.HTTPStatus)
	require.Equal(t, "req_err", apiErr.RequestID)
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
