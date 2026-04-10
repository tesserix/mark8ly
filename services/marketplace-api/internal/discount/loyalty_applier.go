package discount

import (
	"context"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/loyalty"
)

// LoyaltyApplier implements Applier for loyalty point redemption.
// Amendment FIX 1: Apply uses the tx passed in (the order creation
// transaction), NOT its own transaction.
type LoyaltyApplier struct {
	svc           *loyalty.Service
	tenantID      string
	storeID       string
	customerEmail string
	points        int
}

// NewLoyaltyApplier constructs a LoyaltyApplier.
func NewLoyaltyApplier(svc *loyalty.Service, tenantID, storeID, email string, points int) *LoyaltyApplier {
	return &LoyaltyApplier{
		svc:           svc,
		tenantID:      tenantID,
		storeID:       storeID,
		customerEmail: email,
		points:        points,
	}
}

// Apply redeems the specified points within the caller's transaction
// and returns the monetary discount. The caller (checkout_ext.go) owns
// the tx lifecycle — this method must NOT commit or rollback.
func (a *LoyaltyApplier) Apply(ctx context.Context, tx *gorm.DB, in ApplyInput) (ApplyResult, error) {
	value, err := a.svc.RedeemPointsTx(ctx, tx, in.TenantID, in.StoreID, a.customerEmail, a.points, &in.OrderID)
	if err != nil {
		return ApplyResult{}, err
	}
	// Cap at the remaining order subtotal
	if value.GreaterThan(in.Subtotal) {
		value = in.Subtotal
	}
	return ApplyResult{
		DiscountAmount: value,
		Description:    decimal.NewFromInt(int64(a.points)).String() + " loyalty points redeemed",
	}, nil
}
