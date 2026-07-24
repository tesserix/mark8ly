# Mobile-Admin Account Deletion Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give a signed-in mobile-admin user an in-app "Delete account" flow that genuinely deletes their identity and tears down (owner) or detaches (staff) their tenant — satisfying Apple 5.1.1(v) and Google Play's account-deletion requirement.

**Architecture:** Mobile app confirms + refreshes the GIP token, then calls a new tenant-scoped `DELETE /api/v1/mobile/admin/account` on **marketplace-api**. That handler proxies (existing `teamproxy` / `X-Internal-Auth` pattern) to a new `DELETE /internal/tenants/:id/account` on **platform-api**, which is authoritative: it resolves the actor's role, then for an **owner** hard-deletes the tenant (cascading stores/invitations, reconciling onboarding_sessions first) and for a **staff** member removes only their membership — in both cases deleting the GIP user and the OpenFGA tuples. The heavy cross-service purge of marketplace-api domain data (products/orders/etc.) is a **documented Phase 5 follow-up**, done async within the grace window; the MVP (Phases 1–4) is what unblocks store submission.

**Tech Stack:** Go 1.26 (Gin, GORM, OpenFGA go-sdk, Identity Toolkit REST), Next.js/Expo React Native + TypeScript, TanStack Query, jest-expo, Go `testing`.

## Global Constraints

- **Go module paths:** platform-api = `github.com/mark8ly/platform-api`; marketplace-api is the sibling module. Both are in the root `go.work` (`/Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly/go.work`).
- **GOWORK=off rule:** Docker/CI builds use `GOWORK=off` + complete `go.sum`. This plan adds **no new third-party imports** (reuses `gipadmin`, `authz`, `teamproxy`, `gin`, `gorm`, `openfga/go-sdk`). If any task adds an import, that task MUST end with `cd services/<svc> && GOWORK=off go mod tidy`.
- **platform-api schema version:** `platformapi.ExpectedSchemaVersion` (`services/platform-api/migrations.go:14`) is asserted at boot. This plan adds **no migration** (hard-delete uses existing tables). Do NOT bump it.
- **Run tests from the module dir** (the `//go:embed migrations/*.sql` requires it): `cd services/platform-api && go test ./...`, `cd services/marketplace-api && go test ./...`.
- **Mobile tests:** `cd apps/mobile-admin && npx jest`. jest-expo defaults `Platform.OS = "ios"`.
- **Immutability / style:** no in-place mutation of shared structs; small focused files; follow existing patterns in each file.
- **Git workflow (mark8ly + services):** commit directly to `main`, **single-line** conventional-commit messages, **no signature/attribution**. Commit per task.
- **Idempotency:** every delete (GIP user, FGA tuple, DB row) must treat "already gone" as success — deletions get retried.
- **MVP scope:** Phases 1–4 ship the compliant initiate-and-delete flow. Phase 5 (marketplace-api domain purge + tenant-status enforcement) is specified but built after MVP validation.

---

## File Structure

**platform-api** (`services/platform-api/`)
- `internal/gipadmin/delete.go` — **new**: `AdminClient.DeleteAccount(ctx, uid)` (accounts:delete).
- `internal/gipadmin/delete_test.go` — **new**: unit test.
- `internal/authz/authz.go` — **modify**: add `DeleteTuple` + `DeleteStoreParent` to `Client` iface + `fgaClient`.
- `internal/authz/fake.go` — **modify**: implement the two new methods.
- `internal/authz/authz_test.go` (or existing test file) — **modify/new**: cover delete idempotency against the fake.
- `internal/account/` — **new package**: `service.go` (teardown orchestration), `handler.go` (HTTP), `service_test.go`, `handler_test.go`.
- `internal/tenant/repository.go` — **modify**: add `DeleteInTx` + `ReconcileOnboardingForDelete` (or expose via a small store method the account service calls).
- `cmd/server/main.go` — **modify**: construct the account service + register `DELETE /internal/tenants/:id/account`.

**marketplace-api** (`services/marketplace-api/`)
- `internal/teamproxy/client.go` — **modify**: add `DeleteTenantAccount(ctx, tenantID, actorUID)`.
- `internal/handlers/admin/account.go` — **new**: `AccountHandler` + `Delete`.
- `internal/handlers/admin/account_test.go` — **new**.
- `internal/handlers/admin/mobile_routes.go` — **modify**: add `AccountHandler` to `MobileDeps` + register route.
- `cmd/.../main.go` — **modify**: build `AccountHandler` from the shared `*teamproxy.Client`.

**mobile** (`apps/mobile-admin/`, `packages/mobile-shared/`)
- `packages/mobile-shared/api/client.ts` — **modify**: add `deleteTenant<T>(path)` helper.
- `packages/mobile-shared/api/account.ts` — **new**: `createAccountApi(client)` → `deleteAccount()`.
- `packages/mobile-shared/api/demo-*` / `apps/mobile-admin/lib/demo-api-client.ts` — **modify**: demo stub for `deleteTenant`.
- `apps/mobile-admin/lib/admin-api/account-actions.ts` — **new**: `useDeleteAccount()` hook.
- `apps/mobile-admin/app/(tabs)/more/account.tsx` — **modify**: "Delete account" button + typed-confirm screen/dialog.
- `apps/mobile-admin/__tests__/account-delete.test.tsx` — **new**: screen test.
- `apps/mobile-admin/__tests__/use-delete-account.test.tsx` — **new**: hook test.

---

## Phase 1 — platform-api primitives (GIP delete + FGA tuple delete)

### Task 1: `gipadmin.DeleteAccount`

**Files:**
- Create: `services/platform-api/internal/gipadmin/delete.go`
- Test: `services/platform-api/internal/gipadmin/delete_test.go`

**Interfaces:**
- Consumes: existing `AdminClient.postAdmin(ctx, method, body)` (`internal/gipadmin/claims.go:135`), sentinel `ErrUserNotFound` (`client.go:37`).
- Produces: `func (c *AdminClient) DeleteAccount(ctx context.Context, uid string) error` — deletes the GIP user in pool `MP-Internal-e986p`; returns `nil` if the account is already gone.

- [ ] **Step 1: Write the failing test**

`delete_test.go` — spin an `httptest` server, point the client's Identity Toolkit base at it (the existing tests in this package show how they stub `doAdmin`; mirror whatever `claims_test.go` does — if it injects via an unexported base URL field, reuse it; otherwise assert `DeleteAccount` returns nil on a 200 and nil on a `USER_NOT_FOUND` body). Minimum two cases:

```go
func TestDeleteAccount_SucceedsOn200(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/accounts:delete") {
			t.Fatalf("path = %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	})
	if err := c.DeleteAccount(context.Background(), "uid-1"); err != nil {
		t.Fatalf("err = %v", err)
	}
}

func TestDeleteAccount_IdempotentOnUserNotFound(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"USER_NOT_FOUND"}}`))
	})
	if err := c.DeleteAccount(context.Background(), "uid-1"); err != nil {
		t.Fatalf("expected nil on USER_NOT_FOUND, got %v", err)
	}
}

func TestDeleteAccount_RejectsEmptyUID(t *testing.T) {
	c := &AdminClient{}
	if err := c.DeleteAccount(context.Background(), ""); err == nil {
		t.Fatal("expected error for empty uid")
	}
}
```

> If `newTestClient` does not already exist in the package's tests, read `internal/gipadmin/claims_test.go` and copy its client-construction/HTTP-stub helper verbatim into `delete_test.go` (or a shared `helpers_test.go`). Do NOT invent a new stubbing mechanism.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/platform-api && go test ./internal/gipadmin/ -run TestDeleteAccount -v`
Expected: FAIL — `c.DeleteAccount undefined`.

- [ ] **Step 3: Write minimal implementation**

`delete.go`:
```go
package gipadmin

import (
	"context"
	"errors"
	"fmt"
)

// DeleteAccount removes the GIP user identified by uid from the configured
// tenant pool. It is idempotent: a missing account (USER_NOT_FOUND) is treated
// as success, since account deletion is retried and the user may already be
// gone. Deleting the GIP user invalidates all of that user's tokens.
func (c *AdminClient) DeleteAccount(ctx context.Context, uid string) error {
	if uid == "" {
		return fmt.Errorf("gipadmin: uid is required")
	}
	err := c.postAdmin(ctx, "accounts:delete", map[string]any{"localId": uid})
	if errors.Is(err, ErrUserNotFound) {
		return nil
	}
	return err
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd services/platform-api && go test ./internal/gipadmin/ -run TestDeleteAccount -v`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add services/platform-api/internal/gipadmin/delete.go services/platform-api/internal/gipadmin/delete_test.go
git commit -m "feat(platform-api): add gipadmin DeleteAccount (accounts:delete, idempotent)"
```

---

### Task 2: FGA tuple-delete methods

**Files:**
- Modify: `services/platform-api/internal/authz/authz.go` (interface `Client` ~lines 77-126; `fgaClient` methods near the writes at ~193-347)
- Modify: `services/platform-api/internal/authz/fake.go`
- Test: `services/platform-api/internal/authz/authz_test.go` (create if absent)

**Interfaces:**
- Consumes: `client.OpenFgaClient` (`authz.go:129`), the existing `write`/`WriteStoreParent` shapes (`authz.go:328-347`, `198-218`), `isAlreadyExistsError` (`authz.go:349-363`).
- Produces on the `Client` interface:
  - `DeleteTuple(ctx context.Context, userID, relation, tenantID string) error` — deletes `user:<uid> <relation> tenant:<tid>`.
  - `DeleteStoreParent(ctx context.Context, storeID, tenantID string) error` — deletes `tenant:<tid> parent store:<sid>`.
  Both idempotent (deleting a missing tuple returns nil).

- [ ] **Step 1: Write the failing test**

`authz_test.go` (uses the in-memory `fake.go`, so this also pins the fake's behavior):
```go
func TestFake_DeleteTuple_Idempotent(t *testing.T) {
	f := authz.NewFake()
	ctx := context.Background()
	if err := f.WriteOwnership(ctx, "u1", "t1"); err != nil {
		t.Fatal(err)
	}
	if err := f.DeleteTuple(ctx, "u1", "owner", "t1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	ok, _ := f.Check(ctx, "u1", "owner", "t1")
	if ok {
		t.Fatal("tuple still present after delete")
	}
	// Deleting again is a no-op, not an error.
	if err := f.DeleteTuple(ctx, "u1", "owner", "t1"); err != nil {
		t.Fatalf("second delete should be nil, got %v", err)
	}
}
```
> Confirm the fake constructor name by reading `fake.go` (it may be `NewFake`, `NewFakeClient`, etc.) and use the real one.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/platform-api && go test ./internal/authz/ -run TestFake_DeleteTuple -v`
Expected: FAIL — `DeleteTuple` not on the interface / fake.

- [ ] **Step 3: Write minimal implementation**

Add to the `Client` interface in `authz.go`:
```go
	// DeleteTuple removes user:<userID> <relation> tenant:<tenantID>. Idempotent.
	DeleteTuple(ctx context.Context, userID, relation, tenantID string) error
	// DeleteStoreParent removes tenant:<tenantID> parent store:<storeID>. Idempotent.
	DeleteStoreParent(ctx context.Context, storeID, tenantID string) error
```
Add to `fgaClient` (mirror `write`, but `Deletes` with `ClientTupleKeyWithoutCondition`):
```go
func (c *fgaClient) DeleteTuple(ctx context.Context, userID, relation, tenantID string) error {
	body := client.ClientWriteRequest{
		Deletes: []client.ClientTupleKeyWithoutCondition{{
			User:     "user:" + userID,
			Relation: relation,
			Object:   "tenant:" + tenantID,
		}},
	}
	_, err := c.api.Write(ctx).Body(body).Execute()
	if err != nil {
		if isAlreadyExistsError(err) { // missing tuple → validation error → treat as done
			return nil
		}
		return fmt.Errorf("authz: delete %s tuple: %w", relation, err)
	}
	return nil
}

func (c *fgaClient) DeleteStoreParent(ctx context.Context, storeID, tenantID string) error {
	body := client.ClientWriteRequest{
		Deletes: []client.ClientTupleKeyWithoutCondition{{
			User:     "tenant:" + tenantID,
			Relation: "parent",
			Object:   "store:" + storeID,
		}},
	}
	_, err := c.api.Write(ctx).Body(body).Execute()
	if err != nil {
		if isAlreadyExistsError(err) {
			return nil
		}
		return fmt.Errorf("authz: delete store parent: %w", err)
	}
	return nil
}
```
> Verify `client.ClientTupleKeyWithoutCondition` is the correct delete-tuple type in the vendored `openfga/go-sdk` (grep the SDK). If the SDK version uses a different name, use that; do not add a new dependency.

Add the two methods to `fake.go` (mutate its internal tuple set to remove the matching key; return nil if absent).

- [ ] **Step 4: Run test to verify it passes**

Run: `cd services/platform-api && go test ./internal/authz/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add services/platform-api/internal/authz/
git commit -m "feat(platform-api): add idempotent FGA DeleteTuple + DeleteStoreParent"
```

---

## Phase 2 — platform-api account-teardown endpoint

### Task 3: tenant repository delete + onboarding reconciliation

**Files:**
- Modify: `services/platform-api/internal/tenant/repository.go`
- Test: `services/platform-api/internal/tenant/repository_integration_test.go` (build-tagged; extend if present) OR a service-level test with the fake in Task 4.

**Interfaces:**
- Consumes: GORM `*gorm.DB`, existing `Tenant` model, `apperrors`.
- Produces on the tenant `Repository` interface:
  - `ListStoreIDs(ctx, tx, tenantID) ([]string, error)` — store IDs under a tenant (needed to delete FGA store-parent tuples before the DB cascade removes the rows).
  - `DeleteInTx(ctx, tx, tenantID) error` — reconciles completed `onboarding_sessions` then deletes the tenant row (stores + invitations cascade at the DB level).

- [ ] **Step 1: Write the failing test** (build-tagged integration, needs the `make dev` Postgres)

```go
//go:build integration

func TestDeleteInTx_ReconcilesOnboardingThenDeletes(t *testing.T) {
	db := newTestDB(t) // reuse the existing integration harness in this file
	repo := NewRepository(db)
	ctx := context.Background()
	// seed tenant + completed onboarding_session referencing it
	// (reuse existing seed helpers if present)
	seedTenantWithCompletedOnboarding(t, db, "t-del")

	err := db.Transaction(func(tx *gorm.DB) error {
		return repo.DeleteInTx(ctx, tx, "t-del")
	})
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	var n int64
	db.Raw(`SELECT count(*) FROM tenants WHERE id = ?`, "t-del").Scan(&n)
	if n != 0 {
		t.Fatalf("tenant still present")
	}
}
```
> If no integration harness exists in the file, instead prove reconciliation logic in the Task 4 service test with the fake repo, and keep `DeleteInTx` thin. Do not stand up a new DB harness from scratch for the MVP.

- [ ] **Step 2: Run** `cd services/platform-api && go test -tags=integration ./internal/tenant/ -run TestDeleteInTx -v` → FAIL (`DeleteInTx` undefined).

- [ ] **Step 3: Implement** in `repository.go`:
```go
func (r *gormRepository) ListStoreIDs(ctx context.Context, tx *gorm.DB, tenantID string) ([]string, error) {
	db := r.db
	if tx != nil {
		db = tx
	}
	var ids []string
	if err := db.WithContext(ctx).
		Table("stores").Where("tenant_id = ?", tenantID).Pluck("id", &ids).Error; err != nil {
		return nil, fmt.Errorf("tenant: list store ids: %w", err)
	}
	return ids, nil
}

func (r *gormRepository) DeleteInTx(ctx context.Context, tx *gorm.DB, tenantID string) error {
	// onboarding_sessions.tenant_id is ON DELETE SET NULL, but the
	// onboarding_sessions_completed_consistency CHECK requires tenant_id
	// NOT NULL when status='completed'. Deleting the tenant would null it and
	// violate the CHECK. Delete the completed session rows first; their
	// verification codes cascade (ON DELETE CASCADE).
	if err := tx.WithContext(ctx).
		Exec(`DELETE FROM onboarding_sessions WHERE tenant_id = ?`, tenantID).Error; err != nil {
		return fmt.Errorf("tenant: reconcile onboarding_sessions: %w", err)
	}
	// stores + invitations FK to tenants ON DELETE CASCADE — removed automatically.
	res := tx.WithContext(ctx).Exec(`DELETE FROM tenants WHERE id = ?`, tenantID)
	if res.Error != nil {
		return fmt.Errorf("tenant: delete: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return apperrors.NotFound("tenant_not_found", fmt.Sprintf("tenant %q does not exist", tenantID))
	}
	return nil
}
```
Add both to the `Repository` interface. If the interface has a mock/fake used elsewhere, extend it.

- [ ] **Step 4: Run** the test → PASS (or defer to Task 4 per the note).

- [ ] **Step 5: Commit**
```bash
git add services/platform-api/internal/tenant/repository.go services/platform-api/internal/tenant/*_test.go
git commit -m "feat(platform-api): tenant DeleteInTx with onboarding_sessions reconciliation"
```

---

### Task 4: account teardown service (owner vs staff branching)

**Files:**
- Create: `services/platform-api/internal/account/service.go`
- Test: `services/platform-api/internal/account/service_test.go`

**Interfaces:**
- Consumes: `authz.Client` (`GetRole`, `DeleteTuple`, `DeleteStoreParent`, `Write... ` not needed), `gipadmin.AdminClient.DeleteAccount`, tenant `Repository` (`GetByID`, `ListStoreIDs`, `DeleteInTx`), `*gorm.DB`, and the membership store for staff removal (reuse whatever platform-api uses for team membership — read `internal/invitation`/`internal/membership`; the agent noted membership writes live in platform-api).
- Produces:
  - `type Service struct { ... }` with `NewService(db, repo, fga, gip, memberships, outbox, log)`.
  - `func (s *Service) DeleteAccount(ctx, tenantID, actorUID string) error` — resolves role; **owner** → full teardown; **admin/staff/viewer** → membership removal only; unknown role → `apperrors.Forbidden`.

Behavior (owner):
1. `role := fga.GetRole(ctx, actorUID, tenantID)`; if `""` → Forbidden.
2. If `role == authz.RoleOwner`:
   - `storeIDs := repo.ListStoreIDs(ctx, nil, tenantID)`.
   - In a `db.Transaction`: `repo.DeleteInTx(tx, tenantID)`; `outbox.Enqueue(tx, "tenant.deleted", {tenant_id, store_ids})` (for the Phase 5 marketplace purge).
   - After commit (best-effort, log-on-error, each idempotent): `fga.DeleteTuple(ctx, actorUID, "owner", tenantID)`; for each store `fga.DeleteStoreParent(ctx, storeID, tenantID)`; `gip.DeleteAccount(ctx, actorUID)`.
3. Else (staff): remove the membership row + `fga.DeleteTuple(ctx, actorUID, string(role), tenantID)` + `gip.DeleteAccount(ctx, actorUID)`. Tenant untouched.

> GIP + FGA cleanup are **post-commit best-effort** so a GIP hiccup never aborts the DB teardown; the enqueued `tenant.deleted` event is the durable retry channel for the marketplace purge. Log every best-effort failure at WARN.

- [ ] **Step 1: Write the failing test** (uses fakes — no DB): assert owner path calls repo delete + enqueues event + deletes GIP user + owner tuple; staff path leaves tenant, removes membership + tuple + GIP user; unknown role → Forbidden. Sketch:
```go
func TestDeleteAccount_Owner_TearsDownTenant(t *testing.T) {
	fga := authz.NewFake(); _ = fga.WriteOwnership(context.Background(), "owner-1", "t1")
	gip := &fakeGIP{}
	repo := &fakeTenantRepo{stores: map[string][]string{"t1": {"s1"}}}
	ob := &fakeOutbox{}
	svc := NewService(nil, repo, fga, gip, &fakeMembers{}, ob, log)
	if err := svc.DeleteAccount(context.Background(), "t1", "owner-1"); err != nil {
		t.Fatal(err)
	}
	if !repo.deleted["t1"] { t.Error("tenant not deleted") }
	if !gip.deleted["owner-1"] { t.Error("gip user not deleted") }
	if !ob.has("tenant.deleted") { t.Error("tenant.deleted not enqueued") }
}

func TestDeleteAccount_Staff_RemovesMembershipOnly(t *testing.T) { /* ... tenant survives ... */ }
func TestDeleteAccount_UnknownRole_Forbidden(t *testing.T) { /* GetRole "" → apperrors Forbidden */ }
```
> `fakeGIP` = a local test double with method `DeleteAccount(ctx, uid) error`; define a small interface `gipDeleter interface{ DeleteAccount(context.Context, string) error }` in `service.go` so `*gipadmin.AdminClient` satisfies it and the fake is trivial. Same for outbox + memberships.

- [ ] **Step 2: Run** `cd services/platform-api && go test ./internal/account/ -v` → FAIL.
- [ ] **Step 3: Implement** `service.go` per the behavior above (define the small consumer interfaces so the service is unit-testable without a DB; for the owner DB step, accept a `*gorm.DB` and skip the tx when nil in tests by having the repo fake short-circuit — or split the DB tx into a repo method `TeardownTenant(ctx, tenantID, storeIDs)` the fake overrides).
- [ ] **Step 4: Run** → PASS.
- [ ] **Step 5: Commit**
```bash
git add services/platform-api/internal/account/
git commit -m "feat(platform-api): account teardown service with owner/staff branching"
```

---

### Task 5: platform-api internal endpoint + wiring

**Files:**
- Create: `services/platform-api/internal/account/handler.go`
- Test: `services/platform-api/internal/account/handler_test.go`
- Modify: `services/platform-api/cmd/server/main.go`

**Interfaces:**
- Consumes: `account.Service`, gin, `respondError` envelope pattern (`internal/tenant/handler.go:232`).
- Produces: `DELETE /internal/tenants/:id/account` with JSON body `{"uid": "<actorUID>"}`; 204 on success; `respondError` for typed errors (Forbidden→403, NotFound→404).

- [ ] **Step 1: Write the failing test** (gin `httptest`, fake service): 204 on success; body-missing-uid → 400; service Forbidden → 403.
```go
func TestAccountHandler_Delete_204(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := account.NewHandler(&fakeSvc{})
	r := gin.New()
	grp := r.Group("/internal")
	h.Register(grp) // registers /tenants/:id/account
	rec := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/internal/tenants/t1/account",
		strings.NewReader(`{"uid":"owner-1"}`))
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
}
```
- [ ] **Step 2: Run** `cd services/platform-api && go test ./internal/account/ -run TestAccountHandler -v` → FAIL.
- [ ] **Step 3: Implement** `handler.go`:
```go
package account

type deleteRequest struct {
	UID string `json:"uid"`
}

func (h *Handler) delete(c *gin.Context) {
	var req deleteRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.UID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing_uid", "message": "uid is required"})
		return
	}
	if err := h.svc.DeleteAccount(c.Request.Context(), c.Param("id"), req.UID); err != nil {
		respondError(c, err) // reuse the tenant package's mapping or copy it
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) Register(internal *gin.RouterGroup) {
	internal.Group("/tenants").DELETE("/:id/account", h.delete)
}
```
> `respondError` currently lives in `internal/tenant`. Either export a shared `apperrors`→gin responder (preferred: a tiny `internal/httperr` helper) or copy the small mapping into `account/handler.go`. Keep it DRY if a shared helper already exists.

Wire in `cmd/server/main.go` next to `tenantHandler.Register(v1, internal)`:
```go
accountSvc := account.NewService(db, tenantRepo, fga, gipAdmin, membershipStore, outboxEnqueuer, logger)
accountHandler := account.NewHandler(accountSvc)
accountHandler.Register(internal)
```
- [ ] **Step 4: Run** `cd services/platform-api && go test ./internal/account/... && go build ./...` → PASS + builds.
- [ ] **Step 5: Commit**
```bash
git add services/platform-api/internal/account/handler.go services/platform-api/internal/account/handler_test.go services/platform-api/cmd/server/main.go
git commit -m "feat(platform-api): DELETE /internal/tenants/:id/account endpoint"
```

---

## Phase 3 — marketplace-api mobile endpoint (proxy)

### Task 6: teamproxy `DeleteTenantAccount`

**Files:**
- Modify: `services/marketplace-api/internal/teamproxy/client.go`
- Test: `services/marketplace-api/internal/teamproxy/client_test.go` (extend/create)

**Interfaces:**
- Consumes: existing `Client.do` (`teamproxy/client.go:86`).
- Produces: `func (c *Client) DeleteTenantAccount(ctx context.Context, tenantID, actorUID string) error` → `DELETE /internal/tenants/<tenantID>/account` with body `{"uid": actorUID}`.

- [ ] **Step 1: Write the failing test** — `httptest` server asserts method DELETE, path `/internal/tenants/t1/account`, header `X-Internal-Auth: secret`, body `{"uid":"u1"}`; returns 204.
```go
func TestDeleteTenantAccount_SendsAuthAndBody(t *testing.T) {
	var gotAuth, gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("X-Internal-Auth"); gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body); gotBody = string(b)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	c := NewClient(srv.URL, "secret", nil)
	if err := c.DeleteTenantAccount(context.Background(), "t1", "u1"); err != nil {
		t.Fatal(err)
	}
	if gotAuth != "secret" || gotPath != "/internal/tenants/t1/account" || !strings.Contains(gotBody, `"u1"`) {
		t.Fatalf("auth=%q path=%q body=%q", gotAuth, gotPath, gotBody)
	}
}
```
- [ ] **Step 2: Run** `cd services/marketplace-api && go test ./internal/teamproxy/ -run TestDeleteTenantAccount -v` → FAIL.
- [ ] **Step 3: Implement**:
```go
// DeleteTenantAccount asks platform-api to delete the actor's account. For an
// owner this tears down the tenant; for staff it removes only their membership.
// platform-api is authoritative for the owner-vs-staff decision.
func (c *Client) DeleteTenantAccount(ctx context.Context, tenantID, actorUID string) error {
	body := map[string]string{"uid": actorUID}
	return c.do(ctx, http.MethodDelete, "/internal/tenants/"+tenantID+"/account", body, nil)
}
```
- [ ] **Step 4: Run** → PASS.
- [ ] **Step 5: Commit**
```bash
git add services/marketplace-api/internal/teamproxy/
git commit -m "feat(marketplace-api): teamproxy DeleteTenantAccount"
```

---

### Task 7: `AccountHandler` + mobile route

**Files:**
- Create: `services/marketplace-api/internal/handlers/admin/account.go`
- Test: `services/marketplace-api/internal/handlers/admin/account_test.go`
- Modify: `services/marketplace-api/internal/handlers/admin/mobile_routes.go`
- Modify: marketplace-api `cmd/.../main.go` (build the handler from the existing `*teamproxy.Client`)

**Interfaces:**
- Consumes: `*teamproxy.Client` (shared with `TeamHandler`), context keys `user_id` + `tenant_id` (`gip_bearer.go:67`), the proxy-error responder pattern from `team.go:35-56`.
- Produces: `AccountHandler.Delete(c)` → reads `tenant_id`+`user_id`, calls `client.DeleteTenantAccount`, forwards platform-api errors, returns 204.

- [ ] **Step 1: Write the failing test** (mirror `platform_support_test.go`): fake platform-api returns 204 → handler returns 204 and forwarded `X-Internal-Auth`; fake returns 403 → handler returns 403 with the code.
```go
func TestAccount_Delete_Proxies204(t *testing.T) {
	var sawAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("X-Internal-Auth"); w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	gin.SetMode(gin.TestMode)
	h := NewAccountHandler(teamproxy.NewClient(srv.URL, "secret", nil), nil)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("user_id", "u1"); c.Set("tenant_id", "t1"); c.Next() })
	r.DELETE("/account", h.Delete)
	rec := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/account", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent || sawAuth != "secret" {
		t.Fatalf("code=%d auth=%q", rec.Code, sawAuth)
	}
}
```
- [ ] **Step 2: Run** `cd services/marketplace-api && go test ./internal/handlers/admin/ -run TestAccount_Delete -v` → FAIL.
- [ ] **Step 3: Implement** `account.go`:
```go
package admin

type AccountHandler struct {
	client *teamproxy.Client
	logger *slog.Logger
}

func NewAccountHandler(client *teamproxy.Client, logger *slog.Logger) *AccountHandler {
	return &AccountHandler{client: client, logger: logger}
}

// Delete removes the calling user's account. Owner → tenant teardown; staff →
// membership removal. platform-api decides based on the actor UID.
func (h *AccountHandler) Delete(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	actorUID := c.GetString("user_id")
	if err := h.client.DeleteTenantAccount(c.Request.Context(), tenantID, actorUID); err != nil {
		// forward platform-api APIError status/code; copy team.go:35-56 respondErr
		respondProxyErr(c, err, h.logger)
		return
	}
	c.Status(http.StatusNoContent)
}
```
> Copy the `respondErr` proxy-forwarding helper from `team.go:35-56` (rename to `respondProxyErr` or reuse if it's already package-level).

Register in `mobile_routes.go` next to the tenant-scoped `platform-support` group (NOT store-scoped):
```go
if deps.AccountHandler != nil {
	acct := router.Group("/mobile/admin/account", bearerAuth, requireTenant, rateLimiter)
	acct.DELETE("", deps.AccountHandler.Delete)
}
```
Add `AccountHandler *AccountHandler` to `MobileDeps`, and in `main.go` build it: `AccountHandler: admin.NewAccountHandler(teamClient, logger)` (reuse the `*teamproxy.Client` already constructed for `TeamHandler`). **No new config/env — reuses `MARKETPLACE_PLATFORM_API_URL` + `MARKETPLACE_PLATFORM_API_SECRET`.**

> Do NOT add an FGA route gate: any tenant member must be able to delete their own account (Apple requires staff too), and platform-api is authoritative on owner-vs-staff. `requireTenant` already ensures the caller is bound to a tenant.

- [ ] **Step 4: Run** `cd services/marketplace-api && go test ./internal/handlers/admin/... && go build ./...` → PASS + builds.
- [ ] **Step 5: Commit**
```bash
git add services/marketplace-api/internal/handlers/admin/account.go services/marketplace-api/internal/handlers/admin/account_test.go services/marketplace-api/internal/handlers/admin/mobile_routes.go services/marketplace-api/cmd/
git commit -m "feat(marketplace-api): DELETE /mobile/admin/account proxy endpoint"
```

---

## Phase 4 — mobile UI

### Task 8: API client `deleteTenant` helper + account API

**Files:**
- Modify: `packages/mobile-shared/api/client.ts` (helpers block ~231-242)
- Create: `packages/mobile-shared/api/account.ts`
- Modify: `apps/mobile-admin/lib/demo-api-client.ts` (demo stub)
- Test: `packages/mobile-shared/api/__tests__/` or `apps/mobile-admin/__tests__/` per where api-client tests live

**Interfaces:**
- Consumes: `request<T>("DELETE", path, { tenantScope: true })` (client.ts:181-199 routing).
- Produces: `client.deleteTenant<T>(path)`; `createAccountApi(client).deleteAccount()` → `DELETE /mobile/admin/account` (tenant-scoped, returns 204/undefined).

- [ ] **Step 1: Write the failing test** — assert `createAccountApi(fakeClient).deleteAccount()` calls `deleteTenant("/account")`.
- [ ] **Step 2: Run** `cd apps/mobile-admin && npx jest account` → FAIL.
- [ ] **Step 3: Implement**. In `client.ts` helpers, mirror `getTenant`:
```ts
    deleteTenant: <T>(path: string): Promise<T> =>
      request<T>("DELETE", path, { tenantScope: true }),
```
`account.ts`:
```ts
import type { ApiClient } from "./client";

export function createAccountApi(client: ApiClient) {
  return {
    // DELETE /api/v1/mobile/admin/account (tenant-scoped). 204 → void.
    deleteAccount: () => client.deleteTenant<void>("/account"),
  };
}
```
Add a `deleteTenant` stub to the demo client returning `Promise.resolve()`.
- [ ] **Step 4: Run** → PASS.
- [ ] **Step 5: Commit**
```bash
git add packages/mobile-shared/api/client.ts packages/mobile-shared/api/account.ts apps/mobile-admin/lib/demo-api-client.ts apps/mobile-admin/__tests__/
git commit -m "feat(mobile-admin): tenant-scoped deleteTenant client helper + account api"
```

---

### Task 9: `useDeleteAccount` hook

**Files:**
- Create: `apps/mobile-admin/lib/admin-api/account-actions.ts`
- Test: `apps/mobile-admin/__tests__/use-delete-account.test.tsx`

**Interfaces:**
- Consumes: `useApiClient()`, `createAccountApi`, `useAuth()` (`signOut`, `refreshToken`).
- Produces: `useDeleteAccount()` → `useMutation` that (1) `await refreshToken()` (fresh id_token so the server call authenticates), (2) `await api.deleteAccount()`, (3) `onSuccess: await signOut()` (AuthGate then redirects to /login). No query invalidation needed (AuthGate clears the cache on uid change).

- [ ] **Step 1: Write the failing test** — mock api + `useAuth`; assert `deleteAccount` then `signOut` are called on mutate; assert `signOut` NOT called if `deleteAccount` rejects.
- [ ] **Step 2: Run** `cd apps/mobile-admin && npx jest use-delete-account` → FAIL.
- [ ] **Step 3: Implement** (mirror `team-actions.ts` factory):
```ts
import { useMutation } from "@tanstack/react-query";
import { createAccountApi } from "@repo/mobile-shared/api/account";
import { useAuth } from "@repo/mobile-shared/auth/provider";
import { useApiClient } from "@/lib/api-client";

export function useDeleteAccount() {
  const client = useApiClient();
  const api = createAccountApi(client);
  const { signOut, refreshToken } = useAuth();

  return useMutation({
    mutationFn: async () => {
      await refreshToken();     // ensure a fresh GIP id_token for the server call
      await api.deleteAccount();
    },
    onSuccess: async () => {
      await signOut();          // AuthGate redirects to /login + clears cache
    },
  });
}
```
- [ ] **Step 4: Run** → PASS.
- [ ] **Step 5: Commit**
```bash
git add apps/mobile-admin/lib/admin-api/account-actions.ts apps/mobile-admin/__tests__/use-delete-account.test.tsx
git commit -m "feat(mobile-admin): useDeleteAccount hook (delete then sign out)"
```

---

### Task 10: "Delete account" UI + typed confirmation

**Files:**
- Modify: `apps/mobile-admin/app/(tabs)/more/account.tsx`
- Test: `apps/mobile-admin/__tests__/account-delete.test.tsx`

**Interfaces:**
- Consumes: `useDeleteAccount()`, existing destructive-button styling (`account.tsx:120-132`), `Alert` confirm pattern (`account.tsx:39-45`).
- Produces: a "Delete account" section below Sign Out. Because deletion is irreversible, use a **typed confirmation** (a modal/inline `TextInput` requiring the user to type `DELETE`) rather than a bare Alert; disable the confirm button until it matches; on confirm call `mutate()`; show a busy state and surface errors via `authErrorMessage`/`ApiError.message`.

- [ ] **Step 1: Write the failing test** (mirror `security.test.tsx`): mock `useDeleteAccount`; render account screen; enter `DELETE`; press confirm; assert `mutate` called. Second case: mismatched text keeps confirm disabled → `mutate` not called.
- [ ] **Step 2: Run** `cd apps/mobile-admin && npx jest account-delete` → FAIL.
- [ ] **Step 3: Implement** the section + confirm flow in `account.tsx`. Copy: label "Delete account" with `color="danger"`; a warning line explaining owners lose the whole store and it can't be undone; a `TextInput` (reuse `FieldInput`) gated to enable the destructive button only when the value trims to `DELETE`; wire `onPress` → `deleteMutation.mutate()`; disable while `deleteMutation.isPending`; render `deleteMutation.error?.message` inline with `accessibilityRole="alert"`.
- [ ] **Step 4: Run** `cd apps/mobile-admin && npx jest` → all green (full suite).
- [ ] **Step 5: Commit**
```bash
git add "apps/mobile-admin/app/(tabs)/more/account.tsx" apps/mobile-admin/__tests__/account-delete.test.tsx
git commit -m "feat(mobile-admin): in-app Delete Account flow with typed confirmation"
```

---

### Task 11: end-to-end verification + typecheck

- [ ] **Step 1:** `cd apps/mobile-admin && npx tsc --noEmit` → 0 errors.
- [ ] **Step 2:** `cd apps/mobile-admin && npx jest` → all suites pass.
- [ ] **Step 3:** `cd services/platform-api && go build ./... && go test ./...` → pass.
- [ ] **Step 4:** `cd services/marketplace-api && go build ./... && go test ./...` → pass.
- [ ] **Step 5:** If any Go module gained an import: `cd services/<svc> && GOWORK=off go mod tidy` and commit the `go.mod`/`go.sum` delta.
- [ ] **Step 6:** Manual smoke (documented, not automated here): with a demo/test merchant, open Account → Delete account → type DELETE → confirm → land on /login → confirm the GIP user can no longer sign in.

---

## Phase 5 — DEFERRED (specified, build after MVP validated)

These complete the deletion contract but are **not required to submit the initiate-deletion flow**. Ship Phases 1–4 first, then:

1. **marketplace-api domain purge (`tenant.deleted` consumer).** marketplace-api's outbox only publishes today; add a subscriber (or an internal `POST /internal/tenants/:id/purge` that platform-api calls) that deletes every `tenant_id`-scoped row across the ~60 domain packages **in FK order, idempotently, with partial-failure retry**. This is the single riskiest piece — a wrong tenant scope deletes another merchant's data. Gate behind the enqueued `tenant.deleted{tenant_id, store_ids}` event from Task 4. Must land within the privacy-policy-stated deletion window (confirm the window in `apps/onboarding/app/privacy` and cite it in the store Data-Safety forms).
2. **Tenant-status enforcement.** `tenants.status ∈ {active,suspended,archived}` is currently never read in any auth path (confirmed). If suspend/archive is to actually block access (independent of GIP deletion), add a status check to marketplace-api's GIP verifier / tenant resolution. Track as its own task with tests.
3. **Push-token deregistration on delete** (nice-to-have): call the notifications delete-registration path so a deleted user's device stops receiving pushes (ties to the known `mobile_admin_push_e2e_gap`).

---

## Self-Review

**Spec coverage:** in-app initiate (Task 10) ✓; genuine identity deletion (Task 1 GIP delete, wired Tasks 4/5/7/9) ✓; owner teardown (Tasks 3–5) ✓; staff self-removal (Task 4 branch) ✓; both stores' requirement satisfied by the initiate+delete flow ✓; deferred full data purge explicitly scoped (Phase 5) ✓.

**Placeholder scan:** the two intentionally-open spots are (a) reuse-or-copy of the `respondError`/`respondProxyErr` responder, and (b) the membership-store handle for staff removal — both are "read the existing file and use the real symbol" instructions, not code placeholders, because the exact symbol name must be confirmed against the repo at execution time. Every code step otherwise contains literal code.

**Type consistency:** `DeleteAccount(ctx, uid)` (Task 1) is consumed by the account service via the local `gipDeleter` interface (Task 4). `DeleteTuple`/`DeleteStoreParent` (Task 2) match their service calls (Task 4). `DeleteTenantAccount(ctx, tenantID, actorUID)` is defined in teamproxy (Task 6) and called by the marketplace handler (Task 7). `deleteTenant<T>(path)` (Task 8) is called by `createAccountApi` (Task 8) and the hook (Task 9). Route path `/mobile/admin/account` (Task 7) matches the client's `"/account"` under the tenant-scoped mount (Task 8). Consistent.

## Execution Handoff

Two execution options:
1. **Subagent-Driven (recommended)** — one fresh subagent per task, review between tasks.
2. **Inline Execution** — batch with checkpoints.
