# Own courier: flat-rate delivery options — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let a store ship with its own courier at flat prices it sets, instead of live carrier quotes, and mark orders shipped with a tracking link the buyer can open.

**Architecture:** A store-level `shipping_mode` (`carrier` | `own`) picks one branch. In `own` mode, delivery options and couriers come from two new tenant-scoped tables in marketplace-api; checkout prices and re-checks `own:<uuid>` services against them; a manual-shipment endpoint records a shipment with a resolved `tracking_url` and reuses the existing one-time shipped email. Admin gets a mode switch, two editors and a Mark-as-shipped form; storefront and email render the tracking link.

**Tech Stack:** Go (gin, gorm, Postgres migrations), Next.js (apps/admin, apps/storefront), vitest, Go tests (`-tags=integration` for DB).

**Spec:** `docs/superpowers/specs/2026-11-20-own-courier-delivery-options-design.md`

## Global Constraints

- One method per store: `shipping_mode` default `carrier`; in `own` mode only own options are quoted/accepted, in `carrier` mode `own:` values are rejected. Carrier behaviour otherwise unchanged (including its existing first-rate fallback).
- Delivery options: max 5 per store; name 1–40 chars; price 0–99,999 (NUMERIC(12,2), store currency); estimate ≤ 40 chars; enabled flag; position = checkout order.
- Couriers: name 1–40 chars; tracking template optional, `https://` only, contains `{number}` exactly once, ≤ 500 chars.
- Service key for own options: `own:<option uuid>`; `orders.shipping_carrier = "own"`; snapshot `shipping_option_name`, `shipping_option_estimate` on the order.
- Manual shipment: `shipments.carrier = 'own'`, `status = in_transit`, one per order (refused if an active non-cancelled shipment exists), links all order allocations, marks the order fulfilled, sends the shipped email once.
- Tracking URL built server-side: template with `{number}` replaced by `url.QueryEscape(trackingNumber)`; stored only if it parses as `https`.
- New tables are tenant-scoped and must be in `internal/tenantpurge` purge plan.
- No `fmt.Print*`/`fmt.Fprintf` in `services/*/internal` (log-shipping smoke guard); new admin routes registered via existing route groups (route manifest must stay green: regenerate with `go test ./internal/routemanifest -run TestRouteManifestMatchesMountedRoutes -update` if frontend-facing).
- Migrations start at `000150`.

## Review Focus

1. Forged/stale `own:` service at checkout (wrong store's uuid, disabled option, malformed uuid, `own:` while store is in `carrier` mode) → validation error, never priced. *(Task 3 tests.)*
2. Store switches mode with orders in flight → carrier orders keep the label panel; own orders show Mark as shipped; decided by `orders.shipping_carrier`, not current mode. *(Task 5 + Task 7 tests.)*
3. Double submit of Mark as shipped (two clicks / two tabs) → exactly one shipment and one email. *(Task 5 test, with the unique guard.)*
4. Tracking numbers with spaces, `&`, `#`, unicode → link escapes them; template without `{number}` or non-https rejected at save. *(Task 2 + Task 5 tests.)*
5. Own mode with zero enabled options → rates return empty, checkout shows the existing no-shipping fallback, settings page warns; no 500. *(Task 3 + Task 6 tests.)*

---

### Task 1: Schema and models

**Files:**
- Create: `services/marketplace-api/migrations/000150_own_delivery.up.sql`, `.down.sql`
- Create: `services/marketplace-api/internal/delivery/models.go`
- Modify: `services/marketplace-api/internal/stores/models.go` (add field), `internal/order/models.go` (two fields), `internal/shipping/repository.go` `ShipmentRecord` (three fields)
- Modify: `services/marketplace-api/internal/tenantpurge/purge.go` (`purgePlan`)
- Test: `internal/tenantpurge/schema_coverage_integration_test.go` (existing; must pass), `internal/delivery/models_test.go`

**Interfaces:**
- Produces: tables `store_delivery_options(id uuid pk, tenant_id uuid, store_id uuid, name varchar(40), price numeric(12,2), estimate varchar(40) null, enabled bool, position int, created_at, updated_at)`, `store_couriers(id, tenant_id, store_id, name varchar(40), tracking_url_template varchar(500) null, created_at, updated_at)`; `stores.shipping_mode varchar(10) not null default 'carrier' check in ('carrier','own')`, `stores.own_free_shipping_min numeric(12,2) null`; `orders.shipping_option_name varchar(40) null`, `orders.shipping_option_estimate varchar(40) null`; `shipments.carrier` widened to `varchar(40)`, `shipments.courier_id uuid null`, `shipments.courier_name varchar(40) null`, `shipments.tracking_url text null`; partial unique index `shipments_one_active_own_per_order ON shipments(order_id) WHERE carrier='own' AND status <> 'cancelled'`.
- Go: `delivery.Option{ID, TenantID, StoreID string; Name string; Price decimal.Decimal; Estimate *string; Enabled bool; Position int}`, `delivery.Courier{ID, TenantID, StoreID, Name string; TrackingURLTemplate *string}`; `stores.Store.ShippingMode string`, `stores.Store.OwnFreeShippingMin *decimal.Decimal`; `order.Order.ShippingOptionName, ShippingOptionEstimate *string`; `shipping.ShipmentRecord.CourierID *uuid.UUID, CourierName *string, TrackingURL *string`.

- [ ] **Step 1:** Write the up/down migration with the columns above (FKs to `stores(id)` ON DELETE CASCADE; index `(store_id, position)`). Check the cancelled status literal used in `shipments.status` in `internal/shipping` before writing the partial index.
- [ ] **Step 2:** Add the Go structs/fields above (gorm tags matching columns; `TableName()` for the two new structs).
- [ ] **Step 3:** Add both tables to `purgePlan` as store-scoped steps (`storeScoped("store_delivery_options", …)`, `storeScoped("store_couriers", …)`), before `stores`-dependent deletes.
- [ ] **Step 4:** Run `TEST_DATABASE_URL=… go test -tags=integration ./internal/tenantpurge/ ./internal/stores/ ./internal/order/` — expected PASS (schema coverage test fails if a table was missed).
- [ ] **Step 5:** Commit `feat(marketplace-api): own-delivery schema`.

### Task 2: Delivery settings service and admin API

**Files:**
- Create: `services/marketplace-api/internal/delivery/service.go`, `service_test.go`, `tracking.go`, `tracking_test.go`
- Create: `services/marketplace-api/internal/handlers/admin/delivery_settings.go`, `delivery_settings_test.go`
- Modify: `internal/handlers/admin/routes.go` (mount under `storeRoute.Group("/settings/delivery")`, `RequireTenantRelation(authz.RoleAdmin)` like `/settings/shipping`), `cmd/marketplace-api/main.go` (wire handler into deps)

**Interfaces:**
- Produces:
  - `delivery.BuildTrackingURL(template, number string) (string, bool)` — replaces `{number}` with `url.QueryEscape(strings.TrimSpace(number))`; returns false if template empty, result not `https`, or number empty.
  - `delivery.ValidateTemplate(t string) error` — https, exactly one `{number}`, ≤ 500.
  - `delivery.Service` with `Get(ctx, storeID) (Settings, error)`, `SetMode(ctx, storeID, mode string) error`, `ReplaceOptions(ctx, tenantID, storeID string, opts []OptionInput) ([]Option, error)` (validates, ≤ 5, positions by array order, deletes missing), `ListCouriers`, `CreateCourier`, `UpdateCourier`, `DeleteCourier`, `SetOwnFreeShippingMin(ctx, storeID string, v *decimal.Decimal) error`, `EnabledOption(ctx, storeID, optionID string) (*Option, error)` (nil+NotFound when missing/disabled/other store).
  - `delivery.Settings{Mode string; FreeShippingMin *decimal.Decimal; Options []Option; Couriers []Courier}`
  - Routes: `GET /settings/delivery` → Settings; `PUT /settings/delivery/mode {mode}`; `PUT /settings/delivery/options {options:[{id?,name,price,estimate?,enabled}], free_shipping_min?}`; `POST|PATCH|DELETE /settings/delivery/couriers[/:courierId]`.

- [ ] **Step 1: Write failing tests** in `tracking_test.go`:
  - `BuildTrackingURL("https://c.in/t?awb={number}", "AB 12&#")` → `"https://c.in/t?awb=AB+12%26%23", true`
  - `BuildTrackingURL("http://c.in/{number}", "1")` → `false`; `BuildTrackingURL("", "1")` → `false`
  - `ValidateTemplate` rejects no `{number}`, two `{number}`, `javascript:…`, 501 chars; accepts the https example.
- [ ] **Step 2:** Run `go test ./internal/delivery/ -run Tracking` → FAIL (undefined).
- [ ] **Step 3:** Implement `tracking.go`.
- [ ] **Step 4: Write failing service tests** (`service_test.go`, integration tag): 6 options → validation error; name 41 chars → error; price −1 → error; ReplaceOptions keeps order; `EnabledOption` returns NotFound for disabled, for another store's id, and for a malformed id; `SetMode("x")` → error.
- [ ] **Step 5:** Implement `service.go`; run tests → PASS.
- [ ] **Step 6: Handler tests** (`delivery_settings_test.go`): GET returns mode `carrier` by default; PUT options round-trips; courier create rejects bad template with 400 `validation_failed`.
- [ ] **Step 7:** Implement handler + routes + wiring; run `go test ./internal/handlers/admin/ -run Delivery ./internal/routemanifest/` → PASS (update manifest with `-update` if it lists the new admin routes).
- [ ] **Step 8:** Commit `feat(marketplace-api): own-delivery settings API`.

### Task 3: Checkout — quote and re-check own options

**Files:**
- Modify: `internal/handlers/storefront/shipping_rates.go` (`GetRates`, `shippingRateResponse`), `internal/handlers/storefront/checkout_ext.go` (`calculateShipping` and its caller around line 584, order build around 667)
- Modify: `cmd/marketplace-api/main.go` (give both handlers a `*delivery.Service`, e.g. `WithDelivery(svc)`)
- Test: `internal/handlers/storefront/own_delivery_checkout_test.go`

**Interfaces:**
- Consumes: `delivery.Service.Get`, `delivery.Service.EnabledOption` (Task 2); `stores.Store.ShippingMode`, `OwnFreeShippingMin` (Task 1).
- Produces: `shippingRateResponse` gains `Name string \`json:"name,omitempty"\`` and `Estimate string \`json:"estimate,omitempty"\``; own rows: `Service: "own:"+id, Carrier: "own"`. `calculateShipping` returns carrier `"own"` and also an option snapshot; change its signature to `(decimal.Decimal, shippingChoice, error)` with `type shippingChoice struct{ Carrier string; OptionName, OptionEstimate *string }` and update the single caller.

- [ ] **Step 1: Failing tests:**
  - `TestGetRates_OwnModeReturnsOnlyEnabledOwnOptions` — store mode `own`, options Standard 100 (on), Express 200 (off) → one row `{service:"own:<id>", carrier:"own", name:"Standard", price:"100.00", currency_code: store currency}`; carrier never called (fake carrier factory fails the test if invoked).
  - `TestGetRates_OwnModeNoEnabledOptionsIsEmpty` → `200 {data: []}`.
  - `TestGetRates_CarrierModeIgnoresOwnOptions` → existing carrier rows only.
  - `TestCheckout_OwnOptionPricedFromServer` — client sends `shipping_service:"own:<id>"`; order `shipping_total=100.00`, `shipping_carrier="own"`, `shipping_option_name="Standard"`.
  - `TestCheckout_OwnFreeShippingThreshold` — `own_free_shipping_min=1000`, subtotal 1200 → shipping 0.
  - `TestCheckout_RejectsForgedOwnService` — table: disabled option, other store's option id, `own:not-a-uuid`, `own:` in carrier mode, carrier service (`"standard"`) in own mode → each 400 `validation_failed`, no order row.
- [ ] **Step 2:** Run `go test ./internal/handlers/storefront/ -run 'OwnMode|Own'` → FAIL.
- [ ] **Step 3:** In `GetRates`, after resolving the store: if `store.ShippingMode == "own"`, return enabled options (position order) as rows and stop. In `calculateShipping`, branch first on mode: `own` → require `own:` prefix, `EnabledOption`, apply `OwnFreeShippingMin` against the order subtotal; return `ValidationFailed("shipping_service", …)` otherwise; `carrier` → reject `own:` prefix, then existing code unchanged. Persist the snapshot fields on the order.
- [ ] **Step 4:** Run the tests plus the existing storefront suite `go test ./internal/handlers/storefront/` → PASS.
- [ ] **Step 5:** Commit `feat(marketplace-api): price own delivery options at checkout`.

### Task 4: Tracking URL through email and storefront API

**Files:**
- Modify: `internal/orderdoc/service.go` (`ShipmentInfo.TrackingURL string`), `internal/orderdoc/mailer.go`, `internal/orderdoc/templates/shipment_dispatched_email.html`, `.txt`
- Modify: `internal/handlers/admin/shipments.go` `dispatchShipmentDispatchedEmail` (pass `rec.TrackingURL`, and `rec.CourierName` as carrier display when set)
- Modify: `internal/handlers/storefront/order_detail.go` (add `tracking_url` to shipment projection)
- Test: `internal/orderdoc/mailer_test.go` (existing file; add cases), `internal/handlers/storefront/order_detail_test.go`

**Interfaces:**
- Produces: JSON `shipment.tracking_url` (string, omitted when empty) on storefront order detail; email shows a "Track your parcel" link when `TrackingURL != ""`.

- [ ] **Step 1: Failing tests:** mailer renders `href="https://c.in/t?awb=AB1"` and the text template includes the URL when set; renders neither when empty (carrier shipments unchanged). Order detail returns `tracking_url` for a shipment row that has one.
- [ ] **Step 2:** Run → FAIL. **Step 3:** Implement (html/template escapes the href). **Step 4:** Run `go test ./internal/orderdoc/ ./internal/handlers/storefront/ -run 'Dispatched|OrderDetail'` → PASS; `bash ../../scripts/ci/verify-logging-smoke.sh` → PASS.
- [ ] **Step 5:** Commit `feat(marketplace-api): carry tracking links to the shipped email and order page`.

### Task 5: Manual shipment endpoint

**Files:**
- Create: `internal/handlers/admin/shipments_manual.go`, `shipments_manual_test.go`
- Modify: `internal/handlers/admin/routes.go` (`orders.POST("/:id/shipments/manual", RequireTenantRelation(authz.OrdersEditRole), …)`, `orders.PATCH("/:id/shipments/:shipmentId/tracking", …)`)

**Interfaces:**
- Consumes: `delivery.Service` courier lookup + `BuildTrackingURL` (Task 2), `ShipmentRecord` fields (Task 1), `dispatchShipmentDispatchedEmail` (Task 4), `h.orderSvc.MarkFulfillmentComplete(ctx, tx, orderID)`.
- Produces: `POST …/shipments/manual {courier_id, tracking_number?}` → 201 shipment; `PATCH …/shipments/:shipmentId/tracking {courier_id?, tracking_number}` → 200 (own shipments only; no email).

- [ ] **Step 1: Failing tests:**
  - `TestManualShipment_CreatesShippedAndEmailsOnce` — own order, courier Anjani template → shipment `carrier='own'`, `courier_name='Anjani'`, `tracking_url` built, `status=in_transit`, all `order_allocations.shipment_id` set, order fulfilment complete, one email dispatched.
  - `TestManualShipment_RefusedForCarrierOrder` — `orders.shipping_carrier='delhivery'` → 409 `not_own_delivery`.
  - `TestManualShipment_RefusedWhenActiveShipmentExists` → 409 `already_shipped`; two concurrent POSTs → one 201, one 409 (unique index), one email.
  - `TestManualShipment_CourierFromAnotherStore` → 404.
  - `TestManualShipment_NoTemplateStoresNumberOnly` — `tracking_url` NULL.
  - `TestTrackingPatch_FixesNumberWithoutEmail` — URL rebuilt, no second email; PATCH on a carrier shipment → 409.
- [ ] **Step 2:** Run → FAIL. **Step 3:** Implement in one transaction: check order is own + no active shipment, insert shipment (`ship_to` = order shipping address JSON, `ship_from` = store default warehouse address or `{}`), stamp allocations, `MarkFulfillmentComplete`; map the unique-index violation to 409; after commit call `dispatchShipmentDispatchedEmail`. **Step 4:** Run `go test -tags=integration ./internal/handlers/admin/ -run 'ManualShipment|TrackingPatch'` and `./internal/routemanifest/` → PASS.
- [ ] **Step 5:** Commit `feat(marketplace-api): mark own-delivery orders shipped with a courier and tracking`.

### Task 6: Admin settings UI

**Files:**
- Create: `apps/admin/components/settings/OwnDeliverySettings.tsx`, `OwnDeliverySettings.test.tsx`
- Create: `apps/admin/lib/api/delivery-api.ts`, `apps/admin/app/(admin)/settings/shipping/delivery-actions.ts`
- Modify: `apps/admin/components/settings/ShippingSettingsClient.tsx` (mode switch at top; render carrier cards when `carrier`, `OwnDeliverySettings` when `own`), `apps/admin/app/(admin)/settings/shipping/page.tsx` (load delivery settings)

**Interfaces:**
- Consumes: Task 2 routes.
- Produces: `getDeliverySettings(storeId, session)`, `setShippingMode`, `saveDeliveryOptions`, `createCourier`, `updateCourier`, `deleteCourier` in `delivery-api.ts`; server actions of the same names in `delivery-actions.ts`.

- [ ] **Step 1: Failing tests** (vitest):
  - switch renders "Mark8ly couriers" / "Own delivery", current mode checked; choosing Own calls `setShippingMode("own")`.
  - first visit with no options pre-fills one "Standard" row with empty price.
  - a sixth "Add option" is disabled at 5.
  - courier template without `{number}` shows the error and doesn't save; **Test link** opens `template.replace("{number}","TEST123")` in a new tab.
  - own mode with zero enabled options shows the warning "Buyers can't check out until at least one option is on."
- [ ] **Step 2:** Run `npx vitest run components/settings/OwnDeliverySettings` → FAIL. **Step 3:** Implement using existing settings form components/tokens (match `ShippingConfigForm.tsx`). **Step 4:** Run the test + `npx tsc --noEmit -p .` → PASS.
- [ ] **Step 5:** Commit `feat(admin): own delivery settings`.

### Task 7: Admin order page — Mark as shipped

**Files:**
- Create: `apps/admin/components/orders/MarkShippedPanel.tsx`, `MarkShippedPanel.test.tsx`
- Modify: `apps/admin/app/(admin)/orders/[id]/page.tsx` (render `MarkShippedPanel` when `order.shipping_carrier === "own"`, else `ShippingLabelPanel` as today), `apps/admin/lib/api/shipping-api.ts` (`createManualShipment`, `updateShipmentTracking`)

**Interfaces:**
- Consumes: Task 5 routes, Task 6 `getDeliverySettings` (couriers list).

- [ ] **Step 1: Failing tests:** own order with no shipment shows courier select (defaults to last used, from `localStorage` key `m8.lastCourier.<storeId>`) + tracking input + "Mark as shipped"; submit calls `createManualShipment` once and disables the button while pending; after shipping, shows courier, number and link with an "Edit tracking" action; carrier order renders `ShippingLabelPanel`, not this panel; no couriers saved → link to Settings → Shipping.
- [ ] **Step 2:** Run → FAIL. **Step 3:** Implement. **Step 4:** Run tests + `tsc` → PASS.
- [ ] **Step 5:** Commit `feat(admin): mark own-delivery orders shipped`.

### Task 8: Storefront checkout and order page

**Files:**
- Modify: `apps/storefront/lib/api/checkout-api.ts` (rate type: `name?`, `estimate?`), `apps/storefront/app/checkout/page.tsx` (radio label: `name ?? service`; show `estimate` when present, else existing `estimated_days` copy), `apps/storefront/components/OrderTimeline.tsx` (link the tracking number when `tracking_url`)
- Test: `apps/storefront/lib/` pure helpers per frontend-test-topology (storefront vitest runs in `node`; put label/link logic in `apps/storefront/lib/shipping-display.ts` with `shipping-display.test.ts`)

**Interfaces:**
- Produces: `rateLabel(rate): string`, `rateEstimate(rate): string | null`, `trackingHref(shipment): string | null` (returns only `https://` URLs) in `lib/shipping-display.ts`.

- [ ] **Step 1: Failing tests:** `rateLabel({service:"own:x",name:"Standard"})==="Standard"`, carrier rate keeps today's label; `rateEstimate({estimate:"3–6 days"})==="3–6 days"`; `trackingHref({tracking_url:"https://c.in/t?awb=1"})` returns it, `"javascript:alert(1)"` → null, missing → null.
- [ ] **Step 2:** Run `npx vitest run lib/shipping-display` → FAIL. **Step 3:** Implement and use from the page and `OrderTimeline` (`<a target="_blank" rel="noopener noreferrer">`). **Step 4:** Run storefront vitest + `tsc` → PASS.
- [ ] **Step 5:** Commit `feat(storefront): show own delivery options and tracking links`.

### Task 9: End-to-end check and PR

- [ ] **Step 1:** Run full suites: `cd services/marketplace-api && go test ./... && TEST_DATABASE_URL=… go test -tags=integration ./internal/delivery/ ./internal/handlers/... ./internal/tenantpurge/`; `cd apps/admin && npx vitest run && npx tsc --noEmit -p .`; `cd apps/storefront && npx vitest run && npx tsc --noEmit -p .`; `bash scripts/ci/verify-logging-smoke.sh`.
- [ ] **Step 2:** Manually walk the flow on a local stack (Bondi demo store): switch to Own delivery, add Standard ₹/A$ and a courier with a template, place a sandbox order, Mark as shipped, confirm the email link and the order-page link; switch back to carriers and confirm checkout shows carrier rates.
- [ ] **Step 3:** Open the PR referencing the spec; merge once green (user's standing instruction).
