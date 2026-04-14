# Phase 1 — Vendor Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `Vendor` entity to the marketplace-api, auto-create exactly one "self" vendor per tenant, and make `products.vendor_id` NOT NULL — laying the foundation for Model C (shared catalog, multi-vendor) without any user-visible change.

**Architecture:** The Vendor entity lives in marketplace-api (co-located with the `products.vendor_id` column that already exists as nullable UUID). Platform-api calls marketplace-api's idempotent `POST /internal/tenants/:tid/ensure-self-vendor` endpoint after onboarding completes. A one-shot migration backfills vendors for existing tenants (inferred from `DISTINCT tenant_id` in `products`) and populates `products.vendor_id`. A follow-up migration adds `NOT NULL`. A platform-api backfill CLI overwrites placeholder vendor names/slugs with the real tenant values.

**Tech Stack:** Go 1.26 + Gin + GORM (both services), PostgreSQL migrations via golang-migrate, testify for tests.

---

## Spec reference

See `docs/superpowers/specs/2026-04-14-tenant-vendor-store-architecture-design.md` — this plan implements Phase 1 from that spec.

## File structure

### marketplace-api (new)
- `services/marketplace-api/migrations/000027_vendors.up.sql` — create table, backfill vendors, backfill `products.vendor_id`
- `services/marketplace-api/migrations/000027_vendors.down.sql` — drop table (no data preservation; Phase 1 is reversible)
- `services/marketplace-api/migrations/000028_products_vendor_id_not_null.up.sql` — `ALTER COLUMN vendor_id SET NOT NULL`
- `services/marketplace-api/migrations/000028_products_vendor_id_not_null.down.sql` — drop the constraint
- `services/marketplace-api/internal/vendor/models.go`
- `services/marketplace-api/internal/vendor/repository.go`
- `services/marketplace-api/internal/vendor/repository_test.go`
- `services/marketplace-api/internal/vendor/service.go`
- `services/marketplace-api/internal/vendor/service_test.go`
- `services/marketplace-api/internal/vendor/handler.go`
- `services/marketplace-api/internal/vendor/handler_test.go`

### marketplace-api (modify)
- `services/marketplace-api/cmd/marketplace-api/main.go` — wire vendor module into the router
- `services/marketplace-api/migrations.go` — register the two new migrations

### platform-api (new)
- `services/platform-api/internal/marketplaceapi/vendor_client.go` — HTTP client for marketplace-api's vendor endpoints
- `services/platform-api/internal/marketplaceapi/vendor_client_test.go`
- `services/platform-api/cmd/backfill-vendors/main.go` — one-shot CLI that iterates tenants and upserts real vendor name/slug via the client

### platform-api (modify)
- `services/platform-api/internal/onboarding/service.go` — after onboarding commits, call `marketplaceapi.EnsureSelfVendor`
- `services/platform-api/internal/onboarding/service_test.go` — add test coverage for the vendor call

---

## Task 1: Create the vendors migration (schema + backfill)

**Files:**
- Create: `services/marketplace-api/migrations/000027_vendors.up.sql`
- Create: `services/marketplace-api/migrations/000027_vendors.down.sql`

- [ ] **Step 1: Write `000027_vendors.up.sql`**

```sql
-- 000027_vendors.up.sql
-- Phase 1 of the tenant/vendor/store refactor. See
-- docs/superpowers/specs/2026-04-14-tenant-vendor-store-architecture-design.md

-- 1. vendors table
CREATE TABLE IF NOT EXISTS vendors (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  UUID        NOT NULL,
    name       VARCHAR(200) NOT NULL,
    slug       VARCHAR(63) NOT NULL,
    status     VARCHAR(32) NOT NULL DEFAULT 'active',
    is_self    BOOLEAN     NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS vendors_slug_key ON vendors (slug);
CREATE UNIQUE INDEX IF NOT EXISTS vendors_tenant_self_idx
    ON vendors (tenant_id)
    WHERE is_self = true;
CREATE INDEX IF NOT EXISTS vendors_tenant_id_idx ON vendors (tenant_id);

-- 2. Backfill: one self-vendor per tenant with products today.
--    Name/slug are placeholders — platform-api's backfill-vendors CLI
--    overwrites them with the real tenant name + tenant-derived slug.
INSERT INTO vendors (tenant_id, name, slug, status, is_self)
SELECT DISTINCT
    p.tenant_id,
    'Merchant'                                 AS name,
    'vendor-' || REPLACE(p.tenant_id::text, '-', '') AS slug,
    'active'                                   AS status,
    true                                       AS is_self
FROM products p
WHERE p.tenant_id IS NOT NULL
ON CONFLICT DO NOTHING;

-- 3. Backfill products.vendor_id to the tenant's self-vendor.
UPDATE products p
SET    vendor_id = v.id
FROM   vendors v
WHERE  v.tenant_id = p.tenant_id
  AND  v.is_self    = true
  AND  p.vendor_id IS NULL;
```

- [ ] **Step 2: Write `000027_vendors.down.sql`**

```sql
-- 000027_vendors.down.sql
-- Reverses the vendors table creation. Intentionally does NOT try to
-- preserve products.vendor_id backfill — Phase 1 is fully reversible.

DROP INDEX IF EXISTS vendors_tenant_id_idx;
DROP INDEX IF EXISTS vendors_tenant_self_idx;
DROP INDEX IF EXISTS vendors_slug_key;
DROP TABLE IF EXISTS vendors;

-- NOTE: products.vendor_id is left populated. Harmless because the
-- column is still nullable at this point. Migration 000028 NOT NULL
-- is reversed by its own down.sql before this one runs.
```

- [ ] **Step 3: Register migrations in `migrations.go`**

Append the two new files to whatever embedded list `services/marketplace-api/migrations.go` uses. Read the file first and follow the existing pattern (likely a `//go:embed` directive over `migrations/*.sql`).

- [ ] **Step 4: Run the migration against a local DB**

```bash
cd services/marketplace-api
go run ./cmd/migrate -direction up 2>&1 | tail -20
```

Expected: `migrated to 27` or similar; no errors.

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/migrations/000027_vendors.up.sql \
        services/marketplace-api/migrations/000027_vendors.down.sql \
        services/marketplace-api/migrations.go
git commit -m "feat(marketplace-api): add vendors table + backfill from products"
```

---

## Task 2: Vendor model

**Files:**
- Create: `services/marketplace-api/internal/vendor/models.go`

- [ ] **Step 1: Write the model**

```go
// services/marketplace-api/internal/vendor/models.go
package vendor

import "time"

// Vendor is a seller under a tenant. Every tenant has exactly one
// self-vendor (is_self=true) representing the tenant itself. Real
// marketplace vendors (Phase 8+) will be additional rows with
// is_self=false.
type Vendor struct {
	ID        string    `gorm:"column:id;type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	TenantID  string    `gorm:"column:tenant_id;type:uuid;not null"                      json:"tenant_id"`
	Name      string    `gorm:"column:name;type:varchar(200);not null"                   json:"name"`
	Slug      string    `gorm:"column:slug;type:varchar(63);not null;uniqueIndex"        json:"slug"`
	Status    string    `gorm:"column:status;type:varchar(32);not null;default:active"   json:"status"`
	IsSelf    bool      `gorm:"column:is_self;not null;default:false"                    json:"is_self"`
	CreatedAt time.Time `gorm:"column:created_at;not null;default:now()"                 json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;not null;default:now()"                 json:"updated_at"`
}

// TableName pins the Vendor struct to the `vendors` table. GORM's default
// pluralisation would get it right, but pinning makes renames safer.
func (Vendor) TableName() string { return "vendors" }

// Status values.
const (
	StatusActive   = "active"
	StatusInactive = "inactive"
)
```

- [ ] **Step 2: Commit**

```bash
git add services/marketplace-api/internal/vendor/models.go
git commit -m "feat(marketplace-api): add Vendor model"
```

---

## Task 3: Vendor repository

**Files:**
- Create: `services/marketplace-api/internal/vendor/repository.go`
- Create: `services/marketplace-api/internal/vendor/repository_test.go`

- [ ] **Step 1: Write failing test for GetSelfByTenantID**

Examine an existing repo test (e.g. `services/marketplace-api/internal/product/repository_test.go`) to copy the test DB setup pattern, then:

```go
// services/marketplace-api/internal/vendor/repository_test.go
package vendor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRepository_GetSelfByTenantID_NotFound(t *testing.T) {
	db := newTestDB(t) // helper that mirrors existing patterns
	repo := NewRepository(db)

	v, err := repo.GetSelfByTenantID(context.Background(), "00000000-0000-0000-0000-000000000000")
	require.NoError(t, err)
	require.Nil(t, v)
}

func TestRepository_CreateAndGetSelf(t *testing.T) {
	db := newTestDB(t)
	repo := NewRepository(db)

	tenantID := "11111111-1111-1111-1111-111111111111"
	in := &Vendor{
		TenantID: tenantID,
		Name:     "Acme",
		Slug:     "acme-self",
		IsSelf:   true,
	}
	err := repo.Create(context.Background(), in)
	require.NoError(t, err)
	require.NotEmpty(t, in.ID)

	got, err := repo.GetSelfByTenantID(context.Background(), tenantID)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "Acme", got.Name)
	require.True(t, got.IsSelf)
}

func TestRepository_Create_SelfVendorUniquePerTenant(t *testing.T) {
	db := newTestDB(t)
	repo := NewRepository(db)

	tenantID := "22222222-2222-2222-2222-222222222222"
	require.NoError(t, repo.Create(context.Background(), &Vendor{
		TenantID: tenantID, Name: "A", Slug: "slug-a", IsSelf: true,
	}))
	err := repo.Create(context.Background(), &Vendor{
		TenantID: tenantID, Name: "B", Slug: "slug-b", IsSelf: true,
	})
	require.Error(t, err, "inserting a second self-vendor should violate the partial unique index")
}
```

- [ ] **Step 2: Run the test — expect fail**

```bash
cd services/marketplace-api
go test ./internal/vendor/... -run TestRepository -v
```

Expected: FAIL (NewRepository / Repository type not defined).

- [ ] **Step 3: Write the repository**

```go
// services/marketplace-api/internal/vendor/repository.go
package vendor

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

// Repository is the GORM-backed data access for vendors.
type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Create inserts a new vendor row. The caller must set TenantID, Name,
// Slug, IsSelf. ID, CreatedAt, UpdatedAt are filled by the DB defaults.
// Violates the `vendors_tenant_self_idx` partial unique index if a
// self-vendor already exists for the tenant.
func (r *Repository) Create(ctx context.Context, v *Vendor) error {
	return r.db.WithContext(ctx).Create(v).Error
}

// GetByID returns the vendor with that id or nil if not found.
func (r *Repository) GetByID(ctx context.Context, id string) (*Vendor, error) {
	var v Vendor
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&v).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// GetSelfByTenantID returns the tenant's self-vendor or nil if none
// exists yet (backfill not run, or tenant has zero products today).
func (r *Repository) GetSelfByTenantID(ctx context.Context, tenantID string) (*Vendor, error) {
	var v Vendor
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND is_self = ?", tenantID, true).
		First(&v).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// UpdateNameAndSlug overwrites the display name and slug of an
// existing vendor. Used by the backfill CLI to replace the
// migration's placeholder values with real tenant identity.
func (r *Repository) UpdateNameAndSlug(ctx context.Context, id, name, slug string) error {
	return r.db.WithContext(ctx).Model(&Vendor{}).
		Where("id = ?", id).
		Updates(map[string]any{"name": name, "slug": slug}).Error
}
```

Helper for the test (if no existing helper exists, add alongside):

```go
// services/marketplace-api/internal/vendor/repository_test_helper_test.go
package vendor

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	// reuse whichever test-DB helper exists in the codebase; if not,
	// follow the pattern from internal/product/repository_test.go
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	// EXISTING PATTERN: copy from an adjacent test file. Do NOT
	// invent a new harness — mirror what products or categories use.
	db, err := gorm.Open(postgres.Open(testDSN(t)), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Vendor{}))
	t.Cleanup(func() {
		db.Exec("TRUNCATE vendors CASCADE")
	})
	return db
}
```

If the codebase already has a test-DB helper (very likely), use it instead and delete `newTestDB` above.

- [ ] **Step 4: Run the tests — expect pass**

```bash
go test ./internal/vendor/... -run TestRepository -v
```

Expected: PASS (all three).

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/vendor/repository.go \
        services/marketplace-api/internal/vendor/repository_test.go \
        services/marketplace-api/internal/vendor/repository_test_helper_test.go
git commit -m "feat(marketplace-api): add Vendor repository (create, get, upsert)"
```

---

## Task 4: Vendor service — idempotent EnsureSelfVendor

**Files:**
- Create: `services/marketplace-api/internal/vendor/service.go`
- Create: `services/marketplace-api/internal/vendor/service_test.go`

- [ ] **Step 1: Write failing tests**

```go
// services/marketplace-api/internal/vendor/service_test.go
package vendor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestService_EnsureSelfVendor_CreatesOnFirstCall(t *testing.T) {
	svc := newTestService(t)
	tenantID := "33333333-3333-3333-3333-333333333333"

	v, err := svc.EnsureSelfVendor(context.Background(), tenantID, "Acme", "acme")
	require.NoError(t, err)
	require.NotNil(t, v)
	require.Equal(t, "Acme", v.Name)
	require.Equal(t, "acme", v.Slug)
	require.True(t, v.IsSelf)
}

func TestService_EnsureSelfVendor_IdempotentOnSecondCall(t *testing.T) {
	svc := newTestService(t)
	tenantID := "44444444-4444-4444-4444-444444444444"

	first, err := svc.EnsureSelfVendor(context.Background(), tenantID, "Acme", "acme")
	require.NoError(t, err)

	second, err := svc.EnsureSelfVendor(context.Background(), tenantID, "Renamed", "new-slug")
	require.NoError(t, err)
	require.Equal(t, first.ID, second.ID, "idempotent — same vendor row returned")
	require.Equal(t, "Acme", second.Name, "existing name is NOT overwritten by Ensure")
}

func TestService_EnsureSelfVendor_RejectsEmptyInput(t *testing.T) {
	svc := newTestService(t)
	_, err := svc.EnsureSelfVendor(context.Background(), "", "Acme", "acme")
	require.Error(t, err)
	_, err = svc.EnsureSelfVendor(context.Background(), "tenant", "", "acme")
	require.Error(t, err)
	_, err = svc.EnsureSelfVendor(context.Background(), "tenant", "Acme", "")
	require.Error(t, err)
}

func newTestService(t *testing.T) *Service {
	db := newTestDB(t)
	return NewService(NewRepository(db))
}
```

- [ ] **Step 2: Run the tests — expect fail**

```bash
go test ./internal/vendor/... -run TestService -v
```

Expected: FAIL (NewService / Service type not defined).

- [ ] **Step 3: Write the service**

```go
// services/marketplace-api/internal/vendor/service.go
package vendor

import (
	"context"
	"strings"

	apperrors "github.com/mark8ly/marketplace-api/pkg/errors"
	// If the error helper path differs, mirror the one used by
	// internal/product/service.go — don't invent a new one.
)

// Service is the business-logic layer for vendors. Phase 1 only needs
// EnsureSelfVendor (idempotent create-or-return). Later phases add
// CRUD for marketplace vendor onboarding.
type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

// EnsureSelfVendor is called exactly once per tenant (from onboarding),
// and possibly again from the platform-api backfill CLI. If the tenant
// already has a self-vendor, returns that row unchanged — the caller
// must use UpdateNameAndSlug (via the backfill path) to overwrite
// placeholder names.
func (s *Service) EnsureSelfVendor(ctx context.Context, tenantID, name, slug string) (*Vendor, error) {
	tenantID = strings.TrimSpace(tenantID)
	name = strings.TrimSpace(name)
	slug = strings.TrimSpace(slug)
	if tenantID == "" {
		return nil, apperrors.BadRequest("invalid_tenant_id", "tenant id is required")
	}
	if name == "" {
		return nil, apperrors.BadRequest("invalid_name", "vendor name is required")
	}
	if slug == "" {
		return nil, apperrors.BadRequest("invalid_slug", "vendor slug is required")
	}

	existing, err := s.repo.GetSelfByTenantID(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}

	v := &Vendor{
		TenantID: tenantID,
		Name:     name,
		Slug:     slug,
		Status:   StatusActive,
		IsSelf:   true,
	}
	if err := s.repo.Create(ctx, v); err != nil {
		// Race: another caller created the self-vendor between our
		// Get and Create. Retry the Get and return whatever we find.
		if again, err2 := s.repo.GetSelfByTenantID(ctx, tenantID); err2 == nil && again != nil {
			return again, nil
		}
		return nil, err
	}
	return v, nil
}
```

- [ ] **Step 4: Run the tests — expect pass**

```bash
go test ./internal/vendor/... -run TestService -v
```

Expected: PASS (all four).

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/vendor/service.go \
        services/marketplace-api/internal/vendor/service_test.go
git commit -m "feat(marketplace-api): add Vendor service with idempotent EnsureSelfVendor"
```

---

## Task 5: Vendor HTTP handler

**Files:**
- Create: `services/marketplace-api/internal/vendor/handler.go`
- Create: `services/marketplace-api/internal/vendor/handler_test.go`

- [ ] **Step 1: Write failing handler test**

```go
// services/marketplace-api/internal/vendor/handler_test.go
package vendor

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestHandler_EnsureSelfVendor_Creates(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newTestService(t)
	h := NewHandler(svc)

	r := gin.New()
	h.RegisterRoutes(r.Group("/internal"))

	tenantID := "55555555-5555-5555-5555-555555555555"
	body, _ := json.Marshal(map[string]string{"name": "Acme", "slug": "acme"})
	req := httptest.NewRequest(http.MethodPost,
		"/internal/tenants/"+tenantID+"/ensure-self-vendor",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Data Vendor `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, "Acme", resp.Data.Name)
	require.True(t, resp.Data.IsSelf)
}

func TestHandler_EnsureSelfVendor_BadBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newTestService(t)
	h := NewHandler(svc)

	r := gin.New()
	h.RegisterRoutes(r.Group("/internal"))

	req := httptest.NewRequest(http.MethodPost,
		"/internal/tenants/66666666-6666-6666-6666-666666666666/ensure-self-vendor",
		bytes.NewReader([]byte(`{"name": "", "slug": "x"}`)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}
```

- [ ] **Step 2: Run — expect fail**

```bash
go test ./internal/vendor/... -run TestHandler -v
```

- [ ] **Step 3: Write the handler**

```go
// services/marketplace-api/internal/vendor/handler.go
package vendor

import (
	"net/http"

	"github.com/gin-gonic/gin"

	apperrors "github.com/mark8ly/marketplace-api/pkg/errors"
)

// Handler is the thin HTTP layer that exposes vendor endpoints
// for internal callers (platform-api, the backfill CLI).
type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes mounts the vendor routes on the given router group.
// Caller typically passes the `/internal` group so these stay off the
// public API surface.
func (h *Handler) RegisterRoutes(g *gin.RouterGroup) {
	g.POST("/tenants/:tenantID/ensure-self-vendor", h.ensureSelfVendor)
	g.GET("/tenants/:tenantID/self-vendor", h.getSelfVendor)
	g.GET("/vendors/:id", h.getByID)
}

type ensureSelfVendorRequest struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

func (h *Handler) ensureSelfVendor(c *gin.Context) {
	var req ensureSelfVendorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_body",
			"message": "request body is not valid JSON",
		})
		return
	}
	v, err := h.svc.EnsureSelfVendor(c.Request.Context(), c.Param("tenantID"), req.Name, req.Slug)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": v})
}

func (h *Handler) getSelfVendor(c *gin.Context) {
	v, err := h.svc.repo.GetSelfByTenantID(c.Request.Context(), c.Param("tenantID"))
	if err != nil {
		respondError(c, err)
		return
	}
	if v == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "not_found",
			"message": "no self-vendor for this tenant",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": v})
}

func (h *Handler) getByID(c *gin.Context) {
	v, err := h.svc.repo.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		respondError(c, err)
		return
	}
	if v == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "not_found",
			"message": "vendor not found",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": v})
}

func respondError(c *gin.Context, err error) {
	if ae, ok := apperrors.As(err); ok {
		c.JSON(ae.Status, gin.H{"error": ae.Code, "message": ae.Message})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{
		"error":   "internal_error",
		"message": "an unexpected error occurred",
	})
}
```

- [ ] **Step 4: Run — expect pass**

```bash
go test ./internal/vendor/... -v
```

Expected: every test in the package PASSes.

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/vendor/handler.go \
        services/marketplace-api/internal/vendor/handler_test.go
git commit -m "feat(marketplace-api): add internal vendor HTTP handler"
```

---

## Task 6: Wire vendor module into main.go

**Files:**
- Modify: `services/marketplace-api/cmd/marketplace-api/main.go`

- [ ] **Step 1: Read the main.go and find the internal router group**

```bash
grep -n "internal\|/internal\|RouterGroup" services/marketplace-api/cmd/marketplace-api/main.go | head -20
```

Identify where other domain handlers (product, order) are registered onto the `/internal` group.

- [ ] **Step 2: Register the vendor handler**

Find the block where existing handlers are mounted and add:

```go
// Vendor — Phase 1 of the tenant/vendor/store refactor. See
// docs/superpowers/specs/2026-04-14-tenant-vendor-store-architecture-design.md
vendorRepo := vendor.NewRepository(db)
vendorSvc := vendor.NewService(vendorRepo)
vendorH := vendor.NewHandler(vendorSvc)
vendorH.RegisterRoutes(internalGroup) // use the exact group variable name used by the surrounding code
```

And add the import:

```go
"github.com/mark8ly/marketplace-api/internal/vendor"
```

- [ ] **Step 3: Build**

```bash
cd services/marketplace-api
go build ./... 2>&1 | tail -10
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/cmd/marketplace-api/main.go
git commit -m "feat(marketplace-api): wire vendor handler into router"
```

---

## Task 7: Platform-api client for marketplace-api vendor endpoints

**Files:**
- Create: `services/platform-api/internal/marketplaceapi/vendor_client.go`
- Create: `services/platform-api/internal/marketplaceapi/vendor_client_test.go`

- [ ] **Step 1: Write failing test using httptest**

```go
// services/platform-api/internal/marketplaceapi/vendor_client_test.go
package marketplaceapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVendorClient_EnsureSelfVendor_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/internal/tenants/tenant-abc/ensure-self-vendor", r.URL.Path)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"id":        "vendor-1",
				"tenant_id": "tenant-abc",
				"name":      "Acme",
				"slug":      "acme",
				"is_self":   true,
				"status":    "active",
			},
		})
	}))
	defer srv.Close()

	c := NewVendorClient(srv.URL)
	v, err := c.EnsureSelfVendor(context.Background(), "tenant-abc", "Acme", "acme")
	require.NoError(t, err)
	require.Equal(t, "vendor-1", v.ID)
	require.Equal(t, "Acme", v.Name)
}

func TestVendorClient_EnsureSelfVendor_BadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"boom","message":"down"}`))
	}))
	defer srv.Close()

	c := NewVendorClient(srv.URL)
	_, err := c.EnsureSelfVendor(context.Background(), "tenant-abc", "Acme", "acme")
	require.Error(t, err)
}
```

- [ ] **Step 2: Run — expect fail**

```bash
cd services/platform-api
go test ./internal/marketplaceapi/... -v
```

- [ ] **Step 3: Write the client**

```go
// services/platform-api/internal/marketplaceapi/vendor_client.go
package marketplaceapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Vendor mirrors the shape returned by marketplace-api's vendor
// endpoints. Kept minimal — platform-api only cares about the id on
// the happy path.
type Vendor struct {
	ID       string `json:"id"`
	TenantID string `json:"tenant_id"`
	Name     string `json:"name"`
	Slug     string `json:"slug"`
	Status   string `json:"status"`
	IsSelf   bool   `json:"is_self"`
}

// VendorClient is a thin HTTP client for marketplace-api's
// /internal/tenants/:tid/ensure-self-vendor endpoint.
type VendorClient struct {
	baseURL string
	http    *http.Client
}

func NewVendorClient(baseURL string) *VendorClient {
	return &VendorClient{baseURL: baseURL, http: http.DefaultClient}
}

// EnsureSelfVendor calls the idempotent endpoint. Safe to call once
// per tenant; repeated calls return the existing vendor unchanged.
func (c *VendorClient) EnsureSelfVendor(ctx context.Context, tenantID, name, slug string) (*Vendor, error) {
	body, err := json.Marshal(map[string]string{"name": name, "slug": slug})
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/internal/tenants/%s/ensure-self-vendor", c.baseURL, tenantID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode >= 300 {
		raw, _ := io.ReadAll(res.Body)
		return nil, fmt.Errorf("marketplace-api ensure-self-vendor %d: %s", res.StatusCode, string(raw))
	}

	var resp struct {
		Data Vendor `json:"data"`
	}
	if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}
```

- [ ] **Step 4: Run — expect pass**

```bash
go test ./internal/marketplaceapi/... -v
```

- [ ] **Step 5: Commit**

```bash
git add services/platform-api/internal/marketplaceapi/vendor_client.go \
        services/platform-api/internal/marketplaceapi/vendor_client_test.go
git commit -m "feat(platform-api): add marketplace-api vendor client"
```

---

## Task 8: Hook self-vendor creation into onboarding.Complete

**Files:**
- Modify: `services/platform-api/internal/onboarding/service.go`
- Modify: `services/platform-api/internal/onboarding/service_test.go` (or add a new file)

- [ ] **Step 1: Read the current `service.go` around the Complete function**

```bash
sed -n '170,270p' services/platform-api/internal/onboarding/service.go
```

- [ ] **Step 2: Add a `vendorClient` dependency to the Service struct**

Find the `Service` struct definition (near the top of the file) and add:

```go
type Service struct {
    // ...existing fields...
    vendorClient VendorEnsurer // interface defined below
}

// VendorEnsurer lets us swap the real marketplace-api client for a
// fake in tests. Mirrors the shape of
// marketplaceapi.VendorClient.EnsureSelfVendor.
type VendorEnsurer interface {
    EnsureSelfVendor(ctx context.Context, tenantID, name, slug string) (*marketplaceapi.Vendor, error)
}
```

Update `NewService` to accept the `VendorEnsurer` as a parameter, and update every caller (main.go wiring) to pass in a real `marketplaceapi.NewVendorClient(config.MarketplaceAPIURL)`.

- [ ] **Step 3: Call EnsureSelfVendor after the tx commits**

Inside `Complete`, immediately after the `err = s.db.WithContext(ctx).Transaction(...)` block succeeds and before the welcome email:

```go
// Phase 1 of the tenant/vendor/store refactor: create the tenant's
// self-vendor in marketplace-api. Best-effort — a failure is logged
// but does NOT fail onboarding. The platform-api backfill CLI
// (cmd/backfill-vendors) covers misses.
if s.vendorClient != nil {
    if _, vErr := s.vendorClient.EnsureSelfVendor(ctx, t.ID, t.Name, st.Slug); vErr != nil {
        log.Printf("onboarding.Complete: ensure self-vendor for tenant %s: %v", t.ID, vErr)
    }
}
```

(Use the existing logger pattern in the file; this code assumes `"log"` but the codebase may use logrus/slog — match whatever is already imported.)

- [ ] **Step 4: Add a test that verifies the call happens**

In `service_test.go`, add a fake implementing `VendorEnsurer` that records its calls, inject it into the service under test, and assert:

```go
type fakeVendorClient struct {
	calls []struct{ tenantID, name, slug string }
	err   error
}

func (f *fakeVendorClient) EnsureSelfVendor(_ context.Context, tenantID, name, slug string) (*marketplaceapi.Vendor, error) {
	f.calls = append(f.calls, struct{ tenantID, name, slug string }{tenantID, name, slug})
	if f.err != nil {
		return nil, f.err
	}
	return &marketplaceapi.Vendor{ID: "vendor-" + tenantID, TenantID: tenantID, Name: name, Slug: slug, IsSelf: true}, nil
}

func TestComplete_CallsEnsureSelfVendor(t *testing.T) {
	// mirror the existing Complete test fixtures here; the critical
	// assertion is:
	//   require.Len(t, fake.calls, 1)
	//   require.Equal(t, tenantID, fake.calls[0].tenantID)
	//   require.Equal(t, businessName, fake.calls[0].name)
	//   require.Equal(t, slug, fake.calls[0].slug)
}

func TestComplete_SwallowsVendorError(t *testing.T) {
	// with fake.err != nil, Complete must still return a valid
	// CompleteResult and the tenant must still be created.
}
```

- [ ] **Step 5: Run the tests**

```bash
cd services/platform-api
go test ./internal/onboarding/... -v
```

Expected: existing tests still pass; the two new tests pass.

- [ ] **Step 6: Commit**

```bash
git add services/platform-api/internal/onboarding/service.go \
        services/platform-api/internal/onboarding/service_test.go
git commit -m "feat(platform-api): call marketplace-api EnsureSelfVendor after onboarding"
```

---

## Task 9: Wire the vendor client into the platform-api server startup

**Files:**
- Modify: `services/platform-api/cmd/server/main.go`

- [ ] **Step 1: Find the config for MARKETPLACE_API_URL**

```bash
grep -n "MARKETPLACE_API_URL\|MarketplaceAPI" services/platform-api 2>&1 -r | head -10
```

If no config variable exists, add one in the config package:

```go
// services/platform-api/internal/config/config.go (or equivalent)
MarketplaceAPIURL string `env:"MARKETPLACE_API_URL" default:"http://marketplace-api.mark8ly.svc.cluster.local:8086"`
```

- [ ] **Step 2: Construct the client and pass into onboarding.NewService**

In main.go, where `onboarding.NewService(...)` is called, add:

```go
vendorClient := marketplaceapi.NewVendorClient(cfg.MarketplaceAPIURL)
onboardingSvc := onboarding.NewService(
    /* existing args */,
    vendorClient,
)
```

- [ ] **Step 3: Build**

```bash
go build ./cmd/server 2>&1 | tail -10
```

- [ ] **Step 4: Commit**

```bash
git add services/platform-api/cmd/server/main.go \
        services/platform-api/internal/config/config.go # only if you added the env var
git commit -m "feat(platform-api): wire vendor client into onboarding service"
```

---

## Task 10: Backfill CLI for existing tenants

**Files:**
- Create: `services/platform-api/cmd/backfill-vendors/main.go`

- [ ] **Step 1: Write the CLI**

```go
// services/platform-api/cmd/backfill-vendors/main.go
//
// One-shot tool that iterates every tenant in platform-api and ensures
// marketplace-api has a self-vendor with the correct name + slug. Fixes
// the placeholder values written by the 000027 backfill migration and
// backfills any tenants that had zero products when the migration ran.
//
// Usage:
//   go run ./cmd/backfill-vendors
//
// Env:
//   DATABASE_URL           — platform-api DB
//   MARKETPLACE_API_URL    — marketplace-api base URL
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/mark8ly/platform-api/internal/marketplaceapi"
	"github.com/mark8ly/platform-api/internal/store"
	"github.com/mark8ly/platform-api/internal/tenant"
)

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	apiURL := os.Getenv("MARKETPLACE_API_URL")
	if apiURL == "" {
		log.Fatal("MARKETPLACE_API_URL is required")
	}

	db, err := gorm.Open(postgres.Open(dbURL), &gorm.Config{})
	if err != nil {
		log.Fatalf("db open: %v", err)
	}

	vc := marketplaceapi.NewVendorClient(apiURL)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	var tenants []tenant.Tenant
	if err := db.WithContext(ctx).Find(&tenants).Error; err != nil {
		log.Fatalf("list tenants: %v", err)
	}

	ok, fail := 0, 0
	for _, t := range tenants {
		// Each tenant's slug comes from its default store.
		var s store.Store
		if err := db.WithContext(ctx).Where("tenant_id = ?", t.ID).Order("created_at ASC").First(&s).Error; err != nil {
			log.Printf("tenant %s: no store; skipping", t.ID)
			continue
		}
		if _, err := vc.EnsureSelfVendor(ctx, t.ID, t.Name, s.Slug); err != nil {
			log.Printf("tenant %s (%s): ensure failed: %v", t.ID, t.Name, err)
			fail++
			continue
		}
		ok++
	}
	fmt.Printf("backfill complete: %d ok, %d failed\n", ok, fail)
	if fail > 0 {
		os.Exit(1)
	}
}
```

Note: `EnsureSelfVendor` is idempotent only for *creation*. To actually overwrite placeholder names/slugs we need an additional endpoint. Punt to the next step.

- [ ] **Step 2: Build**

```bash
cd services/platform-api
go build ./cmd/backfill-vendors 2>&1 | tail -10
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add services/platform-api/cmd/backfill-vendors/main.go
git commit -m "feat(platform-api): add backfill-vendors CLI for existing tenants"
```

---

## Task 11: Add UpdateSelfVendor endpoint + wire into backfill CLI

Purpose: the 000027 migration writes placeholder name/slug; the backfill CLI must overwrite them.

**Files:**
- Modify: `services/marketplace-api/internal/vendor/handler.go` — add PATCH endpoint
- Modify: `services/marketplace-api/internal/vendor/service.go` — add UpdateSelfVendor method
- Modify: `services/marketplace-api/internal/vendor/service_test.go` — cover the update
- Modify: `services/platform-api/internal/marketplaceapi/vendor_client.go` — add UpdateSelfVendor method
- Modify: `services/platform-api/internal/marketplaceapi/vendor_client_test.go` — cover the update
- Modify: `services/platform-api/cmd/backfill-vendors/main.go` — call UpdateSelfVendor after Ensure

- [ ] **Step 1: TDD the service method**

```go
// service_test.go
func TestService_UpdateSelfVendor_OverwritesNameAndSlug(t *testing.T) {
	svc := newTestService(t)
	tenantID := "77777777-7777-7777-7777-777777777777"
	_, err := svc.EnsureSelfVendor(context.Background(), tenantID, "Merchant", "vendor-placeholder")
	require.NoError(t, err)

	updated, err := svc.UpdateSelfVendor(context.Background(), tenantID, "Acme", "acme")
	require.NoError(t, err)
	require.Equal(t, "Acme", updated.Name)
	require.Equal(t, "acme", updated.Slug)
}

func TestService_UpdateSelfVendor_NotFound(t *testing.T) {
	svc := newTestService(t)
	_, err := svc.UpdateSelfVendor(context.Background(), "unknown", "Acme", "acme")
	require.Error(t, err)
}
```

- [ ] **Step 2: Implement UpdateSelfVendor**

```go
// service.go
func (s *Service) UpdateSelfVendor(ctx context.Context, tenantID, name, slug string) (*Vendor, error) {
	v, err := s.repo.GetSelfByTenantID(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, apperrors.NotFound("not_found", "no self-vendor for tenant")
	}
	name = strings.TrimSpace(name)
	slug = strings.TrimSpace(slug)
	if name == "" || slug == "" {
		return nil, apperrors.BadRequest("invalid_input", "name and slug are required")
	}
	if err := s.repo.UpdateNameAndSlug(ctx, v.ID, name, slug); err != nil {
		return nil, err
	}
	v.Name = name
	v.Slug = slug
	return v, nil
}
```

- [ ] **Step 3: Add the PATCH route + handler**

```go
// handler.go
g.PATCH("/tenants/:tenantID/self-vendor", h.updateSelfVendor)

type updateSelfVendorRequest struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

func (h *Handler) updateSelfVendor(c *gin.Context) {
	var req updateSelfVendorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "message": "request body is not valid JSON"})
		return
	}
	v, err := h.svc.UpdateSelfVendor(c.Request.Context(), c.Param("tenantID"), req.Name, req.Slug)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": v})
}
```

- [ ] **Step 4: Add handler test — assert PATCH returns updated values**

Mirror `TestHandler_EnsureSelfVendor_Creates` but for the PATCH path.

- [ ] **Step 5: Add `UpdateSelfVendor` to platform-api client**

```go
// vendor_client.go
func (c *VendorClient) UpdateSelfVendor(ctx context.Context, tenantID, name, slug string) (*Vendor, error) {
	body, _ := json.Marshal(map[string]string{"name": name, "slug": slug})
	url := fmt.Sprintf("%s/internal/tenants/%s/self-vendor", c.baseURL, tenantID)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPatch, url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		raw, _ := io.ReadAll(res.Body)
		return nil, fmt.Errorf("marketplace-api update-self-vendor %d: %s", res.StatusCode, string(raw))
	}
	var resp struct { Data Vendor `json:"data"` }
	if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}
```

Add a matching httptest-based test.

- [ ] **Step 6: Call UpdateSelfVendor after Ensure in backfill-vendors/main.go**

Replace the `vc.EnsureSelfVendor(ctx, t.ID, t.Name, s.Slug)` call with:

```go
if _, err := vc.EnsureSelfVendor(ctx, t.ID, t.Name, s.Slug); err != nil {
    log.Printf("tenant %s: ensure failed: %v", t.ID, err)
    fail++
    continue
}
if _, err := vc.UpdateSelfVendor(ctx, t.ID, t.Name, s.Slug); err != nil {
    log.Printf("tenant %s: update failed: %v", t.ID, err)
    fail++
    continue
}
ok++
```

- [ ] **Step 7: Run all tests**

```bash
cd services/marketplace-api && go test ./internal/vendor/... -v
cd ../platform-api && go test ./internal/marketplaceapi/... -v
```

Expected: all pass.

- [ ] **Step 8: Commit**

```bash
git add services/marketplace-api/internal/vendor/ \
        services/platform-api/internal/marketplaceapi/ \
        services/platform-api/cmd/backfill-vendors/main.go
git commit -m "feat: add UpdateSelfVendor; backfill CLI now overwrites placeholder names/slugs"
```

---

## Task 12: Add NOT NULL constraint on products.vendor_id

**Files:**
- Create: `services/marketplace-api/migrations/000028_products_vendor_id_not_null.up.sql`
- Create: `services/marketplace-api/migrations/000028_products_vendor_id_not_null.down.sql`
- Modify: `services/marketplace-api/migrations.go`

- [ ] **Step 1: Write the migrations**

```sql
-- 000028_products_vendor_id_not_null.up.sql
--
-- Phase 1 final step: products.vendor_id is now populated for every
-- row (migration 000027). Lock it down.

-- Safety check: abort if any product is still missing a vendor_id.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM products WHERE vendor_id IS NULL) THEN
        RAISE EXCEPTION 'products rows with NULL vendor_id remain; run 000027 backfill first';
    END IF;
END $$;

ALTER TABLE products ALTER COLUMN vendor_id SET NOT NULL;
```

```sql
-- 000028_products_vendor_id_not_null.down.sql
ALTER TABLE products ALTER COLUMN vendor_id DROP NOT NULL;
```

- [ ] **Step 2: Register in `migrations.go`**

Mirror however 000027 was registered.

- [ ] **Step 3: Run migrations**

```bash
cd services/marketplace-api
go run ./cmd/migrate -direction up 2>&1 | tail -20
```

Expected: `migrated to 28` with no errors. If the safety check fires, **stop here** — it means 000027's backfill missed rows and we need to diagnose before locking the constraint.

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/migrations/000028_*.sql \
        services/marketplace-api/migrations.go
git commit -m "feat(marketplace-api): products.vendor_id NOT NULL"
```

---

## Task 13: Update product create path to default vendor_id to self-vendor

**Files:**
- Modify: `services/marketplace-api/internal/product/service.go` (or wherever Create lives)
- Modify: the same file's tests

- [ ] **Step 1: Read the current Create method**

```bash
grep -n "func.*Create\|VendorID" services/marketplace-api/internal/product/service.go | head -10
```

- [ ] **Step 2: Thread the vendor service into the product service**

Add `vendorSvc *vendor.Service` (or `interface{ GetSelfByTenantID(ctx, tid) (*Vendor, error) }`) as a dependency.

- [ ] **Step 3: Write a failing test**

```go
func TestProductService_Create_DefaultsVendorID(t *testing.T) {
	// Given a tenant with a self-vendor already seeded,
	// When Create is called without VendorID,
	// Then the created product has VendorID = self-vendor.ID.
}

func TestProductService_Create_RespectsExplicitVendorID(t *testing.T) {
	// If VendorID is supplied, it is used as-is (no override).
}
```

- [ ] **Step 4: Update Create to populate VendorID when missing**

```go
if in.VendorID == nil || *in.VendorID == "" {
    v, err := s.vendorSvc.repo.GetSelfByTenantID(ctx, in.TenantID) // or expose a getter
    if err != nil {
        return nil, err
    }
    if v == nil {
        return nil, apperrors.FailedPrecondition("no_self_vendor",
            "tenant has no self-vendor; run backfill-vendors")
    }
    id := v.ID
    in.VendorID = &id
}
```

- [ ] **Step 5: Run the product tests**

```bash
go test ./internal/product/... -v
```

- [ ] **Step 6: Commit**

```bash
git add services/marketplace-api/internal/product/service.go \
        services/marketplace-api/internal/product/service_test.go
git commit -m "feat(marketplace-api): product.Create defaults vendor_id to tenant self-vendor"
```

---

## Task 14: Integration smoke test

**Files:**
- Create: `services/marketplace-api/internal/vendor/smoke_integration_test.go`

- [ ] **Step 1: Write an end-to-end test that**
  - Spins up platform-api and marketplace-api with real DBs (or uses a single shared test harness if one exists)
  - Runs migrations
  - Calls `POST /onboarding/sessions/:id/complete` with fixture data to create a tenant
  - Hits marketplace-api's `GET /internal/tenants/:tid/self-vendor` and asserts it exists with the right name + slug
  - Creates a product via the product handler without a vendor_id and asserts the returned row carries the self-vendor id

If there's no existing integration harness, skip this task and file a follow-up ticket. Unit coverage + manual verification (Task 15) is acceptable for Phase 1.

- [ ] **Step 2: Commit**

```bash
git add services/marketplace-api/internal/vendor/smoke_integration_test.go
git commit -m "test: add integration smoke test for vendor foundation"
```

---

## Task 15: Deploy + manual verification

- [ ] **Step 1: Push to main (follows existing CI workflow)**

```bash
git push origin main
```

- [ ] **Step 2: Watch CI** (use the same repo-visibility-flip workaround if billing blocks)

- [ ] **Step 3: After tesserix-k8s PR merges and ArgoCD syncs, rollout**

```bash
kubectl -n argocd annotate application mark8ly-platform-api argocd.argoproj.io/refresh=hard --overwrite
kubectl -n argocd annotate application mark8ly-marketplace-api-admin argocd.argoproj.io/refresh=hard --overwrite
# and repeat the sync patch from recent rollouts
```

- [ ] **Step 4: Run the backfill CLI against prod**

```bash
kubectl -n mark8ly exec deploy/mark8ly-platform-api -- /backfill-vendors
```

(If the binary isn't baked into the image, run it via `kubectl run --rm -it --image=... --env ...` — check with the person who builds the service images.)

- [ ] **Step 5: Verify**

```bash
# From inside the cluster or via a debug pod:
curl http://mark8ly-marketplace-api-admin.mark8ly.svc.cluster.local:8086/internal/tenants/$TENANT_ID/self-vendor
# expect 200 with { "data": { ... "is_self": true, "name": "<real tenant name>" ... } }
```

Check at least one product in the admin — product detail should show a vendor id, and the DB should confirm NOT NULL holds.

---

## Done criteria for Phase 1

- [x] `vendors` table exists in marketplace-api with the Phase 1 schema + indexes
- [x] Every tenant (new and existing) has exactly one self-vendor row
- [x] `products.vendor_id` is NOT NULL, every product references the tenant's self-vendor
- [x] Onboarding automatically ensures a self-vendor for new tenants
- [x] `backfill-vendors` CLI has been run against prod and reports `0 failed`
- [x] No admin or storefront UI change is visible to end users
- [x] All existing tests still pass; new vendor tests pass; product tests updated

## What's next (not in this plan)

- **Phase 2** (`docs/superpowers/plans/...-phase-2-product-listing.md`): add `product_listings`, backfill 1:1 from `products`, dual-write for a release, then switch reads.
- **Phase 3**: remove price/stock columns from `products` once listings are authoritative.
