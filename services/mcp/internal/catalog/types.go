package catalog

import "encoding/json"

// Product is the agent-facing result type for a single product.
// Found indicates whether the product was found in the storefront.
// Prices are strings to preserve decimal precision without rounding.
// All fields are non-nullable: empty slices for collections, empty strings for
// optional text. This ensures the JSON wire format never contains null values,
// matching the derived schema.
type Product struct {
	Found       bool     `json:"found"`
	Handle      string   `json:"handle"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	PriceMin    string   `json:"price_min"`
	PriceMax    string   `json:"price_max"`
	Currency    string   `json:"currency"`
	Categories  []string `json:"categories"`
	ImageURLs   []string `json:"image_urls"`
}

// Category is the agent-facing result type for a category.
type Category struct {
	Name     string `json:"name"`
	Slug     string `json:"slug"`
	Featured bool   `json:"featured"`
}

// Branding is the agent-facing result type for store branding.
// Found indicates whether branding data was found for the store.
// All fields are non-nullable: empty strings for optional text, empty slice
// for promotions. This ensures the JSON wire format never contains null values.
type Branding struct {
	Found        bool        `json:"found"`
	LogoURL      string      `json:"logo_url"`
	Tagline      string      `json:"tagline"`
	AccentColor  string      `json:"accent_color"`
	Announcement string      `json:"announcement"`
	Promotions   []Promotion `json:"promotions"`
}

// Promotion is the agent-facing result type for an active storefront promotion.
type Promotion struct {
	Label      string `json:"label"`
	CouponCode string `json:"coupon_code"`
}

// MarshalJSON ensures nil slices marshal to [] not null, maintaining schema compliance.
func (p Product) MarshalJSON() ([]byte, error) {
	type aux Product
	if p.Categories == nil {
		p.Categories = []string{}
	}
	if p.ImageURLs == nil {
		p.ImageURLs = []string{}
	}
	return json.Marshal(aux(p))
}

// MarshalJSON ensures nil slices marshal to [] not null, maintaining schema compliance.
func (b Branding) MarshalJSON() ([]byte, error) {
	type aux Branding
	if b.Promotions == nil {
		b.Promotions = []Promotion{}
	}
	return json.Marshal(aux(b))
}
