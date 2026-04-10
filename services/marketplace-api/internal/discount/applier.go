// Package discount defines the shared discount application interface.
// Coupon, gift card, and loyalty redemption each implement Applier.
// The checkout handler iterates []Applier in order inside a single
// DB transaction.
package discount

import (
	"context"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// ApplyInput contains the context needed to calculate and record a discount.
type ApplyInput struct {
	TenantID      uuid.UUID
	StoreID       uuid.UUID
	OrderID       uuid.UUID
	CustomerEmail string
	Subtotal      decimal.Decimal // pre-discount subtotal
	CurrencyCode  string
}

// ApplyResult contains the outcome of applying a discount.
type ApplyResult struct {
	DiscountAmount decimal.Decimal // amount deducted from the order
	Description    string          // human-readable label, e.g. "SAVE20 — 20% off"
}

// Applier applies a discount to an order inside the provided transaction.
// The caller (checkout_ext.go) owns the tx lifecycle — Applier must NOT
// commit or rollback. Returns (zero result, nil) when the discount does
// not apply (e.g. free_shipping on a digital-only order).
type Applier interface {
	Apply(ctx context.Context, tx *gorm.DB, in ApplyInput) (ApplyResult, error)
}
