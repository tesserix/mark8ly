package stripe_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	billingstripe "github.com/mark8ly/marketplace-api/internal/billing/stripe"
)

func TestGetSubscription_UnmarshalsNestedPlanPeriod(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/subscriptions/sub_x", r.URL.Path)
		_, _ = w.Write([]byte(`{
            "id":"sub_x","status":"active","currency":"gbp",
            "current_period_start":1710000000,"current_period_end":1712678400,
            "cancel_at_period_end":false,
            "items":{"data":[{"price":{"id":"price_starter_monthly","currency":"gbp","metadata":{"plan":"starter","period":"monthly"}}}]}
        }`))
	}))
	defer srv.Close()

	c := billingstripe.New("sk_test_x")
	c.SetBaseURLForTesting(srv.URL)

	s, err := billingstripe.GetSubscription(context.Background(), c, "sub_x")
	require.NoError(t, err)
	require.Equal(t, "active", s.Status)
	require.Equal(t, "gbp", s.Currency)
	require.Equal(t, "starter", s.Items.Data[0].Price.Metadata["plan"])
}
