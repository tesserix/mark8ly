// Package orderrefund resolves the payment context (provider + refund
// target id + captured total) and payment gateway for an order, so refund
// flows can act on an order without callers hand-rolling the underlying
// payment_transactions / payment_gateway_configs lookups.
package orderrefund

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/carriersecrets"
	"github.com/mark8ly/marketplace-api/internal/crypto"
	"github.com/mark8ly/marketplace-api/internal/payment"
)

// PaymentContext is what a refund flow needs to know about an order's
// captured payment: which provider processed it, the id to refund against,
// and the total amount actually captured (across possibly multiple
// captured payment_transactions rows for the order).
type PaymentContext struct {
	Provider          string
	ProviderPaymentID string
	CapturedTotal     decimal.Decimal
	Found             bool
}

// Resolver reads payment_transactions and payment_gateway_configs to
// answer "what did this order pay with, and how do we talk to that
// gateway."
type Resolver struct {
	db *gorm.DB
	// secretStore resolves the credential references stored on
	// payment_gateway_configs. The columns are named *_encrypted but hold
	// an opaque reference — "gsm://..." once rewrapped, legacy inline
	// "aes:" ciphertext before that — so they are NEVER usable as-is.
	secretStore carriersecrets.Store
	// encryptor is the fallback for inline-mode deployments with no Store.
	encryptor crypto.Encryptor
}

// NewResolver constructs a Resolver bound to db.
//
// Wire WithSecretStore (or WithEncryptor) before use: without one,
// GatewayFor cannot turn the stored references into real credentials and
// refunds fail at the gateway.
func NewResolver(db *gorm.DB) *Resolver { return &Resolver{db: db} }

// WithSecretStore wires the credential resolver. Chainable.
func (r *Resolver) WithSecretStore(s carriersecrets.Store) *Resolver {
	r.secretStore = s
	return r
}

// WithEncryptor wires the inline-mode fallback. Chainable.
func (r *Resolver) WithEncryptor(e crypto.Encryptor) *Resolver {
	r.encryptor = e
	return r
}

// resolveCred turns a stored credential reference into plaintext. Mirrors
// the Store-first / Encryptor-fallback pattern used by the checkout path
// (handlers/storefront/checkout_ext.go resolveCred).
func (r *Resolver) resolveCred(ctx context.Context, ref string) (string, error) {
	if ref == "" {
		return "", nil
	}
	if r.secretStore != nil {
		return r.secretStore.Get(ctx, ref)
	}
	if r.encryptor == nil {
		// Returning ref here would send "gsm://projects/..." to the gateway
		// as an API key — which is exactly the 401 this guard prevents.
		return "", fmt.Errorf("orderrefund: no secret store or encryptor wired — cannot resolve gateway credentials")
	}
	return r.encryptor.Decrypt(ref)
}

// PaymentContextForOrder returns the captured payment for an order. Only rows
// with status='captured' count (that is what the capture webhook / client-verify
// writes; authorized-only or pending orders are non-refundable → Found=false).
//
// ProviderPaymentID is the id we refund against. It must be the CAPTURED payment
// id — Stripe refunds the payment_intent, Razorpay the payment id (pay_...),
// PayPal the capture id. That id lives in provider_payment_id (persisted at
// capture — see the capture-handler fix in this task). We prefer
// provider_payment_id and fall back to provider_intent_id for legacy rows
// captured before that fix (Stripe rows, where intent == the refund target).
func (r *Resolver) PaymentContextForOrder(ctx context.Context, orderID uuid.UUID) (PaymentContext, error) {
	type row struct {
		Provider          string
		ProviderPaymentID string
		ProviderIntentID  string
		Amount            decimal.Decimal
	}
	var rows []row
	if err := r.db.WithContext(ctx).
		Table("payment_transactions").
		Select("provider", "provider_payment_id", "provider_intent_id", "amount").
		Where("order_id = ? AND status = 'captured'", orderID).
		Order("created_at ASC").
		Scan(&rows).Error; err != nil {
		return PaymentContext{}, err
	}
	if len(rows) == 0 {
		return PaymentContext{Found: false}, nil
	}
	total := decimal.Zero
	for _, x := range rows {
		total = total.Add(x.Amount)
	}
	refundTarget := rows[0].ProviderPaymentID
	if refundTarget == "" {
		refundTarget = rows[0].ProviderIntentID
	}
	return PaymentContext{
		Provider:          rows[0].Provider,
		ProviderPaymentID: refundTarget,
		CapturedTotal:     total,
		Found:             true,
	}, nil
}

// gatewayConfigRow is a read-only projection of payment_gateway_configs
// used to instantiate the correct payment.Gateway for a store+provider.
type gatewayConfigRow struct {
	Provider  string `gorm:"column:provider"`
	APIKey    string `gorm:"column:api_key_encrypted"`
	SecretKey string `gorm:"column:secret_key_encrypted"`
	Mode      string `gorm:"column:mode"`
}

func (gatewayConfigRow) TableName() string { return "payment_gateway_configs" }

// GatewayFor looks up the active payment_gateway_configs row for
// (storeID, provider) and constructs the matching payment.Gateway.
func (r *Resolver) GatewayFor(ctx context.Context, storeID uuid.UUID, provider string) (payment.Gateway, error) {
	var cfg gatewayConfigRow
	if err := r.db.WithContext(ctx).
		Where("store_id = ? AND provider = ? AND is_active = true", storeID, provider).
		First(&cfg).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("orderrefund: no active %q gateway config for store %s", provider, storeID)
		}
		return nil, err
	}
	// api_key_encrypted / secret_key_encrypted hold references, not
	// credentials. Passing them straight to payment.NewGateway sent
	// Razorpay the literal "gsm://projects/..." string as its API key and
	// every refund came back 401 Authentication failed.
	apiKey, err := r.resolveCred(ctx, cfg.APIKey)
	if err != nil {
		return nil, fmt.Errorf("orderrefund: resolve %s api_key: %w", provider, err)
	}
	secretKey, err := r.resolveCred(ctx, cfg.SecretKey)
	if err != nil {
		return nil, fmt.Errorf("orderrefund: resolve %s secret_key: %w", provider, err)
	}
	return payment.NewGateway(provider, apiKey, secretKey, cfg.Mode)
}
