package trial_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	billingstripe "github.com/mark8ly/marketplace-api/internal/billing/stripe"
	"github.com/mark8ly/marketplace-api/internal/billing/trial"
)

// TestStripeAdapter_CreateSubscription_DelegatesToClient verifies that
// StripeAdapter.CreateSubscription forwards the call to the underlying
// billingstripe.Client (and therefore reaches the HTTP layer).
func TestStripeAdapter_CreateSubscription_DelegatesToClient(t *testing.T) {
	reached := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/subscriptions" && r.Method == http.MethodPost {
			reached = true
		}
		w.Header().Set("Request-Id", "req_test")
		_, _ = io.WriteString(w, `{"id":"sub_adapter_test","status":"trialing","customer":"cus_x"}`)
	}))
	defer srv.Close()

	c := billingstripe.New("sk_test_adapter")
	c.SetBaseURLForTesting(srv.URL)
	adapter := &trial.StripeAdapter{C: c}

	_, err := adapter.CreateSubscription(context.Background(), billingstripe.CreateSubscriptionInput{
		StoreID:    "store_1",
		Plan:       "starter",
		Period:     "monthly",
		CustomerID: "cus_x",
		PriceID:    "price_x",
		TrialEnd:   1_800_000_000,
	})
	require.NoError(t, err)
	require.True(t, reached, "StripeAdapter must delegate CreateSubscription to the underlying HTTP client")
}
