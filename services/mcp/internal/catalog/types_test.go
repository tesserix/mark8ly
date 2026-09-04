package catalog

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A not-found result must carry found:false and a nested, non-null product
// object — never a bare null and never a top-level null anywhere in the tree.
func TestProductResult_NotFoundMarshalsWithNestedProductAndNoNull(t *testing.T) {
	raw, err := json.Marshal(ProductResult{Found: false})
	require.NoError(t, err)

	assert.NotContains(t, string(raw), "null", "no null anywhere, including inside the nested object")

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	assert.Equal(t, false, decoded["found"])
	product, ok := decoded["product"].(map[string]any)
	require.True(t, ok, "product must be a nested object, not absent or null")
	assert.Equal(t, "", product["handle"])
	assert.Equal(t, []any{}, product["categories"])
	assert.Equal(t, []any{}, product["image_urls"])
}

func TestBrandingResult_NotFoundMarshalsWithNestedBrandingAndNoNull(t *testing.T) {
	raw, err := json.Marshal(BrandingResult{Found: false})
	require.NoError(t, err)

	assert.NotContains(t, string(raw), "null", "no null anywhere, including inside the nested object")

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	assert.Equal(t, false, decoded["found"])
	branding, ok := decoded["branding"].(map[string]any)
	require.True(t, ok, "branding must be a nested object, not absent or null")
	assert.Equal(t, "", branding["logo_url"])
	assert.Equal(t, []any{}, branding["promotions"])
}

func TestProductResult_FoundCarriesThePayloadUnderProduct(t *testing.T) {
	pr := ProductResult{
		Found: true,
		Product: Product{
			Handle: "mug",
			Title:  "Mug",
		},
	}

	raw, err := json.Marshal(pr)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	assert.Equal(t, true, decoded["found"])
	product, ok := decoded["product"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "mug", product["handle"])
	assert.Equal(t, "Mug", product["title"])
}

func TestBrandingResult_FoundCarriesThePayloadUnderBranding(t *testing.T) {
	br := BrandingResult{
		Found: true,
		Branding: Branding{
			LogoURL: "https://cdn/logo.png",
			Tagline: "Fresh brews",
		},
	}

	raw, err := json.Marshal(br)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	assert.Equal(t, true, decoded["found"])
	branding, ok := decoded["branding"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "https://cdn/logo.png", branding["logo_url"])
	assert.Equal(t, "Fresh brews", branding["tagline"])
}

// This is the test that catches the embedding trap: Product's value-receiver
// MarshalJSON would be promoted onto ProductResult if Product were embedded,
// making ProductResult marshal as a bare Product and silently drop Found.
// Asserting both a top-level "found" AND a top-level "product" key fails the
// moment someone "simplifies" the wrapper back to an embed.
func TestProductResult_DoesNotMarshalAsABareProduct(t *testing.T) {
	raw, err := json.Marshal(ProductResult{Found: true, Product: Product{Handle: "mug"}})
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	_, hasFound := decoded["found"]
	_, hasProduct := decoded["product"]
	assert.True(t, hasFound, "top-level found key must survive")
	assert.True(t, hasProduct, "top-level product key must exist — a bare Product has no such key")
	_, hasHandleAtTopLevel := decoded["handle"]
	assert.False(t, hasHandleAtTopLevel, "handle must not leak to the top level, which is what embedding would do")
}

// The reverse trap for Branding, for the same reason.
func TestBrandingResult_DoesNotMarshalAsABareBranding(t *testing.T) {
	raw, err := json.Marshal(BrandingResult{Found: true, Branding: Branding{LogoURL: "x"}})
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	_, hasFound := decoded["found"]
	_, hasBranding := decoded["branding"]
	assert.True(t, hasFound)
	assert.True(t, hasBranding)
	_, hasLogoAtTopLevel := decoded["logo_url"]
	assert.False(t, hasLogoAtTopLevel, "logo_url must not leak to the top level, which is what embedding would do")
}

// Product and Branding no longer carry their own Found field — it is only
// meaningful on the two single-resource wrapper types now.
func TestProduct_HasNoFoundField(t *testing.T) {
	raw, err := json.Marshal(Product{Handle: "mug"})
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	_, hasFound := decoded["found"]
	assert.False(t, hasFound, "Product must not carry found — every list element would otherwise carry a meaningless true")
}

func TestBranding_HasNoFoundField(t *testing.T) {
	raw, err := json.Marshal(Branding{LogoURL: "x"})
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	_, hasFound := decoded["found"]
	assert.False(t, hasFound)
}

// A list result's elements must carry no found key at all — that's the whole
// point of the change.
func TestProductList_ElementsHaveNoFoundKey(t *testing.T) {
	list := ProductList{Products: []Product{{Handle: "mug"}, {Handle: "cup"}}}
	raw, err := json.Marshal(list)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	products, ok := decoded["products"].([]any)
	require.True(t, ok)
	require.Len(t, products, 2)
	for _, item := range products {
		m, ok := item.(map[string]any)
		require.True(t, ok)
		_, hasFound := m["found"]
		assert.False(t, hasFound, "list elements must not carry found")
	}
}
