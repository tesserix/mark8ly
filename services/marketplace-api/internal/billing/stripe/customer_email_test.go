package stripe_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	billingstripe "github.com/mark8ly/marketplace-api/internal/billing/stripe"
)

func TestGetCustomerEmail_EmptyCustomerID(t *testing.T) {
	got, err := billingstripe.GetCustomerEmail(context.Background(), nil, "")
	if err != nil {
		t.Fatalf("err = %v, want nil for an empty customer id", err)
	}
	if got != "" {
		t.Errorf("email = %q, want empty", got)
	}
}

func TestGetCustomerEmail_ReturnsEmailFromStripe(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Request-Id", "req_test")
		_, _ = w.Write([]byte(`{"id":"cus_x","email":"merchant@example.com"}`))
	}))
	defer srv.Close()

	c := billingstripe.New("sk_test_x")
	c.SetBaseURLForTesting(srv.URL)

	got, err := billingstripe.GetCustomerEmail(context.Background(), c, "cus_x")
	require.NoError(t, err)
	require.Equal(t, "merchant@example.com", got)
}

func TestGetCustomerEmail_NoEmailOnCustomer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Request-Id", "req_test")
		_, _ = w.Write([]byte(`{"id":"cus_x"}`))
	}))
	defer srv.Close()

	c := billingstripe.New("sk_test_x")
	c.SetBaseURLForTesting(srv.URL)

	got, err := billingstripe.GetCustomerEmail(context.Background(), c, "cus_x")
	require.NoError(t, err)
	require.Equal(t, "", got)
}
