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

func TestValidateHomepageContent_HeroRejectsJavascriptCtaURL(t *testing.T) {
	body := `{"hero":{"enabled":true,"cta_url":"javascript:alert(1)","cta_label":"Go"}}`
	err := ValidateHomepageContent(json.RawMessage(body))
	require.Error(t, err)
	require.Contains(t, err.Error(), "cta_url")
}

func TestValidateHomepageContent_HeroRejectsDataImageURL(t *testing.T) {
	body := `{"hero":{"enabled":true,"image_url":"data:text/html;base64,PHNjcmlwdD4="}}`
	err := ValidateHomepageContent(json.RawMessage(body))
	require.Error(t, err)
	require.Contains(t, err.Error(), "image_url")
}

func TestValidateHomepageContent_ImageSectionRejectsJavascriptURL(t *testing.T) {
	body := `{"sections":[{"type":"image","url":"javascript:alert(1)"}]}`
	err := ValidateHomepageContent(json.RawMessage(body))
	require.Error(t, err)
	require.Contains(t, err.Error(), "image.url")
}

func TestValidateHomepageContent_HeroAcceptsSiteRelativeAndHttps(t *testing.T) {
	for _, url := range []string{"/collections/new", "https://cdn.example/y.jpg"} {
		body := `{"hero":{"enabled":true,"image_url":"` + url + `"}}`
		require.NoError(t, ValidateHomepageContent(json.RawMessage(body)))
	}
}

func TestValidateHomepageContent_Marquee_OK(t *testing.T) {
	body := `{"sections":[{"type":"marquee","items":["Hand picked","Small batch"]}]}`
	require.NoError(t, ValidateHomepageContent(json.RawMessage(body)))
}

func TestValidateHomepageContent_MarqueeTooManyItems_Errors(t *testing.T) {
	body := `{"sections":[{"type":"marquee","items":["1","2","3","4","5","6","7","8","9"]}]}`
	err := ValidateHomepageContent(json.RawMessage(body))
	require.Error(t, err)
	require.Contains(t, err.Error(), "marquee")
}

func TestValidateHomepageContent_MarqueeEmptyItems_Errors(t *testing.T) {
	body := `{"sections":[{"type":"marquee","items":[]}]}`
	require.Error(t, ValidateHomepageContent(json.RawMessage(body)))
}

func TestValidateHomepageContent_MarqueeItemTooLong_Errors(t *testing.T) {
	long := strings.Repeat("x", 81)
	body := `{"sections":[{"type":"marquee","items":["` + long + `"]}]}`
	require.Error(t, ValidateHomepageContent(json.RawMessage(body)))
}

func TestValidateHomepageContent_MarqueeSpeedInvalid_Errors(t *testing.T) {
	body := `{"sections":[{"type":"marquee","items":["x"],"speed":"warp"}]}`
	err := ValidateHomepageContent(json.RawMessage(body))
	require.Error(t, err)
	require.Contains(t, err.Error(), "marquee.speed")
}

func TestValidateHomepageContent_MarqueeSpeedValid_OK(t *testing.T) {
	for _, sp := range []string{"slow", "normal", "fast"} {
		body := `{"sections":[{"type":"marquee","items":["x"],"speed":"` + sp + `"}]}`
		require.NoError(t, ValidateHomepageContent(json.RawMessage(body)), sp)
	}
}

func TestValidateHomepageContent_PullQuote_OK(t *testing.T) {
	body := `{"sections":[{"type":"pull_quote","text":"Hello","attribution":"Staff"}]}`
	require.NoError(t, ValidateHomepageContent(json.RawMessage(body)))
}

func TestValidateHomepageContent_PullQuoteMissingText_Errors(t *testing.T) {
	require.Error(t, ValidateHomepageContent(json.RawMessage(`{"sections":[{"type":"pull_quote"}]}`)))
	require.Error(t, ValidateHomepageContent(json.RawMessage(`{"sections":[{"type":"pull_quote","text":"  "}]}`)))
}

func TestValidateHomepageContent_Letter_OK(t *testing.T) {
	body := `{"sections":[{"type":"letter","title":"Hello","body":"Body","cta_label":"Read","cta_url":"/pages/about"}]}`
	require.NoError(t, ValidateHomepageContent(json.RawMessage(body)))
}

func TestValidateHomepageContent_LetterRejectsJavascriptCta(t *testing.T) {
	body := `{"sections":[{"type":"letter","title":"H","body":"B","cta_url":"javascript:alert(1)"}]}`
	err := ValidateHomepageContent(json.RawMessage(body))
	require.Error(t, err)
	require.Contains(t, err.Error(), "letter.cta_url")
}

func TestValidateHomepageContent_LetterMissingTitle_Errors(t *testing.T) {
	require.Error(t, ValidateHomepageContent(json.RawMessage(`{"sections":[{"type":"letter","body":"x"}]}`)))
}

func TestValidateHomepageContent_LetterMissingBody_Errors(t *testing.T) {
	require.Error(t, ValidateHomepageContent(json.RawMessage(`{"sections":[{"type":"letter","title":"x"}]}`)))
}

func TestValidateHomepageContent_LetterTitleTooLong_Errors(t *testing.T) {
	long := strings.Repeat("x", 201)
	body := `{"sections":[{"type":"letter","title":"` + long + `","body":"b"}]}`
	require.Error(t, ValidateHomepageContent(json.RawMessage(body)))
}

func TestValidateHomepageContent_FeaturedProductSlugs_OK(t *testing.T) {
	body := `{"sections":[{"type":"featured_products","product_slugs":["a","b","c"]}]}`
	require.NoError(t, ValidateHomepageContent(json.RawMessage(body)))
}

func TestValidateHomepageContent_FeaturedProductSlugsTooMany_Errors(t *testing.T) {
	body := `{"sections":[{"type":"featured_products","product_slugs":["1","2","3","4","5","6","7"]}]}`
	require.Error(t, ValidateHomepageContent(json.RawMessage(body)))
}

func TestValidateHomepageContent_FeaturedRequiresCollectionOrSlugs(t *testing.T) {
	require.Error(t, ValidateHomepageContent(json.RawMessage(`{"sections":[{"type":"featured_products"}]}`)))
}

func TestValidateHomepageContent_FeaturedProductSlugsEmpty_Errors(t *testing.T) {
	body := `{"sections":[{"type":"featured_products","product_slugs":["a","","b"]}]}`
	require.Error(t, ValidateHomepageContent(json.RawMessage(body)))
}

// TestHomepageContent_RoundTrip_NewTypes proves a blob containing each
// of the new section types survives the "validate → store as raw JSON
// → reread → revalidate" cycle the service.Update path puts it
// through. Catches regressions where a new field gets accidentally
// stripped by middleware (e.g. a partial struct unmarshal that drops
// unknown fields).
func TestHomepageContent_RoundTrip_NewTypes(t *testing.T) {
	input := json.RawMessage(`{
		"hero":{"enabled":true,"heading":"Store","cta_label":"Shop","cta_url":"/products"},
		"sections":[
			{"type":"marquee","items":["Hand picked","Small batch"],"speed":"slow"},
			{"type":"pull_quote","text":"A memorable line.","attribution":"Staff"},
			{"type":"letter","eyebrow":"Note","title":"Hello","body":"Body","cta_label":"About","cta_url":"/pages/about"},
			{"type":"featured_products","heading":"Picks","product_slugs":["alpha","bravo","charlie"]},
			{"type":"featured_products","heading":"Collection","collection_slug":"new","limit":4}
		]
	}`)

	// 1) validate on ingress.
	require.NoError(t, ValidateHomepageContent(input))

	// 2) simulate the JSONB column: the DB stores the bytes, verbatim.
	stored := append(json.RawMessage{}, input...)

	// 3) revalidate on read (no mutation should have occurred).
	require.NoError(t, ValidateHomepageContent(stored))

	// 4) assert no field got stripped by an intermediate unmarshal. We
	//    unmarshal into the same sectionInput + heroInput types the
	//    validator uses to confirm the fields round-trip through that
	//    struct, then serialise and compare keys we care about are
	//    still present.
	var parsed struct {
		Hero     *heroInput     `json:"hero"`
		Sections []sectionInput `json:"sections"`
	}
	require.NoError(t, json.Unmarshal(stored, &parsed))
	require.NotNil(t, parsed.Hero)
	require.Len(t, parsed.Sections, 5)

	marquee := parsed.Sections[0]
	require.Equal(t, "marquee", marquee.Type)
	require.Equal(t, []string{"Hand picked", "Small batch"}, marquee.Items)
	require.NotNil(t, marquee.Speed)
	require.Equal(t, "slow", *marquee.Speed)

	pullQuote := parsed.Sections[1]
	require.Equal(t, "pull_quote", pullQuote.Type)
	require.NotNil(t, pullQuote.Text)
	require.Equal(t, "A memorable line.", *pullQuote.Text)

	letter := parsed.Sections[2]
	require.Equal(t, "letter", letter.Type)
	require.NotNil(t, letter.Title)
	require.Equal(t, "Hello", *letter.Title)
	require.NotNil(t, letter.Body)
	require.Equal(t, "Body", *letter.Body)
	require.NotNil(t, letter.CtaURL)
	require.Equal(t, "/pages/about", *letter.CtaURL)

	handPicked := parsed.Sections[3]
	require.Equal(t, "featured_products", handPicked.Type)
	require.Equal(t, []string{"alpha", "bravo", "charlie"}, handPicked.ProductSlugs)
	require.Nil(t, handPicked.CollectionSlug)

	collection := parsed.Sections[4]
	require.Equal(t, "featured_products", collection.Type)
	require.NotNil(t, collection.CollectionSlug)
	require.Equal(t, "new", *collection.CollectionSlug)
	require.Nil(t, collection.ProductSlugs)
	require.NotNil(t, collection.Limit)
	require.Equal(t, 4, *collection.Limit)
}
