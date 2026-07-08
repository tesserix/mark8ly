package payment

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
)

func TestPayPalRefund_SendsRequestIdHeader(t *testing.T) {
	var gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/oauth2/token") {
			_, _ = w.Write([]byte(`{"access_token":"t","expires_in":3600}`))
			return
		}
		gotKey = r.Header.Get("PayPal-Request-Id")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"RF-1","status":"COMPLETED","amount":{"value":"50.00"}}`))
	}))
	defer srv.Close()

	g := NewPayPalGateway("id", "secret", "test")
	g.baseURL = srv.URL

	_, err := g.RefundPayment(context.Background(), RefundInput{
		ProviderPaymentID: "CAP-1",
		Amount:            decimal.NewFromInt(50),
		CurrencyCode:      "USD",
		IdempotencyKey:    "refund_ord7_ret-2",
	})
	if err != nil {
		t.Fatalf("refund: %v", err)
	}
	if gotKey != "refund_ord7_ret-2" {
		t.Fatalf("PayPal-Request-Id header = %q, want %q", gotKey, "refund_ord7_ret-2")
	}
}
