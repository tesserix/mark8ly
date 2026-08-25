# Tenant Purge (#288) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give the platform console one governed, confirmed, audited, irreversible operation that destroys a tenant everywhere — and a non-destructive preview of exactly what it would destroy.

**Architecture:** `POST /api/v1/platform/admin/tenants/{id}/purge` on marketplace-api's `platformadmin` surface calls a new operator-initiated teardown on platform-api, which validates the operator's supplied store-slug set against the tenant's actual set **inside the same transaction** that deletes the tenant row and enqueues `tenant.deleted`. marketplace-api then runs `tenantpurge.Purge` inline for a real destruction report, with the existing outbox drainer as the durability backstop, and writes its audit row **synchronously after** the purge commits — because the purge deletes `audit_logs WHERE tenant_id = ?` and the async emitter would race it. `GET …/purge/preview` runs the same table enumeration with `SELECT count(*)` substituted for `DELETE`.

**Tech Stack:** Go 1.26, Gin, GORM, PostgreSQL 15, testify. Two services: `services/platform-api` and `services/marketplace-api`.

**Spec:** `docs/superpowers/specs/2026-08-25-tenant-purge-design.md`

## Global Constraints

- **No migration in either service.** `ExpectedSchemaVersion` does not move. If you find yourself writing SQL DDL, stop — the plan is wrong, not the schema.
- **Use the LAN IP, never `localhost`**: `postgres://dev:dev@192.168.1.110:5432/marketplace_db` and `postgres://dev:dev@192.168.1.110:5432/platform_api`. A native Postgres squats on 127.0.0.1. The committed Makefile says `localhost`; do not change it.
- **Integration tests run with `-p 1`.** The packages share one local Postgres and parallel runs exhaust its connection limit (`FATAL: sorry, too many clients already`), which presents as data pollution and is not.
- **Integration tests gate on `TEST_DATABASE_URL`**, never `TEST_DB_DSN`. Files using the latter skip silently (#317).
- **`go vet -tags=integration ./...` is the only command that compiles `//go:build integration` files.** It is part of every task's verification, not an optional extra.
- **`go test ./...` from the service root**, never path-scoped, or the schema-version guard in the root package silently does not run.
- Test package conventions differ: `platform-api` integration tests use the **internal** package (`package tenant`); `marketplace-api` uses **external** (`package foo_test`). Both use `//go:build integration`.
- Envelope on the platform surface is exactly `{"data": …}`; timestamps RFC3339 UTC with offset; ids **bare**; never send a `source` field.
- Allocate slices with `make([]T, 0, n)` — a nil slice marshals to `{}` and defeats the caller's `?? []`.
- **Never mount anything on the merchant `/admin/tenants/:tenantId` group.** Two different wildcard names at one path position panic gin at router build time. All new routes go on the `platformadmin` group, which already uses `:id` at that position.
- Commit messages: conventional, single line, no signature.
- **Pre-existing failures — do not fix, do not let them mask yours.** `internal/billing/trial/subscribe_integration_test.go`: 19 tests skip silently and all 19 fail when run (#317). `internal/subscription/planchange` integration: 9 FAIL. `internal/whitelabel` integration: nil-pointer panic. Each confirmed at `origin/main`.
- **A shell `&&` chain that aborts still prints a trailing `echo ok` on its own line.** Check exit codes when a result is going to be reported as evidence. Use `-count=1`; a cached `ok` is not a fresh run.

---

## File Structure

**`services/platform-api`**

| file | responsibility |
|---|---|
| `internal/tenant/repository.go` (modify) | add `StoreRef`, `TeardownSnapshot`, `SnapshotForTeardown` — reads a tenant's identifying state under `SELECT … FOR UPDATE` |
| `internal/account/purge.go` (create) | `Service.PurgeTenant` — confirmation check + teardown tx + best-effort cleanup |
| `internal/account/purge_test.go` (create) | unit tests with fakes, no DB |
| `internal/account/purge_integration_test.go` (create) | the transaction's real behaviour against Postgres |
| `internal/account/handler.go` (modify) | `RegisterOperator` + the operator route |
| `internal/account/handler_test.go` (modify) | handler-level status mapping |
| `cmd/server/main.go` (modify) | construct the service unconditionally, mount on `strictInternal` |

**`services/marketplace-api`**

| file | responsibility |
|---|---|
| `internal/tenantpurge/purge.go` (modify) | `Purge` returns a `Report`; `countPlan` sibling; corrected comment |
| `internal/tenantpurge/purge_test.go` (modify) | corrected rationale; plan-parity assertions |
| `internal/tenantpurge/schema_coverage_integration_test.go` (create) | fails when a tenant-scoped table is neither purged, cascaded, nor declared an exclusion |
| `internal/audit/emitter.go` (modify) | `EmitSync` — synchronous write reusing `buildEntry` |
| `internal/tenantlifecycle/client.go` (modify) | `Teardown` + `ErrConfirmationMismatch` carrying the actual set |
| `internal/handlers/platformadmin/tenant_purge.go` (create) | both handlers, reason codes, wire types |
| `internal/handlers/platformadmin/routes.go` (modify) | `Deps.TenantTeardown`, `Deps.Purger`, mount guard |
| `cmd/marketplace-api/main.go` (modify) | wiring, both existing `tenantpurge.Purge` call sites |

**Interfaces produced, in one place** — later tasks depend on these exact names:

```go
// platform-api  internal/tenant
type StoreRef struct{ ID, Slug string }
type TeardownSnapshot struct {
    TenantID, Name, OwnerUserID string
    Stores                      []StoreRef
}
func (r *gormRepository) SnapshotForTeardown(ctx context.Context, tx *gorm.DB, tenantID string) (*TeardownSnapshot, error)

// platform-api  internal/account
type MismatchError struct{ Expected []string }
type PurgeResult struct {
    TenantID, TenantName string
    StoreIDs, StoreSlugs []string
}
func (s *Service) PurgeTenant(ctx context.Context, tenantID string, suppliedSlugs []string) (*PurgeResult, error)

// marketplace-api  internal/tenantpurge
type TableResult struct{ Table string; RowsDeleted int64 }
type Report struct{ Tables []TableResult; TotalRows int64 }
func Purge(ctx context.Context, db *gorm.DB, tenantID string, storeIDs []string) (Report, error)
func Count(ctx context.Context, db *gorm.DB, tenantID string, storeIDs []string) (Report, error)

// marketplace-api  internal/audit
func (e *Emitter) EmitSync(c *gin.Context, ev Event) error

// marketplace-api  internal/tenantlifecycle
type ConfirmationMismatchError struct{ Expected []string }
type TeardownResult struct {
    TenantID   string   `json:"tenant_id"`
    TenantName string   `json:"tenant_name"`
    StoreIDs   []string `json:"store_ids"`
    StoreSlugs []string `json:"store_slugs"`
}
func (c *Client) Teardown(ctx context.Context, tenantID string, storeSlugs []string) (*TeardownResult, error)
```

---

## Task 1: `SnapshotForTeardown` — read the tenant's identifying state under lock

**Files:**
- Modify: `services/platform-api/internal/tenant/repository.go` (add to the `Repository` interface near `ListStoreIDs` at :40-45, implement beside `ListStoreIDs` at :190)
- Test: `services/platform-api/internal/tenant/repository_integration_test.go` (add to the existing file)

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces: `tenant.StoreRef`, `tenant.TeardownSnapshot`, `Repository.SnapshotForTeardown`.

Why a new method rather than composing `GetByID` + `ListStoreIDs`: both read outside the caller's transaction and neither returns slugs. The confirmation check is only worth anything if the values it compares are read in the same transaction that deletes the row.

- [ ] **Step 1: Write the failing integration test**

Append to `services/platform-api/internal/tenant/repository_integration_test.go` (internal package, per platform-api convention):

```go
func TestSnapshotForTeardown_ReturnsNameOwnerAndStoreSlugs(t *testing.T) {
	db := testDB(t)
	repo := NewRepository(db)

	tenantID := seedTenant(t, db, "The Bondi Store", "owner-uid-1")
	seedStore(t, db, tenantID, "the-bondi-store")
	seedStore(t, db, tenantID, "bondi-outlet")

	// A SECOND tenant with its own store. A snapshot that ignores its
	// tenant_id filter would pick this up, and a one-tenant fixture
	// could never tell the difference.
	otherID := seedTenant(t, db, "The Facade Factory", "owner-uid-2")
	seedStore(t, db, otherID, "the-facade-factory")

	var snap *TeardownSnapshot
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		var err error
		snap, err = repo.SnapshotForTeardown(t.Context(), tx, tenantID)
		return err
	}))

	require.Equal(t, tenantID, snap.TenantID)
	require.Equal(t, "The Bondi Store", snap.Name)
	require.Equal(t, "owner-uid-1", snap.OwnerUserID)

	slugs := make([]string, 0, len(snap.Stores))
	for _, s := range snap.Stores {
		require.NotEmpty(t, s.ID, "store id must be populated, not just the slug")
		slugs = append(slugs, s.Slug)
	}
	sort.Strings(slugs)
	require.Equal(t, []string{"bondi-outlet", "the-bondi-store"}, slugs)
}

func TestSnapshotForTeardown_UnknownTenantIsNotFound(t *testing.T) {
	db := testDB(t)
	repo := NewRepository(db)

	err := db.Transaction(func(tx *gorm.DB) error {
		_, err := repo.SnapshotForTeardown(t.Context(), tx, uuid.NewString())
		return err
	})

	ae, ok := apperrors.As(err)
	require.True(t, ok, "want an *apperrors.AppError, got %T", err)
	require.Equal(t, "tenant_not_found", ae.Code)
}
```

If `seedTenant`/`seedStore`/`testDB` do not already exist in that file with these signatures, adapt to whatever the file already uses rather than adding duplicates — read the file first. Reference-data values must come from platform-api's seeded set: `GB`/`GBP`/`Europe/London` are safe, `IE`/`Europe/Dublin` are **not** — `stores` FKs to `countries`, `currencies` and `timezones` in **this** service (it does not in marketplace-api).

- [ ] **Step 2: Run it and watch it fail**

```bash
cd services/platform-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/platform_api?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 -v ./internal/tenant/ -run TestSnapshotForTeardown 2>&1 | tail -30
```

Expected: compile failure, `snap.SnapshotForTeardown undefined`. Confirm from the **verbose** output that the tests RAN and did not `--- SKIP`; a skip and a pass are one character apart in a wall of output.

- [ ] **Step 3: Add the types and the interface method**

In `internal/tenant/repository.go`, beside the existing `ListStoreIDs` declaration in the `Repository` interface (:40-45):

```go
	// SnapshotForTeardown reads the identifying state an operator purge
	// confirms against, with SELECT ... FOR UPDATE on the tenant row.
	//
	// It exists rather than composing GetByID + ListStoreIDs because both
	// of those read OUTSIDE the caller's transaction, and a confirmation
	// check comparing values read outside the transaction that deletes the
	// row is the same stale read it exists to prevent. The FOR UPDATE also
	// serialises two concurrent purges of one tenant: the second blocks
	// until the first commits, then finds no row.
	//
	// Returns apperrors.NotFound("tenant_not_found") when the tenant does
	// not exist — including when a concurrent purge just removed it.
	SnapshotForTeardown(ctx context.Context, tx *gorm.DB, tenantID string) (*TeardownSnapshot, error)
```

Beside `SuspendResult` (:92):

```go
// StoreRef identifies one store under a tenant: the id an operator purge
// passes to marketplace-api, and the slug it confirms against.
type StoreRef struct {
	ID   string `gorm:"column:id"`
	Slug string `gorm:"column:slug"`
}

// TeardownSnapshot is a tenant's identifying state as of the moment the
// teardown transaction locked its row.
type TeardownSnapshot struct {
	TenantID    string
	Name        string
	OwnerUserID string
	Stores      []StoreRef
}
```

- [ ] **Step 4: Implement it**

Beside `ListStoreIDs` (:190):

```go
func (r *gormRepository) SnapshotForTeardown(ctx context.Context, tx *gorm.DB, tenantID string) (*TeardownSnapshot, error) {
	db := r.db
	if tx != nil {
		db = tx
	}

	var row struct {
		ID          string
		Name        string
		OwnerUserID string
	}
	res := db.WithContext(ctx).
		Raw(`SELECT id, name, owner_user_id FROM tenants WHERE id = ? FOR UPDATE`, tenantID).
		Scan(&row)
	if res.Error != nil {
		return nil, fmt.Errorf("tenant: snapshot for teardown: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return nil, apperrors.NotFound("tenant_not_found",
			fmt.Sprintf("tenant %q does not exist", tenantID))
	}

	stores := make([]StoreRef, 0, 4)
	if err := db.WithContext(ctx).
		Raw(`SELECT id, slug FROM stores WHERE tenant_id = ? ORDER BY slug`, tenantID).
		Scan(&stores).Error; err != nil {
		return nil, fmt.Errorf("tenant: snapshot store refs: %w", err)
	}

	return &TeardownSnapshot{
		TenantID:    row.ID,
		Name:        row.Name,
		OwnerUserID: row.OwnerUserID,
		Stores:      stores,
	}, nil
}
```

- [ ] **Step 5: Run the tests and the vet gate**

```bash
cd services/platform-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/platform_api?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 -v ./internal/tenant/ -run TestSnapshotForTeardown 2>&1 | tail -30
echo "exit=$?"
go vet -tags=integration ./...
echo "vet exit=$?"
```

Expected: both `--- PASS`, both exit 0. Read the exit codes, not the last line of output.

- [ ] **Step 6: Prove the test constrains the code**

Temporarily change the store query's `WHERE tenant_id = ?` to `WHERE 1 = 1` and re-run. `TestSnapshotForTeardown_ReturnsNameOwnerAndStoreSlugs` **must fail** — that is what the second tenant in the fixture is for. Revert.

- [ ] **Step 7: Commit**

```bash
git add services/platform-api/internal/tenant/repository.go services/platform-api/internal/tenant/repository_integration_test.go
git commit -m "feat(tenant): SnapshotForTeardown reads tenant identifying state under lock (#288)"
```

---

## Task 2: `Service.PurgeTenant` — confirmation-checked teardown

**Files:**
- Create: `services/platform-api/internal/account/purge.go`
- Test: `services/platform-api/internal/account/purge_test.go`
- Modify: `services/platform-api/internal/account/service.go` (extend the `TenantRepo` interface only)

**Interfaces:**
- Consumes: `tenant.TeardownSnapshot`, `tenant.StoreRef`, `Repository.SnapshotForTeardown` (Task 1).
- Produces: `account.MismatchError`, `account.PurgeResult`, `Service.PurgeTenant`.

`PurgeTenant` is deliberately **not** a branch inside `DeleteAccount`. `DeleteAccount` opens with `fga.GetRole(actorUID, tenantID)` and requires `RoleOwner`; a platform operator holds no FGA role, so there is no actor to check. Sharing the entry point would mean threading a "skip the authorization" flag through the one function whose job is authorization.

- [ ] **Step 1: Extend the `TenantRepo` interface**

In `internal/account/service.go`, add to `TenantRepo` (which currently declares `ListStoreIDs` and `DeleteInTx`):

```go
	// SnapshotForTeardown is used by the operator purge path to read the
	// tenant's identifying state under lock, inside the same transaction
	// that deletes it. See PurgeTenant.
	SnapshotForTeardown(ctx context.Context, tx *gorm.DB, tenantID string) (*tenant.TeardownSnapshot, error)
```

Add the `tenant` import. If this creates an import cycle, stop and report it rather than working around it — `account` already depends on `tenant`'s repository through the interface, so it should not.

- [ ] **Step 2: Write the failing unit tests**

Create `services/platform-api/internal/account/purge_test.go`:

```go
package account

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/platform-api/internal/tenant"
	apperrors "github.com/mark8ly/platform-api/pkg/errors"
)

type fakeTenantRepo struct {
	snap     *tenant.TeardownSnapshot
	snapErr  error
	deleted  []string
	deleteErr error
}

func (f *fakeTenantRepo) ListStoreIDs(_ context.Context, _ *gorm.DB, _ string) ([]string, error) {
	return nil, nil
}
func (f *fakeTenantRepo) DeleteInTx(_ context.Context, _ *gorm.DB, id string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.deleted = append(f.deleted, id)
	return nil
}
func (f *fakeTenantRepo) SnapshotForTeardown(_ context.Context, _ *gorm.DB, _ string) (*tenant.TeardownSnapshot, error) {
	return f.snap, f.snapErr
}

type recordingOutbox struct{ kinds []string }

func (r *recordingOutbox) enqueue(_ *gorm.DB, kind string, _ any) error {
	r.kinds = append(r.kinds, kind)
	return nil
}

func snapshotWith(slugs ...string) *tenant.TeardownSnapshot {
	refs := make([]tenant.StoreRef, 0, len(slugs))
	for i, s := range slugs {
		refs = append(refs, tenant.StoreRef{ID: "store-" + string(rune('a'+i)), Slug: s})
	}
	return &tenant.TeardownSnapshot{
		TenantID: "t-1", Name: "The Bondi Store", OwnerUserID: "uid-1", Stores: refs,
	}
}

func newTestService(repo TenantRepo, ob outboxEnqueuer) *Service {
	// db nil: teardownTenantTx runs without a real transaction wrapper.
	return NewService(nil, repo, nil, nil, ob, nil)
}

func TestPurgeTenant_MatchingSlugSetTearsDownAndEnqueues(t *testing.T) {
	repo := &fakeTenantRepo{snap: snapshotWith("the-bondi-store", "bondi-outlet")}
	ob := &recordingOutbox{}
	svc := newTestService(repo, ob.enqueue)

	// Deliberately supplied in a DIFFERENT order from the snapshot: the
	// comparison is a set comparison, not a sequence comparison.
	res, err := svc.PurgeTenant(t.Context(), "t-1", []string{"bondi-outlet", "the-bondi-store"})

	require.NoError(t, err)
	require.Equal(t, []string{"t-1"}, repo.deleted)
	require.Equal(t, []string{TenantDeletedOutboxKind}, ob.kinds)
	require.Equal(t, "The Bondi Store", res.TenantName)
	require.ElementsMatch(t, []string{"store-a", "store-b"}, res.StoreIDs)
	require.ElementsMatch(t, []string{"the-bondi-store", "bondi-outlet"}, res.StoreSlugs)
}

// The property under test discriminates between two tenants, so the
// fixture contains two. A check that always passes and a check that
// compares nothing are indistinguishable with one tenant's slugs.
func TestPurgeTenant_AnotherTenantsSlugsAreRefused(t *testing.T) {
	repo := &fakeTenantRepo{snap: snapshotWith("the-bondi-store")}
	ob := &recordingOutbox{}
	svc := newTestService(repo, ob.enqueue)

	_, err := svc.PurgeTenant(t.Context(), "t-1", []string{"the-facade-factory"})

	var me *MismatchError
	require.True(t, errors.As(err, &me), "want *MismatchError, got %T", err)
	require.Equal(t, []string{"the-bondi-store"}, me.Expected)
	require.Empty(t, repo.deleted, "nothing may be deleted on a mismatch")
	require.Empty(t, ob.kinds, "nothing may be enqueued on a mismatch")
}

func TestPurgeTenant_SubsetIsRefused(t *testing.T) {
	repo := &fakeTenantRepo{snap: snapshotWith("the-bondi-store", "bondi-outlet")}
	svc := newTestService(repo, (&recordingOutbox{}).enqueue)

	_, err := svc.PurgeTenant(t.Context(), "t-1", []string{"the-bondi-store"})

	var me *MismatchError
	require.True(t, errors.As(err, &me), "a supplied subset must be a mismatch, got %T", err)
	require.Empty(t, repo.deleted)
}

func TestPurgeTenant_SupersetIsRefused(t *testing.T) {
	repo := &fakeTenantRepo{snap: snapshotWith("the-bondi-store")}
	svc := newTestService(repo, (&recordingOutbox{}).enqueue)

	_, err := svc.PurgeTenant(t.Context(), "t-1", []string{"the-bondi-store", "bondi-outlet"})

	var me *MismatchError
	require.True(t, errors.As(err, &me), "a supplied superset must be a mismatch, got %T", err)
	require.Empty(t, repo.deleted)
}

func TestPurgeTenant_EmptySetMatchesAStorelessTenant(t *testing.T) {
	repo := &fakeTenantRepo{snap: snapshotWith()}
	ob := &recordingOutbox{}
	svc := newTestService(repo, ob.enqueue)

	res, err := svc.PurgeTenant(t.Context(), "t-1", []string{})

	require.NoError(t, err)
	require.Equal(t, []string{"t-1"}, repo.deleted)
	require.Equal(t, []string{}, res.StoreIDs, "must be an empty slice, never nil")
	require.Equal(t, []string{}, res.StoreSlugs)
	require.Equal(t, []string{TenantDeletedOutboxKind}, ob.kinds)
}

func TestPurgeTenant_EmptySetIsRefusedWhenTheTenantHasStores(t *testing.T) {
	repo := &fakeTenantRepo{snap: snapshotWith("the-bondi-store")}
	svc := newTestService(repo, (&recordingOutbox{}).enqueue)

	_, err := svc.PurgeTenant(t.Context(), "t-1", []string{})

	var me *MismatchError
	require.True(t, errors.As(err, &me), "want *MismatchError, got %T", err)
	require.Empty(t, repo.deleted)
}

func TestPurgeTenant_UnknownTenantPropagatesNotFound(t *testing.T) {
	repo := &fakeTenantRepo{snapErr: apperrors.NotFound("tenant_not_found", "nope")}
	svc := newTestService(repo, (&recordingOutbox{}).enqueue)

	_, err := svc.PurgeTenant(t.Context(), "t-1", []string{"x"})

	ae, ok := apperrors.As(err)
	require.True(t, ok, "want an *apperrors.AppError, got %T", err)
	require.Equal(t, "tenant_not_found", ae.Code)
}
```

- [ ] **Step 3: Run and watch it fail**

```bash
cd services/platform-api
go test -count=1 -v ./internal/account/ -run TestPurgeTenant 2>&1 | tail -40
```

Expected: compile failure on `PurgeTenant`, `MismatchError`, `TenantDeletedOutboxKind`. If `TenantDeletedOutboxKind`'s literal value differs from what `teardownTenantTx` currently passes (`"tenant.deleted"`), use the existing constant — do not introduce a second spelling.

- [ ] **Step 4: Implement**

Create `services/platform-api/internal/account/purge.go`:

```go
package account

import (
	"context"
	"fmt"
	"sort"

	"gorm.io/gorm"

	"github.com/mark8ly/platform-api/internal/tenant"
)

// MismatchError is returned by PurgeTenant when the operator's supplied
// store-slug set does not equal the tenant's actual set.
//
// It carries the actual set so the caller can answer 409 with what the
// console should have sent, sparing it a second round trip. Disclosing it
// is safe: the caller is already authenticated on the internal boundary
// and already holds the tenant's detail row.
type MismatchError struct {
	Expected []string
}

func (e *MismatchError) Error() string {
	return fmt.Sprintf("account: store slug confirmation mismatch; expected %v", e.Expected)
}

// PurgeResult is what a successful operator teardown reports back.
// StoreIDs is what marketplace-api needs to scope its own purge; the rest
// is echoed to the operator.
type PurgeResult struct {
	TenantID   string
	TenantName string
	StoreIDs   []string
	StoreSlugs []string
}

// PurgeTenant is the operator-initiated tenant teardown behind
// POST /admin/tenants/{id}/purge (#288). It is IRREVERSIBLE.
//
// It is deliberately NOT a branch of DeleteAccount: that function opens by
// resolving the ACTOR's FGA role and requires RoleOwner, and a platform
// operator holds no FGA role at all. Sharing the entry point would mean
// threading a skip-authorization flag through the one function whose job
// is authorization.
//
// The confirmation check runs INSIDE the teardown transaction, against a
// snapshot taken under SELECT ... FOR UPDATE. Comparing slugs read outside
// the transaction that deletes the row is the same stale read the check
// exists to prevent, only with a shorter window.
//
// Post-commit cleanup mirrors deleteOwnerAccount and is best-effort for
// the same reason: the tenant.deleted outbox event enqueued inside the
// transaction is the real retry channel, so an FGA or GIP hiccup is logged
// rather than surfaced. fga and gip may be nil here — unlike the merchant
// path, this route is mounted unconditionally, because a route that is
// absent answers 404 and the caller cannot tell that apart from "no such
// tenant".
//
// KNOWN GAP, inherited from deleteOwnerAccount rather than introduced
// here: authz.Client has no method enumerating a tenant's members
// (DeleteTuple requires a userID), so staff/admin/viewer tuples and their
// GIP identities survive, pointing at a tenant object that no longer
// exists. Filed separately.
func (s *Service) PurgeTenant(ctx context.Context, tenantID string, suppliedSlugs []string) (*PurgeResult, error) {
	var snap *tenant.TeardownSnapshot

	run := func(tx *gorm.DB) error {
		var err error
		snap, err = s.repo.SnapshotForTeardown(ctx, tx, tenantID)
		if err != nil {
			return err
		}

		actual := slugsOf(snap.Stores)
		if !sameSet(suppliedSlugs, actual) {
			return &MismatchError{Expected: actual}
		}

		if err := s.repo.DeleteInTx(ctx, tx, tenantID); err != nil {
			return err
		}
		return s.outbox(tx, TenantDeletedOutboxKind, tenantDeletedPayload{
			TenantID: tenantID,
			StoreIDs: idsOf(snap.Stores),
		})
	}

	if s.db == nil {
		if err := run(nil); err != nil {
			return nil, err
		}
	} else if err := s.db.WithContext(ctx).Transaction(run); err != nil {
		return nil, err
	}

	s.cleanupAfterTeardown(ctx, snap)

	return &PurgeResult{
		TenantID:   tenantID,
		TenantName: snap.Name,
		StoreIDs:   idsOf(snap.Stores),
		StoreSlugs: slugsOf(snap.Stores),
	}, nil
}

// cleanupAfterTeardown removes the owner's FGA role tuple, the store
// parent tuples and the owner's GIP identity. Every failure is logged and
// swallowed: the transaction has already committed and the outbox event is
// the durable retry channel. Nil-tolerant on both clients.
func (s *Service) cleanupAfterTeardown(ctx context.Context, snap *tenant.TeardownSnapshot) {
	if s.fga != nil {
		if err := s.fga.DeleteTuple(ctx, snap.OwnerUserID, string(authz.RoleOwner), snap.TenantID); err != nil {
			s.warn("account: post-purge owner tuple delete failed",
				"tenant_id", snap.TenantID, "owner_uid", snap.OwnerUserID, "err", err)
		}
		for _, st := range snap.Stores {
			if err := s.fga.DeleteStoreParent(ctx, st.ID, snap.TenantID); err != nil {
				s.warn("account: post-purge store parent delete failed",
					"tenant_id", snap.TenantID, "store_id", st.ID, "err", err)
			}
		}
	} else {
		s.warn("account: post-purge FGA cleanup skipped, no client configured",
			"tenant_id", snap.TenantID)
	}

	if s.gip != nil {
		if err := s.gip.DeleteAccount(ctx, snap.OwnerUserID); err != nil {
			s.warn("account: post-purge gip delete failed",
				"tenant_id", snap.TenantID, "owner_uid", snap.OwnerUserID, "err", err)
		}
	} else {
		s.warn("account: post-purge GIP cleanup skipped, no client configured",
			"tenant_id", snap.TenantID)
	}
}

func idsOf(refs []tenant.StoreRef) []string {
	out := make([]string, 0, len(refs))
	for _, r := range refs {
		out = append(out, r.ID)
	}
	return out
}

func slugsOf(refs []tenant.StoreRef) []string {
	out := make([]string, 0, len(refs))
	for _, r := range refs {
		out = append(out, r.Slug)
	}
	sort.Strings(out)
	return out
}

// sameSet reports whether a and b contain exactly the same slugs,
// ignoring order. A supplied subset is NOT a match: a comparison
// implemented as "every supplied slug exists" would silently accept an
// operator who confirmed one store of two.
func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	x := append([]string(nil), a...)
	y := append([]string(nil), b...)
	sort.Strings(x)
	sort.Strings(y)
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}
```

Add the `authz` import. If `TenantDeletedOutboxKind` is not already an exported constant, promote the literal `"tenant.deleted"` in `teardownTenantTx` to one and use it in both places — two spellings of one outbox kind is exactly the drift this endpoint cannot afford.

- [ ] **Step 5: Run the tests**

```bash
cd services/platform-api
go test -count=1 -v ./internal/account/ -run TestPurgeTenant 2>&1 | tail -40
echo "exit=$?"
```

Expected: all `--- PASS`, exit 0.

- [ ] **Step 6: Prove the set comparison is load-bearing**

Replace `sameSet` with `func sameSet(a, b []string) bool { return true }` and re-run. **Four** tests must fail: `AnotherTenantsSlugsAreRefused`, `SubsetIsRefused`, `SupersetIsRefused`, `EmptySetIsRefusedWhenTheTenantHasStores`. If fewer fail, a fixture is not on the property. Revert.

- [ ] **Step 7: Commit**

```bash
git add services/platform-api/internal/account/purge.go services/platform-api/internal/account/purge_test.go services/platform-api/internal/account/service.go
git commit -m "feat(account): operator-initiated tenant teardown with slug confirmation (#288)"
```

---

## Task 3: The teardown transaction against real Postgres

**Files:**
- Create: `services/platform-api/internal/account/purge_integration_test.go`

**Interfaces:**
- Consumes: `Service.PurgeTenant` (Task 2), `Repository.SnapshotForTeardown` (Task 1).
- Produces: nothing consumed by later tasks.

Task 2 proved the logic against fakes. This proves the three properties fakes cannot: that a mismatch leaves the tenant row intact, that a success deletes it **and** enqueues in one transaction, and that two concurrent purges have exactly one winner.

- [ ] **Step 1: Write the failing tests**

```go
//go:build integration

package account

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPurgeTenant_Integration_MismatchLeavesTenantIntact(t *testing.T) {
	db := testDB(t)
	svc := realService(t, db)

	tenantID := seedTenantWithStores(t, db, "The Bondi Store", "the-bondi-store")

	_, err := svc.PurgeTenant(t.Context(), tenantID, []string{"the-facade-factory"})
	require.Error(t, err)

	var n int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM tenants WHERE id = ?`, tenantID).Scan(&n).Error)
	require.EqualValues(t, 1, n, "a mismatched purge must roll back the transaction")

	var outboxRows int64
	require.NoError(t, db.Raw(
		`SELECT count(*) FROM outbox_events WHERE payload->>'tenant_id' = ?`, tenantID).Scan(&outboxRows).Error)
	require.EqualValues(t, 0, outboxRows, "a mismatched purge must enqueue nothing")
}

func TestPurgeTenant_Integration_SuccessDeletesAndEnqueuesTogether(t *testing.T) {
	db := testDB(t)
	svc := realService(t, db)

	tenantID := seedTenantWithStores(t, db, "The Bondi Store", "the-bondi-store")

	res, err := svc.PurgeTenant(t.Context(), tenantID, []string{"the-bondi-store"})
	require.NoError(t, err)
	require.Len(t, res.StoreIDs, 1)

	var tenants, stores, outboxRows int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM tenants WHERE id = ?`, tenantID).Scan(&tenants).Error)
	require.NoError(t, db.Raw(`SELECT count(*) FROM stores WHERE tenant_id = ?`, tenantID).Scan(&stores).Error)
	require.NoError(t, db.Raw(
		`SELECT count(*) FROM outbox_events WHERE payload->>'tenant_id' = ?`, tenantID).Scan(&outboxRows).Error)

	require.EqualValues(t, 0, tenants)
	require.EqualValues(t, 0, stores, "stores CASCADE from tenants")
	require.EqualValues(t, 1, outboxRows, "exactly one tenant.deleted event")
}

// Two concurrent purges of one tenant. The property discriminates between
// "one winner" and "two winners", so the fixture contains two callers —
// a single call could never tell those apart.
func TestPurgeTenant_Integration_ConcurrentPurgesHaveExactlyOneWinner(t *testing.T) {
	db := testDB(t)
	svc := realService(t, db)

	tenantID := seedTenantWithStores(t, db, "The Bondi Store", "the-bondi-store")

	var wg sync.WaitGroup
	errs := make([]error, 2)
	start := make(chan struct{})
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, errs[i] = svc.PurgeTenant(t.Context(), tenantID, []string{"the-bondi-store"})
		}(i)
	}
	close(start)
	wg.Wait()

	winners := 0
	for _, err := range errs {
		if err == nil {
			winners++
		}
	}
	require.Equal(t, 1, winners, "exactly one purge may succeed, got %d (errors: %v)", winners, errs)

	var outboxRows int64
	require.NoError(t, db.Raw(
		`SELECT count(*) FROM outbox_events WHERE payload->>'tenant_id' = ?`, tenantID).Scan(&outboxRows).Error)
	require.EqualValues(t, 1, outboxRows, "the loser must not enqueue a second purge event")
}
```

Write `testDB`, `realService` and `seedTenantWithStores` as small helpers in this file, following whatever the package's other integration tests already do. `realService` constructs `NewService(db, tenant.NewRepository(db), nil, nil, outbox.Enqueue, slog.Default())` — nil FGA and GIP, which is exactly what Task 2 made tolerable and is what production will pass in an env without them.

Reference data: `GB`/`GBP`/`Europe/London` only. `IE`/`Europe/Dublin` are **not** in platform-api's seed and its `stores` FKs to `countries`/`currencies`/`timezones`.

- [ ] **Step 2: Run and watch them fail**

```bash
cd services/platform-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/platform_api?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 -v ./internal/account/ -run TestPurgeTenant_Integration 2>&1 | tail -40
```

Expected: compile failure on the helpers. Confirm from the verbose output that they RAN once written — not `--- SKIP`.

- [ ] **Step 3: Add the helpers, run to green**

```bash
cd services/platform-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/platform_api?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 -v ./internal/account/ -run TestPurgeTenant_Integration 2>&1 | tail -40
echo "exit=$?"
```

Expected: three `--- PASS`, exit 0.

- [ ] **Step 4: Prove the FOR UPDATE is what serialises them**

Remove `FOR UPDATE` from `SnapshotForTeardown`'s query and re-run the concurrency test several times:

```bash
cd services/platform-api
for i in 1 2 3 4 5; do
  TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/platform_api?sslmode=disable' \
    go test -tags=integration -p 1 -count=1 ./internal/account/ -run ConcurrentPurges 2>&1 | tail -3
done
```

`DeleteInTx`'s `RowsAffected == 0` check should still hold the line, so this may stay green — record what you observe either way, and say plainly in the task report whether the lock or the row check is doing the work. Restore `FOR UPDATE` regardless: two guarantees on an irreversible path is the intent, and a test that cannot distinguish them is a fact worth reporting, not hiding.

- [ ] **Step 5: Full-suite check and commit**

```bash
cd services/platform-api
go vet -tags=integration ./... && echo "VET OK"
go test -count=1 ./... 2>&1 | grep -v "^ok" | head -20
```

```bash
git add services/platform-api/internal/account/purge_integration_test.go
git commit -m "test(account): integration coverage for operator teardown transaction (#288)"
```

---

## Task 4: Mount the operator teardown route

**Files:**
- Modify: `services/platform-api/internal/account/handler.go`
- Modify: `services/platform-api/internal/account/handler_test.go`
- Modify: `services/platform-api/cmd/server/main.go:301-307` and `:353-357`

**Interfaces:**
- Consumes: `Service.PurgeTenant`, `MismatchError`, `PurgeResult` (Task 2).
- Produces: `POST /internal/tenants/:id/teardown`, consumed by Task 8's client.

- [ ] **Step 1: Write the failing handler tests**

Add to `services/platform-api/internal/account/handler_test.go`:

```go
type fakePurger struct {
	res *PurgeResult
	err error
	got []string
}

func (f *fakePurger) DeleteAccount(_ context.Context, _, _ string) error { return nil }
func (f *fakePurger) PurgeTenant(_ context.Context, _ string, slugs []string) (*PurgeResult, error) {
	f.got = slugs
	return f.res, f.err
}

func teardownRouter(svc accountDeleter) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	NewHandler(svc).RegisterOperator(r.Group("/internal"))
	return r
}

func TestTeardown_SuccessReturnsResult(t *testing.T) {
	f := &fakePurger{res: &PurgeResult{
		TenantID: "t-1", TenantName: "The Bondi Store",
		StoreIDs: []string{"s-1"}, StoreSlugs: []string{"the-bondi-store"},
	}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/internal/tenants/t-1/teardown",
		strings.NewReader(`{"store_slugs":["the-bondi-store"]}`))
	teardownRouter(f).ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, []string{"the-bondi-store"}, f.got)
	require.JSONEq(t, `{"data":{"tenant_id":"t-1","tenant_name":"The Bondi Store","store_ids":["s-1"],"store_slugs":["the-bondi-store"]}}`, rec.Body.String())
}

func TestTeardown_MismatchIs409WithExpectedSet(t *testing.T) {
	f := &fakePurger{err: &MismatchError{Expected: []string{"a", "b"}}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/internal/tenants/t-1/teardown",
		strings.NewReader(`{"store_slugs":["wrong"]}`))
	teardownRouter(f).ServeHTTP(rec, req)

	require.Equal(t, http.StatusConflict, rec.Code)
	require.JSONEq(t, `{"error":"confirmation_mismatch","message":"supplied store_slugs do not match the tenant's current stores","expected":["a","b"]}`, rec.Body.String())
}

func TestTeardown_NotFoundIs404(t *testing.T) {
	f := &fakePurger{err: apperrors.NotFound("tenant_not_found", "nope")}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/internal/tenants/t-1/teardown",
		strings.NewReader(`{"store_slugs":[]}`))
	teardownRouter(f).ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
}

// An ABSENT store_slugs must fail. An EMPTY one is a legitimate assertion
// that the tenant has no stores, and only matches a tenant that has none.
// The two are one character apart on the wire and must not collapse.
func TestTeardown_AbsentStoreSlugsIs400_EmptyIsAccepted(t *testing.T) {
	absent := httptest.NewRecorder()
	teardownRouter(&fakePurger{}).ServeHTTP(absent,
		httptest.NewRequest(http.MethodPost, "/internal/tenants/t-1/teardown", strings.NewReader(`{}`)))
	require.Equal(t, http.StatusBadRequest, absent.Code)

	f := &fakePurger{res: &PurgeResult{TenantID: "t-1", StoreIDs: []string{}, StoreSlugs: []string{}}}
	empty := httptest.NewRecorder()
	teardownRouter(f).ServeHTTP(empty,
		httptest.NewRequest(http.MethodPost, "/internal/tenants/t-1/teardown", strings.NewReader(`{"store_slugs":[]}`)))
	require.Equal(t, http.StatusOK, empty.Code)
	require.NotNil(t, f.got, "an empty array must reach the service as a non-nil empty slice")
	require.Empty(t, f.got)
}
```

- [ ] **Step 2: Run and watch it fail**

```bash
cd services/platform-api
go test -count=1 -v ./internal/account/ -run TestTeardown 2>&1 | tail -40
```

Expected: compile failure on `RegisterOperator`.

- [ ] **Step 3: Implement the handler**

In `internal/account/handler.go`, widen the interface and add the route:

```go
// accountDeleter is the subset of *Service the handler depends on.
type accountDeleter interface {
	DeleteAccount(ctx context.Context, tenantID, actorUID string) error
	PurgeTenant(ctx context.Context, tenantID string, storeSlugs []string) (*PurgeResult, error)
}

// teardownRequest is the body for POST /internal/tenants/:id/teardown.
//
// StoreSlugs is a POINTER so an ABSENT field and an EMPTY array stay
// distinguishable. Absent is a client that dropped the confirmation and
// must fail; empty is a deliberate assertion that this tenant has no
// stores, and matches only a tenant that has none. A plain []string
// collapses the two into nil.
type teardownRequest struct {
	StoreSlugs *[]string `json:"store_slugs"`
}

// RegisterOperator mounts the operator-initiated teardown.
//
// Mounted on the STRICT internal group (which answers 503 when the shared
// secret is unset), and mounted UNCONDITIONALLY — unlike Register, whose
// merchant route is gated on FGA and GIP being wired. An absent route
// answers 404, and the caller cannot tell that apart from "no such
// tenant", which on an irreversible endpoint would be a silent lie.
// PurgeTenant tolerates nil FGA and GIP clients for exactly this reason.
func (h *Handler) RegisterOperator(internal *gin.RouterGroup) {
	internal.Group("/tenants").POST("/:id/teardown", h.teardown)
}

func (h *Handler) teardown(c *gin.Context) {
	var req teardownRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.StoreSlugs == nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_request",
			"message": "store_slugs is required; send [] to assert the tenant has no stores",
		})
		return
	}

	res, err := h.svc.PurgeTenant(c.Request.Context(), c.Param("id"), *req.StoreSlugs)
	if err != nil {
		var me *MismatchError
		if errors.As(err, &me) {
			c.JSON(http.StatusConflict, gin.H{
				"error":    "confirmation_mismatch",
				"message":  "supplied store_slugs do not match the tenant's current stores",
				"expected": me.Expected,
			})
			return
		}
		respondError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"tenant_id":   res.TenantID,
		"tenant_name": res.TenantName,
		"store_ids":   res.StoreIDs,
		"store_slugs": res.StoreSlugs,
	}})
}
```

Add the `errors` import.

- [ ] **Step 4: Wire it in `main.go`**

Replace the gated construction at `:301-307` so the **service** is unconditional and only the **merchant route** stays gated:

```go
	// The operator teardown path (#288) needs neither FGA nor GIP to
	// function — its cleanup of both is best-effort post-commit — so the
	// service is constructed unconditionally and its route is mounted
	// unconditionally below. Only the MERCHANT DeleteAccount route stays
	// gated: it calls fga.GetRole and gip.DeleteAccount with no internal
	// nil-check and would panic on first call.
	accountSvc := account.NewService(conn, tenantRepo, fga, gipAdmin, outbox.Enqueue, log)
	accountHandler := account.NewHandler(accountSvc)
	merchantAccountRoutes := fga != nil && gipAdmin != nil
	if !merchantAccountRoutes {
		log.Warn("account: merchant teardown endpoint disabled — missing OpenFGA store or GIP_PROJECT_ID/GIP_TENANT_ID/GIP_WEB_API_KEY; operator teardown (#288) stays mounted")
	}
```

At `:353-357`, beside the other `strictInternal` mounts:

```go
	accountHandler.RegisterOperator(strictInternal)
```

And at `:368-370`, replace the nil check with the flag:

```go
	if merchantAccountRoutes {
		accountHandler.Register(internal)
	}
```

- [ ] **Step 5: Run everything**

```bash
cd services/platform-api
go build ./... && echo "BUILD OK"
go test -count=1 ./... 2>&1 | grep -v "^ok" | head -20
go vet -tags=integration ./... && echo "VET OK"
```

Expected: build clean, no failures beyond any that already exist at `origin/main`.

- [ ] **Step 6: Prove the route is actually mounted**

```bash
cd services/platform-api
grep -n "RegisterOperator" cmd/server/main.go internal/account/handler.go
```

Both must appear. #323 records **five** instances in this estate of a route silently never mounted; a grep is thirty seconds.

- [ ] **Step 7: Commit**

```bash
git add services/platform-api/internal/account/handler.go services/platform-api/internal/account/handler_test.go services/platform-api/cmd/server/main.go
git commit -m "feat(account): mount POST /internal/tenants/:id/teardown on the strict internal group (#288)"
```

---

## Task 5: `tenantpurge` reports what it destroyed

**Files:**
- Modify: `services/marketplace-api/internal/tenantpurge/purge.go`
- Modify: `services/marketplace-api/internal/tenantpurge/purge_test.go`
- Modify: `services/marketplace-api/internal/tenantpurge/purge_integration_test.go`
- Modify: `services/marketplace-api/cmd/marketplace-api/main.go:2067-2074` and `:2191-2199`

**Interfaces:**
- Consumes: nothing.
- Produces: `tenantpurge.TableResult`, `tenantpurge.Report`, `Purge(...) (Report, error)`, `Count(...) (Report, error)`.

`Purge` currently discards `RowsAffected` on every step. #288's AC — "audited with … what was destroyed" — is a measurement, not a claim, and this is where the measurement comes from. `Count` is the same enumeration with `SELECT count(*)` substituted, so preview and purge cannot drift into disagreeing about which tables a purge reaches.

- [ ] **Step 1: Write the failing tests**

Add to `internal/tenantpurge/purge_test.go`:

```go
// The preview and the purge must enumerate exactly the same tables in
// exactly the same order. Two lists of "every tenant-scoped table" that
// must agree, with nothing forcing them to, is the defect this package's
// sibling (subscription/harddelete) already demonstrates.
func TestCountPlan_MatchesPurgePlanTableForTable(t *testing.T) {
	purge := purgePlan(testTenantID, testStoreIDs)
	count := countPlan(testTenantID, testStoreIDs)

	require.Equal(t, len(purge), len(count), "the two plans must have the same length")
	for i := range purge {
		require.Equal(t, purge[i].table, count[i].table, "step %d", i)
		require.Equal(t, purge[i].args, count[i].args, "step %d args", i)
	}
}

func TestCountPlan_SelectsRatherThanDeletes(t *testing.T) {
	for _, s := range countPlan(testTenantID, testStoreIDs) {
		require.True(t, strings.HasPrefix(s.sql, "SELECT count(*)"),
			"step %q must count, got %q", s.table, s.sql)
		require.NotContains(t, s.sql, "DELETE",
			"a preview step must contain no DELETE at all: %q", s.sql)
	}
}
```

Add to `internal/tenantpurge/purge_integration_test.go`:

```go
// Row counts are VALUES, not presence. A report assembled by map lookup
// returns a fabricated 0 for a missing key, and a test asserting the key
// exists passes on it — so every seeded table gets a DISTINCT non-zero
// count and the numbers are asserted.
func TestPurge_ReportsPerTableRowCounts(t *testing.T) {
	db := testDB(t)
	tenantID, storeIDs := seedPurgeFixture(t, db, map[string]int{
		"products": 3,
		"orders":   5,
		"reviews":  2,
	})

	rep, err := Purge(t.Context(), db, tenantID, storeIDs)
	require.NoError(t, err)

	got := map[string]int64{}
	for _, tr := range rep.Tables {
		got[tr.Table] = tr.RowsDeleted
	}
	require.EqualValues(t, 3, got["products"])
	require.EqualValues(t, 5, got["orders"])
	require.EqualValues(t, 2, got["reviews"])
	require.EqualValues(t, 10, rep.TotalRows, "TotalRows must be the sum, not a count of tables")

	// NOTE: this file is `package tenantpurge_test` (EXTERNAL), so purgePlan
	// is not reachable here — the plan-vs-preview parity property is
	// asserted by TestCountPlan_MatchesPurgePlanTableForTable in the
	// INTERNAL purge_test.go, where it compiles. Here we assert only that
	// every step reports, including the zero-row ones: an omitted zero and
	// an unenumerated table are indistinguishable to a reader.
	prev, err := Count(t.Context(), db, tenantID, storeIDs)
	require.NoError(t, err)
	require.Equal(t, len(prev.Tables), len(rep.Tables))
	require.Greater(t, len(rep.Tables), 50, "the plan enumerates ~53 tables explicitly")
}

func TestCount_ReportsTheSameRowsAndDestroysNothing(t *testing.T) {
	db := testDB(t)
	tenantID, storeIDs := seedPurgeFixture(t, db, map[string]int{"products": 3, "orders": 5})

	rep, err := Count(t.Context(), db, tenantID, storeIDs)
	require.NoError(t, err)
	require.EqualValues(t, 8, rep.TotalRows)

	var products int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM products WHERE tenant_id = ?`, tenantID).Scan(&products).Error)
	require.EqualValues(t, 3, products, "Count must not delete anything")
}

// A second purge is a no-op, not a partial re-run (#288 AC4).
func TestPurge_SecondRunReportsZeroAndSucceeds(t *testing.T) {
	db := testDB(t)
	tenantID, storeIDs := seedPurgeFixture(t, db, map[string]int{"products": 3})

	_, err := Purge(t.Context(), db, tenantID, storeIDs)
	require.NoError(t, err)

	rep, err := Purge(t.Context(), db, tenantID, storeIDs)
	require.NoError(t, err)
	require.EqualValues(t, 0, rep.TotalRows)
}
```

`internal/tenantpurge/purge_integration_test.go` is **`package tenantpurge_test` (external)** and already imports `pkg/testdb`. Add your tests there, use `testdb.NewDB(t)` for `db` (it gates on `TEST_DATABASE_URL` and skips when unset — do NOT write a second gating helper; a parallel helper on a different env var is how #317's 19 tests silently never ran), and write `seedPurgeFixture(t, db, map[string]int) (tenantID string, storeIDs []string)` on top of the file's existing `seedTenant` helper. Because the package is external, **`purgePlan` and `countPlan` are not reachable from these tests** — assert against the exported `Purge`/`Count` only. **Do not hand-write `INSERT INTO stores`** — migration `000058` declares `storefront_customer_portal_secret CHAR(64) NOT NULL` and dropped its DEFAULT (verified in production: `is_nullable = NO`, no default), so a raw insert fails. Use the existing helper.

- [ ] **Step 2: Run and watch them fail**

```bash
cd services/marketplace-api
go test -count=1 -v ./internal/tenantpurge/ -run 'TestCountPlan' 2>&1 | tail -20
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 -v ./internal/tenantpurge/ -run 'TestPurge_Reports|TestCount_|TestPurge_SecondRun' 2>&1 | tail -30
```

Expected: compile failures on `countPlan`, `Count`, and `Purge`'s new return type.

- [ ] **Step 3: Implement**

In `internal/tenantpurge/purge.go`:

```go
// TableResult is one table's contribution to a purge or a preview.
type TableResult struct {
	Table       string `json:"table"`
	RowsDeleted int64  `json:"rows"`
}

// Report is what a purge destroyed, or what a preview would destroy.
//
// It lists EVERY step in the plan, including the zero-row ones: to a
// reader, an omitted zero and a table the plan never reaches are
// indistinguishable, and showing what the plan reaches is the whole point
// of the preview.
type Report struct {
	Tables    []TableResult `json:"tables"`
	TotalRows int64         `json:"total_rows"`
}
```

Change `Purge`'s signature and body:

```go
func Purge(ctx context.Context, db *gorm.DB, tenantID string, storeIDs []string) (Report, error) {
	if db == nil {
		return Report{}, fmt.Errorf("tenantpurge: db must not be nil")
	}
	if tenantID == "" {
		return Report{}, fmt.Errorf("tenantpurge: tenantID must not be empty")
	}

	steps := purgePlan(tenantID, storeIDs)
	rep := Report{Tables: make([]TableResult, 0, len(steps))}

	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		rep.Tables = rep.Tables[:0]
		rep.TotalRows = 0
		for _, step := range steps {
			res := tx.Exec(step.sql, step.args...)
			if res.Error != nil {
				return fmt.Errorf("tenantpurge: delete from %s: %w", step.table, res.Error)
			}
			rep.Tables = append(rep.Tables, TableResult{Table: step.table, RowsDeleted: res.RowsAffected})
			rep.TotalRows += res.RowsAffected
		}
		return nil
	})
	if err != nil {
		return Report{}, err
	}
	return rep, nil
}

// Count runs the purge plan's enumeration with SELECT count(*) in place of
// DELETE. It destroys nothing and is what backs the operator preview
// (#288). It derives from countPlan, which derives from the same table
// list as purgePlan, so a preview can never enumerate a different set of
// tables from the purge it previews.
func Count(ctx context.Context, db *gorm.DB, tenantID string, storeIDs []string) (Report, error) {
	if db == nil {
		return Report{}, fmt.Errorf("tenantpurge: db must not be nil")
	}
	if tenantID == "" {
		return Report{}, fmt.Errorf("tenantpurge: tenantID must not be empty")
	}

	steps := countPlan(tenantID, storeIDs)
	rep := Report{Tables: make([]TableResult, 0, len(steps))}
	for _, step := range steps {
		var n int64
		if err := db.WithContext(ctx).Raw(step.sql, step.args...).Scan(&n).Error; err != nil {
			return Report{}, fmt.Errorf("tenantpurge: count %s: %w", step.table, err)
		}
		rep.Tables = append(rep.Tables, TableResult{Table: step.table, RowsDeleted: n})
		rep.TotalRows += n
	}
	return rep, nil
}

// countPlan mirrors purgePlan step for step, rewriting each DELETE as the
// equivalent SELECT count(*). It is derived from purgePlan rather than
// written out a second time — a hand-maintained twin of a hand-maintained
// list is how the two enumerations in this service came to disagree.
func countPlan(tenantID string, storeIDs []string) []deleteStep {
	steps := purgePlan(tenantID, storeIDs)
	out := make([]deleteStep, 0, len(steps))
	for _, s := range steps {
		out = append(out, deleteStep{
			table: s.table,
			sql:   "SELECT count(*) FROM" + strings.TrimPrefix(s.sql, "DELETE FROM"),
			args:  s.args,
		})
	}
	return out
}
```

- [ ] **Step 4: Adapt both `main.go` call sites**

At `cmd/marketplace-api/main.go:2067-2074` and `:2191-2199`, both closures currently return `tenantpurge.Purge(...)` directly. Change each to:

```go
		internalsvc.NewTenantPurgeHandler(func(ctx context.Context, tenantID string, storeIDs []string) error {
			_, err := tenantpurge.Purge(ctx, conn, tenantID, storeIDs)
			return err
		}).Register(r.Group("/internal"), cfg.InternalAuthSecret)
```

(the second uses `engine.Group("/internal")`). The drainer-driven path does not consume the report; only the operator path does.

- [ ] **Step 5: Run to green**

```bash
cd services/marketplace-api
go build ./... && echo "BUILD OK"
go test -count=1 -v ./internal/tenantpurge/ 2>&1 | tail -20
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 -v ./internal/tenantpurge/ 2>&1 | tail -30
echo "exit=$?"
go vet -tags=integration ./... && echo "VET OK"
```

- [ ] **Step 6: Prove the parity test bites**

Add a bogus extra step to `countPlan` (`out = append(out, deleteStep{table: "nope"})`) and re-run `TestCountPlan_MatchesPurgePlanTableForTable`. It **must fail**. Revert.

- [ ] **Step 7: Commit**

```bash
git add services/marketplace-api/internal/tenantpurge/ services/marketplace-api/cmd/marketplace-api/main.go
git commit -m "feat(tenantpurge): report per-table rows destroyed, add non-destructive Count (#288)"
```

---

## Task 6: Correct the protected-tables rationale

**Files:**
- Modify: `services/marketplace-api/internal/tenantpurge/purge.go` (package doc, the "Legally-protected / append-only tables" block)
- Modify: `services/marketplace-api/internal/tenantpurge/purge_test.go` (`protectedTables`' comment)

**Interfaces:**
- Consumes: nothing. Produces: nothing. This is a correctness fix to prose that redirects the next reader away from checking.

The exclusion **decision** is right and does not change. Its stated **reason** is false, and the false version says the database is a second line of defence when it is not.

Measured against production on 2026-08-25: the service connects as `marketplace_api` (username in the `mark8ly-postgres-marketplace-api` secret), and that role **owns** `business_entity_attestations`, `app_contract_attestations`, `subscription_plan_change_audit` and `billing_archive` with full `arwdDxt`. `REVOKE … FROM PUBLIC` never applied to the owner. Only `break_glass_lockouts` genuinely errors — it is owned by `postgres` — and that comment is correct as written.

- [ ] **Step 1: Re-measure before writing (do not trust this plan either)**

```bash
# Select the primary by ROLE, never by pod name. CloudNativePG reschedules
# instances and the names rotate — on 2026-08-25 the documented
# `mark8ly-postgres-2` answered "pods not found" mid-session while the
# primary was `mark8ly-postgres-3`, then came back. A hardcoded pod name is
# a check that stops working without telling you it stopped.
PGPOD=$(kubectl get pods -n mark8ly \
  -l cnpg.io/cluster=mark8ly-postgres,cnpg.io/instanceRole=primary \
  -o jsonpath='{.items[0].metadata.name}')
echo "primary=$PGPOD"

kubectl exec -n mark8ly "$PGPOD" -c postgres -- psql -U postgres -d mark8ly_marketplace_api -tAF'|' -c "
SELECT relname, relowner::regrole::text, coalesce(array_to_string(relacl,' '),'(default)')
FROM pg_class WHERE relnamespace='public'::regnamespace
AND relname IN ('subscription_plan_change_audit','business_entity_attestations','app_contract_attestations','billing_archive','break_glass_lockouts');"
kubectl get secret -n mark8ly mark8ly-postgres-marketplace-api -o jsonpath='{.data.username}' | base64 -d; echo
```

If what you see differs from the paragraph above, the plan is stale — report that and write what you measured, not what the plan says.

- [ ] **Step 2: Replace the rationale in `purge.go`'s package doc**

Replace the block beginning "Legally-protected / append-only tables — deleting these would either error (DB role has DELETE revoked) or defeat their entire purpose" with:

```
// Legally-protected / append-only tables — excluded because deleting them
// would destroy records that must outlive the tenant for compliance.
//
// NOTE, corrected 2026-08-25 against production: an earlier version of
// this comment said these tables were ALSO protected by the database,
// because "the DB role has DELETE revoked". That is FALSE for all four.
// The service connects as `marketplace_api`, which OWNS every one of them
// with full arwdDxt — migration 000045/000075/000050's `REVOKE ... FROM
// PUBLIC` never applied to the owner. Nothing in the database stops a
// future step from deleting them. This Go list and purge_test.go's
// protectedTables are the ONLY things that do.
//
//   - business_entity_attestations — KYB attestation log.
//   - app_contract_attestations — Apple 4.2.6 attestation log.
//   - subscription_plan_change_audit — append-only billing-change trail.
//   - billing_archive — populated by internal/billingarchive.Builder AFTER
//     a store hard-delete specifically so it SURVIVES the tenant's own
//     deletion (7-year GDPR/tax retention, §23.2). It is keyed by
//     original_tenant_id/original_store_id, not tenant_id/store_id — that
//     rename is itself a signal it has a different lifecycle. Purging it
//     here would delete the compliance record the purge is meant to leave
//     behind.
//
// break_glass_lockouts is the one table where the privilege claim IS
// true: it is owned by `postgres` in production, so marketplace_api has
// no DELETE and including it would abort the whole single-tx purge
// (SQLSTATE 42501). See the inline comment in group 5.
```

- [ ] **Step 3: Correct `protectedTables`' comment in `purge_test.go`**

Replace "deleting them would either error (DB role has DELETE revoked) or defeat their entire compliance purpose" with:

```go
// protectedTables must NEVER appear in a purge plan: deleting them would
// destroy records that must outlive the tenant, or (for webhook_events and
// the global reference rows) touch data no tenant owns.
//
// This test is not a formality. Corrected 2026-08-25: the claim that the
// database also protects the first four — "DB role has DELETE revoked" —
// is false. marketplace_api OWNS them with full DELETE. This list is the
// only enforcement there is.
```

- [ ] **Step 4: Run the package's tests**

```bash
cd services/marketplace-api
go test -count=1 ./internal/tenantpurge/ && echo "OK"
```

Comment-only changes; the suite must stay green.

- [ ] **Step 5: Prove the list is still enforced**

Temporarily add `tenantScoped("billing_archive", tenantID)` to `purgePlan`'s group 5 and re-run. The protected-tables test **must fail**. Revert. A guard nobody has watched fail is a guard nobody has tested.

- [ ] **Step 6: Commit**

```bash
git add services/marketplace-api/internal/tenantpurge/
git commit -m "docs(tenantpurge): correct the false privilege claim protecting retention tables (#288)"
```

---

## Task 7: The schema-coverage guard

**Files:**
- Create: `services/marketplace-api/internal/tenantpurge/schema_coverage_integration_test.go`

**Interfaces:**
- Consumes: `purgePlan` (unexported — this test lives in `package tenantpurge`, internal, unlike the rest of marketplace-api's integration tests; say so in the file).
- Produces: nothing.

The plan is correct today — verified against production on 2026-08-25 by reading `information_schema` and computing cascade closure from `pg_constraint`. **Nothing forces it to stay correct**, and #288 makes the gap operator-triggerable. This is the guard the hand-maintained list has never had, and it is the single thing that would have prevented `subscription/harddelete/sweeper.go` from rotting into a sweep that cannot run.

- [ ] **Step 1: Write the test**

```go
//go:build integration

// This file is in the INTERNAL package (unlike marketplace-api's other
// integration tests, which are external `_test` packages) because it
// asserts against purgePlan, which is unexported by design — the plan is
// an implementation detail everywhere except here.
package tenantpurge

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestPurgePlan_CoversEveryTenantScopedTable fails when a tenant-scoped
// table exists that the plan neither deletes explicitly, nor reaches by ON
// DELETE CASCADE from a table it does delete, nor names as a deliberate
// exclusion.
//
// It reads the live schema and the live FK graph rather than the
// migrations, because the migrations are what the plan was originally
// derived from and re-reading them would only re-derive the same answer.
func TestPurgePlan_CoversEveryTenantScopedTable(t *testing.T) {
	db := testdb.NewDB(t)
	require.Empty(t, uncoveredTenantScopedTables(t, db),
		"tenant-scoped tables neither purged, nor cascaded, nor declared an exclusion.\n"+
			"A tenant purge would leave these rows behind. Either add a step to purgePlan or add the "+
			"table to declaredExclusions WITH a justification in purge.go's package doc.")
}

// uncoveredTenantScopedTables is the guard's computation, extracted so the
// test above and the probe test below run the SAME code — one asserting it
// finds nothing, the other asserting it finds a table planted for it.
func uncoveredTenantScopedTables(t *testing.T, db *gorm.DB) []string {
	t.Helper()

	type fk struct {
		Child      string
		Parent     string
		Confdeltype string
	}
	var fks []fk
	require.NoError(t, db.Raw(`
		SELECT conrelid::regclass::text  AS child,
		       confrelid::regclass::text AS parent,
		       confdeltype
		FROM pg_constraint
		WHERE contype = 'f' AND connamespace = 'public'::regnamespace`).Scan(&fks).Error)

	type tbl struct {
		TableName string
		HasTenant bool
		HasStore  bool
	}
	var tables []tbl
	require.NoError(t, db.Raw(`
		SELECT t.table_name,
		       COALESCE(bool_or(c.column_name = 'tenant_id'), false) AS has_tenant,
		       COALESCE(bool_or(c.column_name = 'store_id'),  false) AS has_store
		FROM information_schema.tables t
		LEFT JOIN information_schema.columns c
		  ON c.table_schema = t.table_schema AND c.table_name = t.table_name
		WHERE t.table_schema = 'public' AND t.table_type = 'BASE TABLE'
		GROUP BY t.table_name`).Scan(&tables).Error)

	deleted := map[string]bool{}
	for _, s := range purgePlan("11111111-1111-1111-1111-111111111111", []string{"22222222-2222-2222-2222-222222222222"}) {
		deleted[s.table] = true
	}
	require.NotEmpty(t, deleted, "purgePlan returned no steps — the guard would vacuously pass")

	// Cascade closure: a child of a deleted parent, via ON DELETE CASCADE,
	// is itself deleted. Iterate to a fixed point — the graph has chains
	// (products -> product_variants -> variant_stock).
	for changed := true; changed; {
		changed = false
		for _, e := range fks {
			if e.Confdeltype == "c" && deleted[e.Parent] && !deleted[e.Child] && e.Child != e.Parent {
				deleted[e.Child] = true
				changed = true
			}
		}
	}

	// Tables that carry a tenant_id or store_id and are deliberately NOT
	// purged. Each is justified in purge.go's package doc. Adding to this
	// list is a DECISION about a tenant's data surviving its own deletion,
	// and this test exists to make that decision explicit rather than
	// accidental.
	declaredExclusions := map[string]bool{
		"business_entity_attestations":   true, // KYB attestation log, must outlive the tenant
		"app_contract_attestations":      true, // Apple 4.2.6 attestation log
		"subscription_plan_change_audit": true, // append-only billing-change trail
		"break_glass_lockouts":           true, // owned by postgres; DELETE aborts the whole tx
	}

	uncovered := make([]string, 0, 4)
	for _, tb := range tables {
		if !tb.HasTenant && !tb.HasStore {
			continue // global reference data owns no tenant's rows
		}
		if deleted[tb.TableName] || declaredExclusions[tb.TableName] {
			continue
		}
		uncovered = append(uncovered, tb.TableName)
	}
	sort.Strings(uncovered)
	return uncovered
}

// The guard is only worth having if it can fail. This runs the SAME
// computation against a deliberately-uncovered table and asserts it is
// reported.
//
// Asserting instead that information_schema can see the probe table would
// be a test of information_schema, not of the guard: a coverage function
// that returned "everything is covered" unconditionally would pass it.
func TestPurgePlan_CoverageGuardDetectsAnUncoveredTable(t *testing.T) {
	db := testdb.NewDB(t)

	require.NoError(t, db.Exec(`CREATE TABLE IF NOT EXISTS tenantpurge_guard_probe (
		id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL)`).Error)
	t.Cleanup(func() { _ = db.Exec(`DROP TABLE IF EXISTS tenantpurge_guard_probe`).Error })

	require.Contains(t, uncoveredTenantScopedTables(t, db), "tenantpurge_guard_probe",
		"a tenant-scoped table that the plan neither deletes nor cascades to nor excludes must be reported")
}
```

Use the shared `pkg/testdb.NewDB(t)` — it already gates on `TEST_DATABASE_URL` and skips when unset. Do **not** write a bespoke helper: a parallel helper gating on a different env var is exactly how #317's 19 tests came to skip silently while reading as passes.

Note this file is `package tenantpurge` (INTERNAL) while `purge_integration_test.go` in the same directory is `package tenantpurge_test` (external). Go permits both in one directory; the internal one is required here because the guard asserts against the unexported `purgePlan`.

- [ ] **Step 2: Run it — expect PASS on the first try**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 -v ./internal/tenantpurge/ -run TestPurgePlan_Covers 2>&1 | tail -20
echo "exit=$?"
```

This is the unusual case where red-first does not apply: the plan is already correct, and the test's job is to keep it that way. If it FAILS, that is a **finding** — the plan has drifted since 2026-08-25 and the uncovered tables are the report. Stop and report rather than editing `declaredExclusions` to make it green.

- [ ] **Step 3: Prove the guard discriminates**

`TestPurgePlan_CoverageGuardDetectsAnUncoveredTable` already does this in-suite — it plants `tenantpurge_guard_probe` and asserts the shared helper reports it. Confirm it RAN and PASSED in the verbose output from Step 2, then prove the helper is not vacuously empty:

```bash
cd services/marketplace-api
# make the exclusion list swallow everything; the probe test MUST fail
#   declaredExclusions[tb.TableName] -> true
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 ./internal/tenantpurge/ -run CoverageGuardDetects 2>&1 | tail -10
echo "exit=$?"
```

Expected: FAIL. Revert. If it passes, the helper is not computing what the guard claims and the whole task is theatre.

If the probe table is ever left behind by an interrupted run, drop it with `docker run --rm postgres:15 psql "$DSN" -c "DROP TABLE IF EXISTS tenantpurge_guard_probe"` — `psql` is not installed locally, and the guard test will keep failing until it is gone.

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/internal/tenantpurge/schema_coverage_integration_test.go
git commit -m "test(tenantpurge): fail when a tenant-scoped table escapes the purge plan (#288)"
```

---

## Task 8: `Emitter.EmitSync`

**Files:**
- Modify: `services/marketplace-api/internal/audit/emitter.go`
- Modify: `services/marketplace-api/internal/audit/emitter_test.go`

**Interfaces:**
- Consumes: nothing. Produces: `(*audit.Emitter).EmitSync(c *gin.Context, ev Event) error`.

`purgePlan` contains `DELETE FROM audit_logs WHERE tenant_id = ?`. `Emit` is async, buffered, and drops on a full queue, so an `EmitOperatorAction` on the purge path races its own DELETE: the row may land before the purge and be destroyed, after it and survive, or never be written. AC#2 is not satisfiable through the existing helper.

`EmitSync` reuses `buildEntry` rather than assembling an `Entry` a second time — a second derivation of actor type, operator id, capability and scope would drift the moment either path gained a field.

- [ ] **Step 1: Write the failing tests**

```go
func TestEmitSync_WritesBeforeReturning(t *testing.T) {
	repo := &recordingRepo{}
	e := NewEmitter(EmitterConfig{DB: nil, Repo: repo, Logger: slog.Default()})
	t.Cleanup(func() { e.Stop(context.Background()) })

	c := ginContextWithOperator(t, "op-7", "tenants.purge")
	tenantID := uuid.New()

	err := e.EmitSync(c, Event{
		Action: "tenant.purged", ResourceType: "tenant",
		TenantID: tenantID, Metadata: map[string]any{"total_rows": 42},
	})

	require.NoError(t, err)
	// The row is present on RETURN, with no sleep and no queue drain. An
	// async emitter passes an "eventually" assertion and fails this one.
	require.Len(t, repo.created, 1)
	require.Equal(t, tenantID, repo.created[0].TenantID)
	require.Equal(t, ActorOperator, repo.created[0].ActorType)
	require.Equal(t, "op-7", *repo.created[0].ActorOperatorID)
	require.Equal(t, "tenants.purge", *repo.created[0].Capability)
}

func TestEmitSync_ReturnsTheRepositoryError(t *testing.T) {
	repo := &recordingRepo{createErr: errors.New("boom")}
	e := NewEmitter(EmitterConfig{DB: nil, Repo: repo, Logger: slog.Default()})
	t.Cleanup(func() { e.Stop(context.Background()) })

	err := e.EmitSync(ginContextWithOperator(t, "op-7", "tenants.purge"),
		Event{Action: "tenant.purged", ResourceType: "tenant", TenantID: uuid.New()})

	require.Error(t, err, "an unrecorded irreversible action must be surfaced, never swallowed")
	require.Contains(t, err.Error(), "boom")
}

func TestEmitSync_MissingTenantIsAnError(t *testing.T) {
	repo := &recordingRepo{}
	e := NewEmitter(EmitterConfig{DB: nil, Repo: repo, Logger: slog.Default()})
	t.Cleanup(func() { e.Stop(context.Background()) })

	err := e.EmitSync(ginContextWithOperator(t, "op-7", "c"),
		Event{Action: "tenant.purged", ResourceType: "tenant"}) // no TenantID

	require.Error(t, err)
	require.Empty(t, repo.created, "a tenant-less row must never be written")
}

func TestEmitSync_NilReceiverIsAnError(t *testing.T) {
	var e *Emitter
	require.Error(t, e.EmitSync(nil, Event{Action: "a", ResourceType: "b", TenantID: uuid.New()}))
}
```

`recordingRepo` and `ginContextWithOperator` may already exist in `emitter_test.go` — read the file first and reuse rather than shadow. The context keys `buildEntry` reads are the literals `"platform_operator_id"` and `"platform_capability"` (`emitter.go:220-221`; the exported constants `platformadmin.CtxOperatorID` / `CtxCapability` hold the same strings but `audit` must not import `platformadmin`), so the helper is:

```go
func ginContextWithOperator(t *testing.T, operatorID, capability string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	c.Set("platform_operator_id", operatorID)
	c.Set("platform_capability", capability)
	return c
}
```

Verify those literals against `emitter.go` before writing the test — if they have moved, the test would pass on a row with no operator at all.

- [ ] **Step 2: Run and watch it fail**

```bash
cd services/marketplace-api
go test -count=1 -v ./internal/audit/ -run TestEmitSync 2>&1 | tail -30
```

Expected: compile failure, `e.EmitSync undefined`.

- [ ] **Step 3: Implement**

Immediately after `Emit` in `internal/audit/emitter.go`:

```go
// EmitSync writes an audit row on the CALLER's goroutine and returns the
// outcome. It is the exception to this package's fire-and-forget rule, and
// it exists for exactly one situation: an action whose own effect can
// destroy the audit row recording it.
//
// The tenant purge (#288) deletes `audit_logs WHERE tenant_id = ?`. Emit
// hands the row to a background worker, so an Emit on that path races its
// own DELETE — the row may land before the purge and be destroyed, after
// it and survive, or (queue full) never be written at all. None of those
// is an audit trail.
//
// Use Emit everywhere else. Audit must never gate a business request, and
// this function does exactly that.
//
// buildEntry is REUSED rather than an Entry assembled here: a second
// derivation of actor type, operator id, capability and scope would drift
// the moment either path gained a field.
func (e *Emitter) EmitSync(c *gin.Context, ev Event) error {
	if e == nil {
		return errors.New("audit.EmitSync: nil emitter")
	}
	if ev.Action == "" || ev.ResourceType == "" {
		return errors.New("audit.EmitSync: action and resource_type are required")
	}

	entry := buildEntry(c, ev)
	if entry == nil {
		return errors.New("audit.EmitSync: no tenant in scope, refusing to write a tenant-unscoped row")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := e.repo.Create(ctx, e.db, entry); err != nil {
		return fmt.Errorf("audit.EmitSync: insert: %w", err)
	}
	return nil
}
```

Add the `errors` and `fmt` imports if absent. A **fresh background context** rather than the request's, matching `write` — a client disconnecting mid-purge must not cancel the record of what was destroyed.

- [ ] **Step 4: Run to green**

```bash
cd services/marketplace-api
go test -count=1 -v ./internal/audit/ 2>&1 | tail -30
echo "exit=$?"
```

- [ ] **Step 5: Prove the test would catch an async implementation**

Temporarily change `EmitSync`'s body to `e.Emit(c, ev); return nil`. `TestEmitSync_WritesBeforeReturning` and `TestEmitSync_ReturnsTheRepositoryError` **must both fail**. If the first passes, the assertion is racing rather than asserting synchrony. Revert.

- [ ] **Step 6: Commit**

```bash
git add services/marketplace-api/internal/audit/
git commit -m "feat(audit): EmitSync for actions whose effect destroys their own audit row (#288)"
```

---

## Task 9: `tenantlifecycle.Teardown` client

**Files:**
- Modify: `services/marketplace-api/internal/tenantlifecycle/client.go`
- Modify: `services/marketplace-api/internal/tenantlifecycle/client_test.go`

**Interfaces:**
- Consumes: `POST /internal/tenants/:id/teardown` (Task 4).
- Produces: `tenantlifecycle.TeardownResult`, `tenantlifecycle.ConfirmationMismatchError`, `(*Client).Teardown`.

Extending `tenantlifecycle` rather than adding a fourth client: same upstream service, same secret, same sentinels, and its package doc already states the property that matters here — an error must never be conflated with an empty result.

The existing `post` cannot serve this: it sends no body and discards non-200 bodies, and the 409 body carries the `expected` set the console needs.

- [ ] **Step 1: Write the failing tests**

```go
func TestTeardown_DecodesResult(t *testing.T) {
	var gotBody, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody, gotAuth = string(b), r.Header.Get("X-Internal-Auth")
		require.Equal(t, "/internal/tenants/t-1/teardown", r.URL.Path)
		_, _ = w.Write([]byte(`{"data":{"tenant_id":"t-1","tenant_name":"The Bondi Store","store_ids":["s-1"],"store_slugs":["the-bondi-store"]}}`))
	}))
	defer srv.Close()

	res, err := NewClient(srv.URL, "shh", nil).Teardown(context.Background(), "t-1", []string{"the-bondi-store"})

	require.NoError(t, err)
	require.JSONEq(t, `{"store_slugs":["the-bondi-store"]}`, gotBody)
	require.Equal(t, "shh", gotAuth)
	require.Equal(t, "The Bondi Store", res.TenantName)
	require.Equal(t, []string{"s-1"}, res.StoreIDs)
}

func TestTeardown_EmptySlugSetIsSentAsAnArrayNotNull(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		_, _ = w.Write([]byte(`{"data":{"tenant_id":"t-1","store_ids":[],"store_slugs":[]}}`))
	}))
	defer srv.Close()

	_, err := NewClient(srv.URL, "", nil).Teardown(context.Background(), "t-1", []string{})

	require.NoError(t, err)
	// A nil slice marshals to null, which upstream reads as ABSENT and
	// refuses with 400. The two are one character apart on the wire.
	require.JSONEq(t, `{"store_slugs":[]}`, gotBody)
}

func TestTeardown_409CarriesTheExpectedSet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"confirmation_mismatch","expected":["a","b"]}`))
	}))
	defer srv.Close()

	_, err := NewClient(srv.URL, "", nil).Teardown(context.Background(), "t-1", []string{"wrong"})

	var me *ConfirmationMismatchError
	require.True(t, errors.As(err, &me), "want *ConfirmationMismatchError, got %T", err)
	require.Equal(t, []string{"a", "b"}, me.Expected)
}

func TestTeardown_404IsNotFound_500IsUnavailable(t *testing.T) {
	for _, tc := range []struct {
		status int
		want   error
	}{
		{http.StatusNotFound, ErrNotFound},
		{http.StatusInternalServerError, ErrUnavailable},
		{http.StatusServiceUnavailable, ErrUnavailable},
		{http.StatusBadGateway, ErrUnavailable},
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(tc.status)
		}))
		_, err := NewClient(srv.URL, "", nil).Teardown(context.Background(), "t-1", []string{})
		require.ErrorIs(t, err, tc.want, "status %d", tc.status)
		srv.Close()
	}
}

// A 200 with a broken body must be an error, never a zero result — the
// failure mode this package's doc comment was written about.
func TestTeardown_TruncatedBodyIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"tenant_id":`))
	}))
	defer srv.Close()

	res, err := NewClient(srv.URL, "", nil).Teardown(context.Background(), "t-1", []string{})
	require.Error(t, err)
	require.Nil(t, res)
}
```

- [ ] **Step 2: Run and watch it fail**

```bash
cd services/marketplace-api
go test -count=1 -v ./internal/tenantlifecycle/ -run TestTeardown 2>&1 | tail -30
```

- [ ] **Step 3: Implement**

Add to `internal/tenantlifecycle/client.go`:

```go
// ConfirmationMismatchError signals platform-api refused a teardown
// because the supplied store-slug set did not match the tenant's actual
// set. Expected carries the actual set, so the console can refresh without
// a second round trip.
type ConfirmationMismatchError struct {
	Expected []string
}

func (e *ConfirmationMismatchError) Error() string {
	return fmt.Sprintf("tenantlifecycle: store slug confirmation mismatch; expected %v", e.Expected)
}

// TeardownResult is the outcome of an operator-initiated tenant teardown.
// StoreIDs is what marketplace-api scopes its own purge by.
type TeardownResult struct {
	TenantID   string   `json:"tenant_id"`
	TenantName string   `json:"tenant_name"`
	StoreIDs   []string `json:"store_ids"`
	StoreSlugs []string `json:"store_slugs"`
}

// Teardown calls POST /internal/tenants/:id/teardown (#288). IRREVERSIBLE.
//
// It does not reuse `post`: that helper sends no body and discards
// non-200 bodies, and this call needs both — the confirmation set going
// up, and the 409's `expected` set coming back.
//
// storeSlugs is marshalled as an ARRAY even when empty. A nil slice
// marshals to `null`, which upstream reads as an ABSENT confirmation and
// refuses with 400 — and "I assert this tenant has no stores" is a
// legitimate request that must reach the check.
func (c *Client) Teardown(ctx context.Context, tenantID string, storeSlugs []string) (*TeardownResult, error) {
	if storeSlugs == nil {
		storeSlugs = []string{}
	}
	payload, err := json.Marshal(struct {
		StoreSlugs []string `json:"store_slugs"`
	}{StoreSlugs: storeSlugs})
	if err != nil {
		return nil, fmt.Errorf("tenantlifecycle: encode teardown body: %w", err)
	}

	path := "/internal/tenants/" + url.PathEscape(tenantID) + "/teardown"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("tenantlifecycle: build teardown request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.secret != "" {
		req.Header.Set("X-Internal-Auth", c.secret)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxBody))

	switch resp.StatusCode {
	case http.StatusOK:
		// fall through
	case http.StatusNotFound:
		return nil, ErrNotFound
	case http.StatusConflict:
		var mismatch struct {
			Expected []string `json:"expected"`
		}
		if readErr == nil {
			_ = json.Unmarshal(body, &mismatch)
		}
		if mismatch.Expected == nil {
			mismatch.Expected = []string{}
		}
		return nil, &ConfirmationMismatchError{Expected: mismatch.Expected}
	case http.StatusBadRequest:
		return nil, fmt.Errorf("tenantlifecycle: teardown rejected: %s", string(body))
	default:
		return nil, fmt.Errorf("%w: upstream %d", ErrUnavailable, resp.StatusCode)
	}

	if readErr != nil {
		return nil, fmt.Errorf("%w: read body: %v", ErrUnavailable, readErr)
	}
	var envelope struct {
		Data TeardownResult `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("tenantlifecycle: decode teardown: %w", err)
	}
	if envelope.Data.StoreIDs == nil {
		envelope.Data.StoreIDs = []string{}
	}
	if envelope.Data.StoreSlugs == nil {
		envelope.Data.StoreSlugs = []string{}
	}
	return &envelope.Data, nil
}
```

Add the `bytes` import.

- [ ] **Step 4: Run to green**

```bash
cd services/marketplace-api
go test -count=1 -v ./internal/tenantlifecycle/ 2>&1 | tail -30
echo "exit=$?"
```

- [ ] **Step 5: Prove the null/[] distinction is tested**

Delete the `if storeSlugs == nil { storeSlugs = []string{} }` guard and re-run. `TestTeardown_EmptySlugSetIsSentAsAnArrayNotNull` **must fail**. Revert.

- [ ] **Step 6: Commit**

```bash
git add services/marketplace-api/internal/tenantlifecycle/
git commit -m "feat(tenantlifecycle): Teardown client with slug confirmation and 409 mismatch decoding (#288)"
```

---

## Task 10: The preview and purge handlers

**Files:**
- Create: `services/marketplace-api/internal/handlers/platformadmin/tenant_purge.go`
- Test: `services/marketplace-api/internal/handlers/platformadmin/tenant_purge_test.go`
- Test data: `services/marketplace-api/internal/handlers/platformadmin/testdata/tenant_purge.golden.json`, `tenant_purge_preview.golden.json`

**Interfaces:**
- Consumes: `tenantlifecycle.TeardownResult`, `ConfirmationMismatchError` (Task 9); `tenantpurge.Report`, `Purge`, `Count` (Task 5); `(*audit.Emitter).EmitSync` (Task 8); `tenantdirectory.TenantDetail` and `StoreSummary` (existing); `TenantGateInvalidator` (existing, `routes.go`).
- Produces: `PurgeReasonCodes`, `TenantTeardown`, `Purger`, `NewTenantPurgeHandler`, `(*TenantPurgeHandler).Register`.

- [ ] **Step 1: Write the failing tests**

The constructor and wire types later tasks depend on, pinned here so Task 11 does not have to guess them:

```go
// operatorAuditFunc records a platform-operator action SYNCHRONOUSLY.
// Mirrors #287's lifecycleAuditFunc: test doubles capture the raw
// audit.Event, which the real *audit.Emitter cannot be made to do.
type operatorAuditFunc func(c *gin.Context, tenantID uuid.UUID, ev audit.Event) error

func NewTenantPurgeHandler(
	teardown TenantTeardown,
	purger Purger,
	dir TenantDirectory,
	emit operatorAuditFunc,
	invalidator TenantGateInvalidator,
	logger *slog.Logger,
) *TenantPurgeHandler

// purgeResponse is the POST's `data` payload. Six fields always present;
// `reason` is the only omitempty.
type purgeResponse struct {
	TenantID   string                    `json:"tenant_id"`
	TenantName string                    `json:"tenant_name"`
	StoreIDs   []string                  `json:"store_ids"`
	StoreSlugs []string                  `json:"store_slugs"`
	ReasonCode string                    `json:"reason_code"`
	Reason     string                    `json:"reason,omitempty"`
	Tables     []tenantpurge.TableResult `json:"tables"`
	TotalRows  int64                     `json:"total_rows"`
	PurgedAt   string                    `json:"purged_at"`
}

// previewResponse is the GET's `data` payload.
type previewResponse struct {
	TenantID   string                    `json:"tenant_id"`
	TenantName string                    `json:"tenant_name"`
	Status     string                    `json:"status"`
	StoreSlugs []string                  `json:"store_slugs"`
	Tables     []tenantpurge.TableResult `json:"tables"`
	TotalRows  int64                     `json:"total_rows"`
}
```

Create `tenant_purge_test.go` (external `package platformadmin_test`, per marketplace-api convention). The three tests most likely to be built wrong, in full — write these verbatim:

```go
// Fakes record a monotonic sequence number so ORDER is assertable. Order
// is the whole design here: an audit row written before the purge is
// deleted BY the purge (purgePlan contains DELETE FROM audit_logs WHERE
// tenant_id = ?), so "all three were called" is not the property.
type seq struct{ n int }

func (s *seq) next() int { s.n++; return s.n }

type fakeTeardown struct {
	seq      *seq
	at       int
	gotSlugs []string
	res      *tenantlifecycle.TeardownResult
	err      error
}

func (f *fakeTeardown) Teardown(_ context.Context, _ string, slugs []string) (*tenantlifecycle.TeardownResult, error) {
	f.at, f.gotSlugs = f.seq.next(), slugs
	return f.res, f.err
}

type fakePurger struct {
	seq        *seq
	at         int
	gotTenant  string
	gotStores  []string
	countCalls int
	rep        tenantpurge.Report
	err        error
}

func (f *fakePurger) Purge(_ context.Context, tenantID string, storeIDs []string) (tenantpurge.Report, error) {
	f.at, f.gotTenant, f.gotStores = f.seq.next(), tenantID, storeIDs
	return f.rep, f.err
}

func (f *fakePurger) Count(_ context.Context, _ string, _ []string) (tenantpurge.Report, error) {
	f.countCalls++
	return f.rep, nil
}

func TestPurge_HappyPathTearsDownThenPurgesThenAudits(t *testing.T) {
	sq := &seq{}
	td := &fakeTeardown{seq: sq, res: &tenantlifecycle.TeardownResult{
		TenantID: tenantID, TenantName: "The Bondi Store",
		StoreIDs: []string{"s-1", "s-2"}, StoreSlugs: []string{"a", "b"},
	}}
	pg := &fakePurger{seq: sq, rep: tenantpurge.Report{
		Tables: []tenantpurge.TableResult{{Table: "products", RowsDeleted: 3}}, TotalRows: 3,
	}}
	var auditAt int
	emit := func(*gin.Context, uuid.UUID, audit.Event) error { auditAt = sq.next(); return nil }

	rec := doPurge(t, td, pg, emit, `{"store_slugs":["a","b"],"reason_code":"merchant_request"}`)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, 1, td.at, "teardown must run first")
	require.Equal(t, 2, pg.at, "purge must run after teardown")
	require.Equal(t, 3, auditAt, "the audit row must be written AFTER the purge that would delete it")
}

func TestPurge_PurgeIsScopedToTheStoreIDsTeardownReturned(t *testing.T) {
	sq := &seq{}
	// TWO store ids. One could not distinguish "passes them through" from
	// "passes the first" or "passes an empty slice".
	td := &fakeTeardown{seq: sq, res: &tenantlifecycle.TeardownResult{
		TenantID: tenantID, StoreIDs: []string{"s-1", "s-2"}, StoreSlugs: []string{"a", "b"},
	}}
	pg := &fakePurger{seq: sq}

	doPurge(t, td, pg, noopEmit, `{"store_slugs":["a","b"],"reason_code":"merchant_request"}`)

	require.Equal(t, tenantID, pg.gotTenant)
	require.Equal(t, []string{"s-1", "s-2"}, pg.gotStores)
}

func TestPurge_ReasonIsCappedByRunesNotBytes(t *testing.T) {
	sq := &seq{}
	td := &fakeTeardown{seq: sq, res: &tenantlifecycle.TeardownResult{TenantID: tenantID, StoreIDs: []string{}, StoreSlugs: []string{}}}
	pg := &fakePurger{seq: sq}
	var got audit.Event
	emit := func(_ *gin.Context, _ uuid.UUID, ev audit.Event) error { got = ev; return nil }

	// 600 two-byte runes: a 500-BYTE cut lands mid-rune and yields invalid
	// UTF-8, which Postgres rejects on the jsonb write, which fails the
	// audit emit — an irreversible destruction recorded nowhere.
	long := strings.Repeat("é", 600)

	rec := doPurge(t, td, pg, emit, `{"store_slugs":[],"reason_code":"merchant_request","reason":"`+long+`"}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	stored, _ := got.Metadata["reason"].(string)
	require.Equal(t, 500, utf8.RuneCountInString(stored), "cap counts runes")
	require.True(t, utf8.ValidString(stored), "a byte-truncated multibyte string is invalid UTF-8")
	require.Less(t, len(stored), 1200)
}
```

`doPurge` is a small helper building the gin engine with the three fakes and serving one POST; `noopEmit` returns nil; `tenantID` is a package-level valid UUID string. The remaining tests, each named for the property it proves:

```go
// Fakes: teardown records the slugs it received and returns a canned
// result or error; purger records (tenantID, storeIDs) and returns a
// canned Report; emitter records the audit.Event synchronously — the real
// *audit.Emitter cannot be observed this way, which is why the handler
// takes a function rather than the concrete type (matching #287's
// lifecycleAuditFunc).
```

Tests, each named for the property it proves:

1. `TestPurge_HappyPathTearsDownThenPurgesThenAudits` — asserts the **order**: teardown called before purge, purge called before the audit emit. Record a monotonically increasing sequence number in each fake. Order is the whole design: audit-before-purge is destroyed by the purge.
2. `TestPurge_PurgeIsScopedToTheStoreIDsTeardownReturned` — the fake teardown returns `StoreIDs: ["s-1","s-2"]`; assert the purger received exactly those. **Two** store ids, not one: one cannot distinguish "passes them through" from "passes the first".
3. `TestPurge_ResponseCarriesTheReport` — distinct non-zero counts per table, asserted as values.
4. `TestPurge_MismatchIs409WithExpected` — fake returns `*tenantlifecycle.ConfirmationMismatchError{Expected: []string{"a","b"}}`; assert `409`, `error: "confirmation_mismatch"`, `expected: ["a","b"]`, **and that the purger was never called**.
5. `TestPurge_NotFoundIs404AndPurgesNothing` — `ErrNotFound` → `404`, purger never called.
6. `TestPurge_UnavailableIs503AndPurgesNothing` — `ErrUnavailable` → `503 upstream_unavailable`, purger never called. An unreachable upstream must never read as "nothing to do".
7. `TestPurge_UnknownReasonCodeIs400` — table-driven over `""`, `"nonsense"`, and **one valid code that must succeed in the same test**, so a validator that refuses everything fails.
8. `TestPurge_EveryDeclaredReasonCodeIsAccepted` — loop over `PurgeReasonCodes` itself, not a hand-written copy. A hand-written list is how "the test loops over these constants" became false twice in this milestone.
9. `TestPurge_AbsentStoreSlugsIs400_EmptyIsForwarded` — absent → `400`; `[]` → forwarded to teardown as a non-nil empty slice.
10. `TestPurge_ReasonIsCappedByRunesNotBytes` — send a `reason` of 600 multibyte runes (e.g. `strings.Repeat("é", 600)`); assert the stored reason is 500 **runes** and `utf8.ValidString` holds. A byte cut yields invalid UTF-8, Postgres rejects the jsonb, the audit emit fails — an irreversible destruction recorded nowhere.
11. `TestPurge_AuditFailureIsSurfaced` — emit returns an error; assert the response reports it rather than a bare 200.
12. `TestPurge_AuditRowCarriesOperatorCapabilityReasonAndCounts` — assert the captured `audit.Event`'s metadata values, not their presence.
13. `TestPurge_InvalidTenantIDIs400` — non-UUID `{id}`.
13b. `TestPurge_UnparseableBodyIs400` — a body of `{` → `400 invalid_request`, and the teardown fake never called. gin's JSON binder returns `io.EOF` for a wholly empty body, so an omitted body is rejected here and never reaches the reason-code check; `{}` binds successfully to the zero value and is what the absent-`store_slugs` check catches. The two paths are different and both need a case.
14. `TestPurge_GateIsInvalidatedAfterASuccessfulPurge` — and **not** invalidated on a 409.
15. `TestPurgePreview_ReturnsSlugsAndCounts` — asserts the slug set comes from the directory detail and counts from `Count`.
16. `TestPurgePreview_DestroysNothing` — the fake purger's `Purge` must never be called by the preview route.
17. `TestPurge_GoldenFixture` and `TestPurgePreview_GoldenFixture` — byte-compare marshalled output against `testdata/*.golden.json`.

- [ ] **Step 2: Run and watch them fail**

```bash
cd services/marketplace-api
go test -count=1 -v ./internal/handlers/platformadmin/ -run 'TestPurge' 2>&1 | tail -40
```

- [ ] **Step 3: Implement**

Create `internal/handlers/platformadmin/tenant_purge.go`. The load-bearing parts:

```go
// PurgeReasonCodes is the closed set of reasons a tenant may be purged
// for. Deliberately a different set from SuspendReasonCodes and from
// ExtendReasonCodes: the reasons for destroying a tenant are not the
// reasons for pausing one.
//
// merchant_request and erasure_request are kept distinct because only the
// second carries a statutory clock, and an audit trail that cannot tell
// them apart cannot answer the question a regulator asks.
var PurgeReasonCodes = []string{
	"merchant_request", // the merchant asked for their account and data to be deleted
	"erasure_request",  // a statutory erasure demand (GDPR art.17) — see #259
	"fraud",            // confirmed fraudulent tenant, removed after investigation
	"abandoned",        // onboarding never completed; a dormant tenant reclaimed
	"legal",            // a legal or regulatory demand other than erasure
	"operator_error",   // a tenant created in error, or a test tenant
}

// maxReasonRunes caps the free-text reason. Counted in RUNES, not bytes: a
// byte-truncated multibyte string is invalid UTF-8, Postgres rejects the
// jsonb, and the audit emit fails — which on this endpoint would mean an
// irreversible destruction recorded nowhere.
const maxReasonRunes = 500

// TenantTeardown is the subset of tenantlifecycle.Client this handler
// needs, declared locally so the handler is stubbable.
type TenantTeardown interface {
	Teardown(ctx context.Context, tenantID string, storeSlugs []string) (*tenantlifecycle.TeardownResult, error)
}

// Purger is tenantpurge's two entry points, declared locally for the same
// reason.
type Purger interface {
	Purge(ctx context.Context, tenantID string, storeIDs []string) (tenantpurge.Report, error)
	Count(ctx context.Context, tenantID string, storeIDs []string) (tenantpurge.Report, error)
}

// purgeRequest is the wire body.
//
// StoreSlugs is a POINTER so ABSENT and EMPTY stay distinguishable all the
// way down. Absent is a client that dropped the confirmation and must
// fail; empty asserts the tenant has no stores and must reach the check.
type purgeRequest struct {
	StoreSlugs *[]string `json:"store_slugs"`
	ReasonCode string    `json:"reason_code"`
	Reason     string    `json:"reason"`
}

func (h *TenantPurgeHandler) Register(g *gin.RouterGroup) {
	// MUST stay on the platformadmin group. The merchant tree registers
	// /admin/tenants/:tenantId/... under a DIFFERENT wildcard name at this
	// same path position, and two wildcard names at one position panic gin
	// at router build time. :id here matches suspend/unsuspend, already
	// mounted on this group.
	g.GET("/admin/tenants/:id/purge/preview", h.preview)
	g.POST("/admin/tenants/:id/purge", h.purge)
}
```

The purge handler's body, in order — and the order is the design:

```go
	// 1. Upstream teardown. Its transaction runs the confirmation check
	//    and deletes the tenant row; on return, the tenant.deleted outbox
	//    event guarantees the marketplace purge happens eventually
	//    whatever this request does next.
	res, err := h.teardown.Teardown(c.Request.Context(), tenantIDStr, *req.StoreSlugs)
	if err != nil { /* 409 / 404 / 503, purge nothing */ }

	// 2. Purge inline, for a real destruction report. The drainer is the
	//    backstop: if this fails, it retries, and Purge is idempotent.
	rep, purgeErr := h.purger.Purge(c.Request.Context(), tenantIDStr, res.StoreIDs)

	// 3. Drop the admin gate's cached status — without it the gate serves
	//    a cached status for up to its TTL for a tenant that no longer
	//    exists. Best-effort and nil-safe, matching #287.
	if h.invalidate != nil { h.invalidate.Invalidate(tenantIDStr) }

	// 4. Audit LAST and SYNCHRONOUSLY. purgePlan contains
	//    DELETE FROM audit_logs WHERE tenant_id = ?, so a row written
	//    before step 2 is destroyed by step 2, and an async write races
	//    it. EmitSync after the purge transaction has committed is the
	//    only ordering that survives.
	auditErr := h.emit(c, tenantUUID, audit.Event{
		Action: "tenant.purged", ResourceType: "tenant", ResourceID: tenantIDStr,
		Severity: audit.SeverityCritical,
		Metadata: map[string]any{
			"reason_code": req.ReasonCode,
			"reason":      reason,
			"store_slugs": *req.StoreSlugs,
			"store_ids":   res.StoreIDs,
			"tables":      rep.Tables,
			"total_rows":  rep.TotalRows,
			"capability":  c.GetString(CtxCapability),
		},
	})
```

`h.emit` is a `func(*gin.Context, uuid.UUID, audit.Event) error` closing over `EmitSync`, mirroring #287's `lifecycleAuditFunc` — test doubles capture the event synchronously, which the real `*audit.Emitter` cannot be made to do for `Emit`.

Both `purgeErr` and `auditErr` are reported to the operator rather than swallowed: the tenant row is already gone, so a silent 200 would tell the operator a destruction completed and was recorded when neither may be true. A `purgeErr` returns `500 purge_incomplete` **naming the outbox as the retry channel**; an `auditErr` returns `500 purge_unaudited`. Both say plainly that the tenant is gone regardless.

Note the deferred capability gate:

```go
// CapabilityValueChecked records that this surface verifies capability
// PRESENCE but never its VALUE (see middleware.go). #288's acceptance asks
// for "the highest-privilege capability the gateway can assert", which is
// not expressible until the console's capability vocabulary is settled —
// the same blocker as #333. Inventing a value here would refuse every real
// request, which is why #287 declined to invent capability names.
//
// When #333 lands, this is the ONE place to change: flip it and add the
// per-route required value. Until then the value is recorded on every
// audit row and gated nowhere.
const CapabilityValueChecked = false
```

- [ ] **Step 4: Run to green, generate the goldens**

```bash
cd services/marketplace-api
go test -count=1 -v ./internal/handlers/platformadmin/ -run 'TestPurge' 2>&1 | tail -40
echo "exit=$?"
```

- [ ] **Step 5: Prove the golden fixtures catch both mutations**

A fixture that only catches omissions is theatre. Prove **both** directions:

```bash
cd services/marketplace-api
# (a) rename a field: change `json:"total_rows"` to `json:"totalRows"`
go test -count=1 ./internal/handlers/platformadmin/ -run GoldenFixture 2>&1 | tail -5   # MUST FAIL
# revert, then (b) ADD a field to the response struct
go test -count=1 ./internal/handlers/platformadmin/ -run GoldenFixture 2>&1 | tail -5   # MUST FAIL
# revert
```

- [ ] **Step 6: Prove each refusal by deleting its guard**

For each of `confirmation_mismatch`, `tenant_not_found`, `upstream_unavailable`, `invalid_reason_code`, `invalid_request` (absent slugs) and `invalid_tenant_id`: delete the guard, confirm the matching test fails, revert. A refusal that always fires and one that never fires both pass a one-sided test.

- [ ] **Step 7: Prove the ordering assertion is real**

Reorder the handler to emit the audit **before** calling `h.purger.Purge`. `TestPurge_HappyPathTearsDownThenPurgesThenAudits` **must fail**. If it passes, the sequence numbers are not being asserted and the single most important property of this design is untested. Revert.

- [ ] **Step 8: Commit**

```bash
git add services/marketplace-api/internal/handlers/platformadmin/tenant_purge.go \
        services/marketplace-api/internal/handlers/platformadmin/tenant_purge_test.go \
        services/marketplace-api/internal/handlers/platformadmin/testdata/
git commit -m "feat(platformadmin): tenant purge and preview handlers (#288)"
```

---

## Task 11: Mount the routes and wire production

**Files:**
- Modify: `services/marketplace-api/internal/handlers/platformadmin/routes.go`
- Modify: `services/marketplace-api/internal/handlers/platformadmin/routes_test.go` (or a new `routes_tenant_purge_test.go`, matching the file-per-route convention already in that directory)
- Modify: `services/marketplace-api/cmd/marketplace-api/main.go` (the `platformadmin.Register` Deps literal, ~:2100-2120)

**Interfaces:**
- Consumes: `NewTenantPurgeHandler`, `TenantTeardown`, `Purger` (Task 10).
- Produces: `Deps.TenantTeardown`, `Deps.Purger`.

- [ ] **Step 1: Write the failing mount tests**

```go
// Mounted with every dependency present: the routes resolve (not 404) and
// enforce the matrix.
func TestRegister_MountsPurgeRoutesWhenWired(t *testing.T) { /* signed request → not 404 */ }

// A write with no operator is 401 operator_required; with no capability,
// 401 capability_required. Both, not one — they are different cells.
func TestRegister_PurgeRequiresOperatorAndCapability(t *testing.T) { /* ... */ }

// The preview is a READ: signature only, no operator, no capability.
func TestRegister_PreviewDoesNotRequireOperator(t *testing.T) { /* ... */ }

// A handler that cannot audit must not exist on this surface at all —
// #287's rule, and it matters more here.
func TestRegister_DoesNotMountPurgeWithoutAnEmitter(t *testing.T) { /* → 404 */ }
func TestRegister_DoesNotMountPurgeWithoutTeardownOrPurger(t *testing.T) { /* → 404, each nil in turn */ }

// A bogus sibling under the same prefix must stay 404. This is what makes
// "the route is mounted" mean something rather than "this prefix answers".
func TestRegister_BogusSiblingUnderTenantsStays404(t *testing.T) { /* /admin/tenants/{id}/incinerate → 404 */ }

// Trap 2: the whole route tree must still BUILD. A gin wildcard collision
// panics at router build time, not at request time.
func TestRegister_RouterBuildsWithBothTenantRouteSets(t *testing.T) {
	require.NotPanics(t, func() { /* register merchant admin AND platformadmin on one engine */ })
}
```

- [ ] **Step 2: Run and watch them fail**

```bash
cd services/marketplace-api
go test -count=1 -v ./internal/handlers/platformadmin/ -run TestRegister 2>&1 | tail -40
```

- [ ] **Step 3: Extend `Deps` and `Register`**

In `routes.go`, beside `TenantLifecycle`:

```go
	// TenantTeardown and Purger together serve POST /admin/tenants/:id/purge
	// and GET /admin/tenants/:id/purge/preview (#288) — the surface's
	// IRREVERSIBLE endpoint. Both must be non-nil, along with DB and
	// Emitter, for either route to mount.
	//
	// Emitter is required for the same reason it is required by
	// TenantLifecycle, and more so: a purge that cannot be audited is an
	// irreversible destruction with no record, which is the exact gap this
	// series exists to close. An unmounted route is the right failure.
	TenantTeardown TenantTeardown
	Purger         Purger
```

And in `Register`, following the existing guard style:

```go
	if deps.TenantTeardown != nil && deps.Purger != nil && deps.Emitter != nil &&
		deps.DB != nil && deps.TenantDirectory != nil {
		NewTenantPurgeHandler(
			deps.TenantTeardown, deps.Purger, deps.TenantDirectory,
			NewOperatorActionSyncFunc(deps.Emitter), deps.TenantGateInvalidator, deps.Logger,
		).Register(group)
	}
```

`NewOperatorActionSyncFunc` is `EmitOperatorAction`'s synchronous twin. Put it beside `EmitOperatorAction` in `internal/handlers/platformadmin/audit.go`:

```go
// NewOperatorActionSyncFunc adapts a real *audit.Emitter into an
// operatorAuditFunc that writes SYNCHRONOUSLY, via Emitter.EmitSync.
//
// Reach for this ONLY when the action being audited can destroy its own
// audit row — today that is the tenant purge alone (#288), whose plan
// contains DELETE FROM audit_logs WHERE tenant_id = ?. Everywhere else use
// NewOperatorActionAuditFunc: audit must not gate a business request, and
// this one does.
//
// Unlike EmitOperatorAction, a nil emitter is an ERROR rather than a
// warning. That helper tolerates nil for low-level callers with no way to
// know why it is nil; here the caller is a route that Register mounts only
// when deps.Emitter != nil, so a nil emitter reaching this point means the
// mount guard has been broken and an irreversible action is about to go
// unrecorded.
func NewOperatorActionSyncFunc(em *audit.Emitter) operatorAuditFunc {
	return func(c *gin.Context, tenantID uuid.UUID, ev audit.Event) error {
		if tenantID == uuid.Nil {
			return ErrMissingTenant
		}
		if em == nil {
			return errors.New("platformadmin: nil emitter on a synchronous audit path")
		}
		ev.TenantID = tenantID
		return em.EmitSync(c, ev)
	}
}
```

- [ ] **Step 4: Wire `main.go`**

The `platformadmin.Register(engine.Group("/api/v1/platform"), platformadmin.Deps{...})` literal already builds `tenantLifecycleClient` from `cfg.PlatformAPIURL`. Reuse it:

```go
			TenantTeardown: tenantLifecycleClient,
			Purger:         tenantpurge.NewGormPurger(conn),
```

`tenantLifecycleClient` is declared as the `platformadmin.TenantLifecycle` interface, which does not include `Teardown` — declare a second variable of the concrete `*tenantlifecycle.Client` type and assign both from one construction, rather than constructing the client twice.

Add to `internal/tenantpurge`:

```go
// GormPurger binds a *gorm.DB into Purge and Count so consumers can depend
// on a two-method interface of their own rather than on package-level
// functions.
type GormPurger struct{ db *gorm.DB }

func NewGormPurger(db *gorm.DB) *GormPurger { return &GormPurger{db: db} }

func (g *GormPurger) Purge(ctx context.Context, tenantID string, storeIDs []string) (Report, error) {
	return Purge(ctx, g.db, tenantID, storeIDs)
}

func (g *GormPurger) Count(ctx context.Context, tenantID string, storeIDs []string) (Report, error) {
	return Count(ctx, g.db, tenantID, storeIDs)
}
```

It returns the **concrete** `*GormPurger`, NOT `platformadmin.Purger`. `tenantpurge` must never import `platformadmin` — the dependency runs the other way, and this surface's convention (`TenantLifecycle`, `EstateCounts`, `TenantTeardown`) is that the CONSUMER declares the interface it needs. `*GormPurger` satisfies `platformadmin.Purger` structurally.

- [ ] **Step 5: Run everything**

```bash
cd services/marketplace-api
go build ./... && echo "BUILD OK"
go test -count=1 ./... 2>&1 | grep -v "^ok" | head -30
go vet -tags=integration ./... && echo "VET OK"
```

- [ ] **Step 6: Prove the wiring exists and the mount guard bites**

```bash
cd services/marketplace-api
grep -n "TenantTeardown\|Purger" cmd/marketplace-api/main.go internal/handlers/platformadmin/routes.go
```

Both must appear in `main.go`. #323 records five instances of a route silently never mounted, including a nil interface that panicked at runtime. Then delete `TenantTeardown` from the `main.go` literal and confirm `TestRegister_DoesNotMountPurgeWithoutTeardownOrPurger` still passes while a signed request 404s — the guard, not the compiler, is what protects this. Revert.

- [ ] **Step 7: Commit**

```bash
git add services/marketplace-api/internal/handlers/platformadmin/ services/marketplace-api/internal/tenantpurge/ services/marketplace-api/cmd/marketplace-api/main.go
git commit -m "feat(platformadmin): mount tenant purge and preview routes (#288)"
```

---

## Task 12: End-to-end integration

**Files:**
- Create: `services/marketplace-api/internal/handlers/platformadmin/tenant_purge_integration_test.go`

**Interfaces:**
- Consumes: everything above.
- Produces: nothing.

The unit tests proved the handler's logic against fakes. These prove the three things fakes cannot: that the purge actually empties the tables, that the audit row **survives its own purge**, and that a purge scoped to one tenant leaves another tenant's rows untouched.

- [ ] **Step 1: Write the failing tests**

```go
//go:build integration

package platformadmin_test

// Two tenants, each with a store and seeded rows. The property under test
// — that a purge is scoped to one tenant — discriminates between two
// tenants, so the fixture contains two. One tenant cannot prove tenant
// isolation; that is the lesson #286 paid a Critical for.
func TestPurge_Integration_DestroysOneTenantAndLeavesTheOther(t *testing.T) {
	// seed tenant A (2 products, 1 order) and tenant B (3 products)
	// purge A with A's real slugs
	// assert: A's products/orders = 0, A's stores = 0
	// assert: B's products = 3, B's stores = 1
	// assert: the report's counts equal what was seeded, as VALUES
}

// The purge deletes audit_logs WHERE tenant_id = ?. The row recording the
// purge must survive it. BOTH halves are asserted: without the second, the
// test passes against an emitter that wrote nothing at all.
func TestPurge_Integration_AuditRowSurvivesTheDeleteItRecords(t *testing.T) {
	// seed tenant A with 3 PRE-EXISTING audit_logs rows
	// purge A
	// assert: exactly ONE audit_logs row for tenant A remains
	// assert: that row's action is "tenant.purged"
	// assert: actor_type = 'operator', actor_operator_id, capability
	// assert: metadata->>'reason_code' and metadata->'tables' are populated
	// assert: store_id IS NULL (a purge is tenant-scoped)
}

func TestPurge_Integration_PreviewCountsMatchWhatThePurgeThenDestroys(t *testing.T) {
	// seed tenant A
	// GET preview -> report P
	// POST purge   -> report Q
	// assert P.TotalRows == Q.TotalRows and the per-table counts are equal
	// This is the check that makes the preview trustworthy: it is the only
	// evidence that the number an operator reads before an irreversible
	// action is the number that action then destroys.
}

func TestPurge_Integration_MismatchDestroysNothing(t *testing.T) {
	// seed tenant A with rows
	// purge A supplying tenant B's slug
	// assert 409, and A's product/order/store counts are UNCHANGED
}
```

Seed stores via the existing `seedIntegrationStore` helper in this directory (see `tenant_lifecycle_integration_test.go:22`). **Do not hand-write `INSERT INTO stores`.** Note this suite needs a live platform-api or a stub upstream for the teardown call — use an `httptest.Server` standing in for platform-api, and say so in the file's doc comment, along with what that means the test does and does not prove.

- [ ] **Step 2: Run and watch them fail**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 -v ./internal/handlers/platformadmin/ -run TestPurge_Integration 2>&1 | tail -40
```

Confirm from the verbose output that they RAN. `--- SKIP` and `--- PASS` are one character apart in a wall of output, and a test you have never watched run is not a test.

- [ ] **Step 3: Implement the fixtures, run to green**

```bash
cd services/marketplace-api
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 -v ./internal/handlers/platformadmin/ -run TestPurge_Integration 2>&1 | tail -40
echo "exit=$?"
```

- [ ] **Step 4: Prove the isolation test bites**

Change `tenantScoped`'s SQL to `DELETE FROM %s WHERE tenant_id = ? OR true` and re-run. `DestroysOneTenantAndLeavesTheOther` **must fail** on tenant B's rows. Revert.

- [ ] **Step 5: Prove the audit-survival test bites**

Move the `EmitSync` call to **before** `h.purger.Purge` and re-run. `AuditRowSurvivesTheDeleteItRecords` **must fail** — the purge will have deleted the row it just wrote. Revert. This is the mutation that justifies the whole ordering decision; if it passes, the test is not testing it.

- [ ] **Step 6: Full verification set, both services**

```bash
cd services/marketplace-api
go build ./... ; echo "build=$?"
go test -count=1 ./... 2>&1 | grep -v "^ok" | head -30
go vet -tags=integration ./... ; echo "vet=$?"

cd ../platform-api
go build ./... ; echo "build=$?"
go test -count=1 ./... 2>&1 | grep -v "^ok" | head -30
go vet -tags=integration ./... ; echo "vet=$?"
```

`go test ./...` from each **service root**, never path-scoped — the schema-version guard lives in the root package and a path-scoped run silently excludes it. Read every `echo` value; a `&&` chain that aborts still prints a trailing line.

Anything failing must be matched against the known pre-existing set (`internal/billing/trial` #317, `internal/subscription/planchange`, `internal/whitelabel`). Confirm each at `origin/main` in a clean worktree before calling it pre-existing — do not assume.

- [ ] **Step 7: Commit**

```bash
git add services/marketplace-api/internal/handlers/platformadmin/tenant_purge_integration_test.go
git commit -m "test(platformadmin): end-to-end coverage for tenant purge (#288)"
```

---

## Task 13: File the two out-of-scope findings

**Files:** none — GitHub issues.

These were found while verifying #288's premises and are deliberately not fixed here. Filing them is part of the work: an unfiled finding is a finding that evaporates.

- [ ] **Step 1: Re-measure before filing**

```bash
PGPOD=$(kubectl get pods -n mark8ly \
  -l cnpg.io/cluster=mark8ly-postgres,cnpg.io/instanceRole=primary \
  -o jsonpath='{.items[0].metadata.name}')
kubectl exec -n mark8ly "$PGPOD" -c postgres -- psql -U postgres -d mark8ly_marketplace_api -tAF'|' -c "
SELECT table_name, string_agg(column_name, ',' ORDER BY column_name)
FROM information_schema.columns
WHERE table_schema='public'
  AND table_name IN ('review_reactions','review_replies','review_media','loyalty_transactions',
                     'campaign_recipients','gift_card_transactions','coupon_usage',
                     'product_categories','ticket_replies')
  AND column_name IN ('store_id','tenant_id')
GROUP BY table_name ORDER BY table_name;"
```

Expect none of them to have `store_id`. If any does, correct the issue body to what you measured.

- [ ] **Step 2: File the sweeper issue**

Title: `harddelete.Sweep cannot complete — nine tables swept by a store_id column they do not have`

Body must state: the nine table names; that `sweepTable` builds `DELETE FROM <t> WHERE store_id = ?` unconditionally; that the sweep list reaches `review_reactions` fourth, so it aborts on every run and the 150-day hard-delete pipeline has never completed; that `outbox_events` and `idempotency_keys` are swept by `tenant_id` inside a per-**store** sweep, which would delete a sibling store's rows if it ever got that far; and that `subscription_plan_change_audit` is swept here while `tenantpurge` excludes it as append-only — the two enumerations disagree about a compliance table. Note what came closest to disconfirming "it never runs": it is wired (`main.go:1822` → `lifecycle.NewHardDeleteCron`), so the code path is live; it is the SQL that cannot succeed.

- [ ] **Step 3: File the FGA orphan issue**

Title: `Tenant teardown orphans staff FGA tuples and GIP identities`

Body: `authz.Client` exposes no member enumeration (`DeleteTuple` requires a `userID`), so both the merchant `deleteOwnerAccount` path and #288's operator path clean only the owner's tuple, the store-parent tuples and the owner's GIP identity. Staff/admin/viewer tuples survive against a deleted tenant object, as do those users' GIP identities. Pre-existing, not introduced by #288. Practically inert while tenant ids are UUIDs and never reused — say so, and say that this is why it is a cleanup issue rather than a security one.

- [ ] **Step 4: Cross-reference**

Link both from #288 and from #260's next status comment.

---

## Rollout and verification

Not a task — the controller runs this after merge, and it is the report that goes on #260.

**Measure before the rollout**, so "the route is mounted" means something:

```bash
BASE=https://api.mark8ly.com/api/v1/platform
T=00000000-0000-0000-0000-000000000000
for p in "admin/tenants/$T/purge" "admin/tenants/$T/purge/preview" "admin/tenants/$T/incinerate" "admin/billing/trials"; do
  printf '%-45s %s\n' "$p" "$(curl -s -o /dev/null -w '%{http_code}' -X POST "$BASE/$p")"
done
```

**After** the Kargo promotion lands (`kubectl get stages,promotions -n kargo-mark8ly`; watch for image tag `main-<sha7>`, never a git commit), run the identical loop. Expected:

| path | before | after |
|---|---|---|
| `admin/tenants/{id}/purge` | 404 | **401** |
| `admin/tenants/{id}/purge/preview` | 404 | **401** |
| `admin/tenants/{id}/incinerate` | 404 | 404 |
| `admin/billing/trials` | 401 | 401 |

Exactly two of four move. Then confirm the body says `unauthenticated`, not `not_configured` — the latter means the secret is unset and the surface is inert.

Both deployments must reach `main-<sha7>` with 0 restarts. No migration ships, so `ExpectedSchemaVersion` does not move and the storefront/admin initContainer skew does not apply this round.

**Then run the preview against a real tenant with a signed request** — this is the one check in this milestone that exercises a handler body against production data, and it destroys nothing.

**State both halves in the report.** Data-independent: routing, mounting, signature refusal, operator/capability refusal, UUID and reason-code validation. Genuinely exercised: the preview's enumeration and counts against real tenants. **Not provable**: the purge itself, the confirmation mismatch, the concurrency mutex, the audit-survives-its-own-purge property. There is no scratch tenant and every one of the four is a live merchant. An empty `200` is not a passing integration check, and neither is a `401` from a route whose body has never run.
