# Products M7d — Admin UI: Copy-to-store + Bulk Actions Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship the two remaining list-page power-user flows from Admin UI Slice 2 — **Copy to store** (single-product and bulk) and **Bulk actions bar** (archive/unarchive/publish/unpublish/assign category/copy/delete). No new pages. Everything attaches to the existing `/products` list (from M7a) and `/products/:id` detail (from M7b/M7c). CSV import/export is explicitly deferred to M7e.

**Architecture:** A new `useProductSelection()` hook owns selection state, persisted to `sessionStorage` keyed by `store_id` with a hard cap of 100 ids. The list page subscribes to the hook and renders `<BulkActionsBar>` as a sticky bottom bar (Paper surface, hairline top rule) whenever ≥1 row is selected. Single-product copy reuses `<CopyToStoreDialog>` mounted from the list overflow menu and the detail page action menu; bulk copy reuses the same dialog in `mode="bulk"`. All backend calls go through `lib/api/marketplace-api.ts` and `app/products/actions.ts` server actions — the bar never calls the API directly from a client component. The bulk endpoint is **atomic per row** with per-id FGA enforcement, so partial success is the norm: the bar surfaces a per-id result summary toast, not a global success/failure.

**Tech Stack:** Next.js 16 (App Router + server components + server actions), React 19, Tailwind v4, `@tesserix/web` v1.7.1 primitives (Dialog only — no new dialog systems), `@repo/ui` promoted components, React Hook Form (for dialog form state), Zod, Vitest + RTL, Playwright 1.59+, Paper · Ink · Moss design tokens. Backend: Go 1.26 / Gin / GORM / Postgres under `services/marketplace-api`, typed errors via `pkg/apperrors`, FGA middleware under `internal/authz/`.

**Design Authority:** `docs/superpowers/specs/2026-04-10-products-admin-ui-slice-2-design.md` §3, §5.1, §13.1.1 (role matrix), §13.5 (no-dialogs rule except hard delete).

---

## Status

> **Pending.** All tasks open. Current branch: `feat/products-m7d-m7e` (worktree). Depends on M7a list page (checkbox UI stubbed), M7b detail page action menu slot, M7c `ProductForm` tab shell (untouched here but shares the same form root — do not break it).

---

## Scope check

Adds two top-level components (`CopyToStoreDialog`, `BulkActionsBar`), one hook (`useProductSelection`), selection-aware integration into `apps/admin/components/products/ProductsList.tsx`, new entries in `apps/admin/lib/api/marketplace-api.ts` (single-copy + bulk clients), and new server actions in `apps/admin/app/products/actions.ts`. **May add backend work** to `services/marketplace-api/internal/product/{handlers,service,repository}.go` and a new `internal/authz/` binding if Task 1 verification finds gaps — scoped into M7d, not deferred.

Spec sections authoritative for this milestone:
- Design spec §3 (all subsections — copy dialog, bulk bar, selection state, backend contracts)
- Design spec §5.1 (testing strategy)
- Design spec §13.1.1 (role-based permission matrix for each action)
- Design spec §13.5 (hard-delete confirm is the only dialog permitted beyond the copy dialog)
- `mark8ly/.impeccable.md` — Paper · Ink · Moss design context

**Out of scope (deferred):**
- CSV import/export (M7e)
- Inventory multi-location bulk edit
- Bulk price adjust (percentage/flat)
- Tag/metafield bulk edit
- Any change to M7c variants/media surfaces

---

## Decisions locked (from the spec)

1. **Single dialog policy.** `CopyToStoreDialog` and the hard-delete confirm are the only dialog surfaces permitted in M7d. Everything else is inline feedback (toast + optimistic list update). No "are you sure you want to archive" modal — archive is reversible.

2. **Selection persistence = `sessionStorage` keyed by `store_id`.** Survives reloads and cross-tab within the same origin/session, but is scoped per store so selecting on store A never leaks to store B. URL hash mirrors **count only** (`#sel={count}`) to keep URLs short and under Istio header limits; ids never go in the URL.

3. **Hard cap = 100 ids.** Enforced in the hook (`push` past 100 is a no-op + toast), in the bulk server action (Zod schema `.max(100)`), and in the backend handler (HTTP 422 `bulk_cap_exceeded`). Three layers, not one.

4. **Atomic per row, not per batch.** The bulk endpoint iterates ids, runs FGA per id, applies the action, and accumulates results. Partial success is a 200 with per-id status — never a 207 Multi-Status, never a 4xx when at least one row succeeded. Clients must handle mixed outcomes.

5. **FGA per id for every action, not just delete.** Copy-to-store, archive, publish — all actions run `authz.Check(user, action, "product:"+id)` per row. An unauthorized id comes back as `{status: "error", error: "forbidden"}` in the results array; the bar does NOT return a global 403.

6. **Role gating is client + server.** Client hides actions the user cannot perform (per §13.1.1); server rejects with typed error if a forbidden action is attempted anyway. Client hide is UX; server gate is security.

7. **Copy target store list comes from `serverSession`.** The dialog does not call an API to enumerate stores — it reads the server session at render time. If `SessionHeaders` does not currently include `stores[]`, Task 1 flags this as a backend/session gap.

8. **Copied products land as drafts in the target store.** Fixed behavior, rendered as editorial copy in the dialog, NOT a toggle. Do not add a "publish immediately" option.

9. **`useProductSelection()` is the single source of truth.** No parallel selection state in the list component. The list reads `selectedIds` from the hook and writes via hook setters only. This matches the M7c decision to keep RHF form state centralized.

10. **Paper · Ink · Moss tokens only.** Bulk bar uses `--paper-200` background, `--ink-900` text, `--moss-700` action accents, hairline rule via existing border token. No new hex values. No shadows beyond `--shadow-1` for the sticky bar.

11. **Impeccable chain is a gate.** Task 0 verifies `mark8ly/.impeccable.md`; Task 11 runs the full chain (`frontend-design` → `critique` → `polish` → `arrange` → `typeset` → `audit` → `adapt`) with a `critique` score ≥ 7.5 threshold.

---

## File structure produced by M7d

### New / modified frontend files

```
apps/admin/
  lib/products/
    useProductSelection.ts              (new — sessionStorage-backed hook)
    useProductSelection.test.ts         (new)
  lib/api/
    marketplace-api.ts                  (modify — add copyProduct + bulkProductAction clients)
  lib/validation/
    bulk-action.ts                      (new — Zod schema, 100-id cap)
    bulk-action.test.ts                 (new)
  components/products/
    ProductsList.tsx                    (modify — wire selection hook + mount bulk bar)
    CopyToStoreDialog.tsx               (new)
    CopyToStoreDialog.test.tsx          (new)
    BulkActionsBar.tsx                  (new)
    BulkActionsBar.test.tsx             (new)
    BulkDeleteConfirmDialog.tsx         (new — the one allowed extra dialog)
    BulkCategoryAssignPopover.tsx       (new — reuses M7b ProductCategoriesPicker)
  app/products/
    actions.ts                          (modify — add copyProductAction + bulkProductAction)
    actions.test.ts                     (modify — add coverage for new actions)
  tests/e2e/
    products-copy-to-store.spec.ts      (new — E2E 1)
    products-bulk-actions.spec.ts       (new — E2E 2)
```

### New / modified backend files (only if Task 1 verification finds gaps)

```
services/marketplace-api/
  internal/product/
    handlers.go                         (modify — add Copy + Bulk handlers if missing)
    service.go                          (modify — add BulkApply orchestrator if missing)
    service_bulk_test.go                (new — per-id FGA + partial-success tests)
    service_copy_test.go                (new or extend — if copy path is missing)
    models.go                           (modify — BulkRequest / BulkResultRow types)
  internal/authz/
    product_actions.go                  (modify — expose per-action relation constants)
  cmd/marketplace-api/main.go           (modify — route registration)
```

---

## New npm dependencies

None. The dialog uses `@tesserix/web` Dialog primitive already pinned; the hook uses built-in `sessionStorage`; bulk bar uses existing Tailwind + `@repo/ui` primitives.

---

## Landmines

1. **`sessionStorage` is undefined during SSR.** The hook MUST guard every read/write with `typeof window !== "undefined"`. Otherwise the list page crashes on server render. Write a test that renders the hook in a Node env and asserts no throw.
2. **`sessionStorage` JSON corruption.** Parse failures must fall back to an empty selection, never throw. Log to the frontend logger, do not surface to the user.
3. **Selection must clear on store switch.** The hook's sessionStorage key is `products.selection.{store_id}` — but the list page also needs to clear the **current render's** state when `store_id` changes mid-session. Use a `useEffect` on `store_id` to hydrate.
4. **100-id cap at three layers.** Hook silently drops past-100 adds + toasts; server action Zod rejects past-100; backend rejects past-100 with HTTP 422. All three tested independently.
5. **URL hash `#sel={count}` is count only.** Do not put ids in the hash. Istio and some intermediate proxies enforce ~8KB header limits; 100 UUIDs is ~4KB plus encoding — too close to the margin. The hash is bookmarkable state for the count badge only.
6. **Per-id FGA is mandatory for every action.** The server handler must call `authz.Check` in the row loop, not outside it. A single check against the store-level permission is insufficient — a user may have archive permission on store X but not on a specific vendor-owned product within store X.
7. **Atomic per row, not per batch.** Do not wrap the bulk loop in a single DB transaction. One bad row must not roll back the other 99. Each row gets its own short transaction; results accumulate; response is always 200 if at least the request validation passed.
8. **Optimistic updates must be per-id reversible.** If archive succeeds for 90 rows and fails for 10, the list must re-render with 90 rows archived and 10 rows untouched. Do not mark all 100 archived then roll back on first error — the server response arrives once at the end.
9. **Role gating: staff users must not see the bar at all.** Per §13.1.1, staff has zero bulk actions. Render `null` from `BulkActionsBar` when `role === "staff"`, not an empty bar.
10. **Hard-delete confirm is the ONLY dialog allowed beyond `CopyToStoreDialog`.** Do not add "are you sure you want to archive 47 products" modals. Archive is reversible; confirmation is friction. Toast feedback only.
11. **`CopyToStoreDialog` in bulk mode must show the selection count, not ids.** The dialog title in bulk mode is "Copy {N} products to another store". No id list, no product name list. Users trust the selection they made on the list.
12. **`serverSession.stores` may not exist yet.** Task 1 flags this: if `SessionHeaders` = `{userId, tenantId}` only, we need a richer session or a `GET /api/v1/admin/stores/mine` endpoint. Do not paper over by calling an ad-hoc API from the dialog — route it through `serverSession`.
13. **Bulk category assign reuses the M7b `ProductCategoriesPicker`.** Do not rewrite it. Mount it inside `BulkCategoryAssignPopover.tsx` with `mode="bulk-assign"` and a single "Apply to {N}" submit.
14. **Paper · Ink · Moss tokens only.** No new hex values. No legacy terracotta/sage/cream aliases in new code. Sticky bar background is `--paper-200`, not `#FFFFFF`.
15. **Toast after partial success must render the count of errors.** `"Archived 90 of 100. 10 failed."` not `"Archived 100"`. Include a "View errors" affordance that expands a list of `{product_id, error}` rows.

---

## Task decomposition

**12 tasks** (0 through 11), dependency-ordered. Task 0 and Task 1 are gates. Tasks 2 and 4 (pure logic — hook + dialog) can run in parallel after Task 1 closes. Tasks 5–9 are frontend-serial because they all touch `ProductsList.tsx`. Tasks 10–11 are verification.

Legend: **U** = unit/pure, **C** = component (RTL), **I** = integration (needs Postgres), **E** = E2E (Playwright).

---

### Task 0: Impeccable design context check

**Files:** none (verification only)

**Scope:** Ensure `mark8ly/.impeccable.md` exists and is current before any UI code is written. Pins Paper · Ink · Moss design context for the `frontend-design` / `critique` / `polish` chain used in Task 11.

- [ ] **Step 1: Check for the file**

```bash
test -f mark8ly/.impeccable.md && echo "OK" || echo "MISSING"
```

Expected: `OK`. If `MISSING`, stop and run the `teach-impeccable` skill, commit the result, continue.

- [ ] **Step 2: Verify it mentions Paper · Ink · Moss**

```bash
grep -q "Paper" mark8ly/.impeccable.md && grep -q "Ink" mark8ly/.impeccable.md && grep -q "Moss" mark8ly/.impeccable.md && echo "OK" || echo "STALE"
```

Expected: `OK`. If `STALE`, re-run `teach-impeccable`.

- [ ] **Step 3: Commit (only if regenerated)**

```bash
git add mark8ly/.impeccable.md
git commit -m "chore(impeccable): refresh design context for M7d"
```

---

### Task 1: Backend verification gate + fix sub-tasks

**Files (investigation):**
- Read: `services/marketplace-api/internal/product/handlers.go`
- Read: `services/marketplace-api/internal/product/service.go`
- Read: `services/marketplace-api/internal/authz/` (existing relation constants)
- Read: `services/marketplace-api/cmd/marketplace-api/main.go` (route registration)
- Read: `apps/admin/lib/serverSession.ts` (check for `stores[]` on session)

**Scope:** Close every backend gap required by M7d before any frontend work begins. Work through the verification items in order. Each gap becomes its own sub-task with its own tests and commits.

- [ ] **Step 1: Catalog current copy + bulk + session state**

Run existing product service tests:

```bash
cd services/marketplace-api
go test ./internal/product/... -run 'Copy|Bulk' -v
```

Read `internal/product/handlers.go` and `service.go` end-to-end. Write a scratch file `.planning/m7d-backend-gaps.md` listing which of these are supported and which are gaps:

1. `POST /api/v1/admin/stores/:sourceStoreId/products/:id/copy` with body `{target_store_id, copy_media}` returning `{new_product_id, new_store_id}` (may exist from M5a `Service.Copy`)
2. `POST /api/v1/admin/stores/:storeId/products/bulk` with body `{action, product_ids, params?}` returning `{results: [{id, status, error?}]}`
3. Per-id FGA enforcement on EVERY bulk action (archive, unarchive, publish, unpublish, assign_category, copy, delete) — not just delete
4. Atomic per-row transactions (not a single batch transaction)
5. 100-id backend cap returning HTTP 422 `bulk_cap_exceeded`
6. Owner-only gate on bulk delete at the backend (client gate is insufficient)
7. `serverSession.stores[]` — frontend can enumerate target stores without a new API call, OR a new `GET /api/v1/admin/stores/mine` endpoint exists

- [ ] **Step 2: Write a failing integration test for every gap**

For each missing item, add an integration test under `internal/product/service_bulk_test.go` or `service_copy_test.go` that drives intended behavior through service + repository + FGA layer against real Postgres + FGA test container. Example skeleton:

```go
func TestBulkApply_PerIdFGA_Archive(t *testing.T) {
    ctx, db, fga, cleanup := testenv.Setup(t)
    defer cleanup()

    svc := product.NewService(product.NewRepository(db), fga, nil)
    storeID, _ := testdb.SeedStore(t, db)
    allowedID := testdb.SeedProduct(t, db, storeID)
    forbiddenID := testdb.SeedProduct(t, db, storeID)

    // user has archive on allowedID only
    testdb.GrantFGA(t, fga, "user:u1", "can_archive", "product:"+allowedID)

    res, err := svc.BulkApply(ctx, product.BulkApplyInput{
        UserID:     "u1",
        StoreID:    storeID,
        Action:     product.BulkActionArchive,
        ProductIDs: []string{allowedID, forbiddenID},
    })
    require.NoError(t, err)
    require.Len(t, res.Results, 2)

    assertResult(t, res.Results, allowedID, "ok", "")
    assertResult(t, res.Results, forbiddenID, "error", "forbidden")

    // allowed product is archived, forbidden is untouched
    allowed := testdb.GetProduct(t, db, allowedID)
    require.Equal(t, product.StatusArchived, allowed.Status)
    forbidden := testdb.GetProduct(t, db, forbiddenID)
    require.NotEqual(t, product.StatusArchived, forbidden.Status)
}
```

Run it:

```bash
go test ./internal/product/... -run TestBulkApply_PerIdFGA_Archive -v
```

Expected: **FAIL** (behavior not implemented).

- [ ] **Step 3: Implement minimal fix for that gap**

Extend `product.Service.BulkApply` (or add it). Keep per-gap commits small. Prefer reusing single-product `Archive`/`Publish`/etc. methods inside the row loop — don't duplicate business rules.

- [ ] **Step 4: Run the test to verify it passes**

```bash
go test ./internal/product/... -run TestBulkApply_PerIdFGA_Archive -v
```

Expected: **PASS**.

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/product/
git commit -m "feat(marketplace-api): per-id FGA enforcement in BulkApply (M7d gap)"
```

- [ ] **Step 6: Repeat Steps 2–5 for every remaining gap from Step 1**

For gap #5 (100-id cap):

```go
// internal/product/service.go
const MaxBulkProductIDs = 100

func (s *Service) validateBulkCap(ids []string) error {
    if len(ids) > MaxBulkProductIDs {
        return apperrors.Unprocessable("bulk_cap_exceeded",
            fmt.Sprintf("bulk request has %d ids, max is %d", len(ids), MaxBulkProductIDs))
    }
    return nil
}
```

For gap #7 (`serverSession.stores[]`), the fix is TypeScript-side in `apps/admin/lib/serverSession.ts` — extend `SessionHeaders` to include `stores: Array<{id, slug, name, role}>` and populate from the auth-bff session cookie. If the auth-bff does not carry store memberships, add a backend sub-task to extend the session contract OR add a thin `GET /api/v1/admin/stores/mine` endpoint in marketplace-api (preferred; avoids cross-service session changes).

- [ ] **Step 7: Rerun full marketplace-api suite**

```bash
cd services/marketplace-api
go test ./... -race
```

Expected: all green.

- [ ] **Step 8: Fill the exit matrix before closing Task 1**

Task 1 is done only when **every** row of this matrix is marked ✅. If any row is still open, Task 2 must not start.

| # | Verification item | Test name | Status | Commit |
|---|---|---|---|---|
| 1 | `POST .../products/:id/copy` with `copy_media` toggle returns `{new_product_id, new_store_id}` | `TestCopyProduct_RoundTrip` | ⬜ | `_________` |
| 2 | Copy endpoint enforces source-store read FGA AND target-store write FGA | `TestCopyProduct_DualFGA` | ⬜ | `_________` |
| 3 | `POST .../products/bulk` accepts action ∈ {archive,unarchive,publish,unpublish,assign_category,copy,delete} | `TestBulkApply_ActionMatrix` | ⬜ | `_________` |
| 4 | Per-id FGA enforced on every action (not just delete) | `TestBulkApply_PerIdFGA_AllActions` | ⬜ | `_________` |
| 5 | Atomic per-row: one bad row does not roll back the batch | `TestBulkApply_PartialSuccess` | ⬜ | `_________` |
| 6 | 100-id backend cap → HTTP 422 `bulk_cap_exceeded` | `TestBulkApply_CapExceeded` | ⬜ | `_________` |
| 7 | Bulk delete is owner-only at the backend | `TestBulkApply_DeleteOwnerOnly` | ⬜ | `_________` |
| 8 | Frontend can enumerate target stores (via `serverSession.stores[]` OR `GET /stores/mine`) | `TestStoresMine_ReturnsUserStores` (or type test) | ⬜ | `_________` |

Exit criteria: all 8 rows ✅, all named tests green, `go test ./... -race` green, `.planning/m7d-backend-gaps.md` marked fully closed.

---

### Task 2: `useProductSelection()` hook (U)

**Files:**
- Create: `apps/admin/lib/products/useProductSelection.ts`
- Create: `apps/admin/lib/products/useProductSelection.test.ts`

**Scope:** A pure React hook wrapping `sessionStorage`-backed selection state, keyed per store, with a 100-id hard cap. No UI. Depends on nothing except React and a minimal logger stub. This is the foundation for the entire bulk bar.

- [ ] **Step 1: Write the failing test**

```typescript
// apps/admin/lib/products/useProductSelection.test.ts
import { describe, it, expect, beforeEach, vi } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { useProductSelection } from "./useProductSelection";

describe("useProductSelection", () => {
  beforeEach(() => {
    sessionStorage.clear();
  });

  it("starts empty", () => {
    const { result } = renderHook(() => useProductSelection("store-a"));
    expect(result.current.selectedIds).toEqual([]);
    expect(result.current.count).toBe(0);
  });

  it("toggles ids in and out", () => {
    const { result } = renderHook(() => useProductSelection("store-a"));
    act(() => result.current.toggle("p1"));
    act(() => result.current.toggle("p2"));
    expect(result.current.selectedIds).toEqual(["p1", "p2"]);
    act(() => result.current.toggle("p1"));
    expect(result.current.selectedIds).toEqual(["p2"]);
  });

  it("persists to sessionStorage keyed by store_id", () => {
    const { result } = renderHook(() => useProductSelection("store-a"));
    act(() => result.current.toggle("p1"));
    const raw = sessionStorage.getItem("products.selection.store-a");
    expect(raw).not.toBeNull();
    expect(JSON.parse(raw!)).toEqual(["p1"]);
  });

  it("does not leak across stores", () => {
    const { result: a } = renderHook(() => useProductSelection("store-a"));
    act(() => a.current.toggle("p1"));
    const { result: b } = renderHook(() => useProductSelection("store-b"));
    expect(b.current.selectedIds).toEqual([]);
  });

  it("enforces 100-id hard cap (silently drops + returns false)", () => {
    const { result } = renderHook(() => useProductSelection("store-a"));
    act(() => {
      for (let i = 0; i < 100; i++) result.current.add(`p${i}`);
    });
    expect(result.current.count).toBe(100);
    let accepted: boolean = true;
    act(() => {
      accepted = result.current.add("p100");
    });
    expect(accepted).toBe(false);
    expect(result.current.count).toBe(100);
  });

  it("clearAll empties both state and storage", () => {
    const { result } = renderHook(() => useProductSelection("store-a"));
    act(() => result.current.toggle("p1"));
    act(() => result.current.clearAll());
    expect(result.current.selectedIds).toEqual([]);
    expect(sessionStorage.getItem("products.selection.store-a")).toBeNull();
  });

  it("tolerates corrupt sessionStorage JSON", () => {
    sessionStorage.setItem("products.selection.store-a", "{not json");
    const { result } = renderHook(() => useProductSelection("store-a"));
    expect(result.current.selectedIds).toEqual([]);
  });

  it("is SSR-safe (no window access at module load)", () => {
    // Simulated by ensuring hook does not throw when window is defined but
    // sessionStorage is mocked to throw; real SSR guarded by typeof check.
    const spy = vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("ssr");
    });
    expect(() => renderHook(() => useProductSelection("store-a"))).not.toThrow();
    spy.mockRestore();
  });
});
```

- [ ] **Step 2: Run red**

```bash
cd apps/admin
npm run test -- useProductSelection
```

Expected: all tests fail (hook does not exist).

- [ ] **Step 3: Implement the hook**

```typescript
// apps/admin/lib/products/useProductSelection.ts
import { useCallback, useEffect, useState } from "react";

const MAX_SELECTION = 100;

const storageKey = (storeId: string) => `products.selection.${storeId}`;

const readStorage = (storeId: string): string[] => {
  if (typeof window === "undefined") return [];
  try {
    const raw = window.sessionStorage.getItem(storageKey(storeId));
    if (!raw) return [];
    const parsed = JSON.parse(raw);
    return Array.isArray(parsed) ? parsed.filter((x): x is string => typeof x === "string") : [];
  } catch {
    return [];
  }
};

const writeStorage = (storeId: string, ids: string[]) => {
  if (typeof window === "undefined") return;
  try {
    if (ids.length === 0) window.sessionStorage.removeItem(storageKey(storeId));
    else window.sessionStorage.setItem(storageKey(storeId), JSON.stringify(ids));
  } catch {
    /* ignore quota/privacy errors */
  }
};

export interface ProductSelection {
  selectedIds: string[];
  count: number;
  isSelected: (id: string) => boolean;
  toggle: (id: string) => void;
  add: (id: string) => boolean;
  remove: (id: string) => void;
  clearAll: () => void;
  capReached: boolean;
}

export function useProductSelection(storeId: string): ProductSelection {
  const [ids, setIds] = useState<string[]>(() => readStorage(storeId));

  // Re-hydrate on store switch
  useEffect(() => {
    setIds(readStorage(storeId));
  }, [storeId]);

  const persist = useCallback(
    (next: string[]) => {
      setIds(next);
      writeStorage(storeId, next);
    },
    [storeId],
  );

  const add = useCallback(
    (id: string): boolean => {
      let accepted = true;
      setIds((prev) => {
        if (prev.includes(id)) return prev;
        if (prev.length >= MAX_SELECTION) {
          accepted = false;
          return prev;
        }
        const next = [...prev, id];
        writeStorage(storeId, next);
        return next;
      });
      return accepted;
    },
    [storeId],
  );

  const remove = useCallback(
    (id: string) => {
      setIds((prev) => {
        const next = prev.filter((x) => x !== id);
        writeStorage(storeId, next);
        return next;
      });
    },
    [storeId],
  );

  const toggle = useCallback(
    (id: string) => {
      if (ids.includes(id)) remove(id);
      else add(id);
    },
    [ids, add, remove],
  );

  const clearAll = useCallback(() => persist([]), [persist]);

  const isSelected = useCallback((id: string) => ids.includes(id), [ids]);

  return {
    selectedIds: ids,
    count: ids.length,
    isSelected,
    toggle,
    add,
    remove,
    clearAll,
    capReached: ids.length >= MAX_SELECTION,
  };
}
```

- [ ] **Step 4: Run green**

```bash
npm run test -- useProductSelection
```

Expected: all tests pass.

- [ ] **Step 5: Commit**

```bash
git add apps/admin/lib/products/useProductSelection.ts apps/admin/lib/products/useProductSelection.test.ts
git commit -m "feat(admin): add useProductSelection sessionStorage-backed hook (M7d)"
```

---

### Task 3: `CopyToStoreDialog` component + tests (C)

**Files:**
- Create: `apps/admin/components/products/CopyToStoreDialog.tsx`
- Create: `apps/admin/components/products/CopyToStoreDialog.test.tsx`

**Scope:** The dialog UI only. Reads target stores from a prop (tests do not need server session). Supports `mode: "single" | "bulk"`. Renders a radio list of target stores, a "Also copy media" toggle (default on), and a static editorial info row. Submit fires a prop callback; integration with server actions is Task 4.

- [ ] **Step 1: Write the failing test**

```typescript
// apps/admin/components/products/CopyToStoreDialog.test.tsx
import { describe, it, expect, vi } from "vitest";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { CopyToStoreDialog } from "./CopyToStoreDialog";

const stores = [
  { id: "s1", name: "Main Store", role: "owner" as const },
  { id: "s2", name: "Outlet", role: "admin" as const },
  { id: "s3", name: "Read-only Store", role: "staff" as const },
  { id: "current", name: "Current Store", role: "owner" as const },
];

describe("CopyToStoreDialog", () => {
  it("filters out current store and staff-only stores", () => {
    render(
      <CopyToStoreDialog
        open
        onOpenChange={() => {}}
        mode="single"
        productName="T-shirt"
        currentStoreId="current"
        stores={stores}
        onSubmit={vi.fn()}
      />,
    );
    expect(screen.getByText("Main Store")).toBeInTheDocument();
    expect(screen.getByText("Outlet")).toBeInTheDocument();
    expect(screen.queryByText("Read-only Store")).not.toBeInTheDocument();
    expect(screen.queryByText("Current Store")).not.toBeInTheDocument();
  });

  it("shows single-mode title with product name", () => {
    render(
      <CopyToStoreDialog
        open
        onOpenChange={() => {}}
        mode="single"
        productName="T-shirt"
        currentStoreId="current"
        stores={stores}
        onSubmit={vi.fn()}
      />,
    );
    expect(screen.getByRole("heading", { name: /Copy "T-shirt" to another store/i })).toBeInTheDocument();
  });

  it("shows bulk-mode title with count, not ids", () => {
    render(
      <CopyToStoreDialog
        open
        onOpenChange={() => {}}
        mode="bulk"
        bulkCount={47}
        currentStoreId="current"
        stores={stores}
        onSubmit={vi.fn()}
      />,
    );
    expect(screen.getByRole("heading", { name: /Copy 47 products to another store/i })).toBeInTheDocument();
  });

  it("defaults copy_media to true", () => {
    render(
      <CopyToStoreDialog
        open
        onOpenChange={() => {}}
        mode="single"
        productName="T-shirt"
        currentStoreId="current"
        stores={stores}
        onSubmit={vi.fn()}
      />,
    );
    expect(screen.getByRole("switch", { name: /Also copy media/i })).toBeChecked();
  });

  it("renders static info row (not a toggle)", () => {
    render(
      <CopyToStoreDialog
        open
        onOpenChange={() => {}}
        mode="single"
        productName="T-shirt"
        currentStoreId="current"
        stores={stores}
        onSubmit={vi.fn()}
      />,
    );
    expect(
      screen.getByText(/Copied products are published as drafts in the target store/i),
    ).toBeInTheDocument();
  });

  it("submit fires onSubmit with target_store_id + copy_media", async () => {
    const onSubmit = vi.fn();
    const user = userEvent.setup();
    render(
      <CopyToStoreDialog
        open
        onOpenChange={() => {}}
        mode="single"
        productName="T-shirt"
        currentStoreId="current"
        stores={stores}
        onSubmit={onSubmit}
      />,
    );
    await user.click(screen.getByRole("radio", { name: /Outlet/i }));
    await user.click(screen.getByRole("switch", { name: /Also copy media/i })); // toggle off
    await user.click(screen.getByRole("button", { name: /^Copy$/ }));
    expect(onSubmit).toHaveBeenCalledWith({ target_store_id: "s2", copy_media: false });
  });

  it("submit is disabled until a target store is selected", () => {
    render(
      <CopyToStoreDialog
        open
        onOpenChange={() => {}}
        mode="single"
        productName="T-shirt"
        currentStoreId="current"
        stores={stores}
        onSubmit={vi.fn()}
      />,
    );
    expect(screen.getByRole("button", { name: /^Copy$/ })).toBeDisabled();
  });

  it("shows empty state when no eligible stores", () => {
    render(
      <CopyToStoreDialog
        open
        onOpenChange={() => {}}
        mode="single"
        productName="T-shirt"
        currentStoreId="current"
        stores={[{ id: "current", name: "Current Store", role: "owner" }]}
        onSubmit={vi.fn()}
      />,
    );
    expect(screen.getByText(/no other stores available/i)).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run red**

```bash
npm run test -- CopyToStoreDialog
```

- [ ] **Step 3: Implement the component (scaffold)**

```typescript
// apps/admin/components/products/CopyToStoreDialog.tsx
"use client";
import { useState, useMemo } from "react";
import { Dialog } from "@tesserix/web";

type Role = "owner" | "admin" | "staff";
export interface StoreOption { id: string; name: string; role: Role }

type Props =
  | {
      open: boolean;
      onOpenChange: (open: boolean) => void;
      mode: "single";
      productName: string;
      currentStoreId: string;
      stores: StoreOption[];
      onSubmit: (v: { target_store_id: string; copy_media: boolean }) => void;
      submitting?: boolean;
    }
  | {
      open: boolean;
      onOpenChange: (open: boolean) => void;
      mode: "bulk";
      bulkCount: number;
      currentStoreId: string;
      stores: StoreOption[];
      onSubmit: (v: { target_store_id: string; copy_media: boolean }) => void;
      submitting?: boolean;
    };

export function CopyToStoreDialog(props: Props) {
  const [targetId, setTargetId] = useState<string>("");
  const [copyMedia, setCopyMedia] = useState(true);

  const eligible = useMemo(
    () =>
      props.stores.filter(
        (s) => s.id !== props.currentStoreId && (s.role === "owner" || s.role === "admin"),
      ),
    [props.stores, props.currentStoreId],
  );

  const title =
    props.mode === "single"
      ? `Copy "${props.productName}" to another store`
      : `Copy ${props.bulkCount} products to another store`;

  // Render: Dialog from @tesserix/web
  // - heading (Source Serif 4 --text-lg)
  // - radio list of eligible stores (or empty state)
  // - Switch: "Also copy media" (default checked)
  // - Info row: "Copied products are published as drafts in the target store."
  // - Footer: Cancel + Copy (primary, Moss accent, disabled until targetId set)
  // Tokens: --paper-200 bg, --ink-900 text, --moss-700 accent, hairline rule
  return (
    /* ... dialog markup — see component file */ null as unknown as JSX.Element
  );
}
```

- [ ] **Step 4: Run green**

```bash
npm run test -- CopyToStoreDialog
```

- [ ] **Step 5: Commit**

```bash
git add apps/admin/components/products/CopyToStoreDialog.tsx apps/admin/components/products/CopyToStoreDialog.test.tsx
git commit -m "feat(admin): add CopyToStoreDialog component (M7d)"
```

---

### Task 4: `copyProductAction` server action + API client wiring (C/I)

**Files:**
- Modify: `apps/admin/lib/api/marketplace-api.ts`
- Modify: `apps/admin/app/products/actions.ts`
- Modify: `apps/admin/app/products/actions.test.ts`
- Modify: `apps/admin/components/products/ProductsList.tsx` (wire dialog from overflow menu)

**Scope:** Connect the dialog to the backend. Add a `copyProduct` client to `marketplace-api.ts` matching the existing pattern (reference: how M7b `updateProduct` is wired). Add a `copyProductAction` server action that calls it and revalidates the list. Mount the dialog in `ProductsList.tsx` from the existing M7a overflow menu "Copy to store…" stub.

- [ ] **Step 1: Write the failing server-action test**

```typescript
// excerpt — apps/admin/app/products/actions.test.ts
import { describe, it, expect, vi } from "vitest";
import { copyProductAction } from "./actions";
import * as api from "@/lib/api/marketplace-api";

vi.mock("@/lib/api/marketplace-api");

describe("copyProductAction", () => {
  it("calls marketplace-api.copyProduct and returns new product id", async () => {
    (api.copyProduct as ReturnType<typeof vi.fn>).mockResolvedValue({
      new_product_id: "new-1",
      new_store_id: "s2",
    });
    const res = await copyProductAction({
      source_store_id: "s1",
      product_id: "p1",
      target_store_id: "s2",
      copy_media: true,
    });
    expect(res).toEqual({ ok: true, new_product_id: "new-1", new_store_id: "s2" });
    expect(api.copyProduct).toHaveBeenCalledWith(
      expect.objectContaining({ source_store_id: "s1", product_id: "p1", target_store_id: "s2", copy_media: true }),
    );
  });

  it("returns typed error on apperror", async () => {
    (api.copyProduct as ReturnType<typeof vi.fn>).mockRejectedValue({
      code: "forbidden",
      message: "not allowed",
    });
    const res = await copyProductAction({
      source_store_id: "s1",
      product_id: "p1",
      target_store_id: "s2",
      copy_media: true,
    });
    expect(res).toEqual({ ok: false, error: "forbidden", message: "not allowed" });
  });
});
```

- [ ] **Step 2: Run red**

```bash
npm run test -- actions
```

- [ ] **Step 3: Implement the client + action**

```typescript
// apps/admin/lib/api/marketplace-api.ts (add)
export async function copyProduct(args: {
  source_store_id: string;
  product_id: string;
  target_store_id: string;
  copy_media: boolean;
}, session: SessionHeaders): Promise<{ new_product_id: string; new_store_id: string }> {
  return fetchJson(`/api/v1/admin/stores/${args.source_store_id}/products/${args.product_id}/copy`, {
    method: "POST",
    headers: sessionHeaders(session),
    body: JSON.stringify({ target_store_id: args.target_store_id, copy_media: args.copy_media }),
  });
}
```

```typescript
// apps/admin/app/products/actions.ts (add)
"use server";
export async function copyProductAction(input: {
  source_store_id: string;
  product_id: string;
  target_store_id: string;
  copy_media: boolean;
}) {
  const session = await getServerSession();
  try {
    const { new_product_id, new_store_id } = await copyProduct(input, session);
    revalidatePath(`/products`);
    return { ok: true as const, new_product_id, new_store_id };
  } catch (err: any) {
    return { ok: false as const, error: err.code ?? "unknown", message: err.message ?? "Copy failed" };
  }
}
```

- [ ] **Step 4: Wire in `ProductsList.tsx`**

The overflow menu "Copy to store…" item (stubbed in M7a) now opens `<CopyToStoreDialog mode="single" ...>`. On submit, call `copyProductAction`, show toast "Copied to {store}" with a link to `/products/{new_product_id}` (across stores — use the new store's subdomain route). Toast on error uses `result.message`.

- [ ] **Step 5: Run green**

```bash
npm run test -- actions CopyToStoreDialog ProductsList
```

- [ ] **Step 6: Commit**

```bash
git add apps/admin/lib/api/marketplace-api.ts apps/admin/app/products/actions.ts apps/admin/app/products/actions.test.ts apps/admin/components/products/ProductsList.tsx
git commit -m "feat(admin): wire copyProductAction to CopyToStoreDialog from list overflow menu (M7d)"
```

---

### Task 5: `BulkActionsBar` component + role gating (C)

**Files:**
- Create: `apps/admin/components/products/BulkActionsBar.tsx`
- Create: `apps/admin/components/products/BulkActionsBar.test.tsx`

**Scope:** The sticky bottom bar. Pure presentation + role gating. All action handlers come in as props. Tests drive role visibility and action button wiring — NOT the actual server calls (those are Tasks 6–9).

- [ ] **Step 1: Write the failing test**

```typescript
// apps/admin/components/products/BulkActionsBar.test.tsx
import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { BulkActionsBar } from "./BulkActionsBar";

const noopHandlers = {
  onArchive: vi.fn(),
  onUnarchive: vi.fn(),
  onPublish: vi.fn(),
  onUnpublish: vi.fn(),
  onAssignCategory: vi.fn(),
  onCopyToStore: vi.fn(),
  onDelete: vi.fn(),
  onClear: vi.fn(),
};

describe("BulkActionsBar", () => {
  it("renders null when count is 0", () => {
    const { container } = render(
      <BulkActionsBar count={0} role="owner" {...noopHandlers} />,
    );
    expect(container).toBeEmptyDOMElement();
  });

  it("renders null for staff role regardless of count", () => {
    const { container } = render(
      <BulkActionsBar count={5} role="staff" {...noopHandlers} />,
    );
    expect(container).toBeEmptyDOMElement();
  });

  it("shows count in the bar", () => {
    render(<BulkActionsBar count={7} role="admin" {...noopHandlers} />);
    expect(screen.getByText(/7 selected/i)).toBeInTheDocument();
  });

  it("admin sees all actions except delete", () => {
    render(<BulkActionsBar count={1} role="admin" {...noopHandlers} />);
    expect(screen.getByRole("button", { name: /Archive/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Unarchive/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Publish/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Unpublish/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Assign category/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Copy to store/i })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^Delete$/i })).not.toBeInTheDocument();
  });

  it("owner sees delete", () => {
    render(<BulkActionsBar count={1} role="owner" {...noopHandlers} />);
    expect(screen.getByRole("button", { name: /^Delete$/i })).toBeInTheDocument();
  });

  it("clicking an action calls its handler with no args (ids come from hook)", async () => {
    const user = userEvent.setup();
    render(<BulkActionsBar count={3} role="admin" {...noopHandlers} />);
    await user.click(screen.getByRole("button", { name: /Archive/i }));
    expect(noopHandlers.onArchive).toHaveBeenCalledTimes(1);
  });

  it("clear-selection button calls onClear", async () => {
    const user = userEvent.setup();
    render(<BulkActionsBar count={3} role="admin" {...noopHandlers} />);
    await user.click(screen.getByRole("button", { name: /Clear selection/i }));
    expect(noopHandlers.onClear).toHaveBeenCalledTimes(1);
  });
});
```

- [ ] **Step 2: Run red**

```bash
npm run test -- BulkActionsBar
```

- [ ] **Step 3: Implement**

```typescript
// apps/admin/components/products/BulkActionsBar.tsx
"use client";
import type { ReactNode } from "react";

export type BulkRole = "staff" | "admin" | "owner";

export interface BulkActionsBarProps {
  count: number;
  role: BulkRole;
  onArchive: () => void;
  onUnarchive: () => void;
  onPublish: () => void;
  onUnpublish: () => void;
  onAssignCategory: () => void;
  onCopyToStore: () => void;
  onDelete: () => void;
  onClear: () => void;
}

export function BulkActionsBar(props: BulkActionsBarProps): ReactNode {
  if (props.count === 0) return null;
  if (props.role === "staff") return null;

  const isOwner = props.role === "owner";
  // Sticky bottom, Paper surface, hairline top rule (border-t via --border token),
  // --paper-200 bg, --ink-900 text, --moss-700 accent for primary actions,
  // single --shadow-1 elevation. Layout: count on the left, action row center,
  // clear X on the right.
  return (
    /* ... see component file */ null
  );
}
```

- [ ] **Step 4: Run green**

```bash
npm run test -- BulkActionsBar
```

- [ ] **Step 5: Commit**

```bash
git add apps/admin/components/products/BulkActionsBar.tsx apps/admin/components/products/BulkActionsBar.test.tsx
git commit -m "feat(admin): add BulkActionsBar with role gating (M7d)"
```

---

### Task 6: `bulkProductAction` server action — archive/unarchive/publish/unpublish (C/I)

**Files:**
- Create: `apps/admin/lib/validation/bulk-action.ts`
- Create: `apps/admin/lib/validation/bulk-action.test.ts`
- Modify: `apps/admin/lib/api/marketplace-api.ts` (add `bulkProductAction` client)
- Modify: `apps/admin/app/products/actions.ts` (add `bulkProductAction` server action)
- Modify: `apps/admin/app/products/actions.test.ts`
- Modify: `apps/admin/components/products/ProductsList.tsx` (wire bar handlers for 4 actions)

**Scope:** The four reversible state-change actions. Category, copy, delete are Tasks 7–9. The Zod schema enforces the 100-id cap at the server-action boundary. The client action calls the backend, receives `{results: [...]}`, and returns a summary `{ok: successCount, failed: errorCount, errors: [{id, error}]}` to the caller. `ProductsList.tsx` then renders a toast using the summary.

- [ ] **Step 1: Write the failing test**

```typescript
// apps/admin/app/products/actions.test.ts (add)
import { bulkProductAction } from "./actions";
import * as api from "@/lib/api/marketplace-api";

describe("bulkProductAction", () => {
  it("rejects >100 ids at validation boundary", async () => {
    const ids = Array.from({ length: 101 }, (_, i) => `p${i}`);
    const res = await bulkProductAction({ store_id: "s1", action: "archive", product_ids: ids });
    expect(res.ok).toBe(false);
    expect(res.error).toBe("bulk_cap_exceeded");
    expect(api.bulkProductAction).not.toHaveBeenCalled();
  });

  it("summarizes partial success", async () => {
    (api.bulkProductAction as ReturnType<typeof vi.fn>).mockResolvedValue({
      results: [
        { id: "p1", status: "ok" },
        { id: "p2", status: "error", error: "forbidden" },
        { id: "p3", status: "ok" },
      ],
    });
    const res = await bulkProductAction({
      store_id: "s1",
      action: "archive",
      product_ids: ["p1", "p2", "p3"],
    });
    expect(res).toEqual({
      ok: true,
      succeeded: 2,
      failed: 1,
      errors: [{ id: "p2", error: "forbidden" }],
    });
  });
});
```

- [ ] **Step 2: Run red**

```bash
npm run test -- actions
```

- [ ] **Step 3: Implement**

Zod schema:

```typescript
// apps/admin/lib/validation/bulk-action.ts
import { z } from "zod";

export const BULK_ACTIONS = [
  "archive",
  "unarchive",
  "publish",
  "unpublish",
  "assign_category",
  "copy",
  "delete",
] as const;

export const bulkActionSchema = z.object({
  store_id: z.string().uuid(),
  action: z.enum(BULK_ACTIONS),
  product_ids: z.array(z.string().uuid()).min(1).max(100, { message: "bulk_cap_exceeded" }),
  params: z.record(z.any()).optional(),
});

export type BulkActionInput = z.infer<typeof bulkActionSchema>;
```

API client + server action follow the existing `updateProductAction` pattern from M7b. On exception, return `{ok: false, error, message}`. On success, summarize results into `{ok: true, succeeded, failed, errors[]}`.

- [ ] **Step 4: Wire the four buttons in `ProductsList.tsx`**

Each handler calls `bulkProductAction` then:
- Optimistically removes/updates rows based on the per-id results (only rows with `status: "ok"`)
- Renders toast: `"Archived 90 of 100. 10 failed."` with "View errors" affordance if `failed > 0`
- Calls `selection.clearAll()` on any non-failed result

- [ ] **Step 5: Run green**

```bash
npm run test -- actions BulkActionsBar ProductsList bulk-action
```

- [ ] **Step 6: Commit**

```bash
git add apps/admin/lib/validation/bulk-action.ts apps/admin/lib/validation/bulk-action.test.ts apps/admin/lib/api/marketplace-api.ts apps/admin/app/products/actions.ts apps/admin/app/products/actions.test.ts apps/admin/components/products/ProductsList.tsx
git commit -m "feat(admin): bulk archive/unarchive/publish/unpublish actions (M7d)"
```

---

### Task 7: Bulk delete (owner only) + hard-delete confirm dialog (C)

**Files:**
- Create: `apps/admin/components/products/BulkDeleteConfirmDialog.tsx`
- Create: `apps/admin/components/products/BulkDeleteConfirmDialog.test.tsx`
- Modify: `apps/admin/components/products/ProductsList.tsx`

**Scope:** The one remaining dialog permitted by §13.5. Owner-only. Requires typing the word `delete` in a confirmation input (matching the M7b single-delete confirm pattern). Submits via `bulkProductAction` with `action: "delete"`. Per-id errors are summarized in the same toast pattern as Task 6.

- [ ] **Step 1: Write the failing test**

```typescript
// apps/admin/components/products/BulkDeleteConfirmDialog.test.tsx
describe("BulkDeleteConfirmDialog", () => {
  it("delete button is disabled until user types 'delete'", async () => {
    const user = userEvent.setup();
    render(<BulkDeleteConfirmDialog open count={5} onConfirm={vi.fn()} onOpenChange={() => {}} />);
    const btn = screen.getByRole("button", { name: /Delete 5 products/i });
    expect(btn).toBeDisabled();
    await user.type(screen.getByLabelText(/Type delete to confirm/i), "delete");
    expect(btn).toBeEnabled();
  });

  it("confirm fires callback", async () => {
    const onConfirm = vi.fn();
    const user = userEvent.setup();
    render(<BulkDeleteConfirmDialog open count={5} onConfirm={onConfirm} onOpenChange={() => {}} />);
    await user.type(screen.getByLabelText(/Type delete to confirm/i), "delete");
    await user.click(screen.getByRole("button", { name: /Delete 5 products/i }));
    expect(onConfirm).toHaveBeenCalled();
  });

  it("warns this cannot be undone", () => {
    render(<BulkDeleteConfirmDialog open count={5} onConfirm={vi.fn()} onOpenChange={() => {}} />);
    expect(screen.getByText(/cannot be undone/i)).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run red → implement → run green → Step 3: Commit**

```bash
git add apps/admin/components/products/BulkDeleteConfirmDialog.tsx apps/admin/components/products/BulkDeleteConfirmDialog.test.tsx apps/admin/components/products/ProductsList.tsx
git commit -m "feat(admin): bulk delete with hard-delete confirm dialog (owner only, M7d)"
```

---

### Task 8: Bulk category assign (reuse M7b picker) (C)

**Files:**
- Create: `apps/admin/components/products/BulkCategoryAssignPopover.tsx`
- Create: `apps/admin/components/products/BulkCategoryAssignPopover.test.tsx`
- Modify: `apps/admin/components/products/ProductsList.tsx`

**Scope:** A popover (NOT a dialog — dialog budget is spent) anchored to the "Assign category" button on the bulk bar. Wraps the existing M7b `ProductCategoriesPicker` with a submit footer. Does not allow editing existing category selections per product — it APPENDS the chosen categories to every selected product (union mode). Server action reuses `bulkProductAction` with `action: "assign_category"` and `params: { category_ids }`.

- [ ] **Step 1 → Step 5:** Failing test → red → implement (scaffold popover, re-export picker) → green → commit

```bash
git add apps/admin/components/products/BulkCategoryAssignPopover.tsx apps/admin/components/products/BulkCategoryAssignPopover.test.tsx apps/admin/components/products/ProductsList.tsx
git commit -m "feat(admin): bulk category assign popover reusing M7b picker (M7d)"
```

---

### Task 9: Bulk copy-to-store (reuse `CopyToStoreDialog`) (C)

**Files:**
- Modify: `apps/admin/components/products/ProductsList.tsx`
- Modify: `apps/admin/components/products/CopyToStoreDialog.test.tsx` (add bulk-mode integration cases if needed)

**Scope:** The bar's "Copy to store" button mounts `<CopyToStoreDialog mode="bulk" bulkCount={count} ...>` and on submit calls `bulkProductAction` with `action: "copy", params: { target_store_id, copy_media }`. Toast on mixed results: `"Copied 45 of 50 to {store}. 5 failed."`. Errors listed inline. Selection clears on non-empty success.

- [ ] **Step 1 → Step 5:** Failing test → red → implement → green → commit

```bash
git add apps/admin/components/products/ProductsList.tsx apps/admin/components/products/CopyToStoreDialog.test.tsx
git commit -m "feat(admin): bulk copy-to-store via CopyToStoreDialog bulk mode (M7d)"
```

---

### Task 10: Playwright E2E — copy flow + bulk flow (E)

**Files:**
- Create: `apps/admin/tests/e2e/products-copy-to-store.spec.ts`
- Create: `apps/admin/tests/e2e/products-bulk-actions.spec.ts`

**Scope:** Two E2E flows running against the dev stack (marketplace-api + admin + Postgres). Fixtures seed a user with admin role on 2 stores + owner on a third, and 5 products in the current store.

**E2E 1 — copy-to-store (single):**
1. Visit `/products`
2. Open overflow menu on row "T-shirt"
3. Click "Copy to store…"
4. Assert dialog title "Copy \"T-shirt\" to another store"
5. Assert "Also copy media" is checked by default
6. Select target store "Outlet"
7. Click Copy
8. Assert toast "Copied to Outlet" with link
9. Navigate via the link and assert the product exists as `status=draft`

**E2E 2 — bulk archive with partial failure:**
1. Seed 3 products: 2 with archive permission, 1 without (staff-owned via FGA)
2. Select all 3 checkboxes on the list page
3. Assert bulk bar appears with "3 selected"
4. Click Archive
5. Assert toast "Archived 2 of 3. 1 failed." with "View errors" affordance
6. Click "View errors" → assert the forbidden id + "forbidden" reason visible
7. Assert the 2 authorized rows re-render as archived
8. Reload the page; assert selection has cleared (sessionStorage cleared after bulk success)

- [ ] **Step 1 → Step 4:** Write tests → run red → run against dev stack (tests are part of the plan doc, not executed by the plan author) → commit

```bash
git add apps/admin/tests/e2e/products-copy-to-store.spec.ts apps/admin/tests/e2e/products-bulk-actions.spec.ts
git commit -m "test(admin): e2e copy-to-store + bulk archive flows (M7d)"
```

---

### Task 11: Impeccable chain pass + verification + PR

**Files:** no code — verification + PR creation only.

**Scope:** Run the full impeccable chain against the new surfaces. Gate on `critique` score ≥ 7.5. Address any P0/P1 issues. Then open the PR.

- [ ] **Step 1: `frontend-design` review**

Run the `frontend-design` skill against `BulkActionsBar`, `CopyToStoreDialog`, `BulkDeleteConfirmDialog`, `BulkCategoryAssignPopover`. Capture notes.

- [ ] **Step 2: `critique`**

Score the combined M7d surface. Threshold: ≥ 7.5. Below threshold blocks PR — address issues, re-run.

- [ ] **Step 3: `polish` → `arrange` → `typeset` → `audit` → `adapt`**

Run each in sequence. Fix issues as they arise. Each fix gets its own atomic commit.

- [ ] **Step 4: Verification**

```bash
cd apps/admin
npm run lint
npm run typecheck
npm run test
npx playwright test tests/e2e/products-copy-to-store.spec.ts tests/e2e/products-bulk-actions.spec.ts

cd services/marketplace-api
go test ./... -race
golangci-lint run
```

Expected: all green.

- [ ] **Step 5: Delete the gap log**

```bash
rm .planning/m7d-backend-gaps.md
git add -u .planning/
git commit -m "chore(superpowers): drop M7d gap log"
```

- [ ] **Step 6: Open PR**

```bash
gh pr create --base main --title "feat(products): M7d admin UI — copy-to-store + bulk actions" --body "$(cat <<'EOF'
## Summary
- Add CopyToStoreDialog (single + bulk modes) wired from list overflow menu and bulk bar
- Add BulkActionsBar with role-gated actions (archive/unarchive/publish/unpublish/assign category/copy/delete)
- Add useProductSelection sessionStorage-backed hook with 100-id cap
- Backend: per-id FGA enforcement in BulkApply + 100-id cap + partial-success semantics (see Task 1 exit matrix in plan)

## Test plan
- [ ] Vitest: useProductSelection, CopyToStoreDialog, BulkActionsBar, BulkDeleteConfirmDialog, BulkCategoryAssignPopover, actions
- [ ] Playwright: products-copy-to-store.spec.ts, products-bulk-actions.spec.ts
- [ ] Go: marketplace-api ./... -race
- [ ] Manual: copy single product across stores as admin; bulk archive 50 products including 5 forbidden rows; bulk delete 5 products as owner with typed confirm
EOF
)"
```

---

## Exit criteria (milestone-level)

- Task 1 exit matrix: 8/8 rows ✅
- All Vitest suites green, all Playwright specs green
- `go test ./... -race` green in `services/marketplace-api`
- `critique` score ≥ 7.5 on the combined M7d surfaces
- No new hex values; Paper · Ink · Moss tokens only
- No new dialog surfaces beyond `CopyToStoreDialog` and `BulkDeleteConfirmDialog`
- CSV export still deferred — confirmed not touched by M7d
- PR open against `main` with the test plan checklist
