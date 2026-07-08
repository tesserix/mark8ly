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

// CheckoutGateway is an optional capability interface for providers
// that can host their own payment page (Stripe Checkout, Razorpay hosted
// checkout). The storefront gift-card purchase flow uses this to avoid
// embedding provider-specific JS; the customer is redirected to the
// provider's page, pays, and is redirected back.
//
// Not every provider implements this — use type assertion at call sites.
type CheckoutGateway interface {
	CreateCheckoutSession(ctx context.Context, in CreateCheckoutSessionInput) (*CheckoutSession, error)
}

// CreateCheckoutSessionInput describes a hosted checkout session.
type CreateCheckoutSessionInput struct {
	ReferenceID   string            // our internal id (e.g. gift card uuid) — passed as metadata
	Amount        decimal.Decimal
	CurrencyCode  string
	CustomerEmail string
	Description   string
	Name          string            // short product-style label shown on the hosted page
	SuccessURL    string            // where the provider redirects on success (may include {CHECKOUT_SESSION_ID})
	CancelURL     string            // where the provider redirects on cancel
	Metadata      map[string]string // arbitrary metadata (surfaced on webhook)
}

// CheckoutSession is what a hosted-checkout provider returns.
type CheckoutSession struct {
	ID  string // provider-side session id (webhook idempotency key)
	URL string // public URL to redirect the customer to
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
	CurrencyCode      string
	Reason            string
	IdempotencyKey    string // provider idempotency key; retries with the same key never double-refund
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
	EventType       string // "payment.succeeded", "payment.failed", "refund.succeeded", "checkout.completed"
	OrderID         string
	// Arbitrary provider metadata surfaced to the handler. For Stripe
	// Checkout sessions + PaymentIntents, we propagate the `metadata`
	// map so downstream handlers (gift card webhook, etc.) can correlate
	// via custom keys like `gift_card_id`.
	Metadata        map[string]string
	// ProviderPaymentID is the payment intent / charge id (Stripe) or
	// equivalent — useful when the event originated from a session and
	// the caller needs to record the settled payment intent separately.
	ProviderPaymentID string
	// SessionID is the provider hosted-checkout session id (Stripe
	// checkout.session.*). Empty for direct intent events.
	SessionID string
	Amount          decimal.Decimal
	CurrencyCode    string
	PaymentMethod   string
	RawPayload      []byte
}
