// Package admin — branding.go: HTTP handler for store branding
// endpoints (B1 — Storefront Branding).
package admin

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/branding"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// BrandingHandler handles /admin/stores/:storeId/branding endpoints.
type BrandingHandler struct {
	svc    *branding.Service
	logger *slog.Logger
}

// NewBrandingHandler constructs a BrandingHandler.
func NewBrandingHandler(svc *branding.Service, logger *slog.Logger) *BrandingHandler {
	return &BrandingHandler{svc: svc, logger: logger}
}

// BrandingResponse is the wire DTO for store branding.
type BrandingResponse struct {
	ID                 string                   `json:"id"`
	StoreID            string                   `json:"store_id"`
	LogoURL            *string                  `json:"logo_url,omitempty"`
	FaviconURL         *string                  `json:"favicon_url,omitempty"`
	Tagline            *string                  `json:"tagline,omitempty"`
	ColorBackground    string                   `json:"color_background"`
	ColorText          string                   `json:"color_text"`
	ColorAccent        string                   `json:"color_accent"`
	ColorButtonBg      string                   `json:"color_button_bg"`
	ColorButtonText    string                   `json:"color_button_text"`
	HeadingFont        string                   `json:"heading_font"`
	BodyFont           string                   `json:"body_font"`
	LayoutVariant      string                   `json:"layout_variant"`
	HeroImageURL       *string                  `json:"hero_image_url,omitempty"`
	AnnouncementText   *string                  `json:"announcement_text,omitempty"`
	AnnouncementLink   *string                  `json:"announcement_link,omitempty"`
	AnnouncementBg     *string                  `json:"announcement_bg,omitempty"`
	AnnouncementActive bool                     `json:"announcement_active"`
	FooterTagline      *string                  `json:"footer_tagline,omitempty"`
	FooterCopyright    *string                  `json:"footer_copyright,omitempty"`
	FooterSections     []branding.FooterSection `json:"footer_sections,omitempty"`
	SocialInstagram    *string                  `json:"social_instagram,omitempty"`
	SocialTwitter      *string                  `json:"social_twitter,omitempty"`
	SocialFacebook     *string                  `json:"social_facebook,omitempty"`
	SocialTiktok       *string                  `json:"social_tiktok,omitempty"`
	SocialYoutube      *string                  `json:"social_youtube,omitempty"`
	CustomCSS          *string                  `json:"custom_css,omitempty"`
	ShowPoweredBy      bool                     `json:"show_powered_by"`
	CreatedAt          string                   `json:"created_at"`
	UpdatedAt          string                   `json:"updated_at"`
}

func toBrandingResponse(b branding.StoreBranding) BrandingResponse {
	resp := BrandingResponse{
		ID:                 b.ID.String(),
		StoreID:            b.StoreID.String(),
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
		CreatedAt:          b.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:          b.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
	if sections, err := branding.ParseFooterSections(b.FooterSections); err == nil {
		resp.FooterSections = sections
	}
	return resp
}

// Get handles GET /admin/stores/:storeId/branding.
func (h *BrandingHandler) Get(c *gin.Context) {
	storeID, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("storeId", "invalid uuid"), h.logger)
		return
	}

	b, err := h.svc.Get(c.Request.Context(), storeID)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	c.JSON(http.StatusOK, toBrandingResponse(*b))
}

// UpdateBrandingRequest is the request body for PUT .../branding.
type UpdateBrandingRequest struct {
	LogoURL            *string                   `json:"logo_url"`
	FaviconURL         *string                   `json:"favicon_url"`
	Tagline            *string                   `json:"tagline"`
	ColorBackground    *string                   `json:"color_background"`
	ColorText          *string                   `json:"color_text"`
	ColorAccent        *string                   `json:"color_accent"`
	ColorButtonBg      *string                   `json:"color_button_bg"`
	ColorButtonText    *string                   `json:"color_button_text"`
	HeadingFont        *string                   `json:"heading_font"`
	BodyFont           *string                   `json:"body_font"`
	LayoutVariant      *string                   `json:"layout_variant"`
	HeroImageURL       *string                   `json:"hero_image_url"`
	AnnouncementText   *string                   `json:"announcement_text"`
	AnnouncementLink   *string                   `json:"announcement_link"`
	AnnouncementBg     *string                   `json:"announcement_bg"`
	AnnouncementActive *bool                     `json:"announcement_active"`
	FooterTagline      *string                   `json:"footer_tagline"`
	FooterCopyright    *string                   `json:"footer_copyright"`
	FooterSections     *[]branding.FooterSection `json:"footer_sections"`
	SocialInstagram    *string                   `json:"social_instagram"`
	SocialTwitter      *string                   `json:"social_twitter"`
	SocialFacebook     *string                   `json:"social_facebook"`
	SocialTiktok       *string                   `json:"social_tiktok"`
	SocialYoutube      *string                   `json:"social_youtube"`
	CustomCSS          *string                   `json:"custom_css"`
	ShowPoweredBy      *bool                     `json:"show_powered_by"`
}

// Update handles PUT /admin/stores/:storeId/branding.
func (h *BrandingHandler) Update(c *gin.Context) {
	storeID, err := uuid.Parse(c.Param("storeId"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("storeId", "invalid uuid"), h.logger)
		return
	}
	tenantID, err := uuid.Parse(c.GetString("tenant_id"))
	if err != nil {
		RespondErr(c, apperrors.ValidationFailed("tenant_id", "invalid uuid"), h.logger)
		return
	}

	var req UpdateBrandingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", "invalid request body"), h.logger)
		return
	}

	b, err := h.svc.Update(c.Request.Context(), branding.UpdateInput{
		TenantID:           tenantID,
		StoreID:            storeID,
		LogoURL:            req.LogoURL,
		FaviconURL:         req.FaviconURL,
		Tagline:            req.Tagline,
		ColorBackground:    req.ColorBackground,
		ColorText:          req.ColorText,
		ColorAccent:        req.ColorAccent,
		ColorButtonBg:      req.ColorButtonBg,
		ColorButtonText:    req.ColorButtonText,
		HeadingFont:        req.HeadingFont,
		BodyFont:           req.BodyFont,
		LayoutVariant:      req.LayoutVariant,
		HeroImageURL:       req.HeroImageURL,
		AnnouncementText:   req.AnnouncementText,
		AnnouncementLink:   req.AnnouncementLink,
		AnnouncementBg:     req.AnnouncementBg,
		AnnouncementActive: req.AnnouncementActive,
		FooterTagline:      req.FooterTagline,
		FooterCopyright:    req.FooterCopyright,
		FooterSections:     req.FooterSections,
		SocialInstagram:    req.SocialInstagram,
		SocialTwitter:      req.SocialTwitter,
		SocialFacebook:     req.SocialFacebook,
		SocialTiktok:       req.SocialTiktok,
		SocialYoutube:      req.SocialYoutube,
		CustomCSS:          req.CustomCSS,
		ShowPoweredBy:      req.ShowPoweredBy,
	})
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	c.JSON(http.StatusOK, toBrandingResponse(*b))
}
