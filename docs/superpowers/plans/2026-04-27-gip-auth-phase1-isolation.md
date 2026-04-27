# GIP Auth — Phase 1: Isolation Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Split admin and storefront into distinct, browser-enforced session cookies (`m8a_session` for admin on `.mark8ly.com`, `m8c_session` for storefront on the exact request host including custom domains), bind each cookie to its app via a `Kind` discriminator, and route admin/customer auto-login through separate audience-validated endpoints — so a customer cookie cannot authorize at admin or at another store.

**Architecture:** auth-bff grows two `session.Manager` instances + two autologin endpoints (`/auth/admin/auto-login`, `/auth/customer/auto-login`). The customer manager takes the request host at mint time and stamps it as the cookie `Domain` (works for `*.mark8ly.com` and custom domains). A new partial unique index on `customer_profiles (store_id, gip_uid)` keeps the schema honest. Admin app reads both old + new admin cookies for one release window (transparent grace); storefront reads only the new customer cookie (clean cut).

**Tech Stack:** Go 1.26, Gin, `golang-migrate`, AES-GCM session cookies, Next.js 16 server actions with `headers().get('host')`, marketplace-api as customer-profile authority via a new `/internal/customers/upsert-by-gip` endpoint.

**Spec:** `docs/superpowers/specs/2026-04-27-gip-auth-isolation-merge-design.md`

**Branch policy:** all work commits directly to `main` (no PRs, no feature branches). Each task ends with a commit. CI may need the public→build→private cycle (per memory `feedback_ci_billing_workaround.md`).

---

## File structure

### Created
- `services/marketplace-api/migrations/000084_customer_profiles_gip_uid_uq.up.sql` + `.down.sql`
- `services/auth-bff/internal/autologin/customer.go` — customer-side autologin service + handler
- `services/auth-bff/internal/customers/client.go` — marketplace-api customer profile HTTP client
- `services/marketplace-api/internal/handlers/internal/customers.go` — `POST /internal/customers/upsert-by-gip`
- `apps/storefront/lib/host.ts` — request host extraction + validation helper
- `tests/e2e/auth-isolation.spec.ts` — Playwright cross-cookie isolation suite

### Modified
- `services/auth-bff/internal/session/cookie.go` — add `Kind`, `CustomerID`; add per-host `Mint` variant; reject wrong-kind reads
- `services/auth-bff/internal/session/cookie_test.go` — kind enforcement + per-host domain tests
- `services/auth-bff/internal/autologin/service.go` — split admin path; refactor common token-verify into shared helper
- `services/auth-bff/internal/autologin/handler.go` — keep `/auth/auto-login` alias (forwards to admin); add admin route
- `services/auth-bff/internal/gip/verifier.go` — verify id_token aud matches expected tenant pool
- `services/auth-bff/cmd/server/main.go` — wire two managers + two handlers + customer client
- `services/auth-bff/pkg/config/config.go` — new env vars (`SESSION_ADMIN_COOKIE_NAME`, `SESSION_CUSTOMER_COOKIE_NAME`, `GIP_INTERNAL_TENANT_ID`, `GIP_CUSTOMER_TENANT_ID`, `MARKETPLACE_API_URL`)
- `apps/admin/middleware.ts` — read `m8a_session` then fallback to `m8_session`
- `apps/admin/app/login/actions.ts` — POST to `/auth/admin/auto-login`
- `apps/admin/lib/auth/auth-bff.ts` — `autoLogin()` targets the admin endpoint
- `apps/storefront/middleware.ts` — read only `m8c_session`; validate `store_id` claim against host-resolved store
- `apps/storefront/app/create-account/actions.ts` — call new customer endpoint with request host
- `apps/storefront/app/sign-in/actions.ts` — same
- `apps/storefront/lib/api/storefront-api.ts` (or wherever auth-bff is called) — new `customerAutoLogin()` helper
- `infra/charts/auth-bff/values.yaml` (or equivalent in tesserix-k8s) — env var entries

---

## Pre-flight (one-time, before Task 1)

- [ ] **Verify migration number is still 000084.** Run `ls services/marketplace-api/migrations/ | grep '\.up\.' | sort | tail -3`. If anything has landed past `000083_shipments_pickup_columns`, bump the new migration to `000084 + N`.
- [ ] **Verify auth-bff config struct field names.** Open `services/auth-bff/pkg/config/config.go` and note the existing field for `SessionCookieName` + `SessionCookieDomain`. Plan assumes these become `SessionAdminCookieName`/`SessionAdminCookieDomain` (admin) plus new `SessionCustomerCookieName`. Adjust the plan tasks if names differ.
- [ ] **Confirm storefront has a `host` accessor available.** Open `apps/storefront/middleware.ts` and confirm `request.headers.get('host')` is reachable — same for server actions via `headers()` from `next/headers`.

---

## Task 1: Schema migration — partial unique on customer_profiles (store_id, gip_uid)

**Files:**
- Create: `services/marketplace-api/migrations/000084_customer_profiles_gip_uid_uq.up.sql`
- Create: `services/marketplace-api/migrations/000084_customer_profiles_gip_uid_uq.down.sql`

- [ ] **Step 1: Write the up migration**

```sql
-- 000084: Partial unique index on (store_id, gip_uid) so the customer
-- auto-login lookup can rely on a single row per GIP user per store.
-- WHERE clause keeps existing NULL gip_uid rows (password-only signups)
-- compatible. CONCURRENTLY avoids blocking writes in prod.
BEGIN;

CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS customer_profiles_store_gip_uid_uq
ON customer_profiles (store_id, gip_uid)
WHERE gip_uid IS NOT NULL;

COMMIT;
```

> **Note on `CONCURRENTLY` + `BEGIN`:** Postgres rejects `CREATE INDEX CONCURRENTLY` inside a transaction block. If `golang-migrate` wraps the file in a transaction, drop the `BEGIN`/`COMMIT` here and let the migrate runner handle it. Verify by checking `services/marketplace-api/migrations.go` for `MigrationsTransactional` or similar — adjust this step before applying.

- [ ] **Step 2: Write the down migration**

```sql
DROP INDEX IF EXISTS customer_profiles_store_gip_uid_uq;
```

- [ ] **Step 3: Run migrations locally and verify the index exists**

Run from `services/marketplace-api/`:
```bash
go run ./cmd/migrate up
psql "$DATABASE_URL" -c "\d customer_profiles" | grep gip_uid_uq
```
Expected: `customer_profiles_store_gip_uid_uq` listed as a partial UNIQUE index with `WHERE (gip_uid IS NOT NULL)`.

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/migrations/000084_*.sql
git commit -m "feat(marketplace-api): partial unique index on customer_profiles(store_id, gip_uid)"
```

---

## Task 2: Session struct — add Kind + CustomerID

**Files:**
- Modify: `services/auth-bff/internal/session/cookie.go:40-53`
- Modify: `services/auth-bff/internal/session/cookie_test.go`

- [ ] **Step 1: Write failing tests for Kind discriminator**

Add to `cookie_test.go`:

```go
func TestSessionKindRoundtrip(t *testing.T) {
    mgr := newTestManager(t, "m8a_session", ".mark8ly.com")
    rec := httptest.NewRecorder()

    err := mgr.Mint(rec, Session{
        UID: "u-1", Email: "a@b.com", TenantID: "t-1",
        Kind: SessionKindAdmin,
    })
    require.NoError(t, err)

    req := httptest.NewRequest("GET", "/", nil)
    for _, c := range rec.Result().Cookies() {
        req.AddCookie(c)
    }
    got, err := mgr.Read(req)
    require.NoError(t, err)
    require.NotNil(t, got)
    require.Equal(t, SessionKindAdmin, got.Kind)
}

func TestSessionKindMismatchRejected(t *testing.T) {
    mgr := newTestManager(t, "m8a_session", ".mark8ly.com")
    rec := httptest.NewRecorder()

    err := mgr.Mint(rec, Session{
        UID: "u-1", Email: "a@b.com", TenantID: "t-1",
        Kind: SessionKindCustomer, // wrong kind for an admin manager
    })
    require.ErrorIs(t, err, ErrWrongKind)
}
```

- [ ] **Step 2: Run tests, expect FAIL**

```bash
cd services/auth-bff && go test ./internal/session/ -run TestSessionKind -v
```
Expected: FAIL — `SessionKindAdmin` undefined, `Kind` field missing.

- [ ] **Step 3: Add Kind + CustomerID to Session and ErrWrongKind sentinel**

In `cookie.go`, replace the `Session` struct and add the kind type:

```go
// SessionKind discriminates admin vs customer cookies. Set at mint time;
// auth-bff refuses to mint or read a cookie whose Kind does not match
// the Manager that owns it.
type SessionKind string

const (
    SessionKindAdmin    SessionKind = "admin"
    SessionKindCustomer SessionKind = "customer"
)

// Session is the validated payload of a session cookie.
type Session struct {
    Kind       SessionKind `json:"kind"`
    UID        string      `json:"uid"`
    Email      string      `json:"email"`
    TenantID   string      `json:"tenant_id"`
    StoreID    string      `json:"store_id,omitempty"`
    CustomerID string      `json:"customer_id,omitempty"` // customer kind only
    IssuedAt   time.Time   `json:"iat"`
    ExpiresAt  time.Time   `json:"exp"`
}

var ErrWrongKind = errors.New("session: cookie kind mismatch")
```

Add a `kind SessionKind` field to `Manager` and to `Config`. In `NewManager`, default kind to admin if zero. In `Mint`, return `ErrWrongKind` when `s.Kind != m.kind`. In `Read`/`decode`, return `ErrWrongKind` when the decoded `Kind` doesn't match `m.kind`.

- [ ] **Step 4: Run tests, expect PASS**

```bash
cd services/auth-bff && go test ./internal/session/ -v
```
Expected: PASS for new tests + all existing tests still pass (Session struct addition is backward-compatible at the JSON level — old cookies that lack `kind` decode to `SessionKind("")`, which won't match either manager and gets rejected as `ErrWrongKind`. **This is the desired behavior** — existing pre-Phase-1 cookies will no longer authorize. The admin app's middleware fallback path (Task 8) handles this without forcing immediate re-sign-in.)

- [ ] **Step 5: Commit**

```bash
git add services/auth-bff/internal/session/
git commit -m "feat(auth-bff): add Kind discriminator to session cookies"
```

---

## Task 3: Per-host Mint for customer cookies

**Files:**
- Modify: `services/auth-bff/internal/session/cookie.go`
- Modify: `services/auth-bff/internal/session/cookie_test.go`

- [ ] **Step 1: Write failing test for per-host Mint**

```go
func TestMintForHostStampsDomain(t *testing.T) {
    mgr := newTestCustomerManager(t, "m8c_session") // domain unset — taken from host
    rec := httptest.NewRecorder()

    err := mgr.MintForHost(rec, "store-a.mark8ly.com", Session{
        Kind: SessionKindCustomer,
        UID: "u-1", Email: "a@b.com", TenantID: "t-1", StoreID: "s-1", CustomerID: "c-1",
    })
    require.NoError(t, err)

    cookies := rec.Result().Cookies()
    require.Len(t, cookies, 1)
    require.Equal(t, "store-a.mark8ly.com", cookies[0].Domain)
    require.Equal(t, "m8c_session", cookies[0].Name)
}

func TestMintForHostRejectsAdminManager(t *testing.T) {
    mgr := newTestManager(t, "m8a_session", ".mark8ly.com") // admin manager
    rec := httptest.NewRecorder()

    err := mgr.MintForHost(rec, "store-a.mark8ly.com", Session{
        Kind: SessionKindAdmin, UID: "u-1", TenantID: "t-1",
    })
    require.ErrorIs(t, err, ErrPerHostMintNotAllowed)
}
```

- [ ] **Step 2: Run tests, expect FAIL**

```bash
cd services/auth-bff && go test ./internal/session/ -run TestMintForHost -v
```
Expected: FAIL — `MintForHost` undefined.

- [ ] **Step 3: Implement MintForHost**

In `cookie.go`:

```go
var ErrPerHostMintNotAllowed = errors.New("session: per-host mint requires customer manager")

// MintForHost is the customer-cookie variant of Mint: it takes the
// user-facing host (e.g. "store-a.mark8ly.com" or "shop.brand-a.com")
// and stamps it verbatim as the cookie Domain so the browser will only
// send the cookie back to that exact host. No leading dot — that would
// permit cross-subdomain sends and defeat per-store isolation.
//
// The host MUST be the user-facing one (extracted from the inbound
// request's Host header by the caller); auth-bff itself sees only the
// internal cluster service hostname when called server-to-server.
func (m *Manager) MintForHost(w http.ResponseWriter, host string, s Session) error {
    if m.kind != SessionKindCustomer {
        return ErrPerHostMintNotAllowed
    }
    if host == "" {
        return errors.New("session: host is required for customer cookie")
    }
    if s.Kind != m.kind {
        return ErrWrongKind
    }
    now := time.Now()
    if s.IssuedAt.IsZero() {
        s.IssuedAt = now
    }
    if s.ExpiresAt.IsZero() {
        s.ExpiresAt = now.Add(m.maxAge)
    }
    encoded, err := m.encode(s)
    if err != nil {
        return err
    }
    http.SetCookie(w, &http.Cookie{
        Name:     m.cookieName,
        Value:    encoded,
        Path:     "/",
        Domain:   host, // exact host, no leading dot — browser-enforced isolation
        MaxAge:   int(m.maxAge.Seconds()),
        HttpOnly: true,
        Secure:   m.secure,
        SameSite: http.SameSiteLaxMode,
    })
    return nil
}
```

Add a `newTestCustomerManager` helper to the test file mirroring `newTestManager` but with `Kind: SessionKindCustomer` and `Domain: ""` (so the customer manager's `domain` field stays empty and never sneaks into a cookie).

- [ ] **Step 4: Run tests, expect PASS**

```bash
cd services/auth-bff && go test ./internal/session/ -v
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add services/auth-bff/internal/session/
git commit -m "feat(auth-bff): per-host MintForHost for customer cookies"
```

---

## Task 4: GIP audience verification

**Files:**
- Modify: `services/auth-bff/internal/gip/verifier.go`
- Modify: `services/auth-bff/internal/gip/verifier_test.go`

- [ ] **Step 1: Write failing test for audience check**

```go
func TestVerifyAcceptsExpectedAudience(t *testing.T) {
    v := newFakeVerifier(t, "mp-internal-pool-id")
    claims, err := v.Verify(context.Background(), validIDTokenForPool(t, "mp-internal-pool-id"))
    require.NoError(t, err)
    require.Equal(t, "mp-internal-pool-id", claims.Audience)
}

func TestVerifyRejectsWrongAudience(t *testing.T) {
    v := newFakeVerifier(t, "mp-internal-pool-id")
    _, err := v.Verify(context.Background(), validIDTokenForPool(t, "mp-customer-pool-id"))
    require.ErrorIs(t, err, ErrWrongAudience)
}
```

- [ ] **Step 2: Run tests, expect FAIL**

```bash
cd services/auth-bff && go test ./internal/gip/ -v
```
Expected: FAIL.

- [ ] **Step 3: Implement audience check**

In `verifier.go`, add `ExpectedAudience` to `Config`. In the verify path, after parsing claims, return `ErrWrongAudience` if `claims.Audience != cfg.ExpectedAudience` (the GIP "tenant pool ID" from `firebase.tenant` claim). Add `var ErrWrongAudience = errors.New("gip: wrong audience")`.

The current verifier likely does not enforce audience — confirm by reading `verifier.go` first. Per memory and the spec, GIP `firebase.tenant` claim is the per-pool tenant ID (distinct from "audience" in the OIDC sense, which is the project ID). The check needs to compare the **tenant claim** in the id_token to the pool we expect, not the OIDC `aud`.

> **Implementation note:** Two managers will exist — one with `ExpectedAudience: cfg.GIPInternalTenantID`, one with `ExpectedAudience: cfg.GIPCustomerTenantID`. main.go wires the right verifier into each autologin handler.

- [ ] **Step 4: Run tests, expect PASS**

```bash
cd services/auth-bff && go test ./internal/gip/ -v
```

- [ ] **Step 5: Commit**

```bash
git add services/auth-bff/internal/gip/
git commit -m "feat(auth-bff): GIP id_token tenant pool audience check"
```

---

## Task 5: marketplace-api — internal customer upsert endpoint

**Files:**
- Create: `services/marketplace-api/internal/handlers/internal/customers.go`
- Create: `services/marketplace-api/internal/handlers/internal/customers_test.go`
- Modify: `services/marketplace-api/internal/handlers/internal/routes.go` (or wherever the `/internal` group is registered)

- [ ] **Step 1: Write failing handler test**

```go
func TestUpsertByGIP_NewCustomer(t *testing.T) {
    h := newTestInternalCustomersHandler(t)
    body := `{"store_id":"<store-uuid>","email":"a@b.com","gip_uid":"gip-1","first_name":"A"}`
    req := httptest.NewRequest("POST", "/internal/customers/upsert-by-gip",
        strings.NewReader(body))
    req.Header.Set("X-Internal-Auth", testInternalSecret)
    rec := httptest.NewRecorder()
    h.ServeHTTP(rec, req)
    require.Equal(t, 200, rec.Code)
    var resp struct{ Data struct{ CustomerID string `json:"customer_id"`; TenantID string `json:"tenant_id"` } }
    require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
    require.NotEmpty(t, resp.Data.CustomerID)
    require.NotEmpty(t, resp.Data.TenantID)
}

func TestUpsertByGIP_BackfillsGipUidOnExistingEmail(t *testing.T) {
    h := newTestInternalCustomersHandler(t)
    seedCustomer(t, /*store*/ "<store-uuid>", "a@b.com", /*gip_uid*/ "")
    body := `{"store_id":"<store-uuid>","email":"a@b.com","gip_uid":"gip-1"}`
    req := httptest.NewRequest("POST", "/internal/customers/upsert-by-gip", strings.NewReader(body))
    req.Header.Set("X-Internal-Auth", testInternalSecret)
    rec := httptest.NewRecorder()
    h.ServeHTTP(rec, req)
    require.Equal(t, 200, rec.Code)
    require.Equal(t, "gip-1", lookupCustomer(t, "<store-uuid>", "a@b.com").GipUid)
}

func TestUpsertByGIP_RejectsMissingInternalAuth(t *testing.T) {
    h := newTestInternalCustomersHandler(t)
    body := `{"store_id":"<store-uuid>","email":"a@b.com","gip_uid":"gip-1"}`
    req := httptest.NewRequest("POST", "/internal/customers/upsert-by-gip", strings.NewReader(body))
    rec := httptest.NewRecorder()
    h.ServeHTTP(rec, req)
    require.Equal(t, 401, rec.Code)
}
```

- [ ] **Step 2: Run tests, expect FAIL**

```bash
cd services/marketplace-api && go test ./internal/handlers/internal/ -run TestUpsertByGIP -v
```

- [ ] **Step 3: Implement the handler**

`customers.go` exports `RegisterInternalCustomers(r *gin.RouterGroup, repo CustomerProfileRepo, internalSecret string, log *slog.Logger)`. The handler:

1. Reads `X-Internal-Auth`; rejects 401 if missing or mismatched.
2. Binds JSON `{store_id, email, gip_uid, first_name?, last_name?}`.
3. Looks up `customer_profiles` by `(store_id, gip_uid)` first (cheapest path, requires the index from Task 1).
4. If not found, looks up by `(store_id, lower(email))`. If found and `gip_uid IS NULL`, UPDATE to set gip_uid + optional name fields. Returns the row.
5. If neither match, INSERT a new row with all fields. Use `tenant_id` from `stores.tenant_id` (single subselect or pre-resolved via repo).
6. Returns `{ data: { customer_id, tenant_id } }`.

Reuse the existing `customer_profiles` model + repository (under `services/marketplace-api/internal/customer/` per the migration's domain).

- [ ] **Step 4: Run tests, expect PASS**

```bash
cd services/marketplace-api && go test ./internal/handlers/internal/ -v
```

- [ ] **Step 5: Wire the route in main**

Register inside the existing `/internal` group with the same `X-Internal-Auth` middleware pattern used by `audit-events`.

- [ ] **Step 6: Commit**

```bash
git add services/marketplace-api/internal/handlers/internal/customers*.go services/marketplace-api/internal/handlers/internal/routes.go
git commit -m "feat(marketplace-api): internal upsert-by-gip endpoint for customer profiles"
```

---

## Task 6: auth-bff — customer profile HTTP client

**Files:**
- Create: `services/auth-bff/internal/customers/client.go`
- Create: `services/auth-bff/internal/customers/client_test.go`

- [ ] **Step 1: Write failing client test (httptest-backed)**

```go
func TestUpsertByGIP_RoundTrip(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        require.Equal(t, "POST", r.Method)
        require.Equal(t, "/internal/customers/upsert-by-gip", r.URL.Path)
        require.Equal(t, "test-secret", r.Header.Get("X-Internal-Auth"))
        var body UpsertRequest
        require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
        require.Equal(t, "store-1", body.StoreID)
        json.NewEncoder(w).Encode(map[string]any{
            "data": map[string]string{"customer_id": "c-1", "tenant_id": "t-1"},
        })
    }))
    defer server.Close()
    c := New(Config{BaseURL: server.URL, InternalAuth: "test-secret"})
    res, err := c.UpsertByGIP(context.Background(), UpsertRequest{
        StoreID: "store-1", Email: "a@b.com", GIPUID: "gip-1",
    })
    require.NoError(t, err)
    require.Equal(t, "c-1", res.CustomerID)
    require.Equal(t, "t-1", res.TenantID)
}
```

- [ ] **Step 2: Run tests, expect FAIL**

```bash
cd services/auth-bff && go test ./internal/customers/ -v
```

- [ ] **Step 3: Implement client**

```go
package customers

import (
    "bytes"
    "context"
    "encoding/json"
    "errors"
    "fmt"
    "net/http"
    "time"
)

type Config struct {
    BaseURL      string
    InternalAuth string
    Timeout      time.Duration // defaults to 5s
}

type Client struct {
    baseURL string
    secret  string
    http    *http.Client
}

func New(cfg Config) *Client {
    timeout := cfg.Timeout
    if timeout == 0 {
        timeout = 5 * time.Second
    }
    return &Client{
        baseURL: cfg.BaseURL,
        secret:  cfg.InternalAuth,
        http:    &http.Client{Timeout: timeout},
    }
}

type UpsertRequest struct {
    StoreID   string `json:"store_id"`
    Email     string `json:"email"`
    GIPUID    string `json:"gip_uid"`
    FirstName string `json:"first_name,omitempty"`
    LastName  string `json:"last_name,omitempty"`
}

type UpsertResponse struct {
    CustomerID string
    TenantID   string
}

var ErrStoreNotFound = errors.New("customers: store not found")

func (c *Client) UpsertByGIP(ctx context.Context, req UpsertRequest) (*UpsertResponse, error) {
    body, err := json.Marshal(req)
    if err != nil {
        return nil, err
    }
    httpReq, err := http.NewRequestWithContext(ctx, "POST",
        c.baseURL+"/internal/customers/upsert-by-gip", bytes.NewReader(body))
    if err != nil {
        return nil, err
    }
    httpReq.Header.Set("Content-Type", "application/json")
    httpReq.Header.Set("X-Internal-Auth", c.secret)
    res, err := c.http.Do(httpReq)
    if err != nil {
        return nil, err
    }
    defer res.Body.Close()
    if res.StatusCode == 404 {
        return nil, ErrStoreNotFound
    }
    if res.StatusCode != 200 {
        return nil, fmt.Errorf("customers: upsert returned %d", res.StatusCode)
    }
    var wrapper struct {
        Data UpsertResponse `json:"data"`
    }
    if err := json.NewDecoder(res.Body).Decode(&wrapper); err != nil {
        return nil, err
    }
    return &wrapper.Data, nil
}
```

- [ ] **Step 4: Run tests, expect PASS**

- [ ] **Step 5: Commit**

```bash
git add services/auth-bff/internal/customers/
git commit -m "feat(auth-bff): customer profile client (POST /internal/customers/upsert-by-gip)"
```

---

## Task 7: auth-bff — split autologin into admin + customer paths

**Files:**
- Modify: `services/auth-bff/internal/autologin/service.go`
- Modify: `services/auth-bff/internal/autologin/handler.go`
- Create: `services/auth-bff/internal/autologin/customer.go`
- Modify: `services/auth-bff/internal/autologin/service_test.go`

- [ ] **Step 1: Write failing test for customer auto-login**

```go
func TestCustomerAutoLogin_NewCustomer_MintsHostScopedCookie(t *testing.T) {
    svc := newTestCustomerAutoLogin(t,
        withFakeGIP("gip-1", "a@b.com", "mp-customer-pool"),
        withFakeUpsert("c-1", "t-1"),
        withCustomerSessions("m8c_session"))

    rec := httptest.NewRecorder()
    res, err := svc.CustomerAutoLogin(context.Background(), rec, CustomerRequest{
        IDToken: "valid-customer-token",
        Host:    "store-a.mark8ly.com",
        StoreID: "s-1",
    })
    require.NoError(t, err)
    require.Equal(t, "c-1", res.CustomerID)
    require.Equal(t, "t-1", res.TenantID)

    cookies := rec.Result().Cookies()
    require.Len(t, cookies, 1)
    require.Equal(t, "m8c_session", cookies[0].Name)
    require.Equal(t, "store-a.mark8ly.com", cookies[0].Domain)
}

func TestCustomerAutoLogin_RejectsAdminPoolToken(t *testing.T) {
    svc := newTestCustomerAutoLogin(t,
        withFakeGIP("gip-1", "a@b.com", "mp-internal-pool"))
    rec := httptest.NewRecorder()
    _, err := svc.CustomerAutoLogin(context.Background(), rec, CustomerRequest{
        IDToken: "admin-pool-token", Host: "store-a.mark8ly.com", StoreID: "s-1",
    })
    require.ErrorIs(t, err, ErrTokenInvalid) // wraps ErrWrongAudience
}

func TestCustomerAutoLogin_BlankHostRejected(t *testing.T) {
    svc := newTestCustomerAutoLogin(t)
    rec := httptest.NewRecorder()
    _, err := svc.CustomerAutoLogin(context.Background(), rec, CustomerRequest{
        IDToken: "valid", Host: "", StoreID: "s-1",
    })
    require.Error(t, err)
}
```

- [ ] **Step 2: Run tests, expect FAIL**

```bash
cd services/auth-bff && go test ./internal/autologin/ -v
```

- [ ] **Step 3: Implement CustomerAutoLogin**

`customer.go` defines `CustomerRequest` (`IDToken`, `Host`, `StoreID` — `StoreID` derived from host by the caller, passed to keep service stateless), `CustomerResponse` (`UID`, `Email`, `TenantID`, `StoreID`, `CustomerID`), and `Service.CustomerAutoLogin(ctx, w, req)`. Flow:

1. Verify id_token via the customer-pool verifier (separate `gip.Verifier` instance with `ExpectedAudience = cfg.GIPCustomerTenantID`). On mismatch, wrap as `ErrTokenInvalid`.
2. Reject blank `Host` or blank `StoreID` (caller error).
3. Call `customers.Client.UpsertByGIP(ctx, ...)` with the email and gip_uid from the verified token claims.
4. Build `Session{Kind: SessionKindCustomer, UID, Email, TenantID, StoreID, CustomerID}`.
5. Call `customerSessions.MintForHost(w, req.Host, sess)`.

The existing `Service.AutoLogin` becomes the admin path. Either rename it to `AdminAutoLogin` (preferred) or leave as-is and have `CustomerAutoLogin` parallel it. Pick rename — clarity beats compatibility for an internal service method.

Wire two `Service` instances? Or one service with both methods? **One service with both methods**: `Config` grows `AdminGIP`, `CustomerGIP`, `AdminSessions`, `CustomerSessions`, `Customers *customers.Client`. Cleaner than two parallel services.

- [ ] **Step 4: Run tests, expect PASS**

- [ ] **Step 5: Commit**

```bash
git add services/auth-bff/internal/autologin/
git commit -m "feat(auth-bff): split autologin into admin + customer paths with separate GIP pools"
```

---

## Task 8: auth-bff — register split routes + alias

**Files:**
- Modify: `services/auth-bff/internal/autologin/handler.go`
- Modify: `services/auth-bff/cmd/server/main.go`
- Modify: `services/auth-bff/pkg/config/config.go`

- [ ] **Step 1: Write failing handler integration test**

```go
func TestRoutes_AdminAutoLogin(t *testing.T) {
    r := newTestRouter(t)
    body := `{"id_token":"valid-admin","expected_tenant_id":"mp-internal-pool","workspace_tenant":"t-1"}`
    req := httptest.NewRequest("POST", "/auth/admin/auto-login", strings.NewReader(body))
    rec := httptest.NewRecorder()
    r.ServeHTTP(rec, req)
    require.Equal(t, 200, rec.Code)
}

func TestRoutes_LegacyAutoLoginAliasesAdmin(t *testing.T) {
    r := newTestRouter(t)
    body := `{"id_token":"valid-admin","expected_tenant_id":"mp-internal-pool","workspace_tenant":"t-1"}`
    req := httptest.NewRequest("POST", "/auth/auto-login", strings.NewReader(body))
    rec := httptest.NewRecorder()
    r.ServeHTTP(rec, req)
    require.Equal(t, 200, rec.Code)
}

func TestRoutes_CustomerAutoLogin(t *testing.T) {
    r := newTestRouter(t)
    body := `{"id_token":"valid-customer","host":"store-a.mark8ly.com","store_id":"s-1"}`
    req := httptest.NewRequest("POST", "/auth/customer/auto-login", strings.NewReader(body))
    rec := httptest.NewRecorder()
    r.ServeHTTP(rec, req)
    require.Equal(t, 200, rec.Code)
}
```

- [ ] **Step 2: Run tests, expect FAIL**

```bash
cd services/auth-bff && go test ./internal/autologin/ -run TestRoutes -v
```

- [ ] **Step 3: Update routes**

In `handler.go`:

```go
func (h *Handler) Register(r *gin.RouterGroup) {
    r.POST("/admin/auto-login", h.adminAutoLogin)
    r.POST("/customer/auto-login", h.customerAutoLogin)
    // Transparent alias for one release window — drop in Phase 1.5.
    r.POST("/auto-login", h.adminAutoLogin)
}
```

Add `customerAutoLogin` and `adminAutoLogin` (the latter is the renamed-existing). The customer handler reads `{ id_token, host, store_id }` JSON.

- [ ] **Step 4: Add config entries**

In `pkg/config/config.go`, add:

```go
SessionAdminCookieName    string // default "m8a_session"
SessionAdminCookieDomain  string // default ".mark8ly.com"
SessionCustomerCookieName string // default "m8c_session"
GIPInternalTenantID       string
GIPCustomerTenantID       string
MarketplaceAPIURL         string // already exists — verify
MarketplaceInternalAuthSecret string // already exists — reuse
```

Migrate the existing `SessionCookieName` / `SessionCookieDomain` to the admin variants and delete the old fields. Update `config_test.go`.

- [ ] **Step 5: Wire main.go**

In `cmd/server/main.go`, replace the single session manager + autologin wiring with:

```go
adminVerifier, err := gip.New(ctx, gip.Config{
    ProjectID: cfg.GIPProjectID, ExpectedAudience: cfg.GIPInternalTenantID})
// ... err check
customerVerifier, err := gip.New(ctx, gip.Config{
    ProjectID: cfg.GIPProjectID, ExpectedAudience: cfg.GIPCustomerTenantID})
// ... err check

adminSessions, err := session.NewManager(session.Config{
    Kind: session.SessionKindAdmin,
    CookieName: cfg.SessionAdminCookieName,
    Domain: cfg.SessionAdminCookieDomain,
    Secure: cfg.Env != "dev",
    EncryptKey: cfg.SessionEncryptKey,
})
customerSessions, err := session.NewManager(session.Config{
    Kind: session.SessionKindCustomer,
    CookieName: cfg.SessionCustomerCookieName,
    Domain: "", // request-driven via MintForHost
    Secure: cfg.Env != "dev",
    EncryptKey: cfg.SessionEncryptKey,
})

customersClient := customers.New(customers.Config{
    BaseURL: cfg.MarketplaceAPIURL,
    InternalAuth: cfg.MarketplaceInternalAuthSecret,
})

autologinSvc := autologin.NewService(autologin.Config{
    AdminGIP: adminVerifier,
    CustomerGIP: customerVerifier,
    FGA: fgaClient,
    AdminSessions: adminSessions,
    CustomerSessions: customerSessions,
    Customers: customersClient,
    Registry: sessionRegistry,
    MFA: mfaSvc,
    Audit: auditClient,
    Logger: log,
})

// session.Handler still binds to admin manager only — getSession,
// switchTenant, switchStore are admin-only operations.
sessionHandler := session.NewHandler(adminSessions, fgaClient).
    WithRegistry(sessionRegistry, log).
    WithMFA(mfaSvc).
    WithAudit(auditClient)
```

- [ ] **Step 6: Run all auth-bff tests**

```bash
cd services/auth-bff && go test ./... -v
```
Expected: PASS.

- [ ] **Step 7: Build the binary**

```bash
cd services/auth-bff && go build ./...
```
Expected: clean build.

- [ ] **Step 8: Commit**

```bash
git add services/auth-bff/
git commit -m "feat(auth-bff): wire split admin/customer autologin endpoints + per-pool GIP verifiers"
```

---

## Task 9: admin app — middleware reads m8a_session with m8_session fallback

**Files:**
- Modify: `apps/admin/middleware.ts`
- Modify: `apps/admin/middleware.test.ts` (or create if missing)

- [ ] **Step 1: Write failing test for the fallback path**

If admin has no middleware test today, add a tiny one using a thin wrapper:

```ts
import { describe, it, expect } from "vitest";
import { resolveSessionCookie } from "./lib/auth/session-resolver";

describe("resolveSessionCookie", () => {
  it("prefers m8a_session over m8_session", () => {
    const c = new Headers({ cookie: "m8_session=old; m8a_session=new" });
    expect(resolveSessionCookie(c)).toBe("new");
  });
  it("falls back to m8_session when m8a_session is missing", () => {
    const c = new Headers({ cookie: "m8_session=old" });
    expect(resolveSessionCookie(c)).toBe("old");
  });
  it("returns null when neither is present", () => {
    expect(resolveSessionCookie(new Headers())).toBeNull();
  });
});
```

- [ ] **Step 2: Run, expect FAIL**

```bash
cd apps/admin && npm test -- session-resolver
```

- [ ] **Step 3: Extract `resolveSessionCookie` helper**

`apps/admin/lib/auth/session-resolver.ts`:

```ts
const COOKIE_PRIMARY = "m8a_session";
const COOKIE_LEGACY = "m8_session";

export function resolveSessionCookie(headers: Headers): string | null {
  const cookieHeader = headers.get("cookie");
  if (!cookieHeader) return null;
  const parts = cookieHeader.split(";").map((p) => p.trim());
  const map = new Map<string, string>();
  for (const part of parts) {
    const eq = part.indexOf("=");
    if (eq < 0) continue;
    map.set(part.slice(0, eq), part.slice(eq + 1));
  }
  return map.get(COOKIE_PRIMARY) ?? map.get(COOKIE_LEGACY) ?? null;
}
```

Update `apps/admin/middleware.ts` to call `resolveSessionCookie(request.headers)` wherever it currently reads `m8_session` directly.

- [ ] **Step 4: Run tests, expect PASS**

- [ ] **Step 5: Commit**

```bash
git add apps/admin/lib/auth/session-resolver.ts apps/admin/middleware.ts
git commit -m "feat(admin): read m8a_session with m8_session fallback for one release"
```

---

## Task 10: admin app — login actions call /auth/admin/auto-login

**Files:**
- Modify: `apps/admin/lib/auth/auth-bff.ts`

- [ ] **Step 1: Update endpoint URL**

Find the `autoLogin` function in `apps/admin/lib/auth/auth-bff.ts`. It currently POSTs to `${AUTH_BFF_URL}/auth/auto-login`. Change to `/auth/admin/auto-login`.

The body shape stays the same (`{ id_token, expected_tenant_id, workspace_tenant }`).

- [ ] **Step 2: Run admin tests**

```bash
cd apps/admin && npm test
```
Expected: PASS (any tests that mock the URL must be updated to the new path).

- [ ] **Step 3: Commit**

```bash
git add apps/admin/lib/auth/auth-bff.ts
git commit -m "feat(admin): point autoLogin at /auth/admin/auto-login"
```

---

## Task 11: storefront — host extraction helper

**Files:**
- Create: `apps/storefront/lib/host.ts`
- Create: `apps/storefront/lib/host.test.ts`

- [ ] **Step 1: Write failing tests**

```ts
import { describe, it, expect } from "vitest";
import { sanitizeHost } from "./host";

describe("sanitizeHost", () => {
  it("strips port", () => {
    expect(sanitizeHost("store-a.mark8ly.com:443")).toBe("store-a.mark8ly.com");
  });
  it("rejects empty", () => {
    expect(sanitizeHost("")).toBeNull();
    expect(sanitizeHost(null)).toBeNull();
  });
  it("rejects suspicious chars", () => {
    expect(sanitizeHost("store-a.mark8ly.com/evil")).toBeNull();
    expect(sanitizeHost("store-a..mark8ly.com")).toBeNull();
  });
  it("accepts custom domains", () => {
    expect(sanitizeHost("shop.brand-a.com")).toBe("shop.brand-a.com");
  });
});
```

- [ ] **Step 2: Run, expect FAIL**

```bash
cd apps/storefront && npm test -- host
```

- [ ] **Step 3: Implement**

```ts
// apps/storefront/lib/host.ts
//
// sanitizeHost normalizes the inbound Host header for use as a cookie
// Domain. Strips :port. Rejects anything that is not a plain hostname
// (no path chars, no consecutive dots, no leading/trailing dot).
//
// The output is fed verbatim into Set-Cookie Domain=, so an unsafe
// host MUST return null and the caller MUST refuse to mint.
export function sanitizeHost(raw: string | null | undefined): string | null {
  if (!raw) return null;
  const noPort = raw.split(":")[0] ?? "";
  if (!noPort) return null;
  if (noPort.includes("..") || noPort.startsWith(".") || noPort.endsWith(".")) {
    return null;
  }
  if (!/^[a-zA-Z0-9.-]+$/.test(noPort)) return null;
  return noPort;
}
```

- [ ] **Step 4: Run tests, expect PASS**

- [ ] **Step 5: Commit**

```bash
git add apps/storefront/lib/host.ts apps/storefront/lib/host.test.ts
git commit -m "feat(storefront): host sanitizer for per-host cookie minting"
```

---

## Task 12: storefront — actions call /auth/customer/auto-login with host

**Files:**
- Modify: `apps/storefront/app/create-account/actions.ts`
- Modify: `apps/storefront/app/sign-in/actions.ts`
- Modify: (or create) `apps/storefront/lib/api/auth-bff.ts` — `customerAutoLogin()` helper

- [ ] **Step 1: Add `customerAutoLogin` helper**

```ts
// apps/storefront/lib/api/auth-bff.ts
import { sanitizeHost } from "@/lib/host";

const AUTH_BFF_URL = process.env.AUTH_BFF_URL ?? "http://localhost:8087";

export interface CustomerAutoLoginInput {
  idToken: string;
  host: string;     // request host (already sanitized)
  storeId: string;
}

export interface CustomerAutoLoginResult {
  uid: string;
  email: string;
  tenantId: string;
  storeId: string;
  customerId: string;
  setCookies: string[]; // Set-Cookie header values from auth-bff
}

export class AuthBffError extends Error {
  constructor(public code: string, message: string) { super(message); }
}

export async function customerAutoLogin(
  input: CustomerAutoLoginInput,
): Promise<CustomerAutoLoginResult> {
  const safeHost = sanitizeHost(input.host);
  if (!safeHost) throw new AuthBffError("invalid_host", "Host could not be validated");

  const res = await fetch(`${AUTH_BFF_URL}/auth/customer/auto-login`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      id_token: input.idToken,
      host: safeHost,
      store_id: input.storeId,
    }),
    cache: "no-store",
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({})) as { error?: string; message?: string };
    throw new AuthBffError(body.error ?? "auth_failed", body.message ?? `HTTP ${res.status}`);
  }
  const wrapper = (await res.json()) as { data: { uid: string; email: string; tenant_id: string; store_id: string; customer_id: string } };
  return {
    uid: wrapper.data.uid,
    email: wrapper.data.email,
    tenantId: wrapper.data.tenant_id,
    storeId: wrapper.data.store_id,
    customerId: wrapper.data.customer_id,
    setCookies: res.headers.getSetCookie(),
  };
}
```

- [ ] **Step 2: Update create-account action**

In `apps/storefront/app/create-account/actions.ts`:

1. Import `headers` from `next/headers` and `customerAutoLogin` from the new helper.
2. Replace the existing customerSignUp call with the new flow: read host via `(await headers()).get("host")`, resolve store_id via existing `resolveStoreSlug` + `fetchStoreBySlug`, call `customerAutoLogin({ idToken, host, storeId })`.
3. Proxy `setCookies` to the response via `cookies().set(...)` using the existing `parseSetCookie` helper (port from admin actions if not present).

- [ ] **Step 3: Update sign-in action**

Same pattern in `apps/storefront/app/sign-in/actions.ts`.

- [ ] **Step 4: Run storefront tests**

```bash
cd apps/storefront && npm test
```

- [ ] **Step 5: Commit**

```bash
git add apps/storefront/lib/api/auth-bff.ts apps/storefront/app/create-account/actions.ts apps/storefront/app/sign-in/actions.ts
git commit -m "feat(storefront): customer auto-login via /auth/customer/auto-login with request host"
```

---

## Task 13: storefront — middleware reads only m8c_session, validates store_id

**Files:**
- Modify: `apps/storefront/middleware.ts`

- [ ] **Step 1: Add cookie name + store-id-claim validation**

The middleware must:

1. Read ONLY `m8c_session` (no m8_session fallback). Storefront customer cookies don't predate this phase.
2. Decode the cookie? **No** — the storefront cannot decode auth-bff's encrypted cookie. Instead, call `auth-bff GET /auth/session` (the existing route — reads the cookie via the admin manager today). After Task 8, expand `getSession` to accept either kind based on the cookie name on the request. **Simpler alternative**: have storefront middleware call a dedicated `GET /auth/customer/session` that auth-bff serves via the customer manager.

Pick the dedicated endpoint:

In `services/auth-bff/internal/session/handler.go`, add:

```go
func (h *Handler) Register(r *gin.RouterGroup) {
    r.GET("/session", h.getSession)
    r.GET("/customer/session", h.getCustomerSession) // NEW
    // ... rest unchanged
}

func (h *Handler) getCustomerSession(c *gin.Context) {
    s, err := h.customerMgr.Read(c.Request)
    // ... same response shape as getSession but include store_id + customer_id
}
```

This requires `Handler` to also hold `customerMgr *Manager`. Update `NewHandler` signature in main.go.

3. Storefront middleware compares `s.store_id` to `resolveStoreSlug(host)` → `fetchStoreBySlug(slug).id`. Mismatch → clear cookie + redirect.

- [ ] **Step 2: Update auth-bff session handler tests + main.go to wire customerMgr**

Add a `WithCustomerManager(mgr *Manager)` builder method on `Handler` so `cmd/server/main.go` can pass the customer manager in:

```go
sessionHandler := session.NewHandler(adminSessions, fgaClient).
    WithCustomerManager(customerSessions).
    WithRegistry(sessionRegistry, log).
    WithMFA(mfaSvc).
    WithAudit(auditClient)
```

- [ ] **Step 3: Implement storefront middleware changes**

In `apps/storefront/middleware.ts`:

1. Read cookie `m8c_session`; if absent, treat as logged-out (redirect to `/sign-in` for protected routes; let public routes through).
2. For protected routes, fetch `${AUTH_BFF_URL}/auth/customer/session` forwarding the cookie header. If 200 with `store_id` matching the current host's store, allow. Else clear `m8c_session` + redirect to `/sign-in?reason=invalid_session`.

Cache the host→store_id resolution per request to avoid double round trips.

- [ ] **Step 4: Run storefront tests**

```bash
cd apps/storefront && npm test
```

- [ ] **Step 5: Build storefront**

```bash
cd apps/storefront && npm run build
```
Expected: clean build.

- [ ] **Step 6: Commit**

```bash
git add apps/storefront/middleware.ts services/auth-bff/internal/session/handler.go services/auth-bff/cmd/server/main.go
git commit -m "feat(storefront,auth-bff): customer session endpoint + store_id binding in storefront middleware"
```

---

## Task 14: E2E — auth-isolation.spec.ts

**Files:**
- Create: `tests/e2e/auth-isolation.spec.ts`

- [ ] **Step 1: Write the spec**

Per memory `e2e_test_state.md`, the E2E suite uses Playwright. Add:

```ts
import { test, expect } from "@playwright/test";

test.describe("auth isolation", () => {
  test("admin cookie does not authorize at storefront host", async ({ browser }) => {
    const ctx = await browser.newContext();
    const page = await ctx.newPage();
    await signInAsAdmin(page); // helper from existing suite
    const adminCookies = await ctx.cookies();
    expect(adminCookies.find((c) => c.name === "m8a_session")).toBeTruthy();

    await page.goto(`${process.env.STOREFRONT_BASE_URL}/account`);
    await expect(page).toHaveURL(/\/sign-in/); // redirected
    await ctx.close();
  });

  test("customer cookie at store-a does not work at store-b", async ({ browser }) => {
    const ctx = await browser.newContext();
    const page = await ctx.newPage();
    await signUpAsCustomer(page, /* store: */ "store-a");

    await page.goto(`${storeBaseUrl("store-b")}/account`);
    await expect(page).toHaveURL(/\/sign-in/);
    await ctx.close();
  });

  test("customer cookie has Domain=exact host (no leading dot)", async ({ browser }) => {
    const ctx = await browser.newContext();
    const page = await ctx.newPage();
    await signUpAsCustomer(page, "store-a");
    const cookies = await ctx.cookies();
    const customer = cookies.find((c) => c.name === "m8c_session");
    expect(customer).toBeTruthy();
    expect(customer!.domain).toBe(new URL(storeBaseUrl("store-a")).hostname);
    expect(customer!.domain.startsWith(".")).toBe(false);
    await ctx.close();
  });
});
```

Reuse helpers from existing E2E (`signInAsAdmin`, `signUpAsCustomer`) — port from existing prod-readiness journey if present, otherwise add minimal helpers in `tests/e2e/helpers/`.

- [ ] **Step 2: Run locally against the dev stack**

```bash
npm run e2e -- tests/e2e/auth-isolation.spec.ts
```

If the dev stack isn't running with two store subdomains, document the prerequisite at the top of the file (e.g. "requires `store-a` and `store-b` tenants seeded via the onboarding wizard or seed script").

- [ ] **Step 3: Commit**

```bash
git add tests/e2e/auth-isolation.spec.ts
git commit -m "test(e2e): cross-cookie isolation for admin/storefront/per-store"
```

---

## Task 15: tesserix-k8s — env var entries for new cookie + GIP config

**Files:**
- Modify: `tesserix-k8s/charts/apps/mark8ly-auth-bff/values.yaml`
- Modify: `tesserix-k8s/charts/apps/mark8ly-auth-bff/templates/deployment.yaml` (if env vars need wiring)
- Modify: `tesserix-k8s/charts/apps/mark8ly-storefront/values.yaml` (if storefront needs new env)

- [ ] **Step 1: Add env vars to auth-bff values**

```yaml
env:
  SESSION_ADMIN_COOKIE_NAME: m8a_session
  SESSION_ADMIN_COOKIE_DOMAIN: .mark8ly.com
  SESSION_CUSTOMER_COOKIE_NAME: m8c_session
  GIP_INTERNAL_TENANT_ID: "MP-Internal-e986p"   # confirm exact id from prod GIP console
  GIP_CUSTOMER_TENANT_ID: "MP-Customer-XXXXX"   # CONFIRM in console before applying
  MARKETPLACE_API_URL: http://marketplace-api.mark8ly.svc.cluster.local
  MARKETPLACE_INTERNAL_AUTH_SECRET: <ref to existing secret>
```

- [ ] **Step 2: Verify the legacy SESSION_COOKIE_* env vars are removed (or aliased)**

If something still reads them in main.go, the build will fail at startup. Run the auth-bff binary locally with the new env to confirm it boots cleanly.

- [ ] **Step 3: Commit in tesserix-k8s repo**

```bash
cd ../tesserix-k8s
git add charts/apps/mark8ly-auth-bff/values.yaml
git commit -m "chore(mark8ly-auth-bff): split admin/customer cookie + GIP tenant pool env vars"
git push origin main
```

(ArgoCD will pick this up. The mark8ly side image bump arrives via the bump-k8s CI job once Tasks 1-14 land on `mark8ly@main`.)

---

## Verification & cutover

After all 15 tasks land and the bump-k8s job propagates:

- [ ] **Verify ArgoCD sync.** `kubectl -n argocd get application mark8ly-auth-bff mark8ly-storefront mark8ly-admin` — all should be `Synced + Healthy` on the new revision.
- [ ] **Smoke test admin sign-in.** Go to `https://admin.mark8ly.com/login`, sign in with password, confirm `m8a_session` cookie set on `.mark8ly.com`.
- [ ] **Smoke test storefront sign-in.** Pick a real store, sign in with password, confirm `m8c_session` cookie set on the exact store host (no leading dot).
- [ ] **Smoke test custom domain (if a store has one).** Sign in on `shop.brand-a.com`, confirm cookie domain.
- [ ] **Test isolation.** Customer cookie from store-a should NOT authorize at store-b — this is exactly the e2e test from Task 14, but verify by hand once.
- [ ] **Verify the legacy `m8_session` fallback works for one existing admin user.** A pre-Phase-1 admin session should still let the user in (alias path). After their session expires (~7 days) they'll naturally re-sign-in and get `m8a_session`.

After two weeks (one full session lifetime), drop the legacy alias:

- [ ] **Phase 1.5 cleanup**: remove `/auth/auto-login` alias from auth-bff handler; remove `m8_session` fallback from `apps/admin/lib/auth/session-resolver.ts`. Commit.

---

## Rollback

Each task is independently reversible via `git revert`. The schema migration (Task 1) is safe to leave in place even after a code rollback — it doesn't break the old cookie-handling code. Only Task 8 (route changes) and Task 13 (middleware changes) cause user-visible disruption when reverted; everything else is a code-only change.

If a deploy fails badly post-cutover, the recovery is:
1. Revert Tasks 8-13 in the mark8ly repo.
2. Push, wait for bump-k8s, ArgoCD sync.
3. Existing `m8_session` cookies remain valid; new sessions mint via the legacy code path.
