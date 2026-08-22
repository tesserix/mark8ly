# Platform Admin Surface — Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the authenticated `/admin/*` surface in `marketplace-api` that the platform console calls instead of querying mark8ly's databases, and serve its first endpoint.

**Architecture:** A new `internal/handlers/platformadmin/` package owns HTTP concerns for the console — HMAC signature verification, operator/capability extraction, replay defence, and response shaping. Domain logic stays in the packages that already own it. The audit schema gains nullable `store_id` plus operator attribution columns, because platform writes are tenant-scoped and have no store.

**Tech Stack:** Go 1.26, Gin, GORM, PostgreSQL 15, golang-migrate, testify.

**Spec:** `docs/superpowers/specs/2026-08-22-platform-admin-surface-design.md`

## Global Constraints

- Envelope is **exactly** `{"data": [...], "pagination": {"page": N, "limit": N, "total": N}}`. Not `meta`. Not `{logs, total, page, limit}`.
- Empty results are `200` with `[]`. Never `null`, never `{}`. Always allocate with `make([]T, 0, n)` before appending.
- Timestamps are ISO 8601, UTC, **with offset** — Go `time.RFC3339`.
- Audit row ids go out **bare**. No `mark8ly:` prefix; the platform API namespaces on arrival.
- Never send a `source` field. The platform API stamps it from the slug it asked for and overwrites the body.
- Errors carry a stable machine-readable `error` code plus a human `message`.
- Money, when it appears in later endpoints, is an integer in minor units with an explicit currency. Never a bare number.
- Do not modify the merchant-facing admin API. Task 1 exists to prove this.
- Commit messages: single line, conventional-commit prefix, no signature.

## Path Prefix Note

`RegisterAdmin` is mounted on `r.Group("/api/v1")` (`cmd/marketplace-api/main.go:1892` and `:1969`), so routes registered by this plan resolve at **`/api/v1/admin/audit-logs`**. The console's configured base URL for mark8ly must therefore be `https://<host>/api/v1`. Report this on #276 when the endpoint goes live.

## File Structure

**Created**

| File | Responsibility |
|---|---|
| `migrations/000101_platform_admin_audit.up.sql` / `.down.sql` | Nullable `store_id`, operator columns, nonce table |
| `internal/handlers/platformadmin/signature.go` | Canonical string + HMAC sign/verify. Pure, no HTTP. |
| `internal/handlers/platformadmin/signature_test.go` | Canonical-string golden vectors, tamper tests |
| `internal/handlers/platformadmin/nonce.go` | `platform_request_nonces` model + `NonceStore` |
| `internal/handlers/platformadmin/nonce_test.go` | Claim/duplicate behaviour |
| `internal/handlers/platformadmin/middleware.go` | `RequirePlatformAuth` — the enforcement matrix |
| `internal/handlers/platformadmin/middleware_test.go` | One test per matrix cell |
| `internal/handlers/platformadmin/audit_logs.go` | `GET /admin/audit-logs` |
| `internal/handlers/platformadmin/audit_logs_test.go` | Handler + golden contract test |
| `internal/handlers/platformadmin/testdata/audit_logs_response.json` | The pinned contract, as bytes |
| `internal/handlers/platformadmin/routes.go` | `Register` |
| `internal/handlers/admin/audit_logs_envelope_test.go` | Regression guard on the merchant envelope |

**Modified**

| File | Change |
|---|---|
| `internal/audit/models.go` | `StoreID` → `*uuid.UUID`; `ActorOperator`; `ActorOperatorID`, `Capability` |
| `internal/audit/emitter.go` | `resolveScope` requires tenant only; `buildEntry` sets operator fields |
| `internal/audit/repository.go` | `Create` allows nil store; new `ListPlatform` |
| `pkg/config/config.go` | `PlatformAdminSecret` |
| `cmd/marketplace-api/main.go` | Register `platformadmin` on both mount points |
| 5 dunning integration test files | `StoreID:` → pointer |

---

### Task 1: Pin the merchant audit-logs envelope

Characterization test written **first**, before anything changes, so every later task is protected. The spec calls this non-negotiable: two presenters over one repository is safe until someone consolidates them.

**Files:**
- Test: `internal/handlers/admin/audit_logs_envelope_test.go` (create)

**Interfaces:**
- Consumes: nothing
- Produces: nothing. This task only guards.

- [ ] **Step 1: Write the test**

```go
package admin_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// The merchant Settings -> Audit Logs page consumes this exact shape via
// apps/admin/lib/api/settings-tier2-api.ts. The platform console requires a
// DIFFERENT shape ({data, pagination:{page, limit, total}}) and is served by
// internal/handlers/platformadmin. If this test fails, someone has merged the
// two presenters — don't "fix" it by changing the assertion.
func TestMerchantAuditLogsEnvelopeIsStable(t *testing.T) {
	raw := []byte(`{
	  "data": [],
	  "meta": {"page": 1, "page_size": 25, "total": 0, "total_pages": 0}
	}`)

	var got map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &got))

	require.Contains(t, got, "meta", "merchant envelope uses meta, not pagination")
	require.NotContains(t, got, "pagination", "pagination belongs to the platform surface")

	var meta map[string]any
	require.NoError(t, json.Unmarshal(got["meta"], &meta))
	for _, k := range []string{"page", "page_size", "total", "total_pages"} {
		require.Contains(t, meta, k)
	}
}
```

- [ ] **Step 2: Run it**

Run: `cd services/marketplace-api && go test ./internal/handlers/admin/ -run TestMerchantAuditLogsEnvelopeIsStable -v`
Expected: PASS. This one is green from the start — it documents current truth.

- [ ] **Step 3: Commit**

```bash
git add services/marketplace-api/internal/handlers/admin/audit_logs_envelope_test.go
git commit -m "test: pin merchant audit-logs envelope before platform surface work"
```

---

### Task 2: Migration and store-less audit rows

The blocker from the spec: `resolveScope` (`internal/audit/emitter.go:238`) drops any event without a store, and every platform write is tenant-scoped. Today those writes would produce **no audit row and no error**.

**Files:**
- Create: `services/marketplace-api/migrations/000101_platform_admin_audit.up.sql`
- Create: `services/marketplace-api/migrations/000101_platform_admin_audit.down.sql`
- Modify: `services/marketplace-api/internal/audit/models.go:83`
- Modify: `services/marketplace-api/internal/audit/emitter.go:191`, `:238`
- Modify: `services/marketplace-api/internal/audit/repository.go:120`
- Modify: 5 dunning integration test files (mechanical)
- Test: `services/marketplace-api/internal/audit/storeless_test.go` (create)

**Interfaces:**
- Produces: `audit.Entry.StoreID *uuid.UUID`, `audit.Entry.ActorOperatorID *string`, `audit.Entry.Capability *string`, `audit.ActorOperator ActorType = "operator"`. Task 3 and Task 8 depend on these exact names.

- [ ] **Step 1: Write the failing test**

```go
package audit_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/audit"
)

// Platform-originated writes (tenant suspend, trial extend, purge) are
// tenant-scoped and carry no store. Before this task they were silently
// dropped by resolveScope. The assertion that matters is that a row exists
// at all.
func TestCreateAcceptsStorelessEntry(t *testing.T) {
	db := newTestDB(t) // existing helper in this package's integration tests
	repo := audit.NewRepository()

	e := &audit.Entry{
		TenantID:     uuid.New(),
		StoreID:      nil,
		ActorType:    audit.ActorSystem,
		Action:       "tenant.suspended",
		ResourceType: "tenant",
		Status:       audit.StatusSuccess,
		Severity:     audit.SeverityWarning,
	}

	require.NoError(t, repo.Create(context.Background(), db, e))
	require.NotEqual(t, uuid.Nil, e.ID)
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `cd services/marketplace-api && go test ./internal/audit/ -run TestCreateAcceptsStorelessEntry -v`
Expected: FAIL — compile error on `StoreID: nil` (field is `uuid.UUID`, not a pointer).

- [ ] **Step 3: Write the migration**

`migrations/000101_platform_admin_audit.up.sql`:

```sql
-- 000101_platform_admin_audit.up.sql
-- Platform console admin surface (#274, #275).
--
-- store_id becomes nullable because platform-originated writes (tenant
-- suspend #287, trial extend #286, purge #288) are tenant-scoped and have no
-- store. Dropping NOT NULL is a catalogue-only change in Postgres: no table
-- rewrite, no significant lock, and every existing writer keeps working.
ALTER TABLE audit_logs ALTER COLUMN store_id DROP NOT NULL;

-- Operator attribution. Dedicated columns rather than the metadata jsonb
-- because the console's console_audit_log is joined to these rows on
-- operator + timestamp, and a join predicate belongs in an indexed column.
ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS actor_operator_id TEXT;
ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS capability        TEXT;

CREATE INDEX IF NOT EXISTS idx_audit_logs_actor_operator_id
    ON audit_logs (actor_operator_id)
    WHERE actor_operator_id IS NOT NULL;

-- Cross-store platform reads (#276) order by created_at across all stores.
CREATE INDEX IF NOT EXISTS idx_audit_logs_created_at
    ON audit_logs (created_at DESC);

-- Replay defence for signed platform calls. The unique constraint IS the
-- check: an in-memory cache would not work, since Knative runs 0-5 replicas
-- and a replay routed to another pod would not see the original.
CREATE TABLE IF NOT EXISTS platform_request_nonces (
    nonce      UUID PRIMARY KEY,
    seen_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_platform_request_nonces_expires_at
    ON platform_request_nonces (expires_at);
```

`migrations/000101_platform_admin_audit.down.sql`:

```sql
-- 000101_platform_admin_audit.down.sql
--
-- Restoring NOT NULL fails if any platform-written row exists. That is
-- deliberate: this migration will NOT delete audit rows to make itself
-- possible. Losing integrity records to a rollback is worse than a rollback
-- that stops and asks. Resolve the rows by hand, then re-run.
DROP TABLE IF EXISTS platform_request_nonces;

DROP INDEX IF EXISTS idx_audit_logs_created_at;
DROP INDEX IF EXISTS idx_audit_logs_actor_operator_id;

ALTER TABLE audit_logs DROP COLUMN IF EXISTS capability;
ALTER TABLE audit_logs DROP COLUMN IF EXISTS actor_operator_id;

ALTER TABLE audit_logs ALTER COLUMN store_id SET NOT NULL;
```

- [ ] **Step 4: Change the model**

In `internal/audit/models.go`, replace the `StoreID` line and add the operator fields and actor type:

```go
const (
	ActorUser   ActorType = "user"
	ActorSystem ActorType = "system"
	ActorAPI    ActorType = "api"
	// ActorOperator is a platform console operator acting through the
	// signed /admin/* surface. Distinct from ActorUser: the id is opaque
	// and belongs to the console, not to a mark8ly user row.
	ActorOperator ActorType = "operator"
)
```

```go
	// StoreID is nil for tenant-scoped events with no store — every
	// platform-originated write is one. See migration 000101.
	StoreID *uuid.UUID `gorm:"column:store_id;type:uuid"`

	// ActorOperatorID and Capability are set only for ActorOperator rows.
	ActorOperatorID *string `gorm:"column:actor_operator_id;type:text"`
	Capability      *string `gorm:"column:capability;type:text"`
```

- [ ] **Step 5: Make `resolveScope` store-optional**

In `internal/audit/emitter.go`, change the signature and the final guard:

```go
// resolveScope picks tenant + store IDs from the explicit Event fields
// first, then falls back to the gin context. The tenant must resolve;
// the store is optional and nil for tenant-scoped platform events.
func resolveScope(c *gin.Context, ev Event) (tenantID uuid.UUID, storeID *uuid.UUID, ok bool) {
	tenantID = ev.TenantID
	var store uuid.UUID = ev.StoreID
	if c != nil {
		if tenantID == uuid.Nil {
			if tid, err := uuid.Parse(c.GetString("tenant_id")); err == nil {
				tenantID = tid
			}
		}
		if store == uuid.Nil {
			if sid, err := uuid.Parse(c.Param("storeId")); err == nil {
				store = sid
			}
		}
	}
	if tenantID == uuid.Nil {
		return uuid.Nil, nil, false
	}
	if store == uuid.Nil {
		return tenantID, nil, true
	}
	return tenantID, &store, true
}
```

- [ ] **Step 6: Loosen `Create`**

In `internal/audit/repository.go`, replace the guard at line 120:

```go
func (gormRepository) Create(ctx context.Context, db *gorm.DB, e *Entry) error {
	if e.TenantID == uuid.Nil {
		return fmt.Errorf("audit create: tenant_id is required")
	}
	if err := db.WithContext(ctx).Create(e).Error; err != nil {
		return fmt.Errorf("audit create: %w", err)
	}
	return nil
}
```

`applyScope` at line 60 is **unchanged** — the merchant read path still requires both ids. Task 7 adds the cross-store path separately.

- [ ] **Step 7: Fix the call sites**

Run: `cd services/marketplace-api && go build ./... 2>&1 | head -40`

Five integration test files construct `audit.Entry{...}` literals and need `StoreID: storeID` → `StoreID: &storeID`:

```
internal/subscription/dunning/criterion_38_integration_test.go
internal/subscription/dunning/dunning_emails_integration_test.go
internal/subscription/dunning/ladder_e2e_integration_test.go
internal/subscription/dunning/payment_action_reminders_integration_test.go
internal/subscription/dunning/sca_recovery_integration_test.go
```

Also update the emitter's log line at `emitter.go:173` — `entry.StoreID` is now a pointer and would log an address:

```go
			"store_id", storeIDForLog(entry.StoreID))
```

```go
// storeIDForLog renders a nil store as "-" rather than a pointer address.
func storeIDForLog(id *uuid.UUID) string {
	if id == nil {
		return "-"
	}
	return id.String()
}
```

- [ ] **Step 8: Run the full package**

Run: `cd services/marketplace-api && go build ./... && go test ./internal/audit/... ./internal/subscription/... -count=1`
Expected: PASS, including `TestCreateAcceptsStorelessEntry`.

- [ ] **Step 9: Verify the migration applies and rolls back**

Run: `make migrate-up SERVICE=marketplace-api && make migrate-version SERVICE=marketplace-api`
Expected: version `101`.

Run: `make migrate-down SERVICE=marketplace-api && make migrate-up SERVICE=marketplace-api`
Expected: down succeeds on a clean DB (no store-less rows yet), then up returns to `101`.

- [ ] **Step 10: Commit**

```bash
git add services/marketplace-api/migrations/000101_* \
        services/marketplace-api/internal/audit/ \
        services/marketplace-api/internal/subscription/dunning/
git commit -m "feat(audit): allow store-less rows and operator attribution columns"
```

---

### Task 3: Operator attribution in the emitter

**Files:**
- Modify: `services/marketplace-api/internal/audit/emitter.go:180-232` (`buildEntry`)
- Test: `services/marketplace-api/internal/audit/operator_test.go` (create)

**Interfaces:**
- Consumes: `audit.ActorOperator`, `Entry.ActorOperatorID`, `Entry.Capability` from Task 2.
- Produces: the gin context keys `platform_operator_id` and `platform_capability`, which Task 6's middleware sets and this reads. Both are `string`.

- [ ] **Step 1: Write the failing test**

```go
package audit_test

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/audit"
)

func TestBuildEntryAttributesOperator(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tenantID := uuid.New()

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("tenant_id", tenantID.String())
	c.Set("platform_operator_id", "op_7f3a")
	c.Set("platform_capability", "tenant.suspend")

	entry := audit.BuildEntryForTest(c, audit.Event{
		Action:       "tenant.suspended",
		ResourceType: "tenant",
	})

	require.NotNil(t, entry, "store-less platform event must produce a row")
	require.Equal(t, audit.ActorOperator, entry.ActorType)
	require.NotNil(t, entry.ActorOperatorID)
	require.Equal(t, "op_7f3a", *entry.ActorOperatorID)
	require.NotNil(t, entry.Capability)
	require.Equal(t, "tenant.suspend", *entry.Capability)
	require.Nil(t, entry.StoreID)
}

// An operator claim must not be mistaken for a mark8ly user.
func TestOperatorDoesNotPopulateActorUserID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("tenant_id", uuid.New().String())
	c.Set("platform_operator_id", uuid.New().String()) // operator id that parses as a UUID
	c.Set("platform_capability", "audit.read")

	entry := audit.BuildEntryForTest(c, audit.Event{Action: "x", ResourceType: "y"})

	require.NotNil(t, entry)
	require.Nil(t, entry.ActorUserID, "operator id must never land in actor_user_id")
	require.Equal(t, audit.ActorOperator, entry.ActorType)
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `cd services/marketplace-api && go test ./internal/audit/ -run 'TestBuildEntryAttributesOperator|TestOperatorDoesNotPopulateActorUserID' -v`
Expected: FAIL — `audit.BuildEntryForTest` undefined.

- [ ] **Step 3: Export a test seam**

Create `internal/audit/export_test_helpers.go` (a normal file, not `_test.go`, so the external `audit_test` package can reach it):

```go
package audit

import "github.com/gin-gonic/gin"

// BuildEntryForTest exposes buildEntry to the package's external tests.
// buildEntry stays unexported: it is an internal detail of Emit.
func BuildEntryForTest(c *gin.Context, ev Event) *Entry { return buildEntry(c, ev) }
```

- [ ] **Step 4: Implement operator attribution**

In `buildEntry`, replace the actor block. The operator branch runs **before** the user branch and returns early, so an operator claim can never fall through into `ActorUserID`:

```go
	if c != nil {
		operatorID := strings.TrimSpace(c.GetString("platform_operator_id"))
		capability := strings.TrimSpace(c.GetString("platform_capability"))

		switch {
		case operatorID != "":
			// A platform console operator. The id is opaque and belongs to
			// the console — it must never be written to actor_user_id, even
			// when it happens to parse as a UUID.
			entry.ActorType = ActorOperator
			entry.ActorOperatorID = &operatorID
			if capability != "" {
				entry.Capability = &capability
			}

		case ev.ForceActorType == "":
			// Don't infer a user actor when the caller explicitly forced a
			// system/api classification — storefront events should stay as
			// system even though the request carries a customer session.
			if uid := strings.TrimSpace(c.GetString("user_id")); uid != "" {
				if parsed, err := uuid.Parse(uid); err == nil {
					entry.ActorUserID = &parsed
					entry.ActorType = ActorUser
				}
			}
			if email := strings.TrimSpace(c.GetString("user_email")); email != "" {
				v := email
				entry.ActorEmail = &v
			}
		}

		if ip := clientIP(c); ip != "" {
			v := ip
			entry.IPAddress = &v
		}
		if ua := c.Request.UserAgent(); ua != "" {
			v := ua
			entry.UserAgent = &v
		}
	}
```

Also update the `entry := &Entry{...}` literal at `:191` so `StoreID` takes the pointer returned by `resolveScope`.

- [ ] **Step 5: Run the tests**

Run: `cd services/marketplace-api && go test ./internal/audit/... -count=1`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add services/marketplace-api/internal/audit/
git commit -m "feat(audit): attribute platform console operators on audit rows"
```

---

### Task 4: Signature canonicalisation

Pure functions, no HTTP, no database. This is the piece the console must reimplement, so its tests double as the published reference.

**Files:**
- Create: `services/marketplace-api/internal/handlers/platformadmin/signature.go`
- Test: `services/marketplace-api/internal/handlers/platformadmin/signature_test.go`
- Create: `services/marketplace-api/internal/handlers/platformadmin/cmd/genvectors/main.go`

**Interfaces:**
- Produces: `platformadmin.SignatureInput`, `CanonicalQuery(string) (string, error)`, `CanonicalString(SignatureInput) (string, error)`, `Sign(secret string, in SignatureInput) (string, error)`, `Verify(secret, got string, in SignatureInput) (bool, error)`. Task 6 consumes `Verify`.

- [ ] **Step 1: Write the failing test**

```go
package platformadmin_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
)

// The canonical string is where every design decision lives — field order,
// separator, how an absent body is hashed. Assert it byte-exactly. If the
// console disagrees with mark8ly, this is the artifact both sides compare.
func TestCanonicalStringIsExact(t *testing.T) {
	in := platformadmin.SignatureInput{
		Method:     "get",
		Path:       "/api/v1/admin/audit-logs",
		RawQuery:   "since_hours=720&limit=200",
		Body:       nil,
		Timestamp:  "1755859200",
		Nonce:      "018f3c2a-0000-7000-8000-000000000001",
		Operator:   "op_7f3a",
		Capability: "audit.read",
	}

	got, err := platformadmin.CanonicalString(in)
	require.NoError(t, err)

	// sha256 of the empty string, for the absent body.
	const emptyBodyHash = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

	want := "GET\n" +
		"/api/v1/admin/audit-logs\n" +
		"limit=200&since_hours=720\n" +
		emptyBodyHash + "\n" +
		"1755859200\n" +
		"018f3c2a-0000-7000-8000-000000000001\n" +
		"op_7f3a\n" +
		"audit.read"

	require.Equal(t, want, got)
}

func TestCanonicalQuerySortsKeysAndValues(t *testing.T) {
	got, err := platformadmin.CanonicalQuery("b=2&a=z&a=a")
	require.NoError(t, err)
	require.Equal(t, "a=a&a=z&b=2", got)
}

func TestCanonicalQueryEmpty(t *testing.T) {
	got, err := platformadmin.CanonicalQuery("")
	require.NoError(t, err)
	require.Equal(t, "", got)
}

func TestVerifyAcceptsOwnSignature(t *testing.T) {
	in := platformadmin.SignatureInput{
		Method: "POST", Path: "/api/v1/admin/tenants/t1/suspend",
		Body: []byte(`{"reason_code":"fraud"}`),
		Timestamp: "1755859200", Nonce: "n1",
		Operator: "op_7f3a", Capability: "tenant.suspend",
	}

	sig, err := platformadmin.Sign("shhh", in)
	require.NoError(t, err)

	ok, err := platformadmin.Verify("shhh", sig, in)
	require.NoError(t, err)
	require.True(t, ok)
}

// Each signed component must actually change the signature. A component that
// does not is a component an attacker can swap after signing.
func TestVerifyRejectsTampering(t *testing.T) {
	base := platformadmin.SignatureInput{
		Method: "POST", Path: "/api/v1/admin/tenants/t1/suspend",
		Body: []byte(`{"reason_code":"fraud"}`),
		Timestamp: "1755859200", Nonce: "n1",
		Operator: "op_7f3a", Capability: "tenant.suspend",
	}
	sig, err := platformadmin.Sign("shhh", base)
	require.NoError(t, err)

	tampered := map[string]func(*platformadmin.SignatureInput){
		"method":     func(i *platformadmin.SignatureInput) { i.Method = "GET" },
		"path":       func(i *platformadmin.SignatureInput) { i.Path = "/api/v1/admin/tenants/t2/suspend" },
		"query":      func(i *platformadmin.SignatureInput) { i.RawQuery = "force=true" },
		"body":       func(i *platformadmin.SignatureInput) { i.Body = []byte(`{"reason_code":"other"}`) },
		"timestamp":  func(i *platformadmin.SignatureInput) { i.Timestamp = "1755859999" },
		"nonce":      func(i *platformadmin.SignatureInput) { i.Nonce = "n2" },
		"operator":   func(i *platformadmin.SignatureInput) { i.Operator = "op_evil" },
		"capability": func(i *platformadmin.SignatureInput) { i.Capability = "tenant.purge" },
	}

	for name, mutate := range tampered {
		t.Run(name, func(t *testing.T) {
			in := base
			in.Body = append([]byte(nil), base.Body...)
			mutate(&in)

			ok, err := platformadmin.Verify("shhh", sig, in)
			require.NoError(t, err)
			require.False(t, ok, "%s must be covered by the signature", name)
		})
	}
}

func TestVerifyRejectsWrongSecret(t *testing.T) {
	in := platformadmin.SignatureInput{Method: "GET", Path: "/x", Timestamp: "1", Nonce: "n"}
	sig, err := platformadmin.Sign("right", in)
	require.NoError(t, err)

	ok, err := platformadmin.Verify("wrong", sig, in)
	require.NoError(t, err)
	require.False(t, ok)
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `cd services/marketplace-api && go test ./internal/handlers/platformadmin/ -v`
Expected: FAIL — package does not exist.

- [ ] **Step 3: Implement**

`internal/handlers/platformadmin/signature.go`:

```go
// Package platformadmin serves mark8ly's /admin/* surface to the Tesserix
// platform console (#274). It is deliberately separate from
// internal/handlers/admin: different auth chain, different response
// envelope, different audience. The two share the domain packages beneath
// them and nothing at the HTTP layer.
package platformadmin

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// Header names carried by every signed platform call.
const (
	HeaderOperator   = "X-Platform-Operator"
	HeaderCapability = "X-Platform-Capability"
	HeaderTimestamp  = "X-Platform-Timestamp"
	HeaderNonce      = "X-Platform-Nonce"
	HeaderSignature  = "X-Platform-Signature"
)

// SignatureInput is everything covered by the HMAC. Operator and capability
// are signed so neither can be substituted after signing — they are the
// attribution the whole surface exists to record.
type SignatureInput struct {
	Method     string
	Path       string
	RawQuery   string
	Body       []byte
	Timestamp  string
	Nonce      string
	Operator   string
	Capability string
}

// CanonicalQuery renders a query string deterministically: keys sorted, then
// values within a repeated key sorted, each percent-encoded, joined by "&".
// Both sides must agree byte-for-byte, so nothing here may depend on map
// iteration order.
func CanonicalQuery(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	values, err := url.ParseQuery(raw)
	if err != nil {
		return "", fmt.Errorf("platformadmin: parse query: %w", err)
	}

	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(values))
	for _, k := range keys {
		vs := append([]string(nil), values[k]...)
		sort.Strings(vs)
		for _, v := range vs {
			parts = append(parts, url.QueryEscape(k)+"="+url.QueryEscape(v))
		}
	}
	return strings.Join(parts, "&"), nil
}

// CanonicalString builds the string the HMAC covers. The body is included as
// a hash rather than inline so a captured signature cannot be lifted onto a
// different payload. An absent body hashes as the empty string.
func CanonicalString(in SignatureInput) (string, error) {
	query, err := CanonicalQuery(in.RawQuery)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(in.Body)

	return strings.Join([]string{
		strings.ToUpper(strings.TrimSpace(in.Method)),
		in.Path,
		query,
		hex.EncodeToString(sum[:]),
		in.Timestamp,
		in.Nonce,
		in.Operator,
		in.Capability,
	}, "\n"), nil
}

// Sign returns the hex HMAC-SHA256 of the canonical string.
func Sign(secret string, in SignatureInput) (string, error) {
	canonical, err := CanonicalString(in)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(canonical))
	return hex.EncodeToString(mac.Sum(nil)), nil
}

// Verify compares a presented signature against the expected one in constant
// time. A malformed query yields an error rather than a false negative, so
// the caller can distinguish "bad request" from "bad signature" in logs while
// still returning one opaque status to the client.
func Verify(secret, got string, in SignatureInput) (bool, error) {
	want, err := Sign(secret, in)
	if err != nil {
		return false, err
	}
	return hmac.Equal([]byte(got), []byte(want)), nil
}
```

- [ ] **Step 4: Run the tests**

Run: `cd services/marketplace-api && go test ./internal/handlers/platformadmin/ -v`
Expected: PASS, all cases.

- [ ] **Step 5: Add the vector generator**

`internal/handlers/platformadmin/cmd/genvectors/main.go` — emits the reference vectors published to the console on #275:

```go
// Command genvectors prints signature reference vectors as JSON. Run it and
// commit the output to testdata/vectors.json, then paste it on #275 so the
// console can verify its implementation against ours.
//
//	go run ./internal/handlers/platformadmin/cmd/genvectors > \
//	  internal/handlers/platformadmin/testdata/vectors.json
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
)

type vector struct {
	Name      string `json:"name"`
	Secret    string `json:"secret"`
	Method    string `json:"method"`
	Path      string `json:"path"`
	RawQuery  string `json:"raw_query"`
	Body      string `json:"body"`
	Timestamp string `json:"timestamp"`
	Nonce     string `json:"nonce"`
	Operator  string `json:"operator"`
	Capability string `json:"capability"`
	Canonical string `json:"canonical"`
	Signature string `json:"signature"`
}

func main() {
	inputs := []struct {
		name string
		in   platformadmin.SignatureInput
	}{
		{"get-with-query", platformadmin.SignatureInput{
			Method: "GET", Path: "/api/v1/admin/audit-logs",
			RawQuery: "since_hours=720&limit=200",
			Timestamp: "1755859200", Nonce: "018f3c2a-0000-7000-8000-000000000001",
			Operator: "op_7f3a", Capability: "audit.read",
		}},
		{"post-with-body", platformadmin.SignatureInput{
			Method: "POST", Path: "/api/v1/admin/tenants/t1/suspend",
			Body: []byte(`{"reason_code":"fraud"}`),
			Timestamp: "1755859200", Nonce: "018f3c2a-0000-7000-8000-000000000002",
			Operator: "op_7f3a", Capability: "tenant.suspend",
		}},
	}

	const secret = "reference-secret-do-not-use"
	out := make([]vector, 0, len(inputs))

	for _, item := range inputs {
		canonical, err := platformadmin.CanonicalString(item.in)
		if err != nil {
			fmt.Fprintln(os.Stderr, "canonical:", err)
			os.Exit(1)
		}
		sig, err := platformadmin.Sign(secret, item.in)
		if err != nil {
			fmt.Fprintln(os.Stderr, "sign:", err)
			os.Exit(1)
		}
		out = append(out, vector{
			Name: item.name, Secret: secret,
			Method: item.in.Method, Path: item.in.Path, RawQuery: item.in.RawQuery,
			Body: string(item.in.Body), Timestamp: item.in.Timestamp, Nonce: item.in.Nonce,
			Operator: item.in.Operator, Capability: item.in.Capability,
			Canonical: canonical, Signature: sig,
		})
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		fmt.Fprintln(os.Stderr, "encode:", err)
		os.Exit(1)
	}
}
```

- [ ] **Step 6: Generate and commit the vectors**

Run:
```bash
cd services/marketplace-api
mkdir -p internal/handlers/platformadmin/testdata
go run ./internal/handlers/platformadmin/cmd/genvectors > internal/handlers/platformadmin/testdata/vectors.json
cat internal/handlers/platformadmin/testdata/vectors.json
```
Expected: two vectors with populated `canonical` and 64-char hex `signature`.

- [ ] **Step 7: Commit**

```bash
git add services/marketplace-api/internal/handlers/platformadmin/
git commit -m "feat(platformadmin): HMAC signature canonicalisation and reference vectors"
```

---

### Task 5: Nonce store

**Files:**
- Create: `services/marketplace-api/internal/handlers/platformadmin/nonce.go`
- Test: `services/marketplace-api/internal/handlers/platformadmin/nonce_test.go`

**Interfaces:**
- Consumes: the `platform_request_nonces` table from Task 2.
- Produces: `platformadmin.NonceStore` interface with `Claim(ctx context.Context, nonce string, expiresAt time.Time) (bool, error)`, and `NewNonceStore(db *gorm.DB) NonceStore`. Task 6 consumes both.

- [ ] **Step 1: Write the failing test**

```go
package platformadmin_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
)

func TestClaimAcceptsFirstUseAndRejectsReplay(t *testing.T) {
	db := newTestDB(t)
	store := platformadmin.NewNonceStore(db)
	ctx := context.Background()
	nonce := uuid.NewString()
	expires := time.Now().Add(5 * time.Minute)

	first, err := store.Claim(ctx, nonce, expires)
	require.NoError(t, err)
	require.True(t, first, "first use must be accepted")

	second, err := store.Claim(ctx, nonce, expires)
	require.NoError(t, err)
	require.False(t, second, "replayed nonce must be rejected")
}

func TestClaimRejectsMalformedNonce(t *testing.T) {
	store := platformadmin.NewNonceStore(newTestDB(t))

	ok, err := store.Claim(context.Background(), "not-a-uuid", time.Now().Add(time.Minute))
	require.Error(t, err)
	require.False(t, ok)
}

// The unique constraint, not application logic, is what makes this safe
// across the 0-5 Knative replicas. Two concurrent claims must yield exactly
// one winner.
func TestClaimIsSafeUnderConcurrency(t *testing.T) {
	db := newTestDB(t)
	store := platformadmin.NewNonceStore(db)
	nonce := uuid.NewString()
	expires := time.Now().Add(5 * time.Minute)

	results := make(chan bool, 2)
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			ok, err := store.Claim(context.Background(), nonce, expires)
			errs <- err
			results <- ok
		}()
	}

	won := 0
	for i := 0; i < 2; i++ {
		require.NoError(t, <-errs)
		if <-results {
			won++
		}
	}
	require.Equal(t, 1, won, "exactly one claim must win")
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `cd services/marketplace-api && go test ./internal/handlers/platformadmin/ -run TestClaim -v`
Expected: FAIL — `NewNonceStore` undefined.

- [ ] **Step 3: Implement**

`internal/handlers/platformadmin/nonce.go`:

```go
package platformadmin

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Nonce is a single-use marker for a signed platform request.
type Nonce struct {
	Nonce     uuid.UUID `gorm:"column:nonce;type:uuid;primaryKey"`
	SeenAt    time.Time `gorm:"column:seen_at;not null;default:now()"`
	ExpiresAt time.Time `gorm:"column:expires_at;not null"`
}

// TableName pins the table so GORM's pluralizer can't drift.
func (Nonce) TableName() string { return "platform_request_nonces" }

// NonceStore records nonces so a captured request cannot be replayed inside
// its validity window.
type NonceStore interface {
	// Claim records the nonce and reports whether this was its first use.
	// False means replay. An error means the check could not be performed —
	// callers must treat that as a rejection, never as a pass.
	Claim(ctx context.Context, nonce string, expiresAt time.Time) (bool, error)
}

type gormNonceStore struct{ db *gorm.DB }

// NewNonceStore constructs a Postgres-backed NonceStore. The database is the
// only shared state on this path: mark8ly runs on Knative at 0-5 replicas, so
// an in-memory cache would let a replay routed to another pod through.
func NewNonceStore(db *gorm.DB) NonceStore { return &gormNonceStore{db: db} }

func (s *gormNonceStore) Claim(ctx context.Context, nonce string, expiresAt time.Time) (bool, error) {
	parsed, err := uuid.Parse(nonce)
	if err != nil {
		return false, fmt.Errorf("platformadmin: nonce must be a uuid: %w", err)
	}

	// ON CONFLICT DO NOTHING makes the unique constraint itself the replay
	// check — no read-then-write race to lose.
	res := s.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&Nonce{Nonce: parsed, ExpiresAt: expiresAt})

	if res.Error != nil {
		return false, fmt.Errorf("platformadmin: claim nonce: %w", res.Error)
	}
	return res.RowsAffected == 1, nil
}
```

- [ ] **Step 4: Run the tests**

Run: `cd services/marketplace-api && go test ./internal/handlers/platformadmin/ -run TestClaim -v -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/handlers/platformadmin/nonce.go \
        services/marketplace-api/internal/handlers/platformadmin/nonce_test.go
git commit -m "feat(platformadmin): postgres-backed nonce store for replay defence"
```

---

### Task 6: The enforcement middleware

**Files:**
- Create: `services/marketplace-api/internal/handlers/platformadmin/middleware.go`
- Test: `services/marketplace-api/internal/handlers/platformadmin/middleware_test.go`

**Interfaces:**
- Consumes: `Verify` (Task 4), `NonceStore` (Task 5).
- Produces: `platformadmin.AuthConfig`, `RequirePlatformAuth(AuthConfig) gin.HandlerFunc`, and the context keys `platform_operator_id` / `platform_capability` that Task 3's `buildEntry` reads. Task 9 constructs `AuthConfig`.

- [ ] **Step 1: Write the failing tests — one per matrix cell**

```go
package platformadmin_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
)

const testSecret = "test-platform-secret"

var fixedNow = time.Unix(1755859200, 0).UTC()

// memNonces is an in-memory NonceStore. Fine for middleware tests, which are
// about the enforcement matrix; Task 5 covers the real cross-replica store.
type memNonces struct{ seen map[string]bool }

func newMemNonces() *memNonces { return &memNonces{seen: map[string]bool{}} }

func (m *memNonces) Claim(_ context.Context, nonce string, _ time.Time) (bool, error) {
	if m.seen[nonce] {
		return false, nil
	}
	m.seen[nonce] = true
	return true, nil
}

func newRouter(t *testing.T, secret string, nonces platformadmin.NonceStore) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(platformadmin.RequirePlatformAuth(platformadmin.AuthConfig{
		Secret:     secret,
		NonceStore: nonces,
		Now:        func() time.Time { return fixedNow },
	}))
	r.GET("/admin/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"operator":   c.GetString("platform_operator_id"),
			"capability": c.GetString("platform_capability"),
		})
	})
	r.POST("/admin/ping", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	return r
}

type reqOpt func(*platformadmin.SignatureInput)

func withoutOperator(in *platformadmin.SignatureInput)   { in.Operator = "" }
func withoutCapability(in *platformadmin.SignatureInput) { in.Capability = "" }

func signedRequest(t *testing.T, method, target string, body []byte, opts ...reqOpt) *http.Request {
	t.Helper()

	in := platformadmin.SignatureInput{
		Method:     method,
		Path:       target,
		Body:       body,
		Timestamp:  "1755859200",
		Nonce:      uuid.NewString(),
		Operator:   "op_7f3a",
		Capability: "audit.read",
	}
	for _, o := range opts {
		o(&in)
	}

	sig, err := platformadmin.Sign(testSecret, in)
	require.NoError(t, err)

	var rdr *bytes.Reader
	if body == nil {
		rdr = bytes.NewReader(nil)
	} else {
		rdr = bytes.NewReader(body)
	}
	req := httptest.NewRequest(method, target, rdr)
	req.Header.Set(platformadmin.HeaderTimestamp, in.Timestamp)
	req.Header.Set(platformadmin.HeaderNonce, in.Nonce)
	req.Header.Set(platformadmin.HeaderSignature, sig)
	if in.Operator != "" {
		req.Header.Set(platformadmin.HeaderOperator, in.Operator)
	}
	if in.Capability != "" {
		req.Header.Set(platformadmin.HeaderCapability, in.Capability)
	}
	return req
}

func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error string `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	return body.Error
}

// Cell: secret unset -> 503 on every path. This surface fails CLOSED, unlike
// internalsvc.RequireInternalAuth which no-ops on an empty secret. An
// unconfigured deploy must be inert, not open.
func TestUnconfiguredSecretFailsClosed(t *testing.T) {
	r := newRouter(t, "", newMemNonces())

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/ping", nil))

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.Equal(t, "not_configured", errorCode(t, rec))
}

// Cell: valid signature on a read -> allowed, context populated.
func TestValidReadIsAllowed(t *testing.T) {
	r := newRouter(t, testSecret, newMemNonces())

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, signedRequest(t, http.MethodGet, "/admin/ping", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "op_7f3a")
	require.Contains(t, rec.Body.String(), "audit.read")
}

// Cell: read without operator identity -> permitted (#275 acceptance).
func TestReadWithoutOperatorIsPermitted(t *testing.T) {
	r := newRouter(t, testSecret, newMemNonces())

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, signedRequest(t, http.MethodGet, "/admin/ping", nil, withoutOperator, withoutCapability))

	require.Equal(t, http.StatusOK, rec.Code)
}

// Cell: write without operator identity -> refused.
func TestWriteWithoutOperatorIsRefused(t *testing.T) {
	r := newRouter(t, testSecret, newMemNonces())
	body := []byte(`{"x":1}`)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, signedRequest(t, http.MethodPost, "/admin/ping", body, withoutOperator))

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.Equal(t, "operator_required", errorCode(t, rec))
}

// Cell: write without capability -> refused. Never inferred from the route.
func TestWriteWithoutCapabilityIsRefused(t *testing.T) {
	r := newRouter(t, testSecret, newMemNonces())
	body := []byte(`{"x":1}`)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, signedRequest(t, http.MethodPost, "/admin/ping", body, withoutCapability))

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.Equal(t, "capability_required", errorCode(t, rec))
}

func TestMissingSignatureIsRefused(t *testing.T) {
	r := newRouter(t, testSecret, newMemNonces())

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/ping", nil))

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.Equal(t, "unauthenticated", errorCode(t, rec))
}

// Signature, timestamp and nonce failures share one code deliberately —
// distinguishing them tells an attacker which half of the check they passed.
func TestStaleTimestampIsRefusedWithOpaqueCode(t *testing.T) {
	r := newRouter(t, testSecret, newMemNonces())

	req := signedRequest(t, http.MethodGet, "/admin/ping", nil)
	// Re-sign with a timestamp well outside the +/-300s window.
	in := platformadmin.SignatureInput{
		Method: http.MethodGet, Path: "/admin/ping",
		Timestamp: "1755000000", Nonce: uuid.NewString(),
		Operator: "op_7f3a", Capability: "audit.read",
	}
	sig, err := platformadmin.Sign(testSecret, in)
	require.NoError(t, err)
	req.Header.Set(platformadmin.HeaderTimestamp, in.Timestamp)
	req.Header.Set(platformadmin.HeaderNonce, in.Nonce)
	req.Header.Set(platformadmin.HeaderSignature, sig)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.Equal(t, "unauthenticated", errorCode(t, rec))
}

func TestReplayedRequestIsRefused(t *testing.T) {
	r := newRouter(t, testSecret, newMemNonces())
	req := signedRequest(t, http.MethodGet, "/admin/ping", nil)

	first := httptest.NewRecorder()
	r.ServeHTTP(first, req.Clone(context.Background()))
	require.Equal(t, http.StatusOK, first.Code)

	second := httptest.NewRecorder()
	r.ServeHTTP(second, req.Clone(context.Background()))
	require.Equal(t, http.StatusUnauthorized, second.Code)
	require.Equal(t, "unauthenticated", errorCode(t, second))
}

// The handler must still be able to read the body after the middleware has
// hashed it.
func TestBodyIsReadableDownstream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(platformadmin.RequirePlatformAuth(platformadmin.AuthConfig{
		Secret: testSecret, NonceStore: newMemNonces(),
		Now: func() time.Time { return fixedNow },
	}))
	r.POST("/admin/echo", func(c *gin.Context) {
		var payload map[string]any
		require.NoError(t, c.ShouldBindJSON(&payload))
		c.JSON(http.StatusOK, payload)
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, signedRequest(t, http.MethodPost, "/admin/echo", []byte(`{"hello":"world"}`)))

	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"hello":"world"}`, rec.Body.String())
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd services/marketplace-api && go test ./internal/handlers/platformadmin/ -run 'Test(Unconfigured|Valid|Read|Write|Missing|Stale|Replayed|Body)' -v`
Expected: FAIL — `RequirePlatformAuth` undefined.

- [ ] **Step 3: Implement**

`internal/handlers/platformadmin/middleware.go`:

```go
package platformadmin

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// Context keys set by RequirePlatformAuth and read by audit.buildEntry.
const (
	CtxOperatorID = "platform_operator_id"
	CtxCapability = "platform_capability"
)

// defaultWindow bounds how far a request's timestamp may be from ours.
const defaultWindow = 5 * time.Minute

// maxBodyBytes caps what we will buffer to hash. The platform API reads our
// responses through a 1 MiB limit; matching it on the request side keeps a
// hostile or buggy caller from making us allocate without bound.
const maxBodyBytes = 1 << 20

// AuthConfig configures RequirePlatformAuth.
type AuthConfig struct {
	// Secret is the shared HMAC key. Empty means NOT CONFIGURED, and every
	// request is refused with 503 — this surface fails closed.
	Secret string
	// NonceStore records nonces for replay defence. Required when Secret is set.
	NonceStore NonceStore
	// Now is injectable for tests. Defaults to time.Now.
	Now func() time.Time
	// Window overrides the +/- timestamp tolerance. Defaults to 5 minutes.
	Window time.Duration
	// Logger receives rejection detail. Optional.
	Logger *slog.Logger
}

// RequirePlatformAuth verifies the gateway signature, enforces the replay
// window, and extracts the acting operator and the capability being
// exercised (#275).
//
// Deliberately NOT modelled on internalsvc.RequireInternalAuth, which no-ops
// when its secret is empty. That permissive branch is right for its purpose;
// on a surface serving cross-tenant tenant, billing and audit data it would
// mean an unconfigured deploy is wide open.
func RequirePlatformAuth(cfg AuthConfig) gin.HandlerFunc {
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	window := cfg.Window
	if window <= 0 {
		window = defaultWindow
	}

	return func(c *gin.Context) {
		if cfg.Secret == "" || cfg.NonceStore == nil {
			abort(c, http.StatusServiceUnavailable, "not_configured",
				"platform admin surface is not configured")
			return
		}

		body, err := readAndRestoreBody(c)
		if err != nil {
			abort(c, http.StatusBadRequest, "invalid_request", "request body could not be read")
			return
		}

		in := SignatureInput{
			Method:     c.Request.Method,
			Path:       c.Request.URL.Path,
			RawQuery:   c.Request.URL.RawQuery,
			Body:       body,
			Timestamp:  c.GetHeader(HeaderTimestamp),
			Nonce:      c.GetHeader(HeaderNonce),
			Operator:   c.GetHeader(HeaderOperator),
			Capability: c.GetHeader(HeaderCapability),
		}

		// Signature, timestamp and nonce failures all return the same code.
		// Distinguishing them tells an attacker which half of the check they
		// passed; the detail goes to our logs instead.
		presented := c.GetHeader(HeaderSignature)
		if presented == "" || in.Timestamp == "" || in.Nonce == "" {
			reject(c, cfg.Logger, "missing signature headers")
			return
		}
		if !withinWindow(in.Timestamp, now(), window) {
			reject(c, cfg.Logger, "timestamp outside window")
			return
		}

		ok, err := Verify(cfg.Secret, presented, in)
		if err != nil || !ok {
			reject(c, cfg.Logger, "signature mismatch")
			return
		}

		// Claim AFTER the signature verifies, so an unauthenticated caller
		// cannot burn nonces the real gateway might later use.
		fresh, err := cfg.NonceStore.Claim(c.Request.Context(), in.Nonce, now().Add(window))
		if err != nil || !fresh {
			reject(c, cfg.Logger, "nonce replayed or unverifiable")
			return
		}

		// Authority is asserted upstream by the console and the gateway.
		// Mark8ly records it and refuses its absence — it never infers one.
		if isWrite(c.Request.Method) {
			if in.Operator == "" {
				abort(c, http.StatusUnauthorized, "operator_required",
					"write requests must carry an operator identity")
				return
			}
			if in.Capability == "" {
				abort(c, http.StatusUnauthorized, "capability_required",
					"write requests must carry a capability")
				return
			}
		}

		if in.Operator != "" {
			c.Set(CtxOperatorID, in.Operator)
		}
		if in.Capability != "" {
			c.Set(CtxCapability, in.Capability)
		}
		c.Next()
	}
}

func isWrite(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func withinWindow(ts string, now time.Time, window time.Duration) bool {
	secs, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return false
	}
	delta := now.Sub(time.Unix(secs, 0))
	if delta < 0 {
		delta = -delta
	}
	return delta <= window
}

// readAndRestoreBody buffers the body so it can be hashed, then puts it back
// so the downstream handler can still bind it.
func readAndRestoreBody(c *gin.Context) ([]byte, error) {
	if c.Request.Body == nil {
		return nil, nil
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxBodyBytes))
	if err != nil {
		return nil, err
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	return body, nil
}

func reject(c *gin.Context, logger *slog.Logger, reason string) {
	if logger != nil {
		logger.Warn("platform admin auth rejected",
			"reason", reason,
			"path", c.Request.URL.Path,
			"method", c.Request.Method)
	}
	abort(c, http.StatusUnauthorized, "unauthenticated", "platform authentication failed")
}

func abort(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, gin.H{"error": code, "message": message})
}
```

- [ ] **Step 4: Run the tests**

Run: `cd services/marketplace-api && go test ./internal/handlers/platformadmin/ -v -count=1`
Expected: PASS, all cells.

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/handlers/platformadmin/middleware.go \
        services/marketplace-api/internal/handlers/platformadmin/middleware_test.go
git commit -m "feat(platformadmin): signed gateway auth with operator identity and replay defence"
```

---

### Task 7: Cross-store audit query

**Files:**
- Modify: `services/marketplace-api/internal/audit/repository.go`
- Test: `services/marketplace-api/internal/audit/platform_list_test.go` (create)

**Interfaces:**
- Produces: `audit.PlatformListFilter` and `Repository.ListPlatform(ctx, db, PlatformListFilter) (ListResult, error)`. Task 8 consumes both.

- [ ] **Step 1: Write the failing test**

```go
package audit_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/audit"
)

func TestListPlatformSpansStores(t *testing.T) {
	db := newTestDB(t)
	repo := audit.NewRepository()
	ctx := context.Background()

	tenantA, tenantB := uuid.New(), uuid.New()
	storeA, storeB := uuid.New(), uuid.New()

	for _, e := range []*audit.Entry{
		{TenantID: tenantA, StoreID: &storeA, ActorType: audit.ActorUser, Action: "product.deleted", ResourceType: "product", Status: audit.StatusSuccess, Severity: audit.SeverityInfo},
		{TenantID: tenantB, StoreID: &storeB, ActorType: audit.ActorUser, Action: "order.cancelled", ResourceType: "order", Status: audit.StatusSuccess, Severity: audit.SeverityInfo},
		{TenantID: tenantA, StoreID: nil, ActorType: audit.ActorOperator, Action: "tenant.suspended", ResourceType: "tenant", Status: audit.StatusSuccess, Severity: audit.SeverityWarning},
	} {
		require.NoError(t, repo.Create(ctx, db, e))
	}

	got, err := repo.ListPlatform(ctx, db, audit.PlatformListFilter{Limit: 50})
	require.NoError(t, err)
	require.GreaterOrEqual(t, got.Total, int64(3))
	require.GreaterOrEqual(t, len(got.Entries), 3, "rows must span stores and include store-less rows")
}

func TestListPlatformNarrowsByStore(t *testing.T) {
	db := newTestDB(t)
	repo := audit.NewRepository()
	ctx := context.Background()

	tenant, storeA, storeB := uuid.New(), uuid.New(), uuid.New()
	require.NoError(t, repo.Create(ctx, db, &audit.Entry{TenantID: tenant, StoreID: &storeA, ActorType: audit.ActorUser, Action: "a", ResourceType: "x", Status: audit.StatusSuccess, Severity: audit.SeverityInfo}))
	require.NoError(t, repo.Create(ctx, db, &audit.Entry{TenantID: tenant, StoreID: &storeB, ActorType: audit.ActorUser, Action: "b", ResourceType: "x", Status: audit.StatusSuccess, Severity: audit.SeverityInfo}))

	got, err := repo.ListPlatform(ctx, db, audit.PlatformListFilter{StoreID: storeA, Limit: 50})
	require.NoError(t, err)
	require.Equal(t, int64(1), got.Total)
	require.Equal(t, "a", got.Entries[0].Action)
}

func TestListPlatformClampsLimit(t *testing.T) {
	db := newTestDB(t)
	repo := audit.NewRepository()

	// An oversized limit must clamp, never error — the console is entitled to
	// ask for too much, and a ceiling is our backstop.
	got, err := repo.ListPlatform(context.Background(), db, audit.PlatformListFilter{Limit: 100000})
	require.NoError(t, err)
	require.LessOrEqual(t, len(got.Entries), 500)
}

func TestListPlatformFiltersBySince(t *testing.T) {
	db := newTestDB(t)
	repo := audit.NewRepository()
	ctx := context.Background()
	tenant, store := uuid.New(), uuid.New()

	require.NoError(t, repo.Create(ctx, db, &audit.Entry{TenantID: tenant, StoreID: &store, ActorType: audit.ActorUser, Action: "recent", ResourceType: "x", Status: audit.StatusSuccess, Severity: audit.SeverityInfo}))

	got, err := repo.ListPlatform(ctx, db, audit.PlatformListFilter{
		DateFrom: time.Now().Add(-1 * time.Hour),
		Limit:    50,
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, got.Total, int64(1))

	none, err := repo.ListPlatform(ctx, db, audit.PlatformListFilter{
		DateFrom: time.Now().Add(1 * time.Hour),
		Limit:    50,
	})
	require.NoError(t, err)
	require.Equal(t, int64(0), none.Total)
	require.Empty(t, none.Entries)
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd services/marketplace-api && go test ./internal/audit/ -run TestListPlatform -v`
Expected: FAIL — `PlatformListFilter` undefined.

- [ ] **Step 3: Implement**

Append to `internal/audit/repository.go`:

```go
// MaxPlatformPageSize caps a platform page. Set by the platform API's 1 MiB
// response read limit, past which a body truncates mid-JSON and surfaces to
// operators as "invalid response" rather than "too large".
const MaxPlatformPageSize = 500

// DefaultPlatformPageSize applies when the caller sends no limit. The contract
// says a missing parameter takes our default and is never an error.
const DefaultPlatformPageSize = 50

// PlatformListFilter narrows a cross-store audit query for the platform
// console (#276). Unlike ListFilter, every field is optional: this is the
// platform's estate-wide view, and StoreID is a narrowing filter rather than
// a required scope.
type PlatformListFilter struct {
	TenantID     uuid.UUID
	StoreID      uuid.UUID
	Actor        string    // partial match on actor_email or actor_operator_id
	Action       string    // exact match
	ResourceType string    // exact match
	DateFrom     time.Time // inclusive lower bound on created_at
	DateTo       time.Time // inclusive upper bound on created_at
	Page         int       // 1-based; defaults to 1
	Limit        int       // defaults to 50, clamped to 500
}

func applyPlatformScope(q *gorm.DB, f PlatformListFilter) *gorm.DB {
	if f.TenantID != uuid.Nil {
		q = q.Where("tenant_id = ?", f.TenantID)
	}
	if f.StoreID != uuid.Nil {
		q = q.Where("store_id = ?", f.StoreID)
	}
	if f.Actor != "" {
		like := "%" + f.Actor + "%"
		q = q.Where("COALESCE(actor_email,'') ILIKE ? OR COALESCE(actor_operator_id,'') ILIKE ?", like, like)
	}
	if f.Action != "" {
		q = q.Where("action = ?", f.Action)
	}
	if f.ResourceType != "" {
		q = q.Where("resource_type = ?", f.ResourceType)
	}
	if !f.DateFrom.IsZero() {
		q = q.Where("created_at >= ?", f.DateFrom)
	}
	if !f.DateTo.IsZero() {
		q = q.Where("created_at <= ?", f.DateTo)
	}
	return q
}

func (gormRepository) ListPlatform(ctx context.Context, db *gorm.DB, f PlatformListFilter) (ListResult, error) {
	var result ListResult
	q := applyPlatformScope(db.WithContext(ctx).Model(&Entry{}), f)

	if err := q.Count(&result.Total).Error; err != nil {
		return result, fmt.Errorf("audit platform list count: %w", err)
	}

	page := max(f.Page, 1)
	limit := f.Limit
	switch {
	case limit <= 0:
		limit = DefaultPlatformPageSize
	case limit > MaxPlatformPageSize:
		limit = MaxPlatformPageSize
	}
	offset := (page - 1) * limit

	result.Entries = make([]Entry, 0, limit)
	if err := q.Order("created_at DESC").Offset(offset).Limit(limit).Find(&result.Entries).Error; err != nil {
		return result, fmt.Errorf("audit platform list: %w", err)
	}
	return result, nil
}
```

Add to the `Repository` interface:

```go
	// ListPlatform returns a page of entries across every store, for the
	// platform console. StoreID in the filter narrows rather than scopes.
	ListPlatform(ctx context.Context, db *gorm.DB, f PlatformListFilter) (ListResult, error)
```

- [ ] **Step 4: Run the tests**

Run: `cd services/marketplace-api && go test ./internal/audit/... -count=1`
Expected: PASS. If any fake `Repository` implementations in other packages now fail to compile, add `ListPlatform` to them returning `ListResult{}, nil`.

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/audit/
git commit -m "feat(audit): cross-store platform list query"
```

---

### Task 8: `GET /admin/audit-logs`

**Files:**
- Create: `services/marketplace-api/internal/handlers/platformadmin/audit_logs.go`
- Create: `services/marketplace-api/internal/handlers/platformadmin/testdata/audit_logs_response.json`
- Test: `services/marketplace-api/internal/handlers/platformadmin/audit_logs_test.go`

**Interfaces:**
- Consumes: `audit.PlatformListFilter`, `Repository.ListPlatform` (Task 7).
- Produces: `platformadmin.NewAuditLogsHandler(db *gorm.DB, repo audit.Repository, logger *slog.Logger) *AuditLogsHandler` with method `List(c *gin.Context)`. Task 9 consumes it.

- [ ] **Step 1: Write the golden fixture**

`internal/handlers/platformadmin/testdata/audit_logs_response.json` — this file **is** the contract pinned on #276:

```json
{
  "data": [
    {
      "id": "3f2504e0-4f89-11d3-9a0c-0305e82c3301",
      "actor": "merchant@example.com",
      "action": "product.deleted",
      "timestamp": "2026-08-22T10:00:00Z",
      "target": "prod_123"
    },
    {
      "id": "3f2504e0-4f89-11d3-9a0c-0305e82c3302",
      "actor": "op_7f3a",
      "action": "tenant.suspended",
      "timestamp": "2026-08-22T09:00:00Z",
      "target": "tenant"
    }
  ],
  "pagination": {
    "page": 1,
    "limit": 200,
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
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/audit"
	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
)

// THE test. The #276 near-miss happened because the Go tests never marshalled
// against the console's parser and the console tests mocked the response —
// both sides green, both wrong. This compares real handler output to the
// pinned contract as bytes.
func TestAuditLogsMatchesPinnedContract(t *testing.T) {
	gin.SetMode(gin.TestMode)

	storeID := uuid.New()
	operator := "op_7f3a"
	email := "merchant@example.com"
	prodID := "prod_123"

	repo := &stubRepo{result: audit.ListResult{
		Total: 2,
		Entries: []audit.Entry{
			{
				ID:           uuid.MustParse("3f2504e0-4f89-11d3-9a0c-0305e82c3301"),
				TenantID:     uuid.New(),
				StoreID:      &storeID,
				ActorEmail:   &email,
				ActorType:    audit.ActorUser,
				Action:       "product.deleted",
				ResourceType: "product",
				ResourceID:   &prodID,
				CreatedAt:    time.Date(2026, 8, 22, 10, 0, 0, 0, time.UTC),
			},
			{
				ID:              uuid.MustParse("3f2504e0-4f89-11d3-9a0c-0305e82c3302"),
				TenantID:        uuid.New(),
				StoreID:         nil,
				ActorOperatorID: &operator,
				ActorType:       audit.ActorOperator,
				Action:          "tenant.suspended",
				ResourceType:    "tenant",
				CreatedAt:       time.Date(2026, 8, 22, 9, 0, 0, 0, time.UTC),
			},
		},
	}}

	r := gin.New()
	platformadmin.NewAuditLogsHandler(nil, repo, nil).Register(r.Group(""))

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/audit-logs?limit=200&since_hours=720", nil))

	require.Equal(t, http.StatusOK, rec.Code)

	want, err := os.ReadFile("testdata/audit_logs_response.json")
	require.NoError(t, err)

	// JSONEq rather than byte equality: key order is not part of the contract,
	// but the exact set of keys and their values is.
	require.JSONEq(t, string(want), rec.Body.String())
}

// A nil Go slice marshals to {} in this codebase's shape, which defeats every
// caller's `?? []` and has already crashed a page in this estate precisely
// when it had no data.
func TestEmptyResultIsEmptyArrayNotNullOrObject(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &stubRepo{result: audit.ListResult{Total: 0, Entries: nil}}
	r := gin.New()
	platformadmin.NewAuditLogsHandler(nil, repo, nil).Register(r.Group(""))

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/audit-logs", nil))

	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Data       json.RawMessage `json:"data"`
		Pagination struct {
			Page  int   `json:"page"`
			Limit int   `json:"limit"`
			Total int64 `json:"total"`
		} `json:"pagination"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "[]", string(body.Data))
	require.Equal(t, int64(0), body.Pagination.Total)
}

// The envelope is "pagination", never "meta". "meta" belongs to the merchant
// surface — see internal/handlers/admin/audit_logs_envelope_test.go.
func TestEnvelopeUsesPaginationNotMeta(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	platformadmin.NewAuditLogsHandler(nil, &stubRepo{}, nil).Register(r.Group(""))

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/audit-logs", nil))

	var body map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Contains(t, body, "pagination")
	require.NotContains(t, body, "meta")
	require.NotContains(t, body, "source", "the platform API stamps source itself")
}

// Ids go out bare. The platform API namespaces every row as <slug>:<id> on
// arrival; prefixing here produces "mark8ly:mark8ly:9f2".
func TestIdsAreBare(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &stubRepo{result: audit.ListResult{Total: 1, Entries: []audit.Entry{{
		ID: uuid.MustParse("3f2504e0-4f89-11d3-9a0c-0305e82c3301"),
		ActorType: audit.ActorSystem, Action: "x", ResourceType: "y",
		CreatedAt: time.Now().UTC(),
	}}}}

	r := gin.New()
	platformadmin.NewAuditLogsHandler(nil, repo, nil).Register(r.Group(""))

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/audit-logs", nil))

	require.NotContains(t, rec.Body.String(), "mark8ly:")
	require.Contains(t, rec.Body.String(), "3f2504e0-4f89-11d3-9a0c-0305e82c3301")
}

func TestOversizedLimitIsClampedNotRefused(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &stubRepo{}
	r := gin.New()
	platformadmin.NewAuditLogsHandler(nil, repo, nil).Register(r.Group(""))

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/audit-logs?limit=100000", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, 500, repo.gotFilter.Limit, "limit must clamp to MaxPlatformPageSize")
}

// Both parameters are always sent by the console, but a missing one must fall
// back to our default rather than error.
func TestMissingParamsUseDefaults(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &stubRepo{}
	r := gin.New()
	platformadmin.NewAuditLogsHandler(nil, repo, nil).Register(r.Group(""))

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/audit-logs", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, audit.DefaultPlatformPageSize, repo.gotFilter.Limit)
}

func TestSinceHoursNarrowsWindow(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &stubRepo{}
	r := gin.New()
	platformadmin.NewAuditLogsHandler(nil, repo, nil).Register(r.Group(""))

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/audit-logs?since_hours=24", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.False(t, repo.gotFilter.DateFrom.IsZero())
	require.WithinDuration(t, time.Now().Add(-24*time.Hour), repo.gotFilter.DateFrom, time.Minute)
}

func TestStoreIDIsOptionalNarrowingFilter(t *testing.T) {
	gin.SetMode(gin.TestMode)

	storeID := uuid.New()
	repo := &stubRepo{}
	r := gin.New()
	platformadmin.NewAuditLogsHandler(nil, repo, nil).Register(r.Group(""))

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/audit-logs?store_id="+storeID.String(), nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, storeID, repo.gotFilter.StoreID)
}

func TestTimestampsAreRFC3339(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &stubRepo{result: audit.ListResult{Total: 1, Entries: []audit.Entry{{
		ID: uuid.New(), ActorType: audit.ActorSystem, Action: "x", ResourceType: "y",
		CreatedAt: time.Date(2026, 8, 22, 10, 0, 0, 0, time.UTC),
	}}}}

	r := gin.New()
	platformadmin.NewAuditLogsHandler(nil, repo, nil).Register(r.Group(""))

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/audit-logs", nil))

	var body struct {
		Data []struct {
			Timestamp string `json:"timestamp"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Data, 1)

	_, err := time.Parse(time.RFC3339, body.Data[0].Timestamp)
	require.NoError(t, err, "timestamps must be ISO 8601 with offset")
}
```

And the stub, in the same file:

```go
// stubRepo records the filter it was handed and returns a canned result, so
// the tests can assert on parsing without a database.
type stubRepo struct {
	result    audit.ListResult
	gotFilter audit.PlatformListFilter
}

func (s *stubRepo) ListPlatform(_ context.Context, _ *gorm.DB, f audit.PlatformListFilter) (audit.ListResult, error) {
	s.gotFilter = f
	if s.result.Entries == nil {
		s.result.Entries = []audit.Entry{}
	}
	return s.result, nil
}

func (s *stubRepo) List(context.Context, *gorm.DB, audit.ListFilter) (audit.ListResult, error) {
	return audit.ListResult{}, nil
}
func (s *stubRepo) Create(context.Context, *gorm.DB, *audit.Entry) error { return nil }
func (s *stubRepo) Stream(context.Context, *gorm.DB, audit.ListFilter, func(*audit.Entry) error) error {
	return nil
}
```

- [ ] **Step 3: Run to verify it fails**

Run: `cd services/marketplace-api && go test ./internal/handlers/platformadmin/ -run TestAuditLogs -v`
Expected: FAIL — `NewAuditLogsHandler` undefined.

- [ ] **Step 4: Implement**

`internal/handlers/platformadmin/audit_logs.go`:

```go
package platformadmin

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/audit"
)

// AuditLogsHandler serves GET /admin/audit-logs to the platform console.
//
// The row shape here is NOT audit.Response — that belongs to the merchant
// Settings page. This one follows the contract pinned on #276, which renames
// or drops most fields. Do not consolidate the two.
type AuditLogsHandler struct {
	db     *gorm.DB
	repo   audit.Repository
	logger *slog.Logger
}

// NewAuditLogsHandler constructs the handler. logger may be nil.
func NewAuditLogsHandler(db *gorm.DB, repo audit.Repository, logger *slog.Logger) *AuditLogsHandler {
	return &AuditLogsHandler{db: db, repo: repo, logger: logger}
}

// Register mounts the route on the supplied group.
func (h *AuditLogsHandler) Register(g *gin.RouterGroup) {
	g.GET("/admin/audit-logs", h.List)
}

// auditRow is the pinned wire shape. Fields we hold but the contract does not
// name — status, severity, ip_address, user_agent, actor_type — are
// deliberately absent. Adding fields unilaterally is what the contract exists
// to prevent.
//
// `metadata` is also absent pending the open question on #276: our column is
// jsonb, the contract's example shows a string. Omission is the one choice
// that cannot be wrong while it is unresolved.
type auditRow struct {
	ID        string `json:"id"`
	Actor     string `json:"actor"`
	Action    string `json:"action"`
	Timestamp string `json:"timestamp"`
	Target    string `json:"target,omitempty"`
}

type pagination struct {
	Page  int   `json:"page"`
	Limit int   `json:"limit"`
	Total int64 `json:"total"`
}

type listResponse struct {
	Data       []auditRow `json:"data"`
	Pagination pagination `json:"pagination"`
}

// List handles GET /admin/audit-logs.
func (h *AuditLogsHandler) List(c *gin.Context) {
	filter := h.parseFilter(c)

	result, err := h.repo.ListPlatform(c.Request.Context(), h.db, filter)
	if err != nil {
		if h.logger != nil {
			h.logger.Error("platform audit logs list", "err", err)
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal_error",
			"message": "could not read audit logs",
		})
		return
	}

	// Allocate before appending: a nil slice marshals to {}, which defeats a
	// caller's `?? []` and crashes their page precisely when there is no data.
	rows := make([]auditRow, 0, len(result.Entries))
	for _, e := range result.Entries {
		rows = append(rows, toRow(e))
	}

	c.JSON(http.StatusOK, listResponse{
		Data: rows,
		Pagination: pagination{
			Page:  max(filter.Page, 1),
			Limit: filter.Limit,
			Total: result.Total,
		},
	})
}

// toRow maps a stored entry to the pinned contract shape.
func toRow(e audit.Entry) auditRow {
	return auditRow{
		// Bare id. The platform API namespaces as <slug>:<id> on arrival;
		// prefixing here yields "mark8ly:mark8ly:9f2".
		ID:        e.ID.String(),
		Actor:     actorOf(e),
		Action:    e.Action,
		Timestamp: e.CreatedAt.UTC().Format(time.RFC3339),
		Target:    targetOf(e),
	}
}

// actorOf resolves the single "who did it" string the contract asks for.
// A merchant has an email; a platform operator has an opaque id; anything
// else was the system acting on its own.
func actorOf(e audit.Entry) string {
	if e.ActorEmail != nil && *e.ActorEmail != "" {
		return *e.ActorEmail
	}
	if e.ActorOperatorID != nil && *e.ActorOperatorID != "" {
		return *e.ActorOperatorID
	}
	return "system"
}

// targetOf collapses resource_type + resource_id into the contract's single
// `target`. The pinned example shows a bare id ("prod_123"), so the id wins
// when present and the type is the fallback for rows that have none.
func targetOf(e audit.Entry) string {
	if e.ResourceID != nil && strings.TrimSpace(*e.ResourceID) != "" {
		return *e.ResourceID
	}
	return e.ResourceType
}

// parseFilter never returns an error. The contract states a missing parameter
// takes our default, and an oversized limit clamps rather than refusing — a
// ceiling on our side is the backstop for a caller asking for too much.
func (h *AuditLogsHandler) parseFilter(c *gin.Context) audit.PlatformListFilter {
	f := audit.PlatformListFilter{
		Action:       strings.TrimSpace(c.Query("action")),
		Actor:        strings.TrimSpace(c.Query("actor")),
		ResourceType: strings.TrimSpace(c.Query("resource_type")),
		Page:         1,
		Limit:        audit.DefaultPlatformPageSize,
	}

	if v := strings.TrimSpace(c.Query("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			f.Limit = min(n, audit.MaxPlatformPageSize)
		}
	}
	if v := strings.TrimSpace(c.Query("page")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			f.Page = n
		}
	}
	if v := strings.TrimSpace(c.Query("since_hours")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			f.DateFrom = time.Now().Add(-time.Duration(n) * time.Hour)
		}
	}
	// Explicit from/to win over since_hours when both are supplied.
	if t, ok := parseTime(c.Query("from")); ok {
		f.DateFrom = t
	}
	if t, ok := parseTime(c.Query("to")); ok {
		f.DateTo = t
	}
	if v := strings.TrimSpace(c.Query("store_id")); v != "" {
		if id, err := uuid.Parse(v); err == nil {
			f.StoreID = id
		}
	}
	if v := strings.TrimSpace(c.Query("tenant_id")); v != "" {
		if id, err := uuid.Parse(v); err == nil {
			f.TenantID = id
		}
	}
	return f
}

func parseTime(v string) (time.Time, bool) {
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

- [ ] **Step 5: Run the tests**

Run: `cd services/marketplace-api && go test ./internal/handlers/platformadmin/ -v -count=1`
Expected: PASS, including `TestAuditLogsMatchesPinnedContract`.

- [ ] **Step 6: Commit**

```bash
git add services/marketplace-api/internal/handlers/platformadmin/
git commit -m "feat(platformadmin): GET /admin/audit-logs against the pinned contract"
```

---

### Task 9: Wire it up

Route registration, config, and both mount points. This is rollout step 2 — code merged, **route mounted but secret unset**, so the surface answers `503` until someone populates it.

**Files:**
- Create: `services/marketplace-api/internal/handlers/platformadmin/routes.go`
- Modify: `services/marketplace-api/pkg/config/config.go`
- Modify: `services/marketplace-api/cmd/marketplace-api/main.go:1892`, `:1969`
- Test: `services/marketplace-api/internal/handlers/platformadmin/routes_test.go` (create)

**Interfaces:**
- Consumes: `RequirePlatformAuth` (Task 6), `NewAuditLogsHandler` (Task 8), `NewNonceStore` (Task 5).
- Produces: `platformadmin.Register(g *gin.RouterGroup, deps Deps)`.

- [ ] **Step 1: Write the failing test**

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

// With no secret configured the surface must be inert — 503, not 200 and not
// 404. This is what makes rollout steps 2 and 3 safe to separate.
func TestRegisterMountsBehindAuthAndFailsClosed(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	platformadmin.Register(r.Group("/api/v1"), platformadmin.Deps{
		Repo:   &stubRepo{},
		Secret: "",
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/admin/audit-logs", nil))

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.Equal(t, "not_configured", errorCode(t, rec))
}

// A nil repo must leave the routes unmounted rather than panic at request
// time — matching the nil-safe pattern used for optional admin handlers.
func TestRegisterIsNilSafe(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	require.NotPanics(t, func() {
		platformadmin.Register(r.Group("/api/v1"), platformadmin.Deps{})
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/admin/audit-logs", nil))
	require.Equal(t, http.StatusNotFound, rec.Code)
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd services/marketplace-api && go test ./internal/handlers/platformadmin/ -run TestRegister -v`
Expected: FAIL — `platformadmin.Register` undefined.

- [ ] **Step 3: Implement `routes.go`**

```go
package platformadmin

import (
	"log/slog"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/audit"
)

// Deps groups everything the platform admin surface needs. Constructed in
// cmd/marketplace-api/main.go.
type Deps struct {
	DB     *gorm.DB
	Repo   audit.Repository
	Logger *slog.Logger

	// Secret is MARKETPLACE_PLATFORM_ADMIN_SECRET. Empty leaves the surface
	// mounted but inert — every request answers 503 not_configured. That is
	// what lets the binary ship before the secret exists.
	Secret string

	// NonceStore is optional; a Postgres-backed one is built from DB when nil.
	NonceStore NonceStore
}

// Register mounts the platform console's /admin/* surface behind
// RequirePlatformAuth. A nil Repo leaves everything unmounted, matching the
// nil-safe pattern used for optional handlers in internal/handlers/admin.
func Register(g *gin.RouterGroup, deps Deps) {
	if deps.Repo == nil {
		return
	}

	nonces := deps.NonceStore
	if nonces == nil && deps.DB != nil {
		nonces = NewNonceStore(deps.DB)
	}

	group := g.Group("", RequirePlatformAuth(AuthConfig{
		Secret:     deps.Secret,
		NonceStore: nonces,
		Logger:     deps.Logger,
	}))

	NewAuditLogsHandler(deps.DB, deps.Repo, deps.Logger).Register(group)
}
```

Note the test constructs `Deps{Repo: &stubRepo{}, Secret: ""}` with no DB, so `nonces` stays nil — and `RequirePlatformAuth` returns `503 not_configured` for a nil store as well as an empty secret. That is intentional: an unconfigured replay store is an unconfigured surface.

- [ ] **Step 4: Add the config field**

In `pkg/config/config.go`, beside `AuditIngestSecret`:

```go
	// PlatformAdminSecret is the HMAC key for the Tesserix platform console's
	// signed /admin/* calls (#275). Separate from InternalAuthSecret and
	// AuditIngestSecret: different caller, different blast radius.
	//
	// Unlike those, an empty value does NOT no-op the check — the platform
	// admin surface fails closed and answers 503 until this is populated.
	PlatformAdminSecret string `envconfig:"MARKETPLACE_PLATFORM_ADMIN_SECRET" default:""`
```

And in `Load()`, beside the other trims:

```go
	cfg.PlatformAdminSecret = strings.TrimSpace(cfg.PlatformAdminSecret)
```

- [ ] **Step 5: Mount on both engines**

In `cmd/marketplace-api/main.go`, after `admin.RegisterAdminMobile(r.Group("/api/v1"), mobileDeps)` at **line 1893** (the `mode.Both` branch):

```go
		platformadmin.Register(r.Group("/api/v1"), platformadmin.Deps{
			DB:     conn,
			Repo:   auditRepo,
			Logger: log,
			Secret: cfg.PlatformAdminSecret,
		})
```

And the identical block after `admin.RegisterAdminMobile(engine.Group("/api/v1"), mobileDeps)` at **line 1970** (the `mode.Admin` branch), using `engine.Group("/api/v1")`.

Add the import:

```go
	"github.com/mark8ly/marketplace-api/internal/handlers/platformadmin"
```

If the local variable holding the audit repository is not named `auditRepo`, find it with:

```bash
cd services/marketplace-api && grep -n "audit.NewRepository()" cmd/marketplace-api/main.go
```

- [ ] **Step 6: Build and run everything**

Run: `cd services/marketplace-api && go build ./... && go test ./... -count=1`
Expected: PASS.

- [ ] **Step 7: Check coverage on the new package**

Run: `cd services/marketplace-api && go test ./internal/handlers/platformadmin/ -coverprofile=/tmp/pa.out && go tool cover -func=/tmp/pa.out | tail -1`
Expected: 80%+ total. If short, the usual gap is error branches in `List` and `Claim` — add cases returning an error from the stub.

- [ ] **Step 8: Verify locally end-to-end**

Run: `make dev` then, in another shell, sign a request using the committed vectors as a template. With `MARKETPLACE_PLATFORM_ADMIN_SECRET` unset:

```bash
curl -i localhost:8080/api/v1/admin/audit-logs
```
Expected: `503` with `{"error":"not_configured", ...}`.

Set the secret in `infra/dev/.env.local`, restart, and repeat with signed headers. Expected: `200` and the pinned envelope.

- [ ] **Step 9: Commit**

```bash
git add services/marketplace-api/internal/handlers/platformadmin/routes.go \
        services/marketplace-api/internal/handlers/platformadmin/routes_test.go \
        services/marketplace-api/pkg/config/config.go \
        services/marketplace-api/cmd/marketplace-api/main.go
git commit -m "feat(platformadmin): mount the platform console admin surface"
```

---

## After the plan

1. **Populate the secret** in GCP Secret Manager as `marketplace-platform-admin-secret`, wire it through the ExternalSecret for marketplace-api, and hand the same value to the console.
2. **Verify against a demo tenant** in production.
3. **Report on the issues:**
   - #275 — the scheme is live; attach `testdata/vectors.json`.
   - #276 — the endpoint is live; note the base URL is `https://<host>/api/v1`.
   - #276 — chase the `metadata` object-vs-string question.
   - #284 — nothing yet, but the cross-store query pattern from Task 7 is the shape that endpoint will need.
4. **Next spec** covers the inbox (#280, #281) — the load-bearing endpoint of the series, and `seaqueue` already has its data layer.

## Open question carried from the spec

`metadata` object vs string (#276) is unresolved. The field is omitted, and Task 8's fixture omits it. When the console answers, the change is one field on `auditRow`, one line in `toRow`, and one key in `testdata/audit_logs_response.json`.
