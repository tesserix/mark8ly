// Package instore records sales rung up at a counter or market stall and
// paid outside mark8ly (#1103). A sale is an ordinary order: priced from
// the catalogue, taxed like checkout, and committed together with its stock
// decrement, so in-person and online sales share one stock count.
package instore

import (
	"context"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/mark8ly/marketplace-api/internal/country"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/internal/tax"
)

// Payment methods. Nothing is charged through mark8ly; these record how
// the customer paid.
const (
	PaymentCard     = "card_external" // the seller's own EFTPOS terminal
	PaymentCash     = "cash"
	PaymentTransfer = "bank_transfer"
	PaymentOther    = "other" // e.g. UPI; needs a label
)

// SaleLine is a catalogue line (VariantID set) or a custom line
// (CustomTitle and CustomPrice set, no stock). A catalogue line carries no
// price: the catalogue supplies it.
type SaleLine struct {
	VariantID   *string
	Quantity    int
	CustomTitle *string
	CustomPrice *decimal.Decimal
}

// SaleInput is one sale as the counter screen submits it.
type SaleInput struct {
	TenantID           uuid.UUID
	StoreID            uuid.UUID
	Store              *stores.Store
	IdempotencyKey     string
	Lines              []SaleLine
	Discount           decimal.Decimal
	PaymentMethod      string
	PaymentMethodLabel *string
	CashTendered       *decimal.Decimal
	Note               *string
	CustomerEmail      string // "" for no receipt email
	SoldByUserID       string
	// CanDiscount: staff and above, or a cashier when the store allows it.
	// Covers both discounts and custom lines.
	CanDiscount bool
	// Fulfilled: handed over at the counter. False for made-to-order.
	Fulfilled bool
}

// VariantDetails is what the order snapshots about a catalogue line.
type VariantDetails struct {
	ProductID     string
	Title         string
	SKU           string
	OptionSummary *string
	ImageURL      *string
}

// DetailsLookup reads VariantDetails for live variants of one store.
type DetailsLookup interface {
	Details(ctx context.Context, storeID string, variantIDs []string) (map[string]VariantDetails, error)
}

// TaxQuoter taxes a sale made at the seller's own counter. Satisfied by the
// storefront checkout handler, which owns the tax credentials and the
// seller's tax region.
type TaxQuoter interface {
	QuoteCounterTax(ctx context.Context, store *stores.Store, sc *country.SupportedCountry, items []tax.TaxableItem) (*tax.TaxBreakdown, error)
}
