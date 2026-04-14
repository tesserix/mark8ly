package branding

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Homepage content shape limits. Enforced at the service layer when
// writing to the store_branding.homepage_content JSONB column.
const (
	maxHomepageSections    = 12
	maxHeroHeading         = 200
	maxHeroSubheading      = 400
	maxHeroCtaLabel        = 60
	maxSectionTextMarkdown = 20_000
	maxSectionImageAlt     = 200
	maxSectionImageCaption = 200
	maxSectionQuoteText    = 500
	maxSectionQuoteAttrib  = 200
	maxSectionHeading      = 200
	maxFeaturedLimit       = 24
)

// ValidateHomepageContent checks the shape of a homepage_content JSONB
// blob. Returns an error with a human-readable message describing the
// first problem found; nil on empty/null input or valid content.
//
// Callers should wrap the error in apperrors.ValidationFailed at the
// service layer so handlers render it as 400 with a stable code.
func ValidateHomepageContent(raw json.RawMessage) error {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var c struct {
		Hero     *heroInput     `json:"hero"`
		Sections []sectionInput `json:"sections"`
	}
	if err := json.Unmarshal(raw, &c); err != nil {
		return fmt.Errorf("homepage_content: invalid JSON: %w", err)
	}
	if c.Hero != nil {
		if err := validateHero(c.Hero); err != nil {
			return err
		}
	}
	if len(c.Sections) > maxHomepageSections {
		return fmt.Errorf("homepage_content: at most %d sections allowed", maxHomepageSections)
	}
	for i, s := range c.Sections {
		if err := validateSection(s); err != nil {
			return fmt.Errorf("sections[%d]: %w", i, err)
		}
	}
	return nil
}

type heroInput struct {
	Enabled    *bool   `json:"enabled"`
	ImageURL   *string `json:"image_url"`
	Heading    *string `json:"heading"`
	Subheading *string `json:"subheading"`
	CtaLabel   *string `json:"cta_label"`
	CtaURL     *string `json:"cta_url"`
}

type sectionInput struct {
	Type           string  `json:"type"`
	Heading        *string `json:"heading,omitempty"`
	Markdown       *string `json:"markdown,omitempty"`
	URL            *string `json:"url,omitempty"`
	Alt            *string `json:"alt,omitempty"`
	Caption        *string `json:"caption,omitempty"`
	CollectionSlug *string `json:"collection_slug,omitempty"`
	Limit          *int    `json:"limit,omitempty"`
	Text           *string `json:"text,omitempty"`
	Attribution    *string `json:"attribution,omitempty"`
}

func validateHero(h *heroInput) error {
	if h.Heading != nil && len(*h.Heading) > maxHeroHeading {
		return fmt.Errorf("hero.heading: max %d chars", maxHeroHeading)
	}
	if h.Subheading != nil && len(*h.Subheading) > maxHeroSubheading {
		return fmt.Errorf("hero.subheading: max %d chars", maxHeroSubheading)
	}
	if h.CtaLabel != nil && len(*h.CtaLabel) > maxHeroCtaLabel {
		return fmt.Errorf("hero.cta_label: max %d chars", maxHeroCtaLabel)
	}
	if h.ImageURL != nil && *h.ImageURL != "" && !IsSafeURL(*h.ImageURL) {
		return fmt.Errorf("hero.image_url: unsafe URL scheme")
	}
	if h.CtaURL != nil && *h.CtaURL != "" && !IsSafeURL(*h.CtaURL) {
		return fmt.Errorf("hero.cta_url: unsafe URL scheme")
	}
	return nil
}

func validateSection(s sectionInput) error {
	if s.Heading != nil && len(*s.Heading) > maxSectionHeading {
		return fmt.Errorf("heading: max %d chars", maxSectionHeading)
	}
	switch strings.ToLower(s.Type) {
	case "text":
		if s.Markdown == nil {
			return fmt.Errorf("text: markdown required")
		}
		if len(*s.Markdown) > maxSectionTextMarkdown {
			return fmt.Errorf("text.markdown: max %d chars", maxSectionTextMarkdown)
		}
	case "image":
		if s.URL == nil || *s.URL == "" {
			return fmt.Errorf("image: url required")
		}
		if !IsSafeURL(*s.URL) {
			return fmt.Errorf("image.url: unsafe URL scheme")
		}
		if s.Alt != nil && len(*s.Alt) > maxSectionImageAlt {
			return fmt.Errorf("image.alt: max %d chars", maxSectionImageAlt)
		}
		if s.Caption != nil && len(*s.Caption) > maxSectionImageCaption {
			return fmt.Errorf("image.caption: max %d chars", maxSectionImageCaption)
		}
	case "featured_products":
		if s.CollectionSlug == nil || *s.CollectionSlug == "" {
			return fmt.Errorf("featured_products: collection_slug required")
		}
		if s.Limit != nil && (*s.Limit < 1 || *s.Limit > maxFeaturedLimit) {
			return fmt.Errorf("featured_products.limit: 1..%d", maxFeaturedLimit)
		}
	case "quote":
		if s.Text == nil || *s.Text == "" {
			return fmt.Errorf("quote: text required")
		}
		if len(*s.Text) > maxSectionQuoteText {
			return fmt.Errorf("quote.text: max %d chars", maxSectionQuoteText)
		}
		if s.Attribution != nil && len(*s.Attribution) > maxSectionQuoteAttrib {
			return fmt.Errorf("quote.attribution: max %d chars", maxSectionQuoteAttrib)
		}
	default:
		return fmt.Errorf("unknown section type %q", s.Type)
	}
	return nil
}
