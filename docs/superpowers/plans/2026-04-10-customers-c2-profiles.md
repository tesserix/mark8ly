# Customers C2 — Customer Profiles Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship admin customer list (aggregated stats via joins), customer detail (tabbed: overview, orders, addresses, notes), block/unblock, tag/note editing. No denormalized counters — stats computed at query time using the orders index from C1.

**Architecture:** Admin handlers in `internal/handlers/admin/customers.go`. Extends existing customer repository from C1. Uses correlated subqueries with `orders_store_email_idx` for stats. Skeleton loading in admin UI.

**Tech Stack:** Go 1.26, Gin, GORM. Next.js 16, React 19, Tailwind.

**Prerequisite:** C1 must be on main (provides customer_profiles table + repository).

---

## Task 1 — Extend Customer Repository with Admin Query Methods

> **Files:**
> - `services/marketplace-api/internal/customer/repository.go` (extend)
> - `services/marketplace-api/internal/customer/models.go` (extend)

### 1.1 Add admin-specific models

- [ ] Add `CustomerWithStats` struct that embeds `CustomerProfile` plus computed fields:

```go
// CustomerWithStats extends CustomerProfile with computed aggregation
// fields populated via correlated subqueries against the orders table.
// These fields are NEVER stored — they exist only in query results.
type CustomerWithStats struct {
	CustomerProfile
	OrderCount  int64           `gorm:"column:order_count"  json:"order_count"`
	TotalSpent  decimal.Decimal `gorm:"column:total_spent"  json:"total_spent"`
	LastOrderAt *time.Time      `gorm:"column:last_order_at" json:"last_order_at"`
}
```

- [ ] Add `ListCustomersQuery` for pagination, search, and filtering:

```go
// ListCustomersQuery holds the parsed query params for the admin
// customer list endpoint.
type ListCustomersQuery struct {
	Search   string // free-text against email, first_name, last_name
	Status   string // "active" | "blocked" | "" (all)
	Tag      string // filter by tag containment
	SortBy   string // "created_at" | "order_count" | "total_spent" | "last_order_at"
	SortDir  string // "asc" | "desc"
	Page     int
	PageSize int
}

func (q *ListCustomersQuery) Defaults() {
	if q.Page < 1 {
		q.Page = 1
	}
	if q.PageSize < 1 || q.PageSize > 200 {
		q.PageSize = 50
	}
	if q.SortBy == "" {
		q.SortBy = "created_at"
	}
	if q.SortDir == "" {
		q.SortDir = "desc"
	}
}
```

### 1.2 Add admin repository interface methods

- [ ] Extend the `Repository` interface (or create `AdminRepository` if C1's interface is storefront-only):

```go
// AdminRepository groups read methods that the admin handler layer
// needs. All queries are scoped to (store_id, tenant_id).
type AdminRepository interface {
	// ListForStore returns paginated customers with computed order stats.
	ListForStore(ctx context.Context, storeID, tenantID string, q ListCustomersQuery) ([]CustomerWithStats, int64, error)

	// GetByID returns a single customer profile (no stats — detail page
	// loads stats separately or inline).
	GetByID(ctx context.Context, storeID, tenantID, customerID string) (*CustomerProfile, error)

	// UpdateTags replaces the tags array on a customer profile.
	UpdateTags(ctx context.Context, storeID, tenantID, customerID string, tags []string) (*CustomerProfile, error)

	// UpdateNotes replaces the admin notes text.
	UpdateNotes(ctx context.Context, storeID, tenantID, customerID string, notes string) (*CustomerProfile, error)

	// SetStatus sets status and optionally block_reason.
	SetStatus(ctx context.Context, storeID, tenantID, customerID, status, reason string) (*CustomerProfile, error)

	// ListAddresses returns all addresses for a customer.
	ListAddresses(ctx context.Context, customerID string) ([]CustomerAddress, error)
}
```

### 1.3 Implement ListForStore with correlated subqueries

- [ ] The core query uses correlated subqueries hitting `orders_store_email_idx`:

```go
func (r *repository) ListForStore(ctx context.Context, storeID, tenantID string, q ListCustomersQuery) ([]CustomerWithStats, int64, error) {
	q.Defaults()

	base := r.db.WithContext(ctx).
		Table("customer_profiles AS cp").
		Where("cp.store_id = ? AND cp.tenant_id = ?", storeID, tenantID)

	// Search filter — ILIKE on email, first_name, last_name.
	if q.Search != "" {
		like := "%" + q.Search + "%"
		base = base.Where(
			"(cp.email ILIKE ? OR cp.first_name ILIKE ? OR cp.last_name ILIKE ?)",
			like, like, like,
		)
	}
	if q.Status != "" {
		base = base.Where("cp.status = ?", q.Status)
	}
	if q.Tag != "" {
		base = base.Where("? = ANY(cp.tags)", q.Tag)
	}

	// Count before pagination.
	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("customer list count: %w", err)
	}

	// Stats subqueries — uses orders_store_email_idx (store_id, customer_email).
	statsSelect := `cp.*,
		(SELECT COUNT(*) FROM orders WHERE customer_email = cp.email AND store_id = cp.store_id AND deleted_at IS NULL) AS order_count,
		(SELECT COALESCE(SUM(grand_total), 0) FROM orders WHERE customer_email = cp.email AND store_id = cp.store_id AND deleted_at IS NULL) AS total_spent,
		(SELECT MAX(placed_at) FROM orders WHERE customer_email = cp.email AND store_id = cp.store_id AND deleted_at IS NULL) AS last_order_at`

	// Sort — allow sorting by computed columns.
	orderClause := sanitizeSortColumn(q.SortBy) + " " + sanitizeSortDir(q.SortDir)

	var rows []CustomerWithStats
	if err := base.
		Select(statsSelect).
		Order(orderClause).
		Limit(q.PageSize).
		Offset((q.Page - 1) * q.PageSize).
		Find(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("customer list query: %w", err)
	}
	return rows, total, nil
}

// sanitizeSortColumn returns a safe column reference for ORDER BY.
func sanitizeSortColumn(col string) string {
	switch col {
	case "order_count", "total_spent", "last_order_at":
		return col
	case "email":
		return "cp.email"
	case "name":
		return "cp.first_name"
	default:
		return "cp.created_at"
	}
}

func sanitizeSortDir(dir string) string {
	if strings.EqualFold(dir, "asc") {
		return "ASC"
	}
	return "DESC"
}
```

### 1.4 Implement remaining admin repository methods

- [ ] `GetByID` — straightforward `WHERE id = ? AND store_id = ? AND tenant_id = ?` returning `*CustomerProfile` or `apperrors.NotFound`.
- [ ] `UpdateTags` — `UPDATE customer_profiles SET tags = ?, updated_at = now() WHERE id = ? AND store_id = ? AND tenant_id = ?`.
- [ ] `UpdateNotes` — `UPDATE customer_profiles SET notes = ?, updated_at = now() WHERE ...`.
- [ ] `SetStatus` — `UPDATE customer_profiles SET status = ?, block_reason = ?, updated_at = now() WHERE ...`. Validates status is "active" or "blocked".
- [ ] `ListAddresses` — `SELECT * FROM customer_addresses WHERE customer_id = ? ORDER BY is_default DESC, created_at ASC`.

### 1.5 Unit test the repository

- [ ] Table-driven tests for `ListForStore`: search match, status filter, tag filter, pagination boundary, sort by order_count.
- [ ] Test `SetStatus` with invalid status value returns validation error.
- [ ] Test `GetByID` returns `apperrors.NotFound` for missing customer.

---

## Task 2 — Admin Customer Handler

> **Files:**
> - `services/marketplace-api/internal/handlers/admin/customers.go` (new)
> - `services/marketplace-api/internal/handlers/admin/customers_dto.go` (new)

### 2.1 Create DTO file

- [ ] Follow the `orders_dto.go` pattern. Define wire types:

```go
package admin

import (
	"time"
	"github.com/shopspring/decimal"
)

// ─────────────────────────────────────────────────────────────────────
// Customers — list query
// ─────────────────────────────────────────────────────────────────────

// ListCustomersQuery is the parsed query string for
// GET /admin/stores/:storeId/customers.
type ListCustomersQuery struct {
	Search   string `form:"search"`
	Status   string `form:"status"`
	Tag      string `form:"tag"`
	SortBy   string `form:"sort_by"`
	SortDir  string `form:"sort_dir"`
	Page     int    `form:"page"`
	PageSize int    `form:"page_size"`
}

func (q *ListCustomersQuery) Defaults() {
	if q.Page < 1 {
		q.Page = 1
	}
	if q.PageSize < 1 || q.PageSize > 200 {
		q.PageSize = 50
	}
}

// ─────────────────────────────────────────────────────────────────────
// Customers — response types
// ─────────────────────────────────────────────────────────────────────

// AdminCustomerListItem is a single row in the customer list response.
type AdminCustomerListItem struct {
	ID          string          `json:"id"`
	Email       string          `json:"email"`
	FirstName   *string         `json:"first_name"`
	LastName    *string         `json:"last_name"`
	Phone       *string         `json:"phone"`
	AvatarURL   *string         `json:"avatar_url"`
	Tags        []string        `json:"tags"`
	Status      string          `json:"status"`
	MarketingOptIn bool         `json:"marketing_opt_in"`
	OrderCount  int64           `json:"order_count"`
	TotalSpent  decimal.Decimal `json:"total_spent"`
	LastOrderAt *time.Time      `json:"last_order_at"`
	CreatedAt   time.Time       `json:"created_at"`
}

// AdminCustomerDetail is the full profile returned by GET /customers/:id.
type AdminCustomerDetail struct {
	ID             string          `json:"id"`
	Email          string          `json:"email"`
	FirstName      *string         `json:"first_name"`
	LastName       *string         `json:"last_name"`
	Phone          *string         `json:"phone"`
	AvatarURL      *string         `json:"avatar_url"`
	Tags           []string        `json:"tags"`
	Status         string          `json:"status"`
	BlockReason    *string         `json:"block_reason"`
	Notes          *string         `json:"notes"`
	MarketingOptIn bool            `json:"marketing_opt_in"`
	OrderCount     int64           `json:"order_count"`
	TotalSpent     decimal.Decimal `json:"total_spent"`
	LastOrderAt    *time.Time      `json:"last_order_at"`
	Addresses      []AdminCustomerAddress `json:"addresses"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

// AdminCustomerAddress is an address row in the customer detail.
type AdminCustomerAddress struct {
	ID          string    `json:"id"`
	Label       *string   `json:"label"`
	IsDefault   bool      `json:"is_default"`
	Name        string    `json:"name"`
	Line1       string    `json:"line1"`
	Line2       *string   `json:"line2"`
	City        string    `json:"city"`
	Region      *string   `json:"region"`
	PostalCode  *string   `json:"postal_code"`
	CountryCode string    `json:"country_code"`
	Phone       *string   `json:"phone"`
	CreatedAt   time.Time `json:"created_at"`
}

// ─────────────────────────────────────────────────────────────────────
// Customers — request bodies
// ─────────────────────────────────────────────────────────────────────

// PatchCustomerRequest is the body for PATCH /customers/:id.
type PatchCustomerRequest struct {
	Tags           *[]string `json:"tags"`
	Notes          *string   `json:"notes"`
	MarketingOptIn *bool     `json:"marketing_opt_in"`
}

// BlockCustomerRequest is the body for POST /customers/:id/block.
type BlockCustomerRequest struct {
	Reason string `json:"reason" binding:"required,max=300"`
}
```

### 2.2 Create handler file

- [ ] Follow the `CategoryHandler` / `OrdersHandler` pattern:

```go
package admin

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/customer"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// CustomerHandler bundles dependencies for the admin customer endpoints.
type CustomerHandler struct {
	db     *gorm.DB
	repo   customer.AdminRepository
	logger *slog.Logger
}

// NewCustomerHandler constructs a CustomerHandler.
func NewCustomerHandler(db *gorm.DB, repo customer.AdminRepository, logger *slog.Logger) *CustomerHandler {
	return &CustomerHandler{db: db, repo: repo, logger: logger}
}
```

### 2.3 Implement List handler

- [ ] `GET /admin/stores/:storeId/customers`:

```go
func (h *CustomerHandler) List(c *gin.Context) {
	storeID := c.Param("storeId")
	tenantID := c.GetString("tenant_id")

	var q ListCustomersQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		RespondErr(c, apperrors.ValidationFailed("query", err.Error()), h.logger)
		return
	}
	q.Defaults()

	repoQ := customer.ListCustomersQuery{
		Search:   q.Search,
		Status:   q.Status,
		Tag:      q.Tag,
		SortBy:   q.SortBy,
		SortDir:  q.SortDir,
		Page:     q.Page,
		PageSize: q.PageSize,
	}

	rows, total, err := h.repo.ListForStore(c.Request.Context(), storeID, tenantID, repoQ)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	out := make([]AdminCustomerListItem, 0, len(rows))
	for i := range rows {
		out = append(out, toAdminCustomerListItem(&rows[i]))
	}

	totalPages := int((total + int64(q.PageSize) - 1) / int64(q.PageSize))
	c.JSON(http.StatusOK, gin.H{
		"data": out,
		"meta": gin.H{
			"page":        q.Page,
			"page_size":   q.PageSize,
			"total":       total,
			"total_pages": totalPages,
		},
	})
}
```

### 2.4 Implement Get handler

- [ ] `GET /admin/stores/:storeId/customers/:id` — loads profile, runs stats subquery inline, loads addresses:

```go
func (h *CustomerHandler) Get(c *gin.Context) {
	storeID := c.Param("storeId")
	tenantID := c.GetString("tenant_id")
	customerID := c.Param("id")

	prof, err := h.repo.GetByID(c.Request.Context(), storeID, tenantID, customerID)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	// Compute stats inline — single row, no pagination overhead.
	var stats struct {
		OrderCount  int64           `gorm:"column:order_count"`
		TotalSpent  decimal.Decimal `gorm:"column:total_spent"`
		LastOrderAt *time.Time      `gorm:"column:last_order_at"`
	}
	h.db.WithContext(c.Request.Context()).Raw(`
		SELECT
			COUNT(*) AS order_count,
			COALESCE(SUM(grand_total), 0) AS total_spent,
			MAX(placed_at) AS last_order_at
		FROM orders
		WHERE customer_email = ? AND store_id = ? AND deleted_at IS NULL
	`, prof.Email, storeID).Scan(&stats)

	addrs, err := h.repo.ListAddresses(c.Request.Context(), customerID)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	detail := toAdminCustomerDetail(prof, stats.OrderCount, stats.TotalSpent, stats.LastOrderAt, addrs)
	c.JSON(http.StatusOK, gin.H{"data": detail})
}
```

### 2.5 Implement Patch handler

- [ ] `PATCH /admin/stores/:storeId/customers/:id` — updates tags, notes, marketing_opt_in:

```go
func (h *CustomerHandler) Patch(c *gin.Context) {
	storeID := c.Param("storeId")
	tenantID := c.GetString("tenant_id")
	customerID := c.Param("id")

	var req PatchCustomerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}

	// Verify customer exists and belongs to store.
	prof, err := h.repo.GetByID(c.Request.Context(), storeID, tenantID, customerID)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}

	if req.Tags != nil {
		prof, err = h.repo.UpdateTags(c.Request.Context(), storeID, tenantID, customerID, *req.Tags)
		if err != nil {
			RespondErr(c, err, h.logger)
			return
		}
	}
	if req.Notes != nil {
		prof, err = h.repo.UpdateNotes(c.Request.Context(), storeID, tenantID, customerID, *req.Notes)
		if err != nil {
			RespondErr(c, err, h.logger)
			return
		}
	}
	if req.MarketingOptIn != nil {
		// Inline update — small enough to not warrant a separate repo method.
		if err := h.db.WithContext(c.Request.Context()).
			Model(&customer.CustomerProfile{}).
			Where("id = ? AND store_id = ? AND tenant_id = ?", customerID, storeID, tenantID).
			Update("marketing_opt_in", *req.MarketingOptIn).Error; err != nil {
			RespondErr(c, err, h.logger)
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"data": gin.H{"id": prof.ID, "updated": true}})
}
```

### 2.6 Implement Block / Unblock handlers

- [ ] `POST /admin/stores/:storeId/customers/:id/block`:

```go
func (h *CustomerHandler) Block(c *gin.Context) {
	storeID := c.Param("storeId")
	tenantID := c.GetString("tenant_id")
	customerID := c.Param("id")

	var req BlockCustomerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondErr(c, apperrors.ValidationFailed("body", err.Error()), h.logger)
		return
	}

	prof, err := h.repo.SetStatus(c.Request.Context(), storeID, tenantID, customerID, "blocked", req.Reason)
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"id": prof.ID, "status": prof.Status}})
}
```

- [ ] `POST /admin/stores/:storeId/customers/:id/unblock`:

```go
func (h *CustomerHandler) Unblock(c *gin.Context) {
	storeID := c.Param("storeId")
	tenantID := c.GetString("tenant_id")
	customerID := c.Param("id")

	prof, err := h.repo.SetStatus(c.Request.Context(), storeID, tenantID, customerID, "active", "")
	if err != nil {
		RespondErr(c, err, h.logger)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"id": prof.ID, "status": prof.Status}})
}
```

### 2.7 Implement DTO mappers

- [ ] Add `toAdminCustomerListItem` and `toAdminCustomerDetail` mapper functions at the bottom of `customers.go` (or in `customers_dto.go`).

---

## Task 3 — Wire Admin Routes + Deps + main.go

> **Files:**
> - `services/marketplace-api/internal/authz/customers_roles.go` (new)
> - `services/marketplace-api/internal/handlers/admin/routes.go` (extend)
> - `services/marketplace-api/cmd/marketplace-api/main.go` (extend)

### 3.1 Create authz role constants

- [ ] Follow `orders_roles.go` pattern:

```go
// File: services/marketplace-api/internal/authz/customers_roles.go
package authz

// CustomersViewRole gates GET /admin/customers and GET /admin/customers/:id.
// Staff can view customer profiles.
var CustomersViewRole = RoleStaff

// CustomersEditRole gates PATCH /admin/customers/:id (tags, notes).
// Admin can edit customer metadata.
var CustomersEditRole = RoleAdmin

// CustomersBlockRole gates POST /admin/customers/:id/block and /unblock.
// Admin can block/unblock customers.
var CustomersBlockRole = RoleAdmin
```

### 3.2 Add CustomerHandler to Deps

- [ ] In `routes.go`, add the field to `Deps`:

```go
// Add to Deps struct:
CustomerHandler *CustomerHandler
```

### 3.3 Register customer routes

- [ ] In `RegisterAdmin`, add customer routes inside the `storeRoute` block, following the orders/returns pattern:

```go
// Customers.
if deps.CustomerHandler != nil {
	customers := storeRoute.Group("/customers")
	{
		customers.GET("",
			deps.AuthzMiddleware.RequireTenantRelation(authz.CustomersViewRole),
			deps.CustomerHandler.List)
		customers.GET("/:id",
			deps.AuthzMiddleware.RequireTenantRelation(authz.CustomersViewRole),
			deps.CustomerHandler.Get)
		customers.PATCH("/:id",
			deps.AuthzMiddleware.RequireTenantRelation(authz.CustomersEditRole),
			deps.CustomerHandler.Patch)
		customers.POST("/:id/block",
			deps.AuthzMiddleware.RequireTenantRelation(authz.CustomersBlockRole),
			deps.CustomerHandler.Block)
		customers.POST("/:id/unblock",
			deps.AuthzMiddleware.RequireTenantRelation(authz.CustomersBlockRole),
			deps.CustomerHandler.Unblock)
	}
}
```

### 3.4 Wire in main.go

- [ ] Add customer repository + handler construction in the admin wiring block, after the existing handlers:

```go
// Customer profiles (C2).
customerRepo := customer.NewAdminRepository(conn)
customerHandler := admin.NewCustomerHandler(conn, customerRepo, log)
```

- [ ] Add to the `adminDeps` struct literal:

```go
CustomerHandler: customerHandler,
```

- [ ] Add the `customer` import:

```go
"github.com/mark8ly/marketplace-api/internal/customer"
```

---

## Task 4 — Admin UI: API Client for Customer Endpoints

> **Files:**
> - `apps/admin/lib/api/customers-api.ts` (new)

### 4.1 Create the API client

- [ ] Follow the `marketplace-api.ts` pattern (server-component-only, session headers, null on 401/403/404):

```typescript
// apps/admin/lib/api/customers-api.ts
//
// Admin customer API client. Server-component only.

const MARKETPLACE_API_URL =
  process.env.MARKETPLACE_API_URL ?? "http://localhost:8088";

import type { SessionHeaders, ApiError } from "./marketplace-api";

// ─────────────────────────────────────────────────────────────────────
// Types
// ─────────────────────────────────────────────────────────────────────

export interface AdminCustomerListItem {
  id: string;
  email: string;
  first_name: string | null;
  last_name: string | null;
  phone: string | null;
  avatar_url: string | null;
  tags: string[];
  status: "active" | "blocked";
  marketing_opt_in: boolean;
  order_count: number;
  total_spent: string; // decimal as string
  last_order_at: string | null;
  created_at: string;
}

export interface AdminCustomerDetail {
  id: string;
  email: string;
  first_name: string | null;
  last_name: string | null;
  phone: string | null;
  avatar_url: string | null;
  tags: string[];
  status: "active" | "blocked";
  block_reason: string | null;
  notes: string | null;
  marketing_opt_in: boolean;
  order_count: number;
  total_spent: string;
  last_order_at: string | null;
  addresses: AdminCustomerAddress[];
  created_at: string;
  updated_at: string;
}

export interface AdminCustomerAddress {
  id: string;
  label: string | null;
  is_default: boolean;
  name: string;
  line1: string;
  line2: string | null;
  city: string;
  region: string | null;
  postal_code: string | null;
  country_code: string;
  phone: string | null;
  created_at: string;
}

export interface ListCustomersQuery {
  search?: string;
  status?: "active" | "blocked";
  tag?: string;
  sortBy?: string;
  sortDir?: "asc" | "desc";
  page?: number;
  pageSize?: number;
}

export interface ListCustomersResponse {
  data: AdminCustomerListItem[];
  meta: {
    page: number;
    page_size: number;
    total: number;
    total_pages: number;
  };
}

// ─────────────────────────────────────────────────────────────────────
// API functions
// ─────────────────────────────────────────────────────────────────────

function headers(session: SessionHeaders): HeadersInit {
  return {
    "X-User-Id": session.userId,
    "X-Tenant-Id": session.tenantId,
    Accept: "application/json",
  };
}

export async function listCustomers(
  storeId: string,
  query: ListCustomersQuery,
  session: SessionHeaders,
): Promise<ListCustomersResponse | null> {
  const params = new URLSearchParams();
  if (query.search) params.set("search", query.search);
  if (query.status) params.set("status", query.status);
  if (query.tag) params.set("tag", query.tag);
  if (query.sortBy) params.set("sort_by", query.sortBy);
  if (query.sortDir) params.set("sort_dir", query.sortDir);
  if (query.page) params.set("page", String(query.page));
  if (query.pageSize) params.set("page_size", String(query.pageSize));
  const qs = params.toString();

  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/customers${qs ? `?${qs}` : ""}`;
  const res = await fetch(url, { cache: "no-store", headers: headers(session) });

  if (res.status === 401 || res.status === 403 || res.status === 404) return null;
  if (!res.ok) {
    const errBody = (await res.json().catch(() => null)) as ApiError | null;
    throw new Error(`marketplace-api: listCustomers ${res.status}: ${errBody?.message ?? "unknown error"}`);
  }
  return (await res.json()) as ListCustomersResponse;
}

export async function getCustomer(
  storeId: string,
  customerId: string,
  session: SessionHeaders,
): Promise<AdminCustomerDetail | null> {
  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/customers/${customerId}`;
  const res = await fetch(url, { cache: "no-store", headers: headers(session) });

  if (res.status === 401 || res.status === 403 || res.status === 404) return null;
  if (!res.ok) {
    const errBody = (await res.json().catch(() => null)) as ApiError | null;
    throw new Error(`marketplace-api: getCustomer ${res.status}: ${errBody?.message ?? "unknown error"}`);
  }
  const body = await res.json();
  return body.data as AdminCustomerDetail;
}

export async function patchCustomer(
  storeId: string,
  customerId: string,
  body: { tags?: string[]; notes?: string; marketing_opt_in?: boolean },
  session: SessionHeaders,
): Promise<void> {
  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/customers/${customerId}`;
  const res = await fetch(url, {
    method: "PATCH",
    cache: "no-store",
    headers: { ...headers(session), "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    const errBody = (await res.json().catch(() => null)) as ApiError | null;
    throw new Error(`marketplace-api: patchCustomer ${res.status}: ${errBody?.message ?? "unknown error"}`);
  }
}

export async function blockCustomer(
  storeId: string,
  customerId: string,
  reason: string,
  session: SessionHeaders,
): Promise<void> {
  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/customers/${customerId}/block`;
  const res = await fetch(url, {
    method: "POST",
    cache: "no-store",
    headers: { ...headers(session), "Content-Type": "application/json" },
    body: JSON.stringify({ reason }),
  });
  if (!res.ok) {
    const errBody = (await res.json().catch(() => null)) as ApiError | null;
    throw new Error(`marketplace-api: blockCustomer ${res.status}: ${errBody?.message ?? "unknown error"}`);
  }
}

export async function unblockCustomer(
  storeId: string,
  customerId: string,
  session: SessionHeaders,
): Promise<void> {
  const url = `${MARKETPLACE_API_URL}/api/v1/admin/stores/${storeId}/customers/${customerId}/unblock`;
  const res = await fetch(url, {
    method: "POST",
    cache: "no-store",
    headers: { ...headers(session), "Content-Type": "application/json" },
    body: JSON.stringify({}),
  });
  if (!res.ok) {
    const errBody = (await res.json().catch(() => null)) as ApiError | null;
    throw new Error(`marketplace-api: unblockCustomer ${res.status}: ${errBody?.message ?? "unknown error"}`);
  }
}
```

---

## Task 5 — Admin UI: Customer List Page

> **Files:**
> - `apps/admin/app/customers/page.tsx` (new)
> - `apps/admin/components/customers/CustomersListHeader.tsx` (new)
> - `apps/admin/components/customers/CustomersListFilters.tsx` (new)
> - `apps/admin/components/customers/CustomersList.tsx` (new)
> - `apps/admin/components/customers/CustomersListPagination.tsx` (new)
> - `apps/admin/components/customers/CustomersListEmpty.tsx` (new)
> - `apps/admin/components/customers/CustomerStatusBadge.tsx` (new)

### 5.1 Create the page component

- [ ] Follow `apps/admin/app/products/page.tsx` pattern — server component, session fetch, parallel data loading:

```tsx
// apps/admin/app/customers/page.tsx
import { AdminShell } from "@/components/shell/AdminShell";
import { getServerSessionContext } from "@/lib/auth/serverSession";
import {
  listCustomers,
  type ListCustomersQuery,
} from "@/lib/api/customers-api";

import { CustomersListHeader } from "@/components/customers/CustomersListHeader";
import { CustomersListFilters } from "@/components/customers/CustomersListFilters";
import { CustomersList } from "@/components/customers/CustomersList";
import { CustomersListPagination } from "@/components/customers/CustomersListPagination";
import { CustomersListEmpty } from "@/components/customers/CustomersListEmpty";

interface CustomersPageProps {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}

export default async function CustomersPage({ searchParams }: CustomersPageProps) {
  const session = await getServerSessionContext();
  const { tenantName, email, currentStore, userId, tenantId } = session;
  const params = await searchParams;

  if (!currentStore) {
    return (
      <AdminShell tenantName={tenantName} userEmail={email}>
        <main className="flex flex-col gap-6 px-8 py-6">
          <CustomersListHeader />
          <CustomersListEmpty variant="no-store" />
        </main>
      </AdminShell>
    );
  }

  const query = parseSearchParams(params);
  const response = await listCustomers(currentStore.id, query, { userId, tenantId });
  const customers = response?.data ?? [];
  const meta = response?.meta ?? { page: 1, page_size: 50, total: 0, total_pages: 0 };

  return (
    <AdminShell tenantName={tenantName} userEmail={email}>
      <main className="flex flex-col gap-6 px-8 py-6">
        <CustomersListHeader />
        <CustomersListFilters
          search={query.search}
          status={query.status}
        />
        {customers.length === 0 ? (
          <CustomersListEmpty variant={query.search ? "no-results" : "no-customers"} />
        ) : (
          <>
            <CustomersList customers={customers} storeId={currentStore.id} />
            <CustomersListPagination meta={meta} />
          </>
        )}
      </main>
    </AdminShell>
  );
}

function parseSearchParams(
  params: Record<string, string | string[] | undefined>,
): ListCustomersQuery {
  const str = (key: string) => {
    const v = params[key];
    return typeof v === "string" ? v : undefined;
  };
  const num = (key: string) => {
    const v = str(key);
    return v ? Number(v) : undefined;
  };
  return {
    search: str("search"),
    status: str("status") as "active" | "blocked" | undefined,
    tag: str("tag"),
    sortBy: str("sort_by"),
    sortDir: str("sort_dir") as "asc" | "desc" | undefined,
    page: num("page"),
    pageSize: num("page_size"),
  };
}
```

### 5.2 Create CustomersList component

- [ ] Table with columns: Customer (avatar + name + email), Orders, Total Spent, Last Order, Status, Tags.
- [ ] Each row links to `/customers/[id]`.
- [ ] Use `CustomerStatusBadge` for active/blocked status.
- [ ] Format `total_spent` as currency. Format `last_order_at` as relative date.
- [ ] Skeleton loading: render placeholder rows while stats columns load (server-side in this case, so skeleton is for the initial page load via Suspense boundary).

```tsx
// apps/admin/components/customers/CustomersList.tsx
"use client";

import Link from "next/link";
import type { AdminCustomerListItem } from "@/lib/api/customers-api";
import { CustomerStatusBadge } from "./CustomerStatusBadge";
import { formatDistanceToNow } from "date-fns";

interface CustomersListProps {
  customers: AdminCustomerListItem[];
  storeId: string;
}

export function CustomersList({ customers, storeId }: CustomersListProps) {
  return (
    <div className="overflow-x-auto">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b border-[color:var(--ink-900)]/10 text-left text-xs uppercase tracking-wider text-[color:var(--ink-900)]/50">
            <th className="pb-3 pr-4 font-medium">Customer</th>
            <th className="pb-3 pr-4 font-medium">Orders</th>
            <th className="pb-3 pr-4 font-medium">Total Spent</th>
            <th className="pb-3 pr-4 font-medium">Last Order</th>
            <th className="pb-3 pr-4 font-medium">Status</th>
            <th className="pb-3 font-medium">Tags</th>
          </tr>
        </thead>
        <tbody>
          {customers.map((c) => (
            <tr
              key={c.id}
              className="border-b border-[color:var(--ink-900)]/5 hover:bg-[color:var(--ink-900)]/[0.02] transition-colors"
            >
              <td className="py-3 pr-4">
                <Link href={`/customers/${c.id}`} className="group">
                  <p className="font-medium text-[color:var(--ink-900)] group-hover:text-[color:var(--moss-700)]">
                    {c.first_name ?? ""} {c.last_name ?? ""}
                  </p>
                  <p className="text-xs text-[color:var(--ink-900)]/50">{c.email}</p>
                </Link>
              </td>
              <td className="py-3 pr-4 tabular-nums">{c.order_count}</td>
              <td className="py-3 pr-4 tabular-nums font-[family-name:var(--font-serif)]">
                {formatCurrency(c.total_spent)}
              </td>
              <td className="py-3 pr-4 text-[color:var(--ink-900)]/50">
                {c.last_order_at
                  ? formatDistanceToNow(new Date(c.last_order_at), { addSuffix: true })
                  : "Never"}
              </td>
              <td className="py-3 pr-4">
                <CustomerStatusBadge status={c.status} />
              </td>
              <td className="py-3">
                <div className="flex flex-wrap gap-1">
                  {c.tags.slice(0, 3).map((tag) => (
                    <span
                      key={tag}
                      className="inline-flex rounded-full bg-[color:var(--ink-900)]/5 px-2 py-0.5 text-xs"
                    >
                      {tag}
                    </span>
                  ))}
                  {c.tags.length > 3 && (
                    <span className="text-xs text-[color:var(--ink-900)]/40">
                      +{c.tags.length - 3}
                    </span>
                  )}
                </div>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function formatCurrency(value: string): string {
  const num = parseFloat(value);
  if (isNaN(num)) return value;
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency: "USD",
  }).format(num);
}
```

### 5.3 Create supporting components

- [ ] `CustomersListHeader` — page title "Customers", no create button (customers are auto-created via storefront auth).
- [ ] `CustomersListFilters` — search input (debounced, pushes `?search=` to URL), status dropdown (All / Active / Blocked).
- [ ] `CustomersListPagination` — reuse pattern from `ProductsListPagination`.
- [ ] `CustomersListEmpty` — variants: "no-store", "no-customers" ("No customers yet. They'll appear here after their first order."), "no-results".
- [ ] `CustomerStatusBadge` — "Active" in moss-700 tint, "Blocked" in signal/danger tint.

---

## Task 6 — Admin UI: Customer Detail Page

> **Files:**
> - `apps/admin/app/customers/[id]/page.tsx` (new)
> - `apps/admin/components/customers/CustomerDetailHeader.tsx` (new)
> - `apps/admin/components/customers/CustomerOverviewTab.tsx` (new)
> - `apps/admin/components/customers/CustomerOrdersTab.tsx` (new)
> - `apps/admin/components/customers/CustomerAddressesTab.tsx` (new)
> - `apps/admin/components/customers/CustomerNotesEditor.tsx` (new)
> - `apps/admin/components/customers/CustomerTagsEditor.tsx` (new)
> - `apps/admin/components/customers/CustomerBlockDialog.tsx` (new)

### 6.1 Create the page component

- [ ] Follow `apps/admin/app/products/[id]/page.tsx` pattern:

```tsx
// apps/admin/app/customers/[id]/page.tsx
import { notFound } from "next/navigation";
import { AdminShell } from "@/components/shell/AdminShell";
import { getServerSessionContext } from "@/lib/auth/serverSession";
import { getCustomer } from "@/lib/api/customers-api";
import { CustomerDetailHeader } from "@/components/customers/CustomerDetailHeader";
import { CustomerDetailTabs } from "@/components/customers/CustomerDetailTabs";

interface PageProps {
  params: Promise<{ id: string }>;
}

export default async function CustomerDetailPage({ params }: PageProps) {
  const { id } = await params;
  const session = await getServerSessionContext();
  const { tenantName, email, currentStore, role, userId, tenantId } = session;

  if (!currentStore) {
    notFound();
  }

  const customer = await getCustomer(currentStore.id, id, { userId, tenantId });
  if (!customer) {
    notFound();
  }

  const canEdit = role === "owner" || role === "admin";

  return (
    <AdminShell tenantName={tenantName} userEmail={email}>
      <main className="flex flex-col gap-6 px-8 py-6">
        <CustomerDetailHeader customer={customer} canEdit={canEdit} storeId={currentStore.id} />
        <CustomerDetailTabs customer={customer} canEdit={canEdit} storeId={currentStore.id} />
      </main>
    </AdminShell>
  );
}
```

### 6.2 Create CustomerDetailHeader

- [ ] Shows customer name, email, avatar, status badge.
- [ ] Block/Unblock button (admin+ only) — opens `CustomerBlockDialog` on block.
- [ ] Back link to `/customers`.

### 6.3 Create CustomerDetailTabs

- [ ] Client component with tab state. Tabs: Overview, Orders, Addresses.
- [ ] Use `@tesserix/web` Tabs primitive if available, otherwise build with Radix Tabs.

### 6.4 Create Overview tab

- [ ] Stats cards row: Total Orders, Total Spent, Average Order Value (computed client-side: total_spent / order_count), Last Order date.
- [ ] Stats use serif numerals (`font-[family-name:var(--font-serif)]`).
- [ ] `CustomerTagsEditor` — inline tag editor. Renders tags as pills with "x" to remove, plus an input to add. Calls `patchCustomer` on change.
- [ ] `CustomerNotesEditor` — textarea with save button. Calls `patchCustomer({ notes })` on save.
- [ ] Marketing opt-in toggle.

```tsx
// apps/admin/components/customers/CustomerOverviewTab.tsx
"use client";

import type { AdminCustomerDetail } from "@/lib/api/customers-api";
import { CustomerTagsEditor } from "./CustomerTagsEditor";
import { CustomerNotesEditor } from "./CustomerNotesEditor";

interface CustomerOverviewTabProps {
  customer: AdminCustomerDetail;
  canEdit: boolean;
  storeId: string;
}

export function CustomerOverviewTab({ customer, canEdit, storeId }: CustomerOverviewTabProps) {
  const orderCount = customer.order_count;
  const totalSpent = parseFloat(customer.total_spent);
  const aov = orderCount > 0 ? totalSpent / orderCount : 0;

  return (
    <div className="grid grid-cols-1 gap-8 lg:grid-cols-3">
      {/* Stats — left 2 columns */}
      <div className="lg:col-span-2 space-y-8">
        {/* Key metrics */}
        <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
          <StatCard label="Total Orders" value={String(orderCount)} />
          <StatCard label="Total Spent" value={formatCurrency(totalSpent)} serif />
          <StatCard label="Avg. Order Value" value={formatCurrency(aov)} serif />
          <StatCard
            label="Last Order"
            value={customer.last_order_at ? formatDate(customer.last_order_at) : "Never"}
          />
        </div>

        {/* Tags */}
        <section>
          <h3 className="text-xs uppercase tracking-wider text-[color:var(--ink-900)]/50 mb-3">
            Tags
          </h3>
          <CustomerTagsEditor
            customerId={customer.id}
            storeId={storeId}
            initialTags={customer.tags}
            readOnly={!canEdit}
          />
        </section>
      </div>

      {/* Notes — right column */}
      <div>
        <h3 className="text-xs uppercase tracking-wider text-[color:var(--ink-900)]/50 mb-3">
          Internal Notes
        </h3>
        <CustomerNotesEditor
          customerId={customer.id}
          storeId={storeId}
          initialNotes={customer.notes ?? ""}
          readOnly={!canEdit}
        />
      </div>
    </div>
  );
}
```

### 6.5 Create Orders tab

- [ ] Reuse the order list table pattern from the orders page. Fetch orders filtered by `customer_email` via the existing orders API (add `customer_email` query param support to the orders list endpoint if not present, or filter client-side from the customer detail).
- [ ] Alternatively, add a dedicated endpoint: `GET /admin/stores/:storeId/customers/:id/orders` that queries orders WHERE customer_email = profile.email. This is cleaner but requires a handler addition — use inline GORM query in the customer handler's Get response or a separate tab-load API route.
- [ ] For MVP: include recent orders (last 10) in the detail response and render them inline. Pagination deferred.

### 6.6 Create Addresses tab

- [ ] Render `customer.addresses` as a list of address cards.
- [ ] Each card shows: label, name, full address, phone, default badge.
- [ ] Read-only in admin — addresses are managed by the customer on the storefront.

### 6.7 Create CustomerBlockDialog

- [ ] Uses `@tesserix/web` Dialog primitive.
- [ ] "Block Customer" title, reason textarea (required, max 300 chars), confirm button.
- [ ] Calls `blockCustomer(storeId, customerId, reason, session)` via a server action or API route.
- [ ] On success: `router.refresh()` to reload the page with updated status.

### 6.8 Create CustomerTagsEditor

- [ ] Client component. Renders tags as removable pills.
- [ ] Input field with Enter to add. Validates: no duplicates, max 20 tags, max 50 chars each.
- [ ] On change: debounced `patchCustomer({ tags })` call via API route.

### 6.9 Create CustomerNotesEditor

- [ ] Client component. Textarea with character count.
- [ ] "Save" button that calls `patchCustomer({ notes })` via API route.
- [ ] Success toast via `sonner`.

### 6.10 Create admin API routes for mutations

- [ ] `apps/admin/app/api/customers/[id]/route.ts` — PATCH proxy to marketplace-api.
- [ ] `apps/admin/app/api/customers/[id]/block/route.ts` — POST proxy.
- [ ] `apps/admin/app/api/customers/[id]/unblock/route.ts` — POST proxy.
- [ ] These follow the existing pattern where client components POST to Next.js API routes which proxy to marketplace-api with server-side session headers.

---

## Task 7 — Sidebar: Update Customers Href

> **Files:**
> - `apps/admin/components/shell/AdminShell.tsx` (modify)

### 7.1 Update sidebar nav items

- [ ] Change the customers children hrefs from placeholder `/dashboard` / `/customers` to real routes:

```typescript
// Before:
{ label: "All Customers", href: "/customers" },
{ label: "Reviews", href: "/customers" },

// After:
{ label: "All Customers", href: "/customers" },
{ label: "Reviews", href: "/reviews" },  // C3 will create this page; href is correct now
```

- [ ] Verify `getPageTitle` function already handles `/customers` prefix (it does — returns `{ eyebrow: "Relationships", title: "Customers" }`).

---

## Task 8 — Build Verification

### 8.1 Go build

- [ ] Run `cd services/marketplace-api && go build ./...` — must pass with zero errors.
- [ ] Run `cd services/marketplace-api && go vet ./...` — must pass.

### 8.2 Go tests

- [ ] Run `cd services/marketplace-api && go test ./internal/customer/... -v` — repository tests pass.
- [ ] Run `cd services/marketplace-api && go test ./internal/handlers/admin/... -v` — handler tests pass (if integration tests exist).

### 8.3 TypeScript build

- [ ] Run `cd apps/admin && npx next build` — must pass. Verify no type errors in new files.

### 8.4 Manual smoke test

- [ ] Start dev servers: `make dev` (runs marketplace-api + admin).
- [ ] Navigate to `/customers` — page renders (empty state if no customers in DB).
- [ ] If test data exists: verify list shows customer rows with stats.
- [ ] Click a customer row — detail page renders with tabs.
- [ ] Test tag add/remove, notes save, block/unblock flow.

---

## File Summary

### New files (Go)
| File | Purpose |
|------|---------|
| `services/marketplace-api/internal/customer/repository.go` | Admin repository methods (ListForStore, GetByID, UpdateTags, etc.) |
| `services/marketplace-api/internal/customer/models.go` | CustomerWithStats, ListCustomersQuery |
| `services/marketplace-api/internal/handlers/admin/customers.go` | Handler: List, Get, Patch, Block, Unblock |
| `services/marketplace-api/internal/handlers/admin/customers_dto.go` | Wire DTOs and mapper functions |
| `services/marketplace-api/internal/authz/customers_roles.go` | CustomersViewRole, CustomersEditRole, CustomersBlockRole |

### New files (TypeScript)
| File | Purpose |
|------|---------|
| `apps/admin/lib/api/customers-api.ts` | Server-side API client |
| `apps/admin/app/customers/page.tsx` | Customer list page |
| `apps/admin/app/customers/[id]/page.tsx` | Customer detail page |
| `apps/admin/components/customers/CustomersListHeader.tsx` | Page header |
| `apps/admin/components/customers/CustomersListFilters.tsx` | Search + status filter |
| `apps/admin/components/customers/CustomersList.tsx` | Table component |
| `apps/admin/components/customers/CustomersListPagination.tsx` | Pagination |
| `apps/admin/components/customers/CustomersListEmpty.tsx` | Empty states |
| `apps/admin/components/customers/CustomerStatusBadge.tsx` | Active/Blocked badge |
| `apps/admin/components/customers/CustomerDetailHeader.tsx` | Detail page header + block button |
| `apps/admin/components/customers/CustomerDetailTabs.tsx` | Tab container |
| `apps/admin/components/customers/CustomerOverviewTab.tsx` | Stats + tags + notes |
| `apps/admin/components/customers/CustomerOrdersTab.tsx` | Recent orders table |
| `apps/admin/components/customers/CustomerAddressesTab.tsx` | Address cards |
| `apps/admin/components/customers/CustomerNotesEditor.tsx` | Notes textarea |
| `apps/admin/components/customers/CustomerTagsEditor.tsx` | Tag pill editor |
| `apps/admin/components/customers/CustomerBlockDialog.tsx` | Block confirmation dialog |
| `apps/admin/app/api/customers/[id]/route.ts` | PATCH proxy |
| `apps/admin/app/api/customers/[id]/block/route.ts` | Block proxy |
| `apps/admin/app/api/customers/[id]/unblock/route.ts` | Unblock proxy |

### Modified files
| File | Change |
|------|--------|
| `services/marketplace-api/internal/handlers/admin/routes.go` | Add `CustomerHandler` to Deps, register `/customers` routes |
| `services/marketplace-api/cmd/marketplace-api/main.go` | Wire customer repo + handler, add to adminDeps |
| `apps/admin/components/shell/AdminShell.tsx` | Update Reviews href to `/reviews` |
