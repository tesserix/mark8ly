// Package catalog is the typed client mcp-catalog uses to call
// marketplace-api's public storefront endpoints.
//
// The endpoints it calls read page/page_size for pagination, not the
// limit/offset the hand-written OpenAPI document (now retired) claimed —
// see listPublishedQuery in marketplace-api's
// internal/handlers/storefront/products.go. Sending limit/offset does not
// error; the handler ignores them and returns page 1 every time. Every
// method here builds page/page_size explicitly for that reason.
package catalog

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/tesserix/go-shared/mcp/upstream"
)

// maxPageSize mirrors listPublishedQuery's `binding:"max=100"` in
// marketplace-api. The handler rejects a page_size above this with a
// (deliberately opaque, 404-shaped) validation failure; this client rejects
// it before the request goes out, naming the cap, rather than either
// forwarding a value the handler will bounce or silently clamping it. An
// agent told it received N products when the handler actually gave it 100
// would summarise a catalogue it never saw.
const maxPageSize = 100

// Client calls marketplace-api's storefront endpoints for one product API.
type Client struct {
	upstream *upstream.Client
}

// NewClient builds a Client rooted at baseURL, authenticating with
// storefrontKey via the X-Storefront-Key header the storefront middleware
// chain expects, and bounding every request to timeout.
func NewClient(baseURL, storefrontKey string, timeout time.Duration) (*Client, error) {
	uc, err := upstream.New(baseURL,
		upstream.WithHeader("X-Storefront-Key", storefrontKey),
		upstream.WithTimeout(timeout),
	)
	if err != nil {
		return nil, fmt.Errorf("catalog: %w", err)
	}
	return &Client{upstream: uc}, nil
}

// storefrontProductsEnvelope is the wire shape of the list-products and
// list-by-category endpoints: {"data": [...], "meta": {...}}. Only "data"
// is projected — meta.page/meta.page_size just echo the request.
type storefrontProductsEnvelope struct {
	Data []storefrontProduct `json:"data"`
}

// storefrontCategoriesEnvelope is the wire shape of the list-categories
// endpoint: {"data": [...]}.
type storefrontCategoriesEnvelope struct {
	Data []storefrontCategory `json:"data"`
}

// storefrontBrandingEnvelope is the wire shape of the branding endpoint.
// The real handler (branding.go's BrandingHandler.Get) nests the public
// branding fields under "branding" and, when a campaign has an active
// storefront promotion, adds it as a *singular* sibling key
// "active_promotion" — never plural, never nested inside "branding", and
// never an array. See NewClient's caller-facing GetBranding for how this
// gets folded into storefrontBranding's Promotions slice.
type storefrontBrandingEnvelope struct {
	Branding struct {
		LogoURL          *string `json:"logo_url,omitempty"`
		Tagline          *string `json:"tagline,omitempty"`
		ColorAccent      string  `json:"color_accent"`
		AnnouncementText *string `json:"announcement_text,omitempty"`
	} `json:"branding"`
	ActivePromotion *storefrontPromotion `json:"active_promotion,omitempty"`
}

type storefrontProduct struct {
	ID          string                  `json:"id"`
	Handle      string                  `json:"handle"`
	Title       string                  `json:"title"`
	Description *string                 `json:"description,omitempty"`
	Categories  []storefrontCategoryRef `json:"categories"`
	Media       []storefrontMedia       `json:"media"`
	PriceRange  storefrontPriceRange    `json:"price_range"`
	// Mirrored so the projection can be tested for dropping them. Never
	// projected — see the Global Constraints.
	TaxCode         *string `json:"tax_code,omitempty"`
	TaxRateOverride *string `json:"tax_rate_override,omitempty"`
	TaxCategory     *string `json:"tax_category,omitempty"`
}

type storefrontCategoryRef struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type storefrontCategory struct {
	Name     string `json:"name"`
	Slug     string `json:"slug"`
	Position int    `json:"position"`
	Featured bool   `json:"featured"`
}

type storefrontMedia struct {
	URL       string `json:"url"`
	MediaType string `json:"media_type"`
	Position  int    `json:"position"`
}

// Prices are strings on the wire: marketplace-api uses decimal.Decimal, which
// marshals as a JSON string. Keep them strings the whole way through.
type storefrontPriceRange struct {
	Min          string `json:"min"`
	Max          string `json:"max"`
	CurrencyCode string `json:"currency_code"`
}

type storefrontBranding struct {
	LogoURL          *string               `json:"logo_url,omitempty"`
	Tagline          *string               `json:"tagline,omitempty"`
	ColorAccent      string                `json:"color_accent"`
	AnnouncementText *string               `json:"announcement_text,omitempty"`
	Promotions       []storefrontPromotion `json:"active_promotions"`
}

type storefrontPromotion struct {
	Label      string  `json:"label"`
	CouponCode *string `json:"coupon_code,omitempty"`
}

// pageParams validates page/pageSize against the handler's own bind rules
// and returns the url.Values to send. page<=0 and pageSize<=0 are left off
// the query entirely so the handler's own defaults (page 1, page_size 20)
// apply, matching listPublishedQuery.defaults().
func pageParams(page, pageSize int) (url.Values, error) {
	if pageSize > maxPageSize {
		return nil, fmt.Errorf("catalog: page_size %d exceeds the storefront API's cap of %d", pageSize, maxPageSize)
	}
	params := url.Values{}
	if page > 0 {
		params.Set("page", strconv.Itoa(page))
	}
	if pageSize > 0 {
		params.Set("page_size", strconv.Itoa(pageSize))
	}
	return params, nil
}

// segment validates and escapes one caller-supplied path segment (a store
// slug, product handle, or category slug — all of which arrive from an
// agent). Every path builder in this file routes through it so a future
// method can't forget the check.
//
// Escaping alone (url.PathEscape) is not enough: it leaves "." and ".."
// untouched, and go-shared's upstream.Get runs path.Clean on the joined
// path. With a base URL that has no path component — as this client's
// does — upstream.Get's own traversal guard is inert (it only fires when
// the base path is non-empty), so "stores/../products/mug" collapses to
// "/products/mug", a different, unscoped route, with no error. Rejecting
// "." and ".." here, before any path is built, closes that regardless of
// what the shared client's guard does or doesn't catch.
//
// unescaping the segment and checking for "/" additionally rejects an
// encoded slash (e.g. "a%2Fb"), which — combined with a "." — could
// otherwise reconstruct a traversal after decoding.
func segment(paramName, value string) (string, error) {
	if value == "." || value == ".." {
		return "", fmt.Errorf("catalog: %s %q is not a valid path segment", paramName, value)
	}
	if decoded, err := url.PathUnescape(value); err == nil && strings.Contains(decoded, "/") {
		return "", fmt.Errorf("catalog: %s %q contains a path separator", paramName, value)
	}
	return url.PathEscape(value), nil
}

func storeProductsPath(slug string) (string, error) {
	seg, err := segment("slug", slug)
	if err != nil {
		return "", err
	}
	return "/api/v1/storefront/stores/" + seg + "/products", nil
}

// ListProducts calls GET /api/v1/storefront/stores/{slug}/products.
func (c *Client) ListProducts(ctx context.Context, slug string, page, pageSize int) ([]storefrontProduct, error) {
	params, err := pageParams(page, pageSize)
	if err != nil {
		return nil, err
	}
	path, err := storeProductsPath(slug)
	if err != nil {
		return nil, err
	}
	var env storefrontProductsEnvelope
	if err := c.upstream.Get(ctx, path, params, &env); err != nil {
		return nil, err
	}
	return env.Data, nil
}

// GetProduct calls GET /api/v1/storefront/stores/{slug}/products/{handle}.
// This is a single-resource fetch: no page/page_size and no envelope.
func (c *Client) GetProduct(ctx context.Context, slug, handle string) (storefrontProduct, error) {
	base, err := storeProductsPath(slug)
	if err != nil {
		return storefrontProduct{}, err
	}
	handleSeg, err := segment("handle", handle)
	if err != nil {
		return storefrontProduct{}, err
	}
	var p storefrontProduct
	if err := c.upstream.Get(ctx, base+"/"+handleSeg, nil, &p); err != nil {
		return storefrontProduct{}, err
	}
	return p, nil
}

// ListCategories calls GET /api/v1/storefront/stores/{slug}/categories.
func (c *Client) ListCategories(ctx context.Context, slug string) ([]storefrontCategory, error) {
	slugSeg, err := segment("slug", slug)
	if err != nil {
		return nil, err
	}
	path := "/api/v1/storefront/stores/" + slugSeg + "/categories"
	var env storefrontCategoriesEnvelope
	if err := c.upstream.Get(ctx, path, nil, &env); err != nil {
		return nil, err
	}
	return env.Data, nil
}

// ListByCategory calls
// GET /api/v1/storefront/stores/{slug}/categories/{categorySlug}/products.
func (c *Client) ListByCategory(ctx context.Context, slug, categorySlug string, page, pageSize int) ([]storefrontProduct, error) {
	params, err := pageParams(page, pageSize)
	if err != nil {
		return nil, err
	}
	slugSeg, err := segment("slug", slug)
	if err != nil {
		return nil, err
	}
	categorySeg, err := segment("categorySlug", categorySlug)
	if err != nil {
		return nil, err
	}
	path := "/api/v1/storefront/stores/" + slugSeg +
		"/categories/" + categorySeg + "/products"
	var env storefrontProductsEnvelope
	if err := c.upstream.Get(ctx, path, params, &env); err != nil {
		return nil, err
	}
	return env.Data, nil
}

// GetBranding calls GET /api/v1/storefront/stores/{slug}/branding.
//
// The response nests branding fields under a "branding" key and, when a
// campaign has an active storefront promotion, adds it as a singular
// "active_promotion" sibling key holding at most one promotion — not the
// plural "active_promotions" array this client's storefrontBranding type
// otherwise mirrors. That field folds a present ActivePromotion into a
// one-element Promotions slice, and an absent one into an empty slice.
func (c *Client) GetBranding(ctx context.Context, slug string) (storefrontBranding, error) {
	slugSeg, err := segment("slug", slug)
	if err != nil {
		return storefrontBranding{}, err
	}
	path := "/api/v1/storefront/stores/" + slugSeg + "/branding"
	var env storefrontBrandingEnvelope
	if err := c.upstream.Get(ctx, path, nil, &env); err != nil {
		return storefrontBranding{}, err
	}
	b := storefrontBranding{
		LogoURL:          env.Branding.LogoURL,
		Tagline:          env.Branding.Tagline,
		ColorAccent:      env.Branding.ColorAccent,
		AnnouncementText: env.Branding.AnnouncementText,
	}
	if env.ActivePromotion != nil {
		b.Promotions = []storefrontPromotion{*env.ActivePromotion}
	}
	return b, nil
}
