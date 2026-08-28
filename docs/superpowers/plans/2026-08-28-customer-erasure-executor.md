# #259 — GDPR erasure execution Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give `marketplace-api` a mechanism that takes a `customer_erasure_requests` row from `pending` to `completed`, actually erasing the customer's personal data — deleting what serves only the customer, anonymising what must survive for financial-record reasons, and recording what it did.

**Architecture:** Mirrors `internal/tenantpurge`. A **pure** `erasurePlan(storeID, email, token)` function returns an ordered list of steps, each a parameterised statement with a declared disposition (DELETE or ANONYMISE). An executor runs them in one transaction under an advisory lock. A schema-coverage guard fails when a table linking to a customer has no declared disposition, so a table added next year cannot silently escape erasure.

**Tech Stack:** Go 1.26, GORM, Postgres, golang-migrate, testify.

**Spec:** `docs/superpowers/2026-08-28-259-erasure-design-proposal.md` (merged in #432) and GitHub issue tesserix/mark8ly#259. Read the proposal first — it carries the full data-model survey this plan assumes.

## Global Constraints

- Run all Go commands from `services/marketplace-api`, never path-scoped, always `-count=1`.
- Required command set: `go build ./...`, `go vet ./...`, `go vet -tags=integration ./...`, `go test ./... -count=1`.
- Integration tests: `TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable'`, `-p 1`. **Without that variable they skip silently and a skip prints `ok`.** Any claim about an integration test must name the DSN. **Use the LAN IP — `localhost:5432` reaches a different Postgres with no `dev` role.**
- **The dev database is now migrated to 112.** New migration is **000113**, and `ExpectedSchemaVersion` must go 112 → 113 in the same commit.
- Commits: conventional, single line, no signature, no `Co-Authored-By` trailer, no emoji.
- Stage with explicit paths (`git add <path>`). Never `git add -A`.
- **Never log, and never write into a receipt, the subject's email, name, phone or address.** Counts and the request id are the diagnostics. This whole feature exists to remove that data.
- Work only inside this worktree: `.claude/worktrees/259-erasure-executor`.

## Decisions already made (by the product owner — implement these, do not re-open)

1. **Scope is `(store_id, email)`** — one request, one store. Matches the table's own `UNIQUE (store_id, customer_email)` and the storefront endpoint. Cross-store grouping stays a console concern.
2. **Erasure keys on `customer_email`, not `customer_id`.** Verified: `orders.customer_id` is set only when a logged-in profile is in context (`internal/handlers/storefront/checkout.go:175-181`), so guests have NULL, while `customer_email` is `NOT NULL`. `customer_id` is a *supplementary* match, never the primary one.
3. **Financial records are ANONYMISED, retained 7 years, legal-obligation basis** — matching `billing_archive`'s §23.2 window (`migrations/000046:24`). #365 chose that number so the estate has *"ONE retention story to defend rather than two that have to be reconciled"*; this reuses it for the same reason. Covers `orders`, `order_items`, `order_addresses`, `payment_transactions`, `refund_transactions`, `platform_fee_ledger`, `returns`, `shipments`, `coupon_usage`, `promo_redemptions`, `customer_loyalties`, `gift_cards`.
4. **Reviews are ANONYMISED**, not deleted — deleting retroactively changes a merchant's historical star rating. `review_media` (customer photographs) is deleted.
5. **JSONB blobs are OUT OF SCOPE**, with the risk recorded and a follow-up issue filed. Nine columns can embed a customer email or address; inspecting and safely key-stripping nine differently-shaped payloads is its own effort, and guessing at their shapes risks corrupting payment metadata.

## Verified findings (do not re-litigate)

1. **Nothing has ever processed a request** — zero `UPDATE` statements against `customer_erasure_requests` anywhere in the repo.
2. **The status vocabulary in #259 is WRONG.** The issue claims `processing`, `completed` and `failed` "exist in the schema". The real constraint (`migrations/000059:12`) is `CHECK (status IN ('pending', 'processed', 'rejected'))`. There is **no in-flight state**, so Task 1's migration is mandatory, not optional.
3. **The subject identifier is email only** — the table has `customer_email TEXT NOT NULL` and no `customer_id`, no `gip_uid`.
4. **`tenantpurge` cannot be reused.** Its coverage of `customer_profiles`, `customer_addresses`, `gift_cards`, `campaigns` and `campaign_recipients` is implicit — those are swept by the group-6 `stores` CASCADE and never named in a step. A per-customer erasure deletes no store, so it gets no CASCADE. **Explicit customer-scoped deletes for those tables do not exist anywhere today.**
5. **There is no anonymisation precedent in the codebase.** Everything matching `anonymi|redact|scrub|tombstone` is secret-redaction in API responses or log masking; nothing mutates a stored PII column. This plan invents the pattern.
6. **NOT NULL columns constrain the anonymisation.** `customer_profiles.email`, and `order_addresses.name`, `line1`, `city`, `country_code` are all `NOT NULL` — they must be overwritten with placeholders, not set to NULL. `order_addresses.country_code` is **kept as-is**: it is needed for tax reporting and is not personally identifying on its own.
7. **The inbox surface already exists and is waiting.** `internal/inbox/erasure.go` lists pending rows and declares `process` (`Destructive: true`) and `reject`; `cmd/marketplace-api/inbox_wiring.go:61-77` registers no executor for the kind, so it answers 501. The wiring comment says they should not get a one-click path *"before the behaviour beneath it is settled"* — this plan settles it.
8. **The executor interface is fixed:** `Execute(ctx context.Context, item inbox.Item, actionID, operatorID, notes string) (InboxActionResult, error)` (`inbox_actions.go:60`), returning `InboxActionResult{TenantID, StoreID, Status}` (`:40`).
9. **Idempotency machinery exists and should be reused:** `subscription.WithAdvisoryLock` (`advisory_lock.go:12-17`), and `inbox_action_idempotency` (migration 000107) which the action handler already uses to replay a stored outcome.

## File Structure

- `migrations/000113_customer_erasure_execution.up.sql` / `.down.sql` — status vocabulary, `attempts`, and the retention COMMENTs.
- `migrations.go` — `ExpectedSchemaVersion` 112 → 113.
- `internal/customererasure/plan.go` — the pure plan. **The reviewable artefact.**
- `internal/customererasure/token.go` — the anonymisation token.
- `internal/customererasure/executor.go` — runs the plan, writes the receipt.
- `internal/customererasure/models.go` — the request model and status constants.
- `internal/customererasure/plan_test.go` — pure unit tests, no DB.
- `internal/customererasure/coverage_integration_test.go` — the schema-coverage guard.
- `internal/customererasure/executor_integration_test.go` — end-to-end.
- `internal/handlers/platformadmin/inbox_action_erasure.go` — the inbox executor.
- `cmd/erasure-worker/main.go` — the console-down path.
- `Makefile` — add the package to `test-int`.

---

### Task 1: Migration — status vocabulary, attempts, and the retention basis

**Files:**
- Create: `services/marketplace-api/migrations/000113_customer_erasure_execution.up.sql` / `.down.sql`
- Modify: `services/marketplace-api/migrations.go`

**Interfaces:**
- Produces: statuses `processing`, `completed`, `failed` (in addition to the existing `pending`, `processed`, `rejected`); column `attempts INT NOT NULL DEFAULT 0`; documented retention on the financial tables.

- [ ] **Step 1: Write the up migration**

```sql
-- 000113 — GDPR art.17 erasure EXECUTION (#259).
--
-- Two things, both prerequisites for a worker that can actually run.
--
-- 1. A state machine. The original CHECK admitted only
--    ('pending','processed','rejected') — there was no in-flight state at
--    all, so a worker could not mark a row as being worked on, and a crash
--    mid-erasure was indistinguishable from one never started. #259
--    describes 'processing'/'completed'/'failed' as already existing; they
--    did not. 'processed' is kept as a terminal alias so existing rows stay
--    valid and no backfill is needed.
ALTER TABLE customer_erasure_requests
    DROP CONSTRAINT IF EXISTS customer_erasure_requests_status_check;

ALTER TABLE customer_erasure_requests
    ADD CONSTRAINT customer_erasure_requests_status_check
    CHECK (status IN ('pending', 'processing', 'completed', 'failed', 'processed', 'rejected'));

-- attempts bounds retry: a request that fails repeatedly must stop being
-- retried forever and become visible to an operator instead.
ALTER TABLE customer_erasure_requests
    ADD COLUMN IF NOT EXISTS attempts INT NOT NULL DEFAULT 0;

-- 2. The retention basis, written down where it is enforced.
--    Until now the ONLY retention text in any migration was
--    billing_archive's (000046). The erasure below ANONYMISES rather than
--    deletes these tables, and that choice needs its justification recorded
--    next to the data, not only in a design document.
COMMENT ON TABLE orders IS
    'Financial record. Personal fields are anonymised on GDPR art.17 erasure; the row is retained 7 years under legal-obligation basis, matching billing_archive (§23.2). See #259.';
COMMENT ON TABLE order_addresses IS
    'Financial record (delivery evidence). Personal fields anonymised on erasure; country_code retained for tax reporting. 7-year legal-obligation retention (§23.2). See #259.';
COMMENT ON TABLE payment_transactions IS
    'Financial record. Anonymised, not deleted, on GDPR art.17 erasure; retained 7 years under legal-obligation basis (§23.2). See #259.';
COMMENT ON TABLE refund_transactions IS
    'Financial record. Anonymised, not deleted, on GDPR art.17 erasure; retained 7 years under legal-obligation basis (§23.2). See #259.';
COMMENT ON TABLE platform_fee_ledger IS
    'Financial record. Anonymised, not deleted, on GDPR art.17 erasure; retained 7 years under legal-obligation basis (§23.2). See #259.';
```

- [ ] **Step 2: Write the down migration**

```sql
ALTER TABLE customer_erasure_requests DROP COLUMN IF EXISTS attempts;

-- Rows in a state the old constraint forbids must be normalised before it
-- can be re-added, otherwise the ALTER fails on real data.
UPDATE customer_erasure_requests SET status = 'processed' WHERE status = 'completed';
UPDATE customer_erasure_requests SET status = 'pending'   WHERE status IN ('processing', 'failed');

ALTER TABLE customer_erasure_requests
    DROP CONSTRAINT IF EXISTS customer_erasure_requests_status_check;
ALTER TABLE customer_erasure_requests
    ADD CONSTRAINT customer_erasure_requests_status_check
    CHECK (status IN ('pending', 'processed', 'rejected'));

COMMENT ON TABLE orders IS NULL;
COMMENT ON TABLE order_addresses IS NULL;
COMMENT ON TABLE payment_transactions IS NULL;
COMMENT ON TABLE refund_transactions IS NULL;
COMMENT ON TABLE platform_fee_ledger IS NULL;
```

- [ ] **Step 3: Find the real constraint name first**

The plan guesses `customer_erasure_requests_status_check`. Verify before relying on it:

```bash
docker exec dev-postgres-1 psql -U dev -d marketplace_db -tAc \
  "select conname from pg_constraint where conrelid='customer_erasure_requests'::regclass and contype='c'"
```

If the real name differs, use the real one in both migrations. Do not `DROP CONSTRAINT` a name that does not exist without `IF EXISTS`.

- [ ] **Step 4: Bump the schema version**

`services/marketplace-api/migrations.go`: `ExpectedSchemaVersion` 112 → **113**.

- [ ] **Step 5: Apply and verify**

```bash
cd services/marketplace-api
DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' go run ./cmd/migrate up
DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' go run ./cmd/migrate version
```

Expected: `version=113 dirty=false`. Then confirm the constraint admits the new values:

```bash
docker exec dev-postgres-1 psql -U dev -d marketplace_db -tAc \
  "select pg_get_constraintdef(oid) from pg_constraint where conrelid='customer_erasure_requests'::regclass and contype='c'"
```

- [ ] **Step 6: Build, vet, migration test, commit**

```bash
go build ./... && go vet ./... && go vet -tags=integration ./...
go test . -count=1 -run Migration -v
```

Expected: `TestExpectedSchemaVersionMatchesHighestMigration` PASS.

```bash
git add services/marketplace-api/migrations/000113_customer_erasure_execution.up.sql \
        services/marketplace-api/migrations/000113_customer_erasure_execution.down.sql \
        services/marketplace-api/migrations.go
git commit -m "feat(erasure): add the erasure state machine and record the 7-year financial retention basis (#259)"
```

---

### Task 2: The pure erasure plan and its coverage guard

**Files:**
- Create: `services/marketplace-api/internal/customererasure/token.go`
- Create: `services/marketplace-api/internal/customererasure/plan.go`
- Create: `services/marketplace-api/internal/customererasure/plan_test.go`
- Create: `services/marketplace-api/internal/customererasure/coverage_integration_test.go`

**Interfaces:**
- Produces:
  ```go
  type Disposition string
  const (
      DispositionDelete    Disposition = "delete"
      DispositionAnonymise Disposition = "anonymise"
  )

  type Step struct {
      Table       string
      Disposition Disposition
      SQL         string
      Args        []any
  }

  func Token(requestID uuid.UUID) string
  func erasurePlan(storeID uuid.UUID, email string, token string) []Step
  ```
  `erasurePlan` is unexported and **pure** — no DB handle, no I/O — so it is unit-testable and the coverage guard can assert against it. Task 3's executor consumes it.

This is the **reviewable artefact**. Get it right before any execution code exists.

- [ ] **Step 1: The anonymisation token**

Create `token.go`:

```go
package customererasure

import (
	"fmt"

	"github.com/google/uuid"
)

// Token is the value that replaces a subject's email everywhere the row must
// survive erasure. It is derived from the ERASURE REQUEST ID, not from the
// email: a hash of the email would still be a pseudonym of the personal data
// and brute-forceable against a known address list, whereas the request id is
// unrelated to the person and already exists.
//
// Deterministic per request, so two anonymised orders by the same subject
// still group together — which is what makes the financial record coherent
// after erasure — while identifying nobody.
//
// .invalid is reserved by RFC 2606 and can never be routed, so an anonymised
// address cannot accidentally receive mail.
func Token(requestID uuid.UUID) string {
	return fmt.Sprintf("erased+%s@erased.invalid", requestID.String())
}

// RedactedName replaces a person's name where the column is NOT NULL.
const RedactedName = "Erased customer"

// RedactedLine replaces a NOT NULL address line.
const RedactedLine = "[erased]"
```

- [ ] **Step 2: The plan**

Create `plan.go`. Every step carries the migration that introduced its table, matching `tenantpurge/purge.go`'s convention. Order matters only where an FK is `RESTRICT`; deletes of leaves come before their parents.

```go
// Package customererasure executes GDPR art.17 erasure for one customer in
// one store (#259).
//
// Scope is (store_id, email), matching customer_erasure_requests' own
// UNIQUE (store_id, customer_email). A person with accounts in three stores
// files three requests; grouping them is a console presentation concern.
//
// KEYED ON EMAIL, NOT customer_id. orders.customer_id is set only when a
// logged-in profile is in context (handlers/storefront/checkout.go:175-181),
// so guest orders carry NULL, while customer_email is NOT NULL. customer_id
// is matched as well where present, never instead.
//
// TWO DISPOSITIONS. A row that exists only to serve the customer is DELETED.
// A row that must survive for financial-record reasons is ANONYMISED: its
// personal fields are overwritten and the row is retained 7 years under
// legal-obligation basis, matching billing_archive (§23.2, migration 000046)
// — the same number #365 chose so the estate has one retention story rather
// than two.
//
// OUT OF SCOPE, deliberately: nine JSONB columns can embed a customer email
// or address (stripe_webhook_events.payload, payment_transactions.metadata,
// shipments.ship_from/ship_to, returns.pickup_details, and others). None are
// inspected here — safely key-stripping nine differently-shaped payloads is
// its own effort and guessing at their shapes risks corrupting payment
// metadata. Tracked separately; see the package's coverage guard, which
// records them as known residual PII rather than letting them pass silently.
package customererasure
```

Then the plan itself. Build it as `[]Step` in these groups:

**Group 1 — DELETE, customer-only leaves** (order matters: children before parents):
- `review_reactions` where `customer_profile_id IN (SELECT id FROM customer_profiles WHERE store_id = ? AND email = ?)` — 000017
- `review_media` where `review_id IN (SELECT id FROM reviews WHERE store_id = ? AND customer_email = ?)` — 000017
- `wishlists` (via `customer_id IN (SELECT id FROM customer_profiles ...)`) — 000018
- `customer_addresses` (same subquery) — 000013
- `abandoned_carts` where `store_id = ? AND customer_email = ?` — 000002
- `storefront_push_tokens` (via the profile subquery) — 000022
- `product_notify_subscriptions` (via the profile subquery) — 000023
- `campaign_recipients` where `customer_email = ?` scoped through `campaigns.store_id` — check the actual FK before writing this one
- `notifications` where `recipient_user_id IN (SELECT id::text FROM customer_profiles WHERE store_id = ? AND email = ?)` — 000091. **Note `recipient_user_id` is `varchar`, so the subquery must cast.** (Established while fixing #350: this column holds `customer_profiles.id`.)
- `email_sends` where `store_id = ? AND recipient = ?` — 000108

**Group 2 — ANONYMISE, financial and reputational**:
- `orders`: `SET customer_email = ?, customer_name = ?, customer_id = NULL WHERE store_id = ? AND customer_email = ?` — 000001
- `order_addresses`: `SET name = ?, line1 = ?, line2 = NULL, city = ?, region = NULL, postal_code = NULL, phone = NULL WHERE order_id IN (SELECT id FROM orders WHERE store_id = ? AND customer_email = ?)`. **`country_code` is deliberately NOT touched** — it is required for tax reporting and is not identifying on its own.
- `order_events`: `SET actor_email = ? WHERE actor_email = ? AND order_id IN (...)` — only rows whose actor is the customer
- `reviews`: `SET customer_email = ?, customer_name = ?, customer_profile_id = NULL WHERE store_id = ? AND customer_email = ?`
- `review_replies`: `SET author_email = ?, author_name = ? WHERE author_email = ?` — **only where `author_type` marks a customer**; check the column's values first
- `coupon_usage`, `promo_redemptions`, `customer_loyalties`: email → token, name → RedactedName
- `gift_cards`: **four roles per row** — `sender_email`, `recipient_email`, `purchased_by_email` and their name columns, plus the free-text `message`. Write one step per role that matches, and null the message where the subject is the sender.
- `support_tickets` / `tickets` / their reply tables: `submitted_by_email`/`author_email` → token, names → RedactedName, **only where the author is the customer** not staff
- `payment_transactions`, `refund_transactions`, `platform_fee_ledger`, `returns`, `shipments`: these link via `order_id`; anonymise any directly-held personal column. **Inspect each table's columns first** — several hold personal data only inside JSONB, which is out of scope.

**Group 3 — DELETE last**:
- `customer_profiles`: `WHERE store_id = ? AND email = ?` — 000013. Last, because groups 1 and 2 reference it by subquery.

**Before writing any step, verify the column exists.** The proposal's survey is good but was not exhaustive on every column. Run:
```bash
docker exec dev-postgres-1 psql -U dev -d marketplace_db -c "\d <table>"
```
for each table you touch. **If a column the plan names does not exist, STOP and report it — do not invent a substitute.**

- [ ] **Step 3: Pure unit tests**

Create `plan_test.go` (no build tag, no DB). Assert:
- Every step's SQL is non-empty and every `?` has a matching arg.
- `customer_profiles` is the **last** step (nothing may reference it afterwards).
- Every step declares a `Disposition`.
- No step's SQL contains the raw email as a literal — every use is a bound parameter. (Guards against a future edit interpolating it into the string.)
- `Token` is deterministic and contains neither the email nor a name.

- [ ] **Step 4: The coverage guard — the most important test in this change**

Create `coverage_integration_test.go` (`//go:build integration`), modelled on `internal/tenantpurge/schema_coverage_integration_test.go`. Read that file first and mirror its shape.

It must: read the **live schema**, find every table with a column that links to a customer (`customer_id`, `customer_email`, `customer_profile_id`, `recipient_user_id`, `author_email`, `submitted_by_email`, `actor_email`, `recipient`, `sender_email`, `purchased_by_email`, `email`), and fail when such a table is neither in the plan nor in a `declaredExclusions` map with a written justification.

Seed the exclusions with the ones already established, each with its reason:

```go
declaredExclusions := map[string]string{
	"user_profiles":            "GIP merchant/staff uid, not a customer",
	"admin_push_tokens":        "staff device tokens",
	"store_subscriptions":      "merchant billing contact, not a customer",
	"billing_archive":          "merchant billing record; 7-year legal-obligation retention (§23.2)",
	"warehouses":               "merchant facility contact",
	"store_branding":           "merchant support address",
	"tenant_sso_user_mappings": "merchant SSO identity",
	"enterprise_api_keys":      "created_by is a merchant user",
	"audit_logs":               "governance record; operator rows are retained deliberately (#288, #365)",
	"customer_erasure_requests": "the request itself — its own status is the receipt; retained as evidence the erasure happened",
	"webhook_events":           "no tenant, store or order column; cannot be scoped. JSONB payload — known residual PII, out of scope (#259)",
	"stripe_webhook_events":    "JSONB payload only — known residual PII, out of scope (#259)",
}
```

**Every exclusion must carry a justification string, and the test must fail if one is empty.** That is what stops the map becoming a place to silence the guard.

- [ ] **Step 5: Run and mutation-test the guard**

```bash
cd services/marketplace-api
go test ./internal/customererasure/ -count=1 -v
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 ./internal/customererasure/ -v
```

Expected: all pass.

**Mutation test:** delete one step from the plan — say `wishlists`. Re-run the coverage guard.
Expected: **FAIL**, naming `wishlists`. Restore; confirm pass.

If removing a step does not fail the guard, the guard is not reading what it claims to — STOP and escalate.

- [ ] **Step 6: Commit**

```bash
git add services/marketplace-api/internal/customererasure/
git commit -m "feat(erasure): add the pure erasure plan and a schema-coverage guard (#259)"
```

---

### Task 3: The executor

**Files:**
- Create: `services/marketplace-api/internal/customererasure/models.go`
- Create: `services/marketplace-api/internal/customererasure/executor.go`
- Create: `services/marketplace-api/internal/customererasure/executor_integration_test.go`

**Interfaces:**
- Consumes: `erasurePlan`, `Token` from Task 2; `subscription.WithAdvisoryLock`.
- Produces:
  ```go
  type Request struct { ID uuid.UUID; TenantID, StoreID uuid.UUID; CustomerEmail string; Status string; Attempts int; ... }
  func (Request) TableName() string { return "customer_erasure_requests" }

  type Receipt struct { RequestID uuid.UUID; Deleted map[string]int64; Anonymised map[string]int64; RetainedTables []string }

  type Executor struct { DB *gorm.DB; Logger *slog.Logger }
  func NewExecutor(db *gorm.DB, logger *slog.Logger) (*Executor, error)
  func (e *Executor) Process(ctx context.Context, requestID uuid.UUID) (Receipt, error)
  ```
  `NewExecutor` returns an error on a nil `db` rather than deferring the panic — the pattern #318 established.

- [ ] **Step 1: Status transitions**

`Process` must:
1. Claim the row: `UPDATE ... SET status='processing', attempts = attempts + 1 WHERE id = ? AND status IN ('pending','failed') RETURNING *`. **The claim is the concurrency control** — if it matches zero rows another worker has it, and `Process` returns a sentinel `ErrAlreadyClaimed`, not an error state.
2. Run the plan inside `subscription.WithAdvisoryLock(ctx, e.DB, storeID, ...)`, in one transaction, accumulating `RowsAffected` per step.
3. On success: `status='completed'`, `processed_at=now()`, `notes=<receipt summary>`.
4. On failure: `status='failed'`, `notes=<error>`, and return the error. **The status write must not be inside the rolled-back transaction** — that is exactly the #397 defect. Write it after the transaction unwinds, on the pooled handle, using `context.WithoutCancel`.

- [ ] **Step 2: The receipt must record what was RETAINED, not only what was destroyed**

`notes` gets a compact JSON summary: per-table counts for deletes and anonymisations, plus the list of tables deliberately retained with their basis. "We erased it" is a claim that may need evidencing; "we retained the order rows under legal-obligation basis, anonymised" is the half that answers a regulator.

**The receipt must not contain the email, name, phone or address.** Counts and table names only.

- [ ] **Step 3: Integration tests**

Create `executor_integration_test.go` (`//go:build integration`). Seed a customer with: a profile, an address, a wishlist, a review with media, an order with an order_address and a payment_transaction, and an `email_sends` row. Then assert after `Process`:

- **Deleted:** profile, address, wishlist, review_media, email_sends row are gone.
- **Anonymised and PRESENT:** the order still exists, `customer_email` is the token, `customer_name` is redacted, `customer_id` is NULL, but `total`/`currency`/`created_at` are **unchanged** — the financial record survives intact.
- **`order_addresses.country_code` is unchanged** while `name`/`line1`/`city` are redacted.
- **The review still exists** with its rating unchanged and its author anonymised.
- Status is `completed`, `processed_at` is set, `notes` is non-empty and **contains neither the email nor the name**.
- **Another customer in the same store is untouched** — seed a second customer and assert every one of their rows survives. This is the test that catches an unscoped `WHERE`.

- [ ] **Step 4: Idempotency test**

Run `Process` twice on the same request. The second must not error and must not double-count. Assert the end state is identical.

- [ ] **Step 5: MUTATION TESTS**

- Remove `AND store_id = ?` from the `orders` anonymise step → the "other customer untouched" test must FAIL. Restore.
- Change the `orders` step from ANONYMISE to a DELETE → the "financial record survives" test must FAIL. Restore.
- Move the failure-path status write inside the transaction → the failed-status test must FAIL (the status would roll back). Restore.

Report each observation verbatim. If any leaves the suite green, escalate.

- [ ] **Step 6: Commit**

```bash
git add services/marketplace-api/internal/customererasure/
git commit -m "feat(erasure): execute a customer erasure request and record what was erased and retained (#259)"
```

---

### Task 4: Wire it in — inbox action, worker, and the runner

**Files:**
- Create: `services/marketplace-api/internal/handlers/platformadmin/inbox_action_erasure.go`
- Modify: `services/marketplace-api/cmd/marketplace-api/inbox_wiring.go`
- Create: `services/marketplace-api/cmd/erasure-worker/main.go`
- Modify: `Makefile`

**Interfaces:**
- Consumes: `Executor.Process` from Task 3; the executor interface at `inbox_actions.go:60`.

- [ ] **Step 1: The inbox executor**

Implement `Kind() string { return inbox.KindErasureRequest }` and `Execute(...)` handling two actions:
- `process` → `Executor.Process`, returning `InboxActionResult{TenantID, StoreID, Status: "completed"}`.
- `reject` → sets `status='rejected'` with the operator's `notes`.
- Any other `actionID` → an explicit error, mirroring `MigrationFastPathExecutor`'s comment: an action added to the provider's declaration but not here must fail loudly, never silently erase.

Map an already-claimed request to `inbox.ErrItemNotFound` so the handler answers 409 "already actioned", exactly as the migration executor does for a second decision.

- [ ] **Step 2: Register it, replacing the 501**

In `inbox_wiring.go`, register the new executor alongside `NewMigrationFastPathExecutor`. **Update the comment** that currently says erasure "should not get a one-click path before the behaviour beneath it is settled" — it is settled now, and leaving that text next to a registered executor would mislead the next reader.

- [ ] **Step 3: The console-down path**

`cmd/erasure-worker/main.go`: a CLI that processes pending requests, so erasure works when the console is unavailable — the coupling test from tesserix-home#160. Model it on an existing `cmd/*-cron` binary. Support `--request-id` for one and `--all` with a bounded batch.

- [ ] **Step 4: Add the package to `test-int`**

Add `./internal/customererasure/... \` to the `test-int` list. **Precondition: the package must be fully green first.** If anything fails, do not add it.

- [ ] **Step 5: Full verification**

```bash
cd services/marketplace-api
go build ./... && go vet ./... && go vet -tags=integration ./...
TEST_DATABASE_URL='postgres://dev:dev@192.168.1.110:5432/marketplace_db?sslmode=disable' \
  go test -tags=integration -p 1 -count=1 ./internal/customererasure/... ./internal/handlers/platformadmin/... ./internal/tenantpurge/...
go test ./... -count=1
```

Expected: all green. `platformadmin`'s two `inbox_action_idempotency` failures are **gone** as of the dev DB reaching version 112 — if you see them, the database is behind; report it rather than dismissing them.

- [ ] **Step 6: Commit**

```bash
git add services/marketplace-api/internal/handlers/platformadmin/inbox_action_erasure.go \
        services/marketplace-api/cmd/marketplace-api/inbox_wiring.go \
        services/marketplace-api/cmd/erasure-worker/main.go \
        Makefile
git commit -m "feat(erasure): wire the erasure executor into the admin inbox and add a standalone worker (#259)"
```

---

## Self-Review

**Spec coverage.** #259 asks for four things: a worker/endpoint driving the state machine (Tasks 1, 3, 4), the deletion itself spanning the customer's data while preserving what law requires (Task 2's two dispositions, with the retention basis recorded in Task 1's migration), idempotency (Task 3 Steps 1 and 4), and an audit record of what was deleted and retained (Task 3 Step 2). The console contract is Task 4 Steps 1–3.

**Placeholder scan.** Task 1's migrations and Task 2's token are verbatim. Task 2's plan is deliberately specified as a *table-by-table description with the exact WHERE shapes* rather than 40 literal SQL statements, with an explicit instruction to verify every column against the live schema before writing it and to STOP if one is missing. Writing 40 statements here from a survey that was not exhaustive at column level would manufacture false precision — the verification instruction is the safer specification. Task 3's interfaces are given as exact signatures.

**Type consistency.** `Step`, `Disposition`, `Token`, `erasurePlan`, `Request`, `Receipt`, `Executor.Process` are declared once in the Interfaces blocks and used consistently. `erasurePlan` stays unexported; only the coverage guard and the executor (same package) call it.
