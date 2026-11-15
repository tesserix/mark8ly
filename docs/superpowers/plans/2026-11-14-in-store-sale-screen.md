# In-store sale screen Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A tablet-first **In-store sale** screen in admin (#1105) that rings up catalogue and custom lines, shows the exact total before the seller keys it into the EFTPOS terminal, and records the sale only when the server confirms.

**Architecture:** One small backend addition — `POST /in-store/quote`, sharing `RecordSale`'s pricing, and an `expected_total` check on record — so the "Charge" amount is the server's number. The admin page is a server component that loads the catalogue; a client `SaleScreen` drives a pure cart module (`lib/instore/cart.ts`) and calls server actions (never `MARKETPLACE_API_URL` from the browser).

**Tech Stack:** Go/gin (marketplace-api), Next.js App Router server actions, React client components, vitest + Testing Library (jsdom) in `apps/admin`.

**Spec:** `docs/superpowers/specs/2026-11-14-in-store-sales-design.md` (issue #1105, part of #1101). Backend it builds on: #1112.

## Global Constraints

- "The app says **"paid" before the server has recorded the order.**" is an anti-goal: success shows only after the record call returns 200/201.
- "Client components must not call `MARKETPLACE_API_URL` (in-cluster); use server actions or same-origin routes, as with media actions (#1093)."
- "the cart is kept on the device (local storage), the screen shows "No connection, sale not recorded yet", and **Paid** is retried with the same idempotency key".
- "Only active products appear; archived and draft do not." (the catalogue endpoint already filters).
- Payment methods: Card (EFTPOS) `card_external`, Cash `cash` (tendered + change), Bank transfer `bank_transfer`, Other `other` with a label of at most 40 characters.
- Discounts and custom items only when the caller may (staff and above, or the store's `cashier_discounts_enabled`); the server enforces it, the screen hides the controls.
- Server actions take identity from middleware headers (`x-session-user-id`, `x-session-tenant-id`), never from the browser — copy `serverSession()` from `app/(admin)/products/media-actions.ts`.
- Money crosses the wire as decimal strings; the client never computes tax or the total it charges.

## Out of scope (this plan)

- **Personalisation fields on the sale screen.** The backend does not accept answers on in-store lines yet (deferred in #1112). The sale note covers "HP blah skin" for now.
- **Category tabs.** The catalogue endpoint returns no categories; search covers it until a seller asks. *Ruling made in planning; costs one endpoint field + a tab row if wrong.*
- **Email / Print receipt actions** (#1106, #1107). The done step shows the order number and **New sale**; those issues add their buttons to `SaleDone`.
- **Cashier-only admin** (#1104 UI half): the nav entry and a cashier landing only on `/sell`. This page works for owner, admin and staff today.

## Review Focus

1. **A lost response after the server recorded the sale.** Retrying must return the same order, never a second one, even if a price changed in between. → Task 1 `TestIntegration_InStore_RetryAfterPriceChangeReturnsFirstOrder`.
2. **Cart edited after an uncertain failure.** Editing would make the next Paid a new sale while the first may exist. The cart locks after a network failure until it confirms or the seller discards it. → Task 2 `locks the cart after an uncertain failure`.
3. **Price changed between Charge and Paid.** The seller already keyed the old amount; the server refuses (`total_changed`) and the screen shows the new total. → Task 1 `TestIntegration_InStore_TotalChangedRefused`, Task 4 `shows the new total when the server refuses a changed total`.
4. **Cash tendered below the total, or not a number.** Change is never negative; Paid is disabled. → Task 2 `change`.
5. **Reload mid-sale.** The cart, its idempotency key and its locked state come back from local storage. → Task 2 `persists and restores`.

---

### Task 1: Backend — quote and expected total

**Files:**
- Modify: `services/marketplace-api/internal/instore/service.go` (extract pricing), `internal/instore/types.go`
- Modify: `services/marketplace-api/internal/handlers/admin/instore.go`, `internal/handlers/admin/routes.go` (mount), `internal/routemanifest/route-manifest.json` (regenerate), route-parity web-only entry
- Test: `internal/instore/service_integration_test.go`, `internal/handlers/admin/instore_test.go`

**Interfaces:**
- Produces: `func (s *Service) Quote(ctx context.Context, in SaleInput) (*Quote, error)` with `type Quote struct { Subtotal, TaxTotal, DiscountTotal, GrandTotal decimal.Decimal; CurrencyCode string }`. Ignores payment fields and idempotency key.
- Produces: `SaleInput.ExpectedTotal *decimal.Decimal`; when set and ≠ computed grand total, `RecordSale` returns `TotalChangedError{Expected, Actual decimal.Decimal}` — unless an order already exists for `(store_id, idempotency_key)`, in which case it returns that order with `Reused: true`.
- Produces HTTP: `POST /admin/stores/:storeId/in-store/quote` (same body as sales minus payment fields) → `200 {subtotal, tax_total, discount_total, grand_total, currency_code}`; sales body gains `expected_total` (string); mismatch → `409 {"error":"total_changed","grand_total":"<actual>"}`. Same `can_sell_in_store` gate as the other in-store routes.

- [ ] **Step 1: Write failing tests.** `TestIntegration_InStore_QuoteMatchesRecordedTotal` (quote 2×20.00 with noTax → grand 40.00; record with `ExpectedTotal` 40.00 succeeds). `TestIntegration_InStore_TotalChangedRefused` (expected 39.00 → `TotalChangedError{Actual: 40.00}`, no order, stock unchanged). `TestIntegration_InStore_RetryAfterPriceChangeReturnsFirstOrder` (record with expected 40.00; `UPDATE product_variants SET price = 25.00`; retry same key and expected 40.00 → `Reused`, same order id). Handler: `TestInStoreQuote_200` and `TestInStoreSale_TotalChanged409` with the fake recorder.
- [ ] **Step 2: Run** `go test -tags integration ./internal/instore/... ./internal/handlers/admin/ -run 'InStore'` — Expected: FAIL (undefined `Quote`, `ExpectedTotal`).
- [ ] **Step 3: Implement.** Move the validate → price → discount check → tax → grand arithmetic out of `RecordSale` into one unexported `price(ctx, in)` used by both. For the reuse case look up `orders` by `store_id` and `idempotency_key` before returning `TotalChangedError` and load it through the order repository the same way `order.Service.Create` does on reuse. `InStoreRecorder` gains `Quote`.
- [ ] **Step 4: Run** the step-2 command plus `go test ./...` — Expected: PASS. Regenerate the route manifest (`go test ./internal/routemanifest -run TestRouteManifestMatchesMountedRoutes -update`) and add the parity entry; `TestCashierReachesOnlyInStoreRoutes` must still pass with the new route.
- [ ] **Step 5: Commit** `feat(instore): quote a sale and refuse a changed total (#1105)`.

### Task 2: Cart module

**Files:**
- Create: `apps/admin/lib/instore/cart.ts`
- Test: `apps/admin/lib/instore/cart.test.ts`

**Interfaces:**
- Produces:
  - `type CartLine = { key: string; variantId?: string; title: string; optionSummary?: string; imageUrl?: string; unitPrice: string; quantity: number; custom?: boolean }`
  - `type Cart = { lines: CartLine[]; discount: string; note: string; idempotencyKey: string | null; locked: boolean }`
  - `type CartAction = { type: "add"; line: Omit<CartLine,"key"|"quantity"> } | { type: "setQty"; key: string; quantity: number } | { type: "remove"; key: string } | { type: "discount"; value: string } | { type: "note"; value: string } | { type: "beginCharge" } | { type: "uncertain" } | { type: "refused" } | { type: "reset" }`
  - `function cartReducer(cart: Cart, action: CartAction): Cart`
  - `function subtotal(cart: Cart): string` (display estimate; two decimals)
  - `function change(tendered: string, total: string): string | null` (null when tendered is not a number or below total)
  - `function toSaleLines(cart: Cart): { variant_id?: string; custom_title?: string; custom_price?: string; quantity: number }[]`
  - `function loadCart(storeId: string): Cart` / `function saveCart(storeId: string, cart: Cart): void` — key `mark8ly:instore-cart:<storeId>`; corrupt JSON → empty cart.

- [ ] **Step 1: Write failing tests:** `add merges the same variant` (two adds → one line qty 2); `custom lines never merge`; `setQty 0 removes`; `beginCharge creates a key once` (second beginCharge keeps it); `locks the cart after an uncertain failure` (after `uncertain`, add/setQty/remove/discount return the cart unchanged; `reset` clears lines, key and lock); `refused clears the key and unlocks`; `change` (`change("50","38.50")` → `"11.50"`, `change("30","38.50")` → `null`, `change("abc","1")` → `null`); `persists and restores` (save then load returns equal cart incl. key and locked; corrupt storage → empty cart); `toSaleLines` maps custom lines to `custom_title`/`custom_price` and never sends a price for catalogue lines.
- [ ] **Step 2: Run** `cd apps/admin && npx vitest run lib/instore/cart.test.ts` — Expected: FAIL (module not found).
- [ ] **Step 3: Implement.** Money arithmetic in integer cents (strings → cents → string); `crypto.randomUUID()` for the key.
- [ ] **Step 4: Run** the step-2 command — Expected: PASS.
- [ ] **Step 5: Commit** `feat(admin): in-store cart state (#1105)`.

### Task 3: API client and server actions

**Files:**
- Create: `apps/admin/lib/api/instore-api.ts`, `apps/admin/app/(admin)/sell/actions.ts`
- Test: `apps/admin/lib/api/instore-api.test.ts`

**Interfaces:**
- Consumes: Task 1 HTTP; `SessionHeaders`, `MutationResult`, the header helpers in `lib/api/marketplace-api.ts` (export what is needed rather than copying).
- Produces:
  - `getInStoreCatalog(storeId, session): Promise<InStoreProduct[]>` (types mirror `InStoreCatalogProduct`/`Variant`)
  - `quoteInStoreSale(storeId, body, session): Promise<MutationResult<InStoreQuote>>`
  - `recordInStoreSale(storeId, body, session): Promise<SaleResult>` where `type SaleResult = { ok: true; orderId: string; orderNumber: string; grandTotal: string; reused: boolean } | { ok: false; kind: "out_of_stock"; variantId: string } | { ok: false; kind: "total_changed"; grandTotal: string } | { ok: false; kind: "rejected"; message: string } | { ok: false; kind: "unreachable" }`
  - Server actions `quoteSaleAction(storeId, body)` and `recordSaleAction(storeId, body)` wrapping them with `serverSession()`.
- `unreachable` = fetch threw, or a 5xx/timeout: the outcome is unknown. `rejected` = any other 4xx: definitely not recorded.

- [ ] **Step 1: Write failing tests** with `vi.stubGlobal("fetch", …)`: 201 → `ok` with `reused:false`; 200 → `reused:true`; 409 `out_of_stock` → `variantId`; 409 `total_changed` → `grandTotal`; 400 → `rejected` with the server message; fetch rejects → `unreachable`; 503 → `unreachable`. Assert the request sends `X-User-Id`/`X-Tenant-Id` and `idempotency_key`.
- [ ] **Step 2: Run** `npx vitest run lib/api/instore-api.test.ts` — Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** — Expected: PASS.
- [ ] **Step 5: Commit** `feat(admin): in-store API client and server actions (#1105)`.

### Task 4: Sale screen

**Files:**
- Create: `apps/admin/app/(admin)/sell/page.tsx`, `apps/admin/components/instore/{SaleScreen,ProductGrid,CartPanel,ChargeSheet,SaleDone}.tsx`
- Modify: `apps/admin/components/shell/AdminShell.tsx` (nav: "In-store sale" → `/sell` under Orders)
- Test: `apps/admin/components/instore/SaleScreen.test.tsx`

**Interfaces:**
- Consumes: Tasks 2 and 3.
- `page.tsx` (server): `getServerSessionContext()`; no store → the existing empty state; loads `getInStoreCatalog`; passes `storeId`, `currencyCode`, `catalog`, `canDiscount` (`role` is owner/admin/staff, or `currentStore.cashier_discounts_enabled` if the store type carries it) to `SaleScreen`.
- `SaleScreen` (client) props: `{ storeId: string; currencyCode: string; catalog: InStoreProduct[]; canDiscount: boolean }`. Mock `@/app/(admin)/sell/actions` in tests as `WarehousesSettingsClient.test.tsx` does.

Behaviour (each bullet is a test in `SaleScreen.test.tsx`):
- [ ] **Step 1: Write failing tests:**
  - `filters the grid by search` (typing "bmw" leaves only matching titles).
  - `picks a variant when a product has several` (tap → variant list → cart line with option summary); a single-variant product adds directly. A variant with `available` 0 and policy `deny` is disabled and labelled "Sold out".
  - `hides discount and custom item without permission` (`canDiscount=false`).
  - `adds a custom item` (name + price → line; no price field exists for catalogue lines).
  - `charge shows the server total` (Charge → `quoteSaleAction` called → button reads "Charge A$44.00" from the quote, not the local subtotal).
  - `cash shows change and needs enough tendered` (tendered 50 on 44.00 → "Change A$6.00"; 40 → Paid disabled).
  - `shows paid only after the server confirms` (record pending → no success text; resolves ok → order number and "New sale").
  - `keeps the cart and says not recorded when unreachable` (record → `unreachable` → "No connection, sale not recorded yet", "Try again", cart still shown and locked; Try again calls `recordSaleAction` with the **same** `idempotency_key`).
  - `names the line when out of stock` (→ that line highlighted "Only n left" / "Out of stock"; cart unlocked, key cleared).
  - `shows the new total when the server refuses a changed total` (→ "The total changed to A$46.00. Charge that amount instead." and the Charge amount updates).
  - `restores the cart after reload` (seed localStorage → lines render on mount).
  - `new sale clears the cart` and `switch person links to /logout`.
- [ ] **Step 2: Run** `npx vitest run components/instore` — Expected: FAIL.
- [ ] **Step 3: Implement.** Tablet: grid left, cart panel right (≥ `md`); phone: cart as a bottom sheet opened by a sticky "Cart · n · A$x" bar. Touch targets ≥ 44px. Use existing house components (`AdminPage`, buttons, `formatMoney`) before writing new ones. Locked cart shows a "Discard this sale" action with a warning that it may already be recorded (check Orders first).
- [ ] **Step 4: Run** `npx vitest run components/instore lib/instore lib/api/instore-api.test.ts` and `npx tsc --noEmit -p apps/admin` and the admin lint — Expected: PASS, no errors.
- [ ] **Step 5: Commit** `feat(admin): in-store sale screen (#1105)`.

### Task 5: Verify in the real app

- [ ] Run the admin locally against the dev stack (or the PR preview if one exists) and ring up: a catalogue line, a custom line with discount, a cash sale with change, and an out-of-stock refusal. Confirm the order appears in Orders and stock dropped. Then throttle the network to offline in DevTools, tap Paid, confirm the "not recorded yet" state, go back online, Try again, and confirm exactly one order exists.
- [ ] Record what was verified in the PR description.
