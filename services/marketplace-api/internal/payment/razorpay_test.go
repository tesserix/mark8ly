package payment

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
)

func TestRazorpayRefund_SendsIdempotencyHeaderAndNotes(t *testing.T) {
	var gotKey string
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("X-Refund-Idempotency")
		body, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"rfnd_1","status":"processed","amount":5000}`))
	}))
	defer srv.Close()

	g := NewRazorpayGateway("key", "secret", "test")
	g.baseURL = srv.URL

	_, err := g.RefundPayment(context.Background(), RefundInput{
		ProviderPaymentID: "pay_1",
		Amount:            decimal.NewFromInt(50),
		CurrencyCode:      "INR",
		Reason:            "cancelled",
		IdempotencyKey:    "refund_ord9_cancel",
	})
	if err != nil {
		t.Fatalf("refund: %v", err)
	}
	if gotKey != "refund_ord9_cancel" {
		t.Fatalf("X-Refund-Idempotency header = %q, want %q", gotKey, "refund_ord9_cancel")
	}
	if !strings.Contains(string(body), `"reason":"cancelled"`) {
		t.Fatalf("reason not in notes: %s", body)
	}
}
