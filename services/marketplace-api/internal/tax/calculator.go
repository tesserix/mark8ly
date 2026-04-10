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
type TaxableItem struct {
	ProductID string
	SKU       string
	Amount    decimal.Decimal
	Quantity  int
	HSNCode   string          // India GST — empty for other strategies
	GSTRate   decimal.Decimal // India GST — zero for other strategies
	TaxCode   string          // TaxJar product tax code — empty for non-US
}

// TaxBreakdown is the result of a tax calculation.
type TaxBreakdown struct {
	TaxTotal decimal.Decimal
	Lines    []TaxLine
}

// TaxLine is a single tax line item (one per jurisdiction/tax type).
type TaxLine struct {
	Description  string          // "VAT 20%", "CGST 9%", "CA State Tax"
	Rate         decimal.Decimal
	Amount       decimal.Decimal
	Jurisdiction string // "Maharashtra", "CA-Los Angeles", "" for flat
}
