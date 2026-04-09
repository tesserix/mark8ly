package admin_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/shopspring/decimal"

	"github.com/mark8ly/marketplace-api/internal/handlers/admin"
	"github.com/mark8ly/marketplace-api/internal/product"
)

func sampleAggregate() *product.Aggregate {
	now := time.Now().UTC()
	desc := "body"
	seoT := "seo"
	return &product.Aggregate{
		Product: product.Product{
			ID: "p1", StoreID: "s1", Handle: "h", Title: "T",
			Description: &desc, Status: "active",
			Tags:      pq.StringArray{"a", "b"},
			SEOTitle:  &seoT,
			CreatedAt: now, UpdatedAt: now,
		},
		Options: []product.Option{
			{ID: "opt1", Name: "Size", Position: 0, Values: []product.OptionValue{
				{ID: "ov1", Value: "S", Position: 0},
				{ID: "ov2", Value: "M", Position: 1},
			}},
			{ID: "opt2", Name: "Color", Position: 1, Values: []product.OptionValue{
				{ID: "ov3", Value: "Red", Position: 0},
				{ID: "ov4", Value: "Blue", Position: 1},
			}},
		},
		Variants: []product.Variant{
			mkVariant("v1", "SKU-1", "ov1", "ov3"),
			mkVariant("v2", "SKU-2", "ov1", "ov4"),
			mkVariant("v3", "SKU-3", "ov2", "ov3"),
			mkVariant("v4", "SKU-4", "ov2", "ov4"),
		},
		Media: []product.Media{
			{ID: "m1", URL: "u1", StorageKey: "k1", Position: 0, MediaType: "image"},
			{ID: "m2", URL: "u2", StorageKey: "k2", Position: 1, MediaType: "image"},
		},
	}
}

func mkVariant(id, sku string, ovIDs ...string) product.Variant {
	links := make([]product.VariantOptionValue, 0, len(ovIDs))
	for _, ov := range ovIDs {
		links = append(links, product.VariantOptionValue{VariantID: id, OptionValueID: ov})
	}
	return product.Variant{
		ID: id, SKU: sku, Price: decimal.NewFromFloat(19.99), CurrencyCode: "USD",
		InventoryQuantity: 10, InventoryPolicy: "deny",
		OptionValueLinks: links,
	}
}

func TestToAdminProductResponse_FullGraph(t *testing.T) {
	a := sampleAggregate()
	cats := []admin.AdminCategoryRef{
		{ID: "c1", Name: "Cat1", Slug: "cat1"},
		{ID: "c2", Name: "Cat2", Slug: "cat2"},
	}
	resp := admin.ToAdminProductResponse(a, cats)
	if len(resp.Variants) != 4 {
		t.Fatalf("variants = %d", len(resp.Variants))
	}
	if len(resp.Media) != 2 {
		t.Fatalf("media = %d", len(resp.Media))
	}
	if len(resp.Options) != 2 || resp.Options[0].Name != "Size" {
		t.Fatalf("options = %+v", resp.Options)
	}
	if len(resp.Categories) != 2 {
		t.Fatalf("categories = %d", len(resp.Categories))
	}
	// v1 should reference (Size=S, Color=Red) by resolved names.
	v1 := resp.Variants[0]
	if len(v1.OptionValues) != 2 {
		t.Fatalf("v1 options = %d", len(v1.OptionValues))
	}
	found := map[string]string{}
	for _, ov := range v1.OptionValues {
		found[ov.OptionName] = ov.Value
	}
	if found["Size"] != "S" || found["Color"] != "Red" {
		t.Fatalf("v1 option names not resolved: %+v", found)
	}
}

func TestToAdminProductResponse_NilTags_BecomesEmptySlice(t *testing.T) {
	a := &product.Aggregate{Product: product.Product{ID: "p1", Tags: nil}}
	resp := admin.ToAdminProductResponse(a, nil)
	if resp.Tags == nil {
		t.Fatalf("tags nil, want empty slice")
	}
	if len(resp.Tags) != 0 {
		t.Fatalf("tags = %v", resp.Tags)
	}
}

func TestToAdminProductResponse_OmitNilFields(t *testing.T) {
	a := &product.Aggregate{Product: product.Product{ID: "p1", Title: "T"}}
	resp := admin.ToAdminProductResponse(a, nil)
	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if strings.Contains(s, `"description"`) {
		t.Fatalf("description should be omitted: %s", s)
	}
	if strings.Contains(s, `"seo_title"`) {
		t.Fatalf("seo_title should be omitted: %s", s)
	}
}

func TestToAdminProductResponse_DecimalSerialization(t *testing.T) {
	a := &product.Aggregate{
		Product: product.Product{ID: "p1"},
		Variants: []product.Variant{{
			ID: "v1", SKU: "s", Price: decimal.NewFromFloat(19.99),
			CurrencyCode: "USD", InventoryPolicy: "deny",
		}},
	}
	resp := admin.ToAdminProductResponse(a, nil)
	b, err := json.Marshal(resp.Variants[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"price":"19.99"`) {
		t.Fatalf("price not string-encoded: %s", string(b))
	}
}

func TestAdminVariantResponse_IncludesCostPriceAndInventoryQuantity(t *testing.T) {
	typ := reflect.TypeOf(admin.AdminVariantResponse{})
	for _, field := range []string{"CostPrice", "InventoryQuantity"} {
		if _, ok := typ.FieldByName(field); !ok {
			t.Errorf("AdminVariantResponse missing field %q", field)
		}
	}
}
