package storefront

import (
	"context"
	"errors"
	"fmt"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// errCatalogItemNotFound means a cart line named a product or variant that
// does not exist in this store. It must fail the checkout, never fall back to
// the client's price.
var errCatalogItemNotFound = errors.New("storefront: cart item not found in this store's catalog")

// catalogPrice is the authoritative money for one cart line: the variant's
// price, plus the tax classification that comes from its product.
//
// The tax fields are here for the same reason as the price. tax_rate_override
// is also client-supplied on CheckoutItemRequest and is actually used —
// checkout_ext.go divides it by 100 to get the applied rate — so a shopper
// could send 0.01 and pay almost no tax. dto.go:213 shows the real source is
// the product row.
type catalogPrice struct {
	Amount          decimal.Decimal
	CurrencyCode    string
	TaxCode         *string
	TaxRateOverride *decimal.Decimal
	TaxCategory     *string
}

// catalogPricer resolves the authoritative price for a cart line. Every
// implementation MUST scope its lookup to storeID — otherwise a shopper can
// name another store's cheaper variant and have it honoured here.
//
// Price lives on product_variants only: `products` has no price column, and
// the storefront surfaces a product as a min/max range across its variants
// (see dto.go StorefrontPriceRange). So a variant id is required to price a
// line, and the storefront cart always carries one (cart.ts: variantId is
// non-optional).
type catalogPricer interface {
	PriceFor(ctx context.Context, storeID string, variantID string) (catalogPrice, error)
}

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
func repriceItems(ctx context.Context, p catalogPricer, storeID string, items []CheckoutItemRequest) error {
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

// dbCatalogPricer reads prices from products / product_variants.
type dbCatalogPricer struct{ db *gorm.DB }

// PriceFor reads the variant's own price and its product's tax
// classification, scoped by store_id so a shopper cannot name another store's
// cheaper variant, and respecting soft-delete on both rows so a removed
// product or variant cannot be bought by id.
func (d dbCatalogPricer) PriceFor(ctx context.Context, storeID string, variantID string) (catalogPrice, error) {
	var row struct {
		Price           decimal.Decimal
		CurrencyCode    string
		TaxCode         *string
		TaxRateOverride *decimal.Decimal
		TaxCategory     *string
	}
	err := d.db.WithContext(ctx).
		Table("product_variants AS v").
		Select("v.price, v.currency_code, p.tax_code, p.tax_rate_override, p.tax_category").
		Joins("JOIN products AS p ON p.id = v.product_id AND p.deleted_at IS NULL").
		Where("v.id = ? AND v.store_id = ? AND v.deleted_at IS NULL", variantID, storeID).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return catalogPrice{}, errCatalogItemNotFound
	}
	if err != nil {
		return catalogPrice{}, err
	}
	return catalogPrice{
		Amount:          row.Price,
		CurrencyCode:    row.CurrencyCode,
		TaxCode:         row.TaxCode,
		TaxRateOverride: row.TaxRateOverride,
		TaxCategory:     row.TaxCategory,
	}, nil
}
