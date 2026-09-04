package catalog

// Product is the agent-facing result type for a single product.
// Found indicates whether the product was found in the storefront.
// Prices are strings to preserve decimal precision without rounding.
type Product struct {
	Found       bool
	Handle      string
	Title       string
	Description string
	PriceMin    string
	PriceMax    string
	Currency    string
	Categories  []string
	ImageURLs   []string
}

// Category is the agent-facing result type for a category.
type Category struct {
	Name     string
	Slug     string
	Featured bool
}

// Branding is the agent-facing result type for store branding.
// Found indicates whether branding data was found for the store.
type Branding struct {
	Found        bool
	LogoURL      *string
	Tagline      *string
	AccentColor  string
	Announcement string
	Promotions   []Promotion
}

// Promotion is the agent-facing result type for an active storefront promotion.
type Promotion struct {
	Label      string
	CouponCode *string
}
