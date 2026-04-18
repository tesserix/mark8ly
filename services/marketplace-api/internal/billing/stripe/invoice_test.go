package stripe_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	billingstripe "github.com/mark8ly/marketplace-api/internal/billing/stripe"
)

func TestGetInvoice_ReturnsHostedURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/invoices/in_x", r.URL.Path)
		_, _ = w.Write([]byte(`{"id":"in_x","status":"paid","hosted_invoice_url":"https://inv.stripe/x","amount_paid":1900}`))
	}))
	defer srv.Close()

	c := billingstripe.New("sk_test_x")
	c.SetBaseURLForTesting(srv.URL)

	inv, err := billingstripe.GetInvoice(context.Background(), c, "in_x")
	require.NoError(t, err)
	require.Equal(t, "paid", inv.Status)
	require.Equal(t, "https://inv.stripe/x", inv.HostedInvoiceURL)
	require.EqualValues(t, 1900, inv.AmountPaid)
}
