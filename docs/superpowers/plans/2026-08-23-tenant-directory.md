# Tenant Directory Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Expose mark8ly's tenant directory to the Tesserix platform console, replacing the cross-database read that serves its `tenant_names` enrichment today.

**Architecture:** `platform-api` gains two internal endpoints that own the query — search, filters, pagination, and a store rollup in one grouped query. `marketplace-api` calls them through a new `tenantdirectory` client and reshapes the result onto the `platformadmin` surface, which already carries signature verification, replay defence and the pinned envelope.

**Tech Stack:** Go 1.26, Gin, GORM, PostgreSQL 15, testify.

**Spec:** `docs/superpowers/specs/2026-08-23-tenant-directory-design.md`

## Global Constraints

- Envelope is exactly `{"data": [...], "pagination": {"page": N, "limit": N, "total": N}}`. Reuse `platformadmin`'s existing `pagination` type — do not declare a second one.
- Empty results are `200` with `[]`. Never `null`, never `{}`. Allocate with `make([]T, 0, n)` before appending.
- Timestamps are RFC3339, UTC, with offset.
- Ids go out **bare** — no `mark8ly:` prefix. The platform API namespaces on arrival.
- Never send a `source` field.
- `pagination.limit` reports the **effective** (clamped) limit, so `total / limit` is a correct page count.
- Oversized `limit` clamps, never errors. A missing parameter takes the default.
- Status values come from the existing `tenant.Status*` constants. Do not invent a second vocabulary.
- **No caller-scoping.** This is the platform view, not "tenants I belong to". Do not touch `listMyTenants`.
- Commit messages: single line, conventional-commit prefix, no signature, no `Co-Authored-By` trailer.

## Two Conventions That Differ Between The Services

Read these before writing any test:

- **`platform-api` integration tests use the INTERNAL package** — `package tenant`, not `tenant_test`. See `internal/tenant/repository_integration_test.go`. They import `github.com/mark8ly/platform-api/pkg/testdb`.
- **`marketplace-api` tests use the EXTERNAL package** — `package platformadmin_test`. They import `github.com/mark8ly/marketplace-api/pkg/testdb`.

Both use `//go:build integration` and `*_integration_test.go`.

## Environment

- Local docker Postgres is running. **Use the LAN IP, not localhost** — a native Postgres squats on 127.0.0.1 on this machine, so `localhost` reaches the wrong server and reports `role "dev" does not exist`:
  - marketplace-api: `TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable'`
  - platform-api: `TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/platform_api?sslmode=disable'`
  The committed Makefile says `localhost`, which is correct everywhere else — **do not change it**.
- Integration runs need `-p 1`. These packages share one local Postgres and parallel package execution exhausts its connection limit (`FATAL: sorry, too many clients already`), which looks like data pollution and is not.
- Do NOT run `make dev` — the migrate containers are broken on this machine.
- Never run anything against a remote or GKE database. The only cluster that exists is production.

## File Structure

**`platform-api`**

| File | Responsibility |
|---|---|
| `internal/tenant/directory.go` (create) | `DirectoryFilter`, `DirectoryResult`, `TenantWithStores`, and the two repository queries |
| `internal/tenant/directory_integration_test.go` (create) | Search, filters, pagination, rollup-without-N+1, no caller-scoping |
| `internal/middleware/internal_auth.go` (modify) | `RequireInternalAuthStrict` — no empty-secret escape hatch |
| `internal/middleware/internal_auth_test.go` (create) | The strict guard's three states |
| `internal/tenant/handler.go` (modify) | `RegisterDirectory(g *gin.RouterGroup)` plus the two handlers |
| `internal/tenant/directory_handler_test.go` (create) | Query parsing, clamping, envelope |
| `cmd/server/main.go` (modify) | Mount the directory group behind the strict guard |

**`marketplace-api`**

| File | Responsibility |
|---|---|
| `internal/tenantdirectory/client.go` (create) | Transport and error mapping only |
| `internal/tenantdirectory/client_test.go` (create) | Happy path, 404, 5xx, transport failure |
| `internal/handlers/platformadmin/entities_tenants.go` (create) | Wire shape and nothing else |
| `internal/handlers/platformadmin/entities_tenants_test.go` (create) | Golden fixture, empty case, clamping, upstream failure |
| `internal/handlers/platformadmin/testdata/entities_tenants_response.json` (create) | The contract, as bytes |
| `internal/handlers/platformadmin/routes.go` (modify) | Mount both routes |
| `pkg/config/config.go` (modify) | `PlatformAPIURL` if not already present — check first |
| `cmd/marketplace-api/main.go` (modify) | Construct the client, pass it in `platformadmin.Deps` |

---

### Task 1: platform-api — the directory queries

**Files:**
- Create: `services/platform-api/internal/tenant/directory.go`
- Create: `services/platform-api/internal/tenant/directory_integration_test.go`

**Interfaces:**
- Consumes: `tenant.Tenant` (`internal/tenant/models.go:25`), `store.Store` (`internal/store/models.go:26`)
- Produces: `tenant.DirectoryFilter`, `tenant.DirectoryResult`, `tenant.TenantWithStores`, `tenant.StoreSummary`, and two methods on `Repository`: `ListDirectory(ctx, DirectoryFilter) (DirectoryResult, error)` and `GetWithStores(ctx, id string) (*TenantWithStores, error)`. Tasks 3 and 4 depend on these exact names.

- [ ] **Step 1: Write the failing test**

Create `directory_integration_test.go`. Note `package tenant` — the internal package, matching `repository_integration_test.go`.

```go
//go:build integration

package tenant

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/platform-api/pkg/testdb"
)

// seedTenantWithStore inserts a tenant and one store, returning the tenant id.
func seedTenantWithStore(t *testing.T, db *gorm.DB, name, ownerEmail, status, slug string) string {
	t.Helper()
	tenantID := uuid.NewString()
	require.NoError(t, db.Exec(
		`INSERT INTO tenants (id, name, owner_user_id, owner_email, status)
		 VALUES (?, ?, ?, ?, ?)`,
		tenantID, name, "uid-"+tenantID[:8], ownerEmail, status,
	).Error)
	if slug != "" {
		require.NoError(t, db.Exec(
			`INSERT INTO stores (id, tenant_id, slug, name, country_code, currency_code, timezone, status)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			uuid.NewString(), tenantID, slug, name+" Store", "IE", "EUR", "Europe/Dublin", "active",
		).Error)
	}
	return tenantID
}

// The case a literal reading of #277 would have missed: the tenant has no
// slug of its own, so it must be findable by its STORE's slug.
func TestListDirectory_MatchesStoreSlug(t *testing.T) {
	db := testdb.NewTx(t)
	repo := NewRepository(db)

	id := seedTenantWithStore(t, db, "Unrelated Name", "unrelated@example.com", StatusActive, "findme-slug")

	got, err := repo.ListDirectory(context.Background(), DirectoryFilter{Q: "findme", Limit: 50})
	require.NoError(t, err)
	require.Equal(t, int64(1), got.Total)
	require.Len(t, got.Tenants, 1)
	require.Equal(t, id, got.Tenants[0].ID)
}

func TestListDirectory_MatchesNameAndOwnerEmail(t *testing.T) {
	db := testdb.NewTx(t)
	repo := NewRepository(db)

	byName := seedTenantWithStore(t, db, "Acme Trading", "someone@example.com", StatusActive, "acme-store")
	byEmail := seedTenantWithStore(t, db, "Other Co", "founder@distinctive.test", StatusActive, "other-store")

	byNameRes, err := repo.ListDirectory(context.Background(), DirectoryFilter{Q: "acme", Limit: 50})
	require.NoError(t, err)
	require.Equal(t, int64(1), byNameRes.Total)
	require.Equal(t, byName, byNameRes.Tenants[0].ID)

	byEmailRes, err := repo.ListDirectory(context.Background(), DirectoryFilter{Q: "distinctive", Limit: 50})
	require.NoError(t, err)
	require.Equal(t, int64(1), byEmailRes.Total)
	require.Equal(t, byEmail, byEmailRes.Tenants[0].ID)
}

// A tenant with two stores must appear ONCE, not once per store. The join
// makes duplicate rows the default failure mode.
func TestListDirectory_DeduplicatesAcrossStores(t *testing.T) {
	db := testdb.NewTx(t)
	repo := NewRepository(db)

	id := seedTenantWithStore(t, db, "Multi Store Co", "multi@example.com", StatusActive, "multi-one")
	require.NoError(t, db.Exec(
		`INSERT INTO stores (id, tenant_id, slug, name, country_code, currency_code, timezone, status)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		uuid.NewString(), id, "multi-two", "Second", "IE", "EUR", "Europe/Dublin", "active",
	).Error)

	got, err := repo.ListDirectory(context.Background(), DirectoryFilter{Q: "multi", Limit: 50})
	require.NoError(t, err)
	require.Equal(t, int64(1), got.Total, "a tenant with two stores must count once")
	require.Len(t, got.Tenants, 1)
}

func TestListDirectory_FiltersByStatusAndCreatedRange(t *testing.T) {
	db := testdb.NewTx(t)
	repo := NewRepository(db)

	active := seedTenantWithStore(t, db, "Active Co", "a@example.com", StatusActive, "active-slug")
	seedTenantWithStore(t, db, "Suspended Co", "s@example.com", StatusSuspended, "susp-slug")

	res, err := repo.ListDirectory(context.Background(), DirectoryFilter{Status: StatusActive, Limit: 50})
	require.NoError(t, err)
	require.Equal(t, int64(1), res.Total)
	require.Equal(t, active, res.Tenants[0].ID)

	future, err := repo.ListDirectory(context.Background(), DirectoryFilter{
		CreatedFrom: time.Now().Add(24 * time.Hour), Limit: 50,
	})
	require.NoError(t, err)
	require.Equal(t, int64(0), future.Total)
	require.Empty(t, future.Tenants)
}

// total is the UNPAGINATED count. A total that equals the page size makes
// the console's page arithmetic silently wrong.
func TestListDirectory_TotalIsUnpaginated(t *testing.T) {
	db := testdb.NewTx(t)
	repo := NewRepository(db)

	for i := 0; i < 3; i++ {
		seedTenantWithStore(t, db, "Paged Co", "p@example.com", StatusActive, "paged-"+uuid.NewString()[:8])
	}

	got, err := repo.ListDirectory(context.Background(), DirectoryFilter{Q: "Paged Co", Limit: 1})
	require.NoError(t, err)
	require.Equal(t, int64(3), got.Total)
	require.Len(t, got.Tenants, 1)
}

// The platform view returns tenants regardless of who is asking. There is no
// caller identity in DirectoryFilter at all, which is the point — but assert
// it, because a later "helpful" scoping change would break the console
// silently: it would just see fewer tenants, not an error.
func TestListDirectory_IsNotCallerScoped(t *testing.T) {
	db := testdb.NewTx(t)
	repo := NewRepository(db)

	a := seedTenantWithStore(t, db, "Owner A Co", "owner-a@example.com", StatusActive, "scope-a")
	b := seedTenantWithStore(t, db, "Owner B Co", "owner-b@example.com", StatusActive, "scope-b")

	got, err := repo.ListDirectory(context.Background(), DirectoryFilter{Q: "Owner", Limit: 50})
	require.NoError(t, err)
	require.Equal(t, int64(2), got.Total, "both tenants must appear regardless of ownership")

	ids := map[string]bool{}
	for _, tn := range got.Tenants {
		ids[tn.ID] = true
	}
	require.True(t, ids[a])
	require.True(t, ids[b])
}

func TestListDirectory_ClampsLimit(t *testing.T) {
	db := testdb.NewTx(t)
	repo := NewRepository(db)

	got, err := repo.ListDirectory(context.Background(), DirectoryFilter{Limit: 100000})
	require.NoError(t, err)
	require.LessOrEqual(t, len(got.Tenants), MaxDirectoryPageSize)
	require.NotNil(t, got.Tenants, "must be an allocated slice, never nil")
}

func TestGetWithStores_ReturnsRollup(t *testing.T) {
	db := testdb.NewTx(t)
	repo := NewRepository(db)

	id := seedTenantWithStore(t, db, "Rollup Co", "r@example.com", StatusActive, "rollup-one")
	require.NoError(t, db.Exec(
		`INSERT INTO stores (id, tenant_id, slug, name, country_code, currency_code, timezone, status)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		uuid.NewString(), id, "rollup-two", "Second", "IE", "EUR", "Europe/Dublin", "suspended",
	).Error)

	got, err := repo.GetWithStores(context.Background(), id)
	require.NoError(t, err)
	require.Equal(t, id, got.ID)
	require.Len(t, got.Stores, 2)

	statuses := map[string]string{}
	for _, s := range got.Stores {
		statuses[s.Slug] = s.Status
	}
	require.Equal(t, "active", statuses["rollup-one"])
	require.Equal(t, "suspended", statuses["rollup-two"])
}

func TestGetWithStores_TenantWithNoStores(t *testing.T) {
	db := testdb.NewTx(t)
	repo := NewRepository(db)

	id := seedTenantWithStore(t, db, "Storeless Co", "n@example.com", StatusActive, "")

	got, err := repo.GetWithStores(context.Background(), id)
	require.NoError(t, err)
	require.NotNil(t, got.Stores, "must be an allocated slice, never nil")
	require.Empty(t, got.Stores)
}

func TestGetWithStores_NotFound(t *testing.T) {
	db := testdb.NewTx(t)
	repo := NewRepository(db)

	_, err := repo.GetWithStores(context.Background(), uuid.NewString())
	require.Error(t, err)
}
```

Add the imports the helper needs: `"github.com/google/uuid"` and `"gorm.io/gorm"`.

- [ ] **Step 2: Run it to verify it fails**

Run:
```bash
cd services/platform-api && \
  TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/platform_api?sslmode=disable' \
  go test -tags=integration ./internal/tenant/... -run TestListDirectory -count=1
```
Expected: FAIL — `DirectoryFilter` undefined.

- [ ] **Step 3: Implement**

Create `internal/tenant/directory.go`:

```go
package tenant

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// DefaultDirectoryPageSize applies when the caller sends no limit. The
// contract says a missing parameter takes our default and is never an error.
const DefaultDirectoryPageSize = 50

// MaxDirectoryPageSize caps a page. Mirrors the ceiling the marketplace-api
// side applies, so a caller cannot force an unbounded scan here either.
const MaxDirectoryPageSize = 500

// DirectoryFilter narrows the platform-wide tenant directory. Every field is
// optional — this is the platform operator's view, not a caller-scoped one.
type DirectoryFilter struct {
	// Q matches tenants.name, tenants.owner_email, and any of the tenant's
	// stores.slug. Tenants have no slug of their own (see models.go): the
	// URL identity moved to store.slug, and a tenant with multiple stores
	// has multiple slugs.
	Q           string
	Status      string
	CreatedFrom time.Time
	CreatedTo   time.Time
	Page        int
	Limit       int
}

// StoreSummary is the per-store rollup entry on a tenant detail.
type StoreSummary struct {
	ID     string `json:"id"`
	Slug   string `json:"slug"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

// TenantWithStores is a tenant plus its store rollup.
type TenantWithStores struct {
	Tenant
	Stores []StoreSummary `json:"stores"`
}

// DirectoryResult is a page of tenants plus the unpaginated total.
type DirectoryResult struct {
	Tenants []Tenant
	Total   int64
}

// applyDirectoryFilter builds the WHERE clause shared by the count and the
// page query, so the two can never drift apart.
func applyDirectoryFilter(q *gorm.DB, f DirectoryFilter) *gorm.DB {
	if f.Q != "" {
		like := "%" + f.Q + "%"
		// EXISTS rather than a JOIN: a tenant with two matching stores must
		// appear once, and EXISTS says that directly instead of relying on a
		// DISTINCT that also has to be threaded through the count.
		q = q.Where(
			`tenants.name ILIKE ? OR tenants.owner_email ILIKE ?
			 OR EXISTS (SELECT 1 FROM stores s WHERE s.tenant_id = tenants.id AND s.slug ILIKE ?)`,
			like, like, like,
		)
	}
	if f.Status != "" {
		q = q.Where("tenants.status = ?", f.Status)
	}
	if !f.CreatedFrom.IsZero() {
		q = q.Where("tenants.created_at >= ?", f.CreatedFrom)
	}
	if !f.CreatedTo.IsZero() {
		q = q.Where("tenants.created_at <= ?", f.CreatedTo)
	}
	return q
}

func (r *gormRepository) ListDirectory(ctx context.Context, f DirectoryFilter) (DirectoryResult, error) {
	var result DirectoryResult

	countQ := applyDirectoryFilter(r.db.WithContext(ctx).Model(&Tenant{}), f)
	if err := countQ.Count(&result.Total).Error; err != nil {
		return result, fmt.Errorf("tenant directory count: %w", err)
	}

	page := max(f.Page, 1)
	limit := f.Limit
	switch {
	case limit <= 0:
		limit = DefaultDirectoryPageSize
	case limit > MaxDirectoryPageSize:
		limit = MaxDirectoryPageSize
	}

	// Allocate before Find: a nil slice marshals to {} downstream, which
	// defeats a caller's `?? []`.
	result.Tenants = make([]Tenant, 0, limit)

	pageQ := applyDirectoryFilter(r.db.WithContext(ctx).Model(&Tenant{}), f)
	if err := pageQ.
		Order("tenants.created_at DESC").
		Offset((page - 1) * limit).
		Limit(limit).
		Find(&result.Tenants).Error; err != nil {
		return result, fmt.Errorf("tenant directory list: %w", err)
	}
	return result, nil
}

func (r *gormRepository) GetWithStores(ctx context.Context, id string) (*TenantWithStores, error) {
	t, err := r.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	out := &TenantWithStores{Tenant: *t, Stores: make([]StoreSummary, 0)}

	// One query for every store, not one per store — #277 asks for the
	// detail "without a round trip per store".
	if err := r.db.WithContext(ctx).
		Table("stores").
		Select("id, slug, name, status").
		Where("tenant_id = ?", id).
		Order("slug ASC").
		Scan(&out.Stores).Error; err != nil {
		return nil, fmt.Errorf("tenant store rollup: %w", err)
	}
	return out, nil
}
```

- [ ] **Step 4: Add both methods to the Repository interface**

In `internal/tenant/repository.go`, inside `type Repository interface`:

```go
	// ListDirectory returns a page of the platform-wide tenant directory.
	// NOT caller-scoped — this is the platform operator's view. See #277.
	ListDirectory(ctx context.Context, f DirectoryFilter) (DirectoryResult, error)
	// GetWithStores returns a tenant plus its store rollup in one query.
	GetWithStores(ctx context.Context, id string) (*TenantWithStores, error)
```

Run `go build ./...` and add the two methods to any fake or mock the compiler flags.

- [ ] **Step 5: Run the tests**

Run:
```bash
cd services/platform-api && \
  TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/platform_api?sslmode=disable' \
  go test -tags=integration ./internal/tenant/... -count=1
```
Expected: PASS, including the pre-existing tenant tests.

- [ ] **Step 6: Commit**

```bash
git add services/platform-api/internal/tenant/
git commit -m "feat(tenant): platform-wide directory queries with store rollup"
```

---

### Task 2: platform-api — the fail-closed guard

**Files:**
- Modify: `services/platform-api/internal/middleware/internal_auth.go`
- Create: `services/platform-api/internal/middleware/internal_auth_test.go`

**Interfaces:**
- Produces: `middleware.RequireInternalAuthStrict(secret string) gin.HandlerFunc`. Task 3's registration and Task 6's wiring depend on this name.

**Why this is its own task.** `RequireInternalAuth` no-ops when its secret is empty, which is right for the existing `/internal` routes — `/internal/tenants/{id}/members` needs a tenant id the caller already has, so an unconfigured deploy leaks little. The directory returns **every tenant on the platform, unscoped**. An unconfigured deploy would serve the whole thing unauthenticated.

- [ ] **Step 1: Write the failing test**

```go
package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/platform-api/internal/middleware"
)

func strictRouter(secret string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/x", middleware.RequireInternalAuthStrict(secret), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	return r
}

// The whole point of this middleware. RequireInternalAuth no-ops on an empty
// secret; this one must not, because the route it guards returns every
// tenant on the platform.
func TestStrictRefusesWhenSecretUnset(t *testing.T) {
	rec := httptest.NewRecorder()
	strictRouter("").ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.Contains(t, rec.Body.String(), "not_configured")
}

func TestStrictRefusesWrongSecret(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("X-Internal-Auth", "wrong")

	rec := httptest.NewRecorder()
	strictRouter("right").ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestStrictRefusesMissingHeader(t *testing.T) {
	rec := httptest.NewRecorder()
	strictRouter("right").ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))

	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestStrictAllowsCorrectSecret(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("X-Internal-Auth", "right")

	rec := httptest.NewRecorder()
	strictRouter("right").ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
}

// The permissive variant must keep its escape hatch — other /internal routes
// rely on it during the cutover before the secret is provisioned.
func TestPermissiveStillNoOpsOnEmptySecret(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/y", middleware.RequireInternalAuth(""), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/y", nil))

	require.Equal(t, http.StatusOK, rec.Code)
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `cd services/platform-api && go test ./internal/middleware/... -run TestStrict -v`
Expected: FAIL — `RequireInternalAuthStrict` undefined.

- [ ] **Step 3: Implement**

Append to `internal/middleware/internal_auth.go`:

```go
// RequireInternalAuthStrict is RequireInternalAuth without the empty-secret
// escape hatch: an unset secret refuses every request with 503 rather than
// letting it through.
//
// Use this for internal routes whose response is not already scoped by
// something the caller had to know. RequireInternalAuth's permissive branch
// is fine for /internal/tenants/{id}/members — a caller needs a tenant id to
// ask. It is not fine for the tenant DIRECTORY (#277), which returns every
// tenant on the platform, unscoped: an unconfigured deploy would serve the
// whole thing to anyone who reached the pod.
//
// Deliberately a second function rather than a flag on the first. The
// existing routes' behaviour must not change, and a boolean at every call
// site is easy to get backwards.
func RequireInternalAuthStrict(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if secret == "" {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
				"error":   "not_configured",
				"message": "internal auth secret is not configured",
			})
			return
		}
		if !constantTimeEqual(c.GetHeader("X-Internal-Auth"), secret) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error":   "unauthorized",
				"message": "internal auth required",
			})
			return
		}
		c.Next()
	}
}
```

`constantTimeEqual` already exists in this file — reuse it, do not duplicate.

- [ ] **Step 4: Run the tests**

Run: `cd services/platform-api && go test ./internal/middleware/... -count=1 -v`
Expected: PASS, all five.

- [ ] **Step 5: Commit**

```bash
git add services/platform-api/internal/middleware/
git commit -m "feat(middleware): fail-closed internal auth for unscoped routes"
```

---

### Task 3: platform-api — the directory endpoints

**Files:**
- Modify: `services/platform-api/internal/tenant/handler.go`
- Create: `services/platform-api/internal/tenant/directory_handler_test.go`

**Interfaces:**
- Consumes: `DirectoryFilter`, `DirectoryResult`, `TenantWithStores`, `ListDirectory`, `GetWithStores` (Task 1)
- Produces: `(*Handler).RegisterDirectory(g *gin.RouterGroup)`, serving `GET /tenants` and `GET /tenants/:id/detail` on the supplied group. Task 6 mounts it; Task 4's client calls it.

**Path note:** the detail route is `/tenants/:id/detail`, NOT `/tenants/:id`. The existing internal group already registers `GET /tenants/:id` (`handler.go:65`) returning a bare tenant, and the admin BFF depends on it. A second handler on the same path is a route conflict; a different shape on the same path is a silent breaking change for that caller.

- [ ] **Step 1: Write the failing test**

```go
package tenant

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// stubDirRepo records the filter it received and returns a canned result.
type stubDirRepo struct {
	Repository
	got    DirectoryFilter
	result DirectoryResult
	detail *TenantWithStores
	err    error
}

func (s *stubDirRepo) ListDirectory(_ context.Context, f DirectoryFilter) (DirectoryResult, error) {
	s.got = f
	if s.result.Tenants == nil {
		s.result.Tenants = []Tenant{}
	}
	return s.result, s.err
}

func (s *stubDirRepo) GetWithStores(_ context.Context, _ string) (*TenantWithStores, error) {
	return s.detail, s.err
}

func dirRouter(t *testing.T, repo Repository) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	NewHandler(NewService(repo)).RegisterDirectory(r.Group(""))
	return r
}

func TestDirectoryList_ParsesFilters(t *testing.T) {
	repo := &stubDirRepo{}
	rec := httptest.NewRecorder()
	dirRouter(t, repo).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/tenants?q=acme&status=active&page=2&limit=25", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "acme", repo.got.Q)
	require.Equal(t, "active", repo.got.Status)
	require.Equal(t, 2, repo.got.Page)
	require.Equal(t, 25, repo.got.Limit)
}

// A missing parameter takes the default and is never an error; an oversized
// one clamps rather than being refused.
func TestDirectoryList_DefaultsAndClamps(t *testing.T) {
	repo := &stubDirRepo{}
	rec := httptest.NewRecorder()
	dirRouter(t, repo).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/tenants", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, DefaultDirectoryPageSize, repo.got.Limit)

	repo2 := &stubDirRepo{}
	rec2 := httptest.NewRecorder()
	dirRouter(t, repo2).ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/tenants?limit=100000", nil))
	require.Equal(t, http.StatusOK, rec2.Code)
	require.Equal(t, MaxDirectoryPageSize, repo2.got.Limit)
}

func TestDirectoryList_GarbageParamsDoNotError(t *testing.T) {
	repo := &stubDirRepo{}
	rec := httptest.NewRecorder()
	dirRouter(t, repo).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/tenants?limit=abc&page=-4&created_from=notadate", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, DefaultDirectoryPageSize, repo.got.Limit)
	require.Equal(t, 1, repo.got.Page)
}

func TestDirectoryList_EmptyIsArrayNotNull(t *testing.T) {
	repo := &stubDirRepo{result: DirectoryResult{Tenants: nil, Total: 0}}
	rec := httptest.NewRecorder()
	dirRouter(t, repo).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/tenants", nil))

	var body struct {
		Data json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "[]", string(body.Data))
}
```

Add imports `"context"` and whatever `NewHandler`/`NewService` need — read `handler.go` for their real constructors and match them.

- [ ] **Step 2: Run it to verify it fails**

Run: `cd services/platform-api && go test ./internal/tenant/... -run TestDirectory -v`
Expected: FAIL — `RegisterDirectory` undefined.

- [ ] **Step 3: Implement**

In `internal/tenant/handler.go`:

```go
// RegisterDirectory mounts the platform-wide tenant directory (#277) on the
// supplied group. The CALLER is responsible for gating it — main.go wraps it
// in middleware.RequireInternalAuthStrict, because these routes return every
// tenant on the platform and must not be reachable on an unconfigured deploy.
//
// Deliberately separate from Register: those routes are caller-scoped or
// id-scoped and keep the permissive auth branch.
func (h *Handler) RegisterDirectory(g *gin.RouterGroup) {
	t := g.Group("/tenants")
	{
		t.GET("", h.listDirectory)
		// /detail, not /:id — the internal group already serves GET
		// /tenants/:id with a different shape, and the admin BFF calls it.
		t.GET("/:id/detail", h.getTenantDetail)
	}
}

func (h *Handler) listDirectory(c *gin.Context) {
	f := parseDirectoryFilter(c)

	res, err := h.svc.ListDirectory(c.Request.Context(), f)
	if err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": res.Tenants,
		"pagination": gin.H{
			"page":  max(f.Page, 1),
			"limit": f.Limit,
			"total": res.Total,
		},
	})
}

func (h *Handler) getTenantDetail(c *gin.Context) {
	t, err := h.svc.GetWithStores(c.Request.Context(), c.Param("id"))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": t})
}

// parseDirectoryFilter never returns an error. A missing or malformed
// parameter takes the default; an oversized limit clamps. The console is
// entitled to ask for too much, and a ceiling here is the backstop.
func parseDirectoryFilter(c *gin.Context) DirectoryFilter {
	f := DirectoryFilter{
		Q:      strings.TrimSpace(c.Query("q")),
		Status: strings.TrimSpace(c.Query("status")),
		Page:   1,
		Limit:  DefaultDirectoryPageSize,
	}
	if v := strings.TrimSpace(c.Query("page")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			f.Page = n
		}
	}
	if v := strings.TrimSpace(c.Query("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			f.Limit = min(n, MaxDirectoryPageSize)
		}
	}
	if t, ok := parseRFC3339(c.Query("created_from")); ok {
		f.CreatedFrom = t
	}
	if t, ok := parseRFC3339(c.Query("created_to")); ok {
		f.CreatedTo = t
	}
	return f
}

func parseRFC3339(v string) (time.Time, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}
```

Add `ListDirectory` and `GetWithStores` passthroughs to the service in `internal/tenant/service.go`, matching how `GetByID` is already delegated. Add `strconv`, `strings` and `time` imports as needed.

- [ ] **Step 4: Run the tests**

Run: `cd services/platform-api && go test ./internal/tenant/... -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add services/platform-api/internal/tenant/
git commit -m "feat(tenant): internal directory endpoints"
```

---

### Task 4: marketplace-api — the client

**Files:**
- Create: `services/marketplace-api/internal/tenantdirectory/client.go`
- Create: `services/marketplace-api/internal/tenantdirectory/client_test.go`

**Interfaces:**
- Produces: `tenantdirectory.Client`, `NewClient(baseURL, secret string, httpClient *http.Client) *Client`, `ErrUnavailable`, `ErrNotFound`, the `Tenant` / `StoreSummary` / `ListResult` types, and two methods:
  - `List(ctx context.Context, p ListParams) (*ListResult, error)`
  - `Get(ctx context.Context, id string) (*TenantDetail, error)`

  Task 5 depends on these exact names.

A separate package from `teamproxy` on purpose: that package's stated job is team membership, and a directory read is a different concern that will diverge.

- [ ] **Step 1: Write the failing test**

```go
package tenantdirectory_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/tenantdirectory"
)

func TestListSendsAuthAndParsesEnvelope(t *testing.T) {
	var gotAuth, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("X-Internal-Auth")
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"t1","name":"Acme","owner_email":"a@example.com","status":"active","created_at":"2026-08-22T10:00:00Z"}],"pagination":{"page":1,"limit":50,"total":1}}`))
	}))
	defer srv.Close()

	c := tenantdirectory.NewClient(srv.URL, "s3cret", srv.Client())
	got, err := c.List(context.Background(), tenantdirectory.ListParams{Q: "acme", Limit: 50})
	require.NoError(t, err)

	require.Equal(t, "s3cret", gotAuth)
	require.Contains(t, gotQuery, "q=acme")
	require.Equal(t, int64(1), got.Total)
	require.Len(t, got.Tenants, 1)
	require.Equal(t, "Acme", got.Tenants[0].Name)
}

// An empty upstream page must arrive as an allocated slice, so the handler
// cannot marshal nil.
func TestListEmptyIsAllocated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[],"pagination":{"page":1,"limit":50,"total":0}}`))
	}))
	defer srv.Close()

	got, err := tenantdirectory.NewClient(srv.URL, "s", srv.Client()).
		List(context.Background(), tenantdirectory.ListParams{})
	require.NoError(t, err)
	require.NotNil(t, got.Tenants)
	require.Empty(t, got.Tenants)
}

func TestGetParsesStoreRollup(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"id":"t1","name":"Acme","owner_email":"a@example.com","status":"active","created_at":"2026-08-22T10:00:00Z","stores":[{"id":"s1","slug":"acme","name":"Acme Store","status":"active"}]}}`))
	}))
	defer srv.Close()

	got, err := tenantdirectory.NewClient(srv.URL, "s", srv.Client()).
		Get(context.Background(), "t1")
	require.NoError(t, err)
	require.Len(t, got.Stores, 1)
	require.Equal(t, "acme", got.Stores[0].Slug)
}

func TestGetNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not_found","message":"no such tenant"}`))
	}))
	defer srv.Close()

	_, err := tenantdirectory.NewClient(srv.URL, "s", srv.Client()).Get(context.Background(), "missing")
	require.ErrorIs(t, err, tenantdirectory.ErrNotFound)
}

// A 5xx upstream must NOT look like an empty result — that is the failure the
// handler turns into 503 rather than a misleading 200 with no tenants.
func TestUpstream5xxIsUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	_, err := tenantdirectory.NewClient(srv.URL, "s", srv.Client()).
		List(context.Background(), tenantdirectory.ListParams{})
	require.ErrorIs(t, err, tenantdirectory.ErrUnavailable)
}

func TestTransportFailureIsUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	srv.Close() // closed: connection refused

	_, err := tenantdirectory.NewClient(srv.URL, "s", srv.Client()).
		List(context.Background(), tenantdirectory.ListParams{})
	require.ErrorIs(t, err, tenantdirectory.ErrUnavailable)
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `cd services/marketplace-api && go test ./internal/tenantdirectory/... -v`
Expected: FAIL — package does not exist.

- [ ] **Step 3: Implement**

Create `internal/tenantdirectory/client.go`:

```go
// Package tenantdirectory is a marketplace-api client for platform-api's
// internal tenant-directory endpoints (#277).
//
// Separate from internal/teamproxy on purpose: that package's job is team
// membership for a known tenant. A platform-wide directory read is a
// different concern with a different shape, and the two will diverge.
package tenantdirectory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// ErrUnavailable signals platform-api could not be reached, or answered 5xx.
// Callers MUST NOT treat this as an empty result: an empty directory and an
// unreachable one are different answers, and conflating them shows a console
// operator "no tenants" when the truth is "we could not ask".
var ErrUnavailable = errors.New("tenantdirectory: platform-api unavailable")

// ErrNotFound signals platform-api returned 404 for a tenant id.
var ErrNotFound = errors.New("tenantdirectory: tenant not found")

// maxBody caps what we will read from platform-api.
const maxBody = 4 << 20

// Tenant is the directory row.
type Tenant struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	OwnerEmail string    `json:"owner_email"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
}

// StoreSummary is one entry in a tenant's store rollup.
type StoreSummary struct {
	ID     string `json:"id"`
	Slug   string `json:"slug"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

// TenantDetail is a tenant plus its store rollup.
type TenantDetail struct {
	Tenant
	Stores []StoreSummary `json:"stores"`
}

// ListResult is a page plus the unpaginated total.
type ListResult struct {
	Tenants []Tenant
	Total   int64
	Page    int
	Limit   int
}

// ListParams narrows a directory query. Zero values are omitted.
type ListParams struct {
	Q           string
	Status      string
	CreatedFrom time.Time
	CreatedTo   time.Time
	Page        int
	Limit       int
}

// Client calls platform-api's internal tenant-directory endpoints.
type Client struct {
	baseURL string
	secret  string
	http    *http.Client
}

// NewClient constructs a Client. httpClient may be nil (defaults to a
// 5-second timeout). The secret is sent as X-Internal-Auth when non-empty.
func NewClient(baseURL, secret string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 5 * time.Second}
	}
	return &Client{baseURL: baseURL, secret: secret, http: httpClient}
}

func (c *Client) do(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("tenantdirectory: build request: %w", err)
	}
	if c.secret != "" {
		req.Header.Set("X-Internal-Auth", c.secret)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusNotFound:
		return ErrNotFound
	case resp.StatusCode >= 500:
		return fmt.Errorf("%w: upstream %d", ErrUnavailable, resp.StatusCode)
	case resp.StatusCode != http.StatusOK:
		return fmt.Errorf("tenantdirectory: platform-api %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return fmt.Errorf("%w: read body: %v", ErrUnavailable, err)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("tenantdirectory: decode: %w", err)
	}
	return nil
}

// List fetches a page of the tenant directory.
func (c *Client) List(ctx context.Context, p ListParams) (*ListResult, error) {
	q := url.Values{}
	if p.Q != "" {
		q.Set("q", p.Q)
	}
	if p.Status != "" {
		q.Set("status", p.Status)
	}
	if !p.CreatedFrom.IsZero() {
		q.Set("created_from", p.CreatedFrom.UTC().Format(time.RFC3339))
	}
	if !p.CreatedTo.IsZero() {
		q.Set("created_to", p.CreatedTo.UTC().Format(time.RFC3339))
	}
	if p.Page > 0 {
		q.Set("page", strconv.Itoa(p.Page))
	}
	if p.Limit > 0 {
		q.Set("limit", strconv.Itoa(p.Limit))
	}

	path := "/internal/tenants"
	if encoded := q.Encode(); encoded != "" {
		path += "?" + encoded
	}

	var envelope struct {
		Data       []Tenant `json:"data"`
		Pagination struct {
			Page  int   `json:"page"`
			Limit int   `json:"limit"`
			Total int64 `json:"total"`
		} `json:"pagination"`
	}
	if err := c.do(ctx, path, &envelope); err != nil {
		return nil, err
	}

	// Allocate: upstream `"data": null` would otherwise become a nil slice
	// that the handler marshals as {} instead of [].
	tenants := envelope.Data
	if tenants == nil {
		tenants = []Tenant{}
	}

	return &ListResult{
		Tenants: tenants,
		Total:   envelope.Pagination.Total,
		Page:    envelope.Pagination.Page,
		Limit:   envelope.Pagination.Limit,
	}, nil
}

// Get fetches one tenant with its store rollup.
func (c *Client) Get(ctx context.Context, id string) (*TenantDetail, error) {
	var envelope struct {
		Data TenantDetail `json:"data"`
	}
	if err := c.do(ctx, "/internal/tenants/"+url.PathEscape(id)+"/detail", &envelope); err != nil {
		return nil, err
	}
	if envelope.Data.Stores == nil {
		envelope.Data.Stores = []StoreSummary{}
	}
	return &envelope.Data, nil
}
```

- [ ] **Step 4: Run the tests**

Run: `cd services/marketplace-api && go test ./internal/tenantdirectory/... -count=1 -v`
Expected: PASS, all six.

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/tenantdirectory/
git commit -m "feat(tenantdirectory): client for platform-api's tenant directory"
```

---

### Task 5: marketplace-api — the platformadmin handlers

**Files:**
- Create: `services/marketplace-api/internal/handlers/platformadmin/entities_tenants.go`
- Create: `services/marketplace-api/internal/handlers/platformadmin/entities_tenants_test.go`
- Create: `services/marketplace-api/internal/handlers/platformadmin/testdata/entities_tenants_response.json`

**Interfaces:**
- Consumes: `tenantdirectory.Client`, `ListParams`, `ListResult`, `TenantDetail`, `ErrUnavailable`, `ErrNotFound` (Task 4); the existing `pagination` type in `platformadmin` (`audit_logs.go`)
- Produces: `platformadmin.TenantDirectory` (interface), `NewEntitiesTenantsHandler(dir TenantDirectory, logger *slog.Logger) *EntitiesTenantsHandler`, `(*EntitiesTenantsHandler).Register(g *gin.RouterGroup)`. Task 6 constructs it.

**Name collisions:** `middleware_test.go` and `audit_logs_test.go` already define `testSecret`, `fixedNow`, `memNonces`, `newMemNonces`, `newRouter`, `reqOpt`, `withoutOperator`, `withoutCapability`, `signedRequest`, `errorCode`, and `stubRepo` in `package platformadmin_test`. Do not redeclare any of them.

- [ ] **Step 1: Write the golden fixture**

`testdata/entities_tenants_response.json`:

```json
{
  "data": [
    {
      "id": "3f2504e0-4f89-11d3-9a0c-0305e82c3301",
      "name": "Acme Trading",
      "owner_email": "founder@acme.example",
      "status": "active",
      "created_at": "2026-08-22T10:00:00Z"
    },
    {
      "id": "3f2504e0-4f89-11d3-9a0c-0305e82c3302",
      "name": "Beta Goods",
      "owner_email": "owner@beta.example",
      "status": "suspended",
      "created_at": "2026-08-21T09:30:00Z"
    }
  ],
  "pagination": {
    "page": 1,
    "limit": 50,
    "total": 2
  }
}
```

- [ ] **Step 2: Write the failing tests**

```go
package platformadmin_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
	"github.com/mark8ly/marketplace-api/internal/tenantdirectory"
)

// stubDirectory records params and returns canned results.
type stubDirectory struct {
	gotParams tenantdirectory.ListParams
	list      *tenantdirectory.ListResult
	detail    *tenantdirectory.TenantDetail
	err       error
}

func (s *stubDirectory) List(_ context.Context, p tenantdirectory.ListParams) (*tenantdirectory.ListResult, error) {
	s.gotParams = p
	if s.err != nil {
		return nil, s.err
	}
	if s.list == nil {
		s.list = &tenantdirectory.ListResult{Tenants: []tenantdirectory.Tenant{}}
	}
	return s.list, nil
}

func (s *stubDirectory) Get(_ context.Context, _ string) (*tenantdirectory.TenantDetail, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.detail, nil
}

func tenantsRouter(t *testing.T, dir platformadmin.TenantDirectory) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	platformadmin.NewEntitiesTenantsHandler(dir, nil).Register(r.Group(""))
	return r
}

// THE test. Real handler output compared to the committed contract.
func TestEntitiesTenantsMatchesContract(t *testing.T) {
	dir := &stubDirectory{list: &tenantdirectory.ListResult{
		Total: 2, Page: 1, Limit: 50,
		Tenants: []tenantdirectory.Tenant{
			{
				ID: "3f2504e0-4f89-11d3-9a0c-0305e82c3301", Name: "Acme Trading",
				OwnerEmail: "founder@acme.example", Status: "active",
				CreatedAt: time.Date(2026, 8, 22, 10, 0, 0, 0, time.UTC),
			},
			{
				ID: "3f2504e0-4f89-11d3-9a0c-0305e82c3302", Name: "Beta Goods",
				OwnerEmail: "owner@beta.example", Status: "suspended",
				CreatedAt: time.Date(2026, 8, 21, 9, 30, 0, 0, time.UTC),
			},
		},
	}}

	rec := httptest.NewRecorder()
	tenantsRouter(t, dir).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/entities/tenants", nil))

	require.Equal(t, http.StatusOK, rec.Code)

	want, err := os.ReadFile("testdata/entities_tenants_response.json")
	require.NoError(t, err)
	require.JSONEq(t, string(want), rec.Body.String())
}

func TestEntitiesTenantsEmptyIsArray(t *testing.T) {
	rec := httptest.NewRecorder()
	tenantsRouter(t, &stubDirectory{}).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/entities/tenants", nil))

	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Data json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "[]", string(body.Data))
}

func TestEntitiesTenantsPassesFilters(t *testing.T) {
	dir := &stubDirectory{}
	rec := httptest.NewRecorder()
	tenantsRouter(t, dir).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/entities/tenants?q=acme&status=active&limit=25&page=2", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "acme", dir.gotParams.Q)
	require.Equal(t, "active", dir.gotParams.Status)
	require.Equal(t, 25, dir.gotParams.Limit)
	require.Equal(t, 2, dir.gotParams.Page)
}

// An unreachable upstream must NOT look like an empty directory. A 200 with
// no tenants would have a console operator conclude none exist.
func TestEntitiesTenantsUpstreamDownIs503(t *testing.T) {
	rec := httptest.NewRecorder()
	tenantsRouter(t, &stubDirectory{err: tenantdirectory.ErrUnavailable}).
		ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/entities/tenants", nil))

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.Equal(t, "upstream_unavailable", errorCode(t, rec))
}

func TestEntitiesTenantDetailNotFoundIs404(t *testing.T) {
	rec := httptest.NewRecorder()
	tenantsRouter(t, &stubDirectory{err: tenantdirectory.ErrNotFound}).
		ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/entities/tenants/missing", nil))

	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Equal(t, "not_found", errorCode(t, rec))
}

func TestEntitiesTenantDetailReturnsRollup(t *testing.T) {
	dir := &stubDirectory{detail: &tenantdirectory.TenantDetail{
		Tenant: tenantdirectory.Tenant{
			ID: "t1", Name: "Acme", OwnerEmail: "a@example.com", Status: "active",
			CreatedAt: time.Date(2026, 8, 22, 10, 0, 0, 0, time.UTC),
		},
		Stores: []tenantdirectory.StoreSummary{
			{ID: "s1", Slug: "acme", Name: "Acme Store", Status: "active"},
		},
	}}

	rec := httptest.NewRecorder()
	tenantsRouter(t, dir).ServeHTTP(rec, httptest.NewRequest(
		http.MethodGet, "/admin/entities/tenants/t1", nil))

	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Data struct {
			ID         string `json:"id"`
			StoreCount int    `json:"store_count"`
			Stores     []struct {
				Slug string `json:"slug"`
			} `json:"stores"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "t1", body.Data.ID)
	require.Equal(t, 1, body.Data.StoreCount)
	require.Len(t, body.Data.Stores, 1)
	require.Equal(t, "acme", body.Data.Stores[0].Slug)
}
```

- [ ] **Step 3: Run to verify it fails**

Run: `cd services/marketplace-api && go test ./internal/handlers/platformadmin/ -run TestEntitiesTenants -v`
Expected: FAIL — `NewEntitiesTenantsHandler` undefined.

- [ ] **Step 4: Implement**

Create `entities_tenants.go`:

```go
package platformadmin

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/internal/tenantdirectory"
)

// TenantDirectory is the subset of tenantdirectory.Client this handler needs.
// Declared here so the handler can be tested with a stub and so the transport
// package stays swappable.
type TenantDirectory interface {
	List(ctx context.Context, p tenantdirectory.ListParams) (*tenantdirectory.ListResult, error)
	Get(ctx context.Context, id string) (*tenantdirectory.TenantDetail, error)
}

// EntitiesTenantsHandler serves the platform console's tenant directory
// (#277). It owns the wire shape and nothing else — search, filtering and
// pagination all happen in platform-api, which owns the data.
type EntitiesTenantsHandler struct {
	dir    TenantDirectory
	logger *slog.Logger
}

// NewEntitiesTenantsHandler constructs the handler. logger may be nil.
func NewEntitiesTenantsHandler(dir TenantDirectory, logger *slog.Logger) *EntitiesTenantsHandler {
	return &EntitiesTenantsHandler{dir: dir, logger: logger}
}

// Register mounts both routes on the supplied group.
func (h *EntitiesTenantsHandler) Register(g *gin.RouterGroup) {
	g.GET("/admin/entities/tenants", h.list)
	g.GET("/admin/entities/tenants/:id", h.detail)
}

// tenantRow is the directory wire shape.
type tenantRow struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	OwnerEmail string `json:"owner_email"`
	Status     string `json:"status"`
	CreatedAt  string `json:"created_at"`
}

type tenantListResponse struct {
	Data       []tenantRow `json:"data"`
	Pagination pagination  `json:"pagination"`
}

type tenantStore struct {
	ID     string `json:"id"`
	Slug   string `json:"slug"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

type tenantDetailRow struct {
	tenantRow
	StoreCount int           `json:"store_count"`
	Stores     []tenantStore `json:"stores"`
}

func (h *EntitiesTenantsHandler) list(c *gin.Context) {
	p := parseTenantParams(c)

	res, err := h.dir.List(c.Request.Context(), p)
	if err != nil {
		h.respondErr(c, err)
		return
	}

	// Allocate before appending: a nil slice marshals to {}, which defeats a
	// caller's `?? []` and crashes their page precisely when there is no data.
	rows := make([]tenantRow, 0, len(res.Tenants))
	for _, t := range res.Tenants {
		rows = append(rows, toTenantRow(t))
	}

	c.JSON(http.StatusOK, tenantListResponse{
		Data: rows,
		Pagination: pagination{
			Page:  max(res.Page, 1),
			Limit: res.Limit,
			Total: res.Total,
		},
	})
}

func (h *EntitiesTenantsHandler) detail(c *gin.Context) {
	t, err := h.dir.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.respondErr(c, err)
		return
	}

	stores := make([]tenantStore, 0, len(t.Stores))
	for _, s := range t.Stores {
		stores = append(stores, tenantStore{ID: s.ID, Slug: s.Slug, Name: s.Name, Status: s.Status})
	}

	c.JSON(http.StatusOK, gin.H{"data": tenantDetailRow{
		tenantRow:  toTenantRow(t.Tenant),
		StoreCount: len(stores),
		Stores:     stores,
	}})
}

func toTenantRow(t tenantdirectory.Tenant) tenantRow {
	return tenantRow{
		// Bare id. The platform API namespaces as <slug>:<id> on arrival;
		// prefixing here yields "mark8ly:mark8ly:...".
		ID:         t.ID,
		Name:       t.Name,
		OwnerEmail: t.OwnerEmail,
		Status:     t.Status,
		CreatedAt:  t.CreatedAt.UTC().Format(time.RFC3339),
	}
}

// respondErr maps client errors to the surface's stable codes.
//
// ErrUnavailable becomes 503, never an empty 200: an empty directory and an
// unreachable one are different answers, and a console operator shown "no
// tenants" would believe the first.
func (h *EntitiesTenantsHandler) respondErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, tenantdirectory.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{
			"error": "not_found", "message": "tenant not found",
		})
	case errors.Is(err, tenantdirectory.ErrUnavailable):
		if h.logger != nil {
			h.logger.Error("tenant directory upstream unavailable", "err", err)
		}
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "upstream_unavailable", "message": "tenant directory is unavailable",
		})
	default:
		if h.logger != nil {
			h.logger.Error("tenant directory", "err", err)
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "internal_error", "message": "could not read the tenant directory",
		})
	}
}

// parseTenantParams never errors. A missing parameter takes platform-api's
// default; an oversized limit is clamped there.
func parseTenantParams(c *gin.Context) tenantdirectory.ListParams {
	p := tenantdirectory.ListParams{
		Q:      strings.TrimSpace(c.Query("q")),
		Status: strings.TrimSpace(c.Query("status")),
	}
	if v := strings.TrimSpace(c.Query("page")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			p.Page = n
		}
	}
	if v := strings.TrimSpace(c.Query("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			p.Limit = n
		}
	}
	if t, ok := parseTime(c.Query("created_from")); ok {
		p.CreatedFrom = t
	}
	if t, ok := parseTime(c.Query("created_to")); ok {
		p.CreatedTo = t
	}
	return p
}
```

`parseTime` and `pagination` already exist in `audit_logs.go` in this package — reuse them, do not redeclare.

- [ ] **Step 5: Run the tests**

Run: `cd services/marketplace-api && go test ./internal/handlers/platformadmin/... -count=1`
Expected: PASS, including the existing audit-logs and golden-vectors tests.

- [ ] **Step 6: Verify the golden test actually bites**

Rename the `owner_email` JSON tag on `tenantRow` to `ownerEmail`, re-run `TestEntitiesTenantsMatchesContract`, and confirm it FAILS. Then add a field (e.g. `Source string \`json:"source"\``), re-run, confirm it FAILS again. Restore both with `git checkout --`.

A golden test that catches only omissions is not a contract test.

- [ ] **Step 7: Commit**

```bash
git add services/marketplace-api/internal/handlers/platformadmin/
git commit -m "feat(platformadmin): tenant directory endpoints"
```

---

### Task 6: Wire both services

**Files:**
- Modify: `services/platform-api/cmd/server/main.go:340`
- Modify: `services/marketplace-api/pkg/config/config.go`
- Modify: `services/marketplace-api/internal/handlers/platformadmin/routes.go`
- Modify: `services/marketplace-api/cmd/marketplace-api/main.go`
- Create: `services/marketplace-api/internal/handlers/platformadmin/routes_tenants_test.go`

**Interfaces:**
- Consumes: everything from Tasks 1-5.

- [ ] **Step 1: Mount the directory group in platform-api**

In `cmd/server/main.go`, after the existing `internal := r.Group(...)` at line 340:

```go
	// The tenant directory (#277) returns EVERY tenant on the platform,
	// unscoped, so it gets the fail-closed guard rather than the permissive
	// one above: an unconfigured deploy must refuse, not serve the lot.
	// Deliberately a different group with different middleware — see
	// middleware.RequireInternalAuthStrict.
	tenantDirectory := r.Group("/internal", middleware.RequireInternalAuthStrict(cfg.InternalAuthSecret))
	tenantHandler.RegisterDirectory(tenantDirectory)
```

- [ ] **Step 2: Verify platform-api still builds and serves**

Run: `cd services/platform-api && go build ./... && go vet ./... && go test ./... -count=1`
Expected: clean.

- [ ] **Step 3: Add the config field in marketplace-api**

Check whether `PlatformAPIURL` already exists in `pkg/config/config.go` (`grep -n "PlatformAPI" pkg/config/config.go`). `MARKETPLACE_PLATFORM_API_URL` and `MARKETPLACE_PLATFORM_API_SECRET` are already set on the deployment, so the fields likely exist — reuse them rather than adding duplicates. Only add what is genuinely missing.

- [ ] **Step 4: Add the handler to Deps and Register**

In `internal/handlers/platformadmin/routes.go`, add to `Deps`:

```go
	// TenantDirectory serves /admin/entities/tenants (#277). Nil leaves those
	// routes unmounted, matching the nil-safe pattern used for Repo above.
	TenantDirectory TenantDirectory
```

and in `Register`, inside the authenticated group:

```go
	if deps.TenantDirectory != nil {
		NewEntitiesTenantsHandler(deps.TenantDirectory, deps.Logger).Register(group)
	}
```

- [ ] **Step 5: Write the wiring test**

```go
package platformadmin_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
)

// A nil TenantDirectory must leave the routes unmounted rather than panic at
// request time — matching the nil-safe pattern for Repo.
func TestRegisterTenantDirectoryIsNilSafe(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	platformadmin.Register(r.Group("/api/v1/platform"), platformadmin.Deps{
		Repo: &stubRepo{},
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
		"/api/v1/platform/admin/entities/tenants", nil))
	require.Equal(t, http.StatusNotFound, rec.Code)
}

// Mounted but with no secret: 503 not_configured, same as every other route
// on this surface.
func TestTenantDirectoryFailsClosedWithoutSecret(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	platformadmin.Register(r.Group("/api/v1/platform"), platformadmin.Deps{
		Repo:            &stubRepo{},
		TenantDirectory: &stubDirectory{},
		Secret:          "",
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
		"/api/v1/platform/admin/entities/tenants", nil))
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.Equal(t, "not_configured", errorCode(t, rec))
}
```

- [ ] **Step 6: Construct the client in main.go**

In `cmd/marketplace-api/main.go`, at BOTH `platformadmin.Register` call sites (the `mode.Both` branch and the `mode.Admin` branch — find them with `grep -n "platformadmin.Register" cmd/marketplace-api/main.go`), add to the `Deps` literal:

```go
			TenantDirectory: tenantdirectory.NewClient(
				cfg.PlatformAPIURL, cfg.PlatformAPISecret, nil),
```

Both sites are required. `mode.Both` is the merged local-dev engine; `mode.Admin` is production. Missing one means the routes silently do not exist in that mode, and no test catches it.

Add the import.

- [ ] **Step 7: Run everything**

Run:
```bash
cd services/marketplace-api && go build ./... && go vet ./... && go test ./... -count=1
cd ../platform-api && go build ./... && go vet ./... && go test ./... -count=1
```
Expected: clean both.

Run the integration suites:
```bash
cd services/marketplace-api && \
  TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 ./internal/audit/... ./internal/handlers/platformadmin/... ./internal/tenantpurge/... ./internal/subscription/dunning/... -count=1

cd ../platform-api && \
  TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/platform_api?sslmode=disable' \
  go test -tags=integration -p 1 ./internal/tenant/... ./internal/middleware/... -count=1
```
Expected: clean.

- [ ] **Step 8: Confirm both mount points**

Run: `grep -n "TenantDirectory:" cmd/marketplace-api/main.go`
Expected: exactly two matches.

- [ ] **Step 9: Commit**

```bash
git add services/platform-api/cmd/ services/marketplace-api/
git commit -m "feat(platformadmin): wire the tenant directory into both engines"
```

---

## After the plan

1. **Comment on #277** recording the slug decision — `q` matches name, owner email and store slugs, because tenants have no slug of their own. The console sends `q`; what it matches is ours, but they should not discover it by surprise.
2. **Comment on #278** with the finding that its central requirement (no customer rows under any filter) is unenforceable against `user_profiles`, which carries no role or type. The staff/customer split lives in GIP tenant pools.
3. **Verify in production** once deployed: a signed request to `/api/v1/platform/admin/entities/tenants` returns the envelope, and stopping platform-api yields `503 upstream_unavailable` rather than an empty `200`.
4. **Widen `make test-int`** to include `./internal/tenantdirectory/...` — it is a unit-test-only package, so it needs no database, but the platform-api stanza should gain `./internal/tenant/...` and `./internal/middleware/...`.
