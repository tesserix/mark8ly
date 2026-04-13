package giftcard

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/branding"
	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// StoreThemeLoader resolves a GiftCardEmailTheme + storefront URL for
// a given store. Joins the stores row (for name + slug) with the
// store_branding row (for colors / fonts / logo). Missing rows fall
// back to editorial defaults rather than failing — a merchant who
// never customized their branding still gets a polished email.
type StoreThemeLoader struct {
	db                *gorm.DB
	brandingSvc       *branding.Service
	storefrontURLBase string // e.g. "https://{slug}.mark8ly.com" — {slug} is substituted
}

// NewStoreThemeLoader constructs a loader. storefrontURLBase is a
// template with "{slug}" as the placeholder (same convention as
// platform-api's STOREFRONT_BASE_URL_TEMPLATE). Pass empty to disable
// the "Shop storefront" CTA in the email.
func NewStoreThemeLoader(db *gorm.DB, brandingSvc *branding.Service, storefrontURLBase string) *StoreThemeLoader {
	return &StoreThemeLoader{
		db:                db,
		brandingSvc:       brandingSvc,
		storefrontURLBase: storefrontURLBase,
	}
}

// LoadTheme resolves store + branding into the gift card email theme
// and returns the storefront URL for the CTA button. The second return
// value is the storefront URL (may be empty).
func (l *StoreThemeLoader) LoadTheme(ctx context.Context, storeID uuid.UUID) (GiftCardEmailTheme, string, error) {
	theme := GiftCardEmailTheme{}
	storefrontURL := ""

	var store stores.Store
	err := l.db.WithContext(ctx).
		Where("id = ?", storeID).
		First(&store).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return GiftCardEmailTheme{}, "", fmt.Errorf("giftcard: load store: %w", err)
	}
	theme.StoreName = store.Name

	// Build storefront URL from template if we have a slug + template.
	if store.Slug != "" && l.storefrontURLBase != "" {
		storefrontURL = substituteSlug(l.storefrontURLBase, store.Slug)
	}

	b, err := l.brandingSvc.GetByStoreID(ctx, storeID)
	if err != nil && !errors.Is(err, apperrors.ErrNotFound) {
		return GiftCardEmailTheme{}, "", fmt.Errorf("giftcard: load branding: %w", err)
	}
	if b != nil {
		theme.ColorBackground = b.ColorBackground
		theme.ColorText = b.ColorText
		theme.ColorAccent = b.ColorAccent
		theme.ColorButtonBg = b.ColorButtonBg
		theme.ColorButtonText = b.ColorButtonText
		theme.HeadingFont = fontFamilyFor(b.HeadingFont, true)
		theme.BodyFont = fontFamilyFor(b.BodyFont, false)
		if b.LogoURL != nil {
			theme.LogoURL = *b.LogoURL
		}
		if b.Tagline != nil {
			theme.Tagline = *b.Tagline
		}
		if b.FooterTagline != nil {
			theme.FooterTagline = *b.FooterTagline
		}
		if b.FooterCopyright != nil {
			theme.FooterCopyright = *b.FooterCopyright
		}
	}

	return theme, storefrontURL, nil
}

// substituteSlug replaces the {slug} placeholder in the URL template.
// Duplicated from platform-api's helper to avoid a cross-service
// import; intentionally trivial.
func substituteSlug(template, slug string) string {
	return replaceAll(template, "{slug}", slug)
}

// replaceAll is strings.ReplaceAll — inlined to avoid importing strings
// just for one call from this file.
func replaceAll(s, old, new string) string {
	out := ""
	for {
		i := indexOf(s, old)
		if i < 0 {
			return out + s
		}
		out += s[:i] + new
		s = s[i+len(old):]
	}
}

func indexOf(s, sub string) int {
	if len(sub) == 0 {
		return 0
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// fontFamilyFor maps the merchant's chosen font key (from the branding
// form's allowlist) to a CSS font-family stack suitable for email
// clients. Mirrors campaign.fontFamilyFor so both transactional and
// marketing emails render consistently for a given merchant.
func fontFamilyFor(key string, heading bool) string {
	const (
		serifFallback = "'Source Serif 4', 'Times New Roman', serif"
		sansFallback  = "-apple-system, BlinkMacSystemFont, 'Segoe UI', Helvetica, Arial, sans-serif"
	)
	switch key {
	case "source-serif-4":
		return "'Source Serif 4', Georgia, 'Times New Roman', serif"
	case "playfair-display":
		return "'Playfair Display', Georgia, 'Times New Roman', serif"
	case "lora":
		return "'Lora', Georgia, 'Times New Roman', serif"
	case "inter":
		return "'Inter', " + sansFallback
	case "source-sans-3":
		return "'Source Sans 3', " + sansFallback
	case "dm-sans":
		return "'DM Sans', " + sansFallback
	}
	if heading {
		return "Georgia, " + serifFallback
	}
	return sansFallback
}
