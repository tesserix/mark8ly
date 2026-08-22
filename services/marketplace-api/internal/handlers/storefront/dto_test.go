package storefront_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/mark8ly/marketplace-api/internal/handlers/storefront"
	"github.com/mark8ly/marketplace-api/internal/product"
)

// TestStorefrontDTO_NoLeakFields is the core M6 guarantee: the storefront
// DTO family must not expose any audit, cost, or raw-inventory field.
func TestStorefrontDTO_NoLeakFields(t *testing.T) {
	forbidden := []string{
		"cost_price", "cost_price_cents", "cost",
		"inventory_quantity", "stock_quantity",
		"deleted_at", "updated_by", "created_by",
		"tenant_id",
	}
	types := []reflect.Type{
		reflect.TypeOf(storefront.StorefrontProductResponse{}),
		reflect.TypeOf(storefront.StorefrontVariantResponse{}),
		reflect.TypeOf(storefront.StorefrontCategoryRef{}),
		reflect.TypeOf(storefront.StorefrontCategoryResponse{}),
		reflect.TypeOf(storefront.StorefrontMediaResponse{}),
		reflect.TypeOf(storefront.StorefrontProductOption{}),
		reflect.TypeOf(storefront.StorefrontProductOptionValue{}),
		reflect.TypeOf(storefront.StorefrontVariantOptionRef{}),
		reflect.TypeOf(storefront.StorefrontPriceRange{}),
	}
	for _, tp := range types {
		for i := 0; i < tp.NumField(); i++ {
			f := tp.Field(i)
			tag := f.Tag.Get("json")
			name := strings.SplitN(tag, ",", 2)[0]
			for _, bad := range forbidden {
				if name == bad {
					t.Errorf("%s.%s has forbidden json tag %q", tp.Name(), f.Name, name)
				}
				if strings.EqualFold(f.Name, bad) {
					t.Errorf("%s.%s has forbidden field name %q", tp.Name(), f.Name, f.Name)
				}
			}
		}
	}
}

func pint(v int) *int { return &v }

func makeAggregate(variants []product.Variant) *product.Aggregate {
	return &product.Aggregate{
		Product: product.Product{
			ID:     "p1",
			Handle: "linen-shirt",
			Title:  "Linen Shirt",
		},
		Variants: variants,
	}
}

func decFromInt(n int64) decimal.Decimal { return decimal.NewFromInt(n) }

func TestToStorefrontProductResponse_PriceRange_MinMax(t *testing.T) {
	a := makeAggregate([]product.Variant{
		{ID: "v1", Price: decFromInt(10), CurrencyCode: "USD"},
		{ID: "v2", Price: decFromInt(20), CurrencyCode: "USD"},
		{ID: "v3", Price: decFromInt(15), CurrencyCode: "USD"},
	})
	out := storefront.ToStorefrontProductResponse(a, nil)
	if !out.PriceRange.Min.Equal(decFromInt(10)) {
		t.Errorf("min: got %s want 10", out.PriceRange.Min.String())
	}
	if !out.PriceRange.Max.Equal(decFromInt(20)) {
		t.Errorf("max: got %s want 20", out.PriceRange.Max.String())
	}
	if out.PriceRange.CurrencyCode != "USD" {
		t.Errorf("currency: got %q", out.PriceRange.CurrencyCode)
	}
}

func TestToStorefrontProductResponse_PriceRange_SinglePrice(t *testing.T) {
	a := makeAggregate([]product.Variant{
		{ID: "v1", Price: decFromInt(42), CurrencyCode: "USD"},
	})
	out := storefront.ToStorefrontProductResponse(a, nil)
	if !out.PriceRange.Min.Equal(out.PriceRange.Max) || !out.PriceRange.Min.Equal(decFromInt(42)) {
		t.Errorf("single: got min=%s max=%s", out.PriceRange.Min, out.PriceRange.Max)
	}
}

func TestToStorefrontProductResponse_InStock_PositiveQty_True(t *testing.T) {
	a := makeAggregate([]product.Variant{
		{ID: "v1", Price: decFromInt(10), CurrencyCode: "USD", InventoryQuantity: 5},
	})
	out := storefront.ToStorefrontProductResponse(a, nil)
	if !out.Variants[0].InStock {
		t.Errorf("expected in_stock=true")
	}
}

func TestToStorefrontProductResponse_InStock_ZeroQty_False(t *testing.T) {
	a := makeAggregate([]product.Variant{
		{ID: "v1", Price: decFromInt(10), CurrencyCode: "USD", InventoryQuantity: 0},
	})
	out := storefront.ToStorefrontProductResponse(a, nil)
	if out.Variants[0].InStock {
		t.Errorf("expected in_stock=false")
	}
}

func TestToStorefrontProductResponse_LowStock_BelowThreshold_True(t *testing.T) {
	a := makeAggregate([]product.Variant{
		{ID: "v1", Price: decFromInt(10), CurrencyCode: "USD", InventoryQuantity: 2, LowStockThreshold: pint(5)},
	})
	out := storefront.ToStorefrontProductResponse(a, nil)
	if !out.Variants[0].LowStock {
		t.Errorf("expected low_stock=true")
	}
}

func TestToStorefrontProductResponse_LowStock_NoThreshold_False(t *testing.T) {
	a := makeAggregate([]product.Variant{
		{ID: "v1", Price: decFromInt(10), CurrencyCode: "USD", InventoryQuantity: 2, LowStockThreshold: nil},
	})
	out := storefront.ToStorefrontProductResponse(a, nil)
	if out.Variants[0].LowStock {
		t.Errorf("expected low_stock=false")
	}
}

func TestToStorefrontProductResponse_OmitsAuditFields(t *testing.T) {
	cost := decFromInt(3)
	now := time.Now()
	a := &product.Aggregate{
		Product: product.Product{
			ID: "p1", Handle: "h", Title: "t",
			TenantID:    "tenant-xyz",
			PublishedAt: &now,
		},
		Variants: []product.Variant{
			{ID: "v1", Price: decFromInt(10), CurrencyCode: "USD", InventoryQuantity: 7, CostPrice: &cost},
		},
	}
	out := storefront.ToStorefrontProductResponse(a, nil)
	b, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, bad := range []string{"cost_price", "inventory_quantity", "deleted_at", "tenant_id"} {
		if strings.Contains(s, bad) {
			t.Errorf("marshalled JSON contains forbidden field %q: %s", bad, s)
		}
	}
}
