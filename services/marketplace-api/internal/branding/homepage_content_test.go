package branding

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateHomepageContent_EmptyAndNull_OK(t *testing.T) {
	require.NoError(t, ValidateHomepageContent(nil))
	require.NoError(t, ValidateHomepageContent(json.RawMessage(``)))
	require.NoError(t, ValidateHomepageContent(json.RawMessage(`{}`)))
	require.NoError(t, ValidateHomepageContent(json.RawMessage(`null`)))
}

func TestValidateHomepageContent_InvalidJSON_Errors(t *testing.T) {
	require.Error(t, ValidateHomepageContent(json.RawMessage(`{`)))
}

func TestValidateHomepageContent_HeroDisabled_OK(t *testing.T) {
	require.NoError(t, ValidateHomepageContent(json.RawMessage(`{"hero":{"enabled":false}}`)))
}

func TestValidateHomepageContent_HeroWithFields_OK(t *testing.T) {
	body := `{"hero":{"enabled":true,"heading":"Acme","subheading":"Hand","image_url":"https://x/y.jpg","cta_label":"Shop","cta_url":"/a"}}`
	require.NoError(t, ValidateHomepageContent(json.RawMessage(body)))
}

func TestValidateHomepageContent_HeroHeadingTooLong_Errors(t *testing.T) {
	long := strings.Repeat("x", 201)
	body := `{"hero":{"enabled":true,"heading":"` + long + `"}}`
	err := ValidateHomepageContent(json.RawMessage(body))
	require.Error(t, err)
	require.Contains(t, err.Error(), "hero.heading")
}

func TestValidateHomepageContent_HeroSubheadingTooLong_Errors(t *testing.T) {
	long := strings.Repeat("x", 401)
	body := `{"hero":{"subheading":"` + long + `"}}`
	require.Error(t, ValidateHomepageContent(json.RawMessage(body)))
}

func TestValidateHomepageContent_HeroCtaLabelTooLong_Errors(t *testing.T) {
	long := strings.Repeat("x", 61)
	body := `{"hero":{"cta_label":"` + long + `"}}`
	require.Error(t, ValidateHomepageContent(json.RawMessage(body)))
}

func TestValidateHomepageContent_SectionText_OK(t *testing.T) {
	body := `{"sections":[{"type":"text","markdown":"## Hi"}]}`
	require.NoError(t, ValidateHomepageContent(json.RawMessage(body)))
}

func TestValidateHomepageContent_SectionTextMissingMarkdown_Errors(t *testing.T) {
	body := `{"sections":[{"type":"text"}]}`
	err := ValidateHomepageContent(json.RawMessage(body))
	require.Error(t, err)
	require.Contains(t, err.Error(), "text: markdown required")
}

func TestValidateHomepageContent_SectionImageRequiresURL_Errors(t *testing.T) {
	body := `{"sections":[{"type":"image"}]}`
	require.Error(t, ValidateHomepageContent(json.RawMessage(body)))
}

func TestValidateHomepageContent_SectionImage_OK(t *testing.T) {
	body := `{"sections":[{"type":"image","url":"https://x/y.jpg","alt":"product"}]}`
	require.NoError(t, ValidateHomepageContent(json.RawMessage(body)))
}

func TestValidateHomepageContent_SectionFeaturedRequiresCollection_Errors(t *testing.T) {
	body := `{"sections":[{"type":"featured_products"}]}`
	require.Error(t, ValidateHomepageContent(json.RawMessage(body)))
}

func TestValidateHomepageContent_SectionFeaturedLimitBounds_Errors(t *testing.T) {
	require.Error(t, ValidateHomepageContent(json.RawMessage(`{"sections":[{"type":"featured_products","collection_slug":"new","limit":0}]}`)))
	require.Error(t, ValidateHomepageContent(json.RawMessage(`{"sections":[{"type":"featured_products","collection_slug":"new","limit":25}]}`)))
	require.NoError(t, ValidateHomepageContent(json.RawMessage(`{"sections":[{"type":"featured_products","collection_slug":"new","limit":12}]}`)))
}

func TestValidateHomepageContent_SectionQuoteRequiresText_Errors(t *testing.T) {
	require.Error(t, ValidateHomepageContent(json.RawMessage(`{"sections":[{"type":"quote"}]}`)))
	require.Error(t, ValidateHomepageContent(json.RawMessage(`{"sections":[{"type":"quote","text":""}]}`)))
	require.NoError(t, ValidateHomepageContent(json.RawMessage(`{"sections":[{"type":"quote","text":"Hi"}]}`)))
}

func TestValidateHomepageContent_SectionUnknownType_Errors(t *testing.T) {
	body := `{"sections":[{"type":"bogus"}]}`
	err := ValidateHomepageContent(json.RawMessage(body))
	require.Error(t, err)
	require.Contains(t, err.Error(), `unknown section type "bogus"`)
}

func TestValidateHomepageContent_TooManySections_Errors(t *testing.T) {
	parts := make([]string, 13)
	for i := range parts {
		parts[i] = `{"type":"text","markdown":"x"}`
	}
	body := `{"sections":[` + strings.Join(parts, ",") + `]}`
	err := ValidateHomepageContent(json.RawMessage(body))
	require.Error(t, err)
	require.Contains(t, err.Error(), "at most 12 sections")
}
