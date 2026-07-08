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

func TestPayPalVerifyWebhook_SetsProviderPaymentID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/oauth2/token"):
			_, _ = w.Write([]byte(`{"access_token":"t","expires_in":3600}`))
		case strings.Contains(r.URL.Path, "/verify-webhook-signature"):
			_, _ = w.Write([]byte(`{"verification_status":"SUCCESS"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	g := NewPayPalGateway("id", "secret", "test")
	g.baseURL = srv.URL

	payload := []byte(`{"id":"WH-1","event_type":"PAYMENT.CAPTURE.COMPLETED","resource":{"id":"CAP-77","amount":{"currency_code":"USD","value":"50.00"}}}`)
	signature := `{"transmission_id":"tx-1","transmission_time":"2026-07-09T00:00:00Z","cert_url":"https://example.com/cert","auth_algo":"SHA256withRSA","transmission_sig":"sig","webhook_id":"WH-ID-1"}`

	evt, err := g.VerifyWebhook(context.Background(), payload, signature)
	if err != nil {
		t.Fatalf("verify webhook: %v", err)
	}
	if evt.ProviderPaymentID != "CAP-77" {
		t.Fatalf("ProviderPaymentID = %q, want %q", evt.ProviderPaymentID, "CAP-77")
	}
}
