package storefront_test

import (
	"encoding/json"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"

	"github.com/mark8ly/marketplace-api/internal/handlers/storefront"
	"github.com/mark8ly/marketplace-api/internal/product"
)

// The public media base the storefront is configured with in prod.
const testMediaBase = "https://cdn.mark8ly.com"

func strp(s string) *string { return &s }
func intp(i int) *int       { return &i }

func TestToStorefrontPersonalisationFields_NothingIsNil(t *testing.T) {
	require.Nil(t, storefront.ToStorefrontPersonalisationFields(nil, testMediaBase))
	require.Nil(t, storefront.ToStorefrontPersonalisationFields([]product.PersonalisationField{}, testMediaBase))
}

// The buyer's view is narrower than the merchant's, and the things it
// leaves out are the point of the mapper.
func TestToStorefrontPersonalisationFields_WithholdsMerchantOnlyData(t *testing.T) {
	area := datatypes.JSON([]byte(`{"x":10,"y":10,"w":50,"h":50}`))
	in := []product.PersonalisationField{{
		ID: "f1", TenantID: "tenant-secret", StoreID: "store-secret",
		ProductID: "p1", Key: "photo", Label: "Your photo",
		Kind: product.PersonalisationKindImage, Required: true,
		MinPx: intp(1200), MaxImages: intp(2),
		MockupStorageKey: strp("tenants/t1/mockups/abc/shirt.jpg"),
		PrintArea:        &area,
	}}

	out := storefront.ToStorefrontPersonalisationFields(in, testMediaBase)
	require.Len(t, out, 1)

	blob, err := json.Marshal(out[0])
	require.NoError(t, err)
	body := string(blob)

	// No tenancy, no merchant bookkeeping.
	require.NotContains(t, body, "tenant-secret")
	require.NotContains(t, body, "store-secret")
	require.NotContains(t, body, "created_at")

	// The mockup KEY stays off the wire; the mockup URL goes on it
	// (#966). This reverses the earlier decision to withhold both, and
	// the distinction is the reason: a URL is a thing to fetch, a key is
	// a thing to reason about our storage layout with. The mockup is the
	// merchant's own product artwork in the PUBLIC bucket — the same
	// image a shopper already sees on the product page.
	require.NotContains(t, body, `"mockup_storage_key"`)
	require.Contains(t, body,
		`"mockup_url":"`+testMediaBase+`/tenants/t1/mockups/abc/shirt.jpg"`)
	require.Contains(t, body, `"print_area":{"x":10,"y":10,"w":50,"h":50}`)

	// What the form actually needs survives.
	require.Contains(t, body, `"min_px":1200`)
	require.Contains(t, body, `"max_images":2`)
	require.Contains(t, body, `"required":true`)
}

// A shopper choosing between "Matte" and "Gloss (+2.50)" cannot decide
// without the number, so price_delta is on the wire for both the kinds
// that carry money.
func TestToStorefrontPersonalisationFields_CarriesThePriceImpact(t *testing.T) {
	gloss := decimal.RequireFromString("2.50")
	wrap := decimal.RequireFromString("4.00")

	out := storefront.ToStorefrontPersonalisationFields([]product.PersonalisationField{
		{
			ID: "f1", Kind: product.PersonalisationKindSelect, Label: "Finish",
			Options: []product.PersonalisationOption{
				{ID: "o1", Label: "Matte", PriceDelta: decimal.Zero},
				{ID: "o2", Label: "Gloss", PriceDelta: gloss},
			},
		},
		{ID: "f2", Kind: product.PersonalisationKindCheckbox, Label: "Gift wrap", PriceDelta: &wrap},
	}, testMediaBase)

	require.Len(t, out[0].Options, 2)
	require.Equal(t, "Gloss", out[0].Options[1].Label)
	require.True(t, out[0].Options[1].PriceDelta.Equal(gloss))
	require.NotNil(t, out[1].PriceDelta)
	require.True(t, out[1].PriceDelta.Equal(wrap))
}

// Option VALUE is the machine key; the buyer sees the label. Leaking the
// value buys nothing and invites a client to send it instead of the id,
// which checkout would then have to disambiguate.
func TestToStorefrontPersonalisationFields_SendsOptionIdAndLabelOnly(t *testing.T) {
	out := storefront.ToStorefrontPersonalisationFields([]product.PersonalisationField{{
		ID: "f1", Kind: product.PersonalisationKindSelect,
		Options: []product.PersonalisationOption{
			{ID: "o1", Value: "matte_internal", Label: "Matte"},
		},
	}}, testMediaBase)
	blob, err := json.Marshal(out[0])
	require.NoError(t, err)
	require.NotContains(t, string(blob), "matte_internal")
	require.Contains(t, string(blob), "Matte")
}

func TestStorefrontProductResponse_OmitsPersonalisationWhenThereIsNone(t *testing.T) {
	// A plain product's JSON must not grow an empty array: every
	// storefront response is cached and served to anonymous traffic.
	blob, err := json.Marshal(storefront.StorefrontProductResponse{})
	require.NoError(t, err)
	require.NotContains(t, string(blob), "personalisation")
}

// The 2D composite needs a mockup AND a rectangle to put the artwork in
// (#966). Half a pair renders worse than none: a mockup with nowhere to
// composite shows the merchant's blank shirt as if that were the
// preview, and a rectangle with no mockup has nothing to sit on.
func TestToStorefrontPersonalisationFields_MockupIsAllOrNothing(t *testing.T) {
	area := datatypes.JSON([]byte(`{"x":10,"y":10,"w":50,"h":50}`))
	zeroArea := datatypes.JSON([]byte(`{"x":10,"y":10,"w":0,"h":50}`))
	bad := datatypes.JSON([]byte(`not json`))

	cases := []struct {
		name string
		key  *string
		area *datatypes.JSON
		base string
		want bool
	}{
		{"both present", strp("tenants/t1/m/a.jpg"), &area, testMediaBase, true},
		{"no rectangle", strp("tenants/t1/m/a.jpg"), nil, testMediaBase, false},
		{"no mockup", nil, &area, testMediaBase, false},
		{"empty key", strp(""), &area, testMediaBase, false},
		// A relative URL would resolve against the storefront's own
		// origin and 404, so an unset base drops the mockup instead.
		{"no media base configured", strp("tenants/t1/m/a.jpg"), &area, "", false},
		// Zero width would composite an invisible image over the
		// mockup, which reads as a failed upload.
		{"zero-width rectangle", strp("tenants/t1/m/a.jpg"), &zeroArea, testMediaBase, false},
		{"unparseable rectangle", strp("tenants/t1/m/a.jpg"), &bad, testMediaBase, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := storefront.ToStorefrontPersonalisationFields(
				[]product.PersonalisationField{{
					ID: "f1", Kind: product.PersonalisationKindImage,
					MockupStorageKey: tc.key, PrintArea: tc.area,
				}}, tc.base)
			require.Len(t, out, 1)
			if tc.want {
				require.NotNil(t, out[0].MockupURL)
				require.NotNil(t, out[0].PrintArea)
			} else {
				require.Nil(t, out[0].MockupURL, "a half pair must not be emitted")
				require.Nil(t, out[0].PrintArea)
			}
		})
	}
}

// A trailing slash on the configured base must not produce a double
// slash in the URL.
func TestToStorefrontPersonalisationFields_MockupURLJoinsCleanly(t *testing.T) {
	area := datatypes.JSON([]byte(`{"x":0,"y":0,"w":100,"h":100}`))
	out := storefront.ToStorefrontPersonalisationFields(
		[]product.PersonalisationField{{
			ID: "f1", Kind: product.PersonalisationKindImage,
			MockupStorageKey: strp("tenants/t1/m/a.jpg"), PrintArea: &area,
		}}, "https://cdn.mark8ly.com/")
	require.NotNil(t, out[0].MockupURL)
	require.Equal(t, "https://cdn.mark8ly.com/tenants/t1/m/a.jpg", *out[0].MockupURL)
}
