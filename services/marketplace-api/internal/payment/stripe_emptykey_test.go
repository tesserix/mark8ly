package payment

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
)

// An unconfigured API key must fail LOCALLY, before any request leaves the
// process (#169).
//
// Without this, an empty key is sent as empty basic auth and Stripe answers
// 401 — so a missing credential is indistinguishable from a revoked or wrong
// one. During a refund incident that points the operator at Stripe's
// dashboard instead of at their own configuration, which is the expensive
// kind of misdirection.
func TestStripeRefund_EmptyAPIKeyFailsBeforeSendingRequest(t *testing.T) {
	var called bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	g := NewStripeGateway("", "secret", "test")
	g.baseURL = srv.URL

	_, err := g.RefundPayment(context.Background(), RefundInput{
		ProviderPaymentID: "pi_1",
		Amount:            decimal.RequireFromString("10.00"),
		CurrencyCode:      "USD",
	})
	if err == nil {
		t.Fatal("RefundPayment with an empty API key returned nil error")
	}
	if called {
		t.Error("a request reached Stripe despite there being no API key; the guard must short-circuit first")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "api key") {
		t.Errorf("error = %q, want it to name the missing API key so the operator "+
			"looks at configuration rather than at Stripe", err)
	}
}
