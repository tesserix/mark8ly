# In-store sales — backend core Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** marketplace-api can record a paid in-store sale (catalogue and custom lines, optional discount, external payment method) as a normal order that decrements stock, and a sell-only Cashier role can do that and nothing else.

**Architecture:** Checkout's server-side pricing, stock commit and tax calculation move out of the storefront handler package into `internal/checkoutcore` (move-only), so a new `internal/instore` service can reuse them. A sale is one `order.Service.Create` call whose `WithinTx` commits stock, confirms payment and (by default) marks it fulfilled, so stock and money commit or roll back together. Cashier is a new tenant relation in OpenFGA that counts as a member but is outside the owner > admin > staff > viewer chain.

**Tech Stack:** Go, gin, gorm, Postgres migrations (golang-migrate), OpenFGA.

**Spec:** `docs/superpowers/specs/2026-11-14-in-store-sales-design.md` (issues #1101; this plan covers #1102, #1103 and the backend of #1104). UI, receipts, ABN, reports and refunds get their own plans.

## Global Constraints

- Prices are never taken from the client for catalogue lines: "Catalogue lines priced from the server, never the client."
- "The app says 'paid' before the server has recorded the order" is an anti-goal: the order, stock decrement and paid status commit in ONE transaction.
- Retried sales use the orders `UNIQUE (store_id, idempotency_key)`; a retry returns the first order (HTTP 200) and never creates a second.
- Cashier **cannot**: edit products or prices, see other orders or customers, reports, settings, refunds, invites. Discounts and custom items for cashiers only when the store's `cashier_discounts_enabled` is true (default false).
- Personalisation answers are optional for in-store lines.
- Every new route gets a role constant in `internal/authz/*_roles.go`, an entry in the route-parity test, and a regenerated `route-manifest.json` (`go test ./internal/routemanifest -run TestRouteManifestMatchesMountedRoutes -update`).
- The production OpenFGA model lives in `tesserix-k8s/charts/apps/mark8ly-fga-init/templates/configmap-model.yaml`; it must get the same change as `infra/openfga/model.fga` (separate repo, PR only, ask the user before opening it).
- Commits and PRs carry no AI attribution.

## Review Focus

1. **A cashier reaching a non-sale admin route.** Adding `cashier` to `member` lets cashiers past every gate that only checks membership. Expected: 404 on everything except the in-store routes. Pinned by `TestCashierReachesOnlyInStoreRoutes` (Task 5).
2. **Two lines of the same variant in one sale.** Expected: stock checked and decremented for the combined quantity, not just the last line. Pinned by `TestIntegration_InStore_SameVariantTwoLines` (Task 6).
3. **A retried Paid after a dropped connection.** Expected: same order returned, stock decremented once. Pinned by `TestIntegration_InStore_RetryIsIdempotent` (Task 6).
4. **A sale with no receipt email.** Expected: order saved with empty `customer_email`, and nothing tries to email it. Pinned by `TestIntegration_InStore_NoEmail` (Task 6) and the empty-recipient guard test in Task 3.
5. **A discount larger than the subtotal, or negative.** Expected: 422, nothing recorded. Pinned by `TestInStore_DiscountBounds` (Task 6).

---

### Task 1: Move stock commit and catalogue pricing into `internal/checkoutcore`

Move-only refactor. No behaviour change.

**Files:**
- Create: `services/marketplace-api/internal/checkoutcore/stock.go` (from `internal/handlers/storefront/checkout_stock.go`: `stockLine`, `outOfStockError`, `commitStock`, `storeWarehousesInFillOrder`, `variantIDsOf`, `recordAllocations`, `inventoryPolicies`, `inventoryPolicyContinue`)
- Create: `services/marketplace-api/internal/checkoutcore/pricing.go` (from `checkout_reprice.go`: `catalogPrice`, `catalogPricer`, `dbCatalogPricer`, `errCatalogItemNotFound`)
- Move: `internal/handlers/storefront/checkout_allocation_integration_test.go` → `internal/checkoutcore/stock_allocation_integration_test.go`
- Modify: `internal/handlers/storefront/checkout_stock.go` (keeps `stockLinesFromItems`, `cartTokenForCheckout`), `checkout_reprice.go` (keeps `repriceItems`), `checkout.go`, `checkout_ext.go`

**Interfaces:**
- Produces:
  - `type Line struct { VariantID string; Quantity int }`
  - `type OutOfStockError struct { VariantID string }` (implements `error`)
  - `func CommitStock(ctx context.Context, tx *gorm.DB, holds *stockhold.Repository, cartToken, orderID, storeID string, lines []Line) error`
  - `type CatalogPrice struct { Amount decimal.Decimal; CurrencyCode string; TaxCode *string; TaxRateOverride *decimal.Decimal; TaxCategory *string }`
  - `type CatalogPricer interface { PriceFor(ctx context.Context, storeID, variantID string) (CatalogPrice, error) }`
  - `func NewDBCatalogPricer(db *gorm.DB) CatalogPricer`
  - `var ErrCatalogItemNotFound`

- [ ] **Step 1:** Move the code and rename to the exported names above; storefront call sites use `checkoutcore.*`. `errors.As(err, &checkoutcore.OutOfStockError{})` replaces the storefront's own type checks.
- [ ] **Step 2:** Run `go build ./... && go vet ./internal/checkoutcore/ ./internal/handlers/storefront/` — expected: clean.
- [ ] **Step 3:** Run `go test ./internal/handlers/storefront/ ./internal/checkoutcore/` and, with `TEST_DATABASE_URL` set, `go test -tags integration ./internal/checkoutcore/ ./internal/handlers/storefront/` — expected: same tests pass as before the move (count them before and after).
- [ ] **Step 4:** Commit `refactor(checkout): move stock commit and catalogue pricing to checkoutcore`.

### Task 2: Move tax calculation into `internal/checkoutcore`

**Files:**
- Create: `services/marketplace-api/internal/checkoutcore/tax.go`
- Modify: `internal/handlers/storefront/checkout_ext.go:1202-1310` (`calculateTax` becomes a thin adapter)
- Test: `internal/checkoutcore/tax_test.go`

**Interfaces:**
- Produces: `func CalculateTax(ctx context.Context, db *gorm.DB, resolveCred func(ctx context.Context, ref string) (string, error), store *stores.Store, sc *country.SupportedCountry, items []tax.TaxableItem, dest tax.Address, shippingTotal decimal.Decimal) (*tax.TaxBreakdown, error)`

- [ ] **Step 1: Write the failing test** `TestCalculateTax_FlatRateStore` — an AU store with `sc.TaxStrategy` the flat strategy and `TaxRate` 0.10, one item of 110.00, `dest` = the seller address; assert the breakdown total tax equals what `tax.Service.CalculateOrderTax` returns for the same inputs (compare against a direct call, so the test pins "same as checkout", not a hand-computed GST figure).
- [ ] **Step 2:** `go test ./internal/checkoutcore/ -run TestCalculateTax_FlatRateStore` — expected: FAIL, `CalculateTax` undefined.
- [ ] **Step 3:** Move the body of `calculateTax` into `CalculateTax`; the storefront builds `items` and `dest` from its request and calls it with `h.resolveCred`.
- [ ] **Step 4:** Run the new test and `go test ./internal/handlers/storefront/` (plus integration) — expected: PASS.
- [ ] **Step 5:** Commit `refactor(checkout): move tax calculation to checkoutcore`.

### Task 3: Orders carry channel, payment method and cashier

**Files:**
- Create: `services/marketplace-api/migrations/000144_in_store_sales.up.sql`, `000144_in_store_sales.down.sql`
- Modify: `services/marketplace-api/migrations.go` (`ExpectedSchemaVersion = 144`)
- Modify: `internal/order/models.go`, `internal/order/service.go` (`CreateInput`, `Create`)
- Modify: `internal/stores/models.go` (store setting)
- Modify: the order-document and notification senders that read `Order.CustomerEmail` (find with `grep -rn "\.CustomerEmail" internal --include='*.go'`)
- Test: `internal/order/service_integration_test.go`, `internal/orderdoc/mailer_test.go`

Migration 144 (up):
- `orders.channel varchar(20) NOT NULL DEFAULT 'online' CHECK (channel IN ('online','in_store'))`
- `orders.payment_method varchar(20) NULL CHECK (payment_method IN ('card_external','cash','bank_transfer','other'))`
- `orders.payment_method_label varchar(40) NULL`
- `orders.sold_by_user_id varchar(64) NULL` (the auth user id string, as `c.GetString("user_id")`)
- `orders.cash_tendered numeric(12,2) NULL`
- index `orders (store_id, channel, placed_at DESC)`
- `stores.cashier_discounts_enabled boolean NOT NULL DEFAULT false`
- `customer_email` stays `NOT NULL`; an in-store sale without a receipt email stores `''` (decision: no nullable column, no placeholder address).

Down drops the columns and index.

**Interfaces:**
- Produces:
  - `Order` fields `Channel string`, `PaymentMethod *string`, `PaymentMethodLabel *string`, `SoldByUserID *string`, `CashTendered *decimal.Decimal`
  - `const ChannelOnline = "online"`, `const ChannelInStore = "in_store"`
  - `CreateInput.InStore *InStoreDetails` with `type InStoreDetails struct { PaymentMethod string; PaymentMethodLabel *string; SoldByUserID string; CashTendered *decimal.Decimal }`. `Create` sets `Channel` to `in_store` and copies these when non-nil, else `online`.
  - `Store.CashierDiscountsEnabled bool`

- [ ] **Step 1: Write the failing test** `TestIntegration_OrderCreate_InStoreFieldsPersist` — `Create` with `InStore{PaymentMethod: "cash", SoldByUserID: "u-1", CashTendered: 50.00}` and `CustomerEmail: ""`; reload; assert `Channel == "in_store"`, method `cash`, sold-by `u-1`, tendered `50.00`, email `""`. And `TestIntegration_OrderCreate_DefaultsOnline` — no `InStore` → `Channel == "online"`, method nil.
- [ ] **Step 2:** Run with `-tags integration` — expected: FAIL (columns / fields missing).
- [ ] **Step 3:** Write migration 144, bump `ExpectedSchemaVersion`, add the fields and `InStoreDetails`.
- [ ] **Step 4: Write the failing test** `TestSendDocument_EmptyRecipientIsNoop` in `internal/orderdoc/mailer_test.go` — an order with `CustomerEmail == ""`; assert the fake sender received zero messages and the call returns nil. Add the same guard test for any other sender the grep finds.
- [ ] **Step 5:** Add the empty-recipient guard at the top of each sender.
- [ ] **Step 6:** `go test ./internal/order/ ./internal/orderdoc/` and `-tags integration ./internal/order/` — expected: PASS; also run the migration test suite (`go test -tags integration ./...` filtered to migrations) — expected: up/down round-trips.
- [ ] **Step 7:** Commit `feat(orders): channel, external payment method and cashier on orders (#1102)`.

### Task 4: Cashier relation in OpenFGA and the Go authz client

**Files:**
- Modify: `infra/openfga/model.fga` (tenant type)
- Modify: `infra/dev/seed/fga-init.sh` only if it inlines the model rather than reading the file
- Modify: `services/marketplace-api/internal/authz/client.go`, `fake.go`
- Create: `services/marketplace-api/internal/authz/instore_roles.go`
- Modify: `services/platform-api/internal/authz/authz.go`, `services/platform-api/internal/invitation/models.go:57` (accept `"cashier"`), and platform-api's fake if it mirrors roles (`internal/authz/fake.go`)
- Test: `services/marketplace-api/internal/authz/fake_test.go`, `services/platform-api/internal/invitation/*_test.go`

Model change (tenant):
```
define cashier: [user]
define member: viewer or cashier
define can_sell_in_store: cashier or staff
```
`cashier` is deliberately not part of `viewer`, so every existing `viewer`/`staff`/`admin`/`owner` gate keeps denying it.

**Interfaces:**
- Produces: `authz.RoleCashier Role = "cashier"` (priority 0, below viewer, so `HigherOrEqual` never lets it pass a ranked gate); `authz.RelationCanSellInStore Role = "can_sell_in_store"`; `InStoreSellRole = RelationCanSellInStore` in `instore_roles.go`; FakeClient answers `cashier`, `can_sell_in_store` (cashier or staff+) and `member` (now true for cashier too).

- [ ] **Step 1: Write the failing test** `TestFakeClient_Cashier` — grant cashier; assert `Check` is true for `cashier`, `member`, `can_sell_in_store` and false for `viewer`, `staff`, `admin`, `owner`; grant staff; assert `can_sell_in_store` true, `cashier` false.
- [ ] **Step 2:** Run — expected: FAIL.
- [ ] **Step 3:** Update the model, roles and fakes; platform-api invitations accept `cashier` and write the `cashier` tuple.
- [ ] **Step 4: Write the failing test** in platform-api: inviting with role `cashier` validates and accepting writes relation `cashier`.
- [ ] **Step 5:** Implement; run both services' tests — expected: PASS. Validate the model: `fga model validate --file infra/openfga/model.fga` (or the repo's existing model test, if present) — expected: valid.
- [ ] **Step 6:** Commit `feat(authz): cashier role that can only sell in store (#1104)`.
- [ ] **Step 7:** Prepare (do not open without the user's go-ahead) the matching `tesserix-k8s` change to `charts/apps/mark8ly-fga-init/templates/configmap-model.yaml`. The in-store routes must not ship before that model is live, or `can_sell_in_store` checks error.

### Task 5: Prove cashiers reach nothing but the sale routes

**Files:**
- Create: `services/marketplace-api/internal/handlers/admin/cashier_gate_test.go`

**Interfaces:**
- Consumes: `authz.RoleCashier` (Task 4); the mounted admin router as `route_parity_test.go` builds it.

- [ ] **Step 1: Write the test** `TestCashierReachesOnlyInStoreRoutes` — mount the real admin routes with a `FakeClient` that grants the request's user `cashier`; for every route in the mounted table, send a request with that user (path params filled with fresh uuids, empty JSON body); assert status is 404 for every route whose path does not start with `/admin/stores/:storeId/in-store/`. In-store routes are exempt (Task 7 adds them).
- [ ] **Step 2:** Run — expected: PASS if no admin route relies on membership alone; any route that answers non-404 for a cashier is a real hole.
- [ ] **Step 3:** For each failing route, add the missing `RequireTenantRelation(...)` with the role its neighbours use, and note it in the commit body.
- [ ] **Step 4:** Run again — expected: PASS.
- [ ] **Step 5:** Commit `test(authz): cashiers reach only in-store routes`.

### Task 6: `internal/instore` — record a sale

**Files:**
- Create: `services/marketplace-api/internal/instore/service.go`, `types.go`
- Test: `internal/instore/service_test.go` (pure validation), `internal/instore/service_integration_test.go`

**Interfaces:**
- Consumes: `checkoutcore.CatalogPricer`, `checkoutcore.CommitStock`, `checkoutcore.Line`, `checkoutcore.OutOfStockError`, `checkoutcore.CalculateTax` (Tasks 1–2); `order.CreateInput.InStore` (Task 3); `order.Service.Confirm`, `MarkFulfilled`.
- Produces:
  - `type SaleLine struct { VariantID *string; ProductID *string; Quantity int; CustomTitle *string; CustomPrice *decimal.Decimal; Personalisation map[string]string }` — catalogue line: `VariantID` set, custom fields nil; custom line: `VariantID` nil, `CustomTitle` and `CustomPrice` set.
  - `type SaleInput struct { TenantID, StoreID uuid.UUID; Store *stores.Store; IdempotencyKey string; Lines []SaleLine; Discount decimal.Decimal; PaymentMethod string; PaymentMethodLabel *string; CashTendered *decimal.Decimal; Note *string; CustomerEmail string; SoldByUserID string; CanDiscount bool; Fulfilled bool }`
  - `func (s *Service) RecordSale(ctx context.Context, in SaleInput) (*order.CreateResult, error)`
  - `func NewService(db *gorm.DB, orders *order.Service, holds *stockhold.Repository, pricer checkoutcore.CatalogPricer, resolveCred func(context.Context, string) (string, error)) *Service`

Behaviour, in order:
1. Validate: ≥1 line; quantity 1–999; payment method in the four values, `other` requires a label ≤ 40 chars; `cash` requires `CashTendered ≥ grand total`; discount `0 ≤ d ≤ subtotal`; custom lines and a non-zero discount require `CanDiscount`; custom title 1–200 chars, custom price `> 0`. Failures → `apperrors.ValidationFailed`.
2. Price catalogue lines with the pricer (never the request). Custom lines use their price and carry no product/variant id.
3. Tax via `CalculateTax` with `dest` = the seller address and shipping 0; apply the discount to the taxable amounts the same way checkout applies a coupon's `DiscountTotal` (follow `checkout_ext.go`).
4. `order.Service.Create` with `InStore` set, empty shipping/billing addresses, `Notes` = note, `CustomerEmail` as given (may be `""`), and a `WithinTx` that: `CommitStock` for the catalogue lines (cart token = `"instore:" + IdempotencyKey`); `Confirm(tx, id, &paid, "in-store sale")`; if `Fulfilled`, `MarkFulfilled(tx, id)`. Personalisation answers attach the same way checkout attaches them, skipping empty answers.
5. `OutOfStockError` propagates unchanged so the handler can name the line.

- [ ] **Step 1: Write the failing tests** (`service_test.go`): `TestInStore_DiscountBounds` (discount −1 → validation error; discount > subtotal → validation error; `CanDiscount=false` with discount 5 → validation error); `TestInStore_CustomLineNeedsPermission`; `TestInStore_OtherNeedsLabel`; `TestInStore_CashMustCoverTotal`.
- [ ] **Step 2:** Run — expected: FAIL, package missing.
- [ ] **Step 3:** Implement validation and `RecordSale`.
- [ ] **Step 4: Write the failing integration tests:**
  - `TestIntegration_InStore_PaidAndStockDecremented` — variant stock 5, sell 2 by card → order `payment_status=paid`, `fulfillment_status=fulfilled`, `channel=in_store`, stock 3.
  - `TestIntegration_InStore_SameVariantTwoLines` — stock 3, two lines of 2 → `OutOfStockError`, no order row, stock 3.
  - `TestIntegration_InStore_RetryIsIdempotent` — same key twice → second result `Reused=true`, one order, stock decremented once.
  - `TestIntegration_InStore_NoEmail` — `CustomerEmail ""` → saved; no outbound email recorded.
  - `TestIntegration_InStore_ClientPriceIgnored` — there is no price field for catalogue lines; assert `UnitPrice` equals the variant's DB price.
  - `TestIntegration_InStore_CustomLineNoStock` — one custom line → order item with nil product/variant, no stock rows touched.
  - `TestIntegration_InStore_ContinuePolicyAtZero` — "continue selling" variant at stock 0 sells.
  - `TestIntegration_InStore_RequiredPersonalisationOptional` — product with a required field, no answer → order created.
- [ ] **Step 5:** Run `go test -tags integration ./internal/instore/` — expected: PASS.
- [ ] **Step 6:** Commit `feat(instore): record a paid in-store sale (#1103)`.

### Task 7: Admin routes for the sale screen

**Files:**
- Create: `services/marketplace-api/internal/handlers/admin/instore.go`
- Modify: `internal/handlers/admin/routes.go`, `route_parity_test.go`, `route-manifest.json`, `cmd/marketplace-api/main.go` (wire `instore.Service`)
- Test: `internal/handlers/admin/instore_test.go`

Routes (all `RequireTenantRelation(authz.InStoreSellRole)`, under the existing store middleware):
- `GET /admin/stores/:storeId/in-store/catalog` — active, published products only: id, title, handle, first image URL (position order), variants (id, option summary, price, available stock via the stockhold availability model, inventory policy), personalisation fields. Nothing else from the product.
- `POST /admin/stores/:storeId/in-store/sales` — body maps to `SaleInput` (no price for catalogue lines); `SoldByUserID` from `c.GetString("user_id")`; `CanDiscount` = caller is staff-or-above OR `store.CashierDiscountsEnabled`. 201 new, 200 reused, 409 `{ "error": { "code": "out_of_stock", "variant_id": "…" } }`, 422 validation.
- `GET /admin/stores/:storeId/in-store/sales?scope=mine` — the caller's own in-store orders placed today in the store's timezone: id, order number, total, payment method, placed at, receipt email (masked).

**Interfaces:**
- Consumes: `instore.Service.RecordSale` (Task 6), `authz.InStoreSellRole` (Task 4).
- Produces: JSON shapes above, consumed by the sale-screen plan.

- [ ] **Step 1: Write the failing handler tests:** `TestInStoreSale_Created201`, `TestInStoreSale_Replay200`, `TestInStoreSale_OutOfStock409NamesVariant`, `TestInStoreSale_CashierDiscountForbiddenUnlessEnabled` (cashier + discount → 422; same with `CashierDiscountsEnabled=true` → 201), `TestInStoreCatalog_OnlyActivePublished`, `TestInStoreSalesMine_OnlyCallersToday`.
- [ ] **Step 2:** Run — expected: FAIL.
- [ ] **Step 3:** Implement handler, register routes, wire the service, add parity entries, regenerate the manifest.
- [ ] **Step 4:** Run `go test ./internal/handlers/admin/ ./internal/routemanifest/` and Task 5's `TestCashierReachesOnlyInStoreRoutes` — expected: PASS.
- [ ] **Step 5:** Commit `feat(admin): in-store catalogue and sale endpoints (#1103)`.
- [ ] **Step 6:** Open the PR. Do not merge before the `tesserix-k8s` FGA model change from Task 4 Step 7 is live in production.
