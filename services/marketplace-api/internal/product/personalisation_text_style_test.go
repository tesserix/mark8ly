package product

import (
	"os"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

// The storefront renders and the admin lists fonts from a TypeScript copy
// of this catalogue. If the two drift, a merchant can pick a font the
// server refuses, or the server accepts one the storefront cannot draw.
func TestTextFontsMatchTheFrontendCatalogue(t *testing.T) {
	src, err := os.ReadFile("../../../../packages/ui/src/personalisation-fonts.ts")
	require.NoError(t, err)

	block := regexp.MustCompile(`(?s)PERSONALISATION_FONT_IDS = \[(.*?)\] as const`).FindSubmatch(src)
	require.NotNil(t, block, "PERSONALISATION_FONT_IDS not found")
	var tsIDs []string
	for _, m := range regexp.MustCompile(`"([a-z]+)"`).FindAllSubmatch(block[1], -1) {
		tsIDs = append(tsIDs, string(m[1]))
	}

	goIDs := make([]string, 0, len(TextFonts))
	for _, f := range TextFonts {
		goIDs = append(goIDs, f.ID)
	}
	require.Equal(t, goIDs, tsIDs)
	require.Equal(t, MaxTextFontsPerField, len(TextFonts),
		"the per-field cap is the whole catalogue; revisit both if it grows")
}
