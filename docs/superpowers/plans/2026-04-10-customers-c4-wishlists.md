# Customers C4 — Wishlists Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Ship wishlists: heart icon on product cards + detail page, storefront wishlist page under /account/wishlist, toggle add/remove with optimistic UI, auth required.

**Architecture:** New `internal/wishlist/` package (model, repository). Simple junction table (customer_id, product_id). Storefront handlers behind RequireCustomerAuth. Heart icon as client component with optimistic toggle.

**Tech Stack:** Go 1.26, Gin, GORM. Next.js 16, React 19, Tailwind.

**Prerequisite:** C1 (Storefront Auth) must be on main.

**Spec reference:** `docs/superpowers/specs/2026-04-10-customers-reviews-design.md` -- section 11 (C4 Wishlists), section 12 (Testing).

---

## File structure produced by C4

```
services/marketplace-api/
├── migrations/
│   ├── 000015_wishlists.up.sql              # NEW — wishlists junction table
│   └── 000015_wishlists.down.sql            # NEW
├── internal/
│   └── wishlist/
│       ├── models.go                        # NEW — Wishlist GORM model + response DTOs
│       └── repository.go                    # NEW — Repository interface + GORM implementation
├── internal/handlers/storefront/
│   ├── wishlist.go                          # NEW — WishlistHandler (list, add, remove, check)
│   └── routes.go                           # MODIFY — add WishlistHandler to Deps + register routes
├── cmd/marketplace-api/main.go             # MODIFY — wire wishlist repo + handler
└── migrations.go                           # MODIFY — bump ExpectedSchemaVersion

apps/storefront/
├── lib/api/
│   └── wishlist-api.ts                      # NEW — wishlist API client functions
├── components/
│   ├── WishlistButton.tsx                   # NEW — heart icon client component with optimistic toggle
│   ├── ProductDetails.tsx                   # MODIFY — add WishlistButton next to title
│   └── WishlistProductCard.tsx              # NEW — product card with remove + add-to-cart for wishlist page
├── app/products/
│   └── page.tsx                            # MODIFY — add WishlistButton to ProductCard
└── app/account/wishlist/
    └── page.tsx                            # NEW — wishlist grid page
```

---

## Task 1: Migration — wishlists table

**Files:** `services/marketplace-api/migrations/000015_wishlists.up.sql`, `services/marketplace-api/migrations/000015_wishlists.down.sql`

> **Migration number:** Check the latest migration in `services/marketplace-api/migrations/`. The spec says this can be part of 000013 or separate. Since C1-C3 produce 000013 and 000014, use **000015**. If migrations have shifted, adjust the number to be `latest + 1`.

- [ ] **Step 1: Create up migration**

Create `services/marketplace-api/migrations/000015_wishlists.up.sql`:

```sql
-- 000015_wishlists: Customer wishlist junction table.
BEGIN;

CREATE TABLE wishlists (
    id              UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID          NOT NULL,
    store_id        UUID          NOT NULL,
    customer_id     UUID          NOT NULL REFERENCES customer_profiles(id) ON DELETE CASCADE,
    product_id      UUID          NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT now(),
    UNIQUE (customer_id, product_id)
);

CREATE INDEX wishlists_customer_idx ON wishlists (customer_id);
CREATE INDEX wishlists_product_idx ON wishlists (product_id);

COMMIT;
```

- [ ] **Step 2: Create down migration**

Create `services/marketplace-api/migrations/000015_wishlists.down.sql`:

```sql
BEGIN;
DROP TABLE IF EXISTS wishlists;
COMMIT;
```

- [ ] **Step 3: Bump ExpectedSchemaVersion**

Edit `services/marketplace-api/migrations.go`:

Change:
```go
const ExpectedSchemaVersion uint = 14
```

To:
```go
const ExpectedSchemaVersion uint = 15
```

> **Note:** The current value at time of plan writing is 11. C1 bumps to 13, C3 bumps to 14. If the actual value differs when you execute, set it to `current + 1` to match the new migration number.

- [ ] **Step 4: Verify migration compiles**

```bash
cd services/marketplace-api && go build ./...
```

---

## Task 2: `internal/wishlist/` models + repository

**Files:** `services/marketplace-api/internal/wishlist/models.go`, `services/marketplace-api/internal/wishlist/repository.go`

### Step 1: Create models

- [ ] Create `services/marketplace-api/internal/wishlist/models.go`:

```go
// Package wishlist owns the customer wishlist aggregate — a simple
// junction table between customer_profiles and products.
package wishlist

import (
	"time"

	"github.com/google/uuid"
)

// Wishlist is the GORM model for the wishlists junction table.
// One row = one customer has saved one product.
type Wishlist struct {
	ID         uuid.UUID `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID   uuid.UUID `gorm:"column:tenant_id;type:uuid;not null"`
	StoreID    uuid.UUID `gorm:"column:store_id;type:uuid;not null"`
	CustomerID uuid.UUID `gorm:"column:customer_id;type:uuid;not null"`
	ProductID  uuid.UUID `gorm:"column:product_id;type:uuid;not null"`
	CreatedAt  time.Time `gorm:"column:created_at;not null;default:now()"`
}

func (Wishlist) TableName() string { return "wishlists" }

// WishlistItem is the read projection returned by List — joins
// wishlist row with product details for the storefront page.
type WishlistItem struct {
	ID        uuid.UUID `json:"id"`
	ProductID uuid.UUID `json:"product_id"`
	CreatedAt time.Time `json:"created_at"`

	// Joined from products table
	ProductTitle    string  `json:"product_title"`
	ProductHandle   string  `json:"product_handle"`
	ProductImageURL *string `json:"product_image_url"`
	ProductMinPrice string  `json:"product_min_price"`
	ProductMaxPrice string  `json:"product_max_price"`
	CurrencyCode    string  `json:"currency_code"`
	InStock         bool    `json:"in_stock"`
}
```

### Step 2: Create repository

- [ ] Create `services/marketplace-api/internal/wishlist/repository.go`:

```go
package wishlist

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// Repository is the data-access surface for the wishlist aggregate.
type Repository interface {
	// Add inserts a wishlist row. Uses ON CONFLICT DO NOTHING for
	// idempotent add — calling Add twice for the same (customer, product)
	// is a no-op, not an error.
	Add(ctx context.Context, db *gorm.DB, item *Wishlist) error

	// Remove deletes by customer_id + product_id. Naturally idempotent —
	// DELETE of a non-existent row is not an error.
	Remove(ctx context.Context, db *gorm.DB, customerID, productID uuid.UUID) error

	// List returns all wishlist items for a customer with product details
	// (title, handle, image, price range, stock). Ordered by created_at DESC.
	List(ctx context.Context, db *gorm.DB, customerID uuid.UUID, page, limit int) ([]WishlistItem, int64, error)

	// Check returns true if the given product is in the customer's wishlist.
	Check(ctx context.Context, db *gorm.DB, customerID, productID uuid.UUID) (bool, error)

	// CountByCustomer returns the number of items in a customer's wishlist.
	// Used by the admin customer detail page (C2) for wishlist count stat.
	CountByCustomer(ctx context.Context, db *gorm.DB, customerID uuid.UUID) (int64, error)
}

type gormRepository struct{}

// NewRepository returns a stateless GORM-backed Repository.
func NewRepository() Repository { return &gormRepository{} }

func (gormRepository) Add(ctx context.Context, db *gorm.DB, item *Wishlist) error {
	// ON CONFLICT DO NOTHING — idempotent. The UNIQUE(customer_id, product_id)
	// constraint prevents duplicates; a second add for the same pair silently
	// succeeds without error.
	result := db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "customer_id"}, {Name: "product_id"}},
		DoNothing: true,
	}).Create(item)
	return result.Error
}

func (gormRepository) Remove(ctx context.Context, db *gorm.DB, customerID, productID uuid.UUID) error {
	result := db.WithContext(ctx).
		Where("customer_id = ? AND product_id = ?", customerID, productID).
		Delete(&Wishlist{})
	return result.Error
}

func (gormRepository) List(ctx context.Context, db *gorm.DB, customerID uuid.UUID, page, limit int) ([]WishlistItem, int64, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 50
	}
	offset := (page - 1) * limit

	// Count total items for pagination.
	var total int64
	if err := db.WithContext(ctx).
		Model(&Wishlist{}).
		Where("customer_id = ?", customerID).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if total == 0 {
		return []WishlistItem{}, 0, nil
	}

	// Join wishlists with products to get product details.
	// Also join product_media (position=1) for the cover image and
	// variants for price range and stock.
	var items []WishlistItem
	err := db.WithContext(ctx).Raw(`
		SELECT
			w.id,
			w.product_id,
			w.created_at,
			p.title   AS product_title,
			p.handle  AS product_handle,
			pm.url    AS product_image_url,
			COALESCE(MIN(v.price)::text, '0')  AS product_min_price,
			COALESCE(MAX(v.price)::text, '0')  AS product_max_price,
			COALESCE(MIN(v.currency_code), '')  AS currency_code,
			COALESCE(bool_or(v.inventory_quantity > 0), false) AS in_stock
		FROM wishlists w
		JOIN products p ON p.id = w.product_id
		LEFT JOIN LATERAL (
			SELECT url FROM product_media
			WHERE product_id = p.id
			ORDER BY position ASC
			LIMIT 1
		) pm ON true
		LEFT JOIN variants v ON v.product_id = p.id
		WHERE w.customer_id = ?
		  AND p.status = 'active'
		GROUP BY w.id, w.product_id, w.created_at, p.title, p.handle, pm.url
		ORDER BY w.created_at DESC
		LIMIT ? OFFSET ?
	`, customerID, limit, offset).Scan(&items).Error

	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (gormRepository) Check(ctx context.Context, db *gorm.DB, customerID, productID uuid.UUID) (bool, error) {
	var count int64
	err := db.WithContext(ctx).
		Model(&Wishlist{}).
		Where("customer_id = ? AND product_id = ?", customerID, productID).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (gormRepository) CountByCustomer(ctx context.Context, db *gorm.DB, customerID uuid.UUID) (int64, error) {
	var count int64
	err := db.WithContext(ctx).
		Model(&Wishlist{}).
		Where("customer_id = ?", customerID).
		Count(&count).Error
	return count, err
}
```

- [ ] **Step 3: Verify compilation**

```bash
cd services/marketplace-api && go build ./internal/wishlist/...
```

---

## Task 3: Storefront wishlist handler

**File:** `services/marketplace-api/internal/handlers/storefront/wishlist.go`

- [ ] **Step 1: Create the handler file**

```go
package storefront

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/internal/wishlist"
)

// WishlistHandler bundles dependencies for storefront wishlist endpoints.
// All methods require RequireCustomerAuth middleware upstream.
type WishlistHandler struct {
	db     *gorm.DB
	repo   wishlist.Repository
	logger *slog.Logger
}

// NewWishlistHandler constructs a storefront WishlistHandler.
func NewWishlistHandler(db *gorm.DB, repo wishlist.Repository, logger *slog.Logger) *WishlistHandler {
	return &WishlistHandler{db: db, repo: repo, logger: logger}
}

// --- Request/Response DTOs ---

type wishlistAddRequest struct {
	ProductID string `json:"product_id" binding:"required,uuid"`
}

type wishlistItemResponse struct {
	ID              string  `json:"id"`
	ProductID       string  `json:"product_id"`
	CreatedAt       string  `json:"created_at"`
	ProductTitle    string  `json:"product_title"`
	ProductHandle   string  `json:"product_handle"`
	ProductImageURL *string `json:"product_image_url"`
	ProductMinPrice string  `json:"product_min_price"`
	ProductMaxPrice string  `json:"product_max_price"`
	CurrencyCode    string  `json:"currency_code"`
	InStock         bool    `json:"in_stock"`
}

type wishlistListResponse struct {
	Data  []wishlistItemResponse `json:"data"`
	Total int64                  `json:"total"`
	Page  int                    `json:"page"`
	Limit int                    `json:"limit"`
}

type wishlistCheckResponse struct {
	InWishlist bool `json:"in_wishlist"`
}

// --- Handlers ---

// List handles GET /storefront/stores/:storeSlug/wishlist
func (h *WishlistHandler) List(c *gin.Context) {
	customerProfileID := h.resolveCustomerProfileID(c)
	if customerProfileID == uuid.Nil {
		return
	}

	page := queryInt(c, "page", 1)
	limit := queryInt(c, "limit", 50)

	items, total, err := h.repo.List(c.Request.Context(), h.db, customerProfileID, page, limit)
	if err != nil {
		h.logger.Error("wishlist: list", "err", err, "customer_id", customerProfileID)
		c.JSON(http.StatusInternalServerError, map[string]any{
			"error":   "internal",
			"message": "failed to list wishlist",
		})
		return
	}

	resp := wishlistListResponse{
		Data:  make([]wishlistItemResponse, 0, len(items)),
		Total: total,
		Page:  page,
		Limit: limit,
	}
	for _, item := range items {
		resp.Data = append(resp.Data, wishlistItemResponse{
			ID:              item.ID.String(),
			ProductID:       item.ProductID.String(),
			CreatedAt:       item.CreatedAt.Format("2006-01-02T15:04:05Z"),
			ProductTitle:    item.ProductTitle,
			ProductHandle:   item.ProductHandle,
			ProductImageURL: item.ProductImageURL,
			ProductMinPrice: item.ProductMinPrice,
			ProductMaxPrice: item.ProductMaxPrice,
			CurrencyCode:    item.CurrencyCode,
			InStock:         item.InStock,
		})
	}
	c.JSON(http.StatusOK, resp)
}

// Add handles POST /storefront/stores/:storeSlug/wishlist
func (h *WishlistHandler) Add(c *gin.Context) {
	customerProfileID := h.resolveCustomerProfileID(c)
	if customerProfileID == uuid.Nil {
		return
	}
	store := h.resolveStore(c)
	if store == nil {
		return
	}

	var req wishlistAddRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "validation_failed",
			"message": "product_id is required and must be a valid UUID",
		})
		return
	}

	productID, _ := uuid.Parse(req.ProductID)
	storeID, _ := uuid.Parse(store.ID)
	tenantID, _ := uuid.Parse(store.TenantID)

	item := &wishlist.Wishlist{
		TenantID:   tenantID,
		StoreID:    storeID,
		CustomerID: customerProfileID,
		ProductID:  productID,
	}

	if err := h.repo.Add(c.Request.Context(), h.db, item); err != nil {
		h.logger.Error("wishlist: add", "err", err, "customer_id", customerProfileID, "product_id", productID)
		c.JSON(http.StatusInternalServerError, map[string]any{
			"error":   "internal",
			"message": "failed to add to wishlist",
		})
		return
	}

	c.JSON(http.StatusCreated, map[string]any{
		"message": "added to wishlist",
	})
}

// Remove handles DELETE /storefront/stores/:storeSlug/wishlist/:productId
func (h *WishlistHandler) Remove(c *gin.Context) {
	customerProfileID := h.resolveCustomerProfileID(c)
	if customerProfileID == uuid.Nil {
		return
	}

	productIDStr := c.Param("productId")
	productID, err := uuid.Parse(productIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "validation_failed",
			"message": "invalid product ID",
		})
		return
	}

	if err := h.repo.Remove(c.Request.Context(), h.db, customerProfileID, productID); err != nil {
		h.logger.Error("wishlist: remove", "err", err, "customer_id", customerProfileID, "product_id", productID)
		c.JSON(http.StatusInternalServerError, map[string]any{
			"error":   "internal",
			"message": "failed to remove from wishlist",
		})
		return
	}

	c.JSON(http.StatusOK, map[string]any{
		"message": "removed from wishlist",
	})
}

// Check handles GET /storefront/stores/:storeSlug/wishlist/check?product_id=
func (h *WishlistHandler) Check(c *gin.Context) {
	customerProfileID := h.resolveCustomerProfileID(c)
	if customerProfileID == uuid.Nil {
		return
	}

	productIDStr := c.Query("product_id")
	productID, err := uuid.Parse(productIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "validation_failed",
			"message": "product_id query parameter is required and must be a valid UUID",
		})
		return
	}

	inWishlist, err := h.repo.Check(c.Request.Context(), h.db, customerProfileID, productID)
	if err != nil {
		h.logger.Error("wishlist: check", "err", err, "customer_id", customerProfileID, "product_id", productID)
		c.JSON(http.StatusInternalServerError, map[string]any{
			"error":   "internal",
			"message": "failed to check wishlist",
		})
		return
	}

	c.JSON(http.StatusOK, wishlistCheckResponse{InWishlist: inWishlist})
}

// --- Helpers ---

// resolveCustomerProfileID extracts the customer profile ID set by
// RequireCustomerAuth middleware. Returns uuid.Nil and aborts if missing.
func (h *WishlistHandler) resolveCustomerProfileID(c *gin.Context) uuid.UUID {
	raw, exists := c.Get(CustomerProfileIDKey)
	if !exists {
		c.AbortWithStatusJSON(http.StatusUnauthorized, map[string]any{
			"error":   "unauthorized",
			"message": "customer authentication required",
		})
		return uuid.Nil
	}
	idStr, ok := raw.(string)
	if !ok || idStr == "" {
		c.AbortWithStatusJSON(http.StatusUnauthorized, map[string]any{
			"error":   "unauthorized",
			"message": "customer authentication required",
		})
		return uuid.Nil
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, map[string]any{
			"error":   "unauthorized",
			"message": "invalid customer session",
		})
		return uuid.Nil
	}
	return id
}

// resolveStore extracts the store set by StoreContext middleware.
func (h *WishlistHandler) resolveStore(c *gin.Context) *stores.Store {
	raw, exists := c.Get("store")
	if !exists {
		c.AbortWithStatusJSON(http.StatusInternalServerError, map[string]any{
			"error":   "internal",
			"message": "store context missing",
		})
		return nil
	}
	store, ok := raw.(*stores.Store)
	if !ok || store == nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, map[string]any{
			"error":   "internal",
			"message": "store context invalid",
		})
		return nil
	}
	return store
}

// queryInt reads an integer query parameter with a fallback default.
func queryInt(c *gin.Context, key string, fallback int) int {
	v := c.Query(key)
	if v == "" {
		return fallback
	}
	var n int
	if _, err := fmt.Sscanf(v, "%d", &n); err != nil || n < 1 {
		return fallback
	}
	return n
}
```

> **Note on `CustomerProfileIDKey`:** This constant is defined by C1 in `middleware.go`. It is the gin context key where `RequireCustomerAuth` stores the customer profile UUID string. If C1 named it differently, adjust accordingly. Expected value: `const CustomerProfileIDKey = "customer_profile_id"`.

> **Note on `queryInt`:** Check if this helper already exists in the storefront package from other handlers (e.g., checkout or loyalty). If it does, reuse the existing one and remove the duplicate from this file.

- [ ] **Step 2: Add missing import**

The `queryInt` function uses `fmt.Sscanf`. Ensure `"fmt"` is in the import block. The full import block should be:

```go
import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/stores"
	"github.com/mark8ly/marketplace-api/internal/wishlist"
)
```

- [ ] **Step 3: Verify compilation**

```bash
cd services/marketplace-api && go build ./internal/handlers/storefront/...
```

---

## Task 4: Wire routes + main.go

### Step 1: Add WishlistHandler to Deps struct

- [ ] Edit `services/marketplace-api/internal/handlers/storefront/routes.go`.

Add the field to the `Deps` struct:

```go
// Inside the Deps struct, after LoyaltyHandler:
WishlistHandler  *WishlistHandler
```

### Step 2: Register wishlist routes

- [ ] In `RegisterStorefront` function in `routes.go`, add the wishlist routes **inside the `group` block**, after the loyalty routes. Wishlist routes require customer auth (from C1 middleware):

```go
		// Wishlists — C4. Auth required.
		if deps.WishlistHandler != nil {
			wishlistGroup := group.Group("/wishlist", RequireCustomerAuth())
			{
				wishlistGroup.GET("", deps.WishlistHandler.List)
				wishlistGroup.POST("", deps.WishlistHandler.Add)
				wishlistGroup.DELETE("/:productId", deps.WishlistHandler.Remove)
				wishlistGroup.GET("/check", deps.WishlistHandler.Check)
			}
		}
```

> **Note:** `RequireCustomerAuth()` is the middleware from C1 that returns 401 if no customer session exists. It must be applied as group middleware so all four endpoints are protected.

> **Note on middleware chain:** The full chain is: `RequireStorefrontKey` -> `StoreContext` -> `RequireCustomerAuth` -> handler. The first two are already applied to the parent `group`. Only `RequireCustomerAuth` needs to be added here.

> **Note on OptionalCustomerAuth:** If C1 implemented `OptionalCustomerAuth` as a group-level middleware on the storefront group (to always populate customer context when a cookie is present), then `RequireCustomerAuth()` on the wishlist sub-group just asserts the value is present. Check how C1 wired it. If OptionalCustomerAuth is NOT on the parent group, you may need to add it: `group.Group("/wishlist", OptionalCustomerAuth(customerSvc, db, log), RequireCustomerAuth())`.

### Step 3: Wire in main.go

- [ ] Edit `services/marketplace-api/cmd/marketplace-api/main.go`.

Add the import:

```go
"github.com/mark8ly/marketplace-api/internal/wishlist"
```

In the storefront wiring block (the `if m == mode.Storefront || m == mode.Both {` block), after the loyalty wiring, add:

```go
		// Wishlists C4 wiring.
		wishlistRepo := wishlist.NewRepository()
		wishlistHandler := storefront.NewWishlistHandler(conn, wishlistRepo, log)
```

Then add the handler to the `storefrontDeps` struct initialization:

```go
		storefrontDeps = storefront.Deps{
			// ... existing fields ...
			WishlistHandler:       wishlistHandler,
		}
```

- [ ] **Step 4: Verify full build**

```bash
cd services/marketplace-api && go build ./...
```

---

## Task 5: Storefront UI — WishlistButton (heart icon, client component, optimistic toggle)

**File:** `apps/storefront/components/WishlistButton.tsx`

### Step 1: Create the wishlist API client

- [ ] Create `apps/storefront/lib/api/wishlist-api.ts`:

```typescript
/**
 * Wishlist API client for storefront.
 *
 * All endpoints require customer auth (session cookie). The server-side
 * API proxy at /api/wishlist/* forwards to marketplace-api with the
 * customer session. Client components call these functions directly
 * against the Next.js API routes.
 */

export interface WishlistItem {
  id: string;
  product_id: string;
  created_at: string;
  product_title: string;
  product_handle: string;
  product_image_url: string | null;
  product_min_price: string;
  product_max_price: string;
  currency_code: string;
  in_stock: boolean;
}

export interface WishlistListResponse {
  data: WishlistItem[];
  total: number;
  page: number;
  limit: number;
}

export interface WishlistCheckResponse {
  in_wishlist: boolean;
}

const MARKETPLACE_API_URL =
  process.env.MARKETPLACE_API_URL ?? "http://localhost:8088";

const STOREFRONT_KEY = process.env.MARKETPLACE_STOREFRONT_KEY ?? "";

function commonHeaders(): HeadersInit {
  const headers: Record<string, string> = {
    Accept: "application/json",
    "Content-Type": "application/json",
  };
  if (STOREFRONT_KEY) headers["X-Storefront-Key"] = STOREFRONT_KEY;
  return headers;
}

/**
 * Check if a product is in the customer's wishlist. Server-side only
 * (needs env vars). Returns false on any error.
 */
export async function checkWishlist(
  storeSlug: string,
  productId: string,
  cookieHeader: string,
): Promise<boolean> {
  const url = `${MARKETPLACE_API_URL}/api/v1/storefront/stores/${encodeURIComponent(
    storeSlug,
  )}/wishlist/check?product_id=${encodeURIComponent(productId)}`;
  try {
    const res = await fetch(url, {
      headers: { ...commonHeaders(), Cookie: cookieHeader },
      cache: "no-store",
    });
    if (!res.ok) return false;
    const body = (await res.json()) as WishlistCheckResponse;
    return body.in_wishlist;
  } catch {
    return false;
  }
}

/**
 * List the customer's wishlist. Server-side only.
 */
export async function listWishlist(
  storeSlug: string,
  cookieHeader: string,
  page = 1,
  limit = 50,
): Promise<WishlistListResponse | null> {
  const url = `${MARKETPLACE_API_URL}/api/v1/storefront/stores/${encodeURIComponent(
    storeSlug,
  )}/wishlist?page=${page}&limit=${limit}`;
  try {
    const res = await fetch(url, {
      headers: { ...commonHeaders(), Cookie: cookieHeader },
      cache: "no-store",
    });
    if (!res.ok) return null;
    return (await res.json()) as WishlistListResponse;
  } catch {
    return null;
  }
}

/**
 * Add a product to the wishlist. Called from client via API route proxy.
 */
export async function addToWishlist(
  storeSlug: string,
  productId: string,
): Promise<boolean> {
  try {
    const res = await fetch(`/api/wishlist/add`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ store_slug: storeSlug, product_id: productId }),
    });
    return res.ok || res.status === 201;
  } catch {
    return false;
  }
}

/**
 * Remove a product from the wishlist. Called from client via API route proxy.
 */
export async function removeFromWishlist(
  storeSlug: string,
  productId: string,
): Promise<boolean> {
  try {
    const res = await fetch(`/api/wishlist/remove`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ store_slug: storeSlug, product_id: productId }),
    });
    return res.ok;
  } catch {
    return false;
  }
}
```

> **Note on API route proxying:** Client components cannot call marketplace-api directly (no env vars, no session cookie forwarding). You need Next.js API routes at `apps/storefront/app/api/wishlist/add/route.ts` and `apps/storefront/app/api/wishlist/remove/route.ts` that forward the session cookie to marketplace-api. Follow the same pattern as any existing API route proxies in the app. If C1 established a different pattern (e.g., a single proxy utility), use that instead.

> **Alternative approach:** If C1 provides a `useCustomerFetch` hook or similar that handles auth + cookie forwarding from client components, use that instead of the `/api/wishlist/*` proxy pattern. Check what C1 shipped.

### Step 2: Create Next.js API route proxies

- [ ] Create `apps/storefront/app/api/wishlist/add/route.ts`:

```typescript
import { NextRequest, NextResponse } from "next/server";
import { cookies } from "next/headers";

const MARKETPLACE_API_URL =
  process.env.MARKETPLACE_API_URL ?? "http://localhost:8088";
const STOREFRONT_KEY = process.env.MARKETPLACE_STOREFRONT_KEY ?? "";

export async function POST(req: NextRequest) {
  const body = await req.json();
  const { store_slug, product_id } = body;

  if (!store_slug || !product_id) {
    return NextResponse.json(
      { error: "validation_failed", message: "store_slug and product_id required" },
      { status: 400 },
    );
  }

  const cookieStore = await cookies();
  const cookieHeader = cookieStore.toString();

  const url = `${MARKETPLACE_API_URL}/api/v1/storefront/stores/${encodeURIComponent(
    store_slug,
  )}/wishlist`;

  const res = await fetch(url, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Accept: "application/json",
      Cookie: cookieHeader,
      ...(STOREFRONT_KEY ? { "X-Storefront-Key": STOREFRONT_KEY } : {}),
    },
    body: JSON.stringify({ product_id }),
  });

  const data = await res.json().catch(() => ({}));
  return NextResponse.json(data, { status: res.status });
}
```

- [ ] Create `apps/storefront/app/api/wishlist/remove/route.ts`:

```typescript
import { NextRequest, NextResponse } from "next/server";
import { cookies } from "next/headers";

const MARKETPLACE_API_URL =
  process.env.MARKETPLACE_API_URL ?? "http://localhost:8088";
const STOREFRONT_KEY = process.env.MARKETPLACE_STOREFRONT_KEY ?? "";

export async function POST(req: NextRequest) {
  const body = await req.json();
  const { store_slug, product_id } = body;

  if (!store_slug || !product_id) {
    return NextResponse.json(
      { error: "validation_failed", message: "store_slug and product_id required" },
      { status: 400 },
    );
  }

  const cookieStore = await cookies();
  const cookieHeader = cookieStore.toString();

  const url = `${MARKETPLACE_API_URL}/api/v1/storefront/stores/${encodeURIComponent(
    store_slug,
  )}/wishlist/${encodeURIComponent(product_id)}`;

  const res = await fetch(url, {
    method: "DELETE",
    headers: {
      Accept: "application/json",
      Cookie: cookieHeader,
      ...(STOREFRONT_KEY ? { "X-Storefront-Key": STOREFRONT_KEY } : {}),
    },
  });

  const data = await res.json().catch(() => ({}));
  return NextResponse.json(data, { status: res.status });
}
```

### Step 3: Create WishlistButton component

- [ ] Create `apps/storefront/components/WishlistButton.tsx`:

```tsx
"use client";

// apps/storefront/components/WishlistButton.tsx
//
// Heart icon toggle for wishlisting products. Uses optimistic UI —
// fills the heart immediately on click, reverts on API failure.
// Requires customer auth — redirects to login if not signed in.

import { useCallback, useState, useTransition } from "react";
import { addToWishlist, removeFromWishlist } from "@/lib/api/wishlist-api";

interface WishlistButtonProps {
  productId: string;
  storeSlug: string;
  initialWishlisted: boolean;
  /** Whether the user is logged in. If false, click redirects to login. */
  isAuthenticated: boolean;
  /** Size variant for different contexts. */
  size?: "sm" | "md";
  className?: string;
}

export function WishlistButton({
  productId,
  storeSlug,
  initialWishlisted,
  isAuthenticated,
  size = "md",
  className = "",
}: WishlistButtonProps) {
  const [wishlisted, setWishlisted] = useState(initialWishlisted);
  const [isPending, startTransition] = useTransition();

  const handleToggle = useCallback(
    (e: React.MouseEvent<HTMLButtonElement>) => {
      // Prevent the click from propagating to parent Link elements
      // (important when the heart is inside a ProductCard link).
      e.preventDefault();
      e.stopPropagation();

      if (!isAuthenticated) {
        // Redirect to login. Adjust the URL to match C1's login route.
        window.location.href = `/account/login?redirect=${encodeURIComponent(window.location.pathname)}`;
        return;
      }

      // Optimistic update
      const previous = wishlisted;
      setWishlisted(!previous);

      startTransition(async () => {
        const success = previous
          ? await removeFromWishlist(storeSlug, productId)
          : await addToWishlist(storeSlug, productId);

        if (!success) {
          // Revert on failure
          setWishlisted(previous);
        }
      });
    },
    [wishlisted, isAuthenticated, storeSlug, productId],
  );

  const iconSize = size === "sm" ? 18 : 22;
  const buttonPadding = size === "sm" ? "p-1.5" : "p-2";

  return (
    <button
      type="button"
      onClick={handleToggle}
      disabled={isPending}
      aria-label={wishlisted ? "Remove from wishlist" : "Add to wishlist"}
      aria-pressed={wishlisted}
      className={`${buttonPadding} rounded-full transition-all duration-200 hover:scale-110 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:opacity-50 ${className}`}
    >
      <svg
        xmlns="http://www.w3.org/2000/svg"
        width={iconSize}
        height={iconSize}
        viewBox="0 0 24 24"
        fill={wishlisted ? "var(--moss-700, #2D4A2B)" : "none"}
        stroke={wishlisted ? "var(--moss-700, #2D4A2B)" : "var(--ink-900, #0E0E0C)"}
        strokeWidth={2}
        strokeLinecap="round"
        strokeLinejoin="round"
        aria-hidden="true"
        className={`transition-colors duration-200 ${
          !wishlisted ? "opacity-50 group-hover:opacity-80" : ""
        }`}
      >
        <path d="M20.84 4.61a5.5 5.5 0 0 0-7.78 0L12 5.67l-1.06-1.06a5.5 5.5 0 0 0-7.78 7.78l1.06 1.06L12 21.23l7.78-7.78 1.06-1.06a5.5 5.5 0 0 0 0-7.78z" />
      </svg>
    </button>
  );
}
```

> **Design note:** The heart icon uses moss-700 when filled (saved) per spec: "Filled moss-700 heart: saved." Outlined with ink-900 at 50% opacity when not saved.

---

## Task 6: Add heart to ProductCard + ProductDetails

### Step 1: Modify ProductDetails

- [ ] Edit `apps/storefront/components/ProductDetails.tsx`.

Add the import at the top of the file (after existing imports):

```typescript
import { WishlistButton } from "./WishlistButton";
```

Add props to receive wishlist state. Change the interface:

```typescript
interface ProductDetailsProps {
  product: StorefrontProduct;
  storeSlug: string;
  isWishlisted: boolean;
  isAuthenticated: boolean;
}
```

Update the function signature:

```typescript
export function ProductDetails({ product, storeSlug, isWishlisted, isAuthenticated }: ProductDetailsProps) {
```

Add the WishlistButton next to the product title in the header. Replace the existing `<header>` block:

```tsx
      <header className="flex flex-col gap-2">
        <div className="flex items-start justify-between gap-4">
          <h1 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-4xl leading-tight text-[color:var(--ink-900)]">
            {product.title}
          </h1>
          <WishlistButton
            productId={product.id}
            storeSlug={storeSlug}
            initialWishlisted={isWishlisted}
            isAuthenticated={isAuthenticated}
          />
        </div>
        <div className="flex items-baseline gap-3">
          <p
            className="text-2xl text-[color:var(--ink-900)]"
            style={{ fontFeatureSettings: '"tnum" 1, "lnum" 1' }}
          >
            {displayPrice}
          </p>
          {comparePrice && (
            <p
              className="text-lg text-[color:var(--ink-900)] opacity-40 line-through"
              style={{ fontFeatureSettings: '"tnum" 1, "lnum" 1' }}
            >
              {comparePrice}
            </p>
          )}
        </div>
      </header>
```

### Step 2: Update product detail page to pass new props

- [ ] Edit `apps/storefront/app/products/[handle]/page.tsx`.

Add imports:

```typescript
import { cookies } from "next/headers";
import { checkWishlist } from "@/lib/api/wishlist-api";
```

In the `StorefrontProductPage` function, after fetching the product and before the return, add wishlist check logic:

```typescript
  // Wishlist state — check if the logged-in customer has saved this product.
  // Returns false for unauthenticated users (no cookie → API returns 401 → false).
  const cookieStore = await cookies();
  const cookieHeader = cookieStore.toString();
  const isAuthenticated = Boolean(cookieStore.get("customer_session")); // Adjust cookie name to match C1
  const isWishlisted = isAuthenticated
    ? await checkWishlist(slug, product.id, cookieHeader)
    : false;
```

Update the `<ProductDetails>` component to pass the new props:

```tsx
          <ProductDetails
            product={product}
            storeSlug={slug}
            isWishlisted={isWishlisted}
            isAuthenticated={isAuthenticated}
          />
```

> **Note on cookie name:** C1 defines the session cookie name. Check what C1 used (likely `customer_session` or `__session`). Adjust the `cookieStore.get()` call accordingly.

### Step 3: Add WishlistButton to ProductCard on catalog page

- [ ] Edit `apps/storefront/app/products/page.tsx`.

The `ProductCard` function is defined inline in this file. Add the heart icon to the product card image area.

First, update the `ProductGrid` component to accept and pass auth state. Modify the page component to check auth:

Add imports at the top:

```typescript
import { cookies } from "next/headers";
import { WishlistButton } from "@/components/WishlistButton";
```

In the `StoreProductsPage` function, add auth detection:

```typescript
  const cookieStore = await cookies();
  const isAuthenticated = Boolean(cookieStore.get("customer_session")); // Match C1 cookie name
```

Update `ProductGrid` call:

```tsx
            <ProductGrid products={products} storeSlug={slug} isAuthenticated={isAuthenticated} />
```

Update the `ProductGrid` function:

```tsx
function ProductGrid({ products, storeSlug, isAuthenticated }: { products: StorefrontProduct[]; storeSlug: string; isAuthenticated: boolean }) {
  return (
    <ul className="grid grid-cols-1 gap-x-6 gap-y-12 sm:grid-cols-2 lg:grid-cols-3">
      {products.map((p) => (
        <ProductCard key={p.id} product={p} storeSlug={storeSlug} isAuthenticated={isAuthenticated} />
      ))}
    </ul>
  );
}
```

Update the `ProductCard` function to include the heart overlay on the image:

```tsx
function ProductCard({ product, storeSlug, isAuthenticated }: { product: StorefrontProduct; storeSlug: string; isAuthenticated: boolean }) {
  const cover = product.media[0];
  const min = formatPrice(product.price_range.min, product.price_range.currency_code);
  const max = formatPrice(product.price_range.max, product.price_range.currency_code);
  const isRange = product.price_range.min !== product.price_range.max;
  return (
    <li>
      <Link
        href={`/products/${encodeURIComponent(product.handle)}`}
        className="group block focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
      >
        <div className="relative aspect-square overflow-hidden rounded-md bg-[color:var(--paper-200)]">
          {cover ? (
            // eslint-disable-next-line @next/next/no-img-element
            <img
              src={cover.url}
              alt={cover.alt ?? product.title}
              className="h-full w-full object-cover transition-transform duration-500 group-hover:scale-[1.02]"
            />
          ) : (
            <div className="flex h-full w-full items-center justify-center text-xs uppercase tracking-widest text-[color:var(--ink-900)] opacity-30">
              No image
            </div>
          )}
          <div className="absolute right-2 top-2 z-10">
            <WishlistButton
              productId={product.id}
              storeSlug={storeSlug}
              initialWishlisted={false}
              isAuthenticated={isAuthenticated}
              size="sm"
            />
          </div>
        </div>
        <div className="mt-4 flex items-start justify-between gap-3">
          <h2 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-lg text-[color:var(--ink-900)] group-hover:underline">
            {product.title}
          </h2>
          <span
            className="text-sm text-[color:var(--ink-900)] opacity-80"
            style={{ fontFeatureSettings: '"tnum" 1, "lnum" 1' }}
          >
            {isRange ? `from ${min}` : min}
          </span>
        </div>
      </Link>
    </li>
  );
}
```

> **Note on `initialWishlisted={false}` for cards:** Checking wishlist state for every product in a grid would be N+1 queries. On the catalog page, all hearts start as outlined. When a user clicks one, it optimistically fills. The detail page checks the real state via the `/check` endpoint. This is acceptable UX for v1.

> **Alternative (batch check):** If you want accurate heart state on the grid, add a batch check endpoint `GET /wishlist/check?product_id=a,b,c` that returns a map. This is a v1.1 enhancement, not required now.

---

## Task 7: Storefront UI — /account/wishlist page

**File:** `apps/storefront/app/account/wishlist/page.tsx`

### Step 1: Create the WishlistProductCard component

- [ ] Create `apps/storefront/components/WishlistProductCard.tsx`:

```tsx
"use client";

// apps/storefront/components/WishlistProductCard.tsx
//
// Product card variant used on the wishlist page. Shows remove button
// and add-to-cart button instead of just being a link.

import { useCallback, useState, useTransition } from "react";
import Link from "next/link";
import { removeFromWishlist, type WishlistItem } from "@/lib/api/wishlist-api";
import { useCart } from "./CartProvider";

interface WishlistProductCardProps {
  item: WishlistItem;
  storeSlug: string;
  onRemoved: (productId: string) => void;
}

export function WishlistProductCard({
  item,
  storeSlug,
  onRemoved,
}: WishlistProductCardProps) {
  const [isRemoving, startRemoveTransition] = useTransition();
  const cart = useCart();

  const handleRemove = useCallback(
    (e: React.MouseEvent) => {
      e.preventDefault();
      startRemoveTransition(async () => {
        const success = await removeFromWishlist(storeSlug, item.product_id);
        if (success) {
          onRemoved(item.product_id);
        }
      });
    },
    [storeSlug, item.product_id, onRemoved],
  );

  const handleAddToCart = useCallback(
    (e: React.MouseEvent) => {
      e.preventDefault();
      cart.add({
        productId: item.product_id,
        variantId: item.product_id, // Default variant — detail page has full variant selection
        handle: item.product_handle,
        title: item.product_title,
        priceAmount: item.product_min_price,
        currencyCode: item.currency_code,
        imageUrl: item.product_image_url ?? undefined,
        quantity: 1,
      });
    },
    [cart, item],
  );

  const price = formatPrice(item.product_min_price, item.currency_code);
  const isRange = item.product_min_price !== item.product_max_price;

  return (
    <li className="group relative flex flex-col">
      <Link
        href={`/products/${encodeURIComponent(item.product_handle)}`}
        className="block focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
      >
        <div className="relative aspect-square overflow-hidden rounded-md bg-[color:var(--paper-200)]">
          {item.product_image_url ? (
            // eslint-disable-next-line @next/next/no-img-element
            <img
              src={item.product_image_url}
              alt={item.product_title}
              className="h-full w-full object-cover transition-transform duration-500 group-hover:scale-[1.02]"
            />
          ) : (
            <div className="flex h-full w-full items-center justify-center text-xs uppercase tracking-widest text-[color:var(--ink-900)] opacity-30">
              No image
            </div>
          )}
        </div>
        <div className="mt-4 flex items-start justify-between gap-3">
          <h2 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-lg text-[color:var(--ink-900)] group-hover:underline">
            {item.product_title}
          </h2>
          <span
            className="text-sm text-[color:var(--ink-900)] opacity-80"
            style={{ fontFeatureSettings: '"tnum" 1, "lnum" 1' }}
          >
            {isRange ? `from ${price}` : price}
          </span>
        </div>
      </Link>

      <div className="mt-3 flex gap-2">
        <button
          type="button"
          onClick={handleAddToCart}
          disabled={!item.in_stock}
          className="flex-1 rounded-md bg-[color:var(--ink-900)] px-4 py-2 text-sm font-medium text-[color:var(--paper-200)] transition-opacity hover:opacity-90 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:cursor-not-allowed disabled:opacity-40"
        >
          {item.in_stock ? "Add to cart" : "Out of stock"}
        </button>
        <button
          type="button"
          onClick={handleRemove}
          disabled={isRemoving}
          aria-label={`Remove ${item.product_title} from wishlist`}
          className="rounded-md border border-[color:var(--ink-900)]/15 px-3 py-2 text-sm text-[color:var(--ink-900)] opacity-60 transition-opacity hover:opacity-100 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:opacity-30"
        >
          {isRemoving ? "Removing..." : "Remove"}
        </button>
      </div>
    </li>
  );
}

function formatPrice(amount: string, currencyCode: string): string {
  const n = Number.parseFloat(amount);
  if (!Number.isFinite(n)) return `${currencyCode} ${amount}`;
  try {
    return new Intl.NumberFormat(undefined, {
      style: "currency",
      currency: currencyCode,
    }).format(n);
  } catch {
    return `${currencyCode} ${amount}`;
  }
}
```

> **Note on `useCart`:** This imports from `./CartProvider` which was added in the cart flow (P0 commit 91a297c). If the import path differs, adjust. The `cart.add()` call shape must match the CartProvider's `add` method signature. Check `apps/storefront/components/CartProvider.tsx` for the exact interface.

### Step 2: Create the wishlist page

- [ ] Create `apps/storefront/app/account/wishlist/page.tsx`:

```tsx
// apps/storefront/app/account/wishlist/page.tsx
//
// Customer's saved products page. Fetches the wishlist from
// marketplace-api using the session cookie.

import { cookies, headers } from "next/headers";
import { redirect } from "next/navigation";
import { listWishlist } from "@/lib/api/wishlist-api";
import { slugFromHost } from "@/lib/slug";
import { WishlistGrid } from "./WishlistGrid";

export const dynamic = "force-dynamic";

export const metadata = {
  title: "Wishlist",
};

interface PageProps {
  searchParams: Promise<{ slug?: string; page?: string }>;
}

async function resolveSlug(query: { slug?: string }): Promise<string> {
  const h = await headers();
  const host = h.get("host");
  return (
    query.slug ||
    slugFromHost(host) ||
    process.env.DEFAULT_STORE_SLUG ||
    ""
  );
}

export default async function WishlistPage({ searchParams }: PageProps) {
  const params = await searchParams;
  const slug = await resolveSlug(params);
  const page = params.page ? Number.parseInt(params.page, 10) || 1 : 1;

  const cookieStore = await cookies();
  const cookieHeader = cookieStore.toString();
  const isAuthenticated = Boolean(cookieStore.get("customer_session")); // Match C1 cookie name

  if (!isAuthenticated) {
    redirect(`/account/login?redirect=/account/wishlist`);
  }

  const wishlist = await listWishlist(slug, cookieHeader, page);

  return (
    <div className="space-y-8">
      <header>
        <h1 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-3xl text-[color:var(--ink-900)]">
          Wishlist
        </h1>
        {wishlist && wishlist.total > 0 && (
          <p className="mt-2 text-sm text-[color:var(--ink-900)] opacity-60">
            {wishlist.total} {wishlist.total === 1 ? "item" : "items"} saved
          </p>
        )}
      </header>

      {!wishlist || wishlist.data.length === 0 ? (
        <EmptyWishlist />
      ) : (
        <WishlistGrid items={wishlist.data} storeSlug={slug} />
      )}
    </div>
  );
}

function EmptyWishlist() {
  return (
    <div className="py-16 text-center">
      <p className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-2xl text-[color:var(--ink-900)]">
        Your wishlist is empty
      </p>
      <p className="mt-3 text-sm text-[color:var(--ink-900)] opacity-60">
        Browse products to save your favorites.
      </p>
      <a
        href="/products"
        className="mt-6 inline-block rounded-md bg-[color:var(--ink-900)] px-6 py-2.5 text-sm font-medium text-[color:var(--paper-200)] transition-opacity hover:opacity-90 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
      >
        Browse products
      </a>
    </div>
  );
}
```

### Step 3: Create WishlistGrid client component

- [ ] Create `apps/storefront/app/account/wishlist/WishlistGrid.tsx`:

```tsx
"use client";

// Client component wrapper for the wishlist grid. Manages local
// state so removed items disappear immediately without a full reload.

import { useCallback, useState } from "react";
import { WishlistProductCard } from "@/components/WishlistProductCard";
import type { WishlistItem } from "@/lib/api/wishlist-api";

interface WishlistGridProps {
  items: WishlistItem[];
  storeSlug: string;
}

export function WishlistGrid({ items: initialItems, storeSlug }: WishlistGridProps) {
  const [items, setItems] = useState(initialItems);

  const handleRemoved = useCallback((productId: string) => {
    setItems((prev) => prev.filter((item) => item.product_id !== productId));
  }, []);

  if (items.length === 0) {
    return (
      <div className="py-16 text-center">
        <p className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-2xl text-[color:var(--ink-900)]">
          Your wishlist is empty
        </p>
        <p className="mt-3 text-sm text-[color:var(--ink-900)] opacity-60">
          Browse products to save your favorites.
        </p>
        <a
          href="/products"
          className="mt-6 inline-block rounded-md bg-[color:var(--ink-900)] px-6 py-2.5 text-sm font-medium text-[color:var(--paper-200)] transition-opacity hover:opacity-90 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
        >
          Browse products
        </a>
      </div>
    );
  }

  return (
    <ul className="grid grid-cols-1 gap-x-6 gap-y-12 sm:grid-cols-2 lg:grid-cols-3">
      {items.map((item) => (
        <WishlistProductCard
          key={item.id}
          item={item}
          storeSlug={storeSlug}
          onRemoved={handleRemoved}
        />
      ))}
    </ul>
  );
}
```

### Step 4: Add "Wishlist" tab to account navigation

- [ ] Edit the account sidebar/navigation component. If C1 created `apps/storefront/components/AccountSidebar.tsx` (per the C1 plan), add a "Wishlist" link between "Addresses" and "Loyalty":

```tsx
// In the nav links array, add between Addresses and Loyalty:
{ href: "/account/wishlist", label: "Wishlist" },
```

The exact file and structure depends on what C1 shipped. Look for the component that renders the `/account` layout sidebar/tabs and add the link there.

---

## Task 8: Build verification

- [ ] **Step 1: Go build**

```bash
cd services/marketplace-api && go build ./...
```

Verify zero errors. Fix any import issues.

- [ ] **Step 2: Go vet**

```bash
cd services/marketplace-api && go vet ./...
```

- [ ] **Step 3: TypeScript build**

```bash
cd apps/storefront && npx next build
```

> If build fails due to missing C1 dependencies (CustomerProfileIDKey, RequireCustomerAuth, customer_session cookie, AccountSidebar, etc.), that means C1 has not been merged yet. In that case, create stubs:
>
> - For Go: Add `const CustomerProfileIDKey = "customer_profile_id"` to the storefront package if not present.
> - For Next.js: Stub the auth check (hardcode `isAuthenticated = false`) and mark with `// TODO(C1):` comments.

- [ ] **Step 4: Manual smoke test checklist**

If running locally with `make dev`:

1. Visit a product detail page — heart icon appears next to title, outlined
2. Click heart when not logged in — redirects to login page
3. After login, click heart — fills with moss-700 immediately (optimistic)
4. Refresh page — heart remains filled (persisted)
5. Click filled heart — unfills immediately (optimistic remove)
6. Visit /account/wishlist — shows saved products in grid
7. Click "Remove" on wishlist page — item disappears from grid
8. Click "Add to cart" on wishlist page — item added to cart
9. Remove all items — empty state shows with "Browse products" link
10. Visit /products catalog — hearts visible on product cards

---

## Testing requirements (from spec section 12)

The spec requires these test cases for C4:

- Wishlist add/remove idempotency
- UNIQUE constraint prevents duplicates
- Auth required (401 without session)
- Heart icon toggle
- Wishlist page renders saved products
- "Add to cart" from wishlist
- Remove from wishlist
- Empty state

These should be covered by:
- **Go unit tests:** `internal/wishlist/repository_test.go` — add/remove idempotency, UNIQUE constraint, list with joins
- **Go handler tests:** `internal/handlers/storefront/wishlist_test.go` — 401 without auth, 201 on add, 200 on remove, check endpoint
- **Playwright E2E:** Heart toggle, wishlist page grid, add-to-cart from wishlist, empty state

> Tests are not part of this plan's task list (per the plan scope), but should be written as a follow-up or as part of execution.
