package tax

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"
)

// helper to build a TaxableItem with sensible defaults.
func item(amount string, qty int) TaxableItem {
	return TaxableItem{
		SKU:      "SKU",
		Amount:   decimal.RequireFromString(amount),
		Quantity: qty,
	}
}

// ── India GST ───────────────────────────────────────────────────────────

func TestIndiaGST_IntraState_18pct_SplitsCGSTSGST(t *testing.T) {
	calc := NewIndiaGSTCalculator()
	it := item("100", 2) // line total = 200
	it.TaxRate = decimal.RequireFromString("0.18")

	req := TaxRequest{
		StoreCountryCode: "IN",
		SellerAddress:    Address{Region: "Maharashtra"},
		BuyerAddress:     Address{Region: "Maharashtra"},
		Items:            []TaxableItem{it},
	}

	got, err := calc.Calculate(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	// Expect CGST 9% = 18.00 and SGST 9% = 18.00 → total 36.00.
	want := decimal.RequireFromString("36")
	if !got.TaxTotal.Equal(want) {
		t.Errorf("TaxTotal = %s, want %s", got.TaxTotal, want)
	}
	if len(got.Lines) != 2 {
		t.Errorf("expected 2 tax lines (CGST, SGST), got %d", len(got.Lines))
	}
}

func TestIndiaGST_InterState_28pct_SingleIGST(t *testing.T) {
	calc := NewIndiaGSTCalculator()
	it := item("500", 1)
	it.TaxRate = decimal.RequireFromString("0.28")

	req := TaxRequest{
		StoreCountryCode: "IN",
		SellerAddress:    Address{Region: "Maharashtra"},
		BuyerAddress:     Address{Region: "Karnataka"},
		Items:            []TaxableItem{it},
	}

	got, err := calc.Calculate(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	want := decimal.RequireFromString("140") // 500 * 0.28
	if !got.TaxTotal.Equal(want) {
		t.Errorf("TaxTotal = %s, want %s", got.TaxTotal, want)
	}
	if len(got.Lines) != 1 || got.Lines[0].Description == "" {
		t.Errorf("expected 1 IGST line, got %+v", got.Lines)
	}
}

func TestIndiaGST_Exempt_ProducesZeroTax(t *testing.T) {
	calc := NewIndiaGSTCalculator()
	it := item("999", 3)
	it.TaxRate = decimal.RequireFromString("0.18")
	it.TaxCategory = TaxCategoryExempt

	got, err := calc.Calculate(context.Background(), TaxRequest{
		StoreCountryCode: "IN",
		Items:            []TaxableItem{it},
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !got.TaxTotal.IsZero() {
		t.Errorf("exempt item should produce 0 tax, got %s", got.TaxTotal)
	}
}

func TestIndiaGST_ZeroRate_SkipsLine(t *testing.T) {
	calc := NewIndiaGSTCalculator()
	it := item("42", 1) // no TaxRate set
	got, err := calc.Calculate(context.Background(), TaxRequest{
		Items: []TaxableItem{it},
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !got.TaxTotal.IsZero() {
		t.Errorf("expected 0 tax for zero-rate item, got %s", got.TaxTotal)
	}
}

// ── Flat rate ───────────────────────────────────────────────────────────

func TestFlatRate_UsesCountryDefault_WhenNoOverride(t *testing.T) {
	calc := NewFlatCalculator(decimal.RequireFromString("0.20"), "VAT")
	got, err := calc.Calculate(context.Background(), TaxRequest{
		Items: []TaxableItem{item("100", 1)}, // 100 @ 20% = 20.00
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !got.TaxTotal.Equal(decimal.RequireFromString("20")) {
		t.Errorf("TaxTotal = %s, want 20", got.TaxTotal)
	}
}

func TestFlatRate_ItemOverride_WinsOverDefault(t *testing.T) {
	calc := NewFlatCalculator(decimal.RequireFromString("0.20"), "VAT")
	reduced := item("100", 1)
	reduced.TaxRate = decimal.RequireFromString("0.05") // reduced rate override

	got, err := calc.Calculate(context.Background(), TaxRequest{
		Items: []TaxableItem{reduced},
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !got.TaxTotal.Equal(decimal.RequireFromString("5")) {
		t.Errorf("TaxTotal = %s, want 5 (override)", got.TaxTotal)
	}
}

func TestFlatRate_Exempt_Excluded(t *testing.T) {
	calc := NewFlatCalculator(decimal.RequireFromString("0.20"), "VAT")
	taxable := item("100", 1)
	exempt := item("50", 1)
	exempt.TaxCategory = TaxCategoryExempt

	got, err := calc.Calculate(context.Background(), TaxRequest{
		Items: []TaxableItem{taxable, exempt},
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	// Only the 100 line is taxable: 100 * 0.20 = 20
	if !got.TaxTotal.Equal(decimal.RequireFromString("20")) {
		t.Errorf("TaxTotal = %s, want 20 (exempt excluded)", got.TaxTotal)
	}
}

func TestFlatRate_MixedRates_ProduceDistinctLines(t *testing.T) {
	calc := NewFlatCalculator(decimal.RequireFromString("0.20"), "VAT")
	standard := item("100", 1) // 20% default → 20
	reduced := item("100", 1)  // override 5% → 5
	reduced.TaxRate = decimal.RequireFromString("0.05")

	got, err := calc.Calculate(context.Background(), TaxRequest{
		Items: []TaxableItem{standard, reduced},
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !got.TaxTotal.Equal(decimal.RequireFromString("25")) {
		t.Errorf("TaxTotal = %s, want 25", got.TaxTotal)
	}
	if len(got.Lines) != 2 {
		t.Errorf("expected 2 rate lines (20%% and 5%%), got %d", len(got.Lines))
	}
}

func TestFlatRate_Shipping_TaxedAtDefault_WhenAnyItemTaxable(t *testing.T) {
	calc := NewFlatCalculator(decimal.RequireFromString("0.10"), "GST")
	got, err := calc.Calculate(context.Background(), TaxRequest{
		Items:          []TaxableItem{item("100", 1)}, // item tax 10
		ShippingAmount: decimal.RequireFromString("50"),
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	// item 10 + shipping 5 = 15
	if !got.TaxTotal.Equal(decimal.RequireFromString("15")) {
		t.Errorf("TaxTotal = %s, want 15 (item + shipping)", got.TaxTotal)
	}
}

func TestFlatRate_Shipping_NotTaxed_WhenAllItemsExempt(t *testing.T) {
	calc := NewFlatCalculator(decimal.RequireFromString("0.20"), "VAT")
	exempt := item("100", 1)
	exempt.TaxCategory = TaxCategoryExempt

	got, err := calc.Calculate(context.Background(), TaxRequest{
		Items:          []TaxableItem{exempt},
		ShippingAmount: decimal.RequireFromString("40"),
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !got.TaxTotal.IsZero() {
		t.Errorf("expected 0 tax when all items exempt, got %s", got.TaxTotal)
	}
}

// ── TaxableItem helpers ─────────────────────────────────────────────────

func TestIsExempt_TrueForExemptAndZeroRated(t *testing.T) {
	cases := []struct {
		cat  string
		want bool
	}{
		{"", false},
		{TaxCategoryStandard, false},
		{TaxCategoryReduced, false},
		{TaxCategoryZeroRated, true},
		{TaxCategoryExempt, true},
	}
	for _, tc := range cases {
		got := (TaxableItem{TaxCategory: tc.cat}).IsExempt()
		if got != tc.want {
			t.Errorf("IsExempt(%q) = %v, want %v", tc.cat, got, tc.want)
		}
	}
}
