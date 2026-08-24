# Tenant Suspend / Unsuspend Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `POST /admin/tenants/{id}/suspend` and `/unsuspend` (#287), where "suspended" actually stops the tenant trading rather than setting a column nothing reads.

**Architecture:** Enforcement rides on **store** status, which the storefront already enforces and which marketplace-api already projects. Tenant status is the operator's record of decision and the cascade's source. A new `stores.suspended_by_tenant` flag makes unsuspend non-lossy. Four enforcement points: storefront (already works), `StoreMiddleware` for store-scoped admin API routes, a new tenant gate in marketplace-api for **all** admin routes, and the admin UI middleware. **`auth-bff` is not touched** — see T7 for why the spec's original approach there was replaced.

**Tech Stack:** Go 1.26 (platform-api, marketplace-api), Gin, GORM, Postgres, Next.js middleware (TypeScript) in `apps/admin`.

**Spec:** `docs/superpowers/specs/2026-08-24-tenant-suspend-design.md`

## Global Constraints

- **Two services, two migration schemes.** platform-api uses **4 digits** (latest `0014_unique_tenant_owner_email`, so this is `0015_stores_suspended_by_tenant`); marketplace-api uses **6** (`000001…`). Never copy one into the other — that confusion is trap 4.
- **Two services, two `stores` tables.** platform-api's is the source of truth and FKs to `countries`/`currencies`/`timezones` (`GB`/`GBP`/`Europe/London` are seeded; `IE`/`Europe/Dublin` are not). marketplace-api's is a *local projection* with plain columns and **no** reference-data FKs. Say which one you mean, every time.
- Suspend sets `active → suspended` only, and records `suspended_by_tenant = true` **only for rows it changed**. Unsuspend sets `suspended → active` **only where `suspended_by_tenant = true`**, then clears the flag.
- Boundary rule wherever an age is compared: degraded/stale when `age >= window`, i.e. `ts <= asOf.Add(-window)`. Fixtures sit ON the boundary, offset by **1 millisecond** — `timestamptz` is microsecond-resolution, so a nanosecond offset truncates and both fixtures become the same row.
- Every check that takes a clock takes `asOf` from the caller and compares against it in SQL — never Postgres `now()`.
- **A cached `suspended` is authoritative regardless of age**; a stale `active` may still be served. Asymmetric on purpose.
- Audit on both operations via `platformadmin.EmitOperatorAction(c, emitter, tenantID, ev)` — the tenant is a required parameter because nothing on this surface sets `tenant_id` on the context and `audit.Emit` would silently write no row (trap 3, #310).
- Envelope `{"data": {...}}`, no `pagination`. Ids bare. Timestamps RFC3339 UTC.
- Commits: single-line conventional, **no signature**, no `Co-Authored-By`.
- Integration tests: `//go:build integration`, `-p 1`, LAN IP DSN (never `localhost`). `testdb.NewDB` **skips silently** when its env var is unset — confirm from verbose output that tests RAN. Also run **`go vet -tags=integration ./...`**: the default toolchain never compiles tagged files (trap 8).
- Pre-existing unrelated failures in marketplace-api's `internal/subscription` and `internal/billing/trial` (#316/#317). Not yours; scope with `-run`.
- Ignore the pre-existing `go.work requires go >= 1.26.6` diagnostic.

## Task dependency shape

T1 → T2 → T3 (platform-api, strictly ordered) → T4 → T5 (marketplace-api).
**T6, T7 and T8 are independent of each other** and each depends only on T5's semantics being settled — they may be done in any order, or in parallel by separate agents.

---

### Task 1: platform-api — the `suspended_by_tenant` column

**Files:**
- Create: `services/platform-api/migrations/0015_stores_suspended_by_tenant.up.sql`
- Create: `services/platform-api/migrations/0015_stores_suspended_by_tenant.down.sql`
- Modify: `services/platform-api/internal/store/models.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `stores.suspended_by_tenant` column and `Store.SuspendedByTenant bool`, read by T2.

- [ ] **Step 1: Write the migration**

`0015_stores_suspended_by_tenant.up.sql`:

```sql
-- 0015_stores_suspended_by_tenant.up.sql
-- Records WHO suspended a store, so unsuspending a tenant does not
-- reactivate a store that was suspended individually beforehand (#287).
-- Only rows changed by a tenant-level suspension carry true.
ALTER TABLE stores
    ADD COLUMN suspended_by_tenant BOOLEAN NOT NULL DEFAULT false;

-- Partial index: the unsuspend path selects exactly these rows.
CREATE INDEX stores_suspended_by_tenant_idx
    ON stores (tenant_id)
    WHERE suspended_by_tenant;
```

`0015_stores_suspended_by_tenant.down.sql`:

```sql
DROP INDEX IF EXISTS stores_suspended_by_tenant_idx;
ALTER TABLE stores DROP COLUMN IF EXISTS suspended_by_tenant;
```

- [ ] **Step 2: Add the model field**

In `internal/store/models.go`, on the `Store` struct, following the column-tag style already used there:

```go
	// SuspendedByTenant is true only for stores a TENANT-level suspension
	// changed. Unsuspend restores exactly these rows, so a store suspended
	// individually before the tenant was suspended stays suspended (#287).
	SuspendedByTenant bool `gorm:"column:suspended_by_tenant;not null;default:false" json:"suspended_by_tenant"`
```

- [ ] **Step 3: Apply the migration and confirm the column exists**

```bash
cd services/platform-api && DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/platform_api?sslmode=disable' go run ./cmd/migrate up
```

Then confirm — do not assume the migration ran:

```bash
cd services/platform-api && go run ./cmd/migrate version
```

Expected: version `15`. If `cmd/migrate` has no `version` subcommand, verify by querying the column instead and say in your report which you did.

- [ ] **Step 4: Build**

```bash
cd services/platform-api && go build ./... && go vet ./...
```
Expected: clean.

- [ ] **Step 5: Commit**

```bash
git add services/platform-api/migrations/0015_* services/platform-api/internal/store/models.go
git commit -m "feat(platform-api): add stores.suspended_by_tenant for reversible tenant suspension (#287)"
```

---

### Task 2: platform-api — repository suspend/unsuspend with cascade

**Files:**
- Modify: `services/platform-api/internal/tenant/repository.go`
- Test: `services/platform-api/internal/tenant/suspend_integration_test.go` (create)

**Interfaces:**
- Consumes: T1's column.
- Produces, called by T3:

```go
// SuspendResult reports what a suspend/unsuspend actually changed.
type SuspendResult struct {
	Status         string // the tenant's status AFTER the call
	StoresAffected int
	Changed        bool   // false when the tenant was already in the target state
}

func (r *gormRepository) Suspend(ctx context.Context, tenantID string) (*SuspendResult, error)
func (r *gormRepository) Unsuspend(ctx context.Context, tenantID string) (*SuspendResult, error)
```

Add both to the `Repository` interface in the same file.

**Note on test package:** platform-api integration tests use the **internal** package (`package tenant`), unlike marketplace-api which uses external. Follow the local convention — check a neighbouring `*_integration_test.go` in this service before writing yours.

- [ ] **Step 1: Write the failing reversibility test**

This is the test the naive implementation fails. Create `suspend_integration_test.go`:

```go
//go:build integration

package tenant

// TestSuspendThenUnsuspendPreservesIndividuallySuspendedStore is the whole
// reason suspended_by_tenant exists. A store suspended on its own, before
// the tenant was suspended, must still be suspended after the tenant is
// unsuspended. A cascade that just sets everything back to active fails
// exactly here and nowhere else.
func TestSuspendThenUnsuspendPreservesIndividuallySuspendedStore(t *testing.T) {
	db := newTestDB(t) // follow the local helper convention
	repo := NewRepository(db)
	ctx := context.Background()

	tenantID := seedTenant(t, db, StatusActive)
	activeStore := seedStore(t, db, tenantID, "active")
	alreadySuspended := seedStore(t, db, tenantID, "suspended")

	// Suspend the tenant.
	res, err := repo.Suspend(ctx, tenantID)
	require.NoError(t, err)
	require.True(t, res.Changed)
	require.Equal(t, StatusSuspended, res.Status)
	require.Equal(t, 1, res.StoresAffected, "only the ACTIVE store should be affected")

	// Unsuspend it.
	res, err = repo.Unsuspend(ctx, tenantID)
	require.NoError(t, err)
	require.True(t, res.Changed)
	require.Equal(t, 1, res.StoresAffected)

	require.Equal(t, "active", storeStatus(t, db, activeStore),
		"the store the cascade suspended must be restored")
	require.Equal(t, "suspended", storeStatus(t, db, alreadySuspended),
		"a store suspended individually BEFORE the tenant suspension must stay suspended")
	require.False(t, suspendedByTenant(t, db, activeStore), "flag must be cleared after unsuspend")
}

// A second suspend is a no-op: no extra stores affected, Changed false, and
// the flag on already-cascaded rows is untouched.
func TestSuspendIsIdempotent(t *testing.T) {
	db := newTestDB(t)
	repo := NewRepository(db)
	ctx := context.Background()

	tenantID := seedTenant(t, db, StatusActive)
	s := seedStore(t, db, tenantID, "active")

	first, err := repo.Suspend(ctx, tenantID)
	require.NoError(t, err)
	require.True(t, first.Changed)
	require.Equal(t, 1, first.StoresAffected)

	second, err := repo.Suspend(ctx, tenantID)
	require.NoError(t, err)
	require.False(t, second.Changed, "already suspended: no-op")
	require.Equal(t, 0, second.StoresAffected)
	require.True(t, suspendedByTenant(t, db, s), "flag must survive a repeat suspend")
}
```

Write `seedTenant`, `seedStore`, `storeStatus` and `suspendedByTenant` helpers in the same file. **`stores` in platform-api FKs to the reference tables** — seed with `GB`/`GBP`/`Europe/London`, which are in the seed set; `IE`/`Europe/Dublin` are not and will fail.

- [ ] **Step 2: Run to verify it fails**

```bash
cd services/platform-api && TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/platform_api?sslmode=disable' \
  go test -tags integration -p 1 ./internal/tenant/ -run 'TestSuspend' -v
```
Expected: FAIL — `repo.Suspend` undefined. Confirm from the output that it **ran** rather than skipped; check what env var the local helper reads and use that one.

- [ ] **Step 3: Implement both methods**

In `internal/tenant/repository.go`. Both run in one transaction so the tenant row and its stores can never disagree:

```go
func (r *gormRepository) Suspend(ctx context.Context, tenantID string) (*SuspendResult, error) {
	out := &SuspendResult{}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var t Tenant
		if err := tx.Raw(
			`SELECT * FROM tenants WHERE id = ? FOR UPDATE`, tenantID).Scan(&t).Error; err != nil {
			return err
		}
		if t.ID == "" {
			return ErrNotFound
		}
		out.Status = t.Status
		if t.Status == StatusSuspended {
			return nil // no-op: Changed stays false, StoresAffected 0
		}
		if t.Status != StatusActive {
			// archived, or anything added later: refuse rather than guess.
			return fmt.Errorf("%w: tenant status %q cannot be suspended", ErrConflict, t.Status)
		}

		// Cascade to ACTIVE stores only, flagging exactly what we changed.
		res := tx.Exec(`
			UPDATE stores SET status = 'suspended', suspended_by_tenant = true
			WHERE tenant_id = ? AND status = 'active'`, tenantID)
		if res.Error != nil {
			return res.Error
		}
		out.StoresAffected = int(res.RowsAffected)

		if err := tx.Exec(`UPDATE tenants SET status = ? WHERE id = ?`,
			StatusSuspended, tenantID).Error; err != nil {
			return err
		}
		out.Status = StatusSuspended
		out.Changed = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *gormRepository) Unsuspend(ctx context.Context, tenantID string) (*SuspendResult, error) {
	out := &SuspendResult{}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var t Tenant
		if err := tx.Raw(
			`SELECT * FROM tenants WHERE id = ? FOR UPDATE`, tenantID).Scan(&t).Error; err != nil {
			return err
		}
		if t.ID == "" {
			return ErrNotFound
		}
		out.Status = t.Status
		if t.Status != StatusSuspended {
			return nil // no-op
		}

		// Restore ONLY what the cascade suspended, then clear the flag.
		res := tx.Exec(`
			UPDATE stores SET status = 'active', suspended_by_tenant = false
			WHERE tenant_id = ? AND suspended_by_tenant`, tenantID)
		if res.Error != nil {
			return res.Error
		}
		out.StoresAffected = int(res.RowsAffected)

		if err := tx.Exec(`UPDATE tenants SET status = ? WHERE id = ?`,
			StatusActive, tenantID).Error; err != nil {
			return err
		}
		out.Status = StatusActive
		out.Changed = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
```

If `ErrNotFound`/`ErrConflict` are named differently in this package, use the local names — check `internal/tenant/errors.go` or equivalent rather than inventing sentinels.

- [ ] **Step 4: Run the tests**

Same command as Step 2. Expected: PASS, and confirm they RAN.

- [ ] **Step 5: Mutation — prove the reversibility test discriminates**

Change the unsuspend cascade's `WHERE tenant_id = ? AND suspended_by_tenant` to `WHERE tenant_id = ?` — the naive version. Re-run.
Expected: **FAIL** in `TestSuspendThenUnsuspendPreservesIndividuallySuspendedStore`, because the individually-suspended store comes back active. Record the test name and the actual failure text. Revert.

If it still passes, the fixture does not contain an individually-suspended store and the test proves nothing — fix it before continuing.

- [ ] **Step 6: Commit**

```bash
git add services/platform-api/internal/tenant/
git commit -m "feat(platform-api): reversible tenant suspend/unsuspend with store cascade (#287)"
```

---

### Task 3: platform-api — the two `strictInternal` endpoints

**Files:**
- Modify: `services/platform-api/internal/tenant/handler.go`
- Modify: `services/platform-api/cmd/server/main.go` (mount)
- Test: `services/platform-api/internal/tenant/suspend_handler_test.go` (create)

**Interfaces:**
- Consumes: `Repository.Suspend`/`Unsuspend` and `SuspendResult` from T2.
- Produces, called by T4:

```
POST /internal/tenants/:id/suspend    → 200 {"data":{"tenant_id","status","stores_affected","changed"}}
POST /internal/tenants/:id/unsuspend  → 200, same shape
```

`404` unknown tenant, `409` when the status cannot make the transition (e.g. `archived`).

**Why `strictInternal` and not the existing PATCH:** `updateTenantRequest` is `{Name, UID}` and runs `fga.Check(uid, "can_edit_settings", tenantID)` — a *merchant* authorization model a platform operator has no relation in, and it has no status field. Widening it would push a privileged field through a merchant-authorized path. `strictInternal` (`cmd/server/main.go:353`, `RequireInternalAuthStrict`) is the fail-closed group; the permissive variant would serve the whole estate on an unconfigured deploy.

- [ ] **Step 1: Write the failing handler tests**

Create `suspend_handler_test.go` (follow the local handler-test convention — check an existing handler test in this package for how it builds a router and stubs the repository):

```go
func TestSuspendHandler_ReturnsChangedAndCount(t *testing.T) {
	h := newTestHandler(&stubRepo{suspend: &SuspendResult{
		Status: StatusSuspended, StoresAffected: 2, Changed: true,
	}})
	rec := doPost(t, h, "/internal/tenants/"+testTenantID+"/suspend")

	require.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Data struct {
			TenantID       string `json:"tenant_id"`
			Status         string `json:"status"`
			StoresAffected int    `json:"stores_affected"`
			Changed        bool   `json:"changed"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, testTenantID, body.Data.TenantID)
	require.Equal(t, "suspended", body.Data.Status)
	require.Equal(t, 2, body.Data.StoresAffected)
	require.True(t, body.Data.Changed)
}

// A no-op must be 200 with changed:false — NOT an error. #287's acceptance
// says suspending an already-suspended tenant returns current state.
func TestSuspendHandler_AlreadySuspendedIsOKNotError(t *testing.T) {
	h := newTestHandler(&stubRepo{suspend: &SuspendResult{
		Status: StatusSuspended, StoresAffected: 0, Changed: false,
	}})
	rec := doPost(t, h, "/internal/tenants/"+testTenantID+"/suspend")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"changed":false`)
}

func TestSuspendHandler_UnknownTenantIs404(t *testing.T) {
	h := newTestHandler(&stubRepo{suspendErr: ErrNotFound})
	rec := doPost(t, h, "/internal/tenants/"+testTenantID+"/suspend")
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestSuspendHandler_ArchivedIs409(t *testing.T) {
	h := newTestHandler(&stubRepo{suspendErr: ErrConflict})
	rec := doPost(t, h, "/internal/tenants/"+testTenantID+"/suspend")
	require.Equal(t, http.StatusConflict, rec.Code)
}
```

Write an equivalent unsuspend test. **Give the stub distinct non-zero values** (`StoresAffected: 2`, not `1`) so an assertion cannot pass on a fabricated zero from a missing field.

- [ ] **Step 2: Run to verify they fail**

```bash
cd services/platform-api && go test ./internal/tenant/ -run TestSuspendHandler -v
```
Expected: FAIL, undefined handler.

- [ ] **Step 3: Implement the handlers**

In `internal/tenant/handler.go`, matching the response-shaping style already in that file:

```go
// RegisterLifecycle mounts the operator-facing tenant lifecycle routes.
// Mounted on strictInternal only: these act on the whole estate and are not
// scoped by anything the caller had to know, so an unconfigured deploy must
// answer 503 rather than serve them.
func (h *Handler) RegisterLifecycle(internal *gin.RouterGroup) {
	g := internal.Group("/tenants")
	g.POST("/:id/suspend", h.suspendTenant)
	g.POST("/:id/unsuspend", h.unsuspendTenant)
}

func (h *Handler) suspendTenant(c *gin.Context) {
	h.lifecycle(c, h.repo.Suspend)
}

func (h *Handler) unsuspendTenant(c *gin.Context) {
	h.lifecycle(c, h.repo.Unsuspend)
}

// lifecycle is shared by both routes: identical shaping, identical error
// mapping, one implementation so the two cannot drift.
func (h *Handler) lifecycle(c *gin.Context,
	op func(context.Context, string) (*SuspendResult, error)) {

	id := c.Param("id")
	res, err := op(c.Request.Context(), id)
	switch {
	case errors.Is(err, ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{
			"error": "tenant_not_found", "message": "no tenant with that id"})
		return
	case errors.Is(err, ErrConflict):
		c.JSON(http.StatusConflict, gin.H{
			"error": "invalid_status_transition", "message": err.Error()})
		return
	case err != nil:
		h.logger.Error("tenant lifecycle failed", "tenant_id", id, "err", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "internal_error", "message": "tenant lifecycle operation failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"tenant_id":       id,
		"status":          res.Status,
		"stores_affected": res.StoresAffected,
		"changed":         res.Changed,
	}})
}
```

Use the package's real logger field name and error sentinels; do not introduce new ones.

- [ ] **Step 4: Mount on strictInternal**

In `cmd/server/main.go`, beside the existing `tenantHandler.RegisterDirectory(strictInternal)` at ~line 354:

```go
	tenantHandler.RegisterLifecycle(strictInternal)
```

Check whether this service registers routes at more than one site (marketplace-api does, and it has cost that repo five incidents). If there is only one engine here, say so in your report; if there are two, mount at both.

- [ ] **Step 5: Run tests and build**

```bash
cd services/platform-api && go build ./... && go vet ./... && go test ./internal/tenant/ -run TestSuspend -v
```
Expected: PASS.

- [ ] **Step 6: Prove the no-op test discriminates**

Make the handler return `409` when `Changed` is false. Re-run.
Expected: **FAIL** in `TestSuspendHandler_AlreadySuspendedIsOKNotError`. Record the test name and failure text. Revert.

- [ ] **Step 7: Commit**

```bash
git add services/platform-api/internal/tenant/ services/platform-api/cmd/server/main.go
git commit -m "feat(platform-api): internal tenant suspend/unsuspend endpoints on strictInternal (#287)"
```

---

### Task 4: marketplace-api — a write client for platform-api

Every existing client (`tenantdirectory`, `onboardingfunnel`, `estatecounts`) is **read-only**: a `do(ctx, path, out)` helper that issues GET. This is the surface's first write, so the client needs a POST path built to the same shape — a `maxBody` cap, `X-Internal-Auth`, and an `ErrUnavailable` that is **never** conflated with an empty result.

**Files:**
- Create: `services/marketplace-api/internal/tenantlifecycle/client.go`
- Test: `services/marketplace-api/internal/tenantlifecycle/client_test.go`

**Interfaces:**
- Consumes: T3's endpoints.
- Produces, called by T5:

```go
var ErrUnavailable = errors.New("tenantlifecycle: platform-api unavailable")
var ErrNotFound    = errors.New("tenantlifecycle: tenant not found")
var ErrConflict    = errors.New("tenantlifecycle: invalid status transition")

type Result struct {
	TenantID       string `json:"tenant_id"`
	Status         string `json:"status"`
	StoresAffected int    `json:"stores_affected"`
	Changed        bool   `json:"changed"`
}

func NewClient(baseURL, secret string, httpClient *http.Client) *Client
func (c *Client) Suspend(ctx context.Context, tenantID string) (*Result, error)
func (c *Client) Unsuspend(ctx context.Context, tenantID string) (*Result, error)
```

- [ ] **Step 1: Copy the nearest client, do not invent a fourth shape**

Read `internal/tenantdirectory/client.go` first. Reuse its constants and structure (`maxBody = 4 << 20`, the `X-Internal-Auth` header, the timeout, the error wrapping). The only new thing is a `post` helper alongside its `do`.

- [ ] **Step 2: Write the failing tests**

```go
func TestSuspend_MapsStatusCodes(t *testing.T) {
	cases := []struct {
		name    string
		code    int
		body    string
		wantErr error
	}{
		{"ok", 200, `{"data":{"tenant_id":"t1","status":"suspended","stores_affected":2,"changed":true}}`, nil},
		{"not found", 404, `{"error":"tenant_not_found"}`, tenantlifecycle.ErrNotFound},
		{"conflict", 409, `{"error":"invalid_status_transition"}`, tenantlifecycle.ErrConflict},
		{"server error", 500, `{"error":"internal_error"}`, tenantlifecycle.ErrUnavailable},
		{"bad gateway", 502, ``, tenantlifecycle.ErrUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, http.MethodPost, r.Method)
				require.Equal(t, "/internal/tenants/t1/suspend", r.URL.Path)
				require.NotEmpty(t, r.Header.Get("X-Internal-Auth"), "internal auth header must be sent")
				w.WriteHeader(tc.code)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			got, err := tenantlifecycle.NewClient(srv.URL, "secret", srv.Client()).
				Suspend(context.Background(), "t1")
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				require.Nil(t, got, "an error must never come back with a usable result")
				return
			}
			require.NoError(t, err)
			require.Equal(t, 2, got.StoresAffected)
			require.True(t, got.Changed)
			require.Equal(t, "suspended", got.Status)
		})
	}
}

// A 200 whose body is truncated or unparseable is an error, not a zero
// result. Conflating the two is how a caller ends up reporting "0 stores
// affected, changed: false" for a request that actually did something.
func TestSuspend_UnparseableBodyIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"data":{`))
	}))
	defer srv.Close()
	got, err := tenantlifecycle.NewClient(srv.URL, "secret", srv.Client()).
		Suspend(context.Background(), "t1")
	require.Error(t, err)
	require.Nil(t, got)
}
```

- [ ] **Step 3: Run to verify they fail**

```bash
cd services/marketplace-api && go test ./internal/tenantlifecycle/ -v
```
Expected: FAIL, package does not exist.

- [ ] **Step 4: Implement the client**

Mirror `tenantdirectory`'s structure. The status mapping is the part that matters: `200` → decode; `404` → `ErrNotFound`; `409` → `ErrConflict`; anything else (including any 5xx and any transport error) → `ErrUnavailable`. Cap the read at `maxBody`.

- [ ] **Step 5: Run the tests**

Expected: PASS.

- [ ] **Step 6: Prove the error mapping is real**

Make the client return `nil` error on a 500. Re-run.
Expected: **FAIL** in the `server error` subtest. Record the subtest name and failure text. Revert.

- [ ] **Step 7: Commit**

```bash
git add services/marketplace-api/internal/tenantlifecycle/
git commit -m "feat(marketplace-api): tenantlifecycle write client for platform-api (#287)"
```

---

### Task 5: marketplace-api — the console-facing endpoints

The surface's **first write**. Reason codes, capability, audit, and an immediate local-projection update all land here.

**Files:**
- Create: `services/marketplace-api/internal/handlers/platformadmin/tenant_lifecycle.go`
- Modify: `services/marketplace-api/internal/handlers/platformadmin/routes.go`
- Modify: `services/marketplace-api/cmd/marketplace-api/main.go` (**both** `Register` sites)
- Test: `services/marketplace-api/internal/handlers/platformadmin/tenant_lifecycle_test.go`
- Create: `services/marketplace-api/internal/handlers/platformadmin/testdata/tenant_suspend_response.json`

**Interfaces:**
- Consumes: `tenantlifecycle.Client` (T4), `platformadmin.EmitOperatorAction`, `stores.Repository`.
- Produces: the two routes, and a `TenantLifecycle` interface on `Deps`.

```go
type TenantLifecycle interface {
	Suspend(ctx context.Context, tenantID string) (*tenantlifecycle.Result, error)
	Unsuspend(ctx context.Context, tenantID string) (*tenantlifecycle.Result, error)
}
```

Narrow interface declared in this package (not the concrete client type) so the handler is stubbable — the same reason `EstateCounts` and `OnboardingFunnel` are declared here.

**ROUTING — read before touching `routes.go` (trap 2).** `/admin/tenants/{id}/suspend` collides with the merchant tree's `/admin/tenants/:tenantId/sso`: two different wildcard names at one path position makes gin **panic at router build time**, so the service fails to *start* rather than failing a request. Safe only while these routes are registered on the `platformadmin` group under `/api/v1/platform`. Never the merchant group. Do not "tidy" it.

**CAPABILITY — a ruling you must not silently change.** `RequirePlatformAuth` already rejects a write whose `Capability` header is empty (`middleware.go:153`) but **never validates the value**, and no handler reads `CtxCapability` today. This plan keeps **presence-only** enforcement and **records the asserted capability in the audit row**. Do not invent a required capability string: the console asserts these values and a name chosen here could silently mismatch it, refusing every real request. If value semantics are wanted, that is a console-side decision and its own issue.

- [ ] **Step 1: Write the failing tests**

```go
package platformadmin_test

// stubLifecycle records what it was asked to do and returns canned results.
// Values are DISTINCT and NON-ZERO so an assertion cannot pass on a
// fabricated zero from a missing field.
type stubLifecycle struct {
	res        *tenantlifecycle.Result
	err        error
	gotTenant  string
	calls      int
}

func (s *stubLifecycle) Suspend(_ context.Context, id string) (*tenantlifecycle.Result, error) {
	s.calls++
	s.gotTenant = id
	return s.res, s.err
}
func (s *stubLifecycle) Unsuspend(_ context.Context, id string) (*tenantlifecycle.Result, error) {
	s.calls++
	s.gotTenant = id
	return s.res, s.err
}

func TestSuspend_RequiresKnownReasonCode(t *testing.T) {
	for _, body := range []string{
		`{}`,                                  // missing
		`{"reason_code":""}`,                  // empty
		`{"reason_code":"because_i_said_so"}`, // not in the set
		`{"reason":"free text only"}`,         // free text is not a substitute
	} {
		rec := postLifecycle(t, newLifecycleDeps(t, &stubLifecycle{}), "suspend", body)
		require.Equal(t, http.StatusBadRequest, rec.Code, "body %s must be rejected", body)
		require.Contains(t, rec.Body.String(), "reason_code")
	}
}

func TestSuspend_AcceptsEveryDeclaredCode(t *testing.T) {
	for _, code := range platformadmin.SuspendReasonCodes {
		stub := &stubLifecycle{res: &tenantlifecycle.Result{
			TenantID: testTenant, Status: "suspended", StoresAffected: 3, Changed: true}}
		rec := postLifecycle(t, newLifecycleDeps(t, stub), "suspend",
			`{"reason_code":"`+code+`"}`)
		require.Equal(t, http.StatusOK, rec.Code, "declared code %q must be accepted", code)
	}
}

// The upstream's result is projected, not passed through, and the counts
// come from upstream rather than being invented locally.
func TestSuspend_ProjectsUpstreamResult(t *testing.T) {
	stub := &stubLifecycle{res: &tenantlifecycle.Result{
		TenantID: testTenant, Status: "suspended", StoresAffected: 3, Changed: true}}
	rec := postLifecycle(t, newLifecycleDeps(t, stub), "suspend", `{"reason_code":"abuse"}`)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, testTenant, stub.gotTenant)
	var body struct {
		Data struct {
			TenantID       string `json:"tenant_id"`
			Status         string `json:"status"`
			StoresAffected int    `json:"stores_affected"`
			Changed        bool   `json:"changed"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, 3, body.Data.StoresAffected)
	require.True(t, body.Data.Changed)
	require.Equal(t, "suspended", body.Data.Status)
}

// An upstream ErrUnavailable must NOT read as "nothing to do".
func TestSuspend_UpstreamUnavailableIs503NotEmptySuccess(t *testing.T) {
	stub := &stubLifecycle{err: tenantlifecycle.ErrUnavailable}
	rec := postLifecycle(t, newLifecycleDeps(t, stub), "suspend", `{"reason_code":"abuse"}`)
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.NotContains(t, rec.Body.String(), `"changed"`,
		"a failed suspension must not shape a result at all")
}

func TestSuspend_UpstreamNotFoundIs404_ConflictIs409(t *testing.T) {
	rec := postLifecycle(t, newLifecycleDeps(t, &stubLifecycle{err: tenantlifecycle.ErrNotFound}),
		"suspend", `{"reason_code":"abuse"}`)
	require.Equal(t, http.StatusNotFound, rec.Code)

	rec = postLifecycle(t, newLifecycleDeps(t, &stubLifecycle{err: tenantlifecycle.ErrConflict}),
		"suspend", `{"reason_code":"abuse"}`)
	require.Equal(t, http.StatusConflict, rec.Code)
}

// The audit row must carry the tenant, the operator, and the reason code —
// and there must be exactly ONE per changed call, none for a no-op.
func TestSuspend_AuditsOncePerChangeAndNeverForNoOp(t *testing.T) {
	changed := &stubLifecycle{res: &tenantlifecycle.Result{
		TenantID: testTenant, Status: "suspended", StoresAffected: 3, Changed: true}}
	deps, emitted := newLifecycleDepsCapturingAudit(t, changed)
	postLifecycle(t, deps, "suspend", `{"reason_code":"abuse","reason":"spam orders"}`)

	require.Len(t, *emitted, 1, "exactly one audit row per changed suspension")
	ev := (*emitted)[0]
	require.Equal(t, testTenant, ev.TenantID.String(),
		"an event with no tenant is silently DROPPED — this is the assertion that catches it")
	require.Equal(t, "abuse", ev.Metadata["reason_code"])
	require.Equal(t, "spam orders", ev.Metadata["reason"])
	require.Equal(t, 3, ev.Metadata["stores_affected"])

	noop := &stubLifecycle{res: &tenantlifecycle.Result{
		TenantID: testTenant, Status: "suspended", StoresAffected: 0, Changed: false}}
	deps2, emitted2 := newLifecycleDepsCapturingAudit(t, noop)
	rec := postLifecycle(t, deps2, "suspend", `{"reason_code":"abuse"}`)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Empty(t, *emitted2, "a no-op writes NO audit row (#287 acceptance)")
}

// The local projection is updated in the same request, so enforcement does
// not wait out StoreMiddleware's 5-minute TTL.
func TestSuspend_UpdatesLocalProjectionImmediately(t *testing.T) { /* integration; see Step 5 */ }
```

Write helpers `newLifecycleDeps`, `newLifecycleDepsCapturingAudit`, and `postLifecycle` in the same file, following how `kpisRouter` and the existing signed-request helpers work in this package. The request must carry a valid signature, an operator, and a capability, or it will be rejected before the handler.

- [ ] **Step 2: Run to verify they fail**

```bash
cd services/marketplace-api && go test ./internal/handlers/platformadmin/ -run 'TestSuspend|TestUnsuspend' -v
```
Expected: FAIL, undefined.

- [ ] **Step 3: Implement reason codes and the handler**

In `tenant_lifecycle.go`:

```go
// SuspendReasonCodes is the closed set for a suspension. An audit row
// saying WHAT without WHY is the gap this series exists to close, so the
// code is required; free text is accepted IN ADDITION, never instead.
var SuspendReasonCodes = []string{
	"abuse",         // abusive content or behaviour toward customers or staff
	"fraud",         // suspected fraudulent transactions or identity
	"non_payment",   // billing failed and the dunning ladder is exhausted
	"legal",         // legal or regulatory demand
	"tos_violation", // terms breach not covered by abuse or fraud
	"security",      // compromised account or active security incident
	"voluntary",     // merchant asked for the store to be paused
}

// UnsuspendReasonCodes is deliberately a different set: the reasons for
// lifting a suspension are not the reasons for applying one.
var UnsuspendReasonCodes = []string{
	"resolved", "appeal_upheld", "operator_error", "voluntary_end",
}
```

Validate with an explicit membership check that returns `400` naming the field and echoing the allowed set. Never coerce an unknown code, and never fall back to storing it as free text.

The handler then: validates the body → calls the client → maps `ErrNotFound`/`ErrConflict`/`ErrUnavailable` to `404`/`409`/`503` → on `Changed` **only**, updates the local `stores` projection and emits the audit row → responds with the projected result. Emit via:

```go
if err := EmitOperatorAction(c, deps.Emitter, tenantUUID, audit.Event{
	Action:       "tenant.suspended", // "tenant.unsuspended" on the other route
	ResourceType: "tenant",
	ResourceID:   tenantID,
	Metadata: map[string]any{
		"reason_code":     req.ReasonCode,
		"reason":          req.Reason,
		"stores_affected": res.StoresAffected,
		"capability":      c.GetString(CtxCapability),
	},
}); err != nil {
	// ErrMissingTenant: the operation SUCCEEDED upstream but we cannot
	// attribute it. Log loudly; do not fail the response, and do not
	// pretend it was attributed.
	deps.Logger.Error("suspension not audited", "tenant_id", tenantID, "err", err)
}
```

- [ ] **Step 4: Mount, and wire at BOTH main.go sites**

Add `TenantLifecycle` to `Deps`, mount in `Register` behind a non-nil guard following the `Trials`/`AllSubscriptions` pattern, and construct it at **both** `platformadmin.Register` call sites in `main.go` (the `mode.Both` engine and the `mode.Admin` engine). One site only means production differs from dev silently — that is #323, and `cmd/marketplace-api/wiring_test.go` already asserts the two `Deps` literals carry the same field set, so a one-site change **fails that test**. Good: let it.

- [ ] **Step 5: Integration test for the immediate projection update**

In a `//go:build integration` file, seed a store row in marketplace-api's local `stores` projection, call the handler with a stub client returning `Changed: true`, and assert the local row is now `suspended` **without** waiting for any TTL. Remember marketplace-api's `stores` has **no** reference-data FKs (trap 4), so plausible strings suffice.

- [ ] **Step 6: Golden fixture**

Write `testdata/tenant_suspend_response.json` from real handler output, compare with `require.JSONEq`, and prove by mutation that it catches a field **rename** and a field **addition**. Both must fail. A fixture that only catches omissions is theatre.

- [ ] **Step 7: Mutations — three, all required**

1. Delete the reason-code membership check → `TestSuspend_RequiresKnownReasonCode` must FAIL.
2. Emit the audit row unconditionally instead of only on `Changed` → the no-op half of `TestSuspend_AuditsOncePerChangeAndNeverForNoOp` must FAIL.
3. Drop `TenantID` from the audit event → the tenant assertion must FAIL.

Record the failing test name and actual failure text for each. Revert each.

- [ ] **Step 8: Full run and commit**

```bash
cd services/marketplace-api && go build ./... && go vet ./... && go vet -tags=integration ./... \
  && go test ./internal/handlers/platformadmin/ ./internal/tenantlifecycle/ ./cmd/marketplace-api/
git add services/marketplace-api/
git commit -m "feat(platformadmin): POST /admin/tenants/{id}/suspend and /unsuspend (#287)"
```

---

### Task 6: marketplace-api — enforce store status in `StoreMiddleware`

Independent of T7 and T8.

**Files:**
- Modify: `services/marketplace-api/internal/stores/middleware.go`
- Test: `services/marketplace-api/internal/stores/middleware_test.go`

**Interfaces:** consumes `Store.Status` (already on the projection, mapped at `platform_http.go:150`). Produces no new symbols.

`StoreMiddleware` currently resolves the store, checks tenant ownership, sets `c.Set("store", …)` and calls `c.Next()` — **it never looks at status**. It is the single chokepoint for every store-scoped admin API route.

- [ ] **Step 1: Write the failing tests**

The helpers you need already exist in that file: `newFakeRepo()`, `repo.preload(store)`, `fakeClient`, `newFixtureStore(syncedAt)` (which sets `Status: stores.StatusActive`), `baseCfg(repo, client)`, `buildRouter(cfg, probe)`, `doRequest(r)`, and a `probe` whose `reached` field records whether the handler ran. Use them; do not build a second harness.

```go
// A suspended store must not serve admin API traffic even though it
// resolves cleanly and belongs to the right tenant. `reached` is the real
// assertion: a 404 with the handler still having run would mean the
// middleware let it through and something later happened to 404.
func TestMiddleware_SuspendedStoreIsRefused(t *testing.T) {
	repo := newFakeRepo()
	st := newFixtureStore(time.Now()) // fresh, so no refresh path is involved
	st.Status = stores.StatusSuspended
	repo.preload(st)
	p := &probe{}

	w := doRequest(buildRouter(baseCfg(repo, &fakeClient{}), p))

	if w.Code != http.StatusNotFound {
		t.Fatalf("status: got %d want 404", w.Code)
	}
	if p.reached {
		t.Fatal("handler was reached for a suspended store")
	}
}

func TestMiddleware_ArchivedStoreIsRefused(t *testing.T) {
	repo := newFakeRepo()
	st := newFixtureStore(time.Now())
	st.Status = stores.StatusArchived
	repo.preload(st)
	p := &probe{}

	w := doRequest(buildRouter(baseCfg(repo, &fakeClient{}), p))

	if w.Code != http.StatusNotFound {
		t.Fatalf("status: got %d want 404", w.Code)
	}
	if p.reached {
		t.Fatal("handler was reached for an archived store")
	}
}

// THE asymmetry, half one: a stale row that says `suspended` is still
// enforced. The fixture is deliberately OLDER than StaleCeil and the client
// is made to fail, so the only thing the middleware can act on is the stale
// cached row — which must still refuse.
func TestMiddleware_StaleSuspendedIsStillRefused(t *testing.T) {
	cfg := baseCfg(newFakeRepo(), &fakeClient{err: stores.ErrPlatformUnavailable})
	repo := newFakeRepo()
	st := newFixtureStore(time.Now().Add(-25 * time.Hour)) // past the 24h StaleCeil
	st.Status = stores.StatusSuspended
	repo.preload(st)
	cfg.Repo = repo
	p := &probe{}

	w := doRequest(buildRouter(cfg, p))

	if w.Code != http.StatusNotFound {
		t.Fatalf("status: got %d want 404 (a cached suspended status is authoritative at any age)", w.Code)
	}
	if p.reached {
		t.Fatal("handler was reached for a stale suspended store")
	}
}

// THE asymmetry, half two, and the one that stops the fix going too far: a
// stale ACTIVE row is still served when platform-api is unreachable. If this
// test fails you have made the cache fail-closed and an outage now locks out
// every merchant.
func TestMiddleware_StaleActiveIsStillServed(t *testing.T) {
	cfg := baseCfg(newFakeRepo(), &fakeClient{err: stores.ErrPlatformUnavailable})
	repo := newFakeRepo()
	repo.preload(newFixtureStore(time.Now().Add(-25 * time.Hour))) // stale, Status active
	cfg.Repo = repo
	p := &probe{}

	w := doRequest(buildRouter(cfg, p))

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200 (fail-open on stale ACTIVE is deliberate)", w.Code)
	}
	if !p.reached {
		t.Fatal("handler not reached: stale active must still be served")
	}
}
```

Check `fakeClient`'s real field name for the error it returns and the exact sentinel the package uses for an unavailable platform (`internal/stores/errors.go`) — use those rather than the names above if they differ. The two stale tests must exercise the same code path with only `Status` differing; if you find yourself special-casing anything else between them, the asymmetry is not where you think it is.

- [ ] **Step 2: Run to verify they fail**

```bash
cd services/marketplace-api && go test ./internal/stores/ -run TestStoreMiddleware -v
```
Expected: FAIL — a suspended store is currently served.

- [ ] **Step 3: Implement, and decide the status code deliberately**

Refuse any resolved store whose status is not `StatusActive`, on **both** the fresh-cache path and the post-refresh path, and — the asymmetry — refuse on the stale-serving path too when the cached status is suspended, while continuing to serve a stale `active`.

Use **404**, matching `respondNotFound` already in this file and the storefront's own choice at `platform-api/internal/store/handler.go:150`. A 403 would confirm the store exists to a caller who is no longer entitled to know; the storefront already decided this question and the admin path should not disagree with it. State this choice in your report.

- [ ] **Step 4: Run the tests**

Expected: PASS.

- [ ] **Step 5: Mutations — two**

1. Remove the status check → `TestStoreMiddleware_SuspendedStoreIsRefused` must FAIL.
2. Make the stale path serve a cached suspended row → `TestStoreMiddleware_StaleSuspendedIsStillRefused` must FAIL, while `TestStoreMiddleware_StaleActiveIsStillServed` keeps PASSING. If the second test also fails, you have made the cache fail-closed and locked out every merchant during an outage — that is the wrong fix.

Record names and failure text. Revert both.

- [ ] **Step 6: Commit**

```bash
git add services/marketplace-api/internal/stores/
git commit -m "feat(marketplace-api): refuse admin traffic for non-active stores (#287)"
```

---

### Task 7: marketplace-api — refuse ALL admin traffic for a suspended tenant

Independent of T6 and T8. **This replaces the auth-bff approach the spec originally described.** Investigation showed auth-bff has no platform-api client, and gating only session *issuance* there would leave an existing session free to hit the three non-store-scoped admin groups — `/admin` (`routes.go:162`), `/admin/account` (`:170`) and the SSO group `/admin/tenants/:tenantId` (`:144`) — until it expired. T6 only covers `/admin/stores/:storeId`.

**Files:**
- Create: `services/marketplace-api/internal/tenantgate/gate.go`
- Create: `services/marketplace-api/internal/tenantgate/gate_test.go`
- Modify: `services/marketplace-api/internal/handlers/admin/routes.go` (apply to the four groups)
- Modify: `services/marketplace-api/cmd/marketplace-api/main.go` (construct and pass it)

**Interfaces:**
- Consumes: `tenantdirectory.Client` — **already present in this service, and its `Tenant` type already carries `Status`** (`internal/tenantdirectory/client.go:39`), so no new endpoint and no new client is needed.
- Produces:

```go
// Lookup is the subset of tenantdirectory.Client the gate needs.
type Lookup interface {
	Get(ctx context.Context, id string) (*tenantdirectory.TenantDetail, error)
}

func New(l Lookup, logger *slog.Logger, ttl time.Duration) *Gate
func (g *Gate) RequireActiveTenant() gin.HandlerFunc
```

**Copy the shape of `internal/subscription/readonly`** (`middleware.go:25`, `allowlist.go`). It is the same problem — a status gate over admin routes — and it already solves the router-integration details.

**RULING, do not silently change it: this gate allowlists NOTHING.** `RequireActive` deliberately lets every `GET /admin/**` through and allowlists billing, tax and subscription-recovery routes, because a read-only *subscription* is a billing state the merchant must be able to fix themselves. A tenant suspension is the opposite: an operator action taken for abuse, fraud or a legal demand, where self-service recovery is precisely what must not be available. A suspended tenant gets nothing and contacts support. If operators later want reason-dependent behaviour — say, letting a `non_payment` suspension still reach billing — that is a follow-up issue with its own review, not a quiet allowlist entry here.

**Caching:** tenant status is fetched through `tenantdirectory` and cached in-process with a short TTL. The production admin deployment is `replicas: 1` today, so an in-process cache is coherent; if it is ever scaled, each pod simply fetches more often, which stays correct. Use `singleflight` to coalesce concurrent refreshes, exactly as `StoreMiddleware` does.

**Staleness rules, matching T6's asymmetry:**
- Cached `suspended` is **authoritative at any age** — never re-fetch to "give the tenant the benefit of the doubt".
- Cached `active` past TTL is refreshed; if the refresh fails, serve on the stale `active`.
- **No cached value at all plus a failed lookup: fail OPEN** (serve). A cold cache during a platform-api outage must not lock out every merchant. This is the one hole in the gate and it is deliberate — record it in your report.

- [ ] **Step 1: Write the failing tests**

```go
package tenantgate_test

// stubLookup returns a canned tenant and counts calls so the cache can be
// asserted. Values are distinct and non-zero so nothing passes on a zero.
type stubLookup struct {
	status string
	err    error
	calls  int32
}

func (s *stubLookup) Get(_ context.Context, id string) (*tenantdirectory.TenantDetail, error) {
	atomic.AddInt32(&s.calls, 1)
	if s.err != nil {
		return nil, s.err
	}
	return &tenantdirectory.TenantDetail{
		Tenant: tenantdirectory.Tenant{ID: id, Status: s.status},
	}, nil
}

func buildGateRouter(t *testing.T, l tenantgate.Lookup, ttl time.Duration, reached *bool) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("tenant_id", testTenant) })
	g := tenantgate.New(l, nil, ttl)
	r.GET("/admin/account", g.RequireActiveTenant(), func(c *gin.Context) {
		*reached = true
		c.String(http.StatusOK, "ok")
	})
	return r
}

func doGet(r *gin.Engine, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

// An active tenant passes through.
func TestGate_ActiveTenantPasses(t *testing.T) {
	reached := false
	rec := doGet(buildGateRouter(t, &stubLookup{status: "active"}, time.Minute, &reached), "/admin/account")
	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, reached)
}

// A suspended tenant is refused on a NON-store-scoped route — the whole
// point of this task, since StoreMiddleware never sees these.
func TestGate_SuspendedTenantIsRefusedOnNonStoreRoute(t *testing.T) {
	reached := false
	rec := doGet(buildGateRouter(t, &stubLookup{status: "suspended"}, time.Minute, &reached), "/admin/account")
	require.Equal(t, http.StatusForbidden, rec.Code)
	require.False(t, reached, "handler must not run for a suspended tenant")
}

// NOTHING is allowlisted — not even a GET. This is the assertion that
// catches someone copying readonly.DefaultAllowlist wholesale.
func TestGate_NoAllowlistNotEvenGET(t *testing.T) {
	reached := false
	rec := doGet(buildGateRouter(t, &stubLookup{status: "suspended"}, time.Minute, &reached), "/admin/account")
	require.Equal(t, http.StatusForbidden, rec.Code, "a GET is NOT exempt for a suspended tenant")
	require.False(t, reached)
}

// The cache is used: two requests inside the TTL cause ONE lookup.
func TestGate_CachesWithinTTL(t *testing.T) {
	reached := false
	l := &stubLookup{status: "active"}
	r := buildGateRouter(t, l, time.Minute, &reached)
	doGet(r, "/admin/account")
	doGet(r, "/admin/account")
	require.Equal(t, int32(1), atomic.LoadInt32(&l.calls), "second request must be served from cache")
}

// Cached suspended is authoritative at ANY age: even with the TTL expired
// and the upstream now saying active, the gate keeps refusing until it has
// successfully re-read. Assert the refusal, not the call count.
func TestGate_CachedSuspendedIsAuthoritativeWhenLookupFails(t *testing.T) {
	reached := false
	l := &stubLookup{status: "suspended"}
	r := buildGateRouter(t, l, time.Nanosecond, &reached) // instantly stale
	require.Equal(t, http.StatusForbidden, doGet(r, "/admin/account").Code)

	l.err = tenantdirectory.ErrUnavailable // upstream now unreachable
	require.Equal(t, http.StatusForbidden, doGet(r, "/admin/account").Code,
		"a cached suspended status must not decay into access")
	require.False(t, reached)
}

// Stale ACTIVE plus a failed refresh still serves — fail-open, so an
// outage does not lock out every merchant.
func TestGate_StaleActiveWithFailedRefreshStillServes(t *testing.T) {
	reached := false
	l := &stubLookup{status: "active"}
	r := buildGateRouter(t, l, time.Nanosecond, &reached)
	require.Equal(t, http.StatusOK, doGet(r, "/admin/account").Code)

	l.err = tenantdirectory.ErrUnavailable
	require.Equal(t, http.StatusOK, doGet(r, "/admin/account").Code,
		"stale active must still serve when the refresh fails")
}

// Cold cache plus a failed lookup fails OPEN. Deliberate: assert it so the
// behaviour is a decision on the record rather than an accident.
func TestGate_ColdCacheWithFailedLookupFailsOpen(t *testing.T) {
	reached := false
	rec := doGet(buildGateRouter(t,
		&stubLookup{err: tenantdirectory.ErrUnavailable}, time.Minute, &reached), "/admin/account")
	require.Equal(t, http.StatusOK, rec.Code,
		"cold cache + outage must not lock out every merchant")
	require.True(t, reached)
}

// No tenant on the context means this middleware cannot judge: pass through
// and let the auth layer deal with it, rather than 403-ing every request.
func TestGate_NoTenantOnContextPassesThrough(t *testing.T) { /* build a router without the tenant_id setter */ }
```

Fill in the last test's body following `buildGateRouter` minus the `c.Set("tenant_id", …)` line.

- [ ] **Step 2: Run to verify they fail**

```bash
cd services/marketplace-api && go test ./internal/tenantgate/ -v
```
Expected: FAIL, package does not exist.

- [ ] **Step 3: Implement the gate**

`403` with `{"error":"tenant_suspended"}` — not `402` (that is `RequireActive`'s billing meaning) and not `404` (the tenant plainly exists to a caller already holding a session for it, and T6's 404 is about not confirming a *store* to someone no longer entitled to know it). State this choice in your report.

- [ ] **Step 4: Run the tests.** Expected: PASS.

- [ ] **Step 5: Apply the gate in `routes.go`**

Add it to all four groups — `ssoTenant` (`:144`), `adminRoot` (`:162`), `account` (`:170`) and `storeRoute` (`:209`) — immediately after `authMW` in each chain, so it runs once the tenant is known. Yes, `storeRoute` too: T6 catches the store, this catches the tenant, and a tenant with zero stores has no store for T6 to catch. Do NOT apply it to the group at `:129` that deliberately runs with no `authMW` (read its comment first).

- [ ] **Step 6: Wire in main.go**

Construct once and pass through `admin.Deps`. If `tenantDirectoryClient` is nil (no `PLATFORM_API_URL`), pass a nil gate and have `RequireActiveTenant` be a no-op — matching how the other client-backed features degrade. Assert that no-op in a test: a nil gate must not panic.

- [ ] **Step 7: Mutations — three, all required**

1. Remove the status check → `TestGate_SuspendedTenantIsRefusedOnNonStoreRoute` must FAIL.
2. Make a failed refresh clear the cached `suspended` → `TestGate_CachedSuspendedIsAuthoritativeWhenLookupFails` must FAIL while `TestGate_StaleActiveWithFailedRefreshStillServes` keeps PASSING. If both fail you have made it fail-closed and locked out every merchant during an outage — wrong fix.
3. Add `{http.MethodGet, "/admin/*path"}` as an allowlist → `TestGate_NoAllowlistNotEvenGET` must FAIL.

Record the failing test name and actual failure text for each. Revert each.

- [ ] **Step 8: Commit**

```bash
git add services/marketplace-api/internal/tenantgate/ services/marketplace-api/internal/handlers/admin/routes.go services/marketplace-api/cmd/marketplace-api/main.go
git commit -m "feat(marketplace-api): refuse all admin traffic for suspended tenants (#287)"
```

---

### Task 8: apps/admin — refuse the admin UI for a suspended store

Independent of T6 and T7.

**Files:**
- Modify: `apps/admin/middleware.ts` (~line 331, the `/internal/stores/by-slug/` fetch)
- Test: follow the local convention for middleware tests in that app

**Interfaces:** the internal by-slug response already returns the full store row including `status` — the middleware currently types it as `{ data: { tenant_id: string } }` and reads only `tenant_id`.

- [ ] **Step 1: Write the failing test** — `{slug}-admin.mark8ly.com` for a suspended store must not render the admin app.

- [ ] **Step 2: Run to verify it fails.**

- [ ] **Step 3: Widen the type and add the check**

```ts
const body = (await storeRes.json()) as {
  data: { tenant_id: string; status: string };
};
// A suspended or archived store must not serve the admin UI. Same 404 the
// unknown-slug branch above returns, and the same choice platform-api makes
// for the public storefront — an admin surface should not contradict it.
if (body.data.status !== "active") {
  return new NextResponse(null, { status: 404 });
}
```

- [ ] **Step 4: Note the `/pick-tenant` gap in your report.** The enclosing guard is `if (requestedSlug && !isPickTenant)`, so `/pick-tenant` bypasses this check by design. Do NOT change that in this task — it exists to avoid a redirect loop. Report whether a suspended tenant's user can still reach `/pick-tenant`, and whether anything there is harmful. If it is, that is a follow-up issue, not a silent edit here.

- [ ] **Step 5: Mutation** — revert the check and confirm your test fails. Record the name and failure text.

- [ ] **Step 6: Commit** — `feat(admin): refuse the admin UI for non-active stores (#287)`

---

## After the plan

**Verification after deploy** — separate the checks that carry information from those that merely mean "no data reached this code":

- *Carries information, data-independent:* suspend a scratch tenant and confirm its storefront 404s, its admin subdomain 404s, a fresh login is refused, and the admin API refuses store-scoped calls. Then unsuspend and confirm each reverses. **This is the only check that proves the feature works**, because every enforcement point is a different codebase.
- *Proves less:* the endpoint returning `200` with `stores_affected: 0`. Production has 4 tenants and 4 stores; a zero means the cascade matched nothing, not that it works.
- **Do not test on a real merchant tenant.** There is one GKE cluster and it is production.

**Gate verification on every service the change touches** — platform-api, marketplace-api and the admin app all ship separately, and a new caller against an old callee produces failures that look like bugs. Deploys are Kargo-gated: expect 10–20 minutes from freight appearing.

**Follow-ups this plan deliberately leaves:** Cloudflare edge cache is not purged on suspension (a suspended storefront may serve from the edge until expiry); existing merchant sessions are not revoked; the `archived` transition is untouched; capability **values** are not validated (presence only). Each is stated in the spec's out-of-scope section — file issues rather than widening this work.
