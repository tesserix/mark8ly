package storefront

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
)

// A shopper controls every field of CheckoutItemRequest, unit_price included.
// Before this change the only "server-side" step recomputed the subtotal FROM
// that client price, so POSTing unit_price: 0.01 bought anything for a cent.
// These tests pin the catalog, not the request, as the source of truth.

type fakePricer struct {
	prices map[string]catalogPrice
	calls  []string
}

func (f *fakePricer) PriceFor(_ context.Context, storeID string, variantID string) (catalogPrice, error) {
	key := storeID + "|v:" + variantID
	f.calls = append(f.calls, key)
	p, ok := f.prices[key]
	if !ok {
		return catalogPrice{}, errCatalogItemNotFound
	}
	return p, nil
}

func strp(s string) *string { return &s }

func TestRepriceItems_OverridesTamperedUnitPrice(t *testing.T) {
	p := &fakePricer{prices: map[string]catalogPrice{
		"store-1|v:var-1": {Amount: decimal.RequireFromString("49.95"), CurrencyCode: "AUD"},
	}}
	items := []CheckoutItemRequest{{
		VariantID:    strp("var-1"),
		UnitPrice:    decimal.RequireFromString("0.01"), // tampered
		LineTotal:    decimal.RequireFromString("0.02"), // tampered
		Quantity:     2,
		CurrencyCode: "USD", // tampered
	}}

	if err := repriceItems(context.Background(), p, "store-1", items); err != nil {
		t.Fatalf("repriceItems: %v", err)
	}

	if got := items[0].UnitPrice.String(); got != "49.95" {
		t.Errorf("UnitPrice = %s, want 49.95 (the catalog price)", got)
	}
	if got := items[0].LineTotal.String(); got != "99.9" {
		t.Errorf("LineTotal = %s, want 99.9 (catalog price x quantity)", got)
	}
	if items[0].CurrencyCode != "AUD" {
		t.Errorf("CurrencyCode = %s, want AUD (the catalog currency)", items[0].CurrencyCode)
	}
}

func TestRepriceItems_ScopesLookupToTheStore(t *testing.T) {
	p := &fakePricer{prices: map[string]catalogPrice{
		"store-1|v:var-1": {Amount: decimal.NewFromInt(10), CurrencyCode: "AUD"},
	}}
	items := []CheckoutItemRequest{{VariantID: strp("var-1"), Quantity: 1}}

	if err := repriceItems(context.Background(), p, "store-1", items); err != nil {
		t.Fatalf("repriceItems: %v", err)
	}
	if len(p.calls) != 1 || !strings.HasPrefix(p.calls[0], "store-1|") {
		t.Fatalf("lookup calls = %v; the store must scope every lookup so a shopper cannot borrow another store's price", p.calls)
	}
}

func TestRepriceItems_RejectsItemNotInCatalog(t *testing.T) {
	p := &fakePricer{prices: map[string]catalogPrice{}}
	items := []CheckoutItemRequest{{VariantID: strp("ghost"), Quantity: 1}}

	err := repriceItems(context.Background(), p, "store-1", items)
	if err == nil {
		t.Fatal("want an error for an item with no catalog row; got nil")
	}
	if !errors.Is(err, errCatalogItemNotFound) {
		t.Errorf("err = %v, want errCatalogItemNotFound", err)
	}
}

func TestRepriceItems_RejectsItemWithNoVariant(t *testing.T) {
	// Price lives on product_variants only — `products` has no price column —
	// so a product-only line is not priceable and must not be sold.
	p := &fakePricer{prices: map[string]catalogPrice{}}
	for name, items := range map[string][]CheckoutItemRequest{
		"no ids at all":    {{Quantity: 1}},
		"product id only":  {{ProductID: strp("prod-1"), Quantity: 1}},
		"empty variant id": {{VariantID: strp(""), Quantity: 1}},
	} {
		if err := repriceItems(context.Background(), p, "store-1", items); err == nil {
			t.Errorf("%s: want an error; a line we cannot price is a line we must not sell", name)
		}
	}
	if len(p.calls) != 0 {
		t.Errorf("made %d catalog lookups for unpriceable items; want 0", len(p.calls))
	}
}

func TestRepriceItems_PricesFromTheVariantNotTheProduct(t *testing.T) {
	p := &fakePricer{prices: map[string]catalogPrice{
		"store-1|v:var-1": {Amount: decimal.NewFromInt(25), CurrencyCode: "AUD"},
	}}
	items := []CheckoutItemRequest{{
		ProductID: strp("prod-1"), VariantID: strp("var-1"), Quantity: 1,
	}}

	if err := repriceItems(context.Background(), p, "store-1", items); err != nil {
		t.Fatalf("repriceItems: %v", err)
	}
	if got := items[0].UnitPrice.String(); got != "25" {
		t.Errorf("UnitPrice = %s, want 25: the variant is the thing being bought", got)
	}
}

func TestRepriceItems_OverridesTamperedTaxFields(t *testing.T) {
	// tax_rate_override is client-supplied AND used: checkout_ext.go divides
	// it by 100 to get the applied rate. Sending 0.01 would shave tax to
	// nearly nothing, so it must come from the catalog like the price does.
	catRate := decimal.RequireFromString("10")
	p := &fakePricer{prices: map[string]catalogPrice{
		"store-1|v:var-1": {
			Amount:          decimal.NewFromInt(100),
			CurrencyCode:    "AUD",
			TaxRateOverride: &catRate,
			TaxCode:         strp("GST"),
		},
	}}
	tampered := decimal.RequireFromString("0.01")
	items := []CheckoutItemRequest{{
		VariantID:       strp("var-1"),
		Quantity:        1,
		TaxRateOverride: &tampered,
		TaxCode:         strp("ZERO"),
	}}

	if err := repriceItems(context.Background(), p, "store-1", items); err != nil {
		t.Fatalf("repriceItems: %v", err)
	}
	if items[0].TaxRateOverride == nil || items[0].TaxRateOverride.String() != "10" {
		t.Errorf("TaxRateOverride = %v, want 10 (the catalog rate)", items[0].TaxRateOverride)
	}
	if items[0].TaxCode == nil || *items[0].TaxCode != "GST" {
		t.Errorf("TaxCode = %v, want GST", items[0].TaxCode)
	}
}

func TestRepriceItems_ClearsTaxFieldsTheCatalogDoesNotSet(t *testing.T) {
	// A product with no override must not inherit one the shopper invented.
	p := &fakePricer{prices: map[string]catalogPrice{
		"store-1|v:var-1": {Amount: decimal.NewFromInt(100), CurrencyCode: "AUD"},
	}}
	tampered := decimal.RequireFromString("0.01")
	items := []CheckoutItemRequest{{
		VariantID: strp("var-1"), Quantity: 1, TaxRateOverride: &tampered,
	}}

	if err := repriceItems(context.Background(), p, "store-1", items); err != nil {
		t.Fatalf("repriceItems: %v", err)
	}
	if items[0].TaxRateOverride != nil {
		t.Errorf("TaxRateOverride = %v, want nil: the catalog sets none", items[0].TaxRateOverride)
	}
}
