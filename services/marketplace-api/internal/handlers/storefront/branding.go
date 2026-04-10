// Package storefront — branding.go: public branding endpoint for B1.
// Returns store branding with cache headers (5-min TTL).
package storefront

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/branding"
)

// BrandingHandler handles /storefront/stores/:storeSlug/branding.
type BrandingHandler struct {
	svc    *branding.Service
	logger *slog.Logger
}

// NewBrandingHandler constructs a storefront BrandingHandler.
func NewBrandingHandler(svc *branding.Service, logger *slog.Logger) *BrandingHandler {
	return &BrandingHandler{svc: svc, logger: logger}
}

// PublicBrandingResponse is the public wire DTO — excludes custom_css
// and admin-only fields.
type PublicBrandingResponse struct {
	LogoURL            *string `json:"logo_url,omitempty"`
	FaviconURL         *string `json:"favicon_url,omitempty"`
	Tagline            *string `json:"tagline,omitempty"`
	ColorBackground    string  `json:"color_background"`
	ColorText          string  `json:"color_text"`
	ColorAccent        string  `json:"color_accent"`
	ColorButtonBg      string  `json:"color_button_bg"`
	ColorButtonText    string  `json:"color_button_text"`
	HeadingFont        string  `json:"heading_font"`
	BodyFont           string  `json:"body_font"`
	LayoutVariant      string  `json:"layout_variant"`
	HeroImageURL       *string `json:"hero_image_url,omitempty"`
	AnnouncementText   *string `json:"announcement_text,omitempty"`
	AnnouncementLink   *string `json:"announcement_link,omitempty"`
	AnnouncementBg     *string `json:"announcement_bg,omitempty"`
	AnnouncementActive bool    `json:"announcement_active"`
	FooterTagline      *string `json:"footer_tagline,omitempty"`
	FooterCopyright    *string `json:"footer_copyright,omitempty"`
	SocialInstagram    *string `json:"social_instagram,omitempty"`
	SocialTwitter      *string `json:"social_twitter,omitempty"`
	SocialFacebook     *string `json:"social_facebook,omitempty"`
	SocialTiktok       *string `json:"social_tiktok,omitempty"`
	SocialYoutube      *string `json:"social_youtube,omitempty"`
	CustomCSS          *string `json:"custom_css,omitempty"`
	ShowPoweredBy      bool    `json:"show_powered_by"`
}

func toPublicBrandingResponse(b branding.StoreBranding) PublicBrandingResponse {
	return PublicBrandingResponse{
		LogoURL:            b.LogoURL,
		FaviconURL:         b.FaviconURL,
		Tagline:            b.Tagline,
		ColorBackground:    b.ColorBackground,
		ColorText:          b.ColorText,
		ColorAccent:        b.ColorAccent,
		ColorButtonBg:      b.ColorButtonBg,
		ColorButtonText:    b.ColorButtonText,
		HeadingFont:        b.HeadingFont,
		BodyFont:           b.BodyFont,
		LayoutVariant:      b.LayoutVariant,
		HeroImageURL:       b.HeroImageURL,
		AnnouncementText:   b.AnnouncementText,
		AnnouncementLink:   b.AnnouncementLink,
		AnnouncementBg:     b.AnnouncementBg,
		AnnouncementActive: b.AnnouncementActive,
		FooterTagline:      b.FooterTagline,
		FooterCopyright:    b.FooterCopyright,
		SocialInstagram:    b.SocialInstagram,
		SocialTwitter:      b.SocialTwitter,
		SocialFacebook:     b.SocialFacebook,
		SocialTiktok:       b.SocialTiktok,
		SocialYoutube:      b.SocialYoutube,
		CustomCSS:          b.CustomCSS,
		ShowPoweredBy:      b.ShowPoweredBy,
	}
}

// Get handles GET /storefront/stores/:storeSlug/branding.
// Returns branding with a 5-minute cache header.
func (h *BrandingHandler) Get(c *gin.Context) {
	storeIDStr, exists := c.Get("store_id")
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "not_found", "message": "store not found"})
		return
	}
	storeID, err := uuid.Parse(storeIDStr.(string))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not_found", "message": "store not found"})
		return
	}

	b, err := h.svc.GetByStoreID(c.Request.Context(), storeID)
	if err != nil {
		h.logger.Error("storefront branding get", "err", err, "store_id", storeID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal", "message": "failed to load branding"})
		return
	}

	c.Header("Cache-Control", "public, max-age=300")
	c.JSON(http.StatusOK, toPublicBrandingResponse(*b))
}
