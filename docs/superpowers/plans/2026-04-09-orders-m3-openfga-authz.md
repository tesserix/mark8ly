# Orders M3 — OpenFGA model additions and permission constants

> **STATUS — SHIPPED 2026-04-10 (heavily reduced scope).** Single commit `8f1b7ea` on `main`. The plan as written is mostly **moot** — reality diverged in three big ways:
>
> 1. **There is no `authz/model.fga` file in marketplace-api.** The FGA model lives in `platform-api`. Per the comment in `internal/authz/client.go`: *"marketplace-api NEVER writes tuples — all writes happen in platform-api during onboarding and invitation accept."* The bootstrap program, the model file, and the model-update arc all belong to a different repo.
> 2. **Authorization is role-based, not permission-based.** Products M4 (already on `main` when Orders M3 ran) settled on `Middleware.RequireTenantRelation(authz.RoleStaff/RoleAdmin/RoleOwner)`. There are no `MarketplaceCanCreateProducts`-style permission constants anywhere in the codebase.
> 3. **Cross-tenant denial is already enforced generically** by `internal/authz/middleware.go` (404-on-deny per spec §13.1.1). The existing `middleware_test.go` covers it. M3's planned cross-tenant denial integration tests would have been pure duplication.
>
> **What actually shipped:** `internal/authz/orders_roles.go` — a single Go file with named role constants for orders / returns / abandoned-carts operations (`OrdersViewRole = RoleStaff`, `OrdersEditRole = RoleAdmin`, `OrdersRefundRole = RoleAdmin`, etc.). M4's route registration imports these instead of inlining role values, giving a single source of truth that documents the role policy in one reviewable place.
>
> **The sections below are preserved as historical context.** Future agents extending the orders authz policy should edit `internal/authz/orders_roles.go`, not the model file.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend the committed OpenFGA model in `authz/model.fga` with three new types (`order`, `return`, `abandoned_cart`), extend the existing authz-bootstrap program so it writes the updated model idempotently to the marketplace FGA store, add typed permission constants to `internal/authz/`, and prove cross-tenant denial via integration tests. Zero per-object tuples are written for any order-related entity — the entire model is tenant-scoped, matching Products slice 1.

**Architecture:** Append-only change to the OpenFGA DSL file. The products slice 1 authz-bootstrap program already uses `WriteAuthorizationModel` with idempotent "create or return existing" semantics; this milestone amends the DSL and re-runs the bootstrap against a clean FGA store to prove the new model loads. No middleware work — the products FGA middleware (`fgaMw.Require(permission)`) is reused verbatim. Integration tests use the existing products FGA test harness (real FGA container in CI).

**Tech Stack:** OpenFGA v1.x, `github.com/openfga/go-sdk`, existing `internal/authz/` patterns from Products slice 1. No new external dependencies.

**Spec reference:** Sections §2 decision 12, §5.1 (model additions verbatim), §5.2 (permission map), §9 M3 exit criteria, §14 DoD item "OpenFGA model updated, bootstrap runs idempotently, staff/admin/owner gating enforced on every route".

**Out of scope for M3** (handled later):
- Wiring the permission constants into actual HTTP routes → M4 (the constants exist here; the routes that check them live in M4)
- Storefront authorization → not needed (storefront bypasses FGA per spec §2 decision 12)

---

## Hard prerequisites

1. **Orders M2 landed.** Typed errors, services, outbox — M3 does not touch any of these but the test harness uses them.
2. **Products slice 1 OpenFGA integration shipped.** Specifically, `services/marketplace-api/authz/model.fga` exists, `cmd/authz-bootstrap/main.go` exists and writes the model to the marketplace FGA store, `internal/authz/` exists with products permission constants (`MarketplaceCanCreateProducts`, etc.), and the FGA middleware factory exists.
3. **FGA test container runs in CI.** Products slice 1 set up `openfga/openfga` as a test container in the marketplace-api CI matrix.

**Task 0 verifies all three.**

---

## Decisions locked for this milestone

1. **Model additions are appended, not rewritten.** The existing `product` and `category` types are untouched. New `order`, `return`, `abandoned_cart` types are appended verbatim from spec §5.1.
2. **No per-object tuples for orders, returns, or abandoned carts.** Same reasoning as products slice 1: tuple writes at create time carry no information beyond the DB's tenant_id and introduce drift risk. The spec calls this out in §2 decision 12 and §12 risks.
3. **Permission constants live in a new file `internal/authz/orders.go`.** Products slice 1 owns `internal/authz/products.go` and `internal/authz/categories.go`. Orders gets its own file to keep ownership clean.
4. **Constant naming follows products' convention.** `MarketplaceCanViewOrders`, `MarketplaceCanEditOrders`, `MarketplaceCanRefundOrders`, `MarketplaceCanViewReturns`, `MarketplaceCanEditReturns`, `MarketplaceCanViewAbandonedCarts`, `MarketplaceCanEditAbandonedCarts`. These are tenant-level permission constants — handlers will call `fgaMw.Require(authz.MarketplaceCanViewOrders)`.
5. **Bootstrap program is not rewritten; it is re-run.** The products bootstrap reads `authz/model.fga` and writes it. Since the file has new types, the next bootstrap run lands them. No code change in `cmd/authz-bootstrap/main.go` unless it hard-codes type enumeration (it should not — verify in Task 0).
6. **FGA model version is not explicitly pinned.** OpenFGA auto-generates a new `authorization_model_id` on each write. The bootstrap program captures the latest id and stores it (mechanism inherited from products slice 1).
7. **Integration tests assert cross-tenant denial on ALL new permission constants.** Not just one — the matrix is cheap to run.

---

## File structure produced by M3

```
services/marketplace-api/
├── authz/
│   └── model.fga              # MODIFY — append 3 new types
└── internal/
    └── authz/
        ├── orders.go          # NEW — permission constants for orders/returns/abandoned carts
        └── orders_test.go     # NEW — cross-tenant denial integration tests
```

No other files are created or modified.

---

## Task decomposition

### Task 0: Verify Orders M2 and Products FGA prerequisites

- [ ] **Step 1: Verify M2 files exist and tests pass**

```bash
for p in \
  services/marketplace-api/internal/order/service.go \
  services/marketplace-api/internal/order/return_service.go \
  services/marketplace-api/internal/outbox/drainer.go; do
  test -f "$p" || { echo "MISSING: $p"; exit 1; }
done
cd services/marketplace-api && go test -tags=testing ./internal/order/... ./internal/outbox/... && echo "M2 OK"
```

- [ ] **Step 2: Verify products FGA scaffolding**

```bash
for p in \
  services/marketplace-api/authz/model.fga \
  services/marketplace-api/cmd/authz-bootstrap/main.go \
  services/marketplace-api/internal/authz/products.go \
  services/marketplace-api/internal/authz/middleware.go; do
  test -f "$p" || { echo "MISSING: $p"; exit 1; }
done
echo "products FGA OK"
```

- [ ] **Step 3: Verify the bootstrap program does NOT hard-code types**

```bash
grep -n 'type product\|type category' services/marketplace-api/cmd/authz-bootstrap/main.go
```
Expected: zero matches. If it hard-codes type names, **STOP** — the bootstrap needs to read the model file, not inline its contents. File a follow-up on products to clean this up before M3 can land its new types cleanly.

- [ ] **Step 4: Verify the FGA test container is running (locally or via docker-compose)**

```bash
curl -sf http://localhost:8080/stores || echo "FGA not reachable"
```
Expected: an empty stores list (`{"stores":[]}`) or an already-bootstrapped marketplace store. If unreachable, `docker-compose up -d openfga` and retry.

- [ ] **Step 5: Verify the existing products FGA tests still pass**

```bash
cd services/marketplace-api && go test -tags=testing ./internal/authz/
```
Expected: PASS. Any regression here means the products authz layer is broken independently of orders work — **STOP** and escalate.

No commit. Task 0 is read-only.

---

### Task 1: Append orders/return/abandoned_cart types to `authz/model.fga`

**Files:**
- Modify: `services/marketplace-api/authz/model.fga`

- [ ] **Step 1: Read the current model file**

```bash
cat services/marketplace-api/authz/model.fga
```
Identify where the existing types end. Locate the final type definition (`type category` or similar).

- [ ] **Step 2: Append the three new types verbatim from spec §5.1**

Append these blocks after the last existing type:
```
type order
  relations
    define tenant:   [tenant]
    define viewer:   staff from tenant
    define editor:   admin from tenant
    define refunder: owner from tenant

type return
  relations
    define tenant: [tenant]
    define viewer: staff from tenant
    define editor: admin from tenant

type abandoned_cart
  relations
    define tenant: [tenant]
    define viewer: staff from tenant
    define editor: admin from tenant
```

- [ ] **Step 3: Validate the DSL parses**

Run the bootstrap program in dry-run mode (or equivalent `openfga validate` CLI if available):
```bash
cd services/marketplace-api && go run ./cmd/authz-bootstrap -dry-run
```
Expected: exits 0 with "model validated". If the bootstrap program does not have a `-dry-run` flag, run it against a local FGA instance (which is idempotent) and check the exit code:
```bash
FGA_URL=http://localhost:8080 go run ./cmd/authz-bootstrap
```
Expected: exits 0 with "model written, id=...".

- [ ] **Step 4: Confirm the three new types are visible in the live FGA store**

```bash
curl -s http://localhost:8080/stores | jq '.stores[] | .id' | head -1 | xargs -I {} \
  curl -s "http://localhost:8080/stores/{}/authorization-models" | jq '.authorization_models[0].type_definitions | .[].type' | sort
```
Expected output contains: `abandoned_cart`, `category`, `order`, `product`, `return`, `tenant`, `user`.

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/authz/model.fga
git commit -m "feat(marketplace-api): add order/return/abandoned_cart types to FGA model"
```

---

### Task 2: Write permission constants in `internal/authz/orders.go`

**Files:**
- Create: `services/marketplace-api/internal/authz/orders.go`

- [ ] **Step 1: Read the existing products constants to match the convention**

```bash
cat services/marketplace-api/internal/authz/products.go
```
Identify the shape of `Permission` type (probably a struct or a named string), how relation names are encoded, and whether the file has helper constructors.

- [ ] **Step 2: Write `orders.go` mirroring the products pattern**

Assuming the products file uses a simple struct pattern:
```go
package authz

// Order permissions are all tenant-scoped — no per-object tuples are ever
// written. See spec §2 decision 12 and §5.2.

var (
	// MarketplaceCanViewOrders: staff or higher can read order lists and details.
	MarketplaceCanViewOrders = Permission{
		Type:     "order",
		Relation: "viewer",
		Store:    StoreMarketplace, // inherited constant from products
	}

	// MarketplaceCanEditOrders: admin or higher can transition status, fulfill, cancel, notes.
	MarketplaceCanEditOrders = Permission{
		Type:     "order",
		Relation: "editor",
		Store:    StoreMarketplace,
	}

	// MarketplaceCanRefundOrders: owner only. Gates the refund dialog and the
	// bookkeeping refund endpoint.
	MarketplaceCanRefundOrders = Permission{
		Type:     "order",
		Relation: "refunder",
		Store:    StoreMarketplace,
	}

	// MarketplaceCanViewReturns: staff or higher.
	MarketplaceCanViewReturns = Permission{
		Type:     "return",
		Relation: "viewer",
		Store:    StoreMarketplace,
	}

	// MarketplaceCanEditReturns: admin or higher.
	MarketplaceCanEditReturns = Permission{
		Type:     "return",
		Relation: "editor",
		Store:    StoreMarketplace,
	}

	// MarketplaceCanViewAbandonedCarts: staff or higher.
	MarketplaceCanViewAbandonedCarts = Permission{
		Type:     "abandoned_cart",
		Relation: "viewer",
		Store:    StoreMarketplace,
	}

	// MarketplaceCanEditAbandonedCarts: admin or higher. "Edit" here specifically
	// means triggering recovery emails — no other mutating actions exist in slice 1.
	MarketplaceCanEditAbandonedCarts = Permission{
		Type:     "abandoned_cart",
		Relation: "editor",
		Store:    StoreMarketplace,
	}
)
```

**If the products `Permission` type has a different shape**, adapt accordingly. The principle is: same type, same constructor style, same file-per-domain split.

- [ ] **Step 3: Build**

```bash
cd services/marketplace-api && go build ./internal/authz/...
```
Expected: exits 0.

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/internal/authz/orders.go
git commit -m "feat(marketplace-api): order/return/abandoned_cart permission constants"
```

---

### Task 3: Cross-tenant denial integration tests

**Files:**
- Create: `services/marketplace-api/internal/authz/orders_test.go`

- [ ] **Step 1: Read the existing products test file to understand the harness**

```bash
cat services/marketplace-api/internal/authz/products_test.go 2>/dev/null || \
  find services/marketplace-api -name '*authz*test*'
```
Identify: how the test instantiates an FGA client, how it seeds tenant membership tuples, and how `fgaClient.Check` is called.

- [ ] **Step 2: Write the test file**

```go
package authz_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/authz"
)

func TestOrderPermissions_CrossTenantDenied(t *testing.T) {
	ctx := context.Background()
	h := newAuthzHarness(t) // shared helper defined in products_test.go

	tenantA := uuid.New()
	tenantB := uuid.New()
	staffA := uuid.New()
	adminA := uuid.New()
	ownerA := uuid.New()

	// Seed membership tuples (these land in the shared marketplace FGA store)
	h.WriteTuple(ctx, "tenant:"+tenantA.String()+"#staff", "user:"+staffA.String())
	h.WriteTuple(ctx, "tenant:"+tenantA.String()+"#admin", "user:"+adminA.String())
	h.WriteTuple(ctx, "tenant:"+tenantA.String()+"#owner", "user:"+ownerA.String())

	cases := []struct {
		name  string
		perm  authz.Permission
		user  uuid.UUID
		cross bool // true if we should also assert cross-tenant denial
		allow bool // expected result for same-tenant check
	}{
		{"staff can view orders", authz.MarketplaceCanViewOrders, staffA, true, true},
		{"staff cannot edit orders", authz.MarketplaceCanEditOrders, staffA, true, false},
		{"staff cannot refund orders", authz.MarketplaceCanRefundOrders, staffA, true, false},
		{"admin can view orders", authz.MarketplaceCanViewOrders, adminA, true, true},
		{"admin can edit orders", authz.MarketplaceCanEditOrders, adminA, true, true},
		{"admin cannot refund orders", authz.MarketplaceCanRefundOrders, adminA, true, false},
		{"owner can refund orders", authz.MarketplaceCanRefundOrders, ownerA, true, true},

		{"staff can view returns", authz.MarketplaceCanViewReturns, staffA, true, true},
		{"staff cannot edit returns", authz.MarketplaceCanEditReturns, staffA, true, false},
		{"admin can edit returns", authz.MarketplaceCanEditReturns, adminA, true, true},

		{"staff can view abandoned carts", authz.MarketplaceCanViewAbandonedCarts, staffA, true, true},
		{"staff cannot trigger recovery", authz.MarketplaceCanEditAbandonedCarts, staffA, true, false},
		{"admin can trigger recovery", authz.MarketplaceCanEditAbandonedCarts, adminA, true, true},
	}

	for _, tc := range cases {
		t.Run(tc.name+" same tenant", func(t *testing.T) {
			ok, err := h.Check(ctx, tc.perm, tc.user, tenantA)
			require.NoError(t, err)
			require.Equal(t, tc.allow, ok)
		})
		if tc.cross {
			t.Run(tc.name+" cross tenant", func(t *testing.T) {
				ok, err := h.Check(ctx, tc.perm, tc.user, tenantB)
				require.NoError(t, err)
				require.False(t, ok, "cross-tenant access must ALWAYS be denied")
			})
		}
	}
}
```

Note: `newAuthzHarness`, `h.WriteTuple`, and `h.Check` signatures are inherited from the products test harness. If they exist with different method names (e.g. `h.Write`, `h.HasPermission`), adapt accordingly.

- [ ] **Step 3: Run**

```bash
cd services/marketplace-api && go test -tags=testing -run TestOrderPermissions -v ./internal/authz/
```
Expected: all PASS. The cross-tenant assertions are the most important — they prove the tenant-scoping invariant holds without any per-object tuples.

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/internal/authz/orders_test.go
git commit -m "test(marketplace-api): cross-tenant denial coverage for order permissions"
```

---

### Task 4: Idempotency re-check — run the bootstrap twice

**Files:** none

The whole point of the authz bootstrap is that it can be run multiple times without drift. Verify this hasn't regressed after the model additions.

- [ ] **Step 1: Run the bootstrap twice in a row against the same FGA store**

```bash
cd services/marketplace-api && \
  go run ./cmd/authz-bootstrap && \
  go run ./cmd/authz-bootstrap
```
Expected: both runs exit 0. The second run either reuses the existing model id (no-op) or writes a new model version (depending on products' implementation choice). If the second run errors with "already exists" or similar, the bootstrap is not idempotent and **STOP** — file a follow-up.

- [ ] **Step 2: Verify tuple membership tests still work after a second bootstrap**

```bash
cd services/marketplace-api && go test -tags=testing -run TestOrderPermissions -v ./internal/authz/
```
Expected: PASS. A model rewrite that invalidated existing tuples would fail this test.

No commit — verification only.

---

### Task 5: M3 exit checklist + handoff

**Files:**
- Modify: `services/marketplace-api/internal/order/README.md`

- [ ] **Step 1: Tick the M3 exit criteria from spec §9 M3**

- [x] Model committed to `authz/model.fga` with three new types — Task 1
- [x] Bootstrap writes the model idempotently — Task 4
- [x] Permission constants added to `internal/authz/` — Task 2
- [x] FGA integration tests pass for cross-tenant denial on every new route — Task 3
- [x] Products slice 1 tests still pass — Task 0 step 5 + Task 4

- [ ] **Step 2: Append "M3 handoff" section to `internal/order/README.md`**

Note:
- M3 landed ← you are here
- List of new permission constants
- Middleware usage pattern: `fgaMw.Require(authz.MarketplaceCanViewOrders)`
- Bootstrap idempotency guarantee (can be re-run)
- Reminder: no per-object tuples, tenant-scoped only
- Pending: HTTP handlers (M4), checkout integration (M5)

- [ ] **Step 3: Commit**

```bash
git add services/marketplace-api/internal/order/README.md
git commit -m "docs(marketplace-api): M3 handoff note for order authz constants"
```

---

## Parallelization notes

Tasks 1 and 2 can run in parallel (both depend only on Task 0). Task 3 depends on both. Task 4 depends on Tasks 1 and 3. Task 5 is the final handoff.

## Exit gate to M4

Do not start Orders M4 until:
1. All tasks committed.
2. FGA integration tests pass in CI.
3. A human has confirmed the permission constant naming matches products' convention exactly — the HTTP handlers in M4 will reference these constants by name and inconsistent naming will propagate.
4. The bootstrap program is idempotent (verified in Task 4).

If any item is false, M4 does not start.
