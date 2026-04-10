package branding

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// ServiceConfig groups dependencies for the branding service.
type ServiceConfig struct {
	DB     *gorm.DB
	Repo   Repository
	Logger *slog.Logger
}

// Service implements branding CRUD with validation.
type Service struct {
	db     *gorm.DB
	repo   Repository
	logger *slog.Logger
}

// NewService constructs a branding Service.
func NewService(cfg ServiceConfig) *Service {
	return &Service{
		db:     cfg.DB,
		repo:   cfg.Repo,
		logger: cfg.Logger,
	}
}

// Get returns the branding for a store, or defaults if none exists.
func (s *Service) Get(ctx context.Context, storeID uuid.UUID) (*StoreBranding, error) {
	b, err := s.repo.GetByStoreID(ctx, s.db, storeID)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			return defaultBranding(storeID), nil
		}
		return nil, err
	}
	return b, nil
}

// GetByStoreSlug returns branding for a store looked up by slug.
// The caller must resolve slug → storeID before calling this.
func (s *Service) GetByStoreID(ctx context.Context, storeID uuid.UUID) (*StoreBranding, error) {
	return s.Get(ctx, storeID)
}

// UpdateInput holds the fields that can be set on a branding update.
type UpdateInput struct {
	TenantID         uuid.UUID
	StoreID          uuid.UUID
	LogoURL          *string
	FaviconURL       *string
	Tagline          *string
	ColorBackground  *string
	ColorText        *string
	ColorAccent      *string
	ColorButtonBg    *string
	ColorButtonText  *string
	HeadingFont      *string
	BodyFont         *string
	LayoutVariant    *string
	HeroImageURL     *string
	AnnouncementText *string
	AnnouncementLink *string
	AnnouncementBg   *string
	AnnouncementActive *bool
	FooterTagline    *string
	FooterCopyright  *string
	SocialInstagram  *string
	SocialTwitter    *string
	SocialFacebook   *string
	SocialTiktok     *string
	SocialYoutube    *string
	CustomCSS        *string
	ShowPoweredBy    *bool
}

// Update validates and persists branding changes.
func (s *Service) Update(ctx context.Context, in UpdateInput) (*StoreBranding, error) {
	existing, err := s.repo.GetByStoreID(ctx, s.db, in.StoreID)
	if err != nil && !errors.Is(err, apperrors.ErrNotFound) {
		return nil, err
	}

	b := defaultBranding(in.StoreID)
	if existing != nil {
		b = existing
	}
	b.TenantID = in.TenantID

	// Apply optional fields.
	if in.LogoURL != nil {
		b.LogoURL = in.LogoURL
	}
	if in.FaviconURL != nil {
		b.FaviconURL = in.FaviconURL
	}
	if in.Tagline != nil {
		b.Tagline = in.Tagline
	}
	if in.ColorBackground != nil {
		if !isValidHex(*in.ColorBackground) {
			return nil, apperrors.ValidationFailed("color_background", "invalid hex color")
		}
		b.ColorBackground = *in.ColorBackground
	}
	if in.ColorText != nil {
		if !isValidHex(*in.ColorText) {
			return nil, apperrors.ValidationFailed("color_text", "invalid hex color")
		}
		b.ColorText = *in.ColorText
	}
	if in.ColorAccent != nil {
		if !isValidHex(*in.ColorAccent) {
			return nil, apperrors.ValidationFailed("color_accent", "invalid hex color")
		}
		b.ColorAccent = *in.ColorAccent
	}
	if in.ColorButtonBg != nil {
		if !isValidHex(*in.ColorButtonBg) {
			return nil, apperrors.ValidationFailed("color_button_bg", "invalid hex color")
		}
		b.ColorButtonBg = *in.ColorButtonBg
	}
	if in.ColorButtonText != nil {
		if !isValidHex(*in.ColorButtonText) {
			return nil, apperrors.ValidationFailed("color_button_text", "invalid hex color")
		}
		b.ColorButtonText = *in.ColorButtonText
	}
	if in.HeadingFont != nil {
		if !AllowedFonts[*in.HeadingFont] {
			return nil, apperrors.ValidationFailed("heading_font", "font not in allowed list")
		}
		b.HeadingFont = *in.HeadingFont
	}
	if in.BodyFont != nil {
		if !AllowedFonts[*in.BodyFont] {
			return nil, apperrors.ValidationFailed("body_font", "font not in allowed list")
		}
		b.BodyFont = *in.BodyFont
	}
	if in.LayoutVariant != nil {
		if !AllowedLayouts[*in.LayoutVariant] {
			return nil, apperrors.ValidationFailed("layout_variant", "layout not in allowed list")
		}
		b.LayoutVariant = *in.LayoutVariant
	}
	if in.HeroImageURL != nil {
		b.HeroImageURL = in.HeroImageURL
	}
	if in.AnnouncementText != nil {
		b.AnnouncementText = in.AnnouncementText
	}
	if in.AnnouncementLink != nil {
		b.AnnouncementLink = in.AnnouncementLink
	}
	if in.AnnouncementBg != nil {
		if !isValidHex(*in.AnnouncementBg) {
			return nil, apperrors.ValidationFailed("announcement_bg", "invalid hex color")
		}
		b.AnnouncementBg = in.AnnouncementBg
	}
	if in.AnnouncementActive != nil {
		b.AnnouncementActive = *in.AnnouncementActive
	}
	if in.FooterTagline != nil {
		b.FooterTagline = in.FooterTagline
	}
	if in.FooterCopyright != nil {
		b.FooterCopyright = in.FooterCopyright
	}
	if in.SocialInstagram != nil {
		b.SocialInstagram = in.SocialInstagram
	}
	if in.SocialTwitter != nil {
		b.SocialTwitter = in.SocialTwitter
	}
	if in.SocialFacebook != nil {
		b.SocialFacebook = in.SocialFacebook
	}
	if in.SocialTiktok != nil {
		b.SocialTiktok = in.SocialTiktok
	}
	if in.SocialYoutube != nil {
		b.SocialYoutube = in.SocialYoutube
	}
	if in.CustomCSS != nil {
		sanitized := SanitizeCSS(*in.CustomCSS)
		b.CustomCSS = &sanitized
	}
	if in.ShowPoweredBy != nil {
		b.ShowPoweredBy = *in.ShowPoweredBy
	}

	// WCAG AA contrast validation.
	if err := validateContrast(b); err != nil {
		return nil, err
	}

	if err := s.repo.Upsert(ctx, s.db, b); err != nil {
		return nil, err
	}
	return b, nil
}

func defaultBranding(storeID uuid.UUID) *StoreBranding {
	return &StoreBranding{
		StoreID:         storeID,
		ColorBackground: "#F7F6F2",
		ColorText:       "#0E0E0C",
		ColorAccent:     "#2D4A2B",
		ColorButtonBg:   "#0E0E0C",
		ColorButtonText: "#F7F6F2",
		HeadingFont:     "source-serif-4",
		BodyFont:        "source-sans-3",
		LayoutVariant:   "classic-shop",
		ShowPoweredBy:   true,
	}
}

// --- Contrast validation (WCAG AA) ---

func validateContrast(b *StoreBranding) error {
	// Text on background: >= 4.5:1
	if ratio := contrastRatio(b.ColorText, b.ColorBackground); ratio < 4.5 {
		return apperrors.ValidationFailed("color_text",
			fmt.Sprintf("text/background contrast ratio %.1f:1 is below WCAG AA minimum 4.5:1", ratio))
	}
	// Button text on button background: >= 4.5:1
	if ratio := contrastRatio(b.ColorButtonText, b.ColorButtonBg); ratio < 4.5 {
		return apperrors.ValidationFailed("color_button_text",
			fmt.Sprintf("button text/background contrast ratio %.1f:1 is below WCAG AA minimum 4.5:1", ratio))
	}
	// Accent on background: >= 3:1
	if ratio := contrastRatio(b.ColorAccent, b.ColorBackground); ratio < 3.0 {
		return apperrors.ValidationFailed("color_accent",
			fmt.Sprintf("accent/background contrast ratio %.1f:1 is below WCAG AA minimum 3:1", ratio))
	}
	return nil
}

// contrastRatio calculates the WCAG 2.x contrast ratio between two hex colors.
func contrastRatio(hex1, hex2 string) float64 {
	l1 := relativeLuminance(hex1)
	l2 := relativeLuminance(hex2)
	lighter := math.Max(l1, l2)
	darker := math.Min(l1, l2)
	return (lighter + 0.05) / (darker + 0.05)
}

func relativeLuminance(hex string) float64 {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) != 6 {
		return 0
	}
	r, _ := strconv.ParseInt(hex[0:2], 16, 64)
	g, _ := strconv.ParseInt(hex[2:4], 16, 64)
	b, _ := strconv.ParseInt(hex[4:6], 16, 64)

	rs := linearize(float64(r) / 255.0)
	gs := linearize(float64(g) / 255.0)
	bs := linearize(float64(b) / 255.0)

	return 0.2126*rs + 0.7152*gs + 0.0722*bs
}

func linearize(c float64) float64 {
	if c <= 0.04045 {
		return c / 12.92
	}
	return math.Pow((c+0.055)/1.055, 2.4)
}

var hexColorRE = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

func isValidHex(s string) bool {
	return hexColorRE.MatchString(s)
}

// --- Custom CSS sanitization (Enterprise) ---

var unsafeCSSPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)@import\b`),
	regexp.MustCompile(`(?i)expression\s*\(`),
	regexp.MustCompile(`(?i)javascript\s*:`),
	regexp.MustCompile(`(?i)behavior\s*:`),
}

// externalURLPattern matches url() with non-GCS domains.
var externalURLPattern = regexp.MustCompile(`(?i)url\s*\(\s*['"]?https?://(?!storage\.googleapis\.com)`)

// SanitizeCSS strips dangerous patterns from custom CSS input.
func SanitizeCSS(css string) string {
	for _, p := range unsafeCSSPatterns {
		css = p.ReplaceAllString(css, "/* removed */")
	}
	css = externalURLPattern.ReplaceAllString(css, "/* removed: external url(")
	return css
}
