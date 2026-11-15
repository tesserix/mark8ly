# Cashier role in the admin (#1104) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** An owner can invite someone as a Cashier, who signs in to the sale screen and nothing else, sees their own sales today, and can give discounts only when the store allows it.

**Architecture:** The server side shipped in #1112: the FGA `cashier` relation and `can_sell_in_store`, platform-api accepting `cashier` invites and role changes, marketplace-api's in-store routes (the only ones a cashier passes) including `GET /in-store/sales` (the caller's sales today), and `stores.cashier_discounts_enabled` (owned by marketplace-api, not synced). This plan adds what is missing: an endpoint to read and set that flag, the caller's `can_discount` on the catalogue, and the admin side (role plumbing, cashier-only routing and shell, the invite option, the setting, and "Your sales today").

**Tech Stack:** Go (gin, gorm), Next.js admin (middleware, server components, vitest).

**Spec:** `docs/superpowers/specs/2026-11-14-in-store-sales-design.md` ("Roles")

## Global Constraints

- "A cashier **can**: open the sale screen, read active products and stock, create in-store orders, send or print receipts for them, and list the sales they made today."
- "A cashier **cannot**: edit products or prices, see other orders or any customer list, see reports or settings, refund, invite members."
- Store setting "Cashiers can give discounts and custom prices", off by default.
- Cashier is a tenant-level role only (store-scoped invites stay manager/staff/viewer).
- The server is the authority on every permission; the admin hides what the server would refuse.
- Receipts (send/print) are #1106/#1107, not this plan.

## Review Focus

1. A cashier typing `/orders`, `/settings/team` or `/dashboard` directly → lands on `/sell`, never sees the page. → Task 2 test `cashierPath redirects every other admin page`.
2. A cashier's sale-screen server actions (POST to `/sell`) and sign-out (`/logout`) must still work. → Task 2 test `cashierPath allows the sale screen, its actions and sign-out`.
3. The discount and custom-item controls match the server: hidden for a cashier when the setting is off, shown when on. → Task 1 test `TestInStoreCatalog_CanDiscount` and Task 3 test.
4. A cashier with a locked sale is told to check "Your sales today", not Orders (which they cannot open). → Task 3 test.
5. Only owner and admin can change the cashier discount setting. → Task 1 test `TestInStoreSettings_OwnerAdminOnly`.

---

### Task 1: API — discount setting endpoint, can_discount on the catalogue

**Files:**
- Modify: `services/marketplace-api/internal/handlers/admin/instore.go`, `routes.go`
- Modify: `services/marketplace-api/internal/stores/repository.go` (method `SetCashierDiscounts(ctx, storeID string, on bool) error`, a single-column UPDATE)
- Test: `internal/handlers/admin/instore_test.go`, an integration test for the setter
- Regenerate: `route-manifest.json`

**Interfaces:**
- Produces: `GET /stores/:storeId/in-store/catalog` response gains top-level `"can_discount": bool` (computed by the existing `canDiscount`).
- Produces: `GET /stores/:storeId/in-store/settings` → `{"cashier_discounts_enabled": bool}`; `PUT` same body → 200 with the saved value. Both `RequireTenantRelation(authz.RoleAdmin)`, mounted on `storeRoute` directly, not inside the cashier-reachable `inStore` group.

- [ ] **Step 1: Failing tests**: `TestInStoreCatalog_CanDiscount` (cashier + flag off → false; cashier + flag on → true; staff → true); `TestInStoreSettings_OwnerAdminOnly` (staff and cashier → 403; admin PUT true → 200 and GET returns true); integration `TestIntegration_Stores_SetCashierDiscounts` (flag persists and survives `Upsert` of the same store).
- [ ] **Step 2:** Run → FAIL.
- [ ] **Step 3:** Implement; regenerate the route manifest with the repo's generator.
- [ ] **Step 4:** `go test ./internal/handlers/admin/ -run 'InStore'` and the integration test with `-tags integration -p 1`. PASS.
- [ ] **Step 5:** Commit `feat(api): cashier discount setting and can_discount on the sale catalogue`.

### Task 2: Admin — cashier role and cashier-only routing

**Files:**
- Modify: `apps/admin/lib/api/platform-api.ts` (`TenantRole` adds `"cashier"`), `lib/auth/serverSession.ts` (`normalizeRole` keeps `cashier`), `middleware.ts` (role type; after the role fetch, `if (role === "cashier")` apply `cashierPath`)
- Create: `apps/admin/lib/auth/cashier.ts` with `cashierPath(pathname: string): "allow" | "redirect"`
- Test: `apps/admin/lib/auth/cashier.test.ts`, `serverSession` test if one exists

**Interfaces:**
- Produces: `cashierPath` — allows `/sell` (and sub-paths), `/logout`, and anything the middleware already treats as auth utility or static (reuse `isCanonicalAllowedPath` / `isStaticOrHealthPath` decisions by calling them in middleware before `cashierPath`, not inside it); everything else `"redirect"` to `/sell`.

- [ ] **Step 1: Failing tests**: `cashierPath allows the sale screen, its actions and sign-out` (`/sell`, `/sell?x=1` path `/sell`, `/logout`); `cashierPath redirects every other admin page` (`/dashboard`, `/orders`, `/orders/abc`, `/settings/team`, `/products`, `/customers`, `/`); `normalizeRole keeps cashier` (was coerced to viewer, which would show read-only pages the API then refuses).
- [ ] **Step 2:** Run → FAIL.
- [ ] **Step 3:** Implement. Middleware redirect is a 307 to `/sell` on the same external origin (use the existing `externalOrigin(req)`).
- [ ] **Step 4:** Tests, tsc, lint.
- [ ] **Step 5:** Commit `feat(admin): cashier role signs in to the sale screen only`.

### Task 3: Admin — cashier shell, sale screen and "Your sales today"

**Files:**
- Modify: `apps/admin/components/shell/AdminShell.tsx` (cashier: nav is only In-store sale; logo links to `/sell`; no notification bell)
- Modify: `apps/admin/lib/api/instore-api.ts` (`getInStoreCatalog` returns `{ products, canDiscount }`; add `getMySalesToday(storeId, session): Promise<MySale[] | null>`)
- Modify: `apps/admin/app/(admin)/sell/page.tsx` (`canDiscount` from the catalogue, not the role; loads my sales)
- Create: `apps/admin/components/instore/MySalesToday.tsx`
- Modify: `apps/admin/components/instore/CartPanel.tsx` (locked warning wording takes a prop: cashiers "Check Your sales today below before discarding it…")
- Test: `AdminShell` cashier test, `instore-api` tests, `SaleScreen`/`MySalesToday` tests

**Interfaces:**
- Consumes: Task 1 `can_discount`; existing `GET /in-store/sales` → `{data: [{id, order_number, grand_total, currency_code, payment_method?, placed_at, receipt_email?}]}`.
- Produces: `type MySale = { id: string; order_number: string; grand_total: string; currency_code: string; payment_method?: string; placed_at: string }`; `MySalesToday({ sales, currencyCode })`.

- [ ] **Step 1: Failing tests**: shell `shows only In-store sale to a cashier`; `getInStoreCatalog returns can_discount`; `MySalesToday lists order number, time, method and total, newest first, or says no sales yet`; SaleScreen `tells a cashier to check their sales today when a sale is locked`; the sale screen refreshes the list after a recorded sale (reuses `router.refresh()`).
- [ ] **Step 2:** Run → FAIL.
- [ ] **Step 3:** Implement. Everyone who can sell sees "Your sales today" under the screen (useful for staff too).
- [ ] **Step 4:** Full admin suite, tsc, lint.
- [ ] **Step 5:** Commit `feat(admin): cashier shell, server-decided discounts, your sales today`.

### Task 4: Admin — invite a Cashier, and the discount setting

**Files:**
- Modify: `apps/admin/components/settings/TeamSettings.tsx` (tenant-scope invite options add `cashier`, labelled "Cashier (in-store sales only)"; role-change options add `cashier` for owner and admin; admins may edit cashier members)
- Modify: `apps/admin/lib/api/platform-api.ts` (`InviteRole` adds `"cashier"` if it is a separate union)
- Create: `apps/admin/components/settings/CashierDiscountsSetting.tsx` (switch, server action, shown on the Team page to owner/admin)
- Create/Modify: `apps/admin/lib/api/instore-api.ts` (`getInStoreSettings`, `setCashierDiscounts`), `app/(admin)/settings/team/actions.ts` (`setCashierDiscountsAction(enabled: boolean)` — owner/admin only)
- Test: TeamSettings test, setting component + action tests

- [ ] **Step 1: Failing tests**: `offers Cashier for a tenant-wide invite, not a store invite`; `an admin can move a member to and from cashier`; `CashierDiscountsSetting saves the switch and shows the saved state`; `setCashierDiscountsAction refuses staff`.
- [ ] **Step 2:** Run → FAIL.
- [ ] **Step 3:** Implement. Copy: "Cashiers can give discounts and custom prices" / "Off: only you, admins and staff can."
- [ ] **Step 4:** Full admin suite, tsc, lint.
- [ ] **Step 5:** Commit `feat(admin): invite cashiers, and let them discount when allowed`.

### Not in this plan

- Receipts a cashier can send or print: #1106/#1107.
- A store-scoped cashier (one store of several): the FGA model has `cashier` on the tenant only.
