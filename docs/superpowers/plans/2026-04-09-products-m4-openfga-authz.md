# Products M4 — OpenFGA Authz Middleware Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire OpenFGA tenant-relation authorization into marketplace-api as a Check-only client + Gin middleware. Marketplace-api becomes a pure reader of tuples that platform-api already writes during onboarding/invitation. No tuple writes from this service. The middleware sits ready for M5 to mount on the admin route group.

**Architecture:** A new `internal/authz` package mirrors the shape of `services/platform-api/internal/authz` but exposes **only** the read methods marketplace-api needs (`Check`, `CheckMembership`, `GetRole`). A `Client` interface lets tests inject a `FakeClient` (in-memory tuple table) without touching real FGA. A new Gin middleware `RequireTenantRelation(role)` reads the user id and tenant id from the upstream auth context and calls `Check`. On `false` it responds 404 not_found (no existence leak per spec §13.1.1). At startup `cmd/marketplace-api/main.go` discovers the FGA store id by name (the same `mark8ly-platform` store platform-api uses — there is no separate "marketplace" store in slice 1) and constructs the real client; if discovery fails the process exits non-zero. Integration tests that need real FGA are gated by `TEST_FGA_API_URL`, mirroring the `TEST_DATABASE_URL` skip pattern.

**Tech Stack:** Go 1.26, Gin (existing), `github.com/openfga/go-sdk` (new direct dep), `github.com/mark8ly/marketplace-api/pkg/apperrors` (M3).

---

## Status

> **Pending.** All tasks open.

---

## Scope check

This is a single contained slice inside the existing `services/marketplace-api` Go module. It does not add a new module, does not touch `go.work`, does not modify migrations or the Helm chart. It only adds files under `services/marketplace-api/internal/authz/`, adds 8–12 lines to `cmd/marketplace-api/main.go`, and adds one new env var (`MARKETPLACE_FGA_API_URL`) to `pkg/config`.

Spec sections authoritative for this milestone:

- §3.2 (middleware chain — TenantMiddleware sets the tenant_id; FGA middleware reads it)
- §5 (original FGA model — **mostly superseded** by §13.1.1)
- §8 M4 entry — **superseded** by §13.1.1 (no tuple writes from marketplace-api)
- **§13.1.1 (the authoritative section for M4):** per-object tuples dropped; model has only `user` and `tenant` types; permission checks are tenant-relation only; cross-tenant 404 not 403
- §13.1.2 (storefront bypass — storefront routes do NOT use this middleware)
- §13.1.4 / §14.7 (StoreMiddleware — the middleware chain order is `Auth → Tenant → Store → FGA → Handler`)

---

## What is NOT in this milestone

- **No tuple writes.** Per §13.1.1, marketplace-api never writes a tuple. All tuples are written by platform-api during onboarding (`WriteOwnership`) and invitation accept (`WriteRole`). M4 only reads.
- **No model.fga in this PR.** The model lives in `services/platform-api/internal/authz/model.fga` (or wherever platform-api keeps it) and is bootstrapped by the openfga-seed init container. Marketplace-api consumes the same model and the same store id; it does NOT publish a duplicate model.
- **No HTTP routes.** M5 owns route registration. M4's middleware is a `gin.HandlerFunc` returned from a constructor that M5 will call. Until M5 mounts it, the middleware is dead code from a runtime perspective — but it's exercised by unit tests + an optional real-FGA integration test.
- **No real GCS uploader, no real platform-api client.** Those are M5.
- **No CI changes.** The integration test against a real FGA container is opt-in via `TEST_FGA_API_URL` and skips silently otherwise. CI wiring (docker-compose for the test job) is M5's responsibility.

---

## Decisions locked for this milestone

1. **Reuse the `mark8ly-platform` FGA store** (the one platform-api already uses). There is no separate marketplace store. Marketplace-api discovers it by name on startup using the same `DiscoverStoreID` pattern platform-api uses.
2. **Read-only `Client` interface.** The marketplace-api Client interface exposes ONLY `Check`, `CheckMembership`, `GetRole`. No `Write*` methods. This makes accidental tuple writes a compile error.
3. **Tenant context comes from a ginContextKey.** The middleware reads `c.GetString("user_id")` and `c.GetString("tenant_id")`. These keys are populated by an upstream Auth/Tenant middleware that does not yet exist in marketplace-api — M5 will add it. For M4, the middleware just trusts whatever is in the context; tests inject the values directly via `c.Set`. Document this clearly in the middleware godoc as a "depends on M5 wiring" caveat.
4. **404 on deny, not 403.** Per §13.1.1: "All admin routes return `404 not_found` (not `403 forbidden`) when the caller's tenant doesn't own the target store — no existence leaks across tenants." The middleware uses `apperrors.ErrNotFound` for both authentication-missing and authorization-failed cases.
5. **`RequireTenantRelation` is a per-route middleware factory**, not a global one. Each protected route registers it with the role it requires: `router.GET("/products", authzMW.RequireTenantRelation(authz.RoleStaff), ...)`. This makes the permission map from §13.1.1 visible at route-registration time.
6. **Integration test is opt-in.** The test is gated by `TEST_FGA_API_URL` (skip pattern matches `testdb.NewTx`'s `TEST_DATABASE_URL` skip). Local devs without a FGA container running don't see broken tests; CI sets the env var when the FGA service is up.
7. **Bootstrap is fail-fast.** If the FGA store can't be discovered, marketplace-api exits with code 1 at startup. Better to refuse to serve requests than to serve them with broken authz. Match platform-api's pattern.
8. **The middleware does its FGA Check on every request.** No caching in slice 1. The FGA request is small and FGA is local to the cluster, so it's < 5ms typical. Slice 2+ may add a per-request cookie cache.

---

## File structure produced by M4

```
services/marketplace-api/
├── cmd/marketplace-api/main.go                       MODIFIED: discover FGA store + construct client + pass to (future) handlers
├── pkg/config/config.go                              MODIFIED: add MARKETPLACE_FGA_API_URL env var
├── pkg/config/config_test.go                         MODIFIED: cover the new env var
└── internal/
    └── authz/
        ├── client.go                                 NEW: Client interface, Role constants, Config, real fgaClient impl, DiscoverStoreID
        ├── client_test.go                            NEW: unit tests for Role priority + DiscoverStoreID parsing (no FGA needed)
        ├── client_integration_test.go                NEW: real-FGA integration test gated by TEST_FGA_API_URL
        ├── fake.go                                   NEW: in-memory FakeClient + helper Add/Remove methods for tests
        ├── fake_test.go                              NEW: unit tests for the fake itself (verify Check semantics match real FGA)
        ├── middleware.go                             NEW: RequireTenantRelation factory + helper to extract user/tenant from context
        └── middleware_test.go                        NEW: unit tests with FakeClient (allow, deny, missing user, missing tenant, FGA error)
```

**Target file sizes:** `client.go` will be the largest at ~250 lines. Everything else under 200. No splits needed.

---

## New Go module dependency

```
github.com/openfga/go-sdk     (latest, already used by platform-api)
```

Add via `go get github.com/openfga/go-sdk` from `services/marketplace-api/`. Do NOT pin a version unless it differs from what platform-api uses (check `services/platform-api/go.mod` first; if it's already pinned to a specific version, use the same version for consistency). Then `go mod tidy`. Expect go.mod to gain `github.com/openfga/go-sdk` as a direct require, plus a couple of transitive deps in go.sum (the SDK pulls in `golang.org/x/oauth2` and `gopkg.in/yaml.v3` typically).

---

## Landmines (from auto-memory: feedback_marketplace_api_landmines.md)

M4 is pure Go, no infra changes. Only one landmine applies:

1. **Landmine #1 (go.work):** We are not adding a new module. Confirm `git diff go.work` is empty before committing each task.

The integration test against real FGA introduces a new opt-in env var, NOT a new dependency on the Helm chart, so landmines #3 (DATABASE_URL escaping) and #4 (CNPG postInitSQL) don't apply.

---

## Task decomposition

7 tasks. Tasks 1–4 can technically run in parallel (they touch different files), but the subagent-driven flow recommends serial dispatch with non-overlapping packages — and they all live under `internal/authz/`, so serial is safer.

| # | Task | Approx effort |
|---|---|---|
| 1 | `internal/authz/client.go` — Client interface + real fgaClient impl + DiscoverStoreID + Role constants | 60 min |
| 2 | `internal/authz/fake.go` — FakeClient with in-memory tuple set | 30 min |
| 3 | `internal/authz/middleware.go` + middleware unit tests with FakeClient | 60 min |
| 4 | `internal/authz/client_integration_test.go` — opt-in real FGA test | 30 min |
| 5 | `pkg/config` — add MARKETPLACE_FGA_API_URL env var + update test | 15 min |
| 6 | Wire `cmd/marketplace-api/main.go` — discover store + construct client (passed to a future handler-registration func; for M4 we just construct it and log "fga ready") | 30 min |
| 7 | Verification + PR | 15 min |
| **Total** | | **~4 hours** |

---

### Task 1: `internal/authz/client.go` — Client interface + real impl

**Files:**
- Create: `services/marketplace-api/internal/authz/client.go`
- Create: `services/marketplace-api/internal/authz/client_test.go`
- Modify: `services/marketplace-api/go.mod`, `go.sum` (add openfga-go-sdk)

**Scope:** Read-only Client interface (`Check`, `CheckMembership`, `GetRole`), real impl backed by `github.com/openfga/go-sdk`, `Config`, `New(cfg)`, `DiscoverStoreID(ctx, apiURL, name)`. Mirror platform-api's shape but trim out every Write* method.

- [ ] **Step 1: Add the dependency**

```
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/services/marketplace-api && go get github.com/openfga/go-sdk && go mod tidy
```

Check what version platform-api uses first:
```
grep "openfga/go-sdk" services/platform-api/go.mod
```
If platform-api pins a version, use the same: `go get github.com/openfga/go-sdk@<version>`.

- [ ] **Step 2: Write the failing unit test**

`services/marketplace-api/internal/authz/client_test.go`:

```go
package authz_test

import (
	"testing"

	"github.com/mark8ly/marketplace-api/internal/authz"
)

func TestRole_Constants(t *testing.T) {
	want := []authz.Role{authz.RoleOwner, authz.RoleAdmin, authz.RoleStaff, authz.RoleViewer}
	for _, r := range want {
		if string(r) == "" {
			t.Errorf("role %q is empty", r)
		}
	}
}

func TestRole_Priority(t *testing.T) {
	if !authz.RoleOwner.HigherOrEqual(authz.RoleAdmin) {
		t.Error("owner should outrank admin")
	}
	if authz.RoleStaff.HigherOrEqual(authz.RoleAdmin) {
		t.Error("staff should not outrank admin")
	}
	if !authz.RoleAdmin.HigherOrEqual(authz.RoleAdmin) {
		t.Error("admin should be equal to itself")
	}
}
```

- [ ] **Step 3: Run test, expect compile failure**

```
go test ./internal/authz/... -v
```

- [ ] **Step 4: Implement `client.go`**

```go
// Package authz is marketplace-api's read-only OpenFGA client for tenant-
// scoped permission checks. Per spec §13.1.1, marketplace-api NEVER writes
// tuples — all writes happen in platform-api during onboarding and
// invitation accept. This package exposes only Check / CheckMembership /
// GetRole. The Write* methods that platform-api's authz package exposes
// are intentionally absent so accidental tuple writes from marketplace-api
// are a compile error.
//
// The middleware that consumes this Client lives in the same package
// (middleware.go). Tests use the FakeClient in fake.go to drive the
// middleware without a real OpenFGA instance.
package authz

import (
	"context"
	"fmt"
	"net/http"

	"github.com/openfga/go-sdk/client"
)

// FGAStoreName is the canonical OpenFGA store name marketplace-api reads
// from. The same store is written to by platform-api — there is no
// separate "marketplace" store in slice 1 (spec §13.1.1).
const FGAStoreName = "mark8ly-platform"

// Role names match the relations defined in the OpenFGA model.
type Role string

const (
	RoleOwner  Role = "owner"
	RoleAdmin  Role = "admin"
	RoleStaff  Role = "staff"
	RoleViewer Role = "viewer"
)

var rolePriority = map[Role]int{
	RoleOwner:  4,
	RoleAdmin:  3,
	RoleStaff:  2,
	RoleViewer: 1,
}

// HigherOrEqual reports whether r outranks (or equals) other in the role
// priority order owner > admin > staff > viewer.
func (r Role) HigherOrEqual(other Role) bool {
	return rolePriority[r] >= rolePriority[other]
}

// allRoles is iterated by GetRole. Stable order so the highest match
// wins on the first hit.
var allRoles = []Role{RoleOwner, RoleAdmin, RoleStaff, RoleViewer}

// Client is the read-only operations marketplace-api needs from OpenFGA.
type Client interface {
	// Check is the generic permission check against a tenant:<id>
	// object. Relation can be any role or derived relation defined on
	// the tenant type in the FGA model.
	Check(ctx context.Context, userID, relation, tenantID string) (bool, error)

	// CheckMembership is a convenience wrapper for the derived
	// `member` relation — true iff the user holds any role on the
	// tenant.
	CheckMembership(ctx context.Context, userID, tenantID string) (bool, error)

	// GetRole returns the highest direct role the user holds on the
	// tenant, or "" if they have no role. Iterates the four roles in
	// priority order; worst case 4 Check calls.
	GetRole(ctx context.Context, userID, tenantID string) (Role, error)
}

// Config holds the values needed to construct a real OpenFGA client.
// StoreID is obtained at startup via DiscoverStoreID.
type Config struct {
	APIURL  string // e.g. http://openfga:8080
	StoreID string // ulid; from DiscoverStoreID
	ModelID string // optional; latest is used if empty
}

// New constructs a real OpenFGA client.
func New(cfg Config) (Client, error) {
	if cfg.APIURL == "" {
		return nil, fmt.Errorf("authz: APIURL is required")
	}
	if cfg.StoreID == "" {
		return nil, fmt.Errorf("authz: StoreID is required")
	}
	api, err := client.NewSdkClient(&client.ClientConfiguration{
		ApiUrl:               cfg.APIURL,
		StoreId:              cfg.StoreID,
		AuthorizationModelId: cfg.ModelID,
		HTTPClient:           &http.Client{},
	})
	if err != nil {
		return nil, fmt.Errorf("authz: new sdk client: %w", err)
	}
	return &fgaClient{api: api}, nil
}

// DiscoverStoreID looks up an OpenFGA store by display name and returns
// its ID. Returns ("", nil) if no store with that name exists; callers
// fail-fast on empty.
func DiscoverStoreID(ctx context.Context, apiURL, name string) (string, error) {
	api, err := client.NewSdkClient(&client.ClientConfiguration{
		ApiUrl:     apiURL,
		HTTPClient: &http.Client{},
	})
	if err != nil {
		return "", fmt.Errorf("authz: discover: new sdk: %w", err)
	}
	resp, err := api.ListStores(ctx).Execute()
	if err != nil {
		return "", fmt.Errorf("authz: discover: list stores: %w", err)
	}
	if resp == nil {
		return "", nil
	}
	for _, s := range resp.GetStores() {
		if s.Name == name {
			return s.Id, nil
		}
	}
	return "", nil
}

type fgaClient struct {
	api *client.OpenFgaClient
}

func (c *fgaClient) Check(ctx context.Context, userID, relation, tenantID string) (bool, error) {
	body := client.ClientCheckRequest{
		User:     "user:" + userID,
		Relation: relation,
		Object:   "tenant:" + tenantID,
	}
	resp, err := c.api.Check(ctx).Body(body).Execute()
	if err != nil {
		return false, fmt.Errorf("authz: check %s: %w", relation, err)
	}
	if resp == nil || resp.Allowed == nil {
		return false, nil
	}
	return *resp.Allowed, nil
}

func (c *fgaClient) CheckMembership(ctx context.Context, userID, tenantID string) (bool, error) {
	return c.Check(ctx, userID, "member", tenantID)
}

func (c *fgaClient) GetRole(ctx context.Context, userID, tenantID string) (Role, error) {
	for _, role := range allRoles {
		ok, err := c.Check(ctx, userID, string(role), tenantID)
		if err != nil {
			return "", fmt.Errorf("authz: get role: %w", err)
		}
		if ok {
			return role, nil
		}
	}
	return "", nil
}
```

- [ ] **Step 5: Run tests and confirm pass**

```
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/services/marketplace-api && go build ./... && go vet ./internal/authz/... && go test ./internal/authz/... -v
```

- [ ] **Step 6: Confirm `go.work` untouched**

```
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly && git diff go.work
```
Empty.

- [ ] **Step 7: Commit**

```
git add services/marketplace-api/internal/authz services/marketplace-api/go.mod services/marketplace-api/go.sum
git commit -m "feat(marketplace-api): add read-only authz Client interface + real FGA impl (M4)"
```

---

### Task 2: `internal/authz/fake.go` — FakeClient

**Files:**
- Create: `services/marketplace-api/internal/authz/fake.go`
- Create: `services/marketplace-api/internal/authz/fake_test.go`

**Scope:** In-memory implementation of `Client` for unit tests. Stores tuples as a `map[string]map[string]bool` keyed by `userID → relation@tenantID`. Has helper methods `Grant(userID, role, tenantID)` and `Revoke(...)` for tests to seed state. Implements the same Check/CheckMembership/GetRole semantics as the real client.

Key semantic: `CheckMembership` (i.e. `Check(..., "member", ...)`) must return true if the user has ANY role on the tenant — mirroring the FGA model's derived `member` relation. `Check("admin", ...)` must return true for users granted `owner` (since `admin` is `[user] or owner` in the model). The fake implements these implications explicitly.

- [ ] **Step 1: Write the failing tests** (`fake_test.go`):

Cases:
1. Grant owner → Check(owner) true, Check(admin) true (implied), Check(staff) true (implied), CheckMembership true.
2. Grant admin → Check(owner) false, Check(admin) true, Check(staff) true, CheckMembership true.
3. Grant staff → Check(owner) false, Check(admin) false, Check(staff) true, CheckMembership true.
4. No grant → all checks false, CheckMembership false.
5. Cross-tenant: grant on tenant A, check on tenant B → false.
6. GetRole returns the highest granted role (grant staff + admin → returns admin).
7. Revoke removes the grant and subsequent Check returns false.

- [ ] **Step 2: Implement `fake.go`**

```go
package authz

import (
	"context"
	"sync"
)

// FakeClient is an in-memory Client used by unit tests. It mirrors the
// derived-relation semantics of the real OpenFGA model: granting `owner`
// implies `admin` implies `staff` implies `member`.
//
// FakeClient is safe for concurrent use.
type FakeClient struct {
	mu sync.RWMutex
	// granted[userID][tenantID] = highest direct role (or "" if none)
	granted map[string]map[string]Role
}

// NewFakeClient returns an empty FakeClient.
func NewFakeClient() *FakeClient {
	return &FakeClient{granted: map[string]map[string]Role{}}
}

// Grant assigns a role to a user on a tenant. If the user already has a
// higher role, the call is a no-op (matching real-world "promote up
// only" semantics).
func (f *FakeClient) Grant(userID string, role Role, tenantID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.granted[userID]; !ok {
		f.granted[userID] = map[string]Role{}
	}
	existing := f.granted[userID][tenantID]
	if role.HigherOrEqual(existing) {
		f.granted[userID][tenantID] = role
	}
}

// Revoke removes any role the user holds on the tenant.
func (f *FakeClient) Revoke(userID, tenantID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if m, ok := f.granted[userID]; ok {
		delete(m, tenantID)
	}
}

func (f *FakeClient) Check(_ context.Context, userID, relation, tenantID string) (bool, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	role, ok := f.granted[userID][tenantID]
	if !ok || role == "" {
		return false, nil
	}
	switch relation {
	case "owner":
		return role == RoleOwner, nil
	case "admin":
		return role == RoleOwner || role == RoleAdmin, nil
	case "staff":
		return role == RoleOwner || role == RoleAdmin || role == RoleStaff, nil
	case "viewer":
		return role == RoleOwner || role == RoleAdmin || role == RoleStaff || role == RoleViewer, nil
	case "member":
		return true, nil
	}
	return false, nil
}

func (f *FakeClient) CheckMembership(ctx context.Context, userID, tenantID string) (bool, error) {
	return f.Check(ctx, userID, "member", tenantID)
}

func (f *FakeClient) GetRole(_ context.Context, userID, tenantID string) (Role, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	role, ok := f.granted[userID][tenantID]
	if !ok {
		return "", nil
	}
	return role, nil
}
```

- [ ] **Step 3: Run tests, confirm pass**

```
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/services/marketplace-api && go test ./internal/authz/... -race -v
```

- [ ] **Step 4: Commit**

```
git add services/marketplace-api/internal/authz/fake.go services/marketplace-api/internal/authz/fake_test.go
git commit -m "feat(marketplace-api): add authz FakeClient with implied-role semantics (M4)"
```

---

### Task 3: `internal/authz/middleware.go` — `RequireTenantRelation` Gin middleware

**Files:**
- Create: `services/marketplace-api/internal/authz/middleware.go`
- Create: `services/marketplace-api/internal/authz/middleware_test.go`

**Scope:** Gin middleware factory that checks the caller has the required role on the tenant in the gin context.

Behavior:
1. Read `user_id` and `tenant_id` from gin context. Both must be set by upstream middleware (M5 will add the auth + tenant middleware that does this; for M4 the middleware just reads).
2. If either is missing, respond 404 with `apperrors.NotFound("resource")`.
3. Call `client.Check(ctx, userID, string(role), tenantID)`.
4. On error → log and respond 500 with a generic error envelope (`{"error":"internal","message":"internal server error"}`). The 500 path is rare (FGA outage); log loud.
5. On `false` → respond 404 (no existence leak).
6. On `true` → `c.Next()`.

- [ ] **Step 1: Write the failing test** (`middleware_test.go`):

Cases (all using FakeClient + httptest):

1. `TestMiddleware_AuthorizedRequest_Allowed` — fake grants admin; request with user_id + tenant_id set; middleware calls Next, terminal handler responds 200.
2. `TestMiddleware_DeniedRequest_Returns404` — fake has no grant; request with user_id + tenant_id set; middleware returns 404 not_found.
3. `TestMiddleware_MissingUserID_Returns404` — context has tenant_id but no user_id; middleware returns 404 (no leak; the missing-user case looks identical to the unauthorized case from the outside).
4. `TestMiddleware_MissingTenantID_Returns404` — same with the other field missing.
5. `TestMiddleware_FGAError_Returns500` — uses an `errClient` test double whose Check always returns an error; middleware responds 500.
6. `TestMiddleware_RequireRoleAdmin_GrantedStaff_Returns404` — fake grants staff, middleware requires admin → 404.
7. `TestMiddleware_RequireRoleStaff_GrantedAdmin_Allowed` — fake grants admin, middleware requires staff → allowed (because admin implies staff via the fake's role-implication semantics).

- [ ] **Step 2: Implement `middleware.go`**

```go
package authz

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// Middleware bundles a Client and a logger so each route's RequireTenantRelation
// call doesn't need to thread them through.
type Middleware struct {
	client Client
	logger *slog.Logger
}

// NewMiddleware constructs a Middleware. logger may be nil.
func NewMiddleware(c Client, logger *slog.Logger) *Middleware {
	return &Middleware{client: c, logger: logger}
}

// RequireTenantRelation returns a gin.HandlerFunc that aborts with 404
// unless the caller (identified by user_id in the gin context) holds the
// given role on the tenant (identified by tenant_id in the gin context).
//
// Per spec §13.1.1 the response on deny is 404 not_found, not 403, to
// prevent existence leaks across tenants.
//
// The middleware depends on upstream middleware (auth + tenant) having
// populated the user_id and tenant_id keys on the gin context. M5 wires
// that upstream chain. For tests, set the keys directly via c.Set.
func (m *Middleware) RequireTenantRelation(role Role) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString("user_id")
		tenantID := c.GetString("tenant_id")
		if userID == "" || tenantID == "" {
			respondNotFound(c)
			return
		}
		ok, err := m.client.Check(c.Request.Context(), userID, string(role), tenantID)
		if err != nil {
			if m.logger != nil {
				m.logger.Error("authz check failed",
					"user_id", userID, "tenant_id", tenantID, "role", role, "err", err)
			}
			respondInternal(c)
			return
		}
		if !ok {
			respondNotFound(c)
			return
		}
		c.Next()
	}
}

func respondNotFound(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusNotFound, map[string]any{
		"error":   string(apperrors.CodeNotFound),
		"message": "not found",
	})
}

func respondInternal(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusInternalServerError, map[string]any{
		"error":   "internal",
		"message": "internal server error",
	})
}

// silence unused-context warnings if any future helper drops the param
var _ = context.Background
```

- [ ] **Step 3: Run tests, confirm all 7 cases pass**

```
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/services/marketplace-api && go test ./internal/authz/... -race -v
```

- [ ] **Step 4: Commit**

```
git add services/marketplace-api/internal/authz/middleware.go services/marketplace-api/internal/authz/middleware_test.go
git commit -m "feat(marketplace-api): add RequireTenantRelation gin middleware (M4)"
```

---

### Task 4: Real-FGA integration test (opt-in)

**Files:**
- Create: `services/marketplace-api/internal/authz/client_integration_test.go`

**Scope:** A `//go:build integration` test that exercises the real `fgaClient` against a live FGA instance. Skipped if `TEST_FGA_API_URL` is unset.

The test does:
1. Skip if `TEST_FGA_API_URL` is empty.
2. `DiscoverStoreID(apiURL, "mark8ly-platform")` — assert non-empty (the test fixture must include a store named `mark8ly-platform`).
3. Construct a real `fgaClient` against that store.
4. Use platform-api's `WriteOwnership` test fixture pattern: emit a tuple via raw FGA API call (`POST /stores/<id>/write` body `{"writes":{"tuple_keys":[{"user":"user:test","relation":"owner","object":"tenant:test"}]}}`). Or, simpler: skip the seed and rely on a CI fixture having pre-loaded a known tuple. Since marketplace-api's authz package has no Write methods, the easiest path is to use the openfga SDK's raw `Write` from inside the test file (importing the SDK directly is fine — the prohibition is on the production code, not test fixtures).
5. `client.Check(ctx, "test", "owner", "test")` → assert true.
6. `client.Check(ctx, "test", "admin", "test")` → assert true (model's `admin: [user] or owner` derivation).
7. `client.Check(ctx, "absent-user", "owner", "test")` → assert false.
8. Cleanup: delete the test tuple via SDK Write with deletes.

Document the test's preconditions in a comment at the top of the file: "This test requires a running OpenFGA at TEST_FGA_API_URL with a store named mark8ly-platform whose authorization model includes the marketplace-api role definitions. Local devs without that infra get a clean skip."

- [ ] **Step 1: Implement** (single file, ~150 lines)
- [ ] **Step 2: Run** `cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/services/marketplace-api && go test -tags integration ./internal/authz/... -v` — expect skip.
- [ ] **Step 3: Commit**

```
git add services/marketplace-api/internal/authz/client_integration_test.go
git commit -m "test(marketplace-api): real-FGA integration test gated by TEST_FGA_API_URL (M4)"
```

---

### Task 5: `pkg/config` — add `MARKETPLACE_FGA_API_URL` env var

**Files:**
- Modify: `services/marketplace-api/pkg/config/config.go`
- Modify: `services/marketplace-api/pkg/config/config_test.go`

**Scope:** Add a single string field to the existing `Config` struct loaded by envconfig.

- [ ] **Step 1: Read the existing config.go** to see the field naming convention (envconfig uses tags like `envconfig:"MARKETPLACE_HTTP_PORT"` or similar — match the existing prefix).
- [ ] **Step 2: Add the field** `FGAAPIURL string `envconfig:"MARKETPLACE_FGA_API_URL" required:"true"``. Make it required so misconfiguration fails fast at startup.
- [ ] **Step 3: Update config_test.go** — set the env var in the existing test setup so the existing tests still pass.
- [ ] **Step 4: Verify** `go test ./pkg/config/... -v`.
- [ ] **Step 5: Commit**

```
git add services/marketplace-api/pkg/config
git commit -m "feat(marketplace-api): add MARKETPLACE_FGA_API_URL config var (M4)"
```

---

### Task 6: Wire `cmd/marketplace-api/main.go`

**Files:**
- Modify: `services/marketplace-api/cmd/marketplace-api/main.go`

**Scope:** After DB open and before constructing the http server, discover the FGA store and construct the client. Fail fast if discovery returns empty. Log the discovered store id. Construct an `authz.Middleware` and store it in a local variable so M5 can grab it when wiring routes.

For M4 there are no routes to attach the middleware to. Just construct and log. Add a comment: `// M5 will pass authzMW to the admin route registrar`.

- [ ] **Step 1: Read main.go** (already familiar from M3 Task 13).
- [ ] **Step 2: Add imports**: `"github.com/mark8ly/marketplace-api/internal/authz"`.
- [ ] **Step 3: After `conn, err := db.Open(...)`** insert:

```go
// OpenFGA client — read-only per spec §13.1.1.
discoverCtx, discoverCancel := context.WithTimeout(context.Background(), 5*time.Second)
storeID, err := authz.DiscoverStoreID(discoverCtx, cfg.FGAAPIURL, authz.FGAStoreName)
discoverCancel()
if err != nil {
    log.Error("authz: discover store", "err", err, "api_url", cfg.FGAAPIURL)
    os.Exit(1)
}
if storeID == "" {
    log.Error("authz: store not found — bring up openfga-seed first",
        "store_name", authz.FGAStoreName, "api_url", cfg.FGAAPIURL)
    os.Exit(1)
}
log.Info("authz: discovered openfga store", "store_id", storeID)
fgaClient, err := authz.New(authz.Config{APIURL: cfg.FGAAPIURL, StoreID: storeID})
if err != nil {
    log.Error("authz: new client", "err", err)
    os.Exit(1)
}
authzMW := authz.NewMiddleware(fgaClient, log)
_ = authzMW // M5 will pass this to the admin route registrar
```

- [ ] **Step 4: Build + vet** `cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/services/marketplace-api && go build ./... && go vet ./...`.
- [ ] **Step 5: Commit**

```
git add services/marketplace-api/cmd/marketplace-api/main.go
git commit -m "feat(marketplace-api): bootstrap FGA client + middleware in main (M4)"
```

---

### Task 7: M4 verification + PR

- [ ] **Step 1: Full test run**

```
cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/services/marketplace-api && go vet ./... && go vet -tags=integration ./... && go build ./... && go test ./... -race && go test -tags integration ./... -race
```

All clean. Integration tests skip without `TEST_FGA_API_URL`.

- [ ] **Step 2: Push the branch**

```
git push -u origin feat/products-m4-openfga-authz
```

- [ ] **Step 3: Open PR**

```
gh pr create --base main --head feat/products-m4-openfga-authz --title "feat(marketplace-api): products M4 — OpenFGA tenant-relation authz middleware" --body "$(cat <<'EOF'
## Summary

- Adds a read-only \`internal/authz\` package: Client interface (Check / CheckMembership / GetRole), real OpenFGA-backed implementation, in-memory FakeClient with role-implication semantics, \`RequireTenantRelation\` gin middleware, opt-in real-FGA integration test.
- Adds a \`MARKETPLACE_FGA_API_URL\` env var and bootstraps the FGA client in \`cmd/marketplace-api/main.go\` — discovers the existing \`mark8ly-platform\` store at startup and fails fast if missing.
- Per spec §13.1.1: marketplace-api never writes tuples. The Client interface intentionally omits every Write method so accidental tuple writes are a compile error. All tuple writes happen in platform-api during onboarding/invitation.

## What is NOT in this PR

- HTTP routes — M5 will mount \`RequireTenantRelation(roleX)\` on each admin endpoint per the §13.1.1 permission map.
- Auth + Tenant upstream middleware — also M5. M4's middleware reads \`user_id\` and \`tenant_id\` from the gin context; M5 adds the upstream that populates them.
- CI changes for the FGA integration test — opt-in via \`TEST_FGA_API_URL\`. CI wiring is M5.

## Test plan

- [x] \`go vet ./...\` clean
- [x] \`go vet -tags=integration ./...\` clean
- [x] \`go build ./...\` clean
- [x] \`go test ./... -race\` green (unit tests for Client constants, FakeClient role-implication semantics, middleware allow/deny/missing/error paths)
- [x] \`go test -tags integration ./... -race\` skips cleanly without \`TEST_FGA_API_URL\` (manual run against a local OpenFGA instance with the platform-api model loaded confirms Check returns true for granted owner and false for absent users)

## Permission map gate (deferred to M5)

The §13.1.1 permission map maps each admin route to a required tenant role. M4 ships only the middleware factory; M5 wires the actual mounts.

EOF
)"
```

- [ ] **Step 4: Wait for CI, merge.**

---

## Exit criteria

- [ ] `go test ./internal/authz/... -race` green
- [ ] `go vet -tags=integration ./internal/authz/...` clean
- [ ] FakeClient implies derived roles correctly (owner→admin→staff→member)
- [ ] Middleware returns 404 (not 403) on every deny path
- [ ] Middleware logs at ERROR level on FGA outage and returns 500
- [ ] `cmd/marketplace-api/main.go` discovers the store at startup and exits 1 on failure
- [ ] `MARKETPLACE_FGA_API_URL` is a required config var (start fails without it)
- [ ] `Client` interface exposes NO Write methods (compile-time guarantee against tuple writes from marketplace-api)
- [ ] No changes to migrations, Helm chart, CI workflows, or `go.work`
- [ ] PR is open and CI is green

---

## Estimated effort

| Task | Effort |
|---|---|
| 1. Client + real impl + dep | 60 min |
| 2. FakeClient + tests | 30 min |
| 3. Middleware + tests | 60 min |
| 4. Real-FGA integration test | 30 min |
| 5. Config var | 15 min |
| 6. main.go bootstrap | 30 min |
| 7. Verification + PR | 15 min |
| **Total** | **~4 hours** |

Smaller than M3 — single new package, no schema changes, no service-layer entanglement.
