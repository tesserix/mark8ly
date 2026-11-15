# Record a refund made outside mark8ly (#1110) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** An owner or admin can record that they refunded an in-store sale on their own terminal or in cash, optionally putting items back in stock, and the order shows as refunded or partially refunded — without any payment provider being called.

**Architecture:** A new `instore.RefundService.Record` runs in one transaction: idempotency by a client request id, the existing `order.Service.RecordRefund` for the money state and events, a row per refund (and per restocked line) so restocks are bounded by what was sold, and a stock increment at the location the sale took the units from (its committed stock holds). An owner/admin-only admin route exposes it; the order page gets a "Record a refund" form for in-store orders.

**Tech Stack:** Go (gin, gorm, Postgres), Next.js admin, vitest.

**Spec:** `docs/superpowers/specs/2026-11-14-in-store-sales-design.md` ("Refunding an in-store sale (owner/manager only): record a refund made outside mark8ly (back on the terminal or in cash), which restocks and marks the order refunded without calling any payment provider.")

## Global Constraints

- Owner and admin only (the tenant-level equivalent of "owner/manager"); cashiers and staff cannot.
- In-store orders only; never touches a payment provider.
- Refund amount defaults to the remaining balance; never more than it.
- Restock is per line and never more than the quantity sold minus what earlier refunds already put back. Custom lines have no stock.
- A retried submit with the same request id records nothing new.

## Review Focus

1. Double submit (slow network, two taps) → one refund. → `TestOutsideRefund_RetryIsIdempotent`.
2. Refund more than remains, or restock more than sold → refused, nothing written. → `TestOutsideRefund_Bounds`.
3. An online order → refused (it has a provider payment; use Issue refund). → `TestOutsideRefund_OnlineRefused`.
4. A continue-selling product with no stock row → restock creates the row at the store's default warehouse rather than failing. → `TestOutsideRefund_RestockWithoutStockRow`.
5. Staff or cashier → 404 like other denials. → handler test.

## Rulings made while planning

- Migration **000147** (`in_store_refunds`, `in_store_refund_items`). Merge order: #1118 (145), #1120 (146), then this.
- Restock location: where the sale's committed hold for that variant was; otherwise the variant's first stock row; otherwise a new row at the store's default warehouse. shortcut: a sale that drew one variant from two locations restocks all of it at the first; upgrade if multi-location stores appear.

---

### Task 1: API — record an outside refund

**Files:** `migrations/000147_in_store_refunds.{up,down}.sql`; `internal/instore/refund.go` (+ export `SaleCartToken(storeID uuid.UUID, idempotencyKey string) string`, used by `service.go`); `internal/handlers/admin/instore.go`, `routes.go` (`POST /stores/:storeId/in-store/sales/:id/refund`, `RequireTenantRelation(authz.RoleAdmin)`, on `storeRoute`); `cmd/marketplace-api/main.go`; `route-manifest.json`; `migrations.go` → 147.

**Interfaces:**
- `RefundInput{StoreID, OrderID uuid.UUID; RequestID string; Amount *decimal.Decimal; Method, Reason, RefundedBy string; Items []RefundItem{OrderItemID uuid.UUID; Quantity int}}`
- `RefundResult{PaymentStatus string; RefundedAmount decimal.Decimal; Reused bool}`
- Request JSON: `{refund_request_id, amount?, method, reason?, items?: [{order_item_id, quantity}]}`; response `{payment_status, refunded_amount, reused}`.

- [ ] Failing integration tests: `TestOutsideRefund_FullRestocksAndMarksRefunded`, `TestOutsideRefund_PartialKeepsPartiallyRefunded`, `TestOutsideRefund_RetryIsIdempotent`, `TestOutsideRefund_Bounds`, `TestOutsideRefund_OnlineRefused`, `TestOutsideRefund_RestockWithoutStockRow`; unit `TestOutsideRefund_OwnerAdminOnly`.
- [ ] Implement; run `go test -tags integration -p 1 ./internal/instore/ ./internal/handlers/admin/ ./internal/routemanifest/ .`
- [ ] Commit `feat(api): record a refund made outside mark8ly for an in-store sale`.

### Task 2: Admin — "Record a refund" on the order page

**Files:** `apps/admin/lib/api/instore-api.ts` (`recordOutsideRefund`), `apps/admin/app/(admin)/orders/[id]/actions.ts` (`recordOutsideRefundAction`), `apps/admin/components/orders/OutsideRefundForm.tsx`, order page wiring (owner/admin, in-store, paid or partially refunded).

- [ ] Failing tests: amount defaults to the remaining balance; method radios use the sale screen's labels; per-line "Put back in stock" steppers bounded by quantity, absent for custom lines; submit sends one request id reused on retry; success refreshes; server message shown on failure.
- [ ] Implement; full admin suite, tsc, lint.
- [ ] Commit `feat(admin): record a refund made outside mark8ly`.
