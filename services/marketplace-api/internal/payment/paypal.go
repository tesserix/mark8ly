package payment

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/shopspring/decimal"
)

const (
	paypalLiveURL    = "https://api-m.paypal.com"
	paypalSandboxURL = "https://api-m.sandbox.paypal.com"
)

// PayPalGateway implements Gateway for PayPal using the REST API v2.
type PayPalGateway struct {
	apiKey    string // client ID
	secretKey string // client secret
	mode      string
	baseURL   string
	client    *http.Client

	mu          sync.Mutex
	accessToken string
	tokenExpiry time.Time
}

// NewPayPalGateway returns a PayPal Gateway ready for use.
func NewPayPalGateway(apiKey, secretKey, mode string) *PayPalGateway {
	base := paypalLiveURL
	if mode == "test" {
		base = paypalSandboxURL
	}
	return &PayPalGateway{
		apiKey:    apiKey,
		secretKey: secretKey,
		mode:      mode,
		baseURL:   base,
		client:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (p *PayPalGateway) ProviderName() string { return "paypal" }

func (p *PayPalGateway) SupportedCountries() []string {
	return []string{
		"US", "CA", "GB", "DE", "FR", "IT", "ES", "NL", "AU",
		"SG", "MY", "TH", "PH", "ID", "IN",
	}
}

// CreateIntent creates a PayPal order via POST /v2/checkout/orders.
func (p *PayPalGateway) CreateIntent(ctx context.Context, in CreateIntentInput) (*Intent, error) {
	token, err := p.ensureAccessToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("paypal: create intent: auth: %w", err)
	}

	orderBody := paypalOrderRequest{
		Intent: "CAPTURE",
		PurchaseUnits: []paypalPurchaseUnit{
			{
				ReferenceID: in.OrderID,
				Description: in.Description,
				Amount: paypalAmount{
					CurrencyCode: strings.ToUpper(in.CurrencyCode),
					Value:        in.Amount.StringFixed(2),
				},
			},
		},
		Payer: &paypalPayer{
			EmailAddress: in.CustomerEmail,
		},
	}

	payload, err := json.Marshal(orderBody)
	if err != nil {
		return nil, fmt.Errorf("paypal: create intent: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/v2/checkout/orders", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("paypal: create intent: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("paypal: create intent: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("paypal: create intent: read body: %w", err)
	}

	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("paypal: create intent: status %d: %s", resp.StatusCode, body)
	}

	var result struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Links  []struct {
			Href string `json:"href"`
			Rel  string `json:"rel"`
		} `json:"links"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("paypal: create intent: decode: %w", err)
	}

	// Extract the approval URL as the client token.
	var approvalURL string
	for _, link := range result.Links {
		if link.Rel == "approve" {
			approvalURL = link.Href
			break
		}
	}

	return &Intent{
		ProviderIntentID: result.ID,
		ClientToken:      approvalURL,
		Status:           result.Status,
	}, nil
}

// CapturePayment captures a PayPal order via POST /v2/checkout/orders/:id/capture.
func (p *PayPalGateway) CapturePayment(ctx context.Context, captureID string) (*Capture, error) {
	token, err := p.ensureAccessToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("paypal: capture payment: auth: %w", err)
	}

	endpoint := fmt.Sprintf("%s/v2/checkout/orders/%s/capture", p.baseURL, captureID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("paypal: capture payment: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("paypal: capture payment: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("paypal: capture payment: read body: %w", err)
	}

	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("paypal: capture payment: status %d: %s", resp.StatusCode, body)
	}

	var result struct {
		ID            string `json:"id"`
		Status        string `json:"status"`
		PurchaseUnits []struct {
			Payments struct {
				Captures []struct {
					ID string `json:"id"`
				} `json:"captures"`
			} `json:"payments"`
		} `json:"purchase_units"`
		Payer struct {
			PayerID string `json:"payer_id"`
		} `json:"payer"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("paypal: capture payment: decode: %w", err)
	}

	return &Capture{
		ProviderPaymentID: result.ID,
		Status:            result.Status,
		PaymentMethod:     "paypal",
	}, nil
}

// RefundPayment refunds a captured payment via POST /v2/payments/captures/:id/refund.
func (p *PayPalGateway) RefundPayment(ctx context.Context, in RefundInput) (*Refund, error) {
	token, err := p.ensureAccessToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("paypal: refund payment: auth: %w", err)
	}

	currency := in.CurrencyCode
	if currency == "" {
		currency = "USD"
	}
	refundBody := map[string]any{
		"amount": map[string]string{
			"currency_code": strings.ToUpper(currency),
			// PayPal requires the value's decimal places to match the
			// currency's exponent — "1000" for JPY, "10.00" for USD,
			// "10.000" for KWD. StringFixed(2) unconditionally 422s on
			// zero-decimal currencies.
			"value": in.Amount.StringFixed(currencyExponent(currency)),
		},
		"note_to_payer": in.Reason,
	}

	payload, err := json.Marshal(refundBody)
	if err != nil {
		return nil, fmt.Errorf("paypal: refund payment: marshal: %w", err)
	}

	endpoint := fmt.Sprintf("%s/v2/payments/captures/%s/refund", p.baseURL, in.ProviderPaymentID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("paypal: refund payment: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	if in.IdempotencyKey != "" {
		req.Header.Set("PayPal-Request-Id", in.IdempotencyKey)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("paypal: refund payment: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("paypal: refund payment: read body: %w", err)
	}

	if resp.StatusCode != http.StatusCreated {
		return nil, &GatewayError{Provider: "paypal", StatusCode: resp.StatusCode, Body: string(body)}
	}

	var result struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Amount struct {
			Value string `json:"value"`
		} `json:"amount"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("paypal: refund payment: decode: %w", err)
	}

	refundedAmount, _ := decimal.NewFromString(result.Amount.Value)

	return &Refund{
		ProviderRefundID: result.ID,
		Status:           result.Status,
		Amount:           refundedAmount,
	}, nil
}

// VerifyWebhook verifies a PayPal webhook by calling the PayPal webhook
// verification API endpoint. PayPal does not use a simple HMAC; instead
// it requires calling POST /v1/notifications/verify-webhook-signature.
func (p *PayPalGateway) VerifyWebhook(ctx context.Context, payload []byte, signature string) (*WebhookEvent, error) {
	token, err := p.ensureAccessToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("paypal: verify webhook: auth: %w", err)
	}

	// Parse the raw event first to extract the webhook_id.
	var rawEvent struct {
		ID        string          `json:"id"`
		EventType string          `json:"event_type"`
		Resource  json.RawMessage `json:"resource"`
	}
	if err := json.Unmarshal(payload, &rawEvent); err != nil {
		return nil, fmt.Errorf("paypal: verify webhook: decode event: %w", err)
	}

	// Build the verification request.
	// The signature parameter is expected to be a JSON-encoded object with
	// the header fields PayPal sent: transmission_id, transmission_time,
	// cert_url, auth_algo, transmission_sig, and the webhook_id.
	var sigHeaders paypalSignatureHeaders
	if err := json.Unmarshal([]byte(signature), &sigHeaders); err != nil {
		return nil, fmt.Errorf("paypal: verify webhook: decode signature headers: %w", err)
	}

	verifyBody := map[string]any{
		"auth_algo":         sigHeaders.AuthAlgo,
		"cert_url":          sigHeaders.CertURL,
		"transmission_id":   sigHeaders.TransmissionID,
		"transmission_sig":  sigHeaders.TransmissionSig,
		"transmission_time": sigHeaders.TransmissionTime,
		"webhook_id":        sigHeaders.WebhookID,
		"webhook_event":     json.RawMessage(payload),
	}

	verifyPayload, err := json.Marshal(verifyBody)
	if err != nil {
		return nil, fmt.Errorf("paypal: verify webhook: marshal verify: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.baseURL+"/v1/notifications/verify-webhook-signature",
		bytes.NewReader(verifyPayload))
	if err != nil {
		return nil, fmt.Errorf("paypal: verify webhook: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("paypal: verify webhook: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("paypal: verify webhook: read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("paypal: verify webhook: status %d: %s", resp.StatusCode, body)
	}

	var verifyResult struct {
		VerificationStatus string `json:"verification_status"`
	}
	if err := json.Unmarshal(body, &verifyResult); err != nil {
		return nil, fmt.Errorf("paypal: verify webhook: decode verify: %w", err)
	}

	if verifyResult.VerificationStatus != "SUCCESS" {
		return nil, fmt.Errorf("paypal: verify webhook: verification failed: %s", verifyResult.VerificationStatus)
	}

	// Extract resource details.
	var resource struct {
		ID            string        `json:"id"`
		Amount        *paypalAmount `json:"amount"`
		PurchaseUnits []struct {
			ReferenceID string       `json:"reference_id"`
			Amount      paypalAmount `json:"amount"`
		} `json:"purchase_units"`
	}
	_ = json.Unmarshal(rawEvent.Resource, &resource)

	evt := &WebhookEvent{
		ProviderEventID: rawEvent.ID,
		EventType:       normalizePayPalEvent(rawEvent.EventType),
		PaymentMethod:   "paypal",
		RawPayload:      payload,
	}

	evt.ProviderPaymentID = resource.ID

	if len(resource.PurchaseUnits) > 0 {
		pu := resource.PurchaseUnits[0]
		evt.OrderID = pu.ReferenceID
		evt.CurrencyCode = pu.Amount.CurrencyCode
		amt, _ := decimal.NewFromString(pu.Amount.Value)
		evt.Amount = amt
	} else if resource.Amount != nil {
		evt.CurrencyCode = resource.Amount.CurrencyCode
		amt, _ := decimal.NewFromString(resource.Amount.Value)
		evt.Amount = amt
	}

	return evt, nil
}

// ensureAccessToken returns a valid OAuth2 access token, refreshing if needed.
func (p *PayPalGateway) ensureAccessToken(ctx context.Context) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.accessToken != "" && time.Now().Before(p.tokenExpiry) {
		return p.accessToken, nil
	}

	form := url.Values{}
	form.Set("grant_type", "client_credentials")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.baseURL+"/v1/oauth2/token",
		strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("paypal: get token: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(p.apiKey, p.secretKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("paypal: get token: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("paypal: get token: read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("paypal: get token: status %d: %s", resp.StatusCode, body)
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("paypal: get token: decode: %w", err)
	}

	p.accessToken = tokenResp.AccessToken
	// Expire 60 seconds early to avoid edge-case expiry during a request.
	p.tokenExpiry = time.Now().Add(time.Duration(tokenResp.ExpiresIn-60) * time.Second)

	return p.accessToken, nil
}

func normalizePayPalEvent(eventType string) string {
	switch eventType {
	case "CHECKOUT.ORDER.APPROVED", "PAYMENT.CAPTURE.COMPLETED":
		return "payment.succeeded"
	case "PAYMENT.CAPTURE.DENIED":
		return "payment.failed"
	case "PAYMENT.CAPTURE.REFUNDED":
		return "refund.succeeded"
	default:
		return eventType
	}
}

// PayPal request/response types.

type paypalOrderRequest struct {
	Intent        string               `json:"intent"`
	PurchaseUnits []paypalPurchaseUnit `json:"purchase_units"`
	Payer         *paypalPayer         `json:"payer,omitempty"`
}

type paypalPurchaseUnit struct {
	ReferenceID string       `json:"reference_id"`
	Description string       `json:"description"`
	Amount      paypalAmount `json:"amount"`
}

type paypalAmount struct {
	CurrencyCode string `json:"currency_code"`
	Value        string `json:"value"`
}

type paypalPayer struct {
	EmailAddress string `json:"email_address"`
}

// paypalSignatureHeaders contains the header fields PayPal sends with webhooks
// that are needed for verification. The caller is expected to JSON-encode these
// and pass them as the signature string.
type paypalSignatureHeaders struct {
	AuthAlgo         string `json:"auth_algo"`
	CertURL          string `json:"cert_url"`
	TransmissionID   string `json:"transmission_id"`
	TransmissionSig  string `json:"transmission_sig"`
	TransmissionTime string `json:"transmission_time"`
	WebhookID        string `json:"webhook_id"`
}
