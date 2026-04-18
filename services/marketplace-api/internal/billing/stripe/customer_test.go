package stripe_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	billingstripe "github.com/mark8ly/marketplace-api/internal/billing/stripe"
)

func TestCreateCustomer_SendsMetadataAndIdempotencyKey(t *testing.T) {
	var seenKey string
	var seenForm url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenKey = r.Header.Get("Idempotency-Key")
		b, _ := io.ReadAll(r.Body)
		seenForm, _ = url.ParseQuery(string(b))
		w.Header().Set("Request-Id", "req_test")
		_, _ = w.Write([]byte(`{"id":"cus_x","email":"a@b.com"}`))
	}))
	defer srv.Close()

	c := billingstripe.New("sk_test_x")
	c.SetBaseURLForTesting(srv.URL)

	cu, err := billingstripe.CreateCustomer(context.Background(), c, billingstripe.CreateCustomerInput{
		StoreID: "store-1", TenantID: "tenant-1", Email: "a@b.com", Name: "Acme", Country: "US",
	})
	require.NoError(t, err)
	require.Equal(t, "cus_x", cu.ID)
	require.Equal(t, "customer:store-1", seenKey)
	require.Equal(t, "store-1", seenForm.Get("metadata[store_id]"))
	require.Equal(t, "tenant-1", seenForm.Get("metadata[tenant_id]"))
	require.Equal(t, "US", seenForm.Get("address[country]"))
}
