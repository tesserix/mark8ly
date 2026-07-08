package payment

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shopspring/decimal"
)

func TestStripeRefund_SendsIdempotencyKey(t *testing.T) {
	var gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("Idempotency-Key")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"re_1","status":"succeeded","amount":5000}`))
	}))
	defer srv.Close()

	g := NewStripeGateway("sk_test", "", "test")
	g.baseURL = srv.URL

	_, err := g.RefundPayment(context.Background(), RefundInput{
		ProviderPaymentID: "pi_1",
		Amount:            decimal.NewFromInt(50),
		CurrencyCode:      "usd",
		IdempotencyKey:    "refund:order-123:cancel",
	})
	if err != nil {
		t.Fatalf("refund: %v", err)
	}
	if gotKey != "refund:order-123:cancel" {
		t.Fatalf("Idempotency-Key = %q, want %q", gotKey, "refund:order-123:cancel")
	}
}
