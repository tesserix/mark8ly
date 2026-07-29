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

// TestFromMinorUnits_CurrencyAware is the inverse of toMinorUnits: webhook
// payloads carry minor units and the domain works in decimal major units.
// Getting this wrong by a factor of 100 in the gift-card refund path would
// claw back 100x (or 1/100th of) the refunded value.
func TestFromMinorUnits_CurrencyAware(t *testing.T) {
	cases := []struct {
		name     string
		minor    int64
		currency string
		want     string
	}{
		{"usd two decimals", 1000, "USD", "10.00"},
		{"usd lowercase", 1000, "usd", "10.00"},
		{"usd sub-dollar", 7, "USD", "0.07"},
		{"eur two decimals", 12055, "EUR", "120.55"},
		{"jpy zero decimals", 1000, "JPY", "1000"},
		{"krw zero decimals", 50000, "KRW", "50000"},
		{"kwd three decimals", 10500, "KWD", "10.500"},
		{"bhd three decimals", 1234, "BHD", "1.234"},
		{"unknown currency defaults to 2dp", 500, "ZZZ", "5.00"},
		{"empty currency defaults to 2dp", 500, "", "5.00"},
		{"zero", 0, "USD", "0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := fromMinorUnits(tc.minor, tc.currency)
			want := decimal.RequireFromString(tc.want)
			if !got.Equal(want) {
				t.Fatalf("fromMinorUnits(%d, %q) = %s, want %s", tc.minor, tc.currency, got, want)
			}
		})
	}
}

// Round-tripping through both conversions must be lossless for every
// currency exponent — a drift here silently moves money.
func TestMinorUnits_RoundTrip(t *testing.T) {
	for _, tc := range []struct {
		amount   string
		currency string
	}{
		{"10.00", "USD"}, {"0.01", "USD"}, {"120.55", "EUR"},
		{"1000", "JPY"}, {"10.500", "KWD"}, {"5.00", "ZZZ"},
	} {
		amt := decimal.RequireFromString(tc.amount)
		back := fromMinorUnits(toMinorUnits(amt, tc.currency), tc.currency)
		if !back.Equal(amt) {
			t.Fatalf("round trip %s %s = %s", tc.amount, tc.currency, back)
		}
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
