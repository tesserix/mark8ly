package storefront

import (
	"context"
	"errors"
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/product"
	"github.com/mark8ly/marketplace-api/internal/stockhold"
)

// inventoryPolicyContinue means the merchant sells past zero on purpose.
const inventoryPolicyContinue = "continue"

// outOfStockError names the variant a shopper has to remove.
//
// It carries the id because a bare "out of stock" is unactionable on a cart
// of six items — the storefront needs to know which line to mark.
type outOfStockError struct {
	VariantID string
}

func (e outOfStockError) Error() string {
	return fmt.Sprintf("storefront: variant %s is out of stock", e.VariantID)
}

// stockLine is one checkout item that consumes inventory.
type stockLine struct {
	VariantID string
	Quantity  int
}

// commitStock reserves and consumes inventory for a checkout, INSIDE the
// order's transaction (#230).
//
// # Why it runs in the order transaction
//
// Decrementing after the order commits would leave a paid order whose stock
// was never taken when the decrement failed; decrementing before would strand
// stock when the order failed. order.CreateInput.WithinTx exists for exactly
// this shape — returning an error here rolls the order back entirely, which
// is the only correct outcome: the buyer can retry and nothing has moved.
//
// # Hold-then-commit, even for a cart that already holds
//
// stockhold.Hold is idempotent per (cart_token, variant, location) and its
// availability sum excludes the calling cart, so re-holding what this cart
// already reserved is free and does not double-count. That makes one code
// path serve both cases: a storefront that placed holds at cart-add, and any
// caller that did not. Checkout enforces availability either way, which is
// what stops #230's oversell independently of the UI.
//
// A hold placed at cart-add that has since EXPIRED simply fails here if the
// units are gone — which is correct, and is what the shopper is told.
func commitStock(
	ctx context.Context,
	tx *gorm.DB,
	holds *stockhold.Repository,
	cartToken string,
	lines []stockLine,
) error {
	if len(lines) == 0 {
		return nil
	}

	policies, err := inventoryPolicies(tx, lines)
	if err != nil {
		return err
	}

	for _, line := range lines {
		if policies[line.VariantID] == inventoryPolicyContinue {
			// Sell past zero on purpose. No hold, no gate — and the
			// decrement is clamped, because variant_stock carries a
			// non-negative CHECK (#231) that cannot be policy-aware: the
			// policy lives on product_variants, and a CHECK cannot read
			// another table.
			//
			// The consequence, stated rather than hidden: how far a
			// 'continue' variant is oversold is NOT tracked. The orders
			// are the record of what was sold; this column stops at zero.
			if err := tx.WithContext(ctx).Exec(
				`UPDATE variant_stock
				    SET quantity = GREATEST(quantity - ?, 0), updated_at = now()
				  WHERE variant_id = ? AND location_id = ?`,
				line.Quantity, line.VariantID, product.DefaultLocationID).Error; err != nil {
				return fmt.Errorf("storefront: decrement continue-policy variant: %w", err)
			}
			continue
		}

		err := holds.Hold(ctx, tx, cartToken, line.VariantID,
			product.DefaultLocationID, line.Quantity, HoldTTL)
		if errors.Is(err, stockhold.ErrInsufficientStock) {
			return outOfStockError{VariantID: line.VariantID}
		}
		if err != nil {
			return err
		}
	}

	// One Commit for the whole cart: it decrements every live hold this cart
	// holds and flips them to 'committed' in the same transaction.
	return holds.Commit(ctx, tx, cartToken)
}

// inventoryPolicies reads the policy for each line's variant in one query.
//
// A variant with no row is treated as 'deny' — the column's own default, and
// the safe reading: refusing to oversell something we cannot classify is
// recoverable, overselling it is not.
func inventoryPolicies(tx *gorm.DB, lines []stockLine) (map[string]string, error) {
	ids := make([]string, 0, len(lines))
	for _, l := range lines {
		ids = append(ids, l.VariantID)
	}

	var rows []struct {
		ID              string
		InventoryPolicy string
	}
	if err := tx.Raw(
		`SELECT id, COALESCE(inventory_policy, 'deny') AS inventory_policy
		   FROM product_variants WHERE id IN ?`, ids).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("storefront: read inventory policies: %w", err)
	}

	out := make(map[string]string, len(rows))
	for _, r := range rows {
		out[r.ID] = r.InventoryPolicy
	}
	return out, nil
}

// stockLinesFromItems selects the checkout items that consume inventory.
//
// An item with no variant_id is a custom or unstocked line — refusing those
// would break every unstocked sale, and there is nothing to decrement.
func stockLinesFromItems(items []CheckoutItemRequest) []stockLine {
	lines := make([]stockLine, 0, len(items))
	for _, it := range items {
		if it.VariantID == nil || *it.VariantID == "" || it.Quantity <= 0 {
			continue
		}
		lines = append(lines, stockLine{VariantID: *it.VariantID, Quantity: it.Quantity})
	}
	return lines
}

// cartTokenForCheckout returns the cart identity to commit holds against.
//
// A checkout arriving without one — an API client, or a storefront build
// predating #232 — gets a fresh token, so the hold-then-commit path still
// enforces availability for it. Enforcement must not depend on the caller
// having cooperated.
func cartTokenForCheckout(c *gin.Context) string {
	if ck, err := c.Cookie(CartTokenCookie); err == nil {
		if _, perr := uuid.Parse(ck); perr == nil {
			return ck
		}
	}
	return uuid.NewString()
}
