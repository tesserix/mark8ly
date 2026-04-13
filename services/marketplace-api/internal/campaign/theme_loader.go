package campaign

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

// StoreThemeLoader loads a CampaignTheme by combining the store record
// (for name) with the storefront branding record (for colors / fonts /
// tagline). It is the production implementation of ThemeLoader wired
// into the send worker from main.go.
//
// Both lookups are cheap single-row queries on the shared Cloud SQL
// instance and happen ONCE per campaign dispatch, so there's no reason
// to cache inside the loader.
type StoreThemeLoader struct {
	db          *gorm.DB
	brandingSvc *branding.Service
}

// NewStoreThemeLoader constructs a theme loader.
func NewStoreThemeLoader(db *gorm.DB, brandingSvc *branding.Service) *StoreThemeLoader {
	return &StoreThemeLoader{db: db, brandingSvc: brandingSvc}
}

// LoadTheme resolves a store + its branding into a CampaignTheme. Any
// missing row falls back to editorial defaults — a campaign should never
// fail to send just because the merchant didn't set up branding yet.
func (l *StoreThemeLoader) LoadTheme(ctx context.Context, storeID uuid.UUID) (CampaignTheme, error) {
	theme := CampaignTheme{}

	// Look up store name. We don't hard-fail on missing store — the send
	// worker calls us with a campaign's StoreID and an orphaned campaign
	// should still attempt dispatch with neutral defaults (which will
	// almost certainly fail downstream, surfacing the issue explicitly).
	var store stores.Store
	err := l.db.WithContext(ctx).
		Where("id = ?", storeID).
		First(&store).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return CampaignTheme{}, fmt.Errorf("campaign: load store: %w", err)
	}
	theme.StoreName = store.Name

	// Branding is optional — service returns defaults when none exists,
	// and ErrNotFound is surfaced for truly missing rows. Keep going with
	// whatever we have.
	b, err := l.brandingSvc.GetByStoreID(ctx, storeID)
	if err != nil && !errors.Is(err, apperrors.ErrNotFound) {
		return CampaignTheme{}, fmt.Errorf("campaign: load branding: %w", err)
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

	return theme, nil
}

// fontFamilyFor maps the merchant's chosen font key (from the branding
// form's allowlist) to a CSS font-family stack suitable for email
// clients. Stacks always end with a system-ui fallback because mail
// clients vary wildly in webfont support.
//
// heading=true appends serif fallbacks; false appends sans fallbacks.
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
