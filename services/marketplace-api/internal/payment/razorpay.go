package payment

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

const (
	razorpayLiveURL = "https://api.razorpay.com"
	razorpayTestURL = "https://api.razorpay.com" // Razorpay uses the same URL; test mode is key-based.
)

// RazorpayGateway implements Gateway for Razorpay using the REST API.
type RazorpayGateway struct {
	apiKey    string // key_id
	secretKey string // key_secret (also used as webhook secret)
	mode      string
	baseURL   string
	client    *http.Client
}

// NewRazorpayGateway returns a Razorpay Gateway ready for use.
func NewRazorpayGateway(apiKey, secretKey, mode string) *RazorpayGateway {
	base := razorpayLiveURL
	if mode == "test" {
		base = razorpayTestURL
	}
	return &RazorpayGateway{
		apiKey:    apiKey,
		secretKey: secretKey,
		mode:      mode,
		baseURL:   base,
		client:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (r *RazorpayGateway) ProviderName() string { return "razorpay" }

func (r *RazorpayGateway) SupportedCountries() []string {
	return []string{"IN"}
}

// CreateIntent creates a Razorpay order via POST /v1/orders.
func (r *RazorpayGateway) CreateIntent(ctx context.Context, in CreateIntentInput) (*Intent, error) {
	amountPaise := toMinorUnits(in.Amount, in.CurrencyCode)

	body := map[string]any{
		"amount":   amountPaise,
		"currency": strings.ToUpper(in.CurrencyCode),
		"receipt":  in.OrderID,
		"notes": map[string]string{
			"order_id": in.OrderID,
			"email":    in.CustomerEmail,
		},
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("razorpay: create intent: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.baseURL+"/v1/orders", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("razorpay: create intent: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(r.apiKey, r.secretKey)

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("razorpay: create intent: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("razorpay: create intent: read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("razorpay: create intent: status %d: %s", resp.StatusCode, respBody)
	}

	var result struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("razorpay: create intent: decode: %w", err)
	}

	return &Intent{
		ProviderIntentID: result.ID,
		ClientToken:      result.ID, // Razorpay checkout uses order_id as the client token.
		Status:           result.Status,
	}, nil
}

// CapturePayment captures a Razorpay payment via POST /v1/payments/:id/capture.
func (r *RazorpayGateway) CapturePayment(ctx context.Context, captureID string) (*Capture, error) {
	// Razorpay capture requires the amount; we capture the full amount by
	// fetching the payment first.
	payment, err := r.fetchPayment(ctx, captureID)
	if err != nil {
		return nil, fmt.Errorf("razorpay: capture payment: fetch: %w", err)
	}

	captureBody := map[string]any{
		"amount":   payment.Amount,
		"currency": payment.Currency,
	}

	payload, err := json.Marshal(captureBody)
	if err != nil {
		return nil, fmt.Errorf("razorpay: capture payment: marshal: %w", err)
	}

	endpoint := fmt.Sprintf("%s/v1/payments/%s/capture", r.baseURL, captureID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("razorpay: capture payment: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(r.apiKey, r.secretKey)

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("razorpay: capture payment: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("razorpay: capture payment: read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("razorpay: capture payment: status %d: %s", resp.StatusCode, respBody)
	}

	var result struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Method string `json:"method"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("razorpay: capture payment: decode: %w", err)
	}

	return &Capture{
		ProviderPaymentID: result.ID,
		Status:            result.Status,
		PaymentMethod:     result.Method,
	}, nil
}

// RefundPayment creates a refund via POST /v1/refunds.
func (r *RazorpayGateway) RefundPayment(ctx context.Context, in RefundInput) (*Refund, error) {
	amountPaise := toMinorUnits(in.Amount, "INR")

	body := map[string]any{
		"payment_id": in.ProviderPaymentID,
		"amount":     amountPaise,
	}
	if in.Reason != "" {
		body["notes"] = map[string]string{"reason": in.Reason}
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("razorpay: refund payment: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.baseURL+"/v1/refunds", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("razorpay: refund payment: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(r.apiKey, r.secretKey)
	if in.IdempotencyKey != "" {
		req.Header.Set("X-Refund-Idempotency", in.IdempotencyKey)
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("razorpay: refund payment: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("razorpay: refund payment: read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("razorpay: refund payment: status %d: %s", resp.StatusCode, respBody)
	}

	var result struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Amount int64  `json:"amount"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("razorpay: refund payment: decode: %w", err)
	}

	return &Refund{
		ProviderRefundID: result.ID,
		Status:           result.Status,
		Amount:           decimal.NewFromInt(result.Amount),
	}, nil
}

// VerifyWebhook verifies a Razorpay webhook using HMAC-SHA256.
// Razorpay sends the signature in the X-Razorpay-Signature header.
func (r *RazorpayGateway) VerifyWebhook(_ context.Context, payload []byte, signature string) (*WebhookEvent, error) {
	mac := hmac.New(sha256.New, []byte(r.secretKey))
	mac.Write(payload)
	expected := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(signature), []byte(expected)) {
		return nil, fmt.Errorf("razorpay: verify webhook: signature mismatch")
	}

	var raw struct {
		Event   string `json:"event"`
		Payload struct {
			Payment struct {
				Entity struct {
					ID       string            `json:"id"`
					OrderID  string            `json:"order_id"`
					Amount   int64             `json:"amount"`
					Currency string            `json:"currency"`
					Method   string            `json:"method"`
					Notes    map[string]string `json:"notes"`
				} `json:"entity"`
			} `json:"payment"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(payload, &raw); err != nil {
		return nil, fmt.Errorf("razorpay: verify webhook: decode: %w", err)
	}

	entity := raw.Payload.Payment.Entity

	orderID := entity.Notes["order_id"]
	if orderID == "" {
		orderID = entity.OrderID
	}

	return &WebhookEvent{
		ProviderEventID:   entity.ID,
		ProviderPaymentID: entity.ID,
		EventType:         normalizeRazorpayEvent(raw.Event),
		OrderID:           orderID,
		Amount:            decimal.NewFromInt(entity.Amount),
		CurrencyCode:      strings.ToUpper(entity.Currency),
		PaymentMethod:     entity.Method,
		RawPayload:        payload,
	}, nil
}

// fetchPayment retrieves a Razorpay payment by ID for capture.
func (r *RazorpayGateway) fetchPayment(ctx context.Context, paymentID string) (*razorpayPayment, error) {
	endpoint := fmt.Sprintf("%s/v1/payments/%s", r.baseURL, paymentID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(r.apiKey, r.secretKey)

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, body)
	}

	var p razorpayPayment
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

type razorpayPayment struct {
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

func normalizeRazorpayEvent(event string) string {
	switch event {
	case "payment.captured":
		return "payment.succeeded"
	case "payment.failed":
		return "payment.failed"
	case "refund.created":
		return "refund.succeeded"
	default:
		return event
	}
}
