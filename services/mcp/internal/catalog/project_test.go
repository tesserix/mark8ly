package catalog

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The three tax fields are cart mechanics the storefront echoes back on
// checkout. They are not facts about a product and must never reach a model.
// They are absent from Product by construction — this test pins that the
// projection cannot reintroduce them.
func TestProjectProduct_DropsTaxAndInternalFields(t *testing.T) {
	in := storefrontProduct{
		ID:     "8f14e45f-ce7e-4a1b-9d3f-000000000001",
		Handle: "mug",
		Title:  "Mug",
	}
	in.TaxCode = strptr("GST")
	in.TaxCategory = strptr("standard")

	got := projectProduct(in)

	raw, err := json.Marshal(got)
	require.NoError(t, err)

	for _, banned := range []string{"tax_code", "tax_category", "tax_rate_override", "8f14e45f"} {
		assert.NotContains(t, string(raw), banned,
			"%s must not reach an agent", banned)
	}
	assert.Equal(t, "mug", got.Handle)
}

func TestProjectProduct_FlattensPriceRangeAndCategories(t *testing.T) {
	in := storefrontProduct{Handle: "mug", Title: "Mug"}
	in.PriceRange.Min = "9.50"
	in.PriceRange.Max = "12.00"
	in.PriceRange.CurrencyCode = "AUD"
	in.Categories = []storefrontCategoryRef{{Name: "Mugs", Slug: "mugs"}}
	in.Media = []storefrontMedia{{URL: "https://cdn/x.jpg", MediaType: "image"}}

	got := projectProduct(in)

	assert.Equal(t, "9.50", got.PriceMin)
	assert.Equal(t, "12.00", got.PriceMax)
	assert.Equal(t, "AUD", got.Currency)
	assert.Equal(t, []string{"mugs"}, got.Categories)
	assert.Equal(t, []string{"https://cdn/x.jpg"}, got.ImageURLs)
}

func TestProjectProduct_ExtractsImagesByMediaType(t *testing.T) {
	in := storefrontProduct{Handle: "mug", Title: "Mug"}
	in.Media = []storefrontMedia{
		{URL: "https://cdn/video.mp4", MediaType: "video", Position: 1},
		{URL: "https://cdn/image1.jpg", MediaType: "image", Position: 2},
		{URL: "https://cdn/image2.jpg", MediaType: "image", Position: 0},
	}

	got := projectProduct(in)

	// Should be sorted by position (0, 2, 1), filtered to images only
	assert.Equal(t, []string{"https://cdn/image2.jpg", "https://cdn/image1.jpg"}, got.ImageURLs)
}

func TestProjectProduct_HasFoundField(t *testing.T) {
	in := storefrontProduct{Handle: "mug", Title: "Mug"}
	got := projectProduct(in)

	// Found should be true for a real product
	assert.True(t, got.Found)
}

func TestProjectProduct_IncludesOptionalDescription(t *testing.T) {
	desc := "A ceramic mug"
	in := storefrontProduct{
		Handle:      "mug",
		Title:       "Mug",
		Description: &desc,
	}

	got := projectProduct(in)

	assert.Equal(t, "A ceramic mug", got.Description)
}

func TestProjectProduct_EmptyImageList(t *testing.T) {
	in := storefrontProduct{Handle: "mug", Title: "Mug"}
	in.Media = []storefrontMedia{}

	got := projectProduct(in)

	assert.Empty(t, got.ImageURLs)
}

func TestProjectCategory_TransformsFields(t *testing.T) {
	in := storefrontCategory{Name: "Mugs", Slug: "mugs", Featured: true, Position: 0}

	got := projectCategory(in)

	assert.Equal(t, "Mugs", got.Name)
	assert.Equal(t, "mugs", got.Slug)
	assert.True(t, got.Featured)
}

func TestProjectBranding_DropsInternalFields(t *testing.T) {
	in := storefrontBranding{
		ColorAccent:      "#2D4A2B",
		AnnouncementText: strptr("Sale!"),
	}

	got := projectBranding(in)

	raw, err := json.Marshal(got)
	require.NoError(t, err)

	// Should not contain internal or original JSON field names
	assert.NotContains(t, string(raw), "color_accent")
	assert.NotContains(t, string(raw), "announcement_text")
	assert.NotContains(t, string(raw), "active_promotions")
}

func TestProjectBranding_TransformsFields(t *testing.T) {
	logo := "https://cdn/logo.png"
	tagline := "Great mugs"
	in := storefrontBranding{
		LogoURL:          &logo,
		Tagline:          &tagline,
		ColorAccent:      "#2D4A2B",
		AnnouncementText: strptr("Holiday sale"),
		Promotions: []storefrontPromotion{
			{Label: "20% off", CouponCode: strptr("HOLIDAY20")},
		},
	}

	got := projectBranding(in)

	assert.Equal(t, "https://cdn/logo.png", got.LogoURL)
	assert.Equal(t, "Great mugs", got.Tagline)
	assert.Equal(t, "#2D4A2B", got.AccentColor)
	assert.Equal(t, "Holiday sale", got.Announcement)
	assert.Len(t, got.Promotions, 1)
	assert.Equal(t, "20% off", got.Promotions[0].Label)
	assert.Equal(t, "HOLIDAY20", got.Promotions[0].CouponCode)
}

func TestProjectBranding_HasFoundField(t *testing.T) {
	in := storefrontBranding{ColorAccent: "#2D4A2B"}
	got := projectBranding(in)

	// Found should be true for real branding data
	assert.True(t, got.Found)
}

func TestProjectBranding_EmptyPromotions(t *testing.T) {
	in := storefrontBranding{ColorAccent: "#2D4A2B"}

	got := projectBranding(in)

	assert.Empty(t, got.Promotions)
}

func TestProjectPromotion_TransformsFields(t *testing.T) {
	code := "PROMO123"
	in := storefrontPromotion{
		Label:      "Summer sale",
		CouponCode: &code,
	}

	got := projectPromotion(in)

	assert.Equal(t, "Summer sale", got.Label)
	assert.Equal(t, "PROMO123", got.CouponCode)
}

func TestProjectPromotion_OptionalCouponCode(t *testing.T) {
	in := storefrontPromotion{Label: "Free shipping"}

	got := projectPromotion(in)

	assert.Equal(t, "Free shipping", got.Label)
	assert.Equal(t, "", got.CouponCode)
}

func TestProjectProduct_EmptyCollectionsMarshalAsArraysNotNull(t *testing.T) {
	in := storefrontProduct{Handle: "mug", Title: "Mug"}
	// No categories, no media

	got := projectProduct(in)

	raw, err := json.Marshal(got)
	require.NoError(t, err)
	jsonStr := string(raw)

	// Schema says categories and image_urls are non-nullable arrays.
	// They must marshal as [] never null.
	assert.Contains(t, jsonStr, `"categories":[]`)
	assert.Contains(t, jsonStr, `"image_urls":[]`)
	assert.NotContains(t, jsonStr, `"categories":null`)
	assert.NotContains(t, jsonStr, `"image_urls":null`)
	// No null anywhere in the wire
	assert.NotContains(t, jsonStr, "null")
}

func TestProjectProduct_SnakeCaseJsonKeys(t *testing.T) {
	in := storefrontProduct{
		Handle: "mug",
		Title:  "Mug",
	}
	in.PriceRange.Min = "9.50"
	in.PriceRange.Max = "12.00"
	in.PriceRange.CurrencyCode = "AUD"

	got := projectProduct(in)

	raw, err := json.Marshal(got)
	require.NoError(t, err)
	jsonStr := string(raw)

	// Snake_case keys required
	assert.Contains(t, jsonStr, `"handle"`)
	assert.Contains(t, jsonStr, `"title"`)
	assert.Contains(t, jsonStr, `"price_min"`)
	assert.Contains(t, jsonStr, `"price_max"`)
	assert.Contains(t, jsonStr, `"currency"`)
	assert.Contains(t, jsonStr, `"found"`)
	// No PascalCase keys
	assert.NotContains(t, jsonStr, `"Handle"`)
	assert.NotContains(t, jsonStr, `"Title"`)
	assert.NotContains(t, jsonStr, `"PriceMin"`)
	assert.NotContains(t, jsonStr, `"PriceMax"`)
	assert.NotContains(t, jsonStr, `"Found"`)
}

func TestProjectProduct_ZeroValueNoNull(t *testing.T) {
	// Zero-value Product is the Task 5 not-found shape
	got := Product{}

	raw, err := json.Marshal(got)
	require.NoError(t, err)
	jsonStr := string(raw)

	// No null anywhere, all zero values
	assert.NotContains(t, jsonStr, "null")
	// Empty collections, not null
	assert.Contains(t, jsonStr, `"categories":[]`)
	assert.Contains(t, jsonStr, `"image_urls":[]`)
}

func TestProjectBranding_PointerFieldsFlattenToEmptyStrings(t *testing.T) {
	// No logo, no tagline, no announcement
	in := storefrontBranding{ColorAccent: "#2D4A2B"}

	got := projectBranding(in)

	raw, err := json.Marshal(got)
	require.NoError(t, err)
	jsonStr := string(raw)

	// Must have empty strings, not null
	assert.Contains(t, jsonStr, `"logo_url":""`)
	assert.Contains(t, jsonStr, `"tagline":""`)
	assert.Contains(t, jsonStr, `"announcement":""`)
	// No null anywhere
	assert.NotContains(t, jsonStr, "null")
}

func TestProjectBranding_EmptyPromotionsMarshalAsArrayNotNull(t *testing.T) {
	in := storefrontBranding{ColorAccent: "#2D4A2B"}
	// No promotions

	got := projectBranding(in)

	raw, err := json.Marshal(got)
	require.NoError(t, err)
	jsonStr := string(raw)

	// Schema says promotions is non-nullable array
	assert.Contains(t, jsonStr, `"promotions":[]`)
	assert.NotContains(t, jsonStr, `"promotions":null`)
	// No null anywhere
	assert.NotContains(t, jsonStr, "null")
}

func TestProjectPromotion_CouponCodeFlattensToEmptyString(t *testing.T) {
	// No coupon code
	in := storefrontPromotion{Label: "Free shipping"}

	got := projectPromotion(in)

	raw, err := json.Marshal(got)
	require.NoError(t, err)
	jsonStr := string(raw)

	// Must flatten to empty string, not null
	assert.Contains(t, jsonStr, `"coupon_code":""`)
	// No null anywhere
	assert.NotContains(t, jsonStr, "null")
}

func strptr(s string) *string { return &s }
