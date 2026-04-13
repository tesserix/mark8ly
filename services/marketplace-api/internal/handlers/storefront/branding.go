// Package storefront — branding.go: public branding endpoint for B1.
// Returns store branding with cache headers (5-min TTL).
package storefront

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/branding"
	"github.com/mark8ly/marketplace-api/internal/campaign"
	"github.com/mark8ly/marketplace-api/internal/coupon"
	"github.com/mark8ly/marketplace-api/internal/stores"
)

// BrandingHandler handles /storefront/stores/:storeSlug/branding.
type BrandingHandler struct {
	svc          *branding.Service
	campaignRepo campaign.Repository
	couponRepo   coupon.Repository
	db           *gorm.DB
	logger       *slog.Logger
}

// NewBrandingHandler constructs a storefront BrandingHandler.
func NewBrandingHandler(svc *branding.Service, db *gorm.DB, campaignRepo campaign.Repository, couponRepo coupon.Repository, logger *slog.Logger) *BrandingHandler {
	return &BrandingHandler{svc: svc, db: db, campaignRepo: campaignRepo, couponRepo: couponRepo, logger: logger}
}

// ActivePromotionResponse is the storefront promotion derived from a campaign.
type ActivePromotionResponse struct {
	Label      string  `json:"label"`
	CouponCode *string `json:"coupon_code,omitempty"`
	Link       *string `json:"link,omitempty"`
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
	storeVal, exists := c.Get("store")
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "not_found", "message": "store not found"})
		return
	}
	store, ok := storeVal.(*stores.Store)
	if !ok || store == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not_found", "message": "store not found"})
		return
	}
	storeID, err := uuid.Parse(store.ID)
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

	resp := gin.H{
		"branding": toPublicBrandingResponse(*b),
	}

	// Look up active storefront promotion from campaigns.
	if h.campaignRepo != nil {
		promo, err := h.campaignRepo.FindActiveStorefrontPromotion(c.Request.Context(), h.db, storeID)
		if err != nil {
			h.logger.Error("storefront branding: active promotion lookup", "err", err)
		}
		if promo != nil && promo.StorefrontLabel != nil {
			ap := ActivePromotionResponse{
				Label: *promo.StorefrontLabel,
			}
			// Resolve coupon code if campaign has a linked coupon.
			if promo.CouponID != nil && h.couponRepo != nil {
				cp, err := h.couponRepo.GetByID(c.Request.Context(), h.db, promo.StoreID, *promo.CouponID)
				if err == nil && cp != nil {
					ap.CouponCode = &cp.Code
				}
			}
			resp["active_promotion"] = ap
		}
	}

	c.Header("Cache-Control", "public, max-age=300")
	c.JSON(http.StatusOK, resp)
}
