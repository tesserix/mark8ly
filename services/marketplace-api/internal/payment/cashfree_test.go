package payment

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

// newTestCashfree builds a gateway pointed at a test server.
func newTestCashfree(t *testing.T, h http.HandlerFunc) (*CashfreeGateway, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	g := NewCashfreeGateway("app_id", "secret", "test")
	g.baseURL = srv.URL
	return g, srv
}

// signCashfree produces the composite signature string the webhook route packs
// from the two Cashfree headers, using a fresh timestamp.
func signCashfree(t *testing.T, secret string, payload []byte) string {
	t.Helper()
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts))
	mac.Write(payload)
	return CashfreeWebhookSignature(ts, base64.StdEncoding.EncodeToString(mac.Sum(nil)))
}

// TestCashfreeAmount_RendersRupeesWithoutFloatDrift is the guard on the single
// most expensive way to get this adapter wrong: Cashfree takes a rupee decimal
// on the wire, and rendering it through a float64 silently corrupts the amount.
func TestCashfreeAmount_RendersRupeesWithoutFloatDrift(t *testing.T) {
	cases := []struct {
		minor int64
		want  string
	}{
		{14025, "140.25"},
		{100, "1.00"},
		{5, "0.05"},
		{0, "0.00"},
		{99999999, "999999.99"},
		{-105, "-1.05"},
	}
	for _, tc := range cases {
		got, err := json.Marshal(cashfreeAmount(tc.minor))
		if err != nil {
			t.Fatalf("marshal %d: %v", tc.minor, err)
		}
		if string(got) != tc.want {
			t.Fatalf("cashfreeAmount(%d) marshalled to %s, want %s", tc.minor, got, tc.want)
		}
	}
}

// TestCashfreeAmount_RoundTripsThroughUnmarshal asserts a value read back from
// Cashfree compares equal to the value we sent — both as a bare number and as
// the stringified form some Cashfree fields use.
func TestCashfreeAmount_RoundTripsThroughUnmarshal(t *testing.T) {
	for _, raw := range []string{`140.25`, `"140.25"`, `140.250`} {
		var a cashfreeAmount
		if err := json.Unmarshal([]byte(raw), &a); err != nil {
			t.Fatalf("unmarshal %s: %v", raw, err)
		}
		if a != 14025 {
			t.Fatalf("unmarshal %s = %d paise, want 14025", raw, a)
		}
		if got := a.Decimal().String(); got != "140.25" {
			t.Fatalf("Decimal() of %s = %s, want 140.25", raw, got)
		}
	}
}

// TestCashfreeCreateIntent_SendsRupeeAmountAndReturnsSession covers the happy
// path: the wire body carries rupees (not paise) and the client token is the
// payment_session_id the storefront SDK needs.
func TestCashfreeCreateIntent_SendsRupeeAmountAndReturnsSession(t *testing.T) {
	var body []byte
	var gotHeaders http.Header
	g, _ := newTestCashfree(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		gotHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"cf_order_id":9911,"order_id":"ord-1","payment_session_id":"session_abc","order_status":"ACTIVE","order_amount":140.25,"order_currency":"INR"}`))
	})

	intent, err := g.CreateIntent(context.Background(), CreateIntentInput{
		OrderID:       "ord-1",
		Amount:        decimal.RequireFromString("140.25"),
		CurrencyCode:  "INR",
		CustomerEmail: "buyer@example.com",
		CustomerPhone: "9876543210",
	})
	if err != nil {
		t.Fatalf("create intent: %v", err)
	}

	if !strings.Contains(string(body), `"order_amount":140.25`) {
		t.Fatalf("body must carry rupees, not paise: %s", body)
	}
	if !strings.Contains(string(body), `"customer_phone":"9876543210"`) {
		t.Fatalf("body missing customer_phone: %s", body)
	}
	if gotHeaders.Get("x-client-id") != "app_id" || gotHeaders.Get("x-client-secret") != "secret" {
		t.Fatalf("auth headers not set: %v", gotHeaders)
	}
	if gotHeaders.Get("x-api-version") != cashfreeAPIVersion {
		t.Fatalf("x-api-version = %q, want %q", gotHeaders.Get("x-api-version"), cashfreeAPIVersion)
	}
	if intent.ClientToken != "session_abc" {
		t.Fatalf("ClientToken = %q, want session_abc", intent.ClientToken)
	}
	// The refund path reconstructs the Cashfree order from this value, so it
	// must be the merchant order_id, never cf_order_id.
	if intent.ProviderIntentID != "ord-1" {
		t.Fatalf("ProviderIntentID = %q, want ord-1 (the merchant order id)", intent.ProviderIntentID)
	}
	if intent.Status != "pending" {
		t.Fatalf("Status = %q, want pending", intent.Status)
	}
}

// TestCashfreeCreateIntent_RequiresPhone asserts the mandatory-field check
// fires before any HTTP call, so a missing phone reads as a named field rather
// than an opaque gateway 400.
func TestCashfreeCreateIntent_RequiresPhone(t *testing.T) {
	g, _ := newTestCashfree(t, func(w http.ResponseWriter, _ *http.Request) {
		t.Error("gateway must not be called when customer_phone is missing")
		w.WriteHeader(http.StatusOK)
	})

	_, err := g.CreateIntent(context.Background(), CreateIntentInput{
		OrderID:      "ord-1",
		Amount:       decimal.NewFromInt(100),
		CurrencyCode: "INR",
	})
	if err == nil || !strings.Contains(err.Error(), "customer_phone") {
		t.Fatalf("err = %v, want a customer_phone validation error", err)
	}
}

// TestCashfreeCreateIntent_RejectsNonINR guards the wire format: Cashfree
// settles only in INR, and silently sending a USD amount through the rupee
// renderer would bill the wrong number.
func TestCashfreeCreateIntent_RejectsNonINR(t *testing.T) {
	g, _ := newTestCashfree(t, func(w http.ResponseWriter, _ *http.Request) {
		t.Error("gateway must not be called for a non-INR currency")
		w.WriteHeader(http.StatusOK)
	})

	_, err := g.CreateIntent(context.Background(), CreateIntentInput{
		OrderID:       "ord-1",
		Amount:        decimal.NewFromInt(100),
		CurrencyCode:  "USD",
		CustomerPhone: "9876543210",
	})
	if err == nil || !strings.Contains(err.Error(), "INR only") {
		t.Fatalf("err = %v, want an INR-only currency error", err)
	}
}

// TestCashfreeCreateIntent_ReusesExistingOrderOn409 is the double-tap case: a
// duplicate order_id must reuse the payable order rather than fail, otherwise a
// retry strands an order the buyer can no longer pay.
func TestCashfreeCreateIntent_ReusesExistingOrderOn409(t *testing.T) {
	var fetched bool
	g, _ := newTestCashfree(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"code":"order_already_exists","message":"order id already exists"}`))
			return
		}
		fetched = true
		if r.URL.Path != "/orders/ord-1" {
			t.Errorf("fetch path = %q, want /orders/ord-1", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"order_id":"ord-1","payment_session_id":"session_existing","order_status":"ACTIVE","order_amount":140.25}`))
	})

	intent, err := g.CreateIntent(context.Background(), CreateIntentInput{
		OrderID:       "ord-1",
		Amount:        decimal.RequireFromString("140.25"),
		CurrencyCode:  "INR",
		CustomerPhone: "9876543210",
	})
	if err != nil {
		t.Fatalf("409 must reuse the existing order, got error: %v", err)
	}
	if !fetched {
		t.Fatal("expected a GET /orders/{id} after the 409")
	}
	if intent.ClientToken != "session_existing" {
		t.Fatalf("ClientToken = %q, want session_existing", intent.ClientToken)
	}
}

// TestCashfreeCreateIntent_RefusesUnpayableExistingOrder: a 409 on an order
// that is already PAID or EXPIRED has no session to hand back, and returning an
// empty token would leave the storefront opening a sheet that cannot work.
func TestCashfreeCreateIntent_RefusesUnpayableExistingOrder(t *testing.T) {
	g, _ := newTestCashfree(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"code":"order_already_exists"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"order_id":"ord-1","payment_session_id":"","order_status":"EXPIRED"}`))
	})

	_, err := g.CreateIntent(context.Background(), CreateIntentInput{
		OrderID:       "ord-1",
		Amount:        decimal.NewFromInt(100),
		CurrencyCode:  "INR",
		CustomerPhone: "9876543210",
	})
	if err == nil || !strings.Contains(err.Error(), "not payable") {
		t.Fatalf("err = %v, want a not-payable error", err)
	}
}

// TestCashfreeRefund_IsOrderScopedWithDerivedRefundID covers the two things the
// refund path must get right: the URL is order-scoped, and refund_id is a
// deterministic, in-window digest of the caller's idempotency key.
func TestCashfreeRefund_IsOrderScopedWithDerivedRefundID(t *testing.T) {
	var gotPath, gotIdemHeader string
	var body []byte
	g, _ := newTestCashfree(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotIdemHeader = r.Header.Get(cashfreeHeaderIdempotency)
		body, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"cf_refund_id":77,"refund_id":"abc","order_id":"ord-9","refund_status":"SUCCESS","refund_amount":50.00}`))
	})

	// The coordinator's real key shape — already longer than Cashfree's 40-char
	// refund_id window, which is why it must be digested rather than sent raw.
	const key = "refund_11111111-1111-1111-1111-111111111111_cancel"

	ref, err := g.RefundPayment(context.Background(), RefundInput{
		ProviderPaymentID: "1234567890", // a cf_payment_id — deliberately NOT what the URL uses
		OrderID:           "ord-9",
		Amount:            decimal.NewFromInt(50),
		CurrencyCode:      "INR",
		Reason:            "cancelled",
		IdempotencyKey:    key,
	})
	if err != nil {
		t.Fatalf("refund: %v", err)
	}

	if gotPath != "/orders/ord-9/refunds" {
		t.Fatalf("refund path = %q, want /orders/ord-9/refunds", gotPath)
	}
	wantRefundID := cashfreeRefundID(key)
	if len(wantRefundID) != 32 {
		t.Fatalf("derived refund_id length = %d, want 32 (Cashfree allows 3–40)", len(wantRefundID))
	}
	if !strings.Contains(string(body), fmt.Sprintf(`"refund_id":"%s"`, wantRefundID)) {
		t.Fatalf("body missing derived refund_id %s: %s", wantRefundID, body)
	}
	if gotIdemHeader != wantRefundID {
		t.Fatalf("idempotency header = %q, want %q", gotIdemHeader, wantRefundID)
	}
	if !strings.Contains(string(body), `"refund_amount":50.00`) {
		t.Fatalf("refund amount must be rupees: %s", body)
	}
	if !strings.Contains(string(body), `"refund_note":"cancelled"`) {
		t.Fatalf("reason not sent as refund_note: %s", body)
	}
	if ref.Status != "succeeded" {
		t.Fatalf("Status = %q, want succeeded", ref.Status)
	}
	// Amount must come back in MAJOR units — the ledger is numeric(12,2).
	if ref.Amount.String() != "50" {
		t.Fatalf("Amount = %s, want 50", ref.Amount.String())
	}
}

// TestCashfreeRefundID_IsDeterministic is the basis of the whole idempotency
// guarantee: the same logical refund must derive the same refund_id on every
// retry, or a retry becomes a second real refund.
func TestCashfreeRefundID_IsDeterministic(t *testing.T) {
	a := cashfreeRefundID("refund_ord9_cancel")
	b := cashfreeRefundID("refund_ord9_cancel")
	c := cashfreeRefundID("refund_ord9_return")
	if a != b {
		t.Fatalf("same key derived different refund ids: %s vs %s", a, b)
	}
	if a == c {
		t.Fatal("different keys must derive different refund ids")
	}
	for _, r := range a {
		if !strings.ContainsRune("0123456789abcdef", r) {
			t.Fatalf("refund_id %q contains a character outside Cashfree's charset", a)
		}
	}
}

// TestCashfreeRefund_RequiresOrderID: Cashfree has no payment-scoped refund
// route, so a missing OrderID must fail loudly rather than build a broken URL.
func TestCashfreeRefund_RequiresOrderID(t *testing.T) {
	g, _ := newTestCashfree(t, func(w http.ResponseWriter, _ *http.Request) {
		t.Error("gateway must not be called without an order id")
		w.WriteHeader(http.StatusOK)
	})

	_, err := g.RefundPayment(context.Background(), RefundInput{
		ProviderPaymentID: "1234567890",
		Amount:            decimal.NewFromInt(50),
		CurrencyCode:      "INR",
		IdempotencyKey:    "refund_ord9_cancel",
	})
	if err == nil || !strings.Contains(err.Error(), "OrderID is required") {
		t.Fatalf("err = %v, want an OrderID-required error", err)
	}
}

// TestCashfreeRefund_DuplicateIsReadBackAsSuccess: a 409/422 replay means the
// refund already exists. Reporting it as a failure would send the saga round
// again — which is how a customer receives two refunds.
func TestCashfreeRefund_DuplicateIsReadBackAsSuccess(t *testing.T) {
	for _, status := range []int{http.StatusConflict, http.StatusUnprocessableEntity} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			g, _ := newTestCashfree(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					w.WriteHeader(status)
					_, _ = w.Write([]byte(`{"code":"refund_already_exists","message":"duplicate refund_id"}`))
					return
				}
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"refund_id":"existing","order_id":"ord-9","refund_status":"SUCCESS","refund_amount":50.00}`))
			})

			ref, err := g.RefundPayment(context.Background(), RefundInput{
				OrderID:        "ord-9",
				Amount:         decimal.NewFromInt(50),
				CurrencyCode:   "INR",
				IdempotencyKey: "refund_ord9_cancel",
			})
			if err != nil {
				t.Fatalf("duplicate refund must read back as success, got: %v", err)
			}
			if ref.ProviderRefundID != "existing" {
				t.Fatalf("ProviderRefundID = %q, want existing", ref.ProviderRefundID)
			}
		})
	}
}

// TestCashfreeRefund_4xxIsPermanentGatewayError asserts the refund saga can
// classify a hopeless refund and stop re-driving it, and that the raw body
// (which can echo submitted customer data) never leaks into the error.
func TestCashfreeRefund_4xxIsPermanentGatewayError(t *testing.T) {
	g, _ := newTestCashfree(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"code":"refund_amount_exceeded","message":"exceeds capturable","customer_phone":"9876543210"}`))
	})

	_, err := g.RefundPayment(context.Background(), RefundInput{
		OrderID:        "ord-9",
		Amount:         decimal.NewFromInt(50),
		CurrencyCode:   "INR",
		IdempotencyKey: "refund_ord9_cancel",
	})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !IsPermanentGatewayError(err) {
		t.Fatalf("400 must be classified permanent, got %v", err)
	}
	if strings.Contains(err.Error(), "9876543210") {
		t.Fatalf("error must not carry echoed customer data: %v", err)
	}
}

// TestCashfreeRefundStatus_OnHoldIsPending: ONHOLD money is still coming.
// Calling it failed would make the sweeper re-issue a refund already in flight.
func TestCashfreeRefundStatus_OnHoldIsPending(t *testing.T) {
	cases := map[string]string{
		"SUCCESS":   "succeeded",
		"PENDING":   "pending",
		"ONHOLD":    "pending",
		"FAILED":    "failed",
		"CANCELLED": "failed",
	}
	for in, want := range cases {
		if got := normalizeCashfreeRefundStatus(in); got != want {
			t.Fatalf("normalizeCashfreeRefundStatus(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestCashfreeVerifyWebhook_SignsTimestampAndBody is the signature-scheme
// guard: Cashfree signs base64(HMAC(timestamp+body)), so a hex digest over the
// body alone (the Razorpay scheme) must NOT verify.
func TestCashfreeVerifyWebhook_SignsTimestampAndBody(t *testing.T) {
	const secret = "secret"
	payload := []byte(`{"type":"PAYMENT_SUCCESS_WEBHOOK","data":{"order":{"order_id":"11111111-1111-1111-1111-111111111111","order_amount":140.25,"order_currency":"INR"},"payment":{"cf_payment_id":5566778899,"payment_status":"SUCCESS","payment_amount":140.25,"payment_currency":"INR","payment_group":"upi"}}}`)

	g := NewCashfreeGateway("app_id", secret, "test")

	evt, err := g.VerifyWebhook(context.Background(), payload, signCashfree(t, secret, payload))
	if err != nil {
		t.Fatalf("verify webhook: %v", err)
	}
	if evt.EventType != "payment.succeeded" {
		t.Fatalf("EventType = %q, want payment.succeeded", evt.EventType)
	}
	if evt.OrderID != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("OrderID = %q", evt.OrderID)
	}
	if evt.ProviderPaymentID != "5566778899" {
		t.Fatalf("ProviderPaymentID = %q, want 5566778899", evt.ProviderPaymentID)
	}
	if evt.PaymentMethod != "upi" {
		t.Fatalf("PaymentMethod = %q, want upi", evt.PaymentMethod)
	}
	if evt.Amount.String() != "140.25" {
		t.Fatalf("Amount = %s, want 140.25", evt.Amount.String())
	}
}

// TestCashfreeVerifyWebhook_RejectsBadInput covers every way verification must
// fail closed. A forged or malformed delivery can never produce an event.
func TestCashfreeVerifyWebhook_RejectsBadInput(t *testing.T) {
	const secret = "secret"
	payload := []byte(`{"type":"PAYMENT_SUCCESS_WEBHOOK","data":{"payment":{"cf_payment_id":1}}}`)
	g := NewCashfreeGateway("app_id", secret, "test")

	staleTS := strconv.FormatInt(time.Now().Add(-2*cashfreeWebhookMaxSkew).Unix(), 10)
	staleMac := hmac.New(sha256.New, []byte(secret))
	staleMac.Write([]byte(staleTS))
	staleMac.Write(payload)
	stale := CashfreeWebhookSignature(staleTS, base64.StdEncoding.EncodeToString(staleMac.Sum(nil)))

	cases := map[string]string{
		"no separator":        "justasignature",
		"empty timestamp":     ".sig",
		"empty signature":     "1700000000.",
		"unparseable stamp":   CashfreeWebhookSignature("not-a-number", "sig"),
		"wrong signature":     CashfreeWebhookSignature(strconv.FormatInt(time.Now().Unix(), 10), "d3Jvbmc="),
		"stale but authentic": stale,
	}
	for name, sig := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := g.VerifyWebhook(context.Background(), payload, sig); err == nil {
				t.Fatal("expected verification to fail closed")
			}
		})
	}
}

// TestCashfreeVerifyWebhook_UserDroppedIsAFailure: an abandoned sheet leaves
// the order reserved, the same handling a hard decline gets.
func TestCashfreeVerifyWebhook_UserDroppedIsAFailure(t *testing.T) {
	const secret = "secret"
	payload := []byte(`{"type":"PAYMENT_USER_DROPPED_WEBHOOK","data":{"order":{"order_id":"ord-1"},"payment":{"cf_payment_id":42,"payment_status":"USER_DROPPED"}}}`)
	g := NewCashfreeGateway("app_id", secret, "test")

	evt, err := g.VerifyWebhook(context.Background(), payload, signCashfree(t, secret, payload))
	if err != nil {
		t.Fatalf("verify webhook: %v", err)
	}
	if evt.EventType != "payment.failed" {
		t.Fatalf("EventType = %q, want payment.failed", evt.EventType)
	}
}

// TestCashfreeVerifyWebhook_RefundEventReadsRefundBlock: refund events carry
// their ids under data.refund, not data.payment.
func TestCashfreeVerifyWebhook_RefundEventReadsRefundBlock(t *testing.T) {
	const secret = "secret"
	payload := []byte(`{"type":"REFUND_STATUS_WEBHOOK","data":{"refund":{"cf_refund_id":11,"cf_payment_id":22,"refund_id":"rf_abc","order_id":"ord-7","refund_status":"SUCCESS","refund_amount":25.50}}}`)
	g := NewCashfreeGateway("app_id", secret, "test")

	evt, err := g.VerifyWebhook(context.Background(), payload, signCashfree(t, secret, payload))
	if err != nil {
		t.Fatalf("verify webhook: %v", err)
	}
	if evt.EventType != "refund.succeeded" {
		t.Fatalf("EventType = %q, want refund.succeeded", evt.EventType)
	}
	if evt.OrderID != "ord-7" {
		t.Fatalf("OrderID = %q, want ord-7", evt.OrderID)
	}
	if evt.ProviderEventID != "cfrefund_rf_abc" {
		t.Fatalf("ProviderEventID = %q, want cfrefund_rf_abc", evt.ProviderEventID)
	}
	if evt.Amount.String() != "25.5" {
		t.Fatalf("Amount = %s, want 25.5", evt.Amount.String())
	}
}

// TestCashfreeFetchOrderPayment_SkipsFailedAttempts is the confirm path's core
// guarantee: an order with a failed attempt ahead of the successful one must
// still report the payment that actually took the money — and must never just
// return payments[0].
func TestCashfreeFetchOrderPayment_SkipsFailedAttempts(t *testing.T) {
	g, _ := newTestCashfree(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/orders/ord-1/payments" {
			t.Errorf("path = %q, want /orders/ord-1/payments", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[
			{"cf_payment_id":1,"payment_status":"FAILED","payment_amount":140.25,"payment_currency":"INR","payment_group":"credit_card"},
			{"cf_payment_id":2,"payment_status":"SUCCESS","payment_amount":140.25,"payment_currency":"INR","payment_group":"net_banking"}
		]`))
	})

	got, err := g.FetchOrderPayment(context.Background(), "ord-1")
	if err != nil {
		t.Fatalf("fetch order payment: %v", err)
	}
	if got == nil {
		t.Fatal("expected the captured payment, got nil")
	}
	if got.ProviderPaymentID != "2" {
		t.Fatalf("ProviderPaymentID = %q, want 2 (the SUCCESS attempt)", got.ProviderPaymentID)
	}
	if got.Status != "payment.succeeded" {
		t.Fatalf("Status = %q, want payment.succeeded", got.Status)
	}
	if got.PaymentMethod != "netbanking" {
		t.Fatalf("PaymentMethod = %q, want netbanking", got.PaymentMethod)
	}
	if got.Amount.String() != "140.25" {
		t.Fatalf("Amount = %s, want 140.25", got.Amount.String())
	}
}

// TestCashfreeFetchOrderPayment_NoCaptureIsNotAnError: "the buyer has not paid
// yet" is a normal poll outcome, and the confirm endpoint renders it as
// pending rather than as a failure.
func TestCashfreeFetchOrderPayment_NoCaptureIsNotAnError(t *testing.T) {
	cases := map[string]http.HandlerFunc{
		"only failed attempts": func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[{"cf_payment_id":1,"payment_status":"USER_DROPPED"}]`))
		},
		"empty list": func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[]`))
		},
		"404 no attempts yet": func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"code":"order_not_found"}`))
		},
	}
	for name, h := range cases {
		t.Run(name, func(t *testing.T) {
			g, _ := newTestCashfree(t, h)
			got, err := g.FetchOrderPayment(context.Background(), "ord-1")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != nil {
				t.Fatalf("expected nil (not paid), got %+v", got)
			}
		})
	}
}

// TestCashfreeMethodLabel_MapsToSharedVocabulary keeps an admin screen reading
// the same regardless of which gateway took the payment.
func TestCashfreeMethodLabel_MapsToSharedVocabulary(t *testing.T) {
	cases := map[string]string{
		"upi":           "upi",
		"credit_card":   "card",
		"debit_card":    "card",
		"cardless_emi":  "card",
		"net_banking":   "netbanking",
		"wallet":        "wallet",
		"pay_later":     "paylater",
		"SOMETHING_NEW": "something_new", // passes through, not bucketed
	}
	for group, want := range cases {
		p := &cashfreePayment{PaymentGroup: group}
		if got := p.methodLabel(); got != want {
			t.Fatalf("methodLabel(%q) = %q, want %q", group, got, want)
		}
	}
}

// TestCashfreeGateway_ResolvesEnvironmentByHost: mode selects the HOST, so a
// test-mode gateway physically cannot reach production.
func TestCashfreeGateway_ResolvesEnvironmentByHost(t *testing.T) {
	if got := NewCashfreeGateway("a", "b", "test").baseURL; got != cashfreeTestBaseURL {
		t.Fatalf("test mode baseURL = %q, want %q", got, cashfreeTestBaseURL)
	}
	for _, mode := range []string{"live", ""} {
		if got := NewCashfreeGateway("a", "b", mode).baseURL; got != cashfreeLiveBaseURL {
			t.Fatalf("mode %q baseURL = %q, want %q", mode, got, cashfreeLiveBaseURL)
		}
	}
}

// TestNewGateway_ResolvesCashfree asserts the adapter is reachable through the
// factory every money path goes through, and satisfies both interfaces.
func TestNewGateway_ResolvesCashfree(t *testing.T) {
	gw, err := NewGateway("cashfree", "app_id", "secret", "test")
	if err != nil {
		t.Fatalf("NewGateway(cashfree): %v", err)
	}
	if gw.ProviderName() != "cashfree" {
		t.Fatalf("ProviderName = %q, want cashfree", gw.ProviderName())
	}
	if _, ok := gw.(OrderStatusGateway); !ok {
		t.Fatal("Cashfree must implement OrderStatusGateway — the confirm endpoint depends on it")
	}
	// Cashfree is NOT a hosted-checkout provider: the storefront opens its SDK
	// in-page. Implementing CheckoutGateway would silently reroute checkout
	// down the redirect branch in createPaymentIntent.
	if _, ok := gw.(CheckoutGateway); ok {
		t.Fatal("Cashfree must not implement CheckoutGateway — checkout would take the hosted redirect path")
	}
}
