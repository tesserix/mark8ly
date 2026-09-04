package catalog

import "encoding/json"

// Product is the agent-facing result type for a single product. It is also
// the element type of ProductList, so it carries no Found field — that
// would be true, and meaningless, on every element of every list result.
// Found only matters for the single-resource lookup, where it is carried by
// the wrapper ProductResult instead.
// Prices are strings to preserve decimal precision without rounding.
// All fields are non-nullable: empty slices for collections, empty strings for
// optional text. This ensures the JSON wire format never contains null values,
// matching the derived schema.
type Product struct {
	Handle      string   `json:"handle"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	PriceMin    string   `json:"price_min"`
	PriceMax    string   `json:"price_max"`
	Currency    string   `json:"currency"`
	Categories  []string `json:"categories"`
	ImageURLs   []string `json:"image_urls"`
}

// ProductResult is the output of get_store_product. Found is meaningful only
// here, not on Product itself. Product is NESTED under the "product" field
// rather than embedded: Product has a value-receiver MarshalJSON (below),
// and embedding it would promote that method onto ProductResult, making
// ProductResult marshal as a bare Product and silently drop Found.
type ProductResult struct {
	Found   bool    `json:"found"`
	Product Product `json:"product"`
}

// Category is the agent-facing result type for a category.
type Category struct {
	Name     string `json:"name"`
	Slug     string `json:"slug"`
	Featured bool   `json:"featured"`
}

// Branding is the agent-facing result type for store branding. It carries no
// Found field for the same reason Product does not — Found is meaningful only
// on the single-resource wrapper BrandingResult.
// All fields are non-nullable: empty strings for optional text, empty slice
// for promotions. This ensures the JSON wire format never contains null values.
type Branding struct {
	LogoURL      string      `json:"logo_url"`
	Tagline      string      `json:"tagline"`
	AccentColor  string      `json:"accent_color"`
	Announcement string      `json:"announcement"`
	Promotions   []Promotion `json:"promotions"`
}

// BrandingResult is the output of get_store_branding. Branding is NESTED
// under the "branding" field rather than embedded, for the same reason as
// ProductResult: Branding has a value-receiver MarshalJSON, and embedding it
// would promote that method onto BrandingResult.
type BrandingResult struct {
	Found    bool     `json:"found"`
	Branding Branding `json:"branding"`
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

// MarshalJSON ensures a nil Products slice marshals to [] not null. Product
// and Branding already defend themselves this way; without the same guard here
// the wrappers depend on every construction site remembering make(...), and
// the closed output schema declares "products" as an array.
func (l ProductList) MarshalJSON() ([]byte, error) {
	type aux ProductList
	if l.Products == nil {
		l.Products = []Product{}
	}
	return json.Marshal(aux(l))
}

// MarshalJSON ensures a nil Categories slice marshals to [] not null, for the
// same reason as ProductList.
func (l CategoryList) MarshalJSON() ([]byte, error) {
	type aux CategoryList
	if l.Categories == nil {
		l.Categories = []Category{}
	}
	return json.Marshal(aux(l))
}
