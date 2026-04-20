// Package admin — branding.go: HTTP handler for store branding
// endpoints (B1 — Storefront Branding).
package admin

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/branding"
	"github.com/mark8ly/marketplace-api/internal/media"
	"github.com/mark8ly/marketplace-api/internal/plangate"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// BrandingHandler handles /admin/stores/:storeId/branding endpoints.
type BrandingHandler struct {
	svc      *branding.Service
	uploader media.Uploader         // optional; only used by UploadURL
	audit    *audit.Emitter         // optional — nil-safe
	resolver *plangate.PlanResolver // optional — enables plan-tier gating on paid-tier branding fields
	logger   *slog.Logger
}

// NewBrandingHandler constructs a BrandingHandler.
func NewBrandingHandler(svc *branding.Service, logger *slog.Logger) *BrandingHandler {
	return &BrandingHandler{svc: svc, logger: logger}
}

// SetUploader wires the media uploader for the upload-url endpoint.
// Separate setter so main.go can keep existing call sites unchanged.
func (h *BrandingHandler) SetUploader(u media.Uploader) { h.uploader = u }

// SetPlanResolver wires the plan resolver used to gate paid-tier branding
// fields (e.g. custom_css on Studio+). Without a resolver, field-level
// plan gates are skipped — production must always wire this.
func (h *BrandingHandler) SetPlanResolver(r *plangate.PlanResolver) { h.resolver = r }

// WithAudit attaches an audit emitter so branding changes are recorded.
// Nil-safe.
func (h *BrandingHandler) WithAudit(e *audit.Emitter) *BrandingHandler {
	h.audit = e
	return h
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
	// SEO + AI SEO
	SeoTitleTemplate      *string `json:"seo_title_template,omitempty"`
	SeoDefaultDescription *string `json:"seo_default_description,omitempty"`
	SeoOgImageURL         *string `json:"seo_og_image_url,omitempty"`
	SeoTwitterHandle      *string `json:"seo_twitter_handle,omitempty"`
	SeoGoogleVerification *string `json:"seo_google_verification,omitempty"`
	SeoBingVerification   *string `json:"seo_bing_verification,omitempty"`
	SeoJsonLd             *string `json:"seo_json_ld,omitempty"`
	SeoAiPolicy           string  `json:"seo_ai_policy"`
	SeoLlmsTxt            *string `json:"seo_llms_txt,omitempty"`
	ReturnPolicy          *string `json:"return_policy,omitempty"`
	HomepageContent    json.RawMessage          `json:"homepage_content"`
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
		SeoTitleTemplate:      b.SeoTitleTemplate,
		SeoDefaultDescription: b.SeoDefaultDescription,
		SeoOgImageURL:         b.SeoOgImageURL,
		SeoTwitterHandle:      b.SeoTwitterHandle,
		SeoGoogleVerification: b.SeoGoogleVerification,
		SeoBingVerification:   b.SeoBingVerification,
		SeoJsonLd:             b.SeoJsonLd,
		SeoAiPolicy:           b.SeoAiPolicy,
		SeoLlmsTxt:            b.SeoLlmsTxt,
		ReturnPolicy:          b.ReturnPolicy,
		CreatedAt:          b.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:          b.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
	if sections, err := branding.ParseFooterSections(b.FooterSections); err == nil {
		resp.FooterSections = sections
	}
	if len(b.HomepageContent) > 0 {
		resp.HomepageContent = json.RawMessage(b.HomepageContent)
	} else {
		resp.HomepageContent = json.RawMessage(`{}`)
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
	SeoTitleTemplate      *string `json:"seo_title_template"`
	SeoDefaultDescription *string `json:"seo_default_description"`
	SeoOgImageURL         *string `json:"seo_og_image_url"`
	SeoTwitterHandle      *string `json:"seo_twitter_handle"`
	SeoGoogleVerification *string `json:"seo_google_verification"`
	SeoBingVerification   *string `json:"seo_bing_verification"`
	SeoJsonLd             *string `json:"seo_json_ld"`
	SeoAiPolicy           *string `json:"seo_ai_policy"`
	SeoLlmsTxt            *string `json:"seo_llms_txt"`
	ReturnPolicy          *string `json:"return_policy"`
	HomepageContent    *json.RawMessage          `json:"homepage_content"`
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

	// Field-level plan gate: CustomCSS is Studio+. A field-level (not
	// route-level) gate is used because this PUT mutates many fields at
	// once and Starter merchants must still be able to update non-paid
	// fields (logo, colors, social links). Clearing the value is always
	// permitted — only non-empty writes require the feature.
	if h.resolver != nil && req.CustomCSS != nil && strings.TrimSpace(*req.CustomCSS) != "" {
		plan := h.resolver.Resolve(c.Request.Context(), tenantID, storeID)
		if !plangate.IsAllowed(plan, plangate.FeatureCustomCSS) {
			minPlan := plangate.MinPlanForFeature(plangate.FeatureCustomCSS)
			c.JSON(http.StatusForbidden, gin.H{
				"error":    "plan_required",
				"message":  fmt.Sprintf("Custom CSS requires the %s plan or higher", minPlan),
				"required": string(minPlan),
				"current":  string(plan),
				"feature":  string(plangate.FeatureCustomCSS),
				"field":    "custom_css",
			})
			return
		}
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
		HomepageContent:    req.HomepageContent,
		SocialInstagram:    req.SocialInstagram,
		SocialTwitter:      req.SocialTwitter,
		SocialFacebook:     req.SocialFacebook,
		SocialTiktok:       req.SocialTiktok,
		SocialYoutube:      req.SocialYoutube,
		CustomCSS:          req.CustomCSS,
		ShowPoweredBy:      req.ShowPoweredBy,
		SeoTitleTemplate:      req.SeoTitleTemplate,
		SeoDefaultDescription: req.SeoDefaultDescription,
		SeoOgImageURL:         req.SeoOgImageURL,
		SeoTwitterHandle:      req.SeoTwitterHandle,
		SeoGoogleVerification: req.SeoGoogleVerification,
		SeoBingVerification:   req.SeoBingVerification,
		SeoJsonLd:             req.SeoJsonLd,
		SeoAiPolicy:           req.SeoAiPolicy,
		SeoLlmsTxt:            req.SeoLlmsTxt,
		ReturnPolicy:          req.ReturnPolicy,
	})
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	h.audit.Emit(c, audit.Event{
		Action:       "branding.updated",
		ResourceType: "branding",
		ResourceID:   b.ID.String(),
	})
	c.JSON(http.StatusOK, toBrandingResponse(*b))
}

// BrandingUploadURLRequest is the wire body for POST /branding/upload-url.
type BrandingUploadURLRequest struct {
	Filename    string `json:"filename" binding:"required,max=200"`
	ContentType string `json:"content_type" binding:"required,max=100"`
	Kind        string `json:"kind" binding:"required,oneof=logo favicon hero aside section"`
}

// BrandingUploadURLResponse is the wire response for POST /branding/upload-url.
type BrandingUploadURLResponse struct {
	UploadURL  string    `json:"upload_url"`
	PublicURL  string    `json:"public_url"`
	StorageKey string    `json:"storage_key"`
	ExpiresAt  time.Time `json:"expires_at"`
}

// brandingAllowedTypes maps each branding image kind to the set of
// accepted content-types. Raster-only for hero/aside/section; SVG is
// allowed for logo + favicon where the merchant's brand mark often
// arrives in SVG form.
var brandingAllowedTypes = map[string]map[string]struct{}{
	"logo":    {"image/png": {}, "image/jpeg": {}, "image/webp": {}, "image/svg+xml": {}},
	"favicon": {"image/png": {}, "image/jpeg": {}, "image/webp": {}, "image/svg+xml": {}, "image/x-icon": {}, "image/vnd.microsoft.icon": {}},
	"hero":    {"image/png": {}, "image/jpeg": {}, "image/webp": {}},
	"aside":   {"image/png": {}, "image/jpeg": {}, "image/webp": {}},
	"section": {"image/png": {}, "image/jpeg": {}, "image/webp": {}},
}

// UploadURL handles POST /admin/stores/:storeId/branding/upload-url.
// Returns a V4 signed PUT URL + the public URL to store on the branding
// record. No DB writes happen here — the merchant's next PUT /branding
// call persists the returned `public_url` in the appropriate field.
func (h *BrandingHandler) UploadURL(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		RespondErr(c, apperrors.ValidationFailed("tenant_id", "missing"), h.logger)
		return
	}

	var req BrandingUploadURLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}

	allowed, ok := brandingAllowedTypes[req.Kind]
	if !ok {
		RespondErr(c, apperrors.ValidationFailed("kind", "unknown kind"), h.logger)
		return
	}
	if _, allowedCT := allowed[req.ContentType]; !allowedCT {
		RespondErr(c, apperrors.ValidationFailed("content_type", "not allowed for this kind"), h.logger)
		return
	}

	if h.uploader == nil {
		c.AbortWithStatusJSON(http.StatusNotImplemented, gin.H{
			"error":   "not_implemented",
			"message": "uploads are disabled on this deployment",
		})
		return
	}
	signer, ok := h.uploader.(media.SignedURLGenerator)
	if !ok {
		c.AbortWithStatusJSON(http.StatusNotImplemented, gin.H{
			"error":   "not_implemented",
			"message": "signed upload URLs require a real GCS bucket",
		})
		return
	}
	bn, ok := h.uploader.(interface{ BucketName() string })
	if !ok {
		c.AbortWithStatusJSON(http.StatusNotImplemented, gin.H{
			"error":   "not_implemented",
			"message": "uploader does not expose bucket name",
		})
		return
	}

	key := buildBrandingStorageKey(tenantID, req.Kind, req.Filename)
	uploadURL, expiresAt, err := signer.SignedUploadURL(c.Request.Context(), key, req.ContentType, 15*time.Minute)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	publicURL := fmt.Sprintf("https://storage.googleapis.com/%s/%s", bn.BucketName(), key)
	c.JSON(http.StatusOK, BrandingUploadURLResponse{
		UploadURL:  uploadURL,
		PublicURL:  publicURL,
		StorageKey: key,
		ExpiresAt:  expiresAt,
	})
}

// buildBrandingStorageKey returns `tenants/<tenantId>/branding/<kind>/<uuid>.<ext>`.
// The ext comes from the filename (lowercased, ascii-safe, capped) so merchants
// get readable URLs; a random UUID prefix prevents collisions across reuploads.
func buildBrandingStorageKey(tenantID, kind, filename string) string {
	ext := strings.ToLower(path.Ext(filename))
	safe := make([]byte, 0, len(ext))
	for _, r := range ext {
		if r == '.' || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			safe = append(safe, byte(r))
			if len(safe) >= 8 {
				break
			}
		}
	}
	if len(safe) < 2 {
		safe = []byte(".bin")
	}
	return fmt.Sprintf("tenants/%s/branding/%s/%s%s", tenantID, kind, uuid.NewString(), string(safe))
}
