package instore

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/mark8ly/marketplace-api/internal/checkoutcore"
	"github.com/mark8ly/marketplace-api/internal/country"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/internal/tax"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

type fakePricer map[string]checkoutcore.CatalogPrice

func (f fakePricer) PriceFor(_ context.Context, _ string, variantID string) (checkoutcore.CatalogPrice, error) {
	p, ok := f[variantID]
	if !ok {
		return checkoutcore.CatalogPrice{}, checkoutcore.ErrCatalogItemNotFound
	}
	return p, nil
}

type fakeDetails struct{}

func (fakeDetails) Details(_ context.Context, _ string, ids []string) (map[string]VariantDetails, error) {
	out := map[string]VariantDetails{}
	for _, id := range ids {
		out[id] = VariantDetails{ProductID: uuid.NewString(), Title: "Skin", SKU: "SKU-" + id}
	}
	return out, nil
}

// tenPercent taxes every item at 10%.
type tenPercent struct{}

func (tenPercent) QuoteCounterTax(_ context.Context, _ *stores.Store, _ *country.SupportedCountry, items []tax.TaxableItem) (*tax.TaxBreakdown, error) {
	total := decimal.Zero
	for _, it := range items {
		total = total.Add(it.Amount.Mul(decimal.NewFromInt(int64(it.Quantity))).Mul(decimal.RequireFromString("0.1")))
	}
	return &tax.TaxBreakdown{TaxTotal: total.Round(2)}, nil
}

func newTestService() *Service {
	return &Service{
		pricer:  fakePricer{"v1": {Amount: decimal.RequireFromString("20.00"), CurrencyCode: "AUD"}},
		details: fakeDetails{},
		taxes:   tenPercent{},
		countries: func(context.Context, string) (*country.SupportedCountry, error) {
			return &country.SupportedCountry{}, nil
		},
	}
}

func baseInput() SaleInput {
	v := "v1"
	return SaleInput{
		TenantID:       uuid.New(),
		StoreID:        uuid.New(),
		Store:          &stores.Store{Slug: "sugarcoat", CountryCode: "AU", CurrencyCode: "AUD"},
		IdempotencyKey: uuid.NewString(),
		Lines:          []SaleLine{{VariantID: &v, Quantity: 2}},
		PaymentMethod:  PaymentCard,
		SoldByUserID:   "u-1",
	}
}

func requireValidation(t *testing.T, err error, field string) {
	t.Helper()
	var ae *apperrors.Error
	if !errors.As(err, &ae) || ae.Code != apperrors.CodeValidationFailed {
		t.Fatalf("err = %v, want a validation error on %s", err, field)
	}
	if ae.Details["field"] != field {
		t.Errorf("field = %v, want %q (err %v)", ae.Details["field"], field, err)
	}
}

func TestInStore_DiscountBounds(t *testing.T) {
	s := newTestService()
	ctx := context.Background()

	in := baseInput()
	in.CanDiscount = true
	in.Discount = decimal.NewFromInt(-1)
	_, err := s.RecordSale(ctx, in)
	requireValidation(t, err, "discount")

	in = baseInput()
	in.CanDiscount = true
	in.Discount = decimal.RequireFromString("40.01") // subtotal is 40.00
	_, err = s.RecordSale(ctx, in)
	requireValidation(t, err, "discount")

	in = baseInput()
	in.Discount = decimal.NewFromInt(5) // not permitted
	_, err = s.RecordSale(ctx, in)
	requireValidation(t, err, "discount")
}

func TestInStore_CustomLineNeedsPermission(t *testing.T) {
	s := newTestService()
	title, price := "Market special", decimal.NewFromInt(15)
	in := baseInput()
	in.Lines = append(in.Lines, SaleLine{CustomTitle: &title, CustomPrice: &price, Quantity: 1})
	_, err := s.RecordSale(context.Background(), in)
	requireValidation(t, err, "lines")
}

func TestInStore_CustomLineShape(t *testing.T) {
	s := newTestService()
	zero := decimal.Zero
	long := strings.Repeat("x", 201)
	for name, line := range map[string]SaleLine{
		"zero price":  {CustomTitle: &long, CustomPrice: &zero, Quantity: 1},
		"long title":  {CustomTitle: &long, CustomPrice: ptr(decimal.NewFromInt(1)), Quantity: 1},
		"no title":    {CustomPrice: ptr(decimal.NewFromInt(1)), Quantity: 1},
		"quantity 0":  {VariantID: ptr("v1"), Quantity: 0},
		"qty 1000":    {VariantID: ptr("v1"), Quantity: 1000},
		"both shapes": {VariantID: ptr("v1"), CustomTitle: ptr("x"), CustomPrice: ptr(decimal.NewFromInt(1)), Quantity: 1},
	} {
		in := baseInput()
		in.CanDiscount = true
		in.Lines = []SaleLine{line}
		if _, err := s.RecordSale(context.Background(), in); err == nil {
			t.Errorf("%s: want a validation error", name)
		}
	}
}

func TestInStore_OtherNeedsLabel(t *testing.T) {
	s := newTestService()
	in := baseInput()
	in.PaymentMethod = PaymentOther
	_, err := s.RecordSale(context.Background(), in)
	requireValidation(t, err, "payment_method_label")

	in.PaymentMethod = "bitcoin"
	_, err = s.RecordSale(context.Background(), in)
	requireValidation(t, err, "payment_method")
}

func TestInStore_CashMustCoverTotal(t *testing.T) {
	s := newTestService()
	in := baseInput() // 2 × 20.00 + 10% tax = 44.00
	in.PaymentMethod = PaymentCash
	in.CashTendered = ptr(decimal.RequireFromString("43.99"))
	_, err := s.RecordSale(context.Background(), in)
	requireValidation(t, err, "cash_tendered")
}

func TestInStore_UnknownVariantRefused(t *testing.T) {
	s := newTestService()
	in := baseInput()
	in.Lines = []SaleLine{{VariantID: ptr("not-in-store"), Quantity: 1}}
	_, err := s.RecordSale(context.Background(), in)
	requireValidation(t, err, "lines")
}

func ptr[T any](v T) *T { return &v }
