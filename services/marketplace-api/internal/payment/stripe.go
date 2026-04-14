package payment

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

const (
	stripeLiveURL = "https://api.stripe.com"
	stripeTestURL = "https://api.stripe.com" // Stripe uses the same URL; test mode is key-based.
)

// StripeGateway implements Gateway for Stripe using the REST API.
type StripeGateway struct {
	apiKey    string
	secretKey string // webhook signing secret
	mode      string
	baseURL   string
	client    *http.Client
}

// NewStripeGateway returns a Stripe Gateway ready for use.
func NewStripeGateway(apiKey, secretKey, mode string) *StripeGateway {
	base := stripeLiveURL
	if mode == "test" {
		base = stripeTestURL
	}
	return &StripeGateway{
		apiKey:    apiKey,
		secretKey: secretKey,
		mode:      mode,
		baseURL:   base,
		client:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (s *StripeGateway) ProviderName() string { return "stripe" }

func (s *StripeGateway) SupportedCountries() []string {
	return []string{"US", "CA", "GB", "DE", "FR", "IT", "ES", "NL", "AU", "SG", "MY", "TH", "PH", "ID"}
}

// CreateIntent creates a Stripe PaymentIntent via POST /v1/payment_intents.
func (s *StripeGateway) CreateIntent(ctx context.Context, in CreateIntentInput) (*Intent, error) {
	amountMinor := toMinorUnits(in.Amount, in.CurrencyCode)

	form := url.Values{}
	form.Set("amount", strconv.FormatInt(amountMinor, 10))
	form.Set("currency", strings.ToLower(in.CurrencyCode))
	form.Set("description", in.Description)
	form.Set("receipt_email", in.CustomerEmail)
	form.Set("metadata[order_id]", in.OrderID)
	for k, v := range in.Metadata {
		form.Set(fmt.Sprintf("metadata[%s]", k), v)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+"/v1/payment_intents", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("stripe: create intent: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(s.apiKey, "")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("stripe: create intent: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("stripe: create intent: read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("stripe: create intent: status %d: %s", resp.StatusCode, body)
	}

	var result struct {
		ID           string `json:"id"`
		ClientSecret string `json:"client_secret"`
		Status       string `json:"status"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("stripe: create intent: decode: %w", err)
	}

	return &Intent{
		ProviderIntentID: result.ID,
		ClientToken:      result.ClientSecret,
		Status:           result.Status,
	}, nil
}

// CreateCheckoutSession creates a Stripe Checkout Session. The customer
// is redirected to the returned URL to complete payment; Stripe then
// redirects to SuccessURL (or CancelURL). The gift-card id travels as
// `metadata[gift_card_id]` so the webhook can correlate.
//
// We use `mode=payment` (one-time, no subscription) and a single
// line_item built via `price_data` so we don't need pre-registered
// Stripe Products.
func (s *StripeGateway) CreateCheckoutSession(ctx context.Context, in CreateCheckoutSessionInput) (*CheckoutSession, error) {
	amountMinor := toMinorUnits(in.Amount, in.CurrencyCode)

	form := url.Values{}
	form.Set("mode", "payment")
	form.Set("success_url", in.SuccessURL)
	form.Set("cancel_url", in.CancelURL)
	if in.CustomerEmail != "" {
		form.Set("customer_email", in.CustomerEmail)
	}
	// Single line item built inline via price_data (no Stripe Product needed).
	form.Set("line_items[0][quantity]", "1")
	form.Set("line_items[0][price_data][currency]", strings.ToLower(in.CurrencyCode))
	form.Set("line_items[0][price_data][unit_amount]", strconv.FormatInt(amountMinor, 10))
	name := in.Name
	if name == "" {
		name = "Purchase"
	}
	form.Set("line_items[0][price_data][product_data][name]", name)
	if in.Description != "" {
		form.Set("line_items[0][price_data][product_data][description]", in.Description)
	}

	// Metadata on both the Session and the PaymentIntent so either
	// webhook type can correlate back.
	for k, v := range in.Metadata {
		form.Set(fmt.Sprintf("metadata[%s]", k), v)
		form.Set(fmt.Sprintf("payment_intent_data[metadata][%s]", k), v)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+"/v1/checkout/sessions", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("stripe: create checkout session: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(s.apiKey, "")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("stripe: create checkout session: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("stripe: create checkout session: read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("stripe: create checkout session: status %d: %s", resp.StatusCode, body)
	}

	var result struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("stripe: create checkout session: decode: %w", err)
	}
	return &CheckoutSession{ID: result.ID, URL: result.URL}, nil
}

// CapturePayment captures a PaymentIntent via POST /v1/payment_intents/:id/capture.
func (s *StripeGateway) CapturePayment(ctx context.Context, captureID string) (*Capture, error) {
	endpoint := fmt.Sprintf("%s/v1/payment_intents/%s/capture", s.baseURL, captureID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("stripe: capture payment: %w", err)
	}
	req.SetBasicAuth(s.apiKey, "")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("stripe: capture payment: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("stripe: capture payment: read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("stripe: capture payment: status %d: %s", resp.StatusCode, body)
	}

	var result struct {
		ID            string `json:"id"`
		Status        string `json:"status"`
		PaymentMethod string `json:"payment_method"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("stripe: capture payment: decode: %w", err)
	}

	return &Capture{
		ProviderPaymentID: result.ID,
		Status:            result.Status,
		PaymentMethod:     result.PaymentMethod,
	}, nil
}

// RefundPayment creates a refund via POST /v1/refunds.
func (s *StripeGateway) RefundPayment(ctx context.Context, in RefundInput) (*Refund, error) {
	amountMinor := toMinorUnits(in.Amount, "")

	form := url.Values{}
	form.Set("payment_intent", in.ProviderPaymentID)
	form.Set("amount", strconv.FormatInt(amountMinor, 10))
	if in.Reason != "" {
		form.Set("reason", in.Reason)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+"/v1/refunds", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("stripe: refund payment: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(s.apiKey, "")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("stripe: refund payment: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("stripe: refund payment: read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("stripe: refund payment: status %d: %s", resp.StatusCode, body)
	}

	var result struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Amount int64  `json:"amount"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("stripe: refund payment: decode: %w", err)
	}

	return &Refund{
		ProviderRefundID: result.ID,
		Status:           result.Status,
		Amount:           decimal.NewFromInt(result.Amount),
	}, nil
}

// VerifyWebhook verifies a Stripe webhook signature using HMAC-SHA256
// against the Stripe-Signature header format: t=<ts>,v1=<sig>.
func (s *StripeGateway) VerifyWebhook(_ context.Context, payload []byte, signature string) (*WebhookEvent, error) {
	if err := verifyStripeSignature(payload, signature, s.secretKey); err != nil {
		return nil, fmt.Errorf("stripe: verify webhook: %w", err)
	}

	var raw struct {
		ID   string `json:"id"`
		Type string `json:"type"`
		Data struct {
			Object json.RawMessage `json:"object"`
		} `json:"data"`
	}
	if err := json.Unmarshal(payload, &raw); err != nil {
		return nil, fmt.Errorf("stripe: verify webhook: decode: %w", err)
	}

	evt := &WebhookEvent{
		ProviderEventID: raw.ID,
		EventType:       normalizeStripeEvent(raw.Type),
		RawPayload:      payload,
	}

	// Extract order-level details from the nested object. Stripe surfaces
	// different fields depending on the event type:
	//   - payment_intent.*          → id=<pi_…>, amount, currency, metadata
	//   - checkout.session.*        → id=<cs_…>, amount_total, currency,
	//                                  payment_intent=<pi_…>, metadata
	var obj struct {
		ID            string            `json:"id"`
		Metadata      map[string]string `json:"metadata"`
		Amount        int64             `json:"amount"`
		AmountTotal   int64             `json:"amount_total"`
		Currency      string            `json:"currency"`
		PaymentMethod string            `json:"payment_method"`
		PaymentIntent string            `json:"payment_intent"`
	}
	if err := json.Unmarshal(raw.Data.Object, &obj); err == nil {
		evt.Metadata = obj.Metadata
		evt.OrderID = obj.Metadata["order_id"]
		// For PaymentIntent events, Amount is set; for Checkout Session
		// events, AmountTotal is set. Pick whichever is non-zero.
		if obj.Amount > 0 {
			evt.Amount = decimal.NewFromInt(obj.Amount)
		} else {
			evt.Amount = decimal.NewFromInt(obj.AmountTotal)
		}
		evt.CurrencyCode = strings.ToUpper(obj.Currency)
		evt.PaymentMethod = obj.PaymentMethod

		// Checkout Session events: the object id is cs_…; the settled
		// PaymentIntent id lives in the `payment_intent` field.
		if strings.HasPrefix(obj.ID, "cs_") {
			evt.SessionID = obj.ID
			evt.ProviderPaymentID = obj.PaymentIntent
		} else if strings.HasPrefix(obj.ID, "pi_") {
			evt.ProviderPaymentID = obj.ID
		}
	}

	return evt, nil
}

// verifyStripeSignature checks the Stripe-Signature header.
// Header format: t=<timestamp>,v1=<hex_signature>[,v1=<hex_signature>...]
func verifyStripeSignature(payload []byte, header, secret string) error {
	parts := strings.Split(header, ",")
	var timestamp string
	var signatures []string

	for _, p := range parts {
		kv := strings.SplitN(p, "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "t":
			timestamp = kv[1]
		case "v1":
			signatures = append(signatures, kv[1])
		}
	}

	if timestamp == "" || len(signatures) == 0 {
		return fmt.Errorf("invalid signature header")
	}

	// Reject stale webhooks older than 5 minutes to prevent replay attacks.
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid timestamp in signature header")
	}
	if time.Since(time.Unix(ts, 0)).Abs() > 5*time.Minute {
		return fmt.Errorf("webhook timestamp too old (replay rejected)")
	}

	// Construct the signed payload: "<timestamp>.<payload>"
	signedPayload := []byte(timestamp + "." + string(payload))

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(signedPayload)
	expected := hex.EncodeToString(mac.Sum(nil))

	for _, sig := range signatures {
		if hmac.Equal([]byte(sig), []byte(expected)) {
			return nil
		}
	}

	return fmt.Errorf("signature mismatch")
}

// normalizeStripeEvent maps Stripe event types to our canonical names.
func normalizeStripeEvent(stripeType string) string {
	switch stripeType {
	case "payment_intent.succeeded":
		return "payment.succeeded"
	case "payment_intent.payment_failed":
		return "payment.failed"
	case "charge.refunded":
		return "refund.succeeded"
	case "checkout.session.completed":
		return "checkout.completed"
	case "checkout.session.async_payment_succeeded":
		return "checkout.completed"
	case "checkout.session.async_payment_failed":
		return "checkout.failed"
	case "checkout.session.expired":
		return "checkout.expired"
	default:
		return stripeType
	}
}

// toMinorUnits converts a decimal amount to the smallest currency unit
// (e.g. cents for USD). For zero-decimal currencies like JPY it returns
// the integer value directly.
func toMinorUnits(amount decimal.Decimal, currency string) int64 {
	upper := strings.ToUpper(currency)
	switch upper {
	case "JPY", "KRW", "VND", "BIF", "CLP", "DJF", "GNF", "ISK",
		"KMF", "PYG", "RWF", "UGX", "XAF", "XOF", "XPF":
		return amount.IntPart()
	default:
		return amount.Mul(decimal.NewFromInt(100)).IntPart()
	}
}
