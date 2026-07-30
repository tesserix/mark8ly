package payment

// cashfree.go — the Cashfree Payment Gateway adapter.
//
// Deliberately shaped as a near-mirror of razorpay.go: same constructor
// signature, same per-mode base URL seam, same GatewayError on refund failure,
// same normalized WebhookEvent out the other side. The symmetry is the point —
// every money path in this service resolves a provider name to a Gateway and
// otherwise reads identically, so a reviewer comparing the two files sees the
// gateway differences and nothing else.
//
// This is a port of the battle-tested adapter in Home-Chef-App
// (apps/api/services/cashfree.go). Credentials differ in origin — HomeChef
// reads platform-wide slots from Secret Manager, mark8ly resolves per-store
// rows from payment_gateway_configs via carriersecrets — but the wire
// behaviour is intentionally identical, including the 409 handling and the
// timestamp-bound webhook verification.
//
// Five things genuinely differ from Razorpay, and each one is a place a
// careless port would lose money:
//
//  1. AMOUNTS ARE RUPEES ON THE WIRE. Razorpay speaks integer paise; Cashfree
//     speaks a decimal number of rupees ("order_amount": 140.25). Every amount
//     crossing this boundary goes through cashfreeAmount, which holds minor
//     units internally and renders the decimal by integer arithmetic — never
//     by formatting a float64, which is how 140.25 becomes 140.25000000000001.
//
//  2. ENVIRONMENT IS THE HOSTNAME, not just the key. Razorpay serves live and
//     test from one host and tells them apart by key prefix; Cashfree has
//     sandbox.cashfree.com vs api.cashfree.com. That is strictly safer — a
//     test-mode client physically cannot reach production — and it means the
//     mode must be baked into the client at construction.
//
//  3. THERE IS NO CLIENT-SIDE PAYMENT SIGNATURE. Razorpay Checkout hands the
//     browser an HMAC of order_id|payment_id that the server re-computes. The
//     Cashfree SDK returns no such thing; the authority is a server-side
//     GET /orders/{id}/payments. Hence OrderStatusGateway — the confirm path
//     MUST fetch, and must not be written to "tolerate a missing signature"
//     the way the Razorpay path does.
//
//  4. REFUNDS ARE ORDER-SCOPED. Cashfree has no payment-level refund
//     endpoint, so RefundPayment keys off RefundInput.OrderID (which
//     CreateIntent submitted as the Cashfree order_id) rather than
//     ProviderPaymentID. Refund idempotency is first-class: a
//     merchant-supplied refund_id makes a repeat the SAME refund.
//
//  5. WEBHOOKS SIGN TIMESTAMP+BODY, BASE64. Razorpay signs the body alone and
//     hex-encodes. Cashfree prefixes the delivery timestamp and base64s, so
//     the timestamp has to reach VerifyWebhook — see the composite signature
//     contract on parseCashfreeSignature.

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

const (
	// Environment is selected by HOST, so these are not interchangeable with a
	// key swap — see note 2 above. The /pg suffix is part of the base: every
	// Payment Gateway route hangs off it.
	cashfreeLiveBaseURL = "https://api.cashfree.com/pg"
	cashfreeTestBaseURL = "https://sandbox.cashfree.com/pg"

	// cashfreeAPIVersion pins the response schema. Cashfree versions its API
	// by date header and keeps old versions working, so this is a deliberate
	// pin rather than "latest": the order/payment/refund/webhook shapes this
	// file parses are the 2023-08-01 ones. Bumping it is a reviewed change,
	// not a drift.
	cashfreeAPIVersion = "2023-08-01"

	cashfreeRequestTimeout = 30 * time.Second

	// cashfreeWebhookMaxSkew bounds how old a webhook's timestamp may be.
	//
	// The signature covers timestamp+body, so an attacker who captures one
	// delivery can replay it verbatim forever and it will keep verifying. The
	// webhook_events unique index on (provider, provider_event_id) is the
	// primary defence; this adds a time bound on top, because the timestamp is
	// right there in the signed payload and not checking it throws away the
	// one anti-replay signal the scheme hands us. Generous enough to absorb
	// Cashfree's own retry schedule and any clock drift.
	cashfreeWebhookMaxSkew = 30 * time.Minute
)

// CashfreeGateway implements Gateway (and OrderStatusGateway) for Cashfree
// Payment Gateway using the REST API.
type CashfreeGateway struct {
	appID     string // x-client-id — the public app id, Cashfree's analogue of a Razorpay key_id
	secretKey string // x-client-secret (also the webhook signing secret unless a dedicated one is configured)
	mode      string
	baseURL   string
	client    *http.Client
}

// NewCashfreeGateway returns a Cashfree Gateway ready for use. apiKey is the
// app id and secretKey the secret key, matching the (api_key, secret_key)
// column pair every provider in payment_gateway_configs uses.
func NewCashfreeGateway(apiKey, secretKey, mode string) *CashfreeGateway {
	base := cashfreeLiveBaseURL
	if mode == "test" {
		base = cashfreeTestBaseURL
	}
	return &CashfreeGateway{
		appID:     apiKey,
		secretKey: secretKey,
		mode:      mode,
		baseURL:   base,
		client:    &http.Client{Timeout: cashfreeRequestTimeout},
	}
}

func (c *CashfreeGateway) ProviderName() string { return "cashfree" }

func (c *CashfreeGateway) SupportedCountries() []string {
	return []string{"IN"}
}

// --- Amounts ---

// cashfreeAmount is an amount in MINOR units (paise for INR) that marshals to
// — and unmarshals from — Cashfree's rupees-with-two-decimals wire format.
//
// The whole reason this type exists rather than a plain float64 field:
// rendering money by formatting a float64 is how a ₹140.25 order becomes
// 140.25000000000001 on the wire, and how repeated ÷100 ×100 round-trips shave
// a paise off a settlement. The conversion happens exactly once, here, at the
// boundary — by integer arithmetic on the way out, and through decimal (never
// float) on the way in.
type cashfreeAmount int64

// MarshalJSON renders minor units as a decimal with exactly two places, using
// integer division so no float ever touches the value.
func (a cashfreeAmount) MarshalJSON() ([]byte, error) {
	minor := int64(a)
	sign := ""
	if minor < 0 {
		// Never expected — a negative charge or refund is a caller bug — but
		// rendering "-1.-5" would be worse than rendering "-1.05", and
		// silently clamping to zero would hide the bug.
		sign, minor = "-", -minor
	}
	return fmt.Appendf(nil, "%s%d.%02d", sign, minor/100, minor%100), nil
}

// UnmarshalJSON accepts Cashfree's number (or a stringified number, which some
// fields use) and converts to minor units through decimal — the same rounding
// toMinorUnits applies, so a value read back from Cashfree compares equal to
// the value we sent.
func (a *cashfreeAmount) UnmarshalJSON(b []byte) error {
	s := strings.Trim(strings.TrimSpace(string(b)), `"`)
	if s == "" || s == "null" {
		*a = 0
		return nil
	}
	d, err := decimal.NewFromString(s)
	if err != nil {
		return fmt.Errorf("cashfree: parse amount %q: %w", s, err)
	}
	*a = cashfreeAmount(toMinorUnits(d, "INR"))
	return nil
}

// Decimal returns the amount as a major-unit decimal, the unit every caller in
// this service works in (orders, refunds and the ledger are all numeric(12,2)).
func (a cashfreeAmount) Decimal() decimal.Decimal {
	return decimal.New(int64(a), 0).Div(decimal.NewFromInt(100))
}

// --- Wire types ---

type cashfreeCustomerDetails struct {
	CustomerID    string `json:"customer_id"`
	CustomerPhone string `json:"customer_phone"`
	CustomerName  string `json:"customer_name,omitempty"`
	CustomerEmail string `json:"customer_email,omitempty"`
}

// cashfreeOrderMeta carries the redirect hook. notify_url is deliberately left
// empty: webhooks are configured once in the Cashfree dashboard rather than
// per order, because a per-order URL would silently bypass the signature
// secret we verify with.
type cashfreeOrderMeta struct {
	ReturnURL string `json:"return_url,omitempty"`
}

type cashfreeOrderRequest struct {
	OrderID   string                  `json:"order_id,omitempty"`
	Amount    cashfreeAmount          `json:"order_amount"`
	Currency  string                  `json:"order_currency"`
	Customer  cashfreeCustomerDetails `json:"customer_details"`
	Meta      *cashfreeOrderMeta      `json:"order_meta,omitempty"`
	Tags      map[string]string       `json:"order_tags,omitempty"`
	OrderNote string                  `json:"order_note,omitempty"`
}

// cashfreeOrderResponse is the created (or fetched) order. PaymentSessionID is
// the token the client SDK opens checkout with — it is the Cashfree analogue
// of Razorpay's order_id client token, and it is short-lived, which is why the
// create path reuses an ACTIVE order rather than minting a second one.
type cashfreeOrderResponse struct {
	CFOrderID        json.Number    `json:"cf_order_id"`
	OrderID          string         `json:"order_id"`
	PaymentSessionID string         `json:"payment_session_id"`
	OrderStatus      string         `json:"order_status"`
	Amount           cashfreeAmount `json:"order_amount"`
	Currency         string         `json:"order_currency"`
}

// Cashfree order_status values. EXPIRED / TERMINATED are dead: the session
// cannot be paid, so handing the client that token would only bounce.
const (
	cashfreeOrderActive = "ACTIVE"
	cashfreeOrderPaid   = "PAID"
)

// cashfreePayment is one payment attempt against an order.
//
// PaymentMethod is deliberately json.RawMessage: Cashfree returns an OBJECT
// there ({"upi":{…}} / {"card":{…}}), not a string. PaymentGroup is the flat
// label ("upi", "credit_card", "net_banking", …) and is what methodLabel maps
// into this service's short vocabulary — decoding the object into a typed
// struct would couple us to every instrument Cashfree adds.
type cashfreePayment struct {
	CFPaymentID   json.Number     `json:"cf_payment_id"`
	OrderID       string          `json:"order_id"`
	PaymentStatus string          `json:"payment_status"`
	Amount        cashfreeAmount  `json:"payment_amount"`
	Currency      string          `json:"payment_currency"`
	PaymentGroup  string          `json:"payment_group"`
	PaymentMethod json.RawMessage `json:"payment_method,omitempty"`
}

// Cashfree payment_status values.
const (
	cashfreePaymentSuccess     = "SUCCESS"
	cashfreePaymentFailed      = "FAILED"
	cashfreePaymentUserDropped = "USER_DROPPED"
)

// isCaptured reports whether this payment actually took the money. Cashfree
// has no separate authorize/capture step for the instruments we accept, so
// SUCCESS is the captured state.
func (p *cashfreePayment) isCaptured() bool {
	return p != nil && p.PaymentStatus == cashfreePaymentSuccess
}

// methodLabel maps Cashfree's payment_group onto the short method vocabulary
// already stored in payment_transactions.payment_method ("upi", "card",
// "netbanking", "wallet"), so an admin screen or receipt reads identically no
// matter which gateway took the payment. An unrecognised group passes through
// as-is rather than being flattened to "other" — a new instrument should show
// up in the data, not disappear into a bucket.
func (p *cashfreePayment) methodLabel() string {
	if p == nil {
		return ""
	}
	switch strings.ToLower(p.PaymentGroup) {
	case "upi":
		return "upi"
	case "credit_card", "debit_card", "card", "credit_card_emi", "debit_card_emi", "cardless_emi":
		return "card"
	case "net_banking", "netbanking":
		return "netbanking"
	case "wallet", "app":
		return "wallet"
	case "pay_later", "paylater":
		return "paylater"
	default:
		return strings.ToLower(p.PaymentGroup)
	}
}

type cashfreeRefundRequest struct {
	Amount   cashfreeAmount `json:"refund_amount"`
	RefundID string         `json:"refund_id"`
	Note     string         `json:"refund_note,omitempty"`
	// Speed is STANDARD or INSTANT; empty means STANDARD, which matches
	// Razorpay's "normal".
	Speed string `json:"refund_speed,omitempty"`
}

type cashfreeRefund struct {
	CFRefundID   json.Number    `json:"cf_refund_id"`
	CFPaymentID  json.Number    `json:"cf_payment_id"`
	RefundID     string         `json:"refund_id"`
	OrderID      string         `json:"order_id"`
	RefundStatus string         `json:"refund_status"`
	Amount       cashfreeAmount `json:"refund_amount"`
}

// Cashfree refund_status values.
const (
	cashfreeRefundSuccess   = "SUCCESS"
	cashfreeRefundPending   = "PENDING"
	cashfreeRefundOnHold    = "ONHOLD"
	cashfreeRefundCancelled = "CANCELLED"
	cashfreeRefundFailed    = "FAILED"
)

// normalizeCashfreeRefundStatus maps Cashfree's refund_status onto the status
// vocabulary refund_transactions already persists for Razorpay refunds.
//
// ONHOLD maps to pending, not failed: the money is still coming, it is just
// held for review at Cashfree. Calling it failed would make the refund sweeper
// re-issue a refund that is already in flight.
func normalizeCashfreeRefundStatus(status string) string {
	switch strings.ToUpper(status) {
	case cashfreeRefundSuccess:
		return "succeeded"
	case cashfreeRefundPending, cashfreeRefundOnHold:
		return "pending"
	case cashfreeRefundFailed, cashfreeRefundCancelled:
		return "failed"
	default:
		return strings.ToLower(status)
	}
}

// --- Gateway implementation ---

// CreateIntent creates a Cashfree order via POST /pg/orders and returns its
// payment_session_id as the client token.
//
// order_id is OUR order id, submitted verbatim. That is load-bearing: Cashfree
// refunds are order-scoped with no payment-level endpoint, so this is what
// makes RefundPayment resolvable from the ledger later (see RefundInput.OrderID).
//
// A duplicate order_id returns 409 from Cashfree. That is NOT an error here: it
// means a previous attempt already created this order (a timeout-after-success,
// or a double-tapped "Place order"), so the existing order is fetched and its
// still-valid session reused. Treating the 409 as a failure would strand a
// perfectly payable order and — worse — tempt the caller into minting a second
// order id for the same purchase, which is how a payment arrives for an order
// id nothing recognises.
func (c *CashfreeGateway) CreateIntent(ctx context.Context, in CreateIntentInput) (*Intent, error) {
	// Cashfree settles only in INR (see SupportedCountries). Reject a
	// mismatched currency loudly rather than silently billing the amount as
	// rupees, which is what the wire format would do.
	if in.CurrencyCode != "" && !strings.EqualFold(in.CurrencyCode, "INR") {
		return nil, fmt.Errorf("cashfree: create intent: unsupported currency %q (INR only)", in.CurrencyCode)
	}
	// customer_phone is mandatory at Cashfree and is what UPI intent keys off.
	// Failing here — before any HTTP call — turns an opaque gateway 400 into a
	// message that names the missing field.
	phone := strings.TrimSpace(in.CustomerPhone)
	if phone == "" {
		return nil, fmt.Errorf("cashfree: create intent: customer_phone is required by Cashfree and was empty")
	}

	req := cashfreeOrderRequest{
		OrderID:  in.OrderID,
		Amount:   cashfreeAmount(toMinorUnits(in.Amount, "INR")),
		Currency: "INR",
		Customer: cashfreeCustomerDetails{
			// customer_id must be stable per payer but is not a Cashfree-side
			// record we own, so derive it from the order. Cashfree restricts
			// this field to alphanumerics plus _ and -, which a uuid satisfies.
			CustomerID:    "order_" + in.OrderID,
			CustomerPhone: phone,
			CustomerName:  in.CustomerName,
			CustomerEmail: in.CustomerEmail,
		},
		Tags:      in.Metadata,
		OrderNote: in.Description,
	}

	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("cashfree: create intent: marshal: %w", err)
	}

	body, status, err := c.do(ctx, http.MethodPost, "/orders", payload, nil)
	if err != nil {
		return nil, fmt.Errorf("cashfree: create intent: %w", err)
	}

	var result cashfreeOrderResponse
	switch {
	case status == http.StatusConflict && in.OrderID != "":
		existing, ferr := c.fetchOrder(ctx, in.OrderID)
		if ferr != nil {
			return nil, fmt.Errorf("cashfree: create intent: order %s exists but fetch failed: %w", in.OrderID, ferr)
		}
		if existing.OrderStatus != cashfreeOrderActive || existing.PaymentSessionID == "" {
			// PAID / EXPIRED / TERMINATED: there is nothing payable to hand
			// back. Say which state it is in rather than returning an empty
			// token the storefront would try to open.
			return nil, fmt.Errorf("cashfree: create intent: order %s already exists and is not payable (status %s)",
				in.OrderID, existing.OrderStatus)
		}
		result = *existing
	case status >= 400:
		return nil, cashfreeError(status, body)
	default:
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("cashfree: create intent: decode: %w", err)
		}
	}

	if result.PaymentSessionID == "" {
		return nil, fmt.Errorf("cashfree: create intent: order %s returned no payment_session_id", result.OrderID)
	}

	return &Intent{
		// The Cashfree order id we can address later is the merchant order_id
		// we submitted, not cf_order_id — every /pg/orders/{id} route accepts
		// the merchant id, and it is the one the refund path can reconstruct.
		ProviderIntentID: result.OrderID,
		ClientToken:      result.PaymentSessionID,
		Status:           normalizeCashfreeOrderStatus(result.OrderStatus),
	}, nil
}

// normalizeCashfreeOrderStatus maps Cashfree's order_status onto the
// payment_transactions.status vocabulary the checkout path persists. ACTIVE is
// "pending" — created and payable, nothing captured yet.
func normalizeCashfreeOrderStatus(status string) string {
	switch strings.ToUpper(status) {
	case cashfreeOrderActive:
		return "pending"
	case cashfreeOrderPaid:
		return "captured"
	default:
		return strings.ToLower(status)
	}
}

// CapturePayment confirms the settled payment on a Cashfree order.
//
// Cashfree auto-captures the instruments we accept — there is no separate
// capture call to make — so this is a read: fetch the order's payments and
// report the SUCCESS one. captureID is the Cashfree order id (the value
// CreateIntent returned as ProviderIntentID), because Cashfree scopes payments
// under their order and offers no global payment-fetch route.
func (c *CashfreeGateway) CapturePayment(ctx context.Context, captureID string) (*Capture, error) {
	p, err := c.successfulPayment(ctx, captureID)
	if err != nil {
		return nil, fmt.Errorf("cashfree: capture payment: %w", err)
	}
	if p == nil {
		return nil, fmt.Errorf("cashfree: capture payment: no captured payment on order %s", captureID)
	}
	return &Capture{
		ProviderPaymentID: p.CFPaymentID.String(),
		Status:            "captured",
		PaymentMethod:     p.methodLabel(),
	}, nil
}

// FetchOrderPayment implements OrderStatusGateway. It is the authority the
// confirm path uses: with no client-side signature to check, the gateway's own
// record of what was paid is the only trustworthy input.
//
// Returns (nil, nil) when the order exists but has no captured payment yet —
// the caller reads that as "not paid", which is exactly right.
func (c *CashfreeGateway) FetchOrderPayment(ctx context.Context, providerOrderID string) (*OrderPayment, error) {
	payments, err := c.fetchOrderPayments(ctx, providerOrderID)
	if err != nil {
		return nil, err
	}
	// Prefer the captured attempt. Never just take payments[0]: an order can
	// carry a failed or user-dropped attempt ahead of the one that paid.
	for i := range payments {
		if payments[i].isCaptured() {
			p := &payments[i]
			return &OrderPayment{
				ProviderPaymentID: p.CFPaymentID.String(),
				Status:            "payment.succeeded",
				PaymentMethod:     p.methodLabel(),
				Amount:            p.Amount.Decimal(),
				CurrencyCode:      strings.ToUpper(p.Currency),
			}, nil
		}
	}
	return nil, nil
}

// RefundPayment creates a refund via POST /pg/orders/{order_id}/refunds.
//
// Keyed off in.OrderID, not in.ProviderPaymentID: Cashfree has no
// payment-level refund endpoint. The refund_id is derived deterministically
// from the caller's IdempotencyKey, so a retry is the SAME refund — and a 409
// or 422 (duplicate refund_id / idempotency replay) is read back as success
// rather than reported as a failure that would send the saga round again.
// Mishandling that is how a customer receives two refunds.
func (c *CashfreeGateway) RefundPayment(ctx context.Context, in RefundInput) (*Refund, error) {
	if in.CurrencyCode != "" && !strings.EqualFold(in.CurrencyCode, "INR") {
		return nil, fmt.Errorf("cashfree: refund payment: unsupported currency %q (INR only)", in.CurrencyCode)
	}
	orderID := strings.TrimSpace(in.OrderID)
	if orderID == "" {
		// Refusing here is deliberate: there is no order-id-free refund route
		// to fall back to, and guessing one from ProviderPaymentID (a
		// cf_payment_id) is impossible.
		return nil, fmt.Errorf("cashfree: refund payment: OrderID is required (Cashfree refunds are order-scoped)")
	}
	if in.IdempotencyKey == "" {
		// Letting Cashfree mint the refund_id would remove the dedup that
		// stops a retry becoming a second real refund.
		return nil, fmt.Errorf("cashfree: refund payment: IdempotencyKey is required to derive refund_id")
	}

	refundID := cashfreeRefundID(in.IdempotencyKey)
	req := cashfreeRefundRequest{
		Amount:   cashfreeAmount(toMinorUnits(in.Amount, "INR")),
		RefundID: refundID,
		Note:     in.Reason,
		Speed:    "STANDARD",
	}

	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("cashfree: refund payment: marshal: %w", err)
	}

	path := "/orders/" + orderID + "/refunds"
	body, status, err := c.do(ctx, http.MethodPost, path, payload, map[string]string{
		cashfreeHeaderIdempotency: refundID,
	})
	if err != nil {
		return nil, fmt.Errorf("cashfree: refund payment: %w", err)
	}

	var result cashfreeRefund
	switch {
	case status == http.StatusConflict || status == http.StatusUnprocessableEntity:
		existing, ferr := c.fetchRefund(ctx, orderID, refundID)
		if ferr != nil {
			// Fall through to the typed error so the saga can classify it.
			return nil, &GatewayError{Provider: "cashfree", StatusCode: status, Body: cashfreeErrorBody(body)}
		}
		result = *existing
	case status >= 400:
		// GatewayError (not a bare fmt.Errorf) so the refund saga's
		// Permanent() check can move a hopeless row to 'failed' instead of
		// re-driving it forever. Only the structured envelope is carried, not
		// the raw body — Cashfree echoes submitted values back in some
		// validation errors, and a rejected phone or bank detail must not ride
		// an error message into a log line.
		return nil, &GatewayError{Provider: "cashfree", StatusCode: status, Body: cashfreeErrorBody(body)}
	default:
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("cashfree: refund payment: decode: %w", err)
		}
	}

	return &Refund{
		ProviderRefundID: result.RefundID,
		Status:           normalizeCashfreeRefundStatus(result.RefundStatus),
		Amount:           result.Amount.Decimal(),
	}, nil
}

// cashfreeRefundID normalizes a logical operation id into Cashfree's refund_id
// window: 3–40 characters, alphanumeric plus _ and -.
//
// The coordinator's key is "refund_{uuid}_{scope}", which is already over 40
// characters, so it cannot be sent verbatim. A sha256 digest truncated to 32
// hex chars fits the window, keeps the charset, and stays deterministic — the
// same logical refund derives the same refund_id on every retry, which is the
// entire basis of the idempotency guarantee. This matches the
// normalizeIdempotencyKey digest Home-Chef-App feeds Cashfree.
func cashfreeRefundID(logical string) string {
	sum := sha256.Sum256([]byte(logical))
	return hex.EncodeToString(sum[:])[:32]
}

// VerifyWebhook verifies a Cashfree webhook and normalizes the event.
//
// Cashfree signs base64(HMAC-SHA256(timestamp + rawBody, secret)) — the RAW
// body must be used, not a re-marshalled parse, or the digest will not match.
// The timestamp arrives in a separate header, so `signature` is the composite
// documented on parseCashfreeSignature.
func (c *CashfreeGateway) VerifyWebhook(_ context.Context, payload []byte, signature string) (*WebhookEvent, error) {
	timestamp, sig, err := parseCashfreeSignature(signature)
	if err != nil {
		return nil, err
	}
	// Fail closed on a stale or unparseable timestamp: a caller cannot
	// distinguish "no replay protection" from "verified".
	if !cashfreeTimestampFresh(timestamp) {
		return nil, fmt.Errorf("cashfree: verify webhook: stale or invalid timestamp %q", timestamp)
	}

	mac := hmac.New(sha256.New, []byte(c.secretKey))
	mac.Write([]byte(timestamp))
	mac.Write(payload)
	expected := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(sig), []byte(expected)) {
		return nil, fmt.Errorf("cashfree: verify webhook: signature mismatch")
	}

	var envelope struct {
		Type      string `json:"type"`
		EventTime string `json:"event_time"`
		Data      struct {
			Order struct {
				OrderID  string            `json:"order_id"`
				Amount   cashfreeAmount    `json:"order_amount"`
				Currency string            `json:"order_currency"`
				Tags     map[string]string `json:"order_tags"`
			} `json:"order"`
			Payment cashfreePayment `json:"payment"`
			Refund  cashfreeRefund  `json:"refund"`
		} `json:"data"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return nil, fmt.Errorf("cashfree: verify webhook: decode: %w", err)
	}

	d := envelope.Data
	eventType := normalizeCashfreeEvent(envelope.Type)

	// Refund events carry their ids under data.refund and no data.payment.
	if eventType == "refund.succeeded" || envelope.Type == cashfreeWebhookRefundStatus {
		orderID := d.Refund.OrderID
		if orderID == "" {
			orderID = d.Order.OrderID
		}
		return &WebhookEvent{
			// refund_id is the merchant-supplied id we derived, so it is
			// stable across Cashfree's own retries — the right idempotency
			// key for the webhook_events unique index.
			ProviderEventID:   "cfrefund_" + d.Refund.RefundID,
			EventType:         eventType,
			OrderID:           orderID,
			ProviderPaymentID: d.Refund.CFPaymentID.String(),
			Metadata:          d.Order.Tags,
			Amount:            d.Refund.Amount.Decimal(),
			CurrencyCode:      "INR",
			RawPayload:        payload,
		}, nil
	}

	orderID := d.Order.OrderID
	if orderID == "" {
		orderID = d.Payment.OrderID
	}
	amount := d.Payment.Amount
	if amount == 0 {
		amount = d.Order.Amount
	}
	currency := d.Payment.Currency
	if currency == "" {
		currency = d.Order.Currency
	}

	return &WebhookEvent{
		// Cashfree sends no event-id header. cf_payment_id scoped by event
		// type is stable across genuine retries (a redelivery repeats the same
		// payment) while still letting a later refund event on the same
		// payment through the (provider, provider_event_id) unique index.
		ProviderEventID:   "cfpay_" + eventType + "_" + d.Payment.CFPaymentID.String(),
		EventType:         eventType,
		OrderID:           orderID,
		ProviderPaymentID: d.Payment.CFPaymentID.String(),
		Metadata:          d.Order.Tags,
		Amount:            amount.Decimal(),
		CurrencyCode:      strings.ToUpper(currency),
		PaymentMethod:     d.Payment.methodLabel(),
		RawPayload:        payload,
	}, nil
}

// Cashfree webhook types we act on.
const (
	cashfreeWebhookPaymentSuccess = "PAYMENT_SUCCESS_WEBHOOK"
	cashfreeWebhookPaymentFailed  = "PAYMENT_FAILED_WEBHOOK"
	cashfreeWebhookUserDropped    = "PAYMENT_USER_DROPPED_WEBHOOK"
	cashfreeWebhookRefundStatus   = "REFUND_STATUS_WEBHOOK"
)

// normalizeCashfreeEvent maps Cashfree's event types onto the normalized
// vocabulary processEvent switches on. USER_DROPPED is a failure for our
// purposes — the buyer abandoned the sheet and the order stays reserved,
// which is the same handling a hard decline gets.
func normalizeCashfreeEvent(event string) string {
	switch event {
	case cashfreeWebhookPaymentSuccess:
		return "payment.succeeded"
	case cashfreeWebhookPaymentFailed, cashfreeWebhookUserDropped:
		return "payment.failed"
	case cashfreeWebhookRefundStatus:
		return "refund.succeeded"
	default:
		return event
	}
}

// Cashfree webhook headers. Exported so the webhook route reads them from the
// same place the verifier defines them — a drift between the header the router
// reads and the one the gateway expects would 401 every real delivery.
const (
	CashfreeWebhookSignatureHeader = "x-webhook-signature"
	CashfreeWebhookTimestampHeader = "x-webhook-timestamp"
)

// CashfreeWebhookSignature packs the two headers Cashfree's signature scheme
// needs into the single `signature` string Gateway.VerifyWebhook accepts.
//
// The interface takes one string because every other provider signs the body
// alone; Cashfree prefixes the delivery timestamp, so both values have to
// travel. PayPal already establishes this precedent (it packs its header set as
// JSON). The separator is "." because a Cashfree timestamp is epoch seconds —
// digits only — so the split is unambiguous even though base64 signatures can
// contain "+", "/" and "=".
func CashfreeWebhookSignature(timestamp, signature string) string {
	return timestamp + "." + signature
}

// parseCashfreeSignature splits the composite CashfreeWebhookSignature builds.
func parseCashfreeSignature(composite string) (timestamp, signature string, err error) {
	ts, sig, found := strings.Cut(composite, ".")
	if !found || ts == "" || sig == "" {
		return "", "", fmt.Errorf("cashfree: verify webhook: malformed signature header pair " +
			"(want \"<x-webhook-timestamp>.<x-webhook-signature>\")")
	}
	return ts, sig, nil
}

// cashfreeTimestampFresh bounds the signed timestamp. Cashfree sends epoch
// seconds; an unparseable value fails closed.
func cashfreeTimestampFresh(ts string) bool {
	secs, err := strconv.ParseInt(strings.TrimSpace(ts), 10, 64)
	if err != nil {
		return false
	}
	age := time.Since(time.Unix(secs, 0))
	if age < 0 {
		age = -age // tolerate a slightly fast sender clock
	}
	return age <= cashfreeWebhookMaxSkew
}

// --- Internal reads ---

func (c *CashfreeGateway) fetchOrder(ctx context.Context, orderID string) (*cashfreeOrderResponse, error) {
	body, status, err := c.do(ctx, http.MethodGet, "/orders/"+orderID, nil, nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, cashfreeError(status, body)
	}
	var result cashfreeOrderResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("cashfree: decode fetch-order: %w", err)
	}
	return &result, nil
}

// fetchOrderPayments returns every payment attempt on an order. A 404 means no
// attempts yet — an empty list, not a failure.
func (c *CashfreeGateway) fetchOrderPayments(ctx context.Context, orderID string) ([]cashfreePayment, error) {
	body, status, err := c.do(ctx, http.MethodGet, "/orders/"+orderID+"/payments", nil, nil)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, nil
	}
	if status >= 400 {
		return nil, cashfreeError(status, body)
	}
	var result []cashfreePayment
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("cashfree: decode order payments: %w", err)
	}
	return result, nil
}

// successfulPayment returns the captured payment on an order, or nil when
// there isn't one. Centralised because "which of these attempts is the real
// one" is a question every caller would otherwise answer slightly differently
// — and one that must never be answered by picking the first element.
func (c *CashfreeGateway) successfulPayment(ctx context.Context, orderID string) (*cashfreePayment, error) {
	payments, err := c.fetchOrderPayments(ctx, orderID)
	if err != nil {
		return nil, err
	}
	for i := range payments {
		if payments[i].isCaptured() {
			return &payments[i], nil
		}
	}
	return nil, nil
}

func (c *CashfreeGateway) fetchRefund(ctx context.Context, orderID, refundID string) (*cashfreeRefund, error) {
	body, status, err := c.do(ctx, http.MethodGet, "/orders/"+orderID+"/refunds/"+refundID, nil, nil)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, cashfreeError(status, body)
	}
	var result cashfreeRefund
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("cashfree: decode fetch-refund: %w", err)
	}
	return &result, nil
}

// --- HTTP ---

const cashfreeHeaderIdempotency = "x-idempotency-key"

// do performs one authenticated round-trip and returns the body and status.
// Status is returned rather than folded into an error because several callers
// treat specific non-2xx codes as success (409 on create, 409/422 on refund,
// 404 on an order with no payments yet) — collapsing them into an error string
// and re-parsing it would be fragile.
func (c *CashfreeGateway) do(
	ctx context.Context,
	method, path string,
	body []byte,
	extraHeaders map[string]string,
) ([]byte, int, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return nil, 0, fmt.Errorf("build request: %w", err)
	}

	// Cashfree authenticates with a header pair, not HTTP Basic.
	req.Header.Set("x-client-id", c.appID)
	req.Header.Set("x-client-secret", c.secretKey)
	req.Header.Set("x-api-version", cashfreeAPIVersion)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	for k, v := range extraHeaders {
		if v != "" {
			req.Header.Set(k, v)
		}
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read response: %w", err)
	}
	return respBody, resp.StatusCode, nil
}

// cashfreeErrorBody extracts Cashfree's structured error envelope
// ({"message":…,"code":…,"type":…}).
//
// Only the structured fields are surfaced, never the raw body: Cashfree echoes
// submitted values back in some validation errors, and a rejected customer
// phone or bank detail must not ride an error message into a log line or an
// admin-facing refund failure reason.
func cashfreeErrorBody(body []byte) string {
	var parsed struct {
		Message string `json:"message"`
		Code    string `json:"code"`
		Type    string `json:"type"`
	}
	if err := json.Unmarshal(body, &parsed); err == nil && (parsed.Code != "" || parsed.Message != "") {
		return parsed.Code + ": " + parsed.Message
	}
	return "unrecognized error envelope"
}

func cashfreeError(status int, body []byte) error {
	return fmt.Errorf("cashfree API error (HTTP %d): %s", status, cashfreeErrorBody(body))
}
