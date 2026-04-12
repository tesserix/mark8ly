package admin

// validation_test.go uses package admin (not admin_test) so it can
// exercise unexported helpers toServiceCreateRequest etc.

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

func init() { gin.SetMode(gin.TestMode) }

func bindCreate(t *testing.T, body string) error {
	t.Helper()
	r := gin.New()
	var bindErr error
	r.POST("/x", func(c *gin.Context) {
		var req CreateProductRequest
		bindErr = c.ShouldBindJSON(&req)
	})
	req := httptest.NewRequest(http.MethodPost, "/x", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(httptest.NewRecorder(), req)
	return bindErr
}

func TestCreateProductRequest_BindingTags_RequireTitle(t *testing.T) {
	body := `{"title":"","variants":[{"sku":"s","price":"1.00"}]}`
	if err := bindCreate(t, body); err == nil {
		t.Fatal("expected binding error for empty title")
	}
}

func TestCreateProductRequest_BindingTags_StatusOneOf(t *testing.T) {
	body := `{"title":"T","status":"invalid","variants":[{"sku":"s","price":"1.00"}]}`
	if err := bindCreate(t, body); err == nil {
		t.Fatal("expected binding error for bad status")
	}
}

func TestToServiceCreateRequest_PreservesAllFields(t *testing.T) {
	desc := "body"
	seoT := "seo"
	cat := "cat1"
	req := CreateProductRequest{
		Handle:            "h",
		Title:             "T",
		Description:       &desc,
		Status:            "active",
		Tags:              []string{"a", "b"},
		SEOTitle:          &seoT,
		PrimaryCategoryID: &cat,
		CategoryIDs:       []string{"c1"},
		Options: []CreateProductOptionInput{
			{Name: "Size", Values: []string{"S", "M"}},
		},
		Variants: []CreateProductVariantInput{{
			SKU: "SKU-1", Price: decimal.NewFromFloat(19.99),
			CurrencyCode: "USD", InventoryQuantity: 5, InventoryPolicy: "deny",
			OptionValues: []CreateVariantOptionRefInput{{OptionName: "Size", Value: "S"}},
			Position:     2,
		}},
		Media: []CreateProductMediaInput{
			{StorageKey: "k", URL: "u", Position: 0, MediaType: "image"},
		},
	}
	testUserUUID := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	out := toServiceCreateRequest(req, "t1", "s1", testUserUUID)
	if out.TenantID != "t1" || out.StoreID != "s1" {
		t.Fatalf("tenant/store wrong: %+v", out)
	}
	if out.Title != "T" || out.Handle != "h" || out.Status != "active" {
		t.Fatalf("scalars wrong: %+v", out)
	}
	if out.Description != "body" {
		t.Fatalf("description not dereffed: %q", out.Description)
	}
	if out.CreatedBy == nil || *out.CreatedBy != testUserUUID {
		t.Fatalf("createdBy wrong")
	}
	if len(out.Options) != 1 || len(out.Options[0].Values) != 2 || out.Options[0].Values[0].Value != "S" {
		t.Fatalf("options wrong: %+v", out.Options)
	}
	if len(out.Variants) != 1 {
		t.Fatalf("variants = %d", len(out.Variants))
	}
	v := out.Variants[0]
	if v.SKU != "SKU-1" || v.InitialStock != 5 || v.Position != 2 {
		t.Fatalf("variant wrong: %+v", v)
	}
	if len(v.OptionValues) != 1 || v.OptionValues[0].OptionName != "Size" || v.OptionValues[0].Value != "S" {
		t.Fatalf("variant option values wrong: %+v", v.OptionValues)
	}
	if len(out.Media) != 1 || out.Media[0].MediaType != "image" {
		t.Fatalf("media wrong: %+v", out.Media)
	}
	if len(out.CategoryIDs) != 1 || out.CategoryIDs[0] != "c1" {
		t.Fatalf("category ids wrong")
	}
}

func TestToServiceCreateRequest_DefaultsMediaType(t *testing.T) {
	req := CreateProductRequest{
		Title:    "T",
		Variants: []CreateProductVariantInput{{SKU: "s", Price: decimal.NewFromFloat(1)}},
		Media:    []CreateProductMediaInput{{StorageKey: "k", URL: "u"}},
	}
	out := toServiceCreateRequest(req, "t", "s", "")
	if len(out.Media) != 1 || out.Media[0].MediaType != "image" {
		t.Fatalf("media default wrong: %+v", out.Media)
	}
	if out.CreatedBy != nil {
		t.Fatalf("createdBy should be nil when empty")
	}
}

func TestToServiceCreateCategory_DefaultsIsActiveTrue(t *testing.T) {
	req := CreateCategoryRequest{Name: "Shoes", Position: 3}
	out := toServiceCreateCategory(req, "t1", "s1")
	if out.TenantID != "t1" || out.StoreID != "s1" || out.Name != "Shoes" || out.Position != 3 {
		t.Fatalf("fields wrong: %+v", out)
	}
	if !out.IsActive {
		t.Fatalf("IsActive should default to true when unset")
	}
}

func TestToServiceCreateCategory_RespectsExplicitIsActive(t *testing.T) {
	f := false
	req := CreateCategoryRequest{Name: "X", IsActive: &f}
	out := toServiceCreateCategory(req, "t", "s")
	if out.IsActive {
		t.Fatalf("IsActive should be false")
	}
}

func TestToServiceUpdateCategory_NilFieldsStayNil(t *testing.T) {
	name := "New"
	req := UpdateCategoryRequest{Name: &name}
	out := toServiceUpdateCategory(req, "cid", "t", "s")
	if out.ID != "cid" || out.Name == nil || *out.Name != "New" {
		t.Fatalf("bad: %+v", out)
	}
	if out.Slug != nil || out.Description != nil || out.Position != nil || out.IsActive != nil {
		t.Fatalf("nil fields should remain nil: %+v", out)
	}
}

func TestToServiceUpdateVariantBasics_CarriesFields(t *testing.T) {
	price := decimal.NewFromFloat(12.34)
	sku := "NEW"
	inv := 42
	cc := "USD"
	req := UpdateVariantRequest{SKU: &sku, Price: &price, InventoryQuantity: &inv, CurrencyCode: &cc}
	out := toServiceUpdateVariantBasics(req, "pid", "vid", "sid", "tid")
	if out.ProductID != "pid" || out.VariantID != "vid" || out.StoreID != "sid" || out.TenantID != "tid" {
		t.Fatalf("ids wrong: %+v", out)
	}
	if out.SKU == nil || *out.SKU != "NEW" || out.Price == nil || !out.Price.Equal(price) {
		t.Fatalf("scalars wrong: %+v", out)
	}
	if out.InventoryQuantity == nil || *out.InventoryQuantity != 42 {
		t.Fatalf("inv wrong")
	}
	if out.CurrencyCode == nil || *out.CurrencyCode != "USD" {
		t.Fatalf("currency wrong")
	}
	if out.Barcode != nil || out.WeightGrams != nil {
		t.Fatalf("nil fields should stay nil: %+v", out)
	}
}

func TestToServiceAddMedia_DefaultsMediaTypeToImage(t *testing.T) {
	req := CreateMediaRequest{StorageKey: "k", URL: "u"}
	out := toServiceAddMedia(req, "pid", "sid", "tid")
	if out.MediaType != "image" {
		t.Fatalf("media_type default = %q", out.MediaType)
	}
	if out.StorageKey != "k" || out.URL != "u" || out.ProductID != "pid" {
		t.Fatalf("fields wrong: %+v", out)
	}
}

func TestToServiceUpdateMedia_CarriesOnlySetFields(t *testing.T) {
	alt := "new alt"
	req := UpdateMediaWireRequest{Alt: &alt}
	out := toServiceUpdateMedia(req, "pid", "mid", "sid", "tid")
	if out.ProductID != "pid" || out.MediaID != "mid" {
		t.Fatalf("ids wrong: %+v", out)
	}
	if out.Alt == nil || *out.Alt != "new alt" {
		t.Fatalf("alt wrong")
	}
	if out.Position != nil || out.URL != nil {
		t.Fatalf("nil fields should stay nil: %+v", out)
	}
}

func TestListProductsQuery_Defaults(t *testing.T) {
	q := ListProductsQuery{}
	q.Defaults()
	if q.Page != 1 || q.PageSize != 20 {
		t.Fatalf("defaults = %+v", q)
	}
	q2 := ListProductsQuery{Page: 3, PageSize: 50}
	q2.Defaults()
	if q2.Page != 3 || q2.PageSize != 50 {
		t.Fatalf("defaults clobbered values: %+v", q2)
	}
}
