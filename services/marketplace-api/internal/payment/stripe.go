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

	// Extract order-level details from the nested object when available.
	var obj struct {
		Metadata      map[string]string `json:"metadata"`
		Amount        int64             `json:"amount"`
		Currency      string            `json:"currency"`
		PaymentMethod string            `json:"payment_method"`
	}
	if err := json.Unmarshal(raw.Data.Object, &obj); err == nil {
		evt.OrderID = obj.Metadata["order_id"]
		evt.Amount = decimal.NewFromInt(obj.Amount)
		evt.CurrencyCode = strings.ToUpper(obj.Currency)
		evt.PaymentMethod = obj.PaymentMethod
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
