package payment

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
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
		var readErr error
		// Swallowing this made a read failure look like an assertion failure
		// on the body below (#169). t.Errorf is safe from a handler
		// goroutine; t.Fatalf is not.
		if body, readErr = io.ReadAll(r.Body); readErr != nil {
			t.Errorf("read request body: %v", readErr)
		}
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

func TestRazorpayVerifyWebhook_SetsProviderPaymentID(t *testing.T) {
	secret := "secret"
	payload := []byte(`{"event":"payment.captured","payload":{"payment":{"entity":{"id":"pay_123","order_id":"order_9","amount":5000,"currency":"INR","method":"upi","notes":{"order_id":"11111111-1111-1111-1111-111111111111"}}}}}`)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	signature := hex.EncodeToString(mac.Sum(nil))

	g := NewRazorpayGateway("key", secret, "test")

	evt, err := g.VerifyWebhook(context.Background(), payload, signature)
	if err != nil {
		t.Fatalf("verify webhook: %v", err)
	}
	if evt.ProviderPaymentID != "pay_123" {
		t.Fatalf("ProviderPaymentID = %q, want %q", evt.ProviderPaymentID, "pay_123")
	}
}
