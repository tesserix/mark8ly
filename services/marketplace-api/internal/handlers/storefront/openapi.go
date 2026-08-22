// Package storefront — openapi.go: machine-readable OpenAPI spec for
// the customer-facing read endpoints, plus the route that serves it.
//
// This is the "Phase 3" piece of the SLM/MCP coverage work. The
// `mcp-gateway` service pulls this spec at startup and auto-registers
// every operation tagged with `x-mcp-expose: customer-read` as an MCP
// tool the SLM can call — broadening the SLM's read coverage of the
// storefront without anyone hand-rolling per-endpoint tool wrappers.
//
// Hand-written rather than swag-generated on purpose: we want the
// exact surface that's exposed to the SLM to be reviewable in a
// single file, not derived from comment annotations scattered across
// the codebase. Spec drift is caught by integration tests against
// real handler responses.
package storefront

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// MCPExposureLevel is the controlled vocabulary for the
// `x-mcp-expose` extension. Only one value is meaningful today —
// `customer-read` — but the shape allows future levels (e.g.
// `staff-read`) without restructuring the spec.
const MCPExposureLevel = "customer-read"

// ServeOpenAPISpec mounts a GET handler that returns the OpenAPI 3.0
// document for the storefront customer-facing read endpoints. It's
// public (no storefront key required) because the spec itself is
// metadata, not customer data — same reasoning as why robots.txt
// isn't gated.
func ServeOpenAPISpec(router *gin.RouterGroup) {
	router.GET("/storefront/openapi.json", func(c *gin.Context) {
		c.JSON(http.StatusOK, openAPISpec())
	})
}

// openAPISpec returns the spec as a freshly-built dict per call. We
// don't memoize because the surface is small and rebuilds amortise
// across whatever fanout mcp-gateway does at startup.
func openAPISpec() gin.H {
	return gin.H{
		"openapi": "3.0.3",
		"info": gin.H{
			"title":       "mark8ly storefront read API",
			"version":     "1.0.0",
			"description": "Customer-facing read endpoints exposed to the SLM via mcp-gateway. Every operation tagged `x-mcp-expose: customer-read` is auto-registered as an MCP tool.",
		},
		"servers": []gin.H{
			{
				"url":         "/",
				"description": "Same-origin — the mcp-gateway service in mark8ly namespace targets this base URL.",
			},
		},
		"paths": gin.H{
			"/storefront/stores/{storeSlug}/products":                   listProductsOp(),
			"/storefront/stores/{storeSlug}/products/{handle}":          getProductOp(),
			"/storefront/stores/{storeSlug}/categories":                 listCategoriesOp(),
			"/storefront/stores/{storeSlug}/categories/{slug}/products": listByCategoryOp(),
			"/storefront/stores/{storeSlug}/branding":                   getBrandingOp(),
		},
	}
}

// Helper: every operation under this spec needs the same storeSlug
// path parameter, so build it once.
func storeSlugPathParam() gin.H {
	return gin.H{
		"name":        "storeSlug",
		"in":          "path",
		"required":    true,
		"description": "The store's URL slug (e.g. `tesserix-store`). Distinct from `store_id`.",
		"schema": gin.H{
			"type": "string",
		},
	}
}

func listProductsOp() gin.H {
	return gin.H{
		"get": gin.H{
			"operationId":  "listStoreProducts",
			"x-mcp-expose": MCPExposureLevel,
			"summary":      "List the published products in a store.",
			"description":  "Paginated list of products available for purchase. Use the `limit` and `offset` query parameters to page. Returns title, handle, price, and a short description per product.",
			"parameters": []gin.H{
				storeSlugPathParam(),
				{
					"name":        "limit",
					"in":          "query",
					"description": "Maximum number of products to return. Default 20, max 100.",
					"schema":      gin.H{"type": "integer", "default": 20},
				},
				{
					"name":        "offset",
					"in":          "query",
					"description": "Number of products to skip for pagination.",
					"schema":      gin.H{"type": "integer", "default": 0},
				},
			},
		},
	}
}

func getProductOp() gin.H {
	return gin.H{
		"get": gin.H{
			"operationId":  "getStoreProduct",
			"x-mcp-expose": MCPExposureLevel,
			"summary":      "Get product detail by handle.",
			"description":  "Full product detail including images, variants, price, and inventory state. The `handle` is the URL-safe slug for the product (visible in the storefront URL bar).",
			"parameters": []gin.H{
				storeSlugPathParam(),
				{
					"name":        "handle",
					"in":          "path",
					"required":    true,
					"description": "Product handle (URL-safe slug).",
					"schema":      gin.H{"type": "string"},
				},
			},
		},
	}
}

func listCategoriesOp() gin.H {
	return gin.H{
		"get": gin.H{
			"operationId":  "listStoreCategories",
			"x-mcp-expose": MCPExposureLevel,
			"summary":      "List the categories a store has organised its catalog into.",
			"description":  "Returns the category tree with slug, name, and product counts. Use the slug to drill into products with `listProductsByCategory`.",
			"parameters": []gin.H{
				storeSlugPathParam(),
			},
		},
	}
}

func listByCategoryOp() gin.H {
	return gin.H{
		"get": gin.H{
			"operationId":  "listProductsByCategory",
			"x-mcp-expose": MCPExposureLevel,
			"summary":      "List the products in a category.",
			"description":  "Same shape as `listStoreProducts` but filtered to a single category. Use `listStoreCategories` first to find the category slug.",
			"parameters": []gin.H{
				storeSlugPathParam(),
				{
					"name":        "slug",
					"in":          "path",
					"required":    true,
					"description": "Category slug from `listStoreCategories`.",
					"schema":      gin.H{"type": "string"},
				},
				{
					"name":        "limit",
					"in":          "query",
					"description": "Maximum number of products to return. Default 20, max 100.",
					"schema":      gin.H{"type": "integer", "default": 20},
				},
				{
					"name":        "offset",
					"in":          "query",
					"description": "Number of products to skip for pagination.",
					"schema":      gin.H{"type": "integer", "default": 0},
				},
			},
		},
	}
}

func getBrandingOp() gin.H {
	return gin.H{
		"get": gin.H{
			"operationId":  "getStoreBranding",
			"x-mcp-expose": MCPExposureLevel,
			"summary":      "Get the store's public branding (logo, primary colour, active promotions).",
			"description":  "Useful for the SLM to ground itself in the store's identity when answering open-ended questions (e.g. \"what does this store sell?\").",
			"parameters": []gin.H{
				storeSlugPathParam(),
			},
		},
	}
}
