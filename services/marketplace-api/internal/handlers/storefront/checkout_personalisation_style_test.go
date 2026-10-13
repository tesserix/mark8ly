package storefront

import (
	"encoding/json"
	"testing"

	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"

	"github.com/mark8ly/marketplace-api/internal/product"
)

func styledField(t *testing.T) *product.PersonalisationField {
	t.Helper()
	colours, err := json.Marshal([]product.TextColour{
		{Label: "Gold foil", Hex: "#C9A227"},
		{Label: "White", Hex: "#FFFFFF"},
	})
	require.NoError(t, err)
	j := datatypes.JSON(colours)
	return &product.PersonalisationField{
		Key: "name", Label: "Name on the mug", Kind: product.PersonalisationKindText,
		TextFonts:   pq.StringArray{"script", "classic"},
		TextColours: &j,
	}
}

func s(v string) *string { return &v }

func TestResolveTextStyle_DefaultsToTheFirstOfEach(t *testing.T) {
	font, colour, err := resolveTextStyle(styledField(t), CheckoutPersonalisationRequest{})
	require.NoError(t, err)
	require.Equal(t, "script", *font, "what the preview showed before any pick")
	require.Equal(t, product.TextColour{Label: "Gold foil", Hex: "#C9A227"}, *colour)
}

func TestResolveTextStyle_TakesTheBuyersChoiceButTheCatalogsHex(t *testing.T) {
	font, colour, err := resolveTextStyle(styledField(t), CheckoutPersonalisationRequest{
		Font: s("classic"), Colour: s(" white "),
	})
	require.NoError(t, err)
	require.Equal(t, "classic", *font)
	require.Equal(t, "White", colour.Label)
	require.Equal(t, "#FFFFFF", colour.Hex)
}

func TestResolveTextStyle_RefusesWhatTheFieldDoesNotOffer(t *testing.T) {
	_, _, err := resolveTextStyle(styledField(t), CheckoutPersonalisationRequest{Font: s("block")})
	var p *personalisationProblem
	require.ErrorAs(t, err, &p)
	require.Equal(t, ProblemInvalid, p.Code)

	_, _, err = resolveTextStyle(styledField(t), CheckoutPersonalisationRequest{Colour: s("Neon pink")})
	require.ErrorAs(t, err, &p)
	require.Equal(t, ProblemInvalid, p.Code)
}

func TestResolveTextStyle_AFieldWithoutStyleRecordsNone(t *testing.T) {
	plain := &product.PersonalisationField{Key: "name", Label: "Name", Kind: product.PersonalisationKindText}
	font, colour, err := resolveTextStyle(plain, CheckoutPersonalisationRequest{
		Font: s("script"), Colour: s("Gold foil"),
	})
	require.NoError(t, err, "a stale form's picks are ignored, not an error")
	require.Nil(t, font)
	require.Nil(t, colour)
}
