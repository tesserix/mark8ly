package giftcard

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/carriersecrets"
	"github.com/mark8ly/marketplace-api/internal/crypto"
	"github.com/mark8ly/marketplace-api/internal/payment"
)

// paymentGatewayConfigRow is a read-only projection of the
// payment_gateway_configs table. Mirrors the row shape used by the
// product checkout flow so gift card purchases hit the same credentials
// the store already configured (admin → Settings → Payments).
type paymentGatewayConfigRow struct {
	Provider  string `gorm:"column:provider"`
	APIKey    string `gorm:"column:api_key_encrypted"`
	SecretKey string `gorm:"column:secret_key_encrypted"`
	Mode      string `gorm:"column:mode"`
	IsActive  bool   `gorm:"column:is_active"`
}

func (paymentGatewayConfigRow) TableName() string { return "payment_gateway_configs" }

// DBGatewayResolver looks up the store's configured payment gateway from
// payment_gateway_configs. Uses the SAME credentials the store uses for
// product checkout — no separate gift card payment config.
type DBGatewayResolver struct {
	db *gorm.DB
	// secretStore resolves the credential references held by the
	// *_encrypted columns ("gsm://..." once rewrapped, legacy inline
	// "aes:" before). They are never usable as credentials as-is.
	secretStore carriersecrets.Store
	// encryptor is the fallback for inline-mode deployments.
	encryptor crypto.Encryptor
}

// WithSecretStore wires the credential resolver. Chainable.
func (r *DBGatewayResolver) WithSecretStore(s carriersecrets.Store) *DBGatewayResolver {
	r.secretStore = s
	return r
}

// WithEncryptor wires the inline-mode fallback. Chainable.
func (r *DBGatewayResolver) WithEncryptor(e crypto.Encryptor) *DBGatewayResolver {
	r.encryptor = e
	return r
}

// resolveCred turns a stored credential reference into plaintext. Same
// Store-first / Encryptor-fallback contract as the checkout and refund
// paths — see handlers/storefront/checkout_ext.go resolveCred.
func (r *DBGatewayResolver) resolveCred(ctx context.Context, ref string) (string, error) {
	if ref == "" {
		return "", nil
	}
	if r.secretStore != nil {
		return r.secretStore.Get(ctx, ref)
	}
	if r.encryptor == nil {
		// Returning ref would hand "gsm://projects/..." to the gateway as an
		// API key — a 401 with no usable diagnostic. Fail loudly instead.
		return "", fmt.Errorf("giftcard: no secret store or encryptor wired — cannot resolve gateway credentials")
	}
	return r.encryptor.Decrypt(ref)
}

// NewDBGatewayResolver constructs the resolver.
func NewDBGatewayResolver(db *gorm.DB) *DBGatewayResolver {
	return &DBGatewayResolver{db: db}
}

// ResolveCheckoutGateway returns a CheckoutCapableGateway adapter wrapping
// the store's active payment provider config. Currently Stripe is the
// only provider that supports hosted checkout sessions.
func (r *DBGatewayResolver) ResolveCheckoutGateway(ctx context.Context, storeID uuid.UUID, provider string) (CheckoutCapableGateway, error) {
	if provider == "" {
		provider = "stripe"
	}

	var cfg paymentGatewayConfigRow
	if err := r.db.WithContext(ctx).
		Where("store_id = ? AND provider = ? AND is_active = true", storeID, provider).
		First(&cfg).Error; err != nil {
		return nil, fmt.Errorf("giftcard: no active payment config for %q: %w", provider, err)
	}

	// The *_encrypted columns hold references, not credentials — passing
	// them through verbatim is what made refunds 401 (see orderrefund).
	apiKey, err := r.resolveCred(ctx, cfg.APIKey)
	if err != nil {
		return nil, fmt.Errorf("giftcard: resolve %s api_key: %w", provider, err)
	}
	secretKey, err := r.resolveCred(ctx, cfg.SecretKey)
	if err != nil {
		return nil, fmt.Errorf("giftcard: resolve %s secret_key: %w", provider, err)
	}

	gw, err := payment.NewGateway(provider, apiKey, secretKey, cfg.Mode)
	if err != nil {
		return nil, fmt.Errorf("giftcard: gateway init: %w", err)
	}

	checkoutGW, ok := gw.(payment.CheckoutGateway)
	if !ok {
		return nil, fmt.Errorf("giftcard: provider %q does not support hosted checkout sessions", provider)
	}

	return &paymentCheckoutAdapter{gw: gw, checkout: checkoutGW}, nil
}

// paymentCheckoutAdapter adapts the `payment.CheckoutGateway` interface
// to giftcard.CheckoutCapableGateway. We do the mapping here so the
// giftcard package doesn't import the payment package's wire types.
type paymentCheckoutAdapter struct {
	gw       payment.Gateway
	checkout payment.CheckoutGateway
}

func (a *paymentCheckoutAdapter) ProviderName() string {
	return a.gw.ProviderName()
}

func (a *paymentCheckoutAdapter) CreateCheckoutSession(ctx context.Context, in CheckoutSessionInput) (*CheckoutSessionOutput, error) {
	session, err := a.checkout.CreateCheckoutSession(ctx, payment.CreateCheckoutSessionInput{
		ReferenceID:   in.ReferenceID,
		Amount:        in.Amount,
		CurrencyCode:  in.CurrencyCode,
		CustomerEmail: in.CustomerEmail,
		Description:   in.Description,
		Name:          in.Name,
		SuccessURL:    in.SuccessURL,
		CancelURL:     in.CancelURL,
		Metadata:      in.Metadata,
	})
	if err != nil {
		return nil, err
	}
	return &CheckoutSessionOutput{ID: session.ID, URL: session.URL}, nil
}
