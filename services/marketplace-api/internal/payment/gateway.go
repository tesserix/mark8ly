// Package payment defines the payment gateway provider abstraction.
// Concrete implementations (Stripe, Razorpay, PayPal) live in separate
// files and are wired in P2.
package payment

import (
	"context"

	"github.com/shopspring/decimal"
)

// Gateway is the interface every payment provider must implement.
type Gateway interface {
	CreateIntent(ctx context.Context, in CreateIntentInput) (*Intent, error)
	CapturePayment(ctx context.Context, captureID string) (*Capture, error)
	RefundPayment(ctx context.Context, in RefundInput) (*Refund, error)
	VerifyWebhook(ctx context.Context, payload []byte, signature string) (*WebhookEvent, error)
	ProviderName() string
	SupportedCountries() []string
}

// CreateIntentInput describes what the checkout handler sends to the
// payment provider to initiate a payment.
type CreateIntentInput struct {
	OrderID       string
	Amount        decimal.Decimal
	CurrencyCode  string
	CustomerEmail string
	Description   string
	Metadata      map[string]string
}

// Intent is the provider's response to CreateIntent. ClientToken is the
// value the storefront JS SDK needs: Stripe's client_secret, Razorpay's
// order_id, or PayPal's approval_url.
type Intent struct {
	ProviderIntentID string
	ClientToken      string
	Status           string
}

// Capture represents a successful payment capture confirmation.
type Capture struct {
	ProviderPaymentID string
	Status            string
	PaymentMethod     string
}

// RefundInput describes a refund request.
type RefundInput struct {
	ProviderPaymentID string
	Amount            decimal.Decimal
	Reason            string
}

// Refund is the provider's response to a refund request.
type Refund struct {
	ProviderRefundID string
	Status           string
	Amount           decimal.Decimal
}

// WebhookEvent is the normalized event parsed from a provider webhook
// callback. The handler uses ProviderEventID for idempotency.
type WebhookEvent struct {
	ProviderEventID string
	EventType       string // "payment.succeeded", "payment.failed", "refund.succeeded"
	OrderID         string
	Amount          decimal.Decimal
	CurrencyCode    string
	PaymentMethod   string
	RawPayload      []byte
}
