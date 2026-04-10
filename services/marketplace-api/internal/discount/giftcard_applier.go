package discount

import (
	"context"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/giftcard"
)

// GiftCardApplier implements Applier by debiting a gift card.
// Amendment CRITICAL FIX 1: uses the caller's tx, does NOT open its own.
// Amendment HIGH FIX 5: handles partial debit via min(balance, amount).
type GiftCardApplier struct {
	Svc      *giftcard.Service
	CardID   uuid.UUID
	TenantID uuid.UUID
	Balance  decimal.Decimal // pre-fetched balance for partial debit calc
}

// Apply debits min(Balance, amount) from the gift card inside the provided tx.
// Returns the amount actually deducted (may be less than amount for partial).
func (a *GiftCardApplier) Apply(ctx context.Context, tx *gorm.DB, in ApplyInput) (ApplyResult, error) {
	// Amendment HIGH FIX 5: compute partial debit amount.
	debitAmount := in.Subtotal
	if a.Balance.LessThan(debitAmount) {
		debitAmount = a.Balance
	}

	if debitAmount.LessThanOrEqual(decimal.Zero) {
		return ApplyResult{}, nil
	}

	_, err := a.Svc.Debit(tx, a.CardID, debitAmount, in.OrderID, in.TenantID)
	if err != nil {
		return ApplyResult{}, err
	}

	return ApplyResult{
		DiscountAmount: debitAmount,
		Description:    "Gift card applied",
	}, nil
}
