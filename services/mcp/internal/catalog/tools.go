// This file registers the five agent-facing catalog tools:
// list_store_products, get_store_product, list_store_categories,
// list_products_by_category, and get_store_branding.
//
// Every tool is scoped by a store's public URL slug, never by an internal
// id — the client's segment() already rejects an empty, ".", "..", or
// separator-bearing slug/handle before any request goes out, so a tool need
// not duplicate that check to be safe. What a tool DOES own is turning
// upstream.ErrNotFound into a typed "not found" result for the single-
// resource tools (get_store_product, get_store_branding): an empty
// []Product would read to a model as "this store sells nothing", which is a
// wrong answer that looks like a right one. Every other error — including a
// missing store on a LIST endpoint, where an empty list would be the same
// false signal — propagates unchanged so observe.OutcomeFor can classify it.
package catalog

import (
	"context"
	"errors"

	gsmcp "github.com/tesserix/go-shared/mcp"
	"github.com/tesserix/go-shared/mcp/upstream"
)

// storeSlugInput is embedded by every tool scoped to a single store.
type storeSlugInput struct {
	StoreSlug string `json:"store_slug" desc:"the store's public URL slug, e.g. bondi"`
}

// pageInput is embedded by the tools that page through a product list.
// Both fields are optional: a zero value leaves the request unpaginated and
// the upstream handler's own defaults (page 1, page_size 20) apply.
type pageInput struct {
	Page     int `json:"page,omitempty" desc:"1-based page number to fetch; omit for the first page"`
	PageSize int `json:"page_size,omitempty" desc:"number of products per page, capped at 100; omit for the default"`
}

// ListStoreProductsInput is the input for list_store_products.
type ListStoreProductsInput struct {
	storeSlugInput
	pageInput
}

// GetStoreProductInput is the input for get_store_product.
type GetStoreProductInput struct {
	storeSlugInput
	Handle string `json:"handle" desc:"the product's URL handle within the store, e.g. ceramic-mug"`
}

// ListStoreCategoriesInput is the input for list_store_categories.
type ListStoreCategoriesInput struct {
	storeSlugInput
}

// ListProductsByCategoryInput is the input for list_products_by_category.
type ListProductsByCategoryInput struct {
	storeSlugInput
	CategorySlug string `json:"category_slug" desc:"the category's URL slug within the store, e.g. mugs"`
	pageInput
}

// GetStoreBrandingInput is the input for get_store_branding.
type GetStoreBrandingInput struct {
	storeSlugInput
}

// ProductList is the output of the two product-list tools. A tool's output
// schema must be a closed object (additionalProperties:false); a bare JSON
// array cannot carry that, so the list is wrapped in a named field rather
// than returned directly.
type ProductList struct {
	Products []Product `json:"products"`
}

// CategoryList is the output of list_store_categories, wrapped for the same
// reason as ProductList.
type CategoryList struct {
	Categories []Category `json:"categories"`
}

// RegisterTools registers the five catalog tools against r, calling c to
// fulfil them.
func RegisterTools(r *gsmcp.Registry, c *Client) error {
	// A nil *Client — and the zero &Client{}, which is the shape the next
	// connector will copy from this one — nil-panics on the first tool call.
	// Failing here names the misconfiguration at startup instead of taking
	// down the first agent that calls a tool.
	if c == nil || c.upstream == nil {
		return errors.New("catalog: client is nil; NewClient must build it")
	}
	if err := gsmcp.Register(r, "list_store_products",
		"List the products published in a store's storefront catalog, paginated.",
		listStoreProducts(c)); err != nil {
		return err
	}
	if err := gsmcp.Register(r, "get_store_product",
		"Look up a single product in a store's storefront catalog by its handle. "+
			"Returns found=false, not an error, when the handle does not exist in the store.",
		getStoreProduct(c)); err != nil {
		return err
	}
	if err := gsmcp.Register(r, "list_store_categories",
		"List a store's storefront product categories.",
		listStoreCategories(c)); err != nil {
		return err
	}
	if err := gsmcp.Register(r, "list_products_by_category",
		"List the products published under one category of a store's storefront catalog, paginated.",
		listProductsByCategory(c)); err != nil {
		return err
	}
	if err := gsmcp.Register(r, "get_store_branding",
		"Look up a store's storefront branding: logo, tagline, accent color, "+
			"announcement text, and any active promotions. "+
			"Returns found=false, not an error, when the store does not exist.",
		getStoreBranding(c)); err != nil {
		return err
	}
	return nil
}

func listStoreProducts(c *Client) func(context.Context, ListStoreProductsInput) (ProductList, error) {
	return func(ctx context.Context, in ListStoreProductsInput) (ProductList, error) {
		items, err := c.ListProducts(ctx, in.StoreSlug, in.Page, in.PageSize)
		if err != nil {
			return ProductList{}, err
		}
		return ProductList{Products: projectProducts(items)}, nil
	}
}

func getStoreProduct(c *Client) func(context.Context, GetStoreProductInput) (Product, error) {
	return func(ctx context.Context, in GetStoreProductInput) (Product, error) {
		p, err := c.GetProduct(ctx, in.StoreSlug, in.Handle)
		if err != nil {
			if errors.Is(err, upstream.ErrNotFound) {
				return Product{Found: false}, nil
			}
			return Product{}, err
		}
		return projectProduct(p), nil
	}
}

func listStoreCategories(c *Client) func(context.Context, ListStoreCategoriesInput) (CategoryList, error) {
	return func(ctx context.Context, in ListStoreCategoriesInput) (CategoryList, error) {
		items, err := c.ListCategories(ctx, in.StoreSlug)
		if err != nil {
			return CategoryList{}, err
		}
		return CategoryList{Categories: projectCategories(items)}, nil
	}
}

func listProductsByCategory(c *Client) func(context.Context, ListProductsByCategoryInput) (ProductList, error) {
	return func(ctx context.Context, in ListProductsByCategoryInput) (ProductList, error) {
		items, err := c.ListByCategory(ctx, in.StoreSlug, in.CategorySlug, in.Page, in.PageSize)
		if err != nil {
			return ProductList{}, err
		}
		return ProductList{Products: projectProducts(items)}, nil
	}
}

// projectProducts projects a slice of storefront products, always returning
// a non-nil slice (make(...) is non-nil even at length 0) so ProductList's
// "products" field marshals to [] rather than null when a real store has no
// matches.
// projectCategories is projectProducts' counterpart, non-nil for the same
// reason: the closed output schema declares "categories" as an array.
func projectCategories(items []storefrontCategory) []Category {
	out := make([]Category, len(items))
	for i, it := range items {
		out[i] = projectCategory(it)
	}
	return out
}

func projectProducts(items []storefrontProduct) []Product {
	out := make([]Product, len(items))
	for i, it := range items {
		out[i] = projectProduct(it)
	}
	return out
}

func getStoreBranding(c *Client) func(context.Context, GetStoreBrandingInput) (Branding, error) {
	return func(ctx context.Context, in GetStoreBrandingInput) (Branding, error) {
		b, err := c.GetBranding(ctx, in.StoreSlug)
		if err != nil {
			if errors.Is(err, upstream.ErrNotFound) {
				return Branding{Found: false}, nil
			}
			return Branding{}, err
		}
		return projectBranding(b), nil
	}
}
