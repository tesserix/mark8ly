package giftcard

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

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

	gw, err := payment.NewGateway(provider, cfg.APIKey, cfg.SecretKey, cfg.Mode)
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
