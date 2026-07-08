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
type Resolver struct{ db *gorm.DB }

// NewResolver constructs a Resolver bound to db.
func NewResolver(db *gorm.DB) *Resolver { return &Resolver{db: db} }

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
	return payment.NewGateway(provider, cfg.APIKey, cfg.SecretKey, cfg.Mode)
}
