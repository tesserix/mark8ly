package branding

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func validSection() FooterSection {
	return FooterSection{
		Label: "Company",
		Items: []FooterLinkItem{
			{Label: "About", Kind: "page", PageSlug: "about-us"},
			{Label: "Docs", Kind: "url", URL: "https://docs.example.com"},
		},
	}
}

func TestValidateFooterSections_HappyPath(t *testing.T) {
	sections := []FooterSection{validSection()}
	require.NoError(t, ValidateFooterSections(sections))
}

func TestValidateFooterSections_EmptySlice(t *testing.T) {
	require.NoError(t, ValidateFooterSections([]FooterSection{}))
}

func TestValidateFooterSections_RelativeURL(t *testing.T) {
	sections := []FooterSection{{
		Label: "Nav",
		Items: []FooterLinkItem{
			{Label: "Home", Kind: "url", URL: "/"},
		},
	}}
	require.NoError(t, ValidateFooterSections(sections))
}

func TestValidateFooterSections_RejectsTooManySections(t *testing.T) {
	sections := make([]FooterSection, maxFooterSections+1)
	for i := range sections {
		sections[i] = FooterSection{Label: "Section"}
	}
	err := ValidateFooterSections(sections)
	require.Error(t, err)
	require.Contains(t, err.Error(), "at most")
}

func TestValidateFooterSections_RejectsTooManyItems(t *testing.T) {
	items := make([]FooterLinkItem, maxFooterItemsPerSec+1)
	for i := range items {
		items[i] = FooterLinkItem{Label: "Item", Kind: "url", URL: "https://example.com"}
	}
	sections := []FooterSection{{Label: "Big", Items: items}}
	err := ValidateFooterSections(sections)
	require.Error(t, err)
	require.Contains(t, err.Error(), "more than")
}

func TestValidateFooterSections_RequiresSectionLabel(t *testing.T) {
	sections := []FooterSection{{Label: ""}}
	err := ValidateFooterSections(sections)
	require.Error(t, err)
	require.Contains(t, err.Error(), "label is required")
}

func TestValidateFooterSections_RejectsSectionLabelTooLong(t *testing.T) {
	sections := []FooterSection{{Label: strings.Repeat("a", maxFooterLabelLen+1)}}
	err := ValidateFooterSections(sections)
	require.Error(t, err)
	require.Contains(t, err.Error(), "exceeds")
}

func TestValidateFooterSections_RequiresItemLabel(t *testing.T) {
	sections := []FooterSection{{
		Label: "Nav",
		Items: []FooterLinkItem{{Label: "", Kind: "url", URL: "https://example.com"}},
	}}
	err := ValidateFooterSections(sections)
	require.Error(t, err)
	require.Contains(t, err.Error(), "label is required")
}

func TestValidateFooterSections_RejectsInvalidKind(t *testing.T) {
	sections := []FooterSection{{
		Label: "Nav",
		Items: []FooterLinkItem{{Label: "Link", Kind: "email", URL: "mailto:x@x.com"}},
	}}
	err := ValidateFooterSections(sections)
	require.Error(t, err)
	require.Contains(t, err.Error(), "kind must be")
}

func TestValidateFooterSections_PageKindRejectsURL(t *testing.T) {
	sections := []FooterSection{{
		Label: "Nav",
		Items: []FooterLinkItem{{Label: "About", Kind: "page", PageSlug: "about-us", URL: "https://example.com"}},
	}}
	err := ValidateFooterSections(sections)
	require.Error(t, err)
	require.Contains(t, err.Error(), "must not set url")
}

func TestValidateFooterSections_URLKindRejectsPageSlug(t *testing.T) {
	sections := []FooterSection{{
		Label: "Nav",
		Items: []FooterLinkItem{{Label: "Docs", Kind: "url", URL: "https://docs.example.com", PageSlug: "docs"}},
	}}
	err := ValidateFooterSections(sections)
	require.Error(t, err)
	require.Contains(t, err.Error(), "must not set page_slug")
}

func TestValidateFooterSections_PageSlugFormat(t *testing.T) {
	// Valid slug
	sections := []FooterSection{{
		Label: "Nav",
		Items: []FooterLinkItem{{Label: "About", Kind: "page", PageSlug: "about-us"}},
	}}
	require.NoError(t, ValidateFooterSections(sections))

	// Invalid: starts with hyphen
	sections[0].Items[0].PageSlug = "-bad-slug"
	err := ValidateFooterSections(sections)
	require.Error(t, err)
	require.Contains(t, err.Error(), "page_slug is invalid")

	// Invalid: uppercase
	sections[0].Items[0].PageSlug = "About-Us"
	err = ValidateFooterSections(sections)
	require.Error(t, err)
	require.Contains(t, err.Error(), "page_slug is invalid")
}

func TestValidateFooterSections_URLSchemeWhitelist(t *testing.T) {
	base := FooterSection{
		Label: "Nav",
		Items: []FooterLinkItem{{Label: "Link", Kind: "url"}},
	}

	// https allowed
	base.Items[0].URL = "https://example.com"
	require.NoError(t, ValidateFooterSections([]FooterSection{base}))

	// http allowed
	base.Items[0].URL = "http://example.com"
	require.NoError(t, ValidateFooterSections([]FooterSection{base}))

	// relative allowed
	base.Items[0].URL = "/about"
	require.NoError(t, ValidateFooterSections([]FooterSection{base}))

	// ftp not allowed
	base.Items[0].URL = "ftp://example.com"
	err := ValidateFooterSections([]FooterSection{base})
	require.Error(t, err)
	require.Contains(t, err.Error(), "must start with")

	// javascript not allowed
	base.Items[0].URL = "javascript:alert(1)"
	err = ValidateFooterSections([]FooterSection{base})
	require.Error(t, err)
	require.Contains(t, err.Error(), "must start with")
}

func TestValidateFooterSections_URLTooLong(t *testing.T) {
	sections := []FooterSection{{
		Label: "Nav",
		Items: []FooterLinkItem{{
			Label: "Link",
			Kind:  "url",
			URL:   "https://example.com/" + strings.Repeat("a", maxFooterURLLen),
		}},
	}}
	err := ValidateFooterSections(sections)
	require.Error(t, err)
	require.Contains(t, err.Error(), "exceeds")
}

func TestParseFooterSections_EmptyJSON(t *testing.T) {
	sections, err := ParseFooterSections(nil)
	require.NoError(t, err)
	require.Empty(t, sections)
}

func TestParseFooterSections_ValidJSON(t *testing.T) {
	raw := []byte(`[{"label":"Company","items":[{"label":"About","kind":"page","page_slug":"about-us"}]}]`)
	sections, err := ParseFooterSections(raw)
	require.NoError(t, err)
	require.Len(t, sections, 1)
	require.Equal(t, "Company", sections[0].Label)
}

func TestEncodeFooterSections_RoundTrip(t *testing.T) {
	input := []FooterSection{validSection()}
	encoded, err := EncodeFooterSections(input)
	require.NoError(t, err)
	decoded, err := ParseFooterSections(encoded)
	require.NoError(t, err)
	require.Equal(t, input, decoded)
}
