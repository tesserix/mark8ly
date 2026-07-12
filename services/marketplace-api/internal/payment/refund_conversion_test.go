package payment

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shopspring/decimal"
)

func TestToMinorUnits_CurrencyAware(t *testing.T) {
	cases := []struct {
		name     string
		amount   string
		currency string
		want     int64
	}{
		{"usd two decimals", "10.00", "USD", 1000},
		{"usd lowercase", "10.00", "usd", 1000},
		{"eur two decimals", "120.55", "EUR", 12055},
		{"jpy zero decimals", "1000", "JPY", 1000},
		{"krw zero decimals", "50000", "KRW", 50000},
		{"kwd three decimals", "10.500", "KWD", 10500},
		{"bhd three decimals", "1.234", "BHD", 1234},
		{"unknown currency defaults to 2dp", "5.00", "ZZZ", 500},
		{"empty currency defaults to 2dp", "5.00", "", 500},
		// Rounds to nearest minor unit rather than truncating toward zero,
		// so sub-unit residue from upstream split math is not silently lost.
		{"rounds half away from zero", "12.345", "USD", 1235},
		{"rounds down", "12.344", "USD", 1234},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := toMinorUnits(decimal.RequireFromString(tc.amount), tc.currency)
			if got != tc.want {
				t.Fatalf("toMinorUnits(%s, %q) = %d, want %d", tc.amount, tc.currency, got, tc.want)
			}
		})
	}
}

func TestStripeRefundReason_MapsToEnum(t *testing.T) {
	cases := map[string]string{
		"":                      "requested_by_customer",
		"customer changed mind": "requested_by_customer",
		"order cancelled":       "requested_by_customer",
		"duplicate":             "duplicate",
		"Duplicate":             "duplicate",
		"  fraudulent  ":        "fraudulent",
		"FRAUDULENT":            "fraudulent",
	}
	for raw, want := range cases {
		if got := stripeRefundReason(raw); got != want {
			t.Fatalf("stripeRefundReason(%q) = %q, want %q", raw, got, want)
		}
	}
}

// TestStripeRefund_SendsValidEnumReasonAndCurrency asserts the wire request
// never carries raw admin text as Stripe's `reason` (which Stripe would 400),
// and that the amount honours the currency exponent (JPY is zero-decimal).
func TestStripeRefund_SendsValidEnumReasonAndCurrency(t *testing.T) {
	var gotReason, gotAmount string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotReason = r.PostFormValue("reason")
		gotAmount = r.PostFormValue("amount")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"re_1","status":"succeeded","amount":1000}`))
	}))
	defer srv.Close()

	g := NewStripeGateway("sk_test", "", "test")
	g.baseURL = srv.URL

	_, err := g.RefundPayment(context.Background(), RefundInput{
		ProviderPaymentID: "pi_1",
		Amount:            decimal.NewFromInt(1000), // ¥1000
		CurrencyCode:      "JPY",
		Reason:            "order cancelled", // raw free text, NOT a Stripe enum
		IdempotencyKey:    "refund_order-123_cancel",
	})
	if err != nil {
		t.Fatalf("refund: %v", err)
	}
	if gotReason != "requested_by_customer" {
		t.Fatalf("stripe reason = %q, want requested_by_customer", gotReason)
	}
	if gotAmount != "1000" {
		t.Fatalf("stripe amount = %q, want 1000 (JPY zero-decimal)", gotAmount)
	}
}
