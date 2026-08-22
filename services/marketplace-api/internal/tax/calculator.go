// Package tax defines the tax calculator provider abstraction.
// Concrete implementations (flat rate, India GST, TaxJar) live in
// separate files and are wired in P4.
package tax

import (
	"context"

	"github.com/shopspring/decimal"
)

// Calculator is the interface every tax strategy must implement.
type Calculator interface {
	Calculate(ctx context.Context, in TaxRequest) (*TaxBreakdown, error)
	ProviderName() string
}

// TaxRequest contains all information needed to calculate tax for an order.
type TaxRequest struct {
	StoreCountryCode string
	SellerAddress    Address
	BuyerAddress     Address
	Items            []TaxableItem
	ShippingAmount   decimal.Decimal
	CurrencyCode     string
}

// Address represents a physical address for tax jurisdiction resolution.
type Address struct {
	Line1       string
	City        string
	Region      string
	PostalCode  string
	CountryCode string
}

// TaxableItem describes a single line item for tax calculation.
// Tax fields are interpreted by the active strategy:
//   - india_gst: TaxCode = HSN code; TaxRate = GST percentage as a
//     fraction (e.g. 0.18 for 18%); TaxCategory == "exempt" forces 0.
//   - flat_rate: TaxRate overrides the country default when set;
//     TaxCategory drives tier selection (standard / reduced /
//     zero_rated / exempt).
//   - taxjar: TaxCode is passed as product_tax_code; TaxRate and
//     TaxCategory are ignored (TaxJar resolves the rate server-side).
type TaxableItem struct {
	ProductID   string
	SKU         string
	Amount      decimal.Decimal
	Quantity    int
	TaxCode     string
	TaxRate     decimal.Decimal // fraction (0.18 = 18%)
	TaxCategory string          // "" | "standard" | "reduced" | "zero_rated" | "exempt"
}

// Tax category constants. Empty string is treated as TaxCategoryStandard.
const (
	TaxCategoryStandard  = "standard"
	TaxCategoryReduced   = "reduced"
	TaxCategoryZeroRated = "zero_rated"
	TaxCategoryExempt    = "exempt"
)

// IsExempt returns true when the line item is flagged exempt or
// zero-rated and should be excluded from tax calculation.
func (t TaxableItem) IsExempt() bool {
	return t.TaxCategory == TaxCategoryExempt ||
		t.TaxCategory == TaxCategoryZeroRated
}

// TaxBreakdown is the result of a tax calculation.
type TaxBreakdown struct {
	TaxTotal decimal.Decimal
	Lines    []TaxLine
}

// TaxLine is a single tax line item (one per jurisdiction/tax type).
type TaxLine struct {
	Description  string // "VAT 20%", "CGST 9%", "CA State Tax"
	Rate         decimal.Decimal
	Amount       decimal.Decimal
	Jurisdiction string // "Maharashtra", "CA-Los Angeles", "" for flat
}
