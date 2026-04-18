# P14 — Enterprise API Keys (Pro R/W API + Studio Read-Only) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship the server-side API-key issuance + authentication surface that lets Pro merchants grant third-party integrations read/write access to their store and Studio merchants grant read-only access. Keys are 32 bytes of CSPRNG entropy, base58-encoded, prefixed `mk8_live_`, stored as bcrypt hashes with a plaintext `key_prefix` for O(log n) lookup, bound to a tenant + store with per-key scopes + rate limits, revocable instantly, and rotatable with a 24h overlap window.

**Architecture:** A new `internal/apikeys` package owns key generation, hashing, lookup, and rotation. A new `apikeys.Middleware` sits on the public R/W API router group (`/api/v1/**`) — **parallel to** the admin router (the admin uses `IstioAuth`/`HeaderTrustAuth`, the public API uses bearer keys). The middleware looks up keys by prefix, performs a timing-safe bcrypt verify, populates the Gin context with `tenant_id` + `scopes` + `rate_limit`, runs the rate-limiter, then hands off to `scope.RequireScope(...)` per-route. Admin endpoints for managing keys live under `/admin/stores/:storeId/api-keys` and are gated by `plangate.RequireFeature(FeatureFullAPI)` for write endpoints (create/rotate) and `FeatureReadAPI` for read endpoints (list keys) — matching the spec intent that read-only keys are a Studio+ feature but issuing write keys requires Pro. Observability piggybacks on existing counters; audit events flow through the P1 `audit.Emitter`.

**Tech Stack:** Go 1.26, Gin, GORM, PostgreSQL (new table via golang-migrate), `golang.org/x/crypto/bcrypt`, `crypto/rand`, `crypto/subtle`, `github.com/mr-tron/base58` (add to `go.mod`), existing `internal/plangate` (P3), `internal/ratelimit` (existing token-bucket package), `internal/audit` (P1), `internal/ipprivacy` HMAC helper (P8). An in-memory LRU verified-hash cache (60s TTL) avoids running bcrypt on every request for hot keys.

**Spec:** [`docs/superpowers/specs/2026-04-17-subscription-model-design.md`](../specs/2026-04-17-subscription-model-design.md) — §9 feature-matrix row "Read-only API + webhooks (rate-limited) — Studio+" and "Full read/write API — Pro only"; §18.4 (32-byte entropy, `mk8_live_` prefix, bcrypt hash, per-key scopes + rate limits + tenant binding, immediate revocation, 24h rotation overlap).

**Depends on:** P1 (subscription data model + `audit.Emitter` + `EmitStateTransition`/`EmitSecurity`), P3 (`plangate.RequireFeature(FeatureFullAPI)` and `FeatureReadAPI`), existing `internal/ratelimit/` package, existing `internal/ipprivacy` (P8) HMAC IP hashing.

**Related plans:**
- **P16** (admin UI) — consumes the endpoints in Task 3 to render a key-management page; out of scope here.
- **P17** (observability) — reads the new `api.key.used` / `api.key.rate_limited` counters introduced in Task 9.

---

## Scope Check

In scope:
1. `enterprise_api_keys` table + migration + GORM model + repository.
2. Key generation (CSPRNG → base58 → `mk8_live_` prefix), bcrypt hashing (cost 12), prefix extraction, display formatting.
3. Admin HTTP endpoints: create, list, rotate, revoke — gated on plan via `plangate.RequireFeature`.
4. API authentication middleware: bearer extraction, timing-safe lookup + bcrypt verify (with dummy-hash fallback on lookup miss), rotation-overlap tolerance, revocation check, context population.
5. Scope-enforcement middleware: route-level `scope.RequireScope("products:write")` helper comparing declared scope against the key's `scopes` array.
6. Per-key rate limiting via the existing `internal/ratelimit/` package, keyed by API-key ID, default 100 req/min, configurable at creation (capped by plan).
7. Async `last_used_at` + `last_used_ip_hash` update, fire-and-forget via a small worker channel.
8. Observability counters + audit events (create/rotate/revoke = info, rate-limit-exceeded = warning, auth failures = warning).
9. Security tests: revoked rejected, rotation overlap honored, scope mismatch → 403, timing parity between prefix-miss and prefix-hit-hash-miss.

Out of scope:
- Admin UI for key management — **P16**.
- OAuth 2.0 for third-party apps with per-merchant consent flows — future work.
- Webhook signing / HMAC for outbound events — future work (v1 only handles inbound API auth).
- Public API endpoint implementations themselves — those exist or will be implemented per feature area (products, orders, customers, etc.); this plan only adds the auth/scope gate the routes will attach.
- Cross-tenant key sharing, dev/test environment `mk8_test_` keys (the code supports the prefix but the "test env" pathway is not built here).

---

## File Structure

### Create

- `services/marketplace-api/migrations/NNNN_create_enterprise_api_keys.up.sql`
- `services/marketplace-api/migrations/NNNN_create_enterprise_api_keys.down.sql`
- `services/marketplace-api/internal/apikeys/model.go` — GORM model + status helpers
- `services/marketplace-api/internal/apikeys/generate.go` — CSPRNG + base58 + prefix extraction
- `services/marketplace-api/internal/apikeys/generate_test.go`
- `services/marketplace-api/internal/apikeys/hash.go` — bcrypt wrappers (cost 12 + dummy hash constant)
- `services/marketplace-api/internal/apikeys/hash_test.go`
- `services/marketplace-api/internal/apikeys/repo.go` — lookup by prefix, list, create, revoke, update-last-used
- `services/marketplace-api/internal/apikeys/repo_test.go`
- `services/marketplace-api/internal/apikeys/service.go` — Create / Rotate / Revoke business logic (plan ceiling enforcement, rotation overlap)
- `services/marketplace-api/internal/apikeys/service_test.go`
- `services/marketplace-api/internal/apikeys/cache.go` — 60s LRU verified-hash cache (thread-safe)
- `services/marketplace-api/internal/apikeys/cache_test.go`
- `services/marketplace-api/internal/apikeys/middleware.go` — API-key auth middleware + scope check
- `services/marketplace-api/internal/apikeys/middleware_test.go`
- `services/marketplace-api/internal/apikeys/scopes.go` — canonical scope list + `RequireScope` helper
- `services/marketplace-api/internal/apikeys/scopes_test.go`
- `services/marketplace-api/internal/apikeys/ratelimit.go` — wrapper around `internal/ratelimit/` keyed by key ID
- `services/marketplace-api/internal/handlers/admin/apikeys_handler.go` — HTTP handlers for admin CRUD
- `services/marketplace-api/internal/handlers/admin/apikeys_handler_test.go`
- `services/marketplace-api/internal/apikeys/security_test.go` — timing-parity, revocation, rotation-overlap, scope-mismatch

### Modify

- `services/marketplace-api/internal/handlers/admin/routes.go` — register `/admin/stores/:storeId/api-keys` group gated by plan
- `services/marketplace-api/cmd/marketplace-api/main.go` — wire the service + middleware + public API router group
- `services/marketplace-api/go.mod` / `go.sum` — add `github.com/mr-tron/base58`
- `services/marketplace-api/internal/audit/events.go` — add `EmitAPIKeyEvent(...)` helper (thin wrapper over `EmitSecurity`)

### Delete

- Nothing — this is additive.

---

## Task Sequence Overview

| # | Task | Depends on |
|---|---|---|
| 1 | Migration + GORM model + repo (pure data layer) + unit tests | — |
| 2 | Key generation, hashing, prefix extraction, scope + cache utilities | 1 |
| 3 | Service layer: Create / Rotate / Revoke with plan-ceiling enforcement | 1, 2, P3 |
| 4 | Admin HTTP endpoints gated on plan | 3, P3 |
| 5 | API authentication middleware (bearer, lookup, bcrypt, context) | 2, P8 |
| 6 | Scope enforcement + route registration helper | 5 |
| 7 | Per-key rate limiting | 5 |
| 8 | Async last-used update worker | 5, P8 |
| 9 | Observability counters + audit events | 4, 5, 7 |
| 10 | Security test battery (revoked / rotation-overlap / scope-miss / timing) | all |

---

## Reusable patterns

**A. Key-prefix lookup, then bcrypt verify.** The full key format is `mk8_live_<42-char-base58>`. The first 8 chars of the base58 body are the `key_prefix` stored plaintext and uniquely indexed per `(tenant_id, key_prefix)`. Lookup is a single indexed query; verification is bcrypt. Because prefixes are 8 base58 chars (~46.8 bits), collisions are vanishingly rare per tenant; the unique index catches the theoretical case at creation time (retry with a fresh key).

**B. Timing-safe negative path.** A fixed `dummyBcryptHash` (pre-computed `bcrypt.GenerateFromPassword([]byte("timing-dummy"), 12)`) is compared against the provided suffix whenever the prefix lookup returns zero rows. This guarantees the 401 path spends comparable CPU to a real verify; the observable response time is indistinguishable for prefix-miss vs prefix-hit-hash-miss.

**C. Rotation overlap.** A "rotated" key keeps `revoked_at = NULL` but sets `rotation_replaces = <old_key_id>` on the new row; the old row is marked with `rotated_to` via a synthetic mechanism: we set `revoked_at = now() + 24h` on the **old** row at rotation time (so the cache + middleware treat it as valid for 24h). Middleware treats a key as valid when `revoked_at IS NULL OR revoked_at > now()`. At exactly 24h, the old row's `revoked_at` becomes "past" and the middleware rejects it. No cron needed; comparison is per-request.

**D. Hot-path cache.** A `cache.Store` (sync.Map-backed, 60s TTL) keyed by the full `mk8_live_...` key string (never the hash) records `{key_id, tenant_id, store_id, scopes, rate_limit, revoke_at}` after one successful bcrypt verify. Subsequent hits skip bcrypt. Cache is invalidated immediately on revoke/rotate by pushing a tombstone through an in-memory channel read at request time. Eviction: every 60s a tiny janitor sweeps expired entries.

**E. Scope list = canonical enum.** Scopes live in `scopes.go` as `type Scope string` constants. Route registration passes the required scope; the middleware compares against the key's `scopes` slice after auth. Unknown scopes in a key row fail the create-time validator.

**F. Bcrypt cost = 12.** Balances ≈200ms verify time against the 60s cache (so hot keys incur bcrypt once per minute, not per request). Documented as a constant `const BcryptCost = 12` so it can be lowered to 10 in test builds via a build tag if needed.

---

## Task 1: Migration + GORM model + repository (data layer only)

**Files:**
- Create: `services/marketplace-api/migrations/NNNN_create_enterprise_api_keys.up.sql`
- Create: `services/marketplace-api/migrations/NNNN_create_enterprise_api_keys.down.sql`
- Create: `services/marketplace-api/internal/apikeys/model.go`
- Create: `services/marketplace-api/internal/apikeys/repo.go`
- Create: `services/marketplace-api/internal/apikeys/repo_test.go`

**Spec references:** §18.4.

- [ ] **Step 1: Write the migration**

Choose `NNNN` as the next available sequence number in `services/marketplace-api/migrations/`.

```sql
-- up
CREATE TABLE enterprise_api_keys (
    id                   UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id            UUID        NOT NULL,
    store_id             UUID        NOT NULL,
    key_prefix           VARCHAR(8)  NOT NULL,
    key_hash             VARCHAR(60) NOT NULL,  -- bcrypt output (60 chars)
    scopes               JSONB       NOT NULL DEFAULT '[]'::jsonb,
    rate_limit_per_min   INTEGER     NOT NULL DEFAULT 100
                         CHECK (rate_limit_per_min > 0 AND rate_limit_per_min <= 10000),
    label                VARCHAR(100) NOT NULL,
    created_by           UUID        NOT NULL,  -- user_id (not FK — cross-service)
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at           TIMESTAMPTZ,            -- NULL = active; set = revoked / rotation-overlap expiry
    revoked_reason       VARCHAR(50),
    last_used_at         TIMESTAMPTZ,
    last_used_ip_hash    VARCHAR(64),            -- HMAC-SHA256 via internal/ipprivacy
    rotation_replaces    UUID REFERENCES enterprise_api_keys(id) ON DELETE SET NULL
);

CREATE UNIQUE INDEX idx_api_keys_tenant_prefix
    ON enterprise_api_keys(tenant_id, key_prefix);

CREATE INDEX idx_api_keys_store_active
    ON enterprise_api_keys(store_id)
    WHERE revoked_at IS NULL;

COMMENT ON COLUMN enterprise_api_keys.key_prefix IS
    'First 8 chars of the base58 body (not the mk8_live_ prefix). Plaintext for O(log n) lookup.';
COMMENT ON COLUMN enterprise_api_keys.revoked_at IS
    'NULL = active. Set to now() on revoke. Set to now()+24h on rotation of this row (overlap window).';
```

Down migration drops the table; nothing references it elsewhere.

- [ ] **Step 2: Failing test — model round-trips**

```go
package apikeys_test

import (
    "testing"
    "time"

    "github.com/google/uuid"
    "github.com/stretchr/testify/require"

    "github.com/tesserix/marketplace-api/internal/apikeys"
    "github.com/tesserix/marketplace-api/pkg/testdb"
)

func TestRepo_Create_AndLookupByPrefix(t *testing.T) {
    db := testdb.NewDB(t, "enterprise_api_keys")
    repo := apikeys.NewRepo(db)

    rec := apikeys.APIKey{
        ID:               uuid.New(),
        TenantID:         uuid.New(),
        StoreID:          uuid.New(),
        KeyPrefix:        "ABCD1234",
        KeyHash:          "$2a$12$" + string(make([]byte, 53)),
        Scopes:           apikeys.ScopeSet{"products:read", "orders:read"},
        RateLimitPerMin:  100,
        Label:            "Integration X",
        CreatedBy:        uuid.New(),
    }
    require.NoError(t, repo.Create(nil, &rec))

    got, err := repo.FindByTenantPrefix(nil, rec.TenantID, rec.KeyPrefix)
    require.NoError(t, err)
    require.Equal(t, rec.ID, got.ID)
    require.True(t, got.IsUsable(time.Now()))
}

func TestRepo_FindByTenantPrefix_NotFound(t *testing.T) {
    db := testdb.NewDB(t, "enterprise_api_keys")
    repo := apikeys.NewRepo(db)
    _, err := repo.FindByTenantPrefix(nil, uuid.New(), "ZZZZZZZZ")
    require.ErrorIs(t, err, apikeys.ErrNotFound)
}
```

- [ ] **Step 3: Run — expect FAIL (package doesn't exist)**

```bash
cd services/marketplace-api
go test ./internal/apikeys/... -v
```

- [ ] **Step 4: Write `model.go`**

```go
package apikeys

import (
    "database/sql/driver"
    "encoding/json"
    "errors"
    "fmt"
    "time"

    "github.com/google/uuid"
)

var ErrNotFound = errors.New("apikeys: not found")

// ScopeSet is a JSONB-backed slice of scope strings.
type ScopeSet []string

func (s *ScopeSet) Scan(v any) error {
    if v == nil { *s = nil; return nil }
    switch t := v.(type) {
    case []byte:
        return json.Unmarshal(t, s)
    case string:
        return json.Unmarshal([]byte(t), s)
    default:
        return fmt.Errorf("apikeys: cannot scan %T into ScopeSet", v)
    }
}
func (s ScopeSet) Value() (driver.Value, error) { return json.Marshal(s) }

func (s ScopeSet) Has(scope string) bool {
    for _, x := range s { if x == scope { return true } }
    return false
}

// APIKey is the persisted form of an enterprise key.
type APIKey struct {
    ID                uuid.UUID  `gorm:"primaryKey"`
    TenantID          uuid.UUID  `gorm:"not null;index:idx_api_keys_tenant_prefix,unique,priority:1"`
    StoreID           uuid.UUID  `gorm:"not null"`
    KeyPrefix         string     `gorm:"type:varchar(8);not null;index:idx_api_keys_tenant_prefix,unique,priority:2"`
    KeyHash           string     `gorm:"type:varchar(60);not null"`
    Scopes            ScopeSet   `gorm:"type:jsonb;not null;default:'[]'"`
    RateLimitPerMin   int        `gorm:"not null;default:100"`
    Label             string     `gorm:"type:varchar(100);not null"`
    CreatedBy         uuid.UUID  `gorm:"not null"`
    CreatedAt         time.Time  `gorm:"not null"`
    RevokedAt         *time.Time
    RevokedReason     *string    `gorm:"type:varchar(50)"`
    LastUsedAt        *time.Time
    LastUsedIPHash    *string    `gorm:"type:varchar(64)"`
    RotationReplaces  *uuid.UUID
}

func (APIKey) TableName() string { return "enterprise_api_keys" }

// IsUsable reports whether the key is currently valid for authentication.
// Keys in the 24h rotation-overlap window have revoked_at in the future and
// remain usable.
func (k APIKey) IsUsable(now time.Time) bool {
    return k.RevokedAt == nil || k.RevokedAt.After(now)
}
```

- [ ] **Step 5: Write `repo.go`**

```go
package apikeys

import (
    "context"
    "errors"
    "time"

    "github.com/google/uuid"
    "gorm.io/gorm"
)

type Repo struct{ db *gorm.DB }

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

func (r *Repo) dbCtx(ctx context.Context) *gorm.DB {
    if ctx == nil { return r.db }
    return r.db.WithContext(ctx)
}

func (r *Repo) Create(ctx context.Context, k *APIKey) error {
    return r.dbCtx(ctx).Create(k).Error
}

// FindByTenantPrefix is the hot-path lookup used by the middleware.
// Returns ErrNotFound if nothing matches; does NOT filter by revoked_at
// (caller decides via IsUsable + rotation-overlap logic).
func (r *Repo) FindByTenantPrefix(ctx context.Context, tenantID uuid.UUID, prefix string) (APIKey, error) {
    var k APIKey
    err := r.dbCtx(ctx).
        Where("tenant_id = ? AND key_prefix = ?", tenantID, prefix).
        First(&k).Error
    if errors.Is(err, gorm.ErrRecordNotFound) { return APIKey{}, ErrNotFound }
    return k, err
}

// ListForStore returns metadata-only rows (no hash surfaced; caller is
// responsible for omitting sensitive fields in the HTTP response).
func (r *Repo) ListForStore(ctx context.Context, tenantID, storeID uuid.UUID) ([]APIKey, error) {
    var out []APIKey
    err := r.dbCtx(ctx).
        Where("tenant_id = ? AND store_id = ?", tenantID, storeID).
        Order("created_at DESC").Find(&out).Error
    return out, err
}

// Revoke sets revoked_at = now() (or the supplied instant for rotation overlap).
func (r *Repo) Revoke(ctx context.Context, id uuid.UUID, at time.Time, reason string) error {
    return r.dbCtx(ctx).Model(&APIKey{}).Where("id = ?", id).
        Updates(map[string]any{"revoked_at": at, "revoked_reason": reason}).Error
}

// UpdateLastUsed is fire-and-forget from the middleware worker.
func (r *Repo) UpdateLastUsed(ctx context.Context, id uuid.UUID, at time.Time, ipHash string) error {
    return r.dbCtx(ctx).Model(&APIKey{}).Where("id = ?", id).
        Updates(map[string]any{"last_used_at": at, "last_used_ip_hash": ipHash}).Error
}
```

- [ ] **Step 6: Run tests — expect PASS**

- [ ] **Step 7: Commit**

```bash
git add services/marketplace-api/migrations/NNNN_create_enterprise_api_keys.*.sql \
        services/marketplace-api/internal/apikeys/{model,repo,repo_test}.go
git commit -m "feat(apikeys): enterprise_api_keys table + GORM model + repo"
```

---

## Task 2: Key generation, hashing, prefix extraction, scope + cache utilities

**Files:**
- Create: `services/marketplace-api/internal/apikeys/generate.go`
- Create: `services/marketplace-api/internal/apikeys/generate_test.go`
- Create: `services/marketplace-api/internal/apikeys/hash.go`
- Create: `services/marketplace-api/internal/apikeys/hash_test.go`
- Create: `services/marketplace-api/internal/apikeys/scopes.go`
- Create: `services/marketplace-api/internal/apikeys/scopes_test.go`
- Create: `services/marketplace-api/internal/apikeys/cache.go`
- Create: `services/marketplace-api/internal/apikeys/cache_test.go`
- Modify: `services/marketplace-api/go.mod` (add `github.com/mr-tron/base58`)

**Spec references:** §18.4.

- [ ] **Step 1: Failing tests — generation, hashing, prefix, scopes, cache**

```go
// generate_test.go
func TestGenerate_ProducesValidKeyAndPrefix(t *testing.T) {
    got, err := apikeys.Generate(apikeys.EnvLive)
    require.NoError(t, err)
    require.True(t, strings.HasPrefix(got.Plaintext, "mk8_live_"))
    require.Len(t, got.Prefix, 8)
    require.GreaterOrEqual(t, len(got.Plaintext), len("mk8_live_")+8+4)
    // Display masking: mk8_live_ABCD****WXYZ shape — prefix + last 4 of suffix.
    require.Regexp(t, `^mk8_live_[1-9A-HJ-NP-Za-km-z]{8}\*{4}[1-9A-HJ-NP-Za-km-z]{4}$`,
        apikeys.Display(got.Plaintext))
}

func TestGenerate_UsesTestPrefixForTestEnv(t *testing.T) {
    got, err := apikeys.Generate(apikeys.EnvTest)
    require.NoError(t, err)
    require.True(t, strings.HasPrefix(got.Plaintext, "mk8_test_"))
}

func TestExtractPrefix_ValidInputs(t *testing.T) {
    p, ok := apikeys.ExtractPrefix("mk8_live_ABCD1234qrstuvwxABCDEFGHIJ")
    require.True(t, ok)
    require.Equal(t, "ABCD1234", p)
}

func TestExtractPrefix_RejectsJunk(t *testing.T) {
    _, ok := apikeys.ExtractPrefix("not-a-key")
    require.False(t, ok)
    _, ok = apikeys.ExtractPrefix("mk8_live_")
    require.False(t, ok)
    _, ok = apikeys.ExtractPrefix("mk8_live_SHORT")
    require.False(t, ok)
}
```

```go
// hash_test.go
func TestHashAndVerify_RoundTrip(t *testing.T) {
    plaintext := "mk8_live_testtesttesttesttesttesttesttest12345"
    h, err := apikeys.Hash(plaintext)
    require.NoError(t, err)
    require.NoError(t, apikeys.Verify(h, plaintext))
    require.Error(t, apikeys.Verify(h, plaintext+"x"))
}

func TestDummyHashVerify_AlwaysFailsButTakesSimilarTime(t *testing.T) {
    // Dummy path should always error with bcrypt.ErrMismatchedHashAndPassword.
    err := apikeys.VerifyDummy("anything")
    require.Error(t, err)
}
```

```go
// scopes_test.go
func TestAllScopes_ContainsExpectedSetV1(t *testing.T) {
    all := apikeys.AllScopes()
    for _, want := range []string{
        "products:read","products:write","orders:read","orders:write",
        "customers:read","customers:write","categories:read","categories:write",
        "coupons:read","coupons:write",
    } {
        require.Contains(t, all, apikeys.Scope(want))
    }
    // Explicitly NOT present in v1:
    require.NotContains(t, all, apikeys.Scope("admin:all"))
    require.NotContains(t, all, apikeys.Scope("tenant:admin"))
}

func TestValidateScopes_RejectsUnknown(t *testing.T) {
    require.NoError(t, apikeys.ValidateScopes([]string{"products:read","orders:read"}))
    require.Error(t, apikeys.ValidateScopes([]string{"products:read","delete:everything"}))
}
```

```go
// cache_test.go
func TestCache_HitWithinTTL(t *testing.T) {
    c := apikeys.NewCache(60 * time.Second)
    e := apikeys.CacheEntry{KeyID: uuid.New(), TenantID: uuid.New(), Scopes: []string{"products:read"}}
    c.Put("mk8_live_abc", e)
    got, ok := c.Get("mk8_live_abc")
    require.True(t, ok)
    require.Equal(t, e.KeyID, got.KeyID)
}

func TestCache_MissAfterInvalidate(t *testing.T) {
    c := apikeys.NewCache(60 * time.Second)
    c.Put("mk8_live_abc", apikeys.CacheEntry{KeyID: uuid.New()})
    c.Invalidate("mk8_live_abc")
    _, ok := c.Get("mk8_live_abc")
    require.False(t, ok)
}

func TestCache_MissAfterTTL(t *testing.T) {
    c := apikeys.NewCache(10 * time.Millisecond)
    c.Put("mk8_live_abc", apikeys.CacheEntry{KeyID: uuid.New()})
    time.Sleep(20 * time.Millisecond)
    _, ok := c.Get("mk8_live_abc")
    require.False(t, ok)
}
```

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Write `generate.go`**

```go
package apikeys

import (
    "crypto/rand"
    "errors"
    "strings"

    "github.com/mr-tron/base58"
)

type Env string
const (
    EnvLive Env = "live"
    EnvTest Env = "test"
)

const (
    livePrefix = "mk8_live_"
    testPrefix = "mk8_test_"
    prefixLen  = 8 // first 8 chars of the base58 body
    entropyBytes = 32
)

type Generated struct {
    Plaintext string // full key — return once, never persist
    Prefix    string // first 8 chars of base58 body
}

func Generate(env Env) (Generated, error) {
    raw := make([]byte, entropyBytes)
    if _, err := rand.Read(raw); err != nil { return Generated{}, err }
    body := base58.Encode(raw)
    if len(body) < prefixLen+4 {
        return Generated{}, errors.New("apikeys: unexpectedly short base58 body")
    }
    var pfx string
    switch env {
    case EnvTest: pfx = testPrefix
    default:      pfx = livePrefix
    }
    return Generated{
        Plaintext: pfx + body,
        Prefix:    body[:prefixLen],
    }, nil
}

func ExtractPrefix(key string) (string, bool) {
    body, ok := stripEnvPrefix(key)
    if !ok { return "", false }
    if len(body) < prefixLen+4 { return "", false }
    return body[:prefixLen], true
}

func stripEnvPrefix(key string) (string, bool) {
    switch {
    case strings.HasPrefix(key, livePrefix): return key[len(livePrefix):], true
    case strings.HasPrefix(key, testPrefix): return key[len(testPrefix):], true
    }
    return "", false
}

// Display renders a masked form for UI listings, e.g. mk8_live_ABCD****WXYZ.
// Never surfaced in logs; consumed by the admin API response on create only.
func Display(key string) string {
    body, ok := stripEnvPrefix(key)
    if !ok || len(body) < prefixLen+4 { return "redacted" }
    envPfx := key[:len(key)-len(body)]
    return envPfx + body[:prefixLen] + "****" + body[len(body)-4:]
}
```

- [ ] **Step 4: Write `hash.go`**

```go
package apikeys

import "golang.org/x/crypto/bcrypt"

// BcryptCost is tuned so a single verify runs ~200ms on prod hardware.
// Combined with the 60s verified-hash cache, hot keys incur bcrypt once per
// minute per process — not per request.
const BcryptCost = 12

// dummyHash is a pre-generated bcrypt hash against a fixed seed, used for
// timing-safe negative paths when the prefix lookup misses. Must be generated
// at init() (not embedded as a literal, because the salt changes per build).
var dummyHash []byte

func init() {
    h, err := bcrypt.GenerateFromPassword([]byte("apikeys-timing-dummy-v1"), BcryptCost)
    if err != nil { panic("apikeys: cannot init dummy hash: " + err.Error()) }
    dummyHash = h
}

func Hash(plaintext string) (string, error) {
    h, err := bcrypt.GenerateFromPassword([]byte(plaintext), BcryptCost)
    if err != nil { return "", err }
    return string(h), nil
}

func Verify(hash, plaintext string) error {
    return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plaintext))
}

// VerifyDummy performs a bcrypt comparison that always fails but takes the
// same time as a real verify — called on prefix-lookup misses to keep the
// response time indistinguishable from prefix-hit-hash-miss.
func VerifyDummy(plaintext string) error {
    return bcrypt.CompareHashAndPassword(dummyHash, []byte(plaintext))
}
```

- [ ] **Step 5: Write `scopes.go`**

```go
package apikeys

import "fmt"

type Scope string

const (
    ScopeProductsRead    Scope = "products:read"
    ScopeProductsWrite   Scope = "products:write"
    ScopeOrdersRead      Scope = "orders:read"
    ScopeOrdersWrite     Scope = "orders:write"
    ScopeCustomersRead   Scope = "customers:read"
    ScopeCustomersWrite  Scope = "customers:write"
    ScopeCategoriesRead  Scope = "categories:read"
    ScopeCategoriesWrite Scope = "categories:write"
    ScopeCouponsRead     Scope = "coupons:read"
    ScopeCouponsWrite    Scope = "coupons:write"
)

// AllScopes is the canonical v1 list. No admin:*, no tenant:* — those remain
// GIP-authenticated admin-only flows.
func AllScopes() []Scope {
    return []Scope{
        ScopeProductsRead, ScopeProductsWrite,
        ScopeOrdersRead, ScopeOrdersWrite,
        ScopeCustomersRead, ScopeCustomersWrite,
        ScopeCategoriesRead, ScopeCategoriesWrite,
        ScopeCouponsRead, ScopeCouponsWrite,
    }
}

func ValidateScopes(in []string) error {
    valid := map[string]struct{}{}
    for _, s := range AllScopes() { valid[string(s)] = struct{}{} }
    for _, s := range in {
        if _, ok := valid[s]; !ok {
            return fmt.Errorf("apikeys: unknown scope %q", s)
        }
    }
    return nil
}

// IsReadOnly reports whether every scope in the set is a :read variant.
// Used to enforce "Studio+ may only create read-only keys" (§9).
func IsReadOnlyScopes(in []string) bool {
    for _, s := range in {
        if len(s) < 5 || s[len(s)-5:] != ":read" { return false }
    }
    return true
}
```

- [ ] **Step 6: Write `cache.go`**

```go
package apikeys

import (
    "sync"
    "time"

    "github.com/google/uuid"
)

type CacheEntry struct {
    KeyID          uuid.UUID
    TenantID       uuid.UUID
    StoreID        uuid.UUID
    Scopes         []string
    RateLimitPerMin int
    RevokedAt      *time.Time // copied forward to honor 24h rotation overlap
    ExpiresAt      time.Time
}

type Cache struct {
    ttl time.Duration
    m   sync.Map // map[string]CacheEntry  (keyed by the full plaintext key)
}

func NewCache(ttl time.Duration) *Cache {
    return &Cache{ttl: ttl}
}

func (c *Cache) Put(keyPlaintext string, e CacheEntry) {
    e.ExpiresAt = time.Now().Add(c.ttl)
    c.m.Store(keyPlaintext, e)
}

func (c *Cache) Get(keyPlaintext string) (CacheEntry, bool) {
    v, ok := c.m.Load(keyPlaintext)
    if !ok { return CacheEntry{}, false }
    e := v.(CacheEntry)
    if time.Now().After(e.ExpiresAt) {
        c.m.Delete(keyPlaintext)
        return CacheEntry{}, false
    }
    // Rotation-overlap check: if revoked_at is in the past, treat as miss.
    if e.RevokedAt != nil && !time.Now().Before(*e.RevokedAt) {
        c.m.Delete(keyPlaintext)
        return CacheEntry{}, false
    }
    return e, true
}

func (c *Cache) Invalidate(keyPlaintext string) { c.m.Delete(keyPlaintext) }

// InvalidateByID walks the cache removing any entries for the given key id.
// Called on revoke / rotate — the caller doesn't know the plaintext, only the id.
func (c *Cache) InvalidateByID(id uuid.UUID) {
    c.m.Range(func(k, v any) bool {
        if v.(CacheEntry).KeyID == id {
            c.m.Delete(k)
        }
        return true
    })
}
```

- [ ] **Step 7: Run tests — expect PASS**

- [ ] **Step 8: Commit**

```bash
git add services/marketplace-api/internal/apikeys/{generate,generate_test,hash,hash_test,scopes,scopes_test,cache,cache_test}.go \
        services/marketplace-api/go.mod services/marketplace-api/go.sum
git commit -m "feat(apikeys): CSPRNG + base58 key gen, bcrypt hashing, scopes, in-memory cache"
```

---

## Task 3: Service layer — Create / Rotate / Revoke with plan-ceiling enforcement

**Files:**
- Create: `services/marketplace-api/internal/apikeys/service.go`
- Create: `services/marketplace-api/internal/apikeys/service_test.go`

**Spec references:** §9 (Pro-only writes, Studio+ reads), §18.4 (24h rotation overlap).

- [ ] **Step 1: Failing tests**

```go
func TestService_Create_ReturnsPlaintextOnceAndPersistsHashOnly(t *testing.T) {
    svc := newTestService(t)
    out, err := svc.Create(ctx, apikeys.CreateInput{
        TenantID: tID, StoreID: sID, CreatedBy: uID,
        Scopes:   []string{"products:read","orders:read"},
        RateLimitPerMin: 100, Label: "Integration A",
        Plan: subscription.PlanStudio, // read-only
    })
    require.NoError(t, err)
    require.True(t, strings.HasPrefix(out.Plaintext, "mk8_live_"))
    require.NotEmpty(t, out.ID)

    // Row contains hash, not plaintext.
    var row apikeys.APIKey
    require.NoError(t, db.First(&row, "id=?", out.ID).Error)
    require.NotEqual(t, out.Plaintext, row.KeyHash)
    require.NoError(t, apikeys.Verify(row.KeyHash, out.Plaintext))
}

func TestService_Create_StudioCannotCreateWriteScope(t *testing.T) {
    svc := newTestService(t)
    _, err := svc.Create(ctx, apikeys.CreateInput{
        TenantID: tID, StoreID: sID, CreatedBy: uID,
        Scopes: []string{"products:read","products:write"},
        RateLimitPerMin: 100, Label: "X",
        Plan: subscription.PlanStudio,
    })
    require.ErrorIs(t, err, apikeys.ErrWriteScopeRequiresPro)
}

func TestService_Create_StarterCannotCreateAnyKey(t *testing.T) {
    svc := newTestService(t)
    _, err := svc.Create(ctx, apikeys.CreateInput{
        TenantID: tID, StoreID: sID, CreatedBy: uID,
        Scopes: []string{"products:read"}, RateLimitPerMin: 100, Label: "X",
        Plan: subscription.PlanStarter,
    })
    require.ErrorIs(t, err, apikeys.ErrPlanDoesNotAllowAPI)
}

func TestService_Create_RateLimitClampedByPlanCeiling(t *testing.T) {
    svc := newTestService(t)
    _, err := svc.Create(ctx, apikeys.CreateInput{
        TenantID: tID, StoreID: sID, CreatedBy: uID,
        Scopes: []string{"products:read"}, RateLimitPerMin: 99999, Label: "X",
        Plan: subscription.PlanStudio,
    })
    require.ErrorIs(t, err, apikeys.ErrRateLimitExceedsPlanCeiling)
}

func TestService_Rotate_OldKeyValidFor24h(t *testing.T) {
    svc := newTestService(t)
    orig, err := svc.Create(ctx, validProInput())
    require.NoError(t, err)

    rot, err := svc.Rotate(ctx, tID, orig.ID, "scheduled_rotation")
    require.NoError(t, err)
    require.NotEqual(t, orig.Plaintext, rot.NewPlaintext)

    var oldRow apikeys.APIKey
    require.NoError(t, db.First(&oldRow, "id=?", orig.ID).Error)
    require.NotNil(t, oldRow.RevokedAt, "old row must carry a revoked_at = now()+24h")
    require.True(t, oldRow.RevokedAt.After(time.Now().Add(23*time.Hour)))
    require.True(t, oldRow.RevokedAt.Before(time.Now().Add(25*time.Hour)))

    var newRow apikeys.APIKey
    require.NoError(t, db.First(&newRow, "id=?", rot.NewID).Error)
    require.NotNil(t, newRow.RotationReplaces)
    require.Equal(t, orig.ID, *newRow.RotationReplaces)
}

func TestService_Revoke_SetsRevokedAtImmediately(t *testing.T) {
    svc := newTestService(t)
    k, err := svc.Create(ctx, validProInput())
    require.NoError(t, err)

    require.NoError(t, svc.Revoke(ctx, tID, k.ID, "compromised"))
    var row apikeys.APIKey
    require.NoError(t, db.First(&row, "id=?", k.ID).Error)
    require.NotNil(t, row.RevokedAt)
    require.False(t, row.IsUsable(time.Now()))
}
```

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Write `service.go`**

```go
package apikeys

import (
    "context"
    "errors"
    "fmt"
    "time"

    "github.com/google/uuid"
    "gorm.io/gorm"

    "github.com/tesserix/marketplace-api/internal/subscription"
)

var (
    ErrPlanDoesNotAllowAPI         = errors.New("apikeys: plan does not allow API keys")
    ErrWriteScopeRequiresPro       = errors.New("apikeys: write scopes require Pro plan")
    ErrRateLimitExceedsPlanCeiling = errors.New("apikeys: rate_limit_per_min exceeds plan ceiling")
    ErrRotationOverlap             = 24 * time.Hour
)

// planCeiling returns the max rate_limit_per_min the given plan may set.
// Starter/Trial: 0 (no keys). Studio: 200. Pro: 1000.
func planCeiling(p subscription.SubscriptionPlan) int {
    switch p {
    case subscription.PlanStudio: return 200
    case subscription.PlanPro:    return 1000
    default:                      return 0
    }
}

type Service struct {
    repo  *Repo
    cache *Cache
    env   Env // live/test
}

func NewService(repo *Repo, cache *Cache, env Env) *Service {
    return &Service{repo: repo, cache: cache, env: env}
}

type CreateInput struct {
    TenantID        uuid.UUID
    StoreID         uuid.UUID
    CreatedBy       uuid.UUID
    Scopes          []string
    RateLimitPerMin int
    Label           string
    Plan            subscription.SubscriptionPlan
}

type CreateResult struct {
    ID        uuid.UUID
    Plaintext string // return ONCE — never re-derivable
    Display   string // masked form for echo back if needed
}

func (s *Service) Create(ctx context.Context, in CreateInput) (CreateResult, error) {
    if err := ValidateScopes(in.Scopes); err != nil { return CreateResult{}, err }
    ceiling := planCeiling(in.Plan)
    if ceiling == 0 { return CreateResult{}, ErrPlanDoesNotAllowAPI }
    if !IsReadOnlyScopes(in.Scopes) && in.Plan != subscription.PlanPro {
        return CreateResult{}, ErrWriteScopeRequiresPro
    }
    if in.RateLimitPerMin > ceiling {
        return CreateResult{}, fmt.Errorf("%w (plan ceiling = %d)", ErrRateLimitExceedsPlanCeiling, ceiling)
    }

    gen, err := Generate(s.env)
    if err != nil { return CreateResult{}, err }
    hash, err := Hash(gen.Plaintext)
    if err != nil { return CreateResult{}, err }

    row := APIKey{
        ID: uuid.New(), TenantID: in.TenantID, StoreID: in.StoreID,
        KeyPrefix: gen.Prefix, KeyHash: hash,
        Scopes: ScopeSet(in.Scopes),
        RateLimitPerMin: in.RateLimitPerMin, Label: in.Label,
        CreatedBy: in.CreatedBy,
    }
    if err := s.repo.Create(ctx, &row); err != nil { return CreateResult{}, err }

    return CreateResult{ID: row.ID, Plaintext: gen.Plaintext, Display: Display(gen.Plaintext)}, nil
}

type RotateResult struct {
    NewID        uuid.UUID
    NewPlaintext string
    OldExpiresAt time.Time
}

// Rotate issues a new key with identical scopes/label/rate limit, marks the old
// row's revoked_at = now() + 24h so the middleware keeps accepting it until the
// overlap window ends. The caller must surface both the new plaintext AND the
// overlap expiry in the HTTP response so the integrator can swap at leisure.
func (s *Service) Rotate(ctx context.Context, tenantID, oldID uuid.UUID, reason string) (RotateResult, error) {
    var old APIKey
    // Fetch the old row (use a direct DB call; we don't expose a by-id repo method elsewhere to
    // discourage accidental bypass of tenant scoping).
    if err := s.repo.db.WithContext(ctx).
        Where("id = ? AND tenant_id = ?", oldID, tenantID).First(&old).Error; err != nil {
        if errors.Is(err, gorm.ErrRecordNotFound) { return RotateResult{}, ErrNotFound }
        return RotateResult{}, err
    }
    if !old.IsUsable(time.Now()) {
        return RotateResult{}, errors.New("apikeys: cannot rotate a revoked key")
    }

    gen, err := Generate(s.env)
    if err != nil { return RotateResult{}, err }
    hash, err := Hash(gen.Plaintext)
    if err != nil { return RotateResult{}, err }

    overlapEnd := time.Now().Add(ErrRotationOverlap)
    newRow := APIKey{
        ID: uuid.New(), TenantID: old.TenantID, StoreID: old.StoreID,
        KeyPrefix: gen.Prefix, KeyHash: hash,
        Scopes: old.Scopes, RateLimitPerMin: old.RateLimitPerMin,
        Label: old.Label, CreatedBy: old.CreatedBy,
        RotationReplaces: &old.ID,
    }

    err = s.repo.db.Transaction(func(tx *gorm.DB) error {
        if err := tx.Create(&newRow).Error; err != nil { return err }
        return tx.Model(&APIKey{}).Where("id = ?", old.ID).
            Updates(map[string]any{"revoked_at": overlapEnd, "revoked_reason": reason}).Error
    })
    if err != nil { return RotateResult{}, err }

    s.cache.InvalidateByID(old.ID)

    return RotateResult{NewID: newRow.ID, NewPlaintext: gen.Plaintext, OldExpiresAt: overlapEnd}, nil
}

func (s *Service) Revoke(ctx context.Context, tenantID, id uuid.UUID, reason string) error {
    res := s.repo.db.WithContext(ctx).Model(&APIKey{}).
        Where("id = ? AND tenant_id = ? AND revoked_at IS NULL", id, tenantID).
        Updates(map[string]any{"revoked_at": time.Now(), "revoked_reason": reason})
    if res.Error != nil { return res.Error }
    if res.RowsAffected == 0 { return ErrNotFound }
    s.cache.InvalidateByID(id)
    return nil
}
```

- [ ] **Step 4: Run tests — expect PASS**

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/apikeys/{service,service_test}.go
git commit -m "feat(apikeys): service layer with plan-ceiling enforcement + 24h rotation overlap"
```

---

## Task 4: Admin HTTP endpoints gated on plan

**Files:**
- Create: `services/marketplace-api/internal/handlers/admin/apikeys_handler.go`
- Create: `services/marketplace-api/internal/handlers/admin/apikeys_handler_test.go`
- Modify: `services/marketplace-api/internal/handlers/admin/routes.go`

**Spec references:** §9, §18.4.

- [ ] **Step 1: Failing tests — happy paths + plan gating**

```go
func TestAdmin_CreateAPIKey_ProReturnsPlaintextOnce(t *testing.T) {
    suite := inttest.NewSuite(t)
    tID, sID := suite.SeedStore(subscription.StatusActive, subscription.PlanPro)

    w := suite.AdminPOST(tID, sID,
        "/admin/stores/"+sID.String()+"/api-keys",
        map[string]any{
            "label":              "Test integration",
            "scopes":             []string{"products:read","products:write"},
            "rate_limit_per_min": 100,
        })
    require.Equal(t, 201, w.Code)
    var resp apikeys.CreateResponse
    require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
    require.True(t, strings.HasPrefix(resp.Plaintext, "mk8_live_"))
    require.NotEmpty(t, resp.ID)
}

func TestAdmin_CreateAPIKey_StarterReturns403(t *testing.T) {
    suite := inttest.NewSuite(t)
    tID, sID := suite.SeedStore(subscription.StatusActive, subscription.PlanStarter)

    w := suite.AdminPOST(tID, sID,
        "/admin/stores/"+sID.String()+"/api-keys",
        map[string]any{"label":"x","scopes":[]string{"products:read"},"rate_limit_per_min":50})
    require.Equal(t, 403, w.Code, "plangate.RequireFeature(FeatureReadAPI) must reject Starter")
}

func TestAdmin_CreateAPIKey_StudioWriteScopeReturns400(t *testing.T) {
    // Studio has FeatureReadAPI (so gate passes) but service layer rejects write scope.
    suite := inttest.NewSuite(t)
    tID, sID := suite.SeedStore(subscription.StatusActive, subscription.PlanStudio)
    w := suite.AdminPOST(tID, sID,
        "/admin/stores/"+sID.String()+"/api-keys",
        map[string]any{"label":"x","scopes":[]string{"products:write"},"rate_limit_per_min":50})
    require.Equal(t, 400, w.Code)
}

func TestAdmin_ListAPIKeys_ReturnsMetadataOnly(t *testing.T) {
    suite := inttest.NewSuite(t)
    tID, sID := suite.SeedStore(subscription.StatusActive, subscription.PlanPro)
    suite.SeedAPIKey(tID, sID, []string{"products:read"})

    w := suite.AdminGET(tID, sID, "/admin/stores/"+sID.String()+"/api-keys")
    require.Equal(t, 200, w.Code)
    require.NotContains(t, w.Body.String(), "key_hash", "hash must never be surfaced")
    require.NotContains(t, w.Body.String(), "mk8_live_", "plaintext must never be surfaced in list")
}

func TestAdmin_RotateAPIKey_ReturnsBothOldAndOverlap(t *testing.T) {
    suite := inttest.NewSuite(t)
    tID, sID := suite.SeedStore(subscription.StatusActive, subscription.PlanPro)
    oldID := suite.SeedAPIKey(tID, sID, []string{"products:read"})

    w := suite.AdminPOST(tID, sID,
        "/admin/stores/"+sID.String()+"/api-keys/"+oldID.String()+"/rotate",
        map[string]any{"reason": "scheduled_rotation"})
    require.Equal(t, 200, w.Code)
    var resp apikeys.RotateResponse
    require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
    require.True(t, strings.HasPrefix(resp.NewPlaintext, "mk8_live_"))
    require.Equal(t, oldID, resp.OldID)
    require.True(t, resp.OldValidUntil.After(time.Now().Add(23*time.Hour)))
}

func TestAdmin_RevokeAPIKey_SetsRevokedImmediately(t *testing.T) {
    suite := inttest.NewSuite(t)
    tID, sID := suite.SeedStore(subscription.StatusActive, subscription.PlanPro)
    id := suite.SeedAPIKey(tID, sID, []string{"products:read"})

    w := suite.AdminDELETE(tID, sID, "/admin/stores/"+sID.String()+"/api-keys/"+id.String())
    require.Equal(t, 204, w.Code)
}
```

- [ ] **Step 2: Write `apikeys_handler.go`**

```go
package admin

import (
    "net/http"
    "time"

    "github.com/gin-gonic/gin"
    "github.com/google/uuid"

    "github.com/tesserix/marketplace-api/internal/apikeys"
    "github.com/tesserix/marketplace-api/internal/subscription"
)

type APIKeysHandler struct {
    svc  *apikeys.Service
    repo *apikeys.Repo
}

func NewAPIKeysHandler(svc *apikeys.Service, repo *apikeys.Repo) *APIKeysHandler {
    return &APIKeysHandler{svc: svc, repo: repo}
}

type createReq struct {
    Label           string   `json:"label" binding:"required,max=100"`
    Scopes          []string `json:"scopes" binding:"required,min=1"`
    RateLimitPerMin int      `json:"rate_limit_per_min" binding:"required,min=1,max=10000"`
}

type createResp struct {
    ID        uuid.UUID `json:"id"`
    Plaintext string    `json:"plaintext"`           // shown once
    Display   string    `json:"display"`             // for echo after dismissal
    WarnOnce  string    `json:"warning"`             // "Store this securely — it will not be shown again"
}

func (h *APIKeysHandler) Create(c *gin.Context) {
    var body createReq
    if err := c.ShouldBindJSON(&body); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error":"invalid_request","message":err.Error()}); return
    }
    tenantID := mustTenantID(c)
    storeID  := mustStoreID(c)
    plan     := mustSubscriptionPlan(c)

    out, err := h.svc.Create(c.Request.Context(), apikeys.CreateInput{
        TenantID: tenantID, StoreID: storeID, CreatedBy: mustUserID(c),
        Scopes: body.Scopes, RateLimitPerMin: body.RateLimitPerMin, Label: body.Label,
        Plan: plan,
    })
    if err != nil {
        apikeyServiceErrorToHTTP(c, err); return
    }
    c.JSON(http.StatusCreated, createResp{
        ID: out.ID, Plaintext: out.Plaintext, Display: out.Display,
        WarnOnce: "Store this key securely. It will not be shown again.",
    })
}

type listItem struct {
    ID              uuid.UUID  `json:"id"`
    Label           string     `json:"label"`
    Display         string     `json:"display"`
    Scopes          []string   `json:"scopes"`
    RateLimitPerMin int        `json:"rate_limit_per_min"`
    CreatedAt       time.Time  `json:"created_at"`
    LastUsedAt      *time.Time `json:"last_used_at"`
    RevokedAt       *time.Time `json:"revoked_at,omitempty"`
}

func (h *APIKeysHandler) List(c *gin.Context) {
    tenantID := mustTenantID(c)
    storeID  := mustStoreID(c)
    rows, err := h.repo.ListForStore(c.Request.Context(), tenantID, storeID)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error":"internal"}); return
    }
    out := make([]listItem, 0, len(rows))
    for _, r := range rows {
        out = append(out, listItem{
            ID: r.ID, Label: r.Label,
            Display: "mk8_live_" + r.KeyPrefix + "****", // suffix-last-4 unknown without plaintext
            Scopes: []string(r.Scopes), RateLimitPerMin: r.RateLimitPerMin,
            CreatedAt: r.CreatedAt, LastUsedAt: r.LastUsedAt, RevokedAt: r.RevokedAt,
        })
    }
    c.JSON(http.StatusOK, gin.H{"data": out})
}

type rotateReq struct {
    Reason string `json:"reason" binding:"required,max=50"`
}

type rotateResp struct {
    NewID         uuid.UUID `json:"new_id"`
    NewPlaintext  string    `json:"new_plaintext"`
    OldID         uuid.UUID `json:"old_id"`
    OldValidUntil time.Time `json:"old_valid_until"`
    WarnOnce      string    `json:"warning"`
}

func (h *APIKeysHandler) Rotate(c *gin.Context) {
    tenantID := mustTenantID(c)
    oldID, err := uuid.Parse(c.Param("keyId"))
    if err != nil { c.JSON(400, gin.H{"error":"invalid_key_id"}); return }

    var body rotateReq
    if err := c.ShouldBindJSON(&body); err != nil {
        c.JSON(400, gin.H{"error":"invalid_request","message":err.Error()}); return
    }

    out, err := h.svc.Rotate(c.Request.Context(), tenantID, oldID, body.Reason)
    if err != nil {
        apikeyServiceErrorToHTTP(c, err); return
    }
    c.JSON(http.StatusOK, rotateResp{
        NewID: out.NewID, NewPlaintext: out.NewPlaintext,
        OldID: oldID, OldValidUntil: out.OldExpiresAt,
        WarnOnce: "Store the new key now. The old key remains valid for 24h.",
    })
}

func (h *APIKeysHandler) Revoke(c *gin.Context) {
    tenantID := mustTenantID(c)
    id, err := uuid.Parse(c.Param("keyId"))
    if err != nil { c.JSON(400, gin.H{"error":"invalid_key_id"}); return }

    err = h.svc.Revoke(c.Request.Context(), tenantID, id, "merchant_revoked")
    if err != nil { apikeyServiceErrorToHTTP(c, err); return }
    c.Status(http.StatusNoContent)
}

// subscription_plan is injected by StoreMiddleware (see P3 Task 7).
func mustSubscriptionPlan(c *gin.Context) subscription.SubscriptionPlan {
    v, _ := c.Get("subscription_plan")
    p, _ := v.(subscription.SubscriptionPlan)
    return p
}

func apikeyServiceErrorToHTTP(c *gin.Context, err error) {
    switch {
    case errors.Is(err, apikeys.ErrNotFound):
        c.JSON(http.StatusNotFound, gin.H{"error":"not_found"})
    case errors.Is(err, apikeys.ErrPlanDoesNotAllowAPI),
         errors.Is(err, apikeys.ErrWriteScopeRequiresPro),
         errors.Is(err, apikeys.ErrRateLimitExceedsPlanCeiling):
        c.JSON(http.StatusBadRequest, gin.H{"error":"plan_mismatch","message":err.Error()})
    default:
        c.JSON(http.StatusInternalServerError, gin.H{"error":"internal"})
    }
}
```

- [ ] **Step 3: Register routes in `routes.go`**

```go
// Inside the admin store-scoped group, AFTER readonly.RequireActive:
apiKeys := storeRoute.Group("/api-keys")
apiKeys.Use(plangate.RequireFeature(plangate.FeatureReadAPI)) // Studio+
{
    apiKeys.GET("",    deps.APIKeysHandler.List)
    apiKeys.DELETE("/:keyId", deps.APIKeysHandler.Revoke)

    // Create/rotate require Pro (write scopes); service layer enforces per-scope.
    // We still gate at the route level on FeatureReadAPI (Studio+) so a Studio
    // merchant can create *read-only* keys; the service rejects write-scope on Studio.
    apiKeys.POST("",    deps.APIKeysHandler.Create)
    apiKeys.POST("/:keyId/rotate", deps.APIKeysHandler.Rotate)
}
```

- [ ] **Step 4: Run integration tests — expect PASS**

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/handlers/admin/{apikeys_handler,apikeys_handler_test,routes}.go
git commit -m "feat(admin): api-key CRUD endpoints gated on plan (Studio+ read, Pro write)"
```

---

## Task 5: API authentication middleware

**Files:**
- Create: `services/marketplace-api/internal/apikeys/middleware.go`
- Create: `services/marketplace-api/internal/apikeys/middleware_test.go`

**Spec references:** §18.4.

- [ ] **Step 1: Failing tests**

```go
func TestMiddleware_ValidKey_AllowsAndPopulatesContext(t *testing.T) {
    mw, env := newMiddlewareTestEnv(t)
    k := env.SeedKey(t, "products:read")
    r := gin.New()
    r.Use(mw.Authenticate())
    r.GET("/v1/ping", func(c *gin.Context) {
        tid, _ := c.Get("tenant_id")
        scopes, _ := c.Get("api_key_scopes")
        c.JSON(200, gin.H{"tenant_id": tid, "scopes": scopes})
    })
    req := httptest.NewRequest("GET","/v1/ping",nil)
    req.Header.Set("Authorization","Bearer "+k.Plaintext)
    w := httptest.NewRecorder(); r.ServeHTTP(w, req)
    require.Equal(t, 200, w.Code)
    require.Contains(t, w.Body.String(), k.TenantID.String())
}

func TestMiddleware_MissingBearer_Returns401(t *testing.T) {
    mw, _ := newMiddlewareTestEnv(t)
    r := gin.New(); r.Use(mw.Authenticate()); r.GET("/v1/x", func(c *gin.Context) { c.Status(200) })
    req := httptest.NewRequest("GET","/v1/x",nil)
    w := httptest.NewRecorder(); r.ServeHTTP(w, req)
    require.Equal(t, 401, w.Code)
}

func TestMiddleware_WrongPrefix_Returns401(t *testing.T) {
    mw, _ := newMiddlewareTestEnv(t)
    r := gin.New(); r.Use(mw.Authenticate()); r.GET("/v1/x", func(c *gin.Context) { c.Status(200) })
    req := httptest.NewRequest("GET","/v1/x",nil)
    req.Header.Set("Authorization","Bearer sk_live_xxx")
    w := httptest.NewRecorder(); r.ServeHTTP(w, req)
    require.Equal(t, 401, w.Code)
}

func TestMiddleware_RevokedKey_Returns401(t *testing.T) {
    mw, env := newMiddlewareTestEnv(t)
    k := env.SeedKey(t, "products:read")
    require.NoError(t, env.Svc.Revoke(context.Background(), k.TenantID, k.ID, "test"))
    // Cache must be invalidated by Revoke; otherwise a cached entry keeps the key alive.
    req := httptest.NewRequest("GET","/v1/x",nil)
    req.Header.Set("Authorization","Bearer "+k.Plaintext)
    r := gin.New(); r.Use(mw.Authenticate()); r.GET("/v1/x", func(c *gin.Context) { c.Status(200) })
    w := httptest.NewRecorder(); r.ServeHTTP(w, req)
    require.Equal(t, 401, w.Code)
}

func TestMiddleware_HotPathUsesCache(t *testing.T) {
    mw, env := newMiddlewareTestEnv(t)
    k := env.SeedKey(t, "products:read")
    doOnce := func() int64 {
        req := httptest.NewRequest("GET","/v1/x",nil)
        req.Header.Set("Authorization","Bearer "+k.Plaintext)
        r := gin.New(); r.Use(mw.Authenticate()); r.GET("/v1/x", func(c *gin.Context) { c.Status(200) })
        t0 := time.Now()
        w := httptest.NewRecorder(); r.ServeHTTP(w, req)
        require.Equal(t, 200, w.Code)
        return time.Since(t0).Microseconds()
    }
    firstUs  := doOnce() // cold: runs bcrypt
    secondUs := doOnce() // hot:  cache hit, no bcrypt
    require.Less(t, secondUs, firstUs/5, "cached verify should be ≥5x faster than cold bcrypt")
}
```

- [ ] **Step 2: Write `middleware.go`**

```go
package apikeys

import (
    "net/http"
    "strings"

    "github.com/gin-gonic/gin"
)

type Middleware struct {
    repo  *Repo
    cache *Cache
}

func NewMiddleware(repo *Repo, cache *Cache) *Middleware {
    return &Middleware{repo: repo, cache: cache}
}

// Authenticate extracts a bearer key, verifies, and populates the context.
// On any failure the handler is aborted with 401; responses never leak *why*.
func (m *Middleware) Authenticate() gin.HandlerFunc {
    return func(c *gin.Context) {
        key, ok := extractBearer(c)
        if !ok { abort401(c); return }

        // Hot-path: cache hit.
        if e, ok := m.cache.Get(key); ok {
            populateContext(c, e)
            c.Next()
            return
        }

        // Cold-path: prefix lookup.
        prefix, ok := ExtractPrefix(key)
        if !ok {
            // Spend bcrypt time anyway to keep timing parity.
            _ = VerifyDummy(key)
            abort401(c); return
        }

        // We don't know tenant_id yet — the prefix is scoped per tenant. Look up
        // across all tenants by prefix alone; the unique index on (tenant_id,
        // key_prefix) guarantees at most one row per tenant, but globally
        // multiple tenants could share the same prefix. Do a query across all
        // rows for the prefix and try each (almost always zero or one).
        candidates, err := m.findCandidatesByPrefix(c, prefix)
        if err != nil || len(candidates) == 0 {
            _ = VerifyDummy(key)
            abort401(c); return
        }

        for _, row := range candidates {
            if !row.IsUsable(timeNow()) { continue }
            if err := Verify(row.KeyHash, key); err == nil {
                e := CacheEntry{
                    KeyID: row.ID, TenantID: row.TenantID, StoreID: row.StoreID,
                    Scopes: []string(row.Scopes), RateLimitPerMin: row.RateLimitPerMin,
                    RevokedAt: row.RevokedAt,
                }
                m.cache.Put(key, e)
                populateContext(c, e)
                c.Next()
                return
            }
        }
        abort401(c)
    }
}

func (m *Middleware) findCandidatesByPrefix(c *gin.Context, prefix string) ([]APIKey, error) {
    var rows []APIKey
    err := m.repo.db.WithContext(c.Request.Context()).
        Where("key_prefix = ?", prefix).Find(&rows).Error
    return rows, err
}

func extractBearer(c *gin.Context) (string, bool) {
    h := c.GetHeader("Authorization")
    if !strings.HasPrefix(h, "Bearer ") { return "", false }
    key := strings.TrimPrefix(h, "Bearer ")
    if key == "" { return "", false }
    if !strings.HasPrefix(key, livePrefix) && !strings.HasPrefix(key, testPrefix) {
        return "", false
    }
    return key, true
}

func populateContext(c *gin.Context, e CacheEntry) {
    c.Set("tenant_id",         e.TenantID.String())
    c.Set("store_id",          e.StoreID.String())
    c.Set("api_key_id",        e.KeyID.String())
    c.Set("api_key_scopes",    e.Scopes)
    c.Set("api_key_rate_limit", e.RateLimitPerMin)
    c.Set("auth_method",       "api_key")
}

func abort401(c *gin.Context) {
    c.AbortWithStatusJSON(http.StatusUnauthorized,
        gin.H{"error":"unauthorized","message":"invalid_api_key"})
}

// timeNow is package-level so tests can stub it. Defaults to time.Now.
var timeNow = time.Now
```

- [ ] **Step 3: Run tests — expect PASS**

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/internal/apikeys/{middleware,middleware_test}.go
git commit -m "feat(apikeys): bearer-auth middleware with timing-safe lookup + 60s cache"
```

---

## Task 6: Scope enforcement + route registration helper

**Files:**
- Modify: `services/marketplace-api/internal/apikeys/scopes.go` (add `RequireScope`)
- Modify: `services/marketplace-api/internal/apikeys/scopes_test.go`

- [ ] **Step 1: Failing tests**

```go
func TestRequireScope_AllowsKeyWithMatchingScope(t *testing.T) {
    r := gin.New()
    r.Use(func(c *gin.Context) {
        c.Set("api_key_scopes", []string{"products:read","orders:read"})
        c.Next()
    })
    r.GET("/ok", apikeys.RequireScope(apikeys.ScopeProductsRead), func(c *gin.Context){ c.Status(200) })
    req := httptest.NewRequest("GET","/ok",nil)
    w := httptest.NewRecorder(); r.ServeHTTP(w, req)
    require.Equal(t, 200, w.Code)
}

func TestRequireScope_RejectsWithout403(t *testing.T) {
    r := gin.New()
    r.Use(func(c *gin.Context) { c.Set("api_key_scopes", []string{"orders:read"}); c.Next() })
    r.GET("/denied", apikeys.RequireScope(apikeys.ScopeProductsWrite), func(c *gin.Context){ c.Status(200) })
    req := httptest.NewRequest("GET","/denied",nil)
    w := httptest.NewRecorder(); r.ServeHTTP(w, req)
    require.Equal(t, 403, w.Code, "scope mismatch is 403 (authorized but not permitted)")
    require.Contains(t, w.Body.String(), "insufficient_scope")
}

func TestRequireScope_NoContextScopes_Returns401(t *testing.T) {
    r := gin.New()
    r.GET("/x", apikeys.RequireScope(apikeys.ScopeProductsRead), func(c *gin.Context){ c.Status(200) })
    req := httptest.NewRequest("GET","/x",nil)
    w := httptest.NewRecorder(); r.ServeHTTP(w, req)
    require.Equal(t, 401, w.Code, "no scope set implies middleware not run / no auth")
}
```

- [ ] **Step 2: Add `RequireScope` to `scopes.go`**

```go
func RequireScope(required Scope) gin.HandlerFunc {
    return func(c *gin.Context) {
        raw, ok := c.Get("api_key_scopes")
        if !ok {
            c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error":"unauthorized"}); return
        }
        scopes, _ := raw.([]string)
        for _, s := range scopes {
            if Scope(s) == required {
                c.Next()
                return
            }
        }
        c.AbortWithStatusJSON(http.StatusForbidden,
            gin.H{"error":"insufficient_scope","required": string(required)})
    }
}
```

- [ ] **Step 3: Run — expect PASS**

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/internal/apikeys/{scopes,scopes_test}.go
git commit -m "feat(apikeys): per-route RequireScope helper (403 on mismatch)"
```

---

## Task 7: Per-key rate limiting

**Files:**
- Create: `services/marketplace-api/internal/apikeys/ratelimit.go`
- Create: `services/marketplace-api/internal/apikeys/ratelimit_test.go`

**Spec references:** §18.4 "per-key rate limits".

- [ ] **Step 1: Failing tests**

```go
func TestRateLimit_AllowsUpToLimitPerMinute(t *testing.T) {
    limiter := apikeys.NewRateLimiter(nil /* in-memory fallback */)
    keyID := uuid.New().String()
    for i := 0; i < 5; i++ {
        require.True(t, limiter.Allow(keyID, 5))
    }
    require.False(t, limiter.Allow(keyID, 5), "6th request in the same minute must be denied")
}

func TestRateLimit_IsPerKeyNotPerTenant(t *testing.T) {
    limiter := apikeys.NewRateLimiter(nil)
    k1, k2 := uuid.New().String(), uuid.New().String()
    for i := 0; i < 3; i++ {
        require.True(t, limiter.Allow(k1, 3))
    }
    require.False(t, limiter.Allow(k1, 3))
    // Same tenant, different key — fresh bucket.
    for i := 0; i < 3; i++ {
        require.True(t, limiter.Allow(k2, 3))
    }
}

func TestMiddleware_RateLimitExceeded_Returns429(t *testing.T) {
    mw, env := newMiddlewareTestEnv(t)
    k := env.SeedKeyWithRate(t, "products:read", 2)

    r := gin.New(); r.Use(mw.Authenticate(), mw.EnforceRateLimit())
    r.GET("/x", func(c *gin.Context) { c.Status(200) })

    for i := 0; i < 2; i++ {
        req := httptest.NewRequest("GET","/x",nil)
        req.Header.Set("Authorization","Bearer "+k.Plaintext)
        w := httptest.NewRecorder(); r.ServeHTTP(w, req)
        require.Equal(t, 200, w.Code)
    }
    req := httptest.NewRequest("GET","/x",nil)
    req.Header.Set("Authorization","Bearer "+k.Plaintext)
    w := httptest.NewRecorder(); r.ServeHTTP(w, req)
    require.Equal(t, 429, w.Code)
    require.Contains(t, w.Body.String(), "rate_limited")
}
```

- [ ] **Step 2: Write `ratelimit.go`**

```go
package apikeys

import (
    "net/http"
    "time"

    "github.com/gin-gonic/gin"
    "github.com/redis/go-redis/v9"

    "github.com/tesserix/marketplace-api/internal/ratelimit"
)

type RateLimiter struct {
    inner ratelimit.Limiter
}

// NewRateLimiter wraps internal/ratelimit. When redis is nil, falls back to
// the in-memory token bucket already exported by that package.
func NewRateLimiter(redisClient *redis.Client) *RateLimiter {
    return &RateLimiter{inner: ratelimit.NewTokenBucket(redisClient, "apikey")}
}

// Allow reports whether the key may proceed right now. limit is the per-minute
// ceiling taken from the api_keys row.
func (rl *RateLimiter) Allow(keyID string, limit int) bool {
    return rl.inner.Allow(keyID, limit, time.Minute)
}

// EnforceRateLimit is the gin middleware companion to Authenticate. Must run
// AFTER Authenticate so api_key_id + api_key_rate_limit are on the context.
func (m *Middleware) EnforceRateLimit() gin.HandlerFunc {
    return func(c *gin.Context) {
        keyID, ok := c.Get("api_key_id")
        if !ok { c.Next(); return } // not an api-key request — nothing to limit here
        limit, _ := c.Get("api_key_rate_limit")
        if !m.rl.Allow(keyID.(string), limit.(int)) {
            c.AbortWithStatusJSON(http.StatusTooManyRequests,
                gin.H{"error":"rate_limited","retry_after_seconds": 60})
            return
        }
        c.Next()
    }
}
```

> Note: `Middleware` gains an `rl *RateLimiter` field; update `NewMiddleware` signature accordingly and wire it from `main.go` in Task 9.

- [ ] **Step 3: Run — expect PASS**

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/internal/apikeys/{ratelimit,ratelimit_test}.go \
        services/marketplace-api/internal/apikeys/middleware.go
git commit -m "feat(apikeys): per-key rate limiting with 429 on exceed"
```

---

## Task 8: Async last-used update worker

**Files:**
- Modify: `services/marketplace-api/internal/apikeys/middleware.go`
- Create: `services/marketplace-api/internal/apikeys/lastused.go`
- Create: `services/marketplace-api/internal/apikeys/lastused_test.go`

**Spec references:** §18.4 (tracking), P8 (IP HMAC).

- [ ] **Step 1: Failing test — request updates last_used_at eventually**

```go
func TestLastUsedWorker_UpdatesAsync(t *testing.T) {
    mw, env := newMiddlewareTestEnv(t)
    k := env.SeedKey(t, "products:read")

    req := httptest.NewRequest("GET","/x",nil)
    req.Header.Set("Authorization","Bearer "+k.Plaintext)
    req.RemoteAddr = "203.0.113.5:40000"
    r := gin.New(); r.Use(mw.Authenticate()); r.GET("/x", func(c *gin.Context){ c.Status(200) })
    w := httptest.NewRecorder(); r.ServeHTTP(w, req)
    require.Equal(t, 200, w.Code)

    // Worker channel is drained before assertion.
    mw.FlushLastUsedForTesting()

    var row apikeys.APIKey
    require.NoError(t, env.DB.First(&row, "id=?", k.ID).Error)
    require.NotNil(t, row.LastUsedAt)
    require.NotNil(t, row.LastUsedIPHash)
    require.Len(t, *row.LastUsedIPHash, 64) // HMAC-SHA256 hex
}
```

- [ ] **Step 2: Write `lastused.go`**

```go
package apikeys

import (
    "context"
    "time"

    "github.com/google/uuid"

    "github.com/tesserix/marketplace-api/internal/ipprivacy"
)

type lastUsedJob struct {
    id     uuid.UUID
    at     time.Time
    ipHash string
}

// startLastUsedWorker launches a goroutine that drains the queue and
// fire-and-forgets repo updates. Buffer 1024; on overflow, the oldest is dropped
// (the metric "api.key.last_used_dropped" records this; not on the hot path).
func (m *Middleware) startLastUsedWorker(repo *Repo) {
    m.lastUsedCh = make(chan lastUsedJob, 1024)
    go func() {
        for j := range m.lastUsedCh {
            ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
            _ = repo.UpdateLastUsed(ctx, j.id, j.at, j.ipHash)
            cancel()
        }
    }()
}

func (m *Middleware) enqueueLastUsed(id uuid.UUID, ip string) {
    if m.lastUsedCh == nil { return }
    hash := ipprivacy.HashIP(ip)
    select {
    case m.lastUsedCh <- lastUsedJob{id: id, at: time.Now(), ipHash: hash}:
    default:
        // queue full — drop; metric increments in Task 9
    }
}
```

- [ ] **Step 3: Hook into `Authenticate`**

In `middleware.go` Authenticate, after `populateContext` on the success path:

```go
m.enqueueLastUsed(e.KeyID, c.ClientIP())
```

- [ ] **Step 4: Run tests — expect PASS**

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/apikeys/{lastused,lastused_test,middleware}.go
git commit -m "feat(apikeys): async last-used + ip-hash update worker"
```

---

## Task 9: Observability counters + audit events + main.go wiring

**Files:**
- Modify: `services/marketplace-api/internal/audit/events.go` (add `EmitAPIKeyEvent`)
- Modify: `services/marketplace-api/internal/apikeys/middleware.go` (counter increments)
- Modify: `services/marketplace-api/internal/apikeys/service.go` (audit emit on create/rotate/revoke)
- Modify: `services/marketplace-api/cmd/marketplace-api/main.go` (wire everything + register public API router group)

- [ ] **Step 1: Add audit helper**

```go
// internal/audit/events.go
func (e *Emitter) EmitAPIKeyEvent(c *gin.Context, evt APIKeyEvent) {
    e.EmitSecurity(c, SecurityEvent{
        Kind: "api_key." + evt.Action, // e.g. api_key.created, api_key.revoked
        Severity: evt.Severity,
        TenantID: evt.TenantID,
        Metadata: map[string]any{
            "key_id":      evt.KeyID.String(),
            "store_id":    evt.StoreID.String(),
            "label":       evt.Label,
            "scopes":      evt.Scopes,
            "rate_limit":  evt.RateLimitPerMin,
            "reason":      evt.Reason,
        },
    })
}

type APIKeyEvent struct {
    Action, Reason, Label string
    Severity              Severity
    TenantID, KeyID, StoreID uuid.UUID
    Scopes                []string
    RateLimitPerMin       int
}
```

- [ ] **Step 2: Emit audit from service layer**

In `Create` / `Rotate` / `Revoke`, on success, call `s.emitter.EmitAPIKeyEvent(...)` with action `"created"` (info), `"rotated"` (info), `"revoked"` (info), `"created.invalid_plan"` (warning) respectively.

- [ ] **Step 3: Counter increments in middleware**

```go
// internal/apikeys/middleware.go
var (
    mApiKeyUsed = metrics.NewCounter("api.key.used",          []string{"tenant","scope"})
    mApiKeyRL   = metrics.NewCounter("api.key.rate_limited",  []string{"tenant"})
    mApiKeyAuth = metrics.NewCounter("api.key.auth_failed",   []string{"reason"})
)
```

Increment:
- `mApiKeyUsed` on success — one tick per request (not per scope check).
- `mApiKeyAuth{reason="missing_bearer"|"wrong_prefix"|"revoked"|"hash_mismatch"}` on 401 paths.
- `mApiKeyRL` on 429.

Also: on rate-limit 429, emit an audit event with severity=warning and `action:"rate_limited"`.

- [ ] **Step 4: Wire everything in main.go**

```go
// cmd/marketplace-api/main.go
apiKeyRepo  := apikeys.NewRepo(db)
apiKeyCache := apikeys.NewCache(60 * time.Second)
apiKeySvc   := apikeys.NewService(apiKeyRepo, apiKeyCache, apikeys.EnvLive, emitter)
apiKeyMw    := apikeys.NewMiddleware(apiKeyRepo, apiKeyCache, apikeys.NewRateLimiter(redisClient))

// Admin wiring for the CRUD handler.
deps.APIKeysHandler = admin.NewAPIKeysHandler(apiKeySvc, apiKeyRepo)

// Public R/W API router group — brand-new mount point.
publicAPI := router.Group("/api/v1",
    apiKeyMw.Authenticate(),
    apiKeyMw.EnforceRateLimit(),
)
{
    publicAPI.GET("/products",    apikeys.RequireScope(apikeys.ScopeProductsRead),  productsHandler.List)
    publicAPI.POST("/products",   apikeys.RequireScope(apikeys.ScopeProductsWrite), productsHandler.Create)
    publicAPI.GET("/orders",      apikeys.RequireScope(apikeys.ScopeOrdersRead),    ordersHandler.List)
    publicAPI.POST("/orders",     apikeys.RequireScope(apikeys.ScopeOrdersWrite),   ordersHandler.Create)
    publicAPI.GET("/customers",   apikeys.RequireScope(apikeys.ScopeCustomersRead), customersHandler.List)
    publicAPI.POST("/customers",  apikeys.RequireScope(apikeys.ScopeCustomersWrite),customersHandler.Create)
    publicAPI.GET("/categories",  apikeys.RequireScope(apikeys.ScopeCategoriesRead),categoriesHandler.List)
    publicAPI.POST("/categories", apikeys.RequireScope(apikeys.ScopeCategoriesWrite),categoriesHandler.Create)
    publicAPI.GET("/coupons",     apikeys.RequireScope(apikeys.ScopeCouponsRead),   couponsHandler.List)
    publicAPI.POST("/coupons",    apikeys.RequireScope(apikeys.ScopeCouponsWrite),  couponsHandler.Create)
}
```

Note: the public API handlers may not all exist yet (`productsHandler.List` on the public side vs admin side). Where no public-side handler exists, mount a stub returning 501 Not Implemented so the wiring is provable today; actual logic lands per feature-area plan.

- [ ] **Step 5: Build + smoke**

```bash
cd services/marketplace-api
go build ./...
go test ./internal/apikeys/... -v
```

- [ ] **Step 6: Commit**

```bash
git add services/marketplace-api/internal/audit/events.go \
        services/marketplace-api/internal/apikeys/{middleware,service}.go \
        services/marketplace-api/cmd/marketplace-api/main.go
git commit -m "feat(apikeys): audit events + metrics counters + public /api/v1 router wiring"
```

---

## Task 10: Security test battery

**Files:**
- Create: `services/marketplace-api/internal/apikeys/security_test.go`

**Spec references:** §18.4.

- [ ] **Step 1: Revocation is immediate (cache invalidation enforced)**

```go
func TestSecurity_RevokedKey_RejectedEvenIfCached(t *testing.T) {
    mw, env := newMiddlewareTestEnv(t)
    k := env.SeedKey(t, "products:read")

    // Prime the cache.
    do := func() int {
        req := httptest.NewRequest("GET","/x",nil); req.Header.Set("Authorization","Bearer "+k.Plaintext)
        r := gin.New(); r.Use(mw.Authenticate()); r.GET("/x", func(c *gin.Context){ c.Status(200) })
        w := httptest.NewRecorder(); r.ServeHTTP(w, req)
        return w.Code
    }
    require.Equal(t, 200, do())

    require.NoError(t, env.Svc.Revoke(context.Background(), k.TenantID, k.ID, "compromised"))
    // Revoke calls cache.InvalidateByID — next request must 401.
    require.Equal(t, 401, do())
}
```

- [ ] **Step 2: Rotation overlap — both keys valid for 24h**

```go
func TestSecurity_RotationOverlap_OldAndNewBothValid(t *testing.T) {
    mw, env := newMiddlewareTestEnv(t)
    old := env.SeedKey(t, "products:read")

    rot, err := env.Svc.Rotate(context.Background(), old.TenantID, old.ID, "scheduled")
    require.NoError(t, err)

    // Both accepted during the overlap.
    for _, key := range []string{old.Plaintext, rot.NewPlaintext} {
        req := httptest.NewRequest("GET","/x",nil); req.Header.Set("Authorization","Bearer "+key)
        r := gin.New(); r.Use(mw.Authenticate()); r.GET("/x", func(c *gin.Context){ c.Status(200) })
        w := httptest.NewRecorder(); r.ServeHTTP(w, req)
        require.Equal(t, 200, w.Code, "both keys must auth during overlap")
    }
}

func TestSecurity_RotationOverlap_ExpiresAfter24h(t *testing.T) {
    mw, env := newMiddlewareTestEnv(t)
    old := env.SeedKey(t, "products:read")
    _, err := env.Svc.Rotate(context.Background(), old.TenantID, old.ID, "scheduled")
    require.NoError(t, err)

    // Fast-forward time beyond overlap.
    apikeys.SetTimeNowForTesting(t, func() time.Time { return time.Now().Add(25 * time.Hour) })

    req := httptest.NewRequest("GET","/x",nil); req.Header.Set("Authorization","Bearer "+old.Plaintext)
    r := gin.New(); r.Use(mw.Authenticate()); r.GET("/x", func(c *gin.Context){ c.Status(200) })
    w := httptest.NewRecorder(); r.ServeHTTP(w, req)
    require.Equal(t, 401, w.Code)
}
```

- [ ] **Step 3: Rate limiting is per-key, not per-tenant**

```go
func TestSecurity_RateLimitIsPerKeyNotPerTenant(t *testing.T) {
    mw, env := newMiddlewareTestEnv(t)
    tenantID := uuid.New()
    k1 := env.SeedKeyForTenantWithRate(t, tenantID, "products:read", 2)
    k2 := env.SeedKeyForTenantWithRate(t, tenantID, "products:read", 2)

    run := func(plaintext string) int {
        req := httptest.NewRequest("GET","/x",nil); req.Header.Set("Authorization","Bearer "+plaintext)
        r := gin.New(); r.Use(mw.Authenticate(), mw.EnforceRateLimit())
        r.GET("/x", func(c *gin.Context){ c.Status(200) })
        w := httptest.NewRecorder(); r.ServeHTTP(w, req)
        return w.Code
    }

    // Burn k1's budget.
    require.Equal(t, 200, run(k1.Plaintext))
    require.Equal(t, 200, run(k1.Plaintext))
    require.Equal(t, 429, run(k1.Plaintext))

    // k2 on the same tenant still has a full bucket.
    require.Equal(t, 200, run(k2.Plaintext))
}
```

- [ ] **Step 4: Scope mismatch returns 403 (not 401)**

```go
func TestSecurity_ScopeMismatch_Returns403(t *testing.T) {
    mw, env := newMiddlewareTestEnv(t)
    k := env.SeedKey(t, "orders:read") // only orders:read

    r := gin.New()
    r.Use(mw.Authenticate())
    r.GET("/v1/products",
        apikeys.RequireScope(apikeys.ScopeProductsRead),
        func(c *gin.Context){ c.Status(200) })

    req := httptest.NewRequest("GET","/v1/products",nil)
    req.Header.Set("Authorization","Bearer "+k.Plaintext)
    w := httptest.NewRecorder(); r.ServeHTTP(w, req)
    require.Equal(t, 403, w.Code, "auth succeeded; permission denied → 403")
    require.Contains(t, w.Body.String(), "insufficient_scope")
}
```

- [ ] **Step 5: Timing parity — prefix-miss vs prefix-hit-hash-miss**

```go
func TestSecurity_TimingParity_PrefixMissVsHashMiss(t *testing.T) {
    mw, env := newMiddlewareTestEnv(t)
    k := env.SeedKey(t, "products:read")

    // Generate a key that shares no prefix with any row.
    bogusKey, err := apikeys.Generate(apikeys.EnvLive)
    require.NoError(t, err)
    prefixMissKey := bogusKey.Plaintext

    // Generate a key that SHARES the same 8-char prefix as `k` but a wrong hash.
    // Best we can do: mutate the trailing body of k.Plaintext.
    prefixHitKey := k.Plaintext[:len(k.Plaintext)-4] + "ZZZZ"

    meas := func(plaintext string) time.Duration {
        var best time.Duration = time.Hour
        for i := 0; i < 5; i++ {
            req := httptest.NewRequest("GET","/x",nil); req.Header.Set("Authorization","Bearer "+plaintext)
            r := gin.New(); r.Use(mw.Authenticate()); r.GET("/x", func(c *gin.Context){ c.Status(200) })
            t0 := time.Now()
            w := httptest.NewRecorder(); r.ServeHTTP(w, req)
            d := time.Since(t0)
            require.Equal(t, 401, w.Code)
            if d < best { best = d }
        }
        return best
    }

    miss := meas(prefixMissKey)
    hit  := meas(prefixHitKey)
    delta := miss - hit
    if delta < 0 { delta = -delta }
    require.Less(t, delta, 5*time.Millisecond,
        "prefix-miss vs prefix-hit-hash-miss must differ by <5ms (got %v vs %v)", miss, hit)
}
```

- [ ] **Step 6: Run the whole battery**

```bash
go test ./internal/apikeys/... -run Security -v
```

- [ ] **Step 7: Commit**

```bash
git add services/marketplace-api/internal/apikeys/security_test.go
git commit -m "test(apikeys): revocation/rotation/rate-limit/scope/timing security battery"
```

---

## Final verification

- [ ] `go build ./...` clean.
- [ ] `go test -tags=integration ./...` all green.
- [ ] Migration up + down roundtrip: `migrate up && migrate down 1 && migrate up` clean.
- [ ] `enterprise_api_keys` has unique index `(tenant_id, key_prefix)`.
- [ ] `mk8_live_` is the only production prefix produced; `mk8_test_` emitted only when `Env = EnvTest`.
- [ ] Plaintext is returned exactly once, in `CreateResult` and `RotateResult`. `List` never emits plaintext or hash.
- [ ] Starter/Trial plan → `ErrPlanDoesNotAllowAPI` (400).
- [ ] Studio + write scope → `ErrWriteScopeRequiresPro` (400).
- [ ] `rate_limit_per_min` > plan ceiling → `ErrRateLimitExceedsPlanCeiling` (400).
- [ ] Middleware 401 paths always call `VerifyDummy` (or a successful `Verify` on a real hash) before returning — timing parity < 5ms.
- [ ] Revoke immediately invalidates the cache by ID.
- [ ] Rotation sets `old.revoked_at = now() + 24h`; after 24h, old key rejected.
- [ ] `RequireScope` returns 403 (not 401) on scope mismatch when auth succeeded.
- [ ] Per-key rate limit firing returns 429 and increments `api.key.rate_limited{tenant}`.
- [ ] Audit events emitted on create/rotate/revoke; severity=info for merchant actions, severity=warning for rate-limit-exceeded / plan-mismatch attempts.
- [ ] `last_used_at` and `last_used_ip_hash` populated asynchronously (IP HMAC via P8).

## What's now unlocked

- **Public R/W integrations** on Pro plans (Zapier, Make, custom middleware) using bearer tokens.
- **Read-only integrations** on Studio (analytics readers, BI exports).
- **P16** (admin UI) — binds the `/admin/stores/:storeId/api-keys` endpoints to a key-management screen (display `Display(plaintext)`, scope checkboxes, rate-limit slider clamped to plan ceiling, rotation/revoke CTAs).
- **P17** (observability dashboards) — graphs `api.key.used`, `api.key.rate_limited`, `api.key.auth_failed` per tenant; alerts on auth-fail spikes.
- **Future OAuth 2.0 work** — the scope enum + `RequireScope` helper are reusable; OAuth access tokens would populate the same `api_key_scopes` context key and the existing route-level `RequireScope(...)` would light up automatically.

## Execution handoff

Plan complete. Execute with **superpowers:subagent-driven-development** (recommended) or **superpowers:executing-plans**. Tasks 1 → 10 in order; Tasks 4 and 7 depend on the Middleware struct gaining the `rl` field introduced in Task 7, so do not commit Task 4 until Task 7's signature change is captured in the same branch or Task 4 stubs the rate-limit dependency.
