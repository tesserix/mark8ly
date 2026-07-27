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

// TestRecentOrder_OmitNilFields locks in the omitempty contract the mobile
// TypeScript client depends on: nil pointers must make the key ABSENT from
// the JSON, not present with a null value, because the client models these
// fields with .optional() rather than .nullable().
func TestRecentOrder_OmitNilFields(t *testing.T) {
	ro := admin.RecentOrder{
		ID:            "o1",
		OrderNumber:   "1001",
		CustomerEmail: "a@example.com",
		GrandTotal:    10,
		Status:        "pending",
		CreatedAt:     "2026-01-01T00:00:00Z",
	}
	b, err := json.Marshal(ro)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if strings.Contains(s, `"customer_name"`) {
		t.Fatalf("customer_name should be omitted when nil: %s", s)
	}
	if strings.Contains(s, `"image_url"`) {
		t.Fatalf("image_url should be omitted when nil: %s", s)
	}
}

func TestRecentOrder_IncludesFieldsWhenSet(t *testing.T) {
	name := "Jane Doe"
	img := "https://example.com/p.jpg"
	ro := admin.RecentOrder{
		ID: "o1", OrderNumber: "1001", CustomerEmail: "a@example.com",
		CustomerName: &name, GrandTotal: 10, Status: "pending",
		CreatedAt: "2026-01-01T00:00:00Z", ImageURL: &img,
	}
	b, err := json.Marshal(ro)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, `"customer_name":"Jane Doe"`) {
		t.Fatalf("customer_name not serialized: %s", s)
	}
	if !strings.Contains(s, `"image_url":"https://example.com/p.jpg"`) {
		t.Fatalf("image_url not serialized: %s", s)
	}
}

// TestLowStockItem_ProductIDAlwaysPresent guards the bug fix: product_id
// must always be present (never omitted), since it is the id the mobile
// queue row navigates with — unlike image_url, which is genuinely optional.
func TestLowStockItem_ProductIDAlwaysPresent(t *testing.T) {
	li := admin.LowStockItem{
		ID: "v1", ProductID: "p1", Title: "T", VariantTitle: "Small",
		Quantity: 1, LowStockThreshold: 5,
	}
	b, err := json.Marshal(li)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, `"product_id":"p1"`) {
		t.Fatalf("product_id should always be present: %s", s)
	}
	if strings.Contains(s, `"image_url"`) {
		t.Fatalf("image_url should be omitted when nil: %s", s)
	}
}

// TestLowStockItem_ProductIDPresentEvenWhenEmpty is the discriminating case
// for the no-omitempty guarantee above: a non-empty ProductID would
// serialize whether or not `omitempty` were present, so it can't prove the
// tag is absent. The empty-string zero value is what actually distinguishes
// "no omitempty" (key always present, even as "") from "omitempty" (key
// vanishes on the zero value).
func TestLowStockItem_ProductIDPresentEvenWhenEmpty(t *testing.T) {
	li := admin.LowStockItem{
		ID: "v1", ProductID: "", Title: "T", VariantTitle: "Small",
		Quantity: 1, LowStockThreshold: 5,
	}
	b, err := json.Marshal(li)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, `"product_id":""`) {
		t.Fatalf("product_id key should be present even when empty: %s", s)
	}
}
