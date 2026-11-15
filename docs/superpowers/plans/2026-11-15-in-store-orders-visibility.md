# In-store orders in Orders, the order page and the dashboard (#1109) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A seller can tell in-store sales apart in Orders, filter to them, see how each was paid and who rang it up, and the dashboard counts them without treating every anonymous counter sale as one returning customer.

**Architecture:** The order rows already carry `channel`, `payment_method`, `payment_method_label`, `sold_by_user_id`, `cash_tendered` and `notes` (migration 000144); the admin API just doesn't return them. Task 1 exposes them, adds a `channel` list filter, stores the seller's email at sale time (the admin cannot resolve a user id to a person: the team list has no ids), and fixes customer metrics for empty emails. Tasks 2 and 3 render them.

**Tech Stack:** Go (gin, gorm, Postgres), Next.js admin (server components, vitest + Testing Library).

**Spec:** `docs/superpowers/specs/2026-11-14-in-store-sales-design.md` ("In-store orders"; "Success")

## Global Constraints

- "Visible in the Orders list with an **In-store** badge and a channel filter; counted in the dashboard and reports."
- "Stock, orders, the dashboard and reports include in-store sales with no extra step."
- Channel values: `online`, `in_store`. Payment methods: `card_external`, `cash`, `bank_transfer`, `other` (+ label).
- An in-store order with no receipt email stores `customer_email = ''`; nothing may email it.
- Seller-facing copy: plain words, no jargon ("In-store", "Paid by", "Rung up by").

## Review Focus

1. An in-store order with no email in the Orders list: the customer cell must not be blank or read "for " in its label. → Task 2 test `shows In-store sale when there is no customer`.
2. Customer metrics: many anonymous counter sales must not count as one buyer who keeps returning. → Task 1 test `TestCustomersMetrics_IgnoresEmptyEmail`.
3. `?channel=garbage` on the list: ignored by the admin page, 400 from the API, never a SQL error. → Task 1 test `TestOrdersList_ChannelFilter` and Task 2 parse test.
4. Changing one filter keeps the others (status, payment, search, channel) — the page builds every href by hand today. → Task 2 test `keeps the channel when another filter is picked`.
5. The order page for an in-store sale must not offer shipping-label buying or show an empty address block. → Task 3 test `hides shipping for an in-store sale`.

---

### Task 1: API — return in-store fields, channel filter, seller email, metrics

**Files:**
- Create: `services/marketplace-api/migrations/000145_orders_sold_by_email.{up,down}.sql`
- Modify: `services/marketplace-api/internal/order/models.go` (field `SoldByEmail *string`, column `sold_by_email varchar(320)`)
- Modify: `services/marketplace-api/internal/instore/types.go`, `service.go` (`SaleInput.SoldByEmail string` → `Order.SoldByEmail`, nil when empty)
- Modify: `services/marketplace-api/internal/handlers/admin/instore.go` (email from `c.GetString("user_email")`, falling back to header `X-User-Email`, same as tickets.go)
- Modify: `services/marketplace-api/internal/handlers/admin/orders_dto.go`, `orders.go`
- Modify: `services/marketplace-api/internal/handlers/admin/metrics.go` (customer metrics)
- Test: `internal/handlers/admin/orders_list_channel_integration_test.go` (or the existing orders handler integration test file), `internal/instore/service_integration_test.go`, metrics integration test file

**Interfaces:**
- Produces (wire, `AdminOrderResponse`): `channel string` (always), `payment_method *string`, `payment_method_label *string`, `sold_by_email *string`, `cash_tendered *decimal` (omitempty), `notes *string` (omitempty).
- Produces: `GET .../orders?channel=online|in_store`; any other non-empty value → 400 `validation_failed`.

- [ ] **Step 1: Failing tests**
  - `TestOrdersList_ChannelFilter`: seed one online and one in-store order; `?channel=in_store` returns only the in-store one with `channel:"in_store"`, `payment_method:"cash"`, `sold_by_email` set; `?channel=online` the other; `?channel=pos` → 400.
  - `TestIntegration_InStore_RecordsSellerEmail`: `SaleInput.SoldByEmail = "sam@shop.test"` → stored on the order; empty → NULL.
  - `TestCustomersMetrics_IgnoresEmptyEmail`: three in-store orders with `''` email plus one online order → `unique_buyers == 1`, `returning_total == 0`.
- [ ] **Step 2:** Run them (`TEST_DATABASE_URL=... go test -tags integration -p 1 ./internal/handlers/admin/ ./internal/instore/ -run 'ChannelFilter|SellerEmail|IgnoresEmptyEmail'`). Expected: FAIL (unknown fields / filter ignored / counts 2 and 2).
- [ ] **Step 3:** Implement. Migration: `ALTER TABLE orders ADD COLUMN sold_by_email varchar(320);` (down drops it). `ListOrdersQuery.Channel string \`form:"channel" binding:"omitempty,oneof=online in_store"\``; filter `channel = ?`. Customer metrics: add `AND customer_email <> ''` to the new/returning subquery and the unique-buyers query (the top-buyers query already excludes it). Regenerate nothing else; the route is unchanged.
- [ ] **Step 4:** Run the three tests, then the packages: `go test -tags integration -p 1 ./internal/handlers/admin/ ./internal/instore/ ./internal/order/`. Expected: PASS.
- [ ] **Step 5:** Commit `feat(api): in-store fields on admin orders, channel filter, seller email`.

### Task 2: Orders list — In-store badge, channel filter

**Files:**
- Modify: `apps/admin/lib/api/marketplace-api.ts` (`AdminOrder` gains `channel: "online" | "in_store"`, `payment_method?`, `payment_method_label?`, `sold_by_email?`, `cash_tendered?`, `notes?`; `ListOrdersQuery.channel?: OrderChannel`; `listOrders` sends `channel`)
- Modify: `apps/admin/components/orders/OrdersList.tsx`
- Modify: `apps/admin/app/(admin)/orders/page.tsx`
- Test: `apps/admin/components/orders/OrdersList.test.tsx` (create if absent), `apps/admin/app/(admin)/orders/page.test.tsx` or a test of the exported `parseSearchParams`/`ordersHref`

**Interfaces:**
- Produces: `export type OrderChannel = "online" | "in_store"` in `lib/api/marketplace-api.ts`.
- Produces: `ordersHref(q: ListOrdersQuery, change: Partial<ListOrdersQuery>): string` in `app/(admin)/orders/page.tsx`, replacing the four hand-built href functions (buildHref, buildStatusHref, buildPaymentHref, the search Clear link).

- [ ] **Step 1: Failing tests**
  - `shows an In-store badge on in-store orders only`.
  - `shows In-store sale when there is no customer` (cell text "In-store sale"; aria-label "Order M-1, in-store sale").
  - `keeps the channel when another filter is picked`: `ordersHref({channel:"in_store", status:"fulfilled"}, {paymentStatus:"paid"})` → `/orders?status=fulfilled&payment_status=paid&channel=in_store`; changing `page` keeps all three.
  - `ignores an unknown channel`: `parseSearchParams({channel:"pos"}).channel === undefined`.
- [ ] **Step 2:** `npx vitest run components/orders app/\(admin\)/orders` → FAIL.
- [ ] **Step 3:** Implement. Third chip row "Channel": All / Online / In-store. Badge: small neutral pill "In-store" after the order number. `hasActiveFilters` includes channel.
- [ ] **Step 4:** Tests pass; `npx tsc --noEmit -p .`; `npm run lint` 0 errors.
- [ ] **Step 5:** Commit `feat(admin): In-store badge and channel filter on Orders`.

### Task 3: Order page and dashboard — how it was paid, who rang it up

**Files:**
- Create: `apps/admin/components/orders/InStoreSaleCard.tsx`
- Modify: `apps/admin/app/(admin)/orders/[id]/page.tsx`, `components/orders/OrderDetailHeader.tsx`, `components/dashboard/RecentOrders.tsx`
- Test: `apps/admin/components/orders/InStoreSaleCard.test.tsx`, plus header/recent-orders assertions in their existing tests (create if absent)

**Interfaces:**
- Consumes: Task 2's `AdminOrder` fields.
- Produces: `InStoreSaleCard({ order }: { order: AdminOrder })` — renders nothing unless `order.channel === "in_store"`.

- [ ] **Step 1: Failing tests**
  - `InStoreSaleCard` shows "Paid by Card (EFTPOS)" / "Cash" / "Bank transfer" / the `other` label; for cash with `cash_tendered: "50"` and grand total 28 shows "Cash given A$50.00" and "Change A$22.00"; "Rung up by sam@shop.test", or "Rung up by a team member" when `sold_by_email` is absent; the note when present.
  - `hides shipping for an in-store sale`: the order page renders neither the address card nor the shipping-label panel for `channel: "in_store"`. (If the page is hard to render in a test, extract `showsShipping(order)` and test that.)
  - `OrderDetailHeader` and `RecentOrders` show "In-store sale" in place of an empty customer email.
- [ ] **Step 2:** Run → FAIL.
- [ ] **Step 3:** Implement. Card sits in the aside above the actions bar, hairline style like its neighbours. Payment labels reuse the sale screen's wording (`ChargeSheet` METHODS) — export that list from `components/instore/ChargeSheet.tsx` rather than copying it.
- [ ] **Step 4:** `npx vitest run` (full admin suite), tsc, lint.
- [ ] **Step 5:** Commit `feat(admin): in-store details on the order page and dashboard`.

### Not in this plan (ruled)

- **Channel breakdown in reports:** no report splits by source today, so per the issue ("where reports already split by source") there is nothing to extend. Dashboard and report totals already include in-store orders (they filter only on status and deleted_at).
- **Emailing an invoice to an empty address** from the order page belongs to #1107 (receipts by email).
