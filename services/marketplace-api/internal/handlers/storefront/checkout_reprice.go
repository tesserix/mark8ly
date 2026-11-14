package storefront

import (
	"context"
	"fmt"

	"github.com/shopspring/decimal"

	"github.com/mark8ly/marketplace-api/internal/checkoutcore"
)

// repriceItems overwrites the client-supplied money on every cart line with
// the catalog's own values, in place.
//
// CheckoutItemRequest arrives entirely from the browser, unit_price included.
// The block that used to be the only "server-side" step recomputed the
// subtotal as unit_price * quantity — the client's price — so it validated the
// arithmetic and not the amount. A shopper could POST unit_price: 0.01 and
// legitimately pay a cent for anything in any store.
//
// Anything that cannot be priced from the catalog fails the checkout. There is
// deliberately no fallback to the request: a line we cannot price is a line we
// must not sell.
func repriceItems(ctx context.Context, p checkoutcore.CatalogPricer, storeID string, items []CheckoutItemRequest) error {
	for i := range items {
		it := &items[i]
		if it.VariantID == nil || *it.VariantID == "" {
			return fmt.Errorf("storefront: cart line %d has no variant_id, so it cannot be priced", i)
		}
		price, err := p.PriceFor(ctx, storeID, *it.VariantID)
		if err != nil {
			return fmt.Errorf("storefront: pricing cart line %d: %w", i, err)
		}
		it.UnitPrice = price.Amount
		it.LineTotal = price.Amount.Mul(decimal.NewFromInt(int64(it.Quantity)))
		it.CurrencyCode = price.CurrencyCode
		it.TaxCode = price.TaxCode
		it.TaxRateOverride = price.TaxRateOverride
		it.TaxCategory = price.TaxCategory
	}
	return nil
}
