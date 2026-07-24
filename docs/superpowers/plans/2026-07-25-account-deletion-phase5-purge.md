# Account Deletion Phase 5 — Tenant Data Purge Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Goal:** When a tenant is deleted, actually purge all of that tenant's domain data from marketplace-api (~90 tables) — closing the deletion contract that the MVP (Phases 1–4) left as a dead-lettered `tenant.deleted` event.

**Architecture:** platform-api already enqueues a transactional `tenant.deleted` outbox event `{tenant_id, store_ids}` but has NO handler for it (it currently dead-letters). Register a handler on platform-api's existing outbox drainer that calls a new `marketplaceapi.PurgeTenant` HTTP client method → a new marketplace-api `POST /internal/tenants/:tenantID/purge` (existing `X-Internal-Auth` internal namespace) → a new `internal/tenantpurge` routine that deletes the tenant's rows across all domain tables in FK-safe order inside one transaction, idempotently. **No new infrastructure** (no Pub/Sub topic, no chart env) — everything reuses existing plumbing.

**Tech Stack:** Go 1.26, GORM, Gin, platform-api outbox drainer, existing internal-auth HTTP path.

## Global Constraints

- **Modules/paths:** `github.com/mark8ly/platform-api` and the marketplace-api module, both in the root `go.work`. Services live at `services/platform-api` and `services/marketplace-api`.
- 🔴 **GOWORK=off on ALL Go commands** (local go 1.26.4 < go.work 1.26.5): `cd services/<svc> && GOWORK=off go test ./...`, `GOWORK=off go build ./...`, `GOWORK=off go vet ./...`.
- 🔴 **No new third-party imports** (reuse gin/gorm/existing internal packages). If any added: `cd services/<svc> && GOWORK=off go mod tidy`.
- **No new migration** unless Task 5 (optional idempotency ledger) is built; if so, follow each service's migration numbering + version-constant convention and bump it. Tasks 1–4 add none.
- **4 GLOBAL tables must NEVER be deleted:** `supported_countries`, `fx_rates`, `shipping_zones` (and the per-store `document_number_seq` is harmless — leave it). Deleting any global table corrupts other tenants → this is the single most dangerous mistake in the whole plan.
- **The `tenant.deleted` contract (from the shipped MVP, `services/platform-api/internal/account/service.go`):** kind string literal `"tenant.deleted"`; payload `{ "tenant_id": string, "store_ids": []string }` (store_ids captured before the DB cascade). Subscribe to exactly this.
- **Idempotency:** every purge must be safely re-runnable — the drainer retries. Per-table `DELETE WHERE tenant_id/store_id` is naturally idempotent; ordering must hold on every attempt.
- **Git:** commit directly to `main`, single-line conventional messages, no signature, one per task.
- **Reference:** the Phase 5 scoping map (this feature's investigation) classifies tables as (A) cascade-from-`stores`, (B) RESTRICT-blocked (categories, products, csv_import_jobs), (C) denormalized roots with no stores FK (orders, abandoned_carts, audit_logs), (D) children of B/C, (E) tenant-only (vendors, push tokens, sso configs, break-glass, attestations, …), (F) GLOBAL — do not touch. Migrations are the authoritative source: `services/marketplace-api/migrations/*.sql`.

---

## File Structure

**platform-api**
- `internal/account/tenant_deleted_handler.go` — **new**: outbox handler for kind `tenant.deleted` → `marketplaceClient.PurgeTenant`.
- `internal/marketplaceapi/vendor_client.go` — **modify**: add `PurgeTenant(ctx, tenantID, storeIDs) error`.
- `cmd/server/main.go` — **modify**: register the handler next to the existing `fga.write_membership` / GIP-claim registrations (~lines 311/314).
- Tests alongside each.

**marketplace-api**
- `internal/tenantpurge/purge.go` — **new**: the ordered, idempotent, single-transaction delete routine `Purge(ctx, db, tenantID, storeIDs) error`.
- `internal/tenantpurge/purge_test.go` — **new**: unit (order/global-safety) + integration (seed-full-tenant → purge → assert-empty → re-run).
- `internal/handlers/internalsvc/tenant_purge.go` — **new**: `POST /internal/tenants/:tenantID/purge` handler.
- `cmd/marketplace-api/main.go` — **modify**: mount the route on the existing `/internal` group (`RequireInternalAuth`).
- (Optional Task 5) `internal/tenantpurge` ledger + migration.

---

## Task 1: marketplace-api purge routine (`internal/tenantpurge`)

**This is the core and the riskiest task — build it first so its tests anchor everything.**

**Files:**
- Create: `services/marketplace-api/internal/tenantpurge/purge.go`
- Test: `services/marketplace-api/internal/tenantpurge/purge_test.go`

**Interfaces:**
- Produces: `func Purge(ctx context.Context, db *gorm.DB, tenantID string, storeIDs []string) error` — deletes every row belonging to the tenant across all domain tables, in FK-safe order, inside ONE `db.Transaction`. Idempotent. Never touches global tables.

**Design (derive the exact table list from the migrations — do NOT trust a hand-copied list; the integration test is the real gate):**
1. Read `services/marketplace-api/migrations/*.sql`. Build the authoritative set of domain tables and their FK edges + ON DELETE behavior.
2. Delete in this order inside one transaction (children → parents), scoping each `WHERE tenant_id = ?` when the table has `tenant_id`, else `WHERE store_id IN (?)` using `storeIDs`:
   - **(1) Financial/audit leaves first (RESTRICT-ordered, real money — order is load-bearing):** refund_audit, refund_transactions_saga, refund_transactions, platform_fee_ledger, order_tax_lines, coupon_usage, webhook_events, payment_transactions, shipment_cancel_actions, shipments.
   - **(2) Order children → orders:** return_items → returns, order_events, order_addresses, order_items, then orders; abandoned_carts; csv_import_jobs.
   - **(3) Product/review subtree through RESTRICT edges:** product_categories, reviews subtree (review_reactions/review_replies/review_media → reviews), wishlists, product_notify_subscriptions; then products; then categories.
   - **(4) vendors** (AFTER products — `products.vendor_id` is NOT NULL).
   - **(5) tenant-only tables** (E): sso configs/mappings, storefront_push_tokens, admin_push_tokens, break_glass_accounts/lockouts, enterprise_api_keys, attestations, billing_archive, audit_logs, reminders (payment_action/trial), warehouses, white_label lifecycle/state, etc.
   - **(6) `stores` LAST** — its ON DELETE CASCADE sweeps all Category-A config/child tables automatically.
3. NEVER emit a DELETE against `supported_countries`, `fx_rates`, `shipping_zones`.
4. Use raw `tx.Exec("DELETE FROM <t> WHERE ...")` or GORM `tx.Where(...).Delete(...)`; a delete hitting zero rows is success (idempotent).

- [ ] **Step 1: Write the failing INTEGRATION test** (`//go:build integration`, needs the marketplace-api Postgres — model on existing `*_integration_test.go` in the repo):
  - Seed one tenant + one store with at least one row in EVERY domain table that has tenant_id/store_id (helper that inserts across the schema), plus rows in the 3 GLOBAL tables and rows for a SECOND tenant.
  - Call `Purge(ctx, db, tenant1, [store1])`.
  - Assert: zero rows remain for tenant1 across every domain table; the GLOBAL tables are untouched; the SECOND tenant's rows are all intact.
  - Re-run `Purge` → returns nil (idempotent), still zero/intact.
  > If standing up the integration DB is not possible in this environment, write the integration test (build-tagged, so it compiles but skips) AND add a unit test that runs `Purge` against a sqlmock/`go-txdb` or a table-name-ordering assertion (capture the emitted SQL statement order via a gorm `Session` dry-run / a `ConnPool` spy) proving (a) the delete order is financial-leaves-first, (b) no global table name ever appears, (c) `stores` is last. State clearly in the report which ran.
- [ ] **Step 2: Run** `cd services/marketplace-api && GOWORK=off go test ./internal/tenantpurge/ -v` → FAIL (package/func absent).
- [ ] **Step 3: Implement** `purge.go` per the design. Keep the ordered table list in one clearly-commented slice so the order is auditable in one place.
- [ ] **Step 4: Run** the tests → PASS. Also `GOWORK=off go build ./... && GOWORK=off go vet ./internal/tenantpurge/`.
- [ ] **Step 5: Commit** `feat(marketplace-api): tenant data purge routine (FK-ordered, idempotent)`.

---

## Task 2: marketplace-api purge endpoint

**Files:**
- Create: `services/marketplace-api/internal/handlers/internalsvc/tenant_purge.go`
- Test: `services/marketplace-api/internal/handlers/internalsvc/tenant_purge_test.go`
- Modify: `services/marketplace-api/cmd/marketplace-api/main.go`

**Interfaces:**
- Consumes: `tenantpurge.Purge`, the DB handle, the existing `RequireInternalAuth` middleware + `/internal` group (read `internal/handlers/internalsvc/audit_ingest.go` for the exact internal-auth pattern and `cmd/marketplace-api/main.go` for where `/internal` is mounted).
- Produces: `POST /internal/tenants/:tenantID/purge`, body `{"store_ids": ["..."]}`; runs `Purge`; 200 on success (and on idempotent replay); 500 on purge error (so the caller/drainer retries); guarded by `X-Internal-Auth`.

- [ ] **Step 1: Write the failing test** — gin `httptest`, fake purge func (inject a `purgeFn` so no DB): 200 on success + purge called with the path tenantID + body store_ids; purge error → 500. (Mirror the internalsvc test style.)
- [ ] **Step 2: Run** `cd services/marketplace-api && GOWORK=off go test ./internal/handlers/internalsvc/ -run TenantPurge -v` → FAIL.
- [ ] **Step 3: Implement** the handler (inject the purge function/dependency for testability) + register the route on the existing `/internal` group with `RequireInternalAuth`.
- [ ] **Step 4: Run** `GOWORK=off go test ./internal/handlers/internalsvc/... && GOWORK=off go build ./...` → PASS.
- [ ] **Step 5: Commit** `feat(marketplace-api): POST /internal/tenants/:id/purge endpoint`.

---

## Task 3: platform-api `PurgeTenant` client method

**Files:**
- Modify: `services/platform-api/internal/marketplaceapi/vendor_client.go`
- Test: `services/platform-api/internal/marketplaceapi/vendor_client_test.go` (extend/create)

**Interfaces:**
- Produces: `func (c *VendorClient) PurgeTenant(ctx context.Context, tenantID string, storeIDs []string) error` → `POST /internal/tenants/<tenantID>/purge` with body `{"store_ids": storeIDs}` + `X-Internal-Auth`; non-2xx → error (so the outbox drainer retries). Mirror the existing POST method(s) in `vendor_client.go`.

- [ ] **Step 1: Write the failing test** — `httptest` server asserts POST, path `/internal/tenants/t1/purge`, `X-Internal-Auth` header, body carries store_ids; 200 → nil; 500 → error.
- [ ] **Step 2: Run** `cd services/platform-api && GOWORK=off go test ./internal/marketplaceapi/ -run PurgeTenant -v` → FAIL.
- [ ] **Step 3: Implement** mirroring the existing client method shape (headers, error mapping).
- [ ] **Step 4: Run** → PASS + `GOWORK=off go build ./...`.
- [ ] **Step 5: Commit** `feat(platform-api): marketplaceapi PurgeTenant client method`.

---

## Task 4: platform-api `tenant.deleted` outbox handler + wiring

**This closes the dead-letter gap — after this, the MVP's enqueued event actually drives a purge.**

**Files:**
- Create: `services/platform-api/internal/account/tenant_deleted_handler.go`
- Test: `services/platform-api/internal/account/tenant_deleted_handler_test.go`
- Modify: `services/platform-api/cmd/server/main.go`

**Interfaces:**
- Consumes: the outbox drainer's handler-registration API (read `internal/outbox/outbox.go` — the handler signature the drainer dispatches to, and how `fga.write_membership` / GIP-claim handlers are registered at `cmd/server/main.go` ~311/314), `marketplaceapi.(*VendorClient).PurgeTenant`, the payload shape `{tenant_id, store_ids}`.
- Produces: a handler function/struct that unmarshals the `tenant.deleted` payload and calls `PurgeTenant`; returns the client error unchanged so the drainer retries with backoff (never swallow — a swallowed error silently orphans the data).

- [ ] **Step 1: Write the failing test** — construct the handler with a fake purge client; feed it a `tenant.deleted` payload `{"tenant_id":"t1","store_ids":["s1"]}`; assert it calls `PurgeTenant("t1", ["s1"])`; assert a client error propagates out (so the drainer will retry); assert a malformed payload returns an error (not a panic).
- [ ] **Step 2: Run** `cd services/platform-api && GOWORK=off go test ./internal/account/ -run TenantDeletedHandler -v` → FAIL.
- [ ] **Step 3: Implement** the handler, then register it in `cmd/server/main.go` for kind `"tenant.deleted"` next to the existing handler registrations, passing the already-constructed `marketplaceapi` client (confirm it's built there; the MVP wiring referenced `cfg.MarketplaceAPIURL` + `MarketplaceInternalAuthSecret`). Guard registration if the client is nil (dev/unconfigured), mirroring existing optional-handler gating.
- [ ] **Step 4: Run** `GOWORK=off go test ./internal/account/... && GOWORK=off go build ./...` → PASS.
- [ ] **Step 5: Commit** `feat(platform-api): consume tenant.deleted → purge marketplace tenant data`.

---

## Task 5 (OPTIONAL): idempotency ledger

Only build if the team wants a fast replay short-circuit / audit trail. A `tenant_purges(tenant_id PK, store_ids, completed_at)` row (model on the existing `idempotency_keys` table) written at the end of a successful purge; the endpoint returns 200 immediately if a row exists. Requires a new marketplace-api migration (+ version-constant bump). If built, add the migration, the ledger check in Task 2's handler, and a test that a second purge call short-circuits. **Default: SKIP for the first cut** — the per-table deletes are already idempotent; add only if replay volume/audit demands it.

---

## Task 6: end-to-end verification

- [ ] `cd services/marketplace-api && GOWORK=off go build ./... && GOWORK=off go test ./...` → pass.
- [ ] `cd services/platform-api && GOWORK=off go build ./... && GOWORK=off go test ./...` → pass.
- [ ] If integration DB available: run the `-tags=integration` purge test and confirm the seed-full-tenant → empty → idempotent flow. Otherwise note it as device/staging-verification.
- [ ] If any module gained an import: `GOWORK=off go mod tidy` and commit the delta.
- [ ] Trace the assembled path end-to-end by reading: `account.Service` emits `tenant.deleted{tenant_id, store_ids}` → drainer dispatches to the new handler → `PurgeTenant` POSTs → marketplace endpoint → `Purge` runs the ordered transaction. Confirm the payload field names match across all four hops (`tenant_id`, `store_ids`).
- [ ] **Staging validation (documented, mandatory before prod trust):** on staging, delete a throwaway test tenant that has data seeded across domains; confirm every domain table is empty for that tenant and NO other tenant's rows or global tables changed. This is the real-world gate for a destructive purge — do not skip it before enabling in prod.

---

## Self-Review

- **Coverage:** dead-letter gap closed (Task 4); purge routine (Task 1); transport (Tasks 2–3); the FK-ordering + global-table-safety + idempotency all pinned by Task 1's integration test, which is the true correctness gate (not the hand-listed order).
- **Placeholders:** the per-table order is specified but the implementer MUST derive/verify the exact table set from the migrations — this is deliberate (a hand-maintained 90-table SQL list in a plan would drift); the exhaustive integration test enforces completeness.
- **Type consistency:** `PurgeTenant(ctx, tenantID, storeIDs)` (Task 3) is called by the handler (Task 4) and hits the endpoint (Task 2) that calls `Purge(ctx, db, tenantID, storeIDs)` (Task 1). Payload `{tenant_id, store_ids}` consistent with the emitter across all hops.

## Risks

The purge is irreversible and cross-cutting. The two ways it goes wrong: (1) **deleting a global table** → other tenants corrupted (mitigation: explicit never-touch list + a test asserting no global table appears in the emitted SQL); (2) **wrong FK order** → a RESTRICT violation aborts the tx (safe — rolls back, drainer retries, no partial delete) OR an over-broad `store_id`/`tenant_id` scope hitting another tenant (mitigation: every DELETE scoped by the event's tenant_id/store_ids; the integration test seeds a second tenant and asserts it's untouched). Staging validation on a throwaway tenant is mandatory before enabling in prod.
