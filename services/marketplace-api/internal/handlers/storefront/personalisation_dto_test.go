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

func strp(s string) *string { return &s }
func intp(i int) *int       { return &i }

func TestToStorefrontPersonalisationFields_NothingIsNil(t *testing.T) {
	require.Nil(t, storefront.ToStorefrontPersonalisationFields(nil))
	require.Nil(t, storefront.ToStorefrontPersonalisationFields([]product.PersonalisationField{}))
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

	out := storefront.ToStorefrontPersonalisationFields(in)
	require.Len(t, out, 1)

	blob, err := json.Marshal(out[0])
	require.NoError(t, err)
	body := string(blob)

	// No bucket paths, no tenancy, no merchant bookkeeping.
	require.NotContains(t, body, "tenant-secret")
	require.NotContains(t, body, "store-secret")
	require.NotContains(t, body, "mockups")
	require.NotContains(t, body, "print_area")
	require.NotContains(t, body, "created_at")

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
	})

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
	}})
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
