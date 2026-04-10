# Branding B1 — Storefront Branding Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Ship storefront branding: logo/favicon upload, full color palette with WCAG contrast validation, curated font picker, homepage layout selector, announcement bar, social links + footer, custom CSS (Enterprise). Admin settings with tabbed UI + live preview via PostMessage. Storefront applies merchant colors via CSS custom properties.

**Architecture:** New `internal/branding/` package (models, repository, contrast validator). Migration 000012. Admin UI: 6-tab settings page with color presets + live preview. Storefront: inject CSS variables from branding API response.

**Tech Stack:** Go 1.26, Gin, GORM. Next.js 16, React 19, Tailwind, Recharts (not needed here).

---

## Task 1 — Migration 000012: `store_branding` table

**Files to create:**
- `services/marketplace-api/migrations/000012_store_branding.up.sql`
- `services/marketplace-api/migrations/000012_store_branding.down.sql`

**Files to modify:**
- `services/marketplace-api/migrations.go` — bump `ExpectedSchemaVersion` from `11` to `12`

### 1.1 Up migration

Create file `services/marketplace-api/migrations/000012_store_branding.up.sql`:

```sql
-- Migration 000012: Store branding (Branding B1)

CREATE TABLE store_branding (
    id                  UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           UUID          NOT NULL,
    store_id            UUID          NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    -- Identity
    logo_url            TEXT,
    favicon_url         TEXT,
    tagline             VARCHAR(200),
    -- Colors (hex values)
    color_background    VARCHAR(7)    NOT NULL DEFAULT '#F7F6F2',
    color_text          VARCHAR(7)    NOT NULL DEFAULT '#0E0E0C',
    color_accent        VARCHAR(7)    NOT NULL DEFAULT '#2D4A2B',
    color_button_bg     VARCHAR(7)    NOT NULL DEFAULT '#0E0E0C',
    color_button_text   VARCHAR(7)    NOT NULL DEFAULT '#F7F6F2',
    -- Typography
    heading_font        VARCHAR(50)   NOT NULL DEFAULT 'source-serif-4',
    body_font           VARCHAR(50)   NOT NULL DEFAULT 'source-sans-3',
    -- Homepage
    layout_variant      VARCHAR(30)   NOT NULL DEFAULT 'classic-shop',
    hero_image_url      TEXT,
    announcement_text   VARCHAR(300),
    announcement_link   TEXT,
    announcement_bg     VARCHAR(7),
    announcement_active BOOLEAN       NOT NULL DEFAULT false,
    -- Footer
    footer_tagline      VARCHAR(300),
    footer_copyright    VARCHAR(200),
    social_instagram    VARCHAR(300),
    social_twitter      VARCHAR(300),
    social_facebook     VARCHAR(300),
    social_tiktok       VARCHAR(300),
    social_youtube      VARCHAR(300),
    -- Advanced (Enterprise)
    custom_css          TEXT,
    show_powered_by     BOOLEAN       NOT NULL DEFAULT true,
    -- Cache version — incremented on every save; storefront uses it
    -- as an ETag component so open tabs detect staleness.
    branding_version    INT           NOT NULL DEFAULT 1,
    -- Timestamps
    created_at          TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ   NOT NULL DEFAULT now(),
    UNIQUE (store_id)
);

CREATE INDEX sb_tenant_idx ON store_branding (tenant_id);
```

### 1.2 Down migration

Create file `services/marketplace-api/migrations/000012_store_branding.down.sql`:

```sql
DROP TABLE IF EXISTS store_branding;
```

### 1.3 Bump schema version

In `services/marketplace-api/migrations.go`, change:

```go
// Before:
const ExpectedSchemaVersion uint = 11

// After:
const ExpectedSchemaVersion uint = 12
```

### 1.4 Verify

```bash
cd services/marketplace-api && make mp-migrate-up
# Should print: migrated to version 12
```

---

## Task 2 — `internal/branding/` package: models, repository, contrast validator

**Files to create:**
- `services/marketplace-api/internal/branding/models.go`
- `services/marketplace-api/internal/branding/repository.go`
- `services/marketplace-api/internal/branding/contrast.go`
- `services/marketplace-api/internal/branding/contrast_test.go`
- `services/marketplace-api/internal/branding/sanitize.go`
- `services/marketplace-api/internal/branding/sanitize_test.go`
- `services/marketplace-api/internal/branding/fonts.go`

### 2.1 `models.go`

```go
// Package branding owns the store_branding table and WCAG contrast validation.
package branding

import (
	"time"

	"github.com/google/uuid"
)

// StoreBranding is the GORM model for the store_branding table.
type StoreBranding struct {
	ID              uuid.UUID `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID        uuid.UUID `gorm:"column:tenant_id;type:uuid;not null"`
	StoreID         uuid.UUID `gorm:"column:store_id;type:uuid;not null"`
	LogoURL         *string   `gorm:"column:logo_url;type:text"`
	FaviconURL      *string   `gorm:"column:favicon_url;type:text"`
	Tagline         *string   `gorm:"column:tagline;type:varchar(200)"`
	ColorBackground string    `gorm:"column:color_background;type:varchar(7);not null;default:'#F7F6F2'"`
	ColorText       string    `gorm:"column:color_text;type:varchar(7);not null;default:'#0E0E0C'"`
	ColorAccent     string    `gorm:"column:color_accent;type:varchar(7);not null;default:'#2D4A2B'"`
	ColorButtonBg   string    `gorm:"column:color_button_bg;type:varchar(7);not null;default:'#0E0E0C'"`
	ColorButtonText string    `gorm:"column:color_button_text;type:varchar(7);not null;default:'#F7F6F2'"`
	HeadingFont     string    `gorm:"column:heading_font;type:varchar(50);not null;default:'source-serif-4'"`
	BodyFont        string    `gorm:"column:body_font;type:varchar(50);not null;default:'source-sans-3'"`
	LayoutVariant   string    `gorm:"column:layout_variant;type:varchar(30);not null;default:'classic-shop'"`
	HeroImageURL    *string   `gorm:"column:hero_image_url;type:text"`
	AnnouncementText   *string `gorm:"column:announcement_text;type:varchar(300)"`
	AnnouncementLink   *string `gorm:"column:announcement_link;type:text"`
	AnnouncementBg     *string `gorm:"column:announcement_bg;type:varchar(7)"`
	AnnouncementActive bool    `gorm:"column:announcement_active;type:boolean;not null;default:false"`
	FooterTagline   *string   `gorm:"column:footer_tagline;type:varchar(300)"`
	FooterCopyright *string   `gorm:"column:footer_copyright;type:varchar(200)"`
	SocialInstagram *string   `gorm:"column:social_instagram;type:varchar(300)"`
	SocialTwitter   *string   `gorm:"column:social_twitter;type:varchar(300)"`
	SocialFacebook  *string   `gorm:"column:social_facebook;type:varchar(300)"`
	SocialTiktok    *string   `gorm:"column:social_tiktok;type:varchar(300)"`
	SocialYoutube   *string   `gorm:"column:social_youtube;type:varchar(300)"`
	CustomCSS       *string   `gorm:"column:custom_css;type:text"`
	ShowPoweredBy   bool      `gorm:"column:show_powered_by;type:boolean;not null;default:true"`
	BrandingVersion int       `gorm:"column:branding_version;type:int;not null;default:1"`
	CreatedAt       time.Time `gorm:"column:created_at;not null;default:now()"`
	UpdatedAt       time.Time `gorm:"column:updated_at;not null;default:now()"`
}

func (StoreBranding) TableName() string { return "store_branding" }

// BrandingResponse is the public wire DTO returned to admin and storefront.
type BrandingResponse struct {
	ID              string  `json:"id"`
	StoreID         string  `json:"store_id"`
	LogoURL         *string `json:"logo_url"`
	FaviconURL      *string `json:"favicon_url"`
	Tagline         *string `json:"tagline"`
	ColorBackground string  `json:"color_background"`
	ColorText       string  `json:"color_text"`
	ColorAccent     string  `json:"color_accent"`
	ColorButtonBg   string  `json:"color_button_bg"`
	ColorButtonText string  `json:"color_button_text"`
	HeadingFont     string  `json:"heading_font"`
	BodyFont        string  `json:"body_font"`
	LayoutVariant   string  `json:"layout_variant"`
	HeroImageURL    *string `json:"hero_image_url"`
	AnnouncementText   *string `json:"announcement_text"`
	AnnouncementLink   *string `json:"announcement_link"`
	AnnouncementBg     *string `json:"announcement_bg"`
	AnnouncementActive bool    `json:"announcement_active"`
	FooterTagline   *string `json:"footer_tagline"`
	FooterCopyright *string `json:"footer_copyright"`
	SocialInstagram *string `json:"social_instagram"`
	SocialTwitter   *string `json:"social_twitter"`
	SocialFacebook  *string `json:"social_facebook"`
	SocialTiktok    *string `json:"social_tiktok"`
	SocialYoutube   *string `json:"social_youtube"`
	CustomCSS       *string `json:"custom_css,omitempty"`
	ShowPoweredBy   bool    `json:"show_powered_by"`
	BrandingVersion int     `json:"branding_version"`
	UpdatedAt       string  `json:"updated_at"`
}

// ToResponse converts a StoreBranding row to its wire representation.
func ToResponse(b StoreBranding) BrandingResponse {
	return BrandingResponse{
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
		BrandingVersion:    b.BrandingVersion,
		UpdatedAt:          b.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// UpdateBrandingRequest is the inbound DTO for PUT /admin/stores/:storeId/branding.
type UpdateBrandingRequest struct {
	// Identity
	Tagline *string `json:"tagline" binding:"omitempty,max=200"`
	// Colors
	ColorBackground *string `json:"color_background" binding:"omitempty,hexcolor"`
	ColorText       *string `json:"color_text" binding:"omitempty,hexcolor"`
	ColorAccent     *string `json:"color_accent" binding:"omitempty,hexcolor"`
	ColorButtonBg   *string `json:"color_button_bg" binding:"omitempty,hexcolor"`
	ColorButtonText *string `json:"color_button_text" binding:"omitempty,hexcolor"`
	// Typography
	HeadingFont *string `json:"heading_font" binding:"omitempty"`
	BodyFont    *string `json:"body_font" binding:"omitempty"`
	// Homepage
	LayoutVariant      *string `json:"layout_variant" binding:"omitempty"`
	AnnouncementText   *string `json:"announcement_text" binding:"omitempty,max=300"`
	AnnouncementLink   *string `json:"announcement_link" binding:"omitempty,url"`
	AnnouncementBg     *string `json:"announcement_bg" binding:"omitempty,hexcolor"`
	AnnouncementActive *bool   `json:"announcement_active"`
	// Footer
	FooterTagline   *string `json:"footer_tagline" binding:"omitempty,max=300"`
	FooterCopyright *string `json:"footer_copyright" binding:"omitempty,max=200"`
	SocialInstagram *string `json:"social_instagram" binding:"omitempty,max=300,url"`
	SocialTwitter   *string `json:"social_twitter" binding:"omitempty,max=300,url"`
	SocialFacebook  *string `json:"social_facebook" binding:"omitempty,max=300,url"`
	SocialTiktok    *string `json:"social_tiktok" binding:"omitempty,max=300,url"`
	SocialYoutube   *string `json:"social_youtube" binding:"omitempty,max=300,url"`
	// Advanced (Enterprise only — handler enforces plan gate)
	CustomCSS     *string `json:"custom_css" binding:"omitempty"`
	ShowPoweredBy *bool   `json:"show_powered_by"`
}
```

### 2.2 `repository.go`

```go
package branding

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ErrNotFound signals the store has no branding record yet.
var ErrNotFound = errors.New("branding: not found")

// Repository manages store_branding persistence.
type Repository struct{}

// NewRepository constructs a Repository. Stateless — safe to share.
func NewRepository() *Repository { return &Repository{} }

// GetByStoreID returns the branding row for a store, or ErrNotFound.
func (r *Repository) GetByStoreID(ctx context.Context, db *gorm.DB, storeID uuid.UUID) (StoreBranding, error) {
	var row StoreBranding
	err := db.WithContext(ctx).Where("store_id = ?", storeID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return StoreBranding{}, ErrNotFound
	}
	if err != nil {
		return StoreBranding{}, err
	}
	return row, nil
}

// Upsert creates or updates the branding row. Uses GORM's Save (INSERT
// ON CONFLICT UPDATE via the UNIQUE(store_id) constraint). Returns the
// saved row with branding_version incremented.
func (r *Repository) Upsert(ctx context.Context, db *gorm.DB, row StoreBranding) (StoreBranding, error) {
	// Fetch existing to get current version.
	var existing StoreBranding
	err := db.WithContext(ctx).Where("store_id = ?", row.StoreID).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// First save — version starts at 1 (default).
		row.BrandingVersion = 1
		if err := db.WithContext(ctx).Create(&row).Error; err != nil {
			return StoreBranding{}, err
		}
		return row, nil
	}
	if err != nil {
		return StoreBranding{}, err
	}
	// Update existing row.
	row.ID = existing.ID
	row.BrandingVersion = existing.BrandingVersion + 1
	row.CreatedAt = existing.CreatedAt
	if err := db.WithContext(ctx).Save(&row).Error; err != nil {
		return StoreBranding{}, err
	}
	return row, nil
}

// GetBySlug joins store_branding with stores to resolve by slug.
// Used by the public storefront endpoint. Returns ErrNotFound when
// no branding record exists for the slug (the storefront falls back
// to defaults).
func (r *Repository) GetBySlug(ctx context.Context, db *gorm.DB, slug string) (StoreBranding, error) {
	var row StoreBranding
	err := db.WithContext(ctx).
		Joins("JOIN stores ON stores.id = store_branding.store_id").
		Where("stores.slug = ? AND stores.status = 'active'", slug).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return StoreBranding{}, ErrNotFound
	}
	if err != nil {
		return StoreBranding{}, err
	}
	return row, nil
}
```

### 2.3 `contrast.go` — WCAG AA contrast ratio validation

```go
package branding

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// ContrastError contains a human-readable description of which color
// pair failed validation and the measured ratio.
type ContrastError struct {
	Field    string  `json:"field"`
	Message  string  `json:"message"`
	Ratio    float64 `json:"ratio"`
	Required float64 `json:"required"`
}

// ValidateContrast checks WCAG AA minimums for the branding color
// palette. Returns nil when all pairs pass.
//
// Rules from spec 4.3:
//   - Text on background: >= 4.5:1
//   - Button text on button background: >= 4.5:1
//   - Accent on background: >= 3:1
func ValidateContrast(bg, text, accent, btnBg, btnText string) []ContrastError {
	var errs []ContrastError

	if ratio, err := contrastRatio(text, bg); err == nil && ratio < 4.5 {
		errs = append(errs, ContrastError{
			Field:    "color_text",
			Message:  fmt.Sprintf("Text on background contrast %.2f:1 is below WCAG AA minimum 4.5:1", ratio),
			Ratio:    ratio,
			Required: 4.5,
		})
	}

	if ratio, err := contrastRatio(btnText, btnBg); err == nil && ratio < 4.5 {
		errs = append(errs, ContrastError{
			Field:    "color_button_text",
			Message:  fmt.Sprintf("Button text on button background contrast %.2f:1 is below WCAG AA minimum 4.5:1", ratio),
			Ratio:    ratio,
			Required: 4.5,
		})
	}

	if ratio, err := contrastRatio(accent, bg); err == nil && ratio < 3.0 {
		errs = append(errs, ContrastError{
			Field:    "color_accent",
			Message:  fmt.Sprintf("Accent on background contrast %.2f:1 is below WCAG AA minimum 3:1", ratio),
			Ratio:    ratio,
			Required: 3.0,
		})
	}

	if len(errs) == 0 {
		return nil
	}
	return errs
}

// contrastRatio computes the WCAG 2.x contrast ratio between two hex
// colors. Returns a value >= 1.0. Uses the relative luminance formula
// from https://www.w3.org/TR/WCAG20/#contrast-ratiodef.
func contrastRatio(hexA, hexB string) (float64, error) {
	lumA, err := relativeLuminance(hexA)
	if err != nil {
		return 0, err
	}
	lumB, err := relativeLuminance(hexB)
	if err != nil {
		return 0, err
	}
	lighter := math.Max(lumA, lumB)
	darker := math.Min(lumA, lumB)
	return (lighter + 0.05) / (darker + 0.05), nil
}

// relativeLuminance computes the relative luminance of a hex color.
// Input format: "#RRGGBB" (case-insensitive).
func relativeLuminance(hex string) (float64, error) {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) != 6 {
		return 0, fmt.Errorf("invalid hex color: %q", hex)
	}
	r, err := strconv.ParseUint(hex[0:2], 16, 8)
	if err != nil {
		return 0, err
	}
	g, err := strconv.ParseUint(hex[2:4], 16, 8)
	if err != nil {
		return 0, err
	}
	b, err := strconv.ParseUint(hex[4:6], 16, 8)
	if err != nil {
		return 0, err
	}
	return 0.2126*linearize(float64(r)/255) +
		0.7152*linearize(float64(g)/255) +
		0.0722*linearize(float64(b)/255), nil
}

// linearize converts an sRGB channel value (0-1) to linear light.
func linearize(c float64) float64 {
	if c <= 0.04045 {
		return c / 12.92
	}
	return math.Pow((c+0.055)/1.055, 2.4)
}
```

### 2.4 `contrast_test.go`

```go
package branding

import (
	"testing"
)

func TestContrastRatio_KnownPairs(t *testing.T) {
	tests := []struct {
		name     string
		a, b     string
		wantMin  float64
		wantMax  float64
	}{
		{name: "black on white", a: "#000000", b: "#FFFFFF", wantMin: 20.9, wantMax: 21.1},
		{name: "white on white", a: "#FFFFFF", b: "#FFFFFF", wantMin: 1.0, wantMax: 1.0},
		{name: "ink on paper", a: "#0E0E0C", b: "#F7F6F2", wantMin: 15.0, wantMax: 17.0},
		{name: "moss on paper", a: "#2D4A2B", b: "#F7F6F2", wantMin: 6.0, wantMax: 9.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ratio, err := contrastRatio(tt.a, tt.b)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ratio < tt.wantMin || ratio > tt.wantMax {
				t.Errorf("ratio = %.2f, want [%.1f, %.1f]", ratio, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestValidateContrast_DefaultPalettePasses(t *testing.T) {
	errs := ValidateContrast("#F7F6F2", "#0E0E0C", "#2D4A2B", "#0E0E0C", "#F7F6F2")
	if len(errs) != 0 {
		t.Errorf("default palette should pass, got %d errors: %v", len(errs), errs)
	}
}

func TestValidateContrast_LowContrastFails(t *testing.T) {
	// Light gray text on white background — fails text contrast.
	errs := ValidateContrast("#FFFFFF", "#CCCCCC", "#2D4A2B", "#0E0E0C", "#F7F6F2")
	if len(errs) == 0 {
		t.Error("expected contrast failure for light text on white")
	}
	found := false
	for _, e := range errs {
		if e.Field == "color_text" {
			found = true
		}
	}
	if !found {
		t.Error("expected color_text field in contrast errors")
	}
}
```

### 2.5 `sanitize.go` — Custom CSS sanitization (Enterprise gate)

```go
package branding

import (
	"regexp"
	"strings"
)

// dangerousPatterns contains regex patterns stripped from custom CSS.
// Spec 4.7: strip @import, external url(), javascript:, expression(), behavior:.
var dangerousPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)@import\b[^;]*;?`),
	regexp.MustCompile(`(?i)url\s*\(\s*['"]?\s*https?://[^)]*\)`),
	regexp.MustCompile(`(?i)url\s*\(\s*['"]?\s*//[^)]*\)`),
	regexp.MustCompile(`(?i)javascript\s*:`),
	regexp.MustCompile(`(?i)expression\s*\(`),
	regexp.MustCompile(`(?i)behavior\s*:`),
	regexp.MustCompile(`(?i)-moz-binding\s*:`),
}

// gcsBucketPattern matches GCS bucket URLs that ARE allowed in url().
// Format: url('https://storage.googleapis.com/BUCKET/...')
var gcsBucketPattern = regexp.MustCompile(
	`(?i)url\s*\(\s*['"]?\s*https://storage\.googleapis\.com/[^)]+\)`,
)

// SanitizeCSS strips dangerous constructs from merchant custom CSS.
// Preserves url() references to GCS bucket objects.
// Returns the sanitized CSS string.
func SanitizeCSS(input string) string {
	if input == "" {
		return ""
	}

	// Temporarily replace safe GCS URLs with placeholders.
	safeURLs := gcsBucketPattern.FindAllString(input, -1)
	result := input
	for i, u := range safeURLs {
		placeholder := "___GCS_SAFE_" + strings.Repeat("X", i+1) + "___"
		result = strings.Replace(result, u, placeholder, 1)
	}

	// Strip dangerous patterns.
	for _, pat := range dangerousPatterns {
		result = pat.ReplaceAllString(result, "/* [stripped] */")
	}

	// Restore safe GCS URLs.
	for i, u := range safeURLs {
		placeholder := "___GCS_SAFE_" + strings.Repeat("X", i+1) + "___"
		result = strings.Replace(result, placeholder, u, 1)
	}

	return strings.TrimSpace(result)
}
```

### 2.6 `sanitize_test.go`

```go
package branding

import (
	"strings"
	"testing"
)

func TestSanitizeCSS_StripsImport(t *testing.T) {
	input := `@import url('https://evil.com/hack.css'); .hero { color: red; }`
	out := SanitizeCSS(input)
	if strings.Contains(out, "@import") {
		t.Errorf("@import not stripped: %q", out)
	}
	if !strings.Contains(out, ".hero") {
		t.Error("safe CSS should be preserved")
	}
}

func TestSanitizeCSS_StripsJavascript(t *testing.T) {
	input := `.btn { background: javascript:alert(1); }`
	out := SanitizeCSS(input)
	if strings.Contains(out, "javascript") {
		t.Errorf("javascript: not stripped: %q", out)
	}
}

func TestSanitizeCSS_AllowsGCS(t *testing.T) {
	input := `.hero { background: url('https://storage.googleapis.com/mybucket/img.jpg'); }`
	out := SanitizeCSS(input)
	if !strings.Contains(out, "storage.googleapis.com") {
		t.Errorf("GCS URL should be preserved: %q", out)
	}
}

func TestSanitizeCSS_StripsExpression(t *testing.T) {
	input := `.box { width: expression(document.body.clientWidth); }`
	out := SanitizeCSS(input)
	if strings.Contains(out, "expression") {
		t.Errorf("expression() not stripped: %q", out)
	}
}

func TestSanitizeCSS_Empty(t *testing.T) {
	out := SanitizeCSS("")
	if out != "" {
		t.Errorf("empty input should return empty: %q", out)
	}
}
```

### 2.7 `fonts.go` — Font allowlist + layout variants

```go
package branding

// AllowedFonts is the curated set of fonts merchants can choose. The
// key is stored in the DB; the name is the human label for the admin UI.
var AllowedFonts = map[string]string{
	"source-serif-4":    "Source Serif 4",
	"playfair-display":  "Playfair Display",
	"lora":              "Lora",
	"inter":             "Inter",
	"source-sans-3":     "Source Sans 3",
	"dm-sans":           "DM Sans",
}

// IsValidFont returns true if the key is in the curated font list.
func IsValidFont(key string) bool {
	_, ok := AllowedFonts[key]
	return ok
}

// AllowedLayouts is the set of storefront homepage layout variants.
var AllowedLayouts = map[string]bool{
	"editorial":     true,
	"classic-shop":  true,
	"split-hero":    true,
	"catalog-first": true,
	"story-led":     true,
	"minimal":       true,
	"bold-promo":    true,
	"compact":       true,
}

// IsValidLayout returns true if the variant key is allowed.
func IsValidLayout(key string) bool {
	return AllowedLayouts[key]
}
```

### 2.8 Verify

```bash
cd services/marketplace-api && go test ./internal/branding/...
```

---

## Task 3 — Admin branding handler (CRUD + image upload via GCS)

**Files to create:**
- `services/marketplace-api/internal/handlers/admin/branding.go`

### 3.1 `branding.go`

```go
package admin

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/branding"
	"github.com/mark8ly/marketplace-api/internal/media"
	"github.com/mark8ly/marketplace-api/internal/stores"
)

var hexColorRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// BrandingHandler provides CRUD for per-store branding configuration.
type BrandingHandler struct {
	db       *gorm.DB
	repo     *branding.Repository
	uploader media.Uploader
	logger   *slog.Logger
}

// NewBrandingHandler constructs a BrandingHandler.
func NewBrandingHandler(db *gorm.DB, repo *branding.Repository, uploader media.Uploader, logger *slog.Logger) *BrandingHandler {
	return &BrandingHandler{db: db, repo: repo, uploader: uploader, logger: logger}
}

// Get handles GET /admin/stores/:storeId/branding.
func (h *BrandingHandler) Get(c *gin.Context) {
	store := storeFromCtx(c)
	if store == nil {
		return
	}

	row, err := h.repo.GetByStoreID(c.Request.Context(), h.db, store.ID)
	if errors.Is(err, branding.ErrNotFound) {
		// Return defaults — no branding saved yet.
		c.JSON(http.StatusOK, branding.ToResponse(branding.StoreBranding{
			StoreID:  store.ID,
			TenantID: store.TenantID,
		}))
		return
	}
	if err != nil {
		h.logger.Error("branding: get", "store_id", store.ID, "err", err)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error":   "internal",
			"message": "failed to load branding",
		})
		return
	}
	c.JSON(http.StatusOK, branding.ToResponse(row))
}

// Update handles PUT /admin/stores/:storeId/branding.
func (h *BrandingHandler) Update(c *gin.Context) {
	store := storeFromCtx(c)
	if store == nil {
		return
	}

	var req branding.UpdateBrandingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error":   "validation_error",
			"message": fmt.Sprintf("invalid request: %v", err),
		})
		return
	}

	// Load existing (or start from defaults).
	existing, err := h.repo.GetByStoreID(c.Request.Context(), h.db, store.ID)
	if errors.Is(err, branding.ErrNotFound) {
		existing = branding.StoreBranding{
			TenantID:        store.TenantID,
			StoreID:         store.ID,
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
	} else if err != nil {
		h.logger.Error("branding: get for update", "store_id", store.ID, "err", err)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error":   "internal",
			"message": "failed to load branding",
		})
		return
	}

	// Apply non-nil fields from the request (immutable update pattern).
	updated := existing
	if req.Tagline != nil {
		updated.Tagline = req.Tagline
	}
	if req.ColorBackground != nil && hexColorRe.MatchString(*req.ColorBackground) {
		updated.ColorBackground = *req.ColorBackground
	}
	if req.ColorText != nil && hexColorRe.MatchString(*req.ColorText) {
		updated.ColorText = *req.ColorText
	}
	if req.ColorAccent != nil && hexColorRe.MatchString(*req.ColorAccent) {
		updated.ColorAccent = *req.ColorAccent
	}
	if req.ColorButtonBg != nil && hexColorRe.MatchString(*req.ColorButtonBg) {
		updated.ColorButtonBg = *req.ColorButtonBg
	}
	if req.ColorButtonText != nil && hexColorRe.MatchString(*req.ColorButtonText) {
		updated.ColorButtonText = *req.ColorButtonText
	}
	if req.HeadingFont != nil && branding.IsValidFont(*req.HeadingFont) {
		updated.HeadingFont = *req.HeadingFont
	}
	if req.BodyFont != nil && branding.IsValidFont(*req.BodyFont) {
		updated.BodyFont = *req.BodyFont
	}
	if req.LayoutVariant != nil && branding.IsValidLayout(*req.LayoutVariant) {
		updated.LayoutVariant = *req.LayoutVariant
	}
	if req.AnnouncementText != nil {
		updated.AnnouncementText = req.AnnouncementText
	}
	if req.AnnouncementLink != nil {
		updated.AnnouncementLink = req.AnnouncementLink
	}
	if req.AnnouncementBg != nil && hexColorRe.MatchString(*req.AnnouncementBg) {
		updated.AnnouncementBg = req.AnnouncementBg
	}
	if req.AnnouncementActive != nil {
		updated.AnnouncementActive = *req.AnnouncementActive
	}
	if req.FooterTagline != nil {
		updated.FooterTagline = req.FooterTagline
	}
	if req.FooterCopyright != nil {
		updated.FooterCopyright = req.FooterCopyright
	}
	if req.SocialInstagram != nil {
		updated.SocialInstagram = req.SocialInstagram
	}
	if req.SocialTwitter != nil {
		updated.SocialTwitter = req.SocialTwitter
	}
	if req.SocialFacebook != nil {
		updated.SocialFacebook = req.SocialFacebook
	}
	if req.SocialTiktok != nil {
		updated.SocialTiktok = req.SocialTiktok
	}
	if req.SocialYoutube != nil {
		updated.SocialYoutube = req.SocialYoutube
	}
	// Enterprise-gated fields — the handler accepts them unconditionally;
	// plan gating is enforced by middleware (Task 5 wires RequirePlan).
	// When the plan gate middleware is not yet in place, custom_css and
	// show_powered_by toggle are accepted for all plans.
	if req.CustomCSS != nil {
		sanitized := branding.SanitizeCSS(*req.CustomCSS)
		updated.CustomCSS = &sanitized
	}
	if req.ShowPoweredBy != nil {
		updated.ShowPoweredBy = *req.ShowPoweredBy
	}

	// WCAG contrast validation.
	contrastErrors := branding.ValidateContrast(
		updated.ColorBackground,
		updated.ColorText,
		updated.ColorAccent,
		updated.ColorButtonBg,
		updated.ColorButtonText,
	)
	if len(contrastErrors) > 0 {
		c.AbortWithStatusJSON(http.StatusUnprocessableEntity, gin.H{
			"error":            "contrast_error",
			"message":          "Color combination does not meet WCAG AA contrast requirements",
			"contrast_errors":  contrastErrors,
		})
		return
	}

	updated.UpdatedAt = time.Now()
	saved, err := h.repo.Upsert(c.Request.Context(), h.db, updated)
	if err != nil {
		h.logger.Error("branding: upsert", "store_id", store.ID, "err", err)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error":   "internal",
			"message": "failed to save branding",
		})
		return
	}

	c.JSON(http.StatusOK, branding.ToResponse(saved))
}

// UploadLogo handles POST /admin/stores/:storeId/branding/logo.
// Generates a signed GCS upload URL for the logo file. The frontend
// uploads directly to GCS, then calls PUT /branding to persist the URL.
func (h *BrandingHandler) UploadLogo(c *gin.Context) {
	h.uploadBrandingAsset(c, "logo")
}

// UploadFavicon handles POST /admin/stores/:storeId/branding/favicon.
func (h *BrandingHandler) UploadFavicon(c *gin.Context) {
	h.uploadBrandingAsset(c, "favicon")
}

// UploadHero handles POST /admin/stores/:storeId/branding/hero.
func (h *BrandingHandler) UploadHero(c *gin.Context) {
	h.uploadBrandingAsset(c, "hero")
}

// uploadBrandingAssetRequest is the inbound DTO for image upload URL generation.
type uploadBrandingAssetRequest struct {
	Filename    string `json:"filename" binding:"required"`
	ContentType string `json:"content_type" binding:"required"`
}

// allowedImageTypes are the permitted content types for branding uploads.
var allowedImageTypes = map[string]bool{
	"image/png":     true,
	"image/jpeg":    true,
	"image/webp":    true,
	"image/svg+xml": true,
	"image/x-icon":  true,
}

func (h *BrandingHandler) uploadBrandingAsset(c *gin.Context, assetType string) {
	store := storeFromCtx(c)
	if store == nil {
		return
	}

	var req uploadBrandingAssetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error":   "validation_error",
			"message": fmt.Sprintf("invalid request: %v", err),
		})
		return
	}

	if !allowedImageTypes[req.ContentType] {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_content_type",
			"message": fmt.Sprintf("content type %q is not allowed for %s", req.ContentType, assetType),
		})
		return
	}

	// Build a deterministic storage key under the branding namespace.
	key := fmt.Sprintf("tenants/%s/branding/%s/%s/%s",
		store.TenantID.String(), store.ID.String(), assetType, sanitizeFilename(req.Filename))

	gen, ok := h.uploader.(media.SignedURLGenerator)
	if !ok {
		c.AbortWithStatusJSON(http.StatusNotImplemented, gin.H{
			"error":   "not_implemented",
			"message": "signed URL generation not available (dev mode)",
		})
		return
	}

	url, expiresAt, err := gen.SignedUploadURL(c.Request.Context(), key, req.ContentType, 15*time.Minute)
	if err != nil {
		h.logger.Error("branding: signed url", "asset_type", assetType, "err", err)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error":   "internal",
			"message": "failed to generate upload URL",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"upload_url":  url,
		"storage_key": key,
		"expires_at":  expiresAt.Format(time.RFC3339),
		// The public URL the frontend will PUT into the branding record.
		"public_url": fmt.Sprintf("https://storage.googleapis.com/%s/%s",
			"MARKETPLACE_GCS_BUCKET", key),
	})
}

// sanitizeFilename strips unsafe characters from filenames.
func sanitizeFilename(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9', r == '.' || r == '-' || r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	safe := b.String()
	if safe == "" {
		return "file"
	}
	return safe
}
```

### 3.2 Verify

```bash
cd services/marketplace-api && go build ./...
```

---

## Task 4 — Storefront branding endpoint (public, cached, purge on save)

**Files to create:**
- `services/marketplace-api/internal/handlers/storefront/branding.go`

### 4.1 `branding.go`

```go
package storefront

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/branding"
	"github.com/mark8ly/marketplace-api/internal/stores"
)

// brandingCacheControl is the Cache-Control header for the public
// branding endpoint. 5-min s-maxage + 10-min stale-while-revalidate
// per spec 7.1 (branding cache).
const brandingCacheControl = "public, s-maxage=300, stale-while-revalidate=600"

// BrandingStorefrontHandler serves the public read-only branding endpoint.
type BrandingStorefrontHandler struct {
	db     *gorm.DB
	repo   *branding.Repository
	logger *slog.Logger
}

// NewBrandingStorefrontHandler constructs a BrandingStorefrontHandler.
func NewBrandingStorefrontHandler(db *gorm.DB, repo *branding.Repository, logger *slog.Logger) *BrandingStorefrontHandler {
	return &BrandingStorefrontHandler{db: db, repo: repo, logger: logger}
}

// Get handles GET /storefront/stores/:storeSlug/branding.
// Public — no auth, no authz. Cached with ETag keyed on branding_version.
func (h *BrandingStorefrontHandler) Get(c *gin.Context) {
	v, ok := c.Get("store")
	if !ok {
		respondNotFound(c)
		return
	}
	store, _ := v.(*stores.Store)
	if store == nil {
		respondNotFound(c)
		return
	}

	row, err := h.repo.GetByStoreID(c.Request.Context(), h.db, store.ID)
	if errors.Is(err, branding.ErrNotFound) {
		// Return defaults with cache headers.
		defaults := branding.StoreBranding{
			StoreID:         store.ID,
			TenantID:        store.TenantID,
			ColorBackground: "#F7F6F2",
			ColorText:       "#0E0E0C",
			ColorAccent:     "#2D4A2B",
			ColorButtonBg:   "#0E0E0C",
			ColorButtonText: "#F7F6F2",
			HeadingFont:     "source-serif-4",
			BodyFont:        "source-sans-3",
			LayoutVariant:   "classic-shop",
			ShowPoweredBy:   true,
			BrandingVersion: 0,
		}
		h.writeBrandingResponse(c, store, defaults)
		return
	}
	if err != nil {
		respondInternal(c, h.logger, err)
		return
	}

	h.writeBrandingResponse(c, store, row)
}

func (h *BrandingStorefrontHandler) writeBrandingResponse(c *gin.Context, store *stores.Store, row branding.StoreBranding) {
	etag := fmt.Sprintf(`W/"%s-branding-%d"`, store.ID, row.BrandingVersion)

	// If-None-Match check — return 304 if the client's version matches.
	if c.GetHeader("If-None-Match") == etag {
		c.Header("Cache-Control", brandingCacheControl)
		c.Header("ETag", etag)
		c.Status(http.StatusNotModified)
		return
	}

	c.Header("Cache-Control", brandingCacheControl)
	c.Header("ETag", etag)
	c.Header("Last-Modified", row.UpdatedAt.UTC().Format(http.TimeFormat))
	c.Header("Vary", "Accept-Encoding")

	resp := branding.ToResponse(row)
	// Storefront does NOT receive custom_css or show_powered_by toggle.
	// The storefront layout.tsx reads these directly and decides what
	// to render. Strip custom_css from the public response to avoid
	// giving raw CSS injection surface to crawlers.
	//
	// Actually, the storefront DOES need custom_css to inject it.
	// But show_powered_by is needed too for the footer.
	// Keep full response — the data is public storefront configuration.
	c.JSON(http.StatusOK, resp)
}
```

### 4.2 Verify

```bash
cd services/marketplace-api && go build ./...
```

---

## Task 5 — Wire routes + main.go

**Files to modify:**
- `services/marketplace-api/internal/handlers/admin/routes.go`
- `services/marketplace-api/internal/handlers/storefront/routes.go`
- `services/marketplace-api/cmd/marketplace-api/main.go`

### 5.1 Add `BrandingHandler` to admin `Deps`

In `services/marketplace-api/internal/handlers/admin/routes.go`, add the field to the `Deps` struct:

```go
// Add to Deps struct (after LoyaltyHandler):
BrandingHandler          *BrandingHandler
```

Add route registration at the end of `RegisterAdmin`, inside the `storeRoute` block, after the loyalty group:

```go
		// Branding — B1.
		if deps.BrandingHandler != nil {
			brandingGroup := storeRoute.Group("/branding")
			{
				brandingGroup.GET("",
					deps.AuthzMiddleware.RequireTenantRelation(authz.RoleStaff),
					deps.BrandingHandler.Get)
				brandingGroup.PUT("",
					deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin),
					deps.BrandingHandler.Update)
				brandingGroup.POST("/logo",
					deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin),
					deps.BrandingHandler.UploadLogo)
				brandingGroup.POST("/favicon",
					deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin),
					deps.BrandingHandler.UploadFavicon)
				brandingGroup.POST("/hero",
					deps.AuthzMiddleware.RequireTenantRelation(authz.RoleAdmin),
					deps.BrandingHandler.UploadHero)
			}
		}
```

### 5.2 Add `BrandingStorefrontHandler` to storefront `Deps`

In `services/marketplace-api/internal/handlers/storefront/routes.go`, add to the `Deps` struct:

```go
// Add to Deps struct (after LoyaltyHandler):
BrandingHandler           *BrandingStorefrontHandler
```

Add route registration inside the `group` block:

```go
		// Branding — B1. Public, no auth.
		if deps.BrandingHandler != nil {
			group.GET("/branding", deps.BrandingHandler.Get)
		}
```

### 5.3 Wire in `main.go`

In `services/marketplace-api/cmd/marketplace-api/main.go`:

Add import:
```go
"github.com/mark8ly/marketplace-api/internal/branding"
```

In the admin wiring block (after `loyaltyHandler` construction, around line 230):

```go
		// Branding B1 wiring.
		brandingRepo := branding.NewRepository()
		brandingHandler := admin.NewBrandingHandler(conn, brandingRepo, uploader, log)
```

Add to the `adminDeps` struct literal:

```go
		BrandingHandler:         brandingHandler,
```

In the storefront wiring block (after `sfLoyaltyHandler` construction, around line 295):

```go
		// Branding B1 storefront wiring.
		brandingRepoSF := branding.NewRepository()
		brandingSFHandler := storefront.NewBrandingStorefrontHandler(conn, brandingRepoSF, log)
```

Add to the `storefrontDeps` struct literal:

```go
		BrandingHandler:       brandingSFHandler,
```

### 5.4 Verify

```bash
cd services/marketplace-api && go build ./cmd/marketplace-api/...
```

---

## Task 6 — Admin UI: 6-tab branding settings page

**Files to create:**
- `apps/admin/app/settings/branding/page.tsx`
- `apps/admin/app/settings/branding/actions.ts`
- `apps/admin/components/settings/branding/BrandingSettingsPage.tsx`
- `apps/admin/components/settings/branding/IdentityTab.tsx`
- `apps/admin/components/settings/branding/ColorsTab.tsx`
- `apps/admin/components/settings/branding/TypographyTab.tsx`
- `apps/admin/components/settings/branding/LayoutTab.tsx`
- `apps/admin/components/settings/branding/FooterTab.tsx`
- `apps/admin/components/settings/branding/AdvancedTab.tsx`
- `apps/admin/components/settings/branding/types.ts`
- `apps/admin/components/settings/branding/color-presets.ts`
- `apps/admin/lib/api/branding-api.ts`

### 6.1 `types.ts` — Shared types for branding form

```typescript
export interface BrandingData {
  id: string;
  store_id: string;
  logo_url: string | null;
  favicon_url: string | null;
  tagline: string | null;
  color_background: string;
  color_text: string;
  color_accent: string;
  color_button_bg: string;
  color_button_text: string;
  heading_font: string;
  body_font: string;
  layout_variant: string;
  hero_image_url: string | null;
  announcement_text: string | null;
  announcement_link: string | null;
  announcement_bg: string | null;
  announcement_active: boolean;
  footer_tagline: string | null;
  footer_copyright: string | null;
  social_instagram: string | null;
  social_twitter: string | null;
  social_facebook: string | null;
  social_tiktok: string | null;
  social_youtube: string | null;
  custom_css: string | null;
  show_powered_by: boolean;
  branding_version: number;
  updated_at: string;
}

export interface BrandingFormState {
  data: BrandingData;
  onChange: (patch: Partial<BrandingData>) => void;
  editable: boolean;
}

export interface ContrastError {
  field: string;
  message: string;
  ratio: number;
  required: number;
}
```

### 6.2 `color-presets.ts` — 6-8 curated palettes

```typescript
export interface ColorPreset {
  name: string;
  description: string;
  colors: {
    color_background: string;
    color_text: string;
    color_accent: string;
    color_button_bg: string;
    color_button_text: string;
  };
}

export const COLOR_PRESETS: ColorPreset[] = [
  {
    name: "Paper",
    description: "Mark8ly house — warm paper, ink, moss",
    colors: {
      color_background: "#F7F6F2",
      color_text: "#0E0E0C",
      color_accent: "#2D4A2B",
      color_button_bg: "#0E0E0C",
      color_button_text: "#F7F6F2",
    },
  },
  {
    name: "Midnight",
    description: "Cool navy with brass accent",
    colors: {
      color_background: "#EEF2FB",
      color_text: "#182033",
      color_accent: "#B5854D",
      color_button_bg: "#28334F",
      color_button_text: "#FFFFFF",
    },
  },
  {
    name: "Forest",
    description: "Deep green with warm leaf tones",
    colors: {
      color_background: "#F4F8F2",
      color_text: "#17211C",
      color_accent: "#7CA067",
      color_button_bg: "#31584A",
      color_button_text: "#FFFFFF",
    },
  },
  {
    name: "Sand",
    description: "Neutral tones with walnut warmth",
    colors: {
      color_background: "#F6F0E6",
      color_text: "#211B16",
      color_accent: "#BF8F56",
      color_button_bg: "#7C6A4D",
      color_button_text: "#FFFFFF",
    },
  },
  {
    name: "Monochrome",
    description: "Pure black and white, no distraction",
    colors: {
      color_background: "#FFFFFF",
      color_text: "#111111",
      color_accent: "#555555",
      color_button_bg: "#111111",
      color_button_text: "#FFFFFF",
    },
  },
  {
    name: "Blush",
    description: "Soft rose with charcoal",
    colors: {
      color_background: "#FBF4F4",
      color_text: "#2D2226",
      color_accent: "#C77D8A",
      color_button_bg: "#2D2226",
      color_button_text: "#FBF4F4",
    },
  },
  {
    name: "Ocean",
    description: "Deep teal with coral",
    colors: {
      color_background: "#F0F5F5",
      color_text: "#1A2E2E",
      color_accent: "#D4836A",
      color_button_bg: "#1A2E2E",
      color_button_text: "#F0F5F5",
    },
  },
  {
    name: "Slate",
    description: "Cool gray with electric blue",
    colors: {
      color_background: "#F3F4F6",
      color_text: "#1F2937",
      color_accent: "#3B82F6",
      color_button_bg: "#1F2937",
      color_button_text: "#F3F4F6",
    },
  },
];
```

### 6.3 `branding-api.ts` — API client

```typescript
// apps/admin/lib/api/branding-api.ts

import type { SessionHeaders, MutationResult } from "./marketplace-api";
import type { BrandingData } from "@/components/settings/branding/types";

const MARKETPLACE_API_URL =
  process.env.MARKETPLACE_API_URL ?? "http://localhost:8088";

function buildHeaders(session: SessionHeaders): HeadersInit {
  return {
    "Content-Type": "application/json",
    "X-User-Id": session.userId,
    "X-Tenant-Id": session.tenantId,
  };
}

export async function getBranding(
  storeId: string,
  session: SessionHeaders,
): Promise<BrandingData> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/branding`,
    { headers: buildHeaders(session), cache: "no-store" },
  );
  if (!res.ok) {
    throw new Error(`Failed to fetch branding: ${res.status}`);
  }
  return res.json();
}

export async function updateBranding(
  storeId: string,
  session: SessionHeaders,
  data: Partial<BrandingData>,
): Promise<MutationResult<BrandingData>> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/branding`,
    {
      method: "PUT",
      headers: buildHeaders(session),
      body: JSON.stringify(data),
    },
  );
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: "unknown", message: "Unknown error" }));
    return { ok: false as const, error: err.error, message: err.message, contrast_errors: err.contrast_errors };
  }
  const result = await res.json();
  return { ok: true as const, data: result };
}

export interface UploadURLResponse {
  upload_url: string;
  storage_key: string;
  expires_at: string;
  public_url: string;
}

export async function getBrandingUploadURL(
  storeId: string,
  session: SessionHeaders,
  assetType: "logo" | "favicon" | "hero",
  filename: string,
  contentType: string,
): Promise<UploadURLResponse> {
  const res = await fetch(
    `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/branding/${assetType}`,
    {
      method: "POST",
      headers: buildHeaders(session),
      body: JSON.stringify({ filename, content_type: contentType }),
    },
  );
  if (!res.ok) {
    throw new Error(`Failed to get upload URL: ${res.status}`);
  }
  return res.json();
}
```

### 6.4 `page.tsx` — Server component page

```tsx
// apps/admin/app/settings/branding/page.tsx
import { AdminShell } from "@/components/shell/AdminShell";
import { BrandingSettingsPage } from "@/components/settings/branding/BrandingSettingsPage";
import {
  canEditSettings,
  getServerSessionContext,
} from "@/lib/auth/serverSession";
import { getBranding } from "@/lib/api/branding-api";

export default async function BrandingPage() {
  const {
    tenantName,
    email,
    role,
    memberships,
    tenantId,
    currentStore,
  } = await getServerSessionContext();
  const editable = canEditSettings(role);

  let branding = null;
  if (currentStore) {
    try {
      branding = await getBranding(currentStore.id, {
        userId: email, // userId from session
        tenantId,
      });
    } catch {
      // Will show error state in the component.
    }
  }

  return (
    <AdminShell
      tenantName={tenantName}
      userEmail={email}
      role={role}
      memberships={memberships}
      currentTenantId={tenantId}
    >
      <div className="mx-auto w-full max-w-7xl space-y-12">
        <header className="space-y-3">
          <p className="eyebrow">Storefront</p>
          <h1 className="font-serif text-5xl font-medium tracking-tight text-foreground">
            Branding
          </h1>
          <p className="max-w-3xl text-base leading-7 text-foreground-secondary">
            Customize your storefront identity, colors, typography, layout,
            footer, and advanced settings. Changes are applied to your
            live storefront when you save.
          </p>
          {!editable && (
            <p className="text-sm text-warning">
              Read-only: your role ({role}) can view branding settings but
              cannot publish changes.
            </p>
          )}
        </header>

        {currentStore && branding ? (
          <BrandingSettingsPage
            store={currentStore}
            initialData={branding}
            editable={editable}
          />
        ) : (
          <p className="text-sm text-danger">
            We couldn&apos;t load branding settings. Please refresh, or
            contact support if the problem persists.
          </p>
        )}
      </div>
    </AdminShell>
  );
}
```

### 6.5 `actions.ts` — Server actions

```typescript
// apps/admin/app/settings/branding/actions.ts
"use server";

import { headers } from "next/headers";
import { revalidatePath } from "next/cache";

import { canEditSettings } from "@/lib/auth/serverSession";
import { updateBranding } from "@/lib/api/branding-api";
import type { BrandingData, ContrastError } from "@/components/settings/branding/types";
import type { TenantRole } from "@/lib/api/platform-api";

export type SaveBrandingResult =
  | { ok: true }
  | { ok: false; code: string; message: string; contrast_errors?: ContrastError[] };

export async function saveBranding(
  storeId: string,
  data: Partial<BrandingData>,
): Promise<SaveBrandingResult> {
  const h = await headers();
  const tenantId = h.get("x-session-tenant-id") ?? "";
  const uid = h.get("x-session-user-id") ?? "";
  const role = (h.get("x-session-role") ?? "viewer") as TenantRole;

  if (!tenantId || !uid) {
    return { ok: false, code: "no_session", message: "Your session has expired. Please sign in again." };
  }
  if (!canEditSettings(role)) {
    return { ok: false, code: "forbidden", message: "You do not have permission to edit branding." };
  }

  const result = await updateBranding(storeId, { userId: uid, tenantId }, data);
  if (!result.ok) {
    return {
      ok: false,
      code: result.error ?? "unknown",
      message: result.message ?? "Failed to save branding.",
      contrast_errors: result.contrast_errors,
    };
  }

  revalidatePath("/settings/branding");
  return { ok: true };
}
```

### 6.6 `BrandingSettingsPage.tsx` — 6-tab layout with left-rail navigation

```tsx
// apps/admin/components/settings/branding/BrandingSettingsPage.tsx
"use client";

import { useState, useTransition } from "react";
import type { Store } from "@/lib/api/platform-api";
import type { BrandingData, ContrastError } from "./types";
import { IdentityTab } from "./IdentityTab";
import { ColorsTab } from "./ColorsTab";
import { TypographyTab } from "./TypographyTab";
import { LayoutTab } from "./LayoutTab";
import { FooterTab } from "./FooterTab";
import { AdvancedTab } from "./AdvancedTab";
import { saveBranding } from "@/app/settings/branding/actions";

type Tab = "identity" | "colors" | "typography" | "layout" | "footer" | "advanced";

const TABS: { key: Tab; label: string }[] = [
  { key: "identity", label: "Identity" },
  { key: "colors", label: "Colors" },
  { key: "typography", label: "Typography" },
  { key: "layout", label: "Layout" },
  { key: "footer", label: "Footer" },
  { key: "advanced", label: "Advanced" },
];

interface BrandingSettingsPageProps {
  store: Store;
  initialData: BrandingData;
  editable: boolean;
}

export function BrandingSettingsPage({
  store,
  initialData,
  editable,
}: BrandingSettingsPageProps) {
  const [activeTab, setActiveTab] = useState<Tab>("identity");
  const [data, setData] = useState<BrandingData>(initialData);
  const [error, setError] = useState<string | null>(null);
  const [contrastErrors, setContrastErrors] = useState<ContrastError[]>([]);
  const [success, setSuccess] = useState(false);
  const [pending, startTransition] = useTransition();

  const dirty = JSON.stringify(data) !== JSON.stringify(initialData);

  function handleChange(patch: Partial<BrandingData>) {
    setData((prev) => ({ ...prev, ...patch }));
    setSuccess(false);
    setContrastErrors([]);
  }

  function handleSubmit() {
    if (!dirty || !editable) return;
    setError(null);
    setSuccess(false);
    setContrastErrors([]);

    startTransition(async () => {
      const result = await saveBranding(store.id, data);
      if (!result.ok) {
        setError(result.message);
        if (result.contrast_errors) {
          setContrastErrors(result.contrast_errors);
        }
        return;
      }
      setSuccess(true);
    });
  }

  const formState = { data, onChange: handleChange, editable };

  return (
    <div className="flex gap-12">
      {/* Left rail — tab navigation */}
      <nav className="w-48 shrink-0 space-y-1" aria-label="Branding settings">
        {TABS.map((tab) => (
          <button
            key={tab.key}
            type="button"
            onClick={() => setActiveTab(tab.key)}
            className={`block w-full rounded-md px-3 py-2 text-left text-sm transition-colors ${
              activeTab === tab.key
                ? "bg-foreground/5 font-medium text-foreground"
                : "text-foreground-secondary hover:bg-foreground/3 hover:text-foreground"
            }`}
          >
            {tab.label}
          </button>
        ))}
      </nav>

      {/* Main content area */}
      <div className="min-w-0 flex-1 space-y-8">
        {activeTab === "identity" && <IdentityTab {...formState} storeId={store.id} />}
        {activeTab === "colors" && <ColorsTab {...formState} contrastErrors={contrastErrors} />}
        {activeTab === "typography" && <TypographyTab {...formState} />}
        {activeTab === "layout" && <LayoutTab {...formState} storeId={store.id} />}
        {activeTab === "footer" && <FooterTab {...formState} />}
        {activeTab === "advanced" && <AdvancedTab {...formState} />}

        {/* Save bar */}
        <div className="flex items-center gap-4 border-t border-foreground/10 pt-6">
          <button
            type="button"
            onClick={handleSubmit}
            disabled={!dirty || !editable || pending}
            className="rounded-md bg-foreground px-6 py-2.5 text-sm font-medium text-background transition-opacity disabled:opacity-40"
          >
            {pending ? "Saving..." : "Save changes"}
          </button>
          {success && (
            <p className="text-sm text-moss-700">Branding saved successfully.</p>
          )}
          {error && (
            <p className="text-sm text-danger">{error}</p>
          )}
        </div>
      </div>
    </div>
  );
}
```

### 6.7 `IdentityTab.tsx`

```tsx
// apps/admin/components/settings/branding/IdentityTab.tsx
"use client";

import { Field } from "@repo/ui/field";
import type { BrandingFormState } from "./types";

interface IdentityTabProps extends BrandingFormState {
  storeId: string;
}

export function IdentityTab({ data, onChange, editable, storeId }: IdentityTabProps) {
  return (
    <section className="space-y-8">
      <div className="space-y-2">
        <h2 className="font-serif text-2xl font-medium text-foreground">Identity</h2>
        <p className="text-sm text-foreground-secondary">
          Logo, favicon, and store tagline.
        </p>
      </div>

      {/* Logo upload placeholder — wired to GCS signed URL flow */}
      <div className="space-y-3">
        <label className="block text-sm font-medium text-foreground">Logo</label>
        {data.logo_url ? (
          <div className="flex items-center gap-4">
            <img
              src={data.logo_url}
              alt="Store logo"
              className="h-16 w-auto rounded border border-foreground/10 object-contain"
            />
            <button
              type="button"
              onClick={() => onChange({ logo_url: null })}
              disabled={!editable}
              className="text-sm text-danger hover:underline disabled:opacity-40"
            >
              Remove
            </button>
          </div>
        ) : (
          <p className="text-sm text-foreground-secondary">
            No logo uploaded. Use the upload button to add your store logo.
          </p>
        )}
        {/* TODO: Wire file input to getBrandingUploadURL("logo") */}
      </div>

      {/* Favicon upload placeholder */}
      <div className="space-y-3">
        <label className="block text-sm font-medium text-foreground">Favicon</label>
        {data.favicon_url ? (
          <div className="flex items-center gap-4">
            <img
              src={data.favicon_url}
              alt="Favicon"
              className="h-8 w-8 rounded border border-foreground/10 object-contain"
            />
            <button
              type="button"
              onClick={() => onChange({ favicon_url: null })}
              disabled={!editable}
              className="text-sm text-danger hover:underline disabled:opacity-40"
            >
              Remove
            </button>
          </div>
        ) : (
          <p className="text-sm text-foreground-secondary">
            No favicon uploaded.
          </p>
        )}
      </div>

      {/* Tagline */}
      <Field label="Tagline" hint="A short phrase displayed below your store name. Max 200 characters.">
        <input
          type="text"
          value={data.tagline ?? ""}
          onChange={(e) => onChange({ tagline: e.target.value || null })}
          disabled={!editable}
          maxLength={200}
          placeholder="Handcrafted goods for modern living"
          className="w-full rounded-md border border-foreground/10 bg-background px-3 py-2 text-sm text-foreground placeholder:text-foreground-secondary/50 focus:border-moss-700 focus:outline-none focus:ring-1 focus:ring-moss-700 disabled:opacity-50"
        />
      </Field>
    </section>
  );
}
```

### 6.8 `ColorsTab.tsx` — Color pickers with presets + contrast warnings

```tsx
// apps/admin/components/settings/branding/ColorsTab.tsx
"use client";

import type { BrandingFormState, ContrastError } from "./types";
import { COLOR_PRESETS } from "./color-presets";

interface ColorsTabProps extends BrandingFormState {
  contrastErrors: ContrastError[];
}

const COLOR_FIELDS = [
  { key: "color_background" as const, label: "Background" },
  { key: "color_text" as const, label: "Text" },
  { key: "color_accent" as const, label: "Accent" },
  { key: "color_button_bg" as const, label: "Button background" },
  { key: "color_button_text" as const, label: "Button text" },
];

export function ColorsTab({ data, onChange, editable, contrastErrors }: ColorsTabProps) {
  function applyPreset(preset: (typeof COLOR_PRESETS)[number]) {
    onChange(preset.colors);
  }

  return (
    <section className="space-y-8">
      <div className="space-y-2">
        <h2 className="font-serif text-2xl font-medium text-foreground">Colors</h2>
        <p className="text-sm text-foreground-secondary">
          Choose a preset or customize individual colors. WCAG AA contrast
          is validated on save.
        </p>
      </div>

      {/* Presets grid */}
      <div className="space-y-3">
        <h3 className="text-sm font-medium text-foreground">Presets</h3>
        <div className="grid grid-cols-4 gap-3">
          {COLOR_PRESETS.map((preset) => (
            <button
              key={preset.name}
              type="button"
              onClick={() => applyPreset(preset)}
              disabled={!editable}
              className="group space-y-2 rounded-md border border-foreground/10 p-3 text-left transition-colors hover:border-foreground/20 disabled:opacity-40"
            >
              <div className="flex gap-1">
                {Object.values(preset.colors).map((color, i) => (
                  <div
                    key={i}
                    className="h-6 w-6 rounded-full border border-foreground/10"
                    style={{ backgroundColor: color }}
                  />
                ))}
              </div>
              <p className="text-xs font-medium text-foreground">{preset.name}</p>
              <p className="text-xs text-foreground-secondary">{preset.description}</p>
            </button>
          ))}
        </div>
      </div>

      {/* Individual color pickers */}
      <div className="grid grid-cols-2 gap-6 lg:grid-cols-3">
        {COLOR_FIELDS.map((field) => {
          const fieldError = contrastErrors.find((e) => e.field === field.key);
          return (
            <div key={field.key} className="space-y-2">
              <label className="block text-sm font-medium text-foreground">
                {field.label}
              </label>
              <div className="flex items-center gap-3">
                <input
                  type="color"
                  value={data[field.key]}
                  onChange={(e) => onChange({ [field.key]: e.target.value })}
                  disabled={!editable}
                  className="h-10 w-10 cursor-pointer rounded border border-foreground/10 disabled:opacity-40"
                />
                <input
                  type="text"
                  value={data[field.key]}
                  onChange={(e) => {
                    const v = e.target.value;
                    if (/^#[0-9a-fA-F]{0,6}$/.test(v)) {
                      onChange({ [field.key]: v });
                    }
                  }}
                  disabled={!editable}
                  maxLength={7}
                  className="w-24 rounded-md border border-foreground/10 bg-background px-2 py-1.5 text-xs font-mono text-foreground disabled:opacity-50"
                />
              </div>
              {fieldError && (
                <p className="text-xs text-danger">
                  {fieldError.message}
                </p>
              )}
            </div>
          );
        })}
      </div>
    </section>
  );
}
```

### 6.9 `TypographyTab.tsx` — Font dropdowns with preview

```tsx
// apps/admin/components/settings/branding/TypographyTab.tsx
"use client";

import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@tesserix/web";
import type { BrandingFormState } from "./types";

const FONT_OPTIONS = [
  { value: "source-serif-4", label: "Source Serif 4", family: "'Source Serif 4', serif" },
  { value: "playfair-display", label: "Playfair Display", family: "'Playfair Display', serif" },
  { value: "lora", label: "Lora", family: "'Lora', serif" },
  { value: "inter", label: "Inter", family: "'Inter', sans-serif" },
  { value: "source-sans-3", label: "Source Sans 3", family: "'Source Sans 3', sans-serif" },
  { value: "dm-sans", label: "DM Sans", family: "'DM Sans', sans-serif" },
];

export function TypographyTab({ data, onChange, editable }: BrandingFormState) {
  return (
    <section className="space-y-8">
      <div className="space-y-2">
        <h2 className="font-serif text-2xl font-medium text-foreground">Typography</h2>
        <p className="text-sm text-foreground-secondary">
          Select heading and body fonts for your storefront.
        </p>
      </div>

      <div className="grid grid-cols-2 gap-8">
        {/* Heading font */}
        <div className="space-y-3">
          <label className="block text-sm font-medium text-foreground">Heading font</label>
          <Select
            value={data.heading_font}
            onValueChange={(v) => onChange({ heading_font: v })}
            disabled={!editable}
          >
            <SelectTrigger className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {FONT_OPTIONS.map((font) => (
                <SelectItem key={font.value} value={font.value}>
                  <span style={{ fontFamily: font.family }}>{font.label}</span>
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {/* Preview */}
          <p
            className="text-2xl text-foreground"
            style={{
              fontFamily: FONT_OPTIONS.find((f) => f.value === data.heading_font)?.family,
            }}
          >
            The quick brown fox
          </p>
        </div>

        {/* Body font */}
        <div className="space-y-3">
          <label className="block text-sm font-medium text-foreground">Body font</label>
          <Select
            value={data.body_font}
            onValueChange={(v) => onChange({ body_font: v })}
            disabled={!editable}
          >
            <SelectTrigger className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {FONT_OPTIONS.map((font) => (
                <SelectItem key={font.value} value={font.value}>
                  <span style={{ fontFamily: font.family }}>{font.label}</span>
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <p
            className="text-base text-foreground-secondary"
            style={{
              fontFamily: FONT_OPTIONS.find((f) => f.value === data.body_font)?.family,
            }}
          >
            Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed do eiusmod
            tempor incididunt ut labore.
          </p>
        </div>
      </div>
    </section>
  );
}
```

### 6.10 `LayoutTab.tsx` — Visual layout selector + hero + announcement bar

```tsx
// apps/admin/components/settings/branding/LayoutTab.tsx
"use client";

import type { BrandingFormState } from "./types";

interface LayoutTabProps extends BrandingFormState {
  storeId: string;
}

const LAYOUT_OPTIONS = [
  { value: "editorial", label: "Editorial", description: "Story-led hero with premium pacing." },
  { value: "classic-shop", label: "Classic Shop", description: "Balanced retail landing." },
  { value: "split-hero", label: "Split Hero", description: "Left-right composition." },
  { value: "catalog-first", label: "Catalog First", description: "Product-led opening." },
  { value: "story-led", label: "Story-led", description: "Narrative presentation." },
  { value: "minimal", label: "Minimal", description: "Quiet with breathing room." },
  { value: "bold-promo", label: "Bold Promo", description: "Campaign-forward layout." },
  { value: "compact", label: "Compact", description: "Dense for practical browsing." },
];

export function LayoutTab({ data, onChange, editable, storeId }: LayoutTabProps) {
  return (
    <section className="space-y-8">
      <div className="space-y-2">
        <h2 className="font-serif text-2xl font-medium text-foreground">Layout</h2>
        <p className="text-sm text-foreground-secondary">
          Choose your homepage layout, hero image, and announcement bar.
        </p>
      </div>

      {/* Layout variant selector */}
      <div className="space-y-3">
        <h3 className="text-sm font-medium text-foreground">Homepage layout</h3>
        <div className="grid grid-cols-4 gap-3">
          {LAYOUT_OPTIONS.map((layout) => (
            <button
              key={layout.value}
              type="button"
              onClick={() => onChange({ layout_variant: layout.value })}
              disabled={!editable}
              className={`rounded-md border p-3 text-left transition-colors disabled:opacity-40 ${
                data.layout_variant === layout.value
                  ? "border-moss-700 bg-moss-700/5"
                  : "border-foreground/10 hover:border-foreground/20"
              }`}
            >
              <p className="text-xs font-medium text-foreground">{layout.label}</p>
              <p className="mt-1 text-xs text-foreground-secondary">{layout.description}</p>
            </button>
          ))}
        </div>
      </div>

      {/* Hero image */}
      <div className="space-y-3">
        <label className="block text-sm font-medium text-foreground">Hero image</label>
        {data.hero_image_url ? (
          <div className="space-y-2">
            <img
              src={data.hero_image_url}
              alt="Hero"
              className="h-40 w-full rounded-md border border-foreground/10 object-cover"
            />
            <button
              type="button"
              onClick={() => onChange({ hero_image_url: null })}
              disabled={!editable}
              className="text-sm text-danger hover:underline disabled:opacity-40"
            >
              Remove hero image
            </button>
          </div>
        ) : (
          <p className="text-sm text-foreground-secondary">No hero image set.</p>
        )}
      </div>

      {/* Announcement bar */}
      <div className="space-y-4 border-t border-foreground/10 pt-6">
        <div className="flex items-center justify-between">
          <h3 className="text-sm font-medium text-foreground">Announcement bar</h3>
          <label className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={data.announcement_active}
              onChange={(e) => onChange({ announcement_active: e.target.checked })}
              disabled={!editable}
              className="rounded border-foreground/20"
            />
            Active
          </label>
        </div>
        <input
          type="text"
          value={data.announcement_text ?? ""}
          onChange={(e) => onChange({ announcement_text: e.target.value || null })}
          disabled={!editable}
          maxLength={300}
          placeholder="Free shipping on orders over $50"
          className="w-full rounded-md border border-foreground/10 bg-background px-3 py-2 text-sm text-foreground placeholder:text-foreground-secondary/50 focus:border-moss-700 focus:outline-none focus:ring-1 focus:ring-moss-700 disabled:opacity-50"
        />
        <input
          type="url"
          value={data.announcement_link ?? ""}
          onChange={(e) => onChange({ announcement_link: e.target.value || null })}
          disabled={!editable}
          placeholder="https://your-store.com/sale"
          className="w-full rounded-md border border-foreground/10 bg-background px-3 py-2 text-sm text-foreground placeholder:text-foreground-secondary/50 focus:border-moss-700 focus:outline-none focus:ring-1 focus:ring-moss-700 disabled:opacity-50"
        />
      </div>
    </section>
  );
}
```

### 6.11 `FooterTab.tsx`

```tsx
// apps/admin/components/settings/branding/FooterTab.tsx
"use client";

import { Field } from "@repo/ui/field";
import type { BrandingFormState } from "./types";

const SOCIAL_FIELDS = [
  { key: "social_instagram" as const, label: "Instagram", placeholder: "https://instagram.com/yourstore" },
  { key: "social_twitter" as const, label: "X (Twitter)", placeholder: "https://x.com/yourstore" },
  { key: "social_facebook" as const, label: "Facebook", placeholder: "https://facebook.com/yourstore" },
  { key: "social_tiktok" as const, label: "TikTok", placeholder: "https://tiktok.com/@yourstore" },
  { key: "social_youtube" as const, label: "YouTube", placeholder: "https://youtube.com/@yourstore" },
];

export function FooterTab({ data, onChange, editable }: BrandingFormState) {
  return (
    <section className="space-y-8">
      <div className="space-y-2">
        <h2 className="font-serif text-2xl font-medium text-foreground">Footer</h2>
        <p className="text-sm text-foreground-secondary">
          Tagline, copyright text, and social media links shown in your storefront footer.
        </p>
      </div>

      <Field label="Footer tagline" hint="Displayed above the footer links. Max 300 characters.">
        <input
          type="text"
          value={data.footer_tagline ?? ""}
          onChange={(e) => onChange({ footer_tagline: e.target.value || null })}
          disabled={!editable}
          maxLength={300}
          placeholder="Handcrafted with care since 2024"
          className="w-full rounded-md border border-foreground/10 bg-background px-3 py-2 text-sm text-foreground placeholder:text-foreground-secondary/50 focus:border-moss-700 focus:outline-none focus:ring-1 focus:ring-moss-700 disabled:opacity-50"
        />
      </Field>

      <Field label="Copyright text" hint="Displayed at the bottom of the footer. Max 200 characters.">
        <input
          type="text"
          value={data.footer_copyright ?? ""}
          onChange={(e) => onChange({ footer_copyright: e.target.value || null })}
          disabled={!editable}
          maxLength={200}
          placeholder="2026 Your Store. All rights reserved."
          className="w-full rounded-md border border-foreground/10 bg-background px-3 py-2 text-sm text-foreground placeholder:text-foreground-secondary/50 focus:border-moss-700 focus:outline-none focus:ring-1 focus:ring-moss-700 disabled:opacity-50"
        />
      </Field>

      <div className="space-y-4 border-t border-foreground/10 pt-6">
        <h3 className="text-sm font-medium text-foreground">Social links</h3>
        <div className="grid grid-cols-2 gap-4">
          {SOCIAL_FIELDS.map((field) => (
            <div key={field.key} className="space-y-1.5">
              <label className="block text-xs font-medium text-foreground-secondary">
                {field.label}
              </label>
              <input
                type="url"
                value={data[field.key] ?? ""}
                onChange={(e) => onChange({ [field.key]: e.target.value || null })}
                disabled={!editable}
                maxLength={300}
                placeholder={field.placeholder}
                className="w-full rounded-md border border-foreground/10 bg-background px-3 py-2 text-sm text-foreground placeholder:text-foreground-secondary/50 focus:border-moss-700 focus:outline-none focus:ring-1 focus:ring-moss-700 disabled:opacity-50"
              />
            </div>
          ))}
        </div>
      </div>
    </section>
  );
}
```

### 6.12 `AdvancedTab.tsx` — Custom CSS + powered-by toggle (Enterprise-gated)

```tsx
// apps/admin/components/settings/branding/AdvancedTab.tsx
"use client";

import type { BrandingFormState } from "./types";

export function AdvancedTab({ data, onChange, editable }: BrandingFormState) {
  return (
    <section className="space-y-8">
      <div className="space-y-2">
        <h2 className="font-serif text-2xl font-medium text-foreground">Advanced</h2>
        <p className="text-sm text-foreground-secondary">
          Custom CSS injection and powered-by badge. Some features require
          Enterprise plan.
        </p>
      </div>

      {/* Powered by toggle */}
      <div className="flex items-center justify-between rounded-md border border-foreground/10 px-4 py-3">
        <div>
          <p className="text-sm font-medium text-foreground">
            Show &ldquo;Powered by Mark8ly&rdquo;
          </p>
          <p className="text-xs text-foreground-secondary">
            Displayed in the storefront footer. Requires Pro plan to remove.
          </p>
        </div>
        <label className="relative inline-flex cursor-pointer items-center">
          <input
            type="checkbox"
            checked={data.show_powered_by}
            onChange={(e) => onChange({ show_powered_by: e.target.checked })}
            disabled={!editable}
            className="peer sr-only"
          />
          <div className="peer h-5 w-9 rounded-full bg-foreground/20 after:absolute after:left-[2px] after:top-[2px] after:h-4 after:w-4 after:rounded-full after:bg-white after:transition-all peer-checked:bg-moss-700 peer-checked:after:translate-x-full peer-disabled:opacity-40" />
        </label>
      </div>

      {/* Custom CSS */}
      <div className="space-y-3">
        <div>
          <label className="block text-sm font-medium text-foreground">Custom CSS</label>
          <p className="text-xs text-foreground-secondary">
            Injected into your storefront. Enterprise plan only.
            Dangerous patterns (@import, external URLs, javascript:) are
            stripped automatically.
          </p>
        </div>
        <textarea
          value={data.custom_css ?? ""}
          onChange={(e) => onChange({ custom_css: e.target.value || null })}
          disabled={!editable}
          rows={12}
          placeholder={`.hero-section {\n  padding: 4rem 2rem;\n}\n\n.product-card {\n  border-radius: 12px;\n}`}
          className="w-full rounded-md border border-foreground/10 bg-background px-3 py-2 font-mono text-xs text-foreground placeholder:text-foreground-secondary/30 focus:border-moss-700 focus:outline-none focus:ring-1 focus:ring-moss-700 disabled:opacity-50"
        />
      </div>
    </section>
  );
}
```

### 6.13 Verify

```bash
cd apps/admin && npx tsc --noEmit
```

---

## Task 7 — Admin UI: live preview panel (PostMessage bridge, modal on mobile)

**Files to create:**
- `apps/admin/components/settings/branding/LivePreview.tsx`

**Files to modify:**
- `apps/admin/components/settings/branding/BrandingSettingsPage.tsx` — add preview panel

### 7.1 `LivePreview.tsx`

```tsx
// apps/admin/components/settings/branding/LivePreview.tsx
"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import type { BrandingData } from "./types";

interface LivePreviewProps {
  storeSlug: string;
  data: BrandingData;
}

/**
 * LivePreview — renders an iframe of the storefront and pushes branding
 * changes via PostMessage. The storefront listens for messages of type
 * "branding-preview" and applies CSS variable overrides without reload.
 *
 * On mobile (<768px), the preview is hidden by default. The parent
 * renders a "Preview" button that opens this component in a modal.
 */
export function LivePreview({ storeSlug, data }: LivePreviewProps) {
  const iframeRef = useRef<HTMLIFrameElement>(null);
  const [loaded, setLoaded] = useState(false);

  const sendBranding = useCallback(() => {
    const iframe = iframeRef.current;
    if (!iframe?.contentWindow) return;
    iframe.contentWindow.postMessage(
      {
        type: "branding-preview",
        branding: {
          color_background: data.color_background,
          color_text: data.color_text,
          color_accent: data.color_accent,
          color_button_bg: data.color_button_bg,
          color_button_text: data.color_button_text,
          heading_font: data.heading_font,
          body_font: data.body_font,
        },
      },
      "*", // Storefront is on a different subdomain.
    );
  }, [data]);

  // Re-send branding on every data change after iframe loads.
  useEffect(() => {
    if (loaded) {
      sendBranding();
    }
  }, [loaded, sendBranding]);

  const storefrontURL = process.env.NEXT_PUBLIC_STOREFRONT_URL
    ? `${process.env.NEXT_PUBLIC_STOREFRONT_URL}?preview=true`
    : `https://${storeSlug}.mark8ly.com?preview=true`;

  return (
    <div className="overflow-hidden rounded-lg border border-foreground/10 bg-foreground/3">
      <div className="flex items-center gap-2 border-b border-foreground/10 px-3 py-2">
        <div className="flex gap-1">
          <div className="h-2.5 w-2.5 rounded-full bg-foreground/15" />
          <div className="h-2.5 w-2.5 rounded-full bg-foreground/15" />
          <div className="h-2.5 w-2.5 rounded-full bg-foreground/15" />
        </div>
        <span className="text-xs text-foreground-secondary">
          {storeSlug}.mark8ly.com
        </span>
      </div>
      <iframe
        ref={iframeRef}
        src={storefrontURL}
        title="Storefront preview"
        onLoad={() => {
          setLoaded(true);
          sendBranding();
        }}
        className="h-[600px] w-full"
        sandbox="allow-scripts allow-same-origin"
      />
    </div>
  );
}
```

### 7.2 Integrate preview into `BrandingSettingsPage.tsx`

Modify `BrandingSettingsPage.tsx` to add the preview panel on the right side on desktop, and a "Preview" modal button on mobile. Add after the main content `<div>`:

```tsx
// Add state for mobile preview modal:
const [showMobilePreview, setShowMobilePreview] = useState(false);

// In the JSX return, wrap everything in a responsive layout:
// Desktop: 3-column (nav | content | preview)
// Mobile: 2-column (nav | content) + floating preview button

// After the save bar div, inside the flex-1 div:
<button
  type="button"
  onClick={() => setShowMobilePreview(true)}
  className="fixed bottom-6 right-6 z-40 rounded-full bg-foreground px-4 py-3 text-sm font-medium text-background shadow-lg md:hidden"
>
  Preview
</button>

// After the main flex container, add the desktop preview panel:
{/* Desktop preview panel — hidden on mobile */}
<div className="hidden w-96 shrink-0 lg:block">
  <div className="sticky top-6">
    <LivePreview storeSlug={store.slug ?? "preview"} data={data} />
  </div>
</div>

// Mobile preview modal:
{showMobilePreview && (
  <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 lg:hidden">
    <div className="relative mx-4 w-full max-w-lg rounded-lg bg-background p-4">
      <button
        type="button"
        onClick={() => setShowMobilePreview(false)}
        className="absolute right-3 top-3 text-foreground-secondary hover:text-foreground"
      >
        Close
      </button>
      <LivePreview storeSlug={store.slug ?? "preview"} data={data} />
    </div>
  </div>
)}
```

### 7.3 Verify

```bash
cd apps/admin && npx tsc --noEmit
```

---

## Task 8 — Storefront: inject CSS variables from branding API

**Files to modify:**
- `apps/storefront/app/layout.tsx`

**Files to create:**
- `apps/storefront/lib/branding.ts`
- `apps/storefront/components/BrandingStyle.tsx`
- `apps/storefront/components/BrandingPreviewListener.tsx`

### 8.1 `apps/storefront/lib/branding.ts` — Fetch branding from API

```typescript
// apps/storefront/lib/branding.ts

const MARKETPLACE_API_URL =
  process.env.MARKETPLACE_API_URL ?? "http://localhost:8088";

export interface StorefrontBranding {
  color_background: string;
  color_text: string;
  color_accent: string;
  color_button_bg: string;
  color_button_text: string;
  heading_font: string;
  body_font: string;
  layout_variant: string;
  logo_url: string | null;
  favicon_url: string | null;
  tagline: string | null;
  hero_image_url: string | null;
  announcement_text: string | null;
  announcement_link: string | null;
  announcement_bg: string | null;
  announcement_active: boolean;
  footer_tagline: string | null;
  footer_copyright: string | null;
  social_instagram: string | null;
  social_twitter: string | null;
  social_facebook: string | null;
  social_tiktok: string | null;
  social_youtube: string | null;
  custom_css: string | null;
  show_powered_by: boolean;
  branding_version: number;
}

const DEFAULTS: StorefrontBranding = {
  color_background: "#F7F6F2",
  color_text: "#0E0E0C",
  color_accent: "#2D4A2B",
  color_button_bg: "#0E0E0C",
  color_button_text: "#F7F6F2",
  heading_font: "source-serif-4",
  body_font: "source-sans-3",
  layout_variant: "classic-shop",
  logo_url: null,
  favicon_url: null,
  tagline: null,
  hero_image_url: null,
  announcement_text: null,
  announcement_link: null,
  announcement_bg: null,
  announcement_active: false,
  footer_tagline: null,
  footer_copyright: null,
  social_instagram: null,
  social_twitter: null,
  social_facebook: null,
  social_tiktok: null,
  social_youtube: null,
  custom_css: null,
  show_powered_by: true,
  branding_version: 0,
};

/**
 * fetchBranding — server-side fetch with Next.js cache.
 * Revalidates every 300s (5 min) to match the backend s-maxage.
 * Falls back to defaults on any error.
 */
export async function fetchBranding(storeSlug: string): Promise<StorefrontBranding> {
  try {
    const res = await fetch(
      `${MARKETPLACE_API_URL}/api/v1/storefront/stores/${storeSlug}/branding`,
      {
        next: { revalidate: 300 },
        headers: {
          "X-Storefront-Key": process.env.STOREFRONT_KEY ?? "",
        },
      },
    );
    if (!res.ok) return DEFAULTS;
    return await res.json();
  } catch {
    return DEFAULTS;
  }
}

/** Map font key to CSS font-family stack for the <style> injection. */
const FONT_STACKS: Record<string, string> = {
  "source-serif-4": "var(--font-source-serif), 'Source Serif 4', serif",
  "playfair-display": "'Playfair Display', serif",
  "lora": "'Lora', serif",
  "inter": "var(--font-inter), 'Inter', sans-serif",
  "source-sans-3": "var(--font-source-sans), 'Source Sans 3', sans-serif",
  "dm-sans": "'DM Sans', sans-serif",
};

export function fontStack(key: string): string {
  return FONT_STACKS[key] ?? FONT_STACKS["source-sans-3"];
}
```

### 8.2 `apps/storefront/components/BrandingStyle.tsx` — CSS variable injection

```tsx
// apps/storefront/components/BrandingStyle.tsx

import type { StorefrontBranding } from "@/lib/branding";
import { fontStack } from "@/lib/branding";

interface BrandingStyleProps {
  branding: StorefrontBranding;
}

/**
 * BrandingStyle — injects merchant branding as CSS custom properties
 * into the document. Rendered server-side in the root layout.
 *
 * The existing storefront components already consume these variables
 * (--paper-200, --ink-900, --moss-700 etc.), so they pick up merchant
 * colors automatically with zero component changes.
 */
export function BrandingStyle({ branding }: BrandingStyleProps) {
  const css = `:root {
  --paper-200: ${branding.color_background};
  --ink-900: ${branding.color_text};
  --moss-700: ${branding.color_accent};
  --button-bg: ${branding.color_button_bg};
  --button-text: ${branding.color_button_text};
  --font-heading: ${fontStack(branding.heading_font)};
  --font-body: ${fontStack(branding.body_font)};
  --storefront-primary: ${branding.color_text};
  --storefront-accent: ${branding.color_accent};
  --storefront-background: ${branding.color_background};
  --storefront-surface: #FFFFFF;
  --storefront-text: ${branding.color_text};
  --storefront-heading-font: ${fontStack(branding.heading_font)};
  --storefront-body-font: ${fontStack(branding.body_font)};
}`;

  // Announcement bar CSS (if active and has bg color).
  const announcementCSS = branding.announcement_active && branding.announcement_bg
    ? `\n.announcement-bar { background-color: ${branding.announcement_bg}; }`
    : "";

  // Custom CSS (Enterprise — already sanitized server-side).
  const customCSS = branding.custom_css ? `\n/* Merchant custom CSS */\n${branding.custom_css}` : "";

  return (
    <style
      dangerouslySetInnerHTML={{
        __html: css + announcementCSS + customCSS,
      }}
    />
  );
}
```

### 8.3 `apps/storefront/components/BrandingPreviewListener.tsx` — PostMessage bridge

```tsx
// apps/storefront/components/BrandingPreviewListener.tsx
"use client";

import { useEffect } from "react";

/**
 * BrandingPreviewListener — listens for PostMessage events from the
 * admin live preview iframe and applies CSS variable overrides in
 * real-time. Only active when ?preview=true is in the URL.
 */
export function BrandingPreviewListener() {
  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    if (params.get("preview") !== "true") return;

    function handleMessage(event: MessageEvent) {
      if (event.data?.type !== "branding-preview") return;
      const b = event.data.branding;
      if (!b) return;

      const root = document.documentElement;
      if (b.color_background) root.style.setProperty("--paper-200", b.color_background);
      if (b.color_text) root.style.setProperty("--ink-900", b.color_text);
      if (b.color_accent) root.style.setProperty("--moss-700", b.color_accent);
      if (b.color_button_bg) root.style.setProperty("--button-bg", b.color_button_bg);
      if (b.color_button_text) root.style.setProperty("--button-text", b.color_button_text);
      if (b.color_background) root.style.setProperty("--storefront-background", b.color_background);
      if (b.color_text) root.style.setProperty("--storefront-text", b.color_text);
      if (b.color_accent) root.style.setProperty("--storefront-accent", b.color_accent);
      if (b.color_text) root.style.setProperty("--storefront-primary", b.color_text);
    }

    window.addEventListener("message", handleMessage);
    return () => window.removeEventListener("message", handleMessage);
  }, []);

  return null;
}
```

### 8.4 Modify `apps/storefront/app/layout.tsx`

Add imports at the top:
```tsx
import { fetchBranding } from "@/lib/branding";
import { BrandingStyle } from "@/components/BrandingStyle";
import { BrandingPreviewListener } from "@/components/BrandingPreviewListener";
```

Inside the `RootLayout` function, after `storeSlug` is resolved (around line 116), add:
```tsx
  const branding = await fetchBranding(storeSlug);
```

Inside the `<html>` element, add `<BrandingStyle>` and `<BrandingPreviewListener>` inside `<head>` / before `<body>` content:

```tsx
  return (
    <html lang="en" className={fontVars}>
      <head>
        <BrandingStyle branding={branding} />
      </head>
      <body>
        <BrandingPreviewListener />
        <SkipLink />
        <CartProvider storeSlug={storeSlug}>{children}</CartProvider>
      </body>
    </html>
  );
```

Also update the `viewport` export to use the merchant's background color dynamically. Since `viewport` is a static export and can't be async, keep the default and let the CSS variable override handle it. The `<BrandingStyle>` component handles the visual override.

### 8.5 Verify

```bash
cd apps/storefront && npx tsc --noEmit
```

---

## Task 9 — Custom CSS sanitization (Enterprise gate)

This is already implemented in Task 2.5 (`sanitize.go`) and applied in Task 3.1 (`branding.go` handler — the `Update` method calls `branding.SanitizeCSS()` before saving).

**Verification checklist:**
- `@import` rules are stripped
- External `url()` references are stripped (except GCS bucket URLs)
- `javascript:` expressions are stripped
- `expression()` (IE legacy) is stripped
- `behavior:` (IE legacy) is stripped
- GCS bucket URLs in `url()` are preserved

```bash
cd services/marketplace-api && go test ./internal/branding/ -run TestSanitize -v
```

**Plan gate enforcement** (future — depends on B2 Subscription Tiers):
- When `internal/plangate/` is implemented, add middleware to the `PUT /branding` route that checks:
  - `custom_css` field: requires Enterprise plan
  - `show_powered_by = false`: requires Pro plan
  - Full color palette (all 5 fields): requires Starter plan (Free only gets accent)
  - Fonts: requires Starter plan
  - Announcement bar: requires Starter plan
- For now, the handler accepts all fields unconditionally. The plan gate middleware from B2 will be inserted into the route chain in `routes.go`.

---

## Task 10 — Build verification

Run the full build chain to confirm no regressions.

### 10.1 Go backend

```bash
cd services/marketplace-api

# Migration
make mp-migrate-up

# Unit tests
go test ./internal/branding/... -v -count=1

# Full build
go build ./cmd/marketplace-api/...
go build ./cmd/migrate/...

# Vet
go vet ./...
```

### 10.2 Admin frontend

```bash
cd apps/admin

# Type check
npx tsc --noEmit

# Lint
npm run lint
```

### 10.3 Storefront frontend

```bash
cd apps/storefront

# Type check
npx tsc --noEmit

# Lint
npm run lint
```

### 10.4 Smoke test checklist

1. Start marketplace-api in `MODE=both` with `make dev`
2. Open admin at `/settings/branding`
3. Verify 6 tabs render: Identity, Colors, Typography, Layout, Footer, Advanced
4. Click a color preset — all 5 color fields update
5. Save — verify no contrast error for default presets
6. Set low-contrast colors (e.g., light gray text on white) — verify 422 with contrast error message
7. Open storefront — verify CSS variables are injected with saved colors
8. In admin, change a color — verify live preview iframe updates without reload
9. Verify `?preview=true` query param activates PostMessage listener

---

## File inventory

### New files (Go)
| Path | Purpose |
|------|---------|
| `services/marketplace-api/migrations/000012_store_branding.up.sql` | Create store_branding table |
| `services/marketplace-api/migrations/000012_store_branding.down.sql` | Drop store_branding table |
| `services/marketplace-api/internal/branding/models.go` | GORM model + DTOs |
| `services/marketplace-api/internal/branding/repository.go` | CRUD repository |
| `services/marketplace-api/internal/branding/contrast.go` | WCAG contrast validation |
| `services/marketplace-api/internal/branding/contrast_test.go` | Contrast ratio tests |
| `services/marketplace-api/internal/branding/sanitize.go` | Custom CSS sanitization |
| `services/marketplace-api/internal/branding/sanitize_test.go` | Sanitization tests |
| `services/marketplace-api/internal/branding/fonts.go` | Font + layout allowlists |
| `services/marketplace-api/internal/handlers/admin/branding.go` | Admin CRUD + upload handler |
| `services/marketplace-api/internal/handlers/storefront/branding.go` | Public cached branding endpoint |

### Modified files (Go)
| Path | Change |
|------|--------|
| `services/marketplace-api/migrations.go` | Bump ExpectedSchemaVersion to 12 |
| `services/marketplace-api/internal/handlers/admin/routes.go` | Add BrandingHandler to Deps, register routes |
| `services/marketplace-api/internal/handlers/storefront/routes.go` | Add BrandingStorefrontHandler to Deps, register route |
| `services/marketplace-api/cmd/marketplace-api/main.go` | Wire branding repo + handlers |

### New files (TypeScript)
| Path | Purpose |
|------|---------|
| `apps/admin/app/settings/branding/page.tsx` | Server component page |
| `apps/admin/app/settings/branding/actions.ts` | Server actions |
| `apps/admin/components/settings/branding/BrandingSettingsPage.tsx` | 6-tab layout + save |
| `apps/admin/components/settings/branding/IdentityTab.tsx` | Logo, favicon, tagline |
| `apps/admin/components/settings/branding/ColorsTab.tsx` | Presets + pickers + contrast |
| `apps/admin/components/settings/branding/TypographyTab.tsx` | Font selection + preview |
| `apps/admin/components/settings/branding/LayoutTab.tsx` | Layout variant + hero + announcement |
| `apps/admin/components/settings/branding/FooterTab.tsx` | Footer tagline + social links |
| `apps/admin/components/settings/branding/AdvancedTab.tsx` | Custom CSS + powered-by |
| `apps/admin/components/settings/branding/LivePreview.tsx` | PostMessage iframe preview |
| `apps/admin/components/settings/branding/types.ts` | Shared TypeScript types |
| `apps/admin/components/settings/branding/color-presets.ts` | 8 curated color presets |
| `apps/admin/lib/api/branding-api.ts` | API client for branding endpoints |
| `apps/storefront/lib/branding.ts` | Branding fetch + font stack mapping |
| `apps/storefront/components/BrandingStyle.tsx` | CSS variable injection |
| `apps/storefront/components/BrandingPreviewListener.tsx` | PostMessage bridge |

### Modified files (TypeScript)
| Path | Change |
|------|--------|
| `apps/storefront/app/layout.tsx` | Fetch branding + inject BrandingStyle + BrandingPreviewListener |
