// Package branding implements storefront branding customization for B1.
package branding

import (
	"time"

	"github.com/google/uuid"
)

// StoreBranding is the GORM model for the store_branding table.
type StoreBranding struct {
	ID               uuid.UUID `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID         uuid.UUID `gorm:"column:tenant_id;type:uuid;not null"`
	StoreID          uuid.UUID `gorm:"column:store_id;type:uuid;not null;uniqueIndex"`
	LogoURL          *string   `gorm:"column:logo_url"`
	FaviconURL       *string   `gorm:"column:favicon_url"`
	Tagline          *string   `gorm:"column:tagline;type:varchar(200)"`
	ColorBackground  string    `gorm:"column:color_background;type:varchar(7);not null;default:'#F7F6F2'"`
	ColorText        string    `gorm:"column:color_text;type:varchar(7);not null;default:'#0E0E0C'"`
	ColorAccent      string    `gorm:"column:color_accent;type:varchar(7);not null;default:'#2D4A2B'"`
	ColorButtonBg    string    `gorm:"column:color_button_bg;type:varchar(7);not null;default:'#0E0E0C'"`
	ColorButtonText  string    `gorm:"column:color_button_text;type:varchar(7);not null;default:'#F7F6F2'"`
	HeadingFont      string    `gorm:"column:heading_font;type:varchar(50);not null;default:'source-serif-4'"`
	BodyFont         string    `gorm:"column:body_font;type:varchar(50);not null;default:'source-sans-3'"`
	LayoutVariant    string    `gorm:"column:layout_variant;type:varchar(30);not null;default:'classic-shop'"`
	HeroImageURL     *string   `gorm:"column:hero_image_url"`
	AnnouncementText *string   `gorm:"column:announcement_text;type:varchar(300)"`
	AnnouncementLink *string   `gorm:"column:announcement_link"`
	AnnouncementBg   *string   `gorm:"column:announcement_bg;type:varchar(7)"`
	AnnouncementActive bool    `gorm:"column:announcement_active;not null;default:false"`
	FooterTagline    *string   `gorm:"column:footer_tagline;type:varchar(300)"`
	FooterCopyright  *string   `gorm:"column:footer_copyright;type:varchar(200)"`
	SocialInstagram  *string   `gorm:"column:social_instagram;type:varchar(300)"`
	SocialTwitter    *string   `gorm:"column:social_twitter;type:varchar(300)"`
	SocialFacebook   *string   `gorm:"column:social_facebook;type:varchar(300)"`
	SocialTiktok     *string   `gorm:"column:social_tiktok;type:varchar(300)"`
	SocialYoutube    *string   `gorm:"column:social_youtube;type:varchar(300)"`
	CustomCSS        *string   `gorm:"column:custom_css"`
	ShowPoweredBy    bool      `gorm:"column:show_powered_by;not null;default:true"`
	CreatedAt        time.Time `gorm:"column:created_at;not null;default:now()"`
	UpdatedAt        time.Time `gorm:"column:updated_at;not null;default:now()"`
}

func (StoreBranding) TableName() string { return "store_branding" }

// AllowedFonts is the curated list of permitted font keys.
var AllowedFonts = map[string]bool{
	"source-serif-4":   true,
	"playfair-display": true,
	"lora":             true,
	"inter":            true,
	"source-sans-3":    true,
	"dm-sans":          true,
}

// AllowedLayouts is the permitted set of layout variants.
var AllowedLayouts = map[string]bool{
	"classic-shop":    true,
	"editorial":       true,
	"minimal":         true,
	"hero-focus":      true,
}
