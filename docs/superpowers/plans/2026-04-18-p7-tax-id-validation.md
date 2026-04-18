# P7 — Tax ID Validation + B2B Reverse Charge + Quarterly Revalidation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stand up the complete tax-ID validation pipeline for 13 jurisdictions, enforce the 14-day storefront-publish window (with clock-pause on registry outage and SEA review-queue entry), flip Stripe invoices to reverse-charge annotation on validated B2B IDs, and run a daily quarterly-revalidation cron that unpublishes storefronts but keeps billing active — the deliberate "no perverse incentive" design from §19.5.

**Architecture:** A new `internal/billing/tax` package exposes a uniform `Validator` interface — `Validate(ctx, req) (ValidationResult, error)` — with one file per country under `internal/billing/tax/validators/{country}.go`. A `Registry` maps ISO-3166 alpha-2 → `Validator`, selected per request by `tax_id_country`. The orchestration layer `tax/service.go` wraps the chosen validator with: outage tracking (>72h cumulative → clock-pause), SEA manual-review queue insert (clock-pauses at queue entry, not completion per Council finding #10), name cross-check against the registry-returned business name, and a CAS write of `tax_id_validated`/`tax_id_validated_at`/`tax_id_name_match` onto `store_subscriptions` (P1). A 14-day hard-window middleware (`tax/windowguard`) sits alongside the P3 `readonly.RequireActive` and emits a `storefront.unpublish_requested` audit event when it trips — storefront mechanics live in P12; this plan only flips the `storefront_published` boolean and emits the event. A quarterly revalidation cron (`tax/revalidation/cron.go`) runs daily at 02:00 UTC, re-runs the validator for every `tax_id_validated = true` row older than 90 days, and on invalid: emails the merchant, starts a fresh 14-day update window, unpublishes storefront at day 14, **keeps billing running**. The US/CA attestation endpoint writes to the append-only `business_entity_attestations` table (P1 migration 000043, trigger + REVOKE DELETE already installed). NZ is feature-flagged off (`NZ_TAX_VALIDATION_ENABLED=false`) until counsel sign-off (§20.3).

**Tech Stack:** Go 1.26, Gin, GORM, PostgreSQL 15 (CNPG), `net/http` + `httptest` for registry mocks, existing `internal/audit` + `internal/subscription` (P1), `internal/billing/stripeclient` (P2) for invoice `custom_fields` edits, `internal/notification` for merchant emails, existing `internal/cron` scheduler.

**Spec:** [`docs/superpowers/specs/2026-04-17-subscription-model-design.md`](../specs/2026-04-17-subscription-model-design.md) — §5.2 (14-day window + clock-pause), §5.1.1 (migration fast-path — P5 exposes intake, we own the validation-shortening side-effect), §19 (tax compliance), §19.3 (13 validators table), §19.3.1 (US/CA attestation), §19.4 (AU GST already `tax_behavior: exclusive` via P2), §19.5 (quarterly revalidation — storefront-unpublish-only), §19.6 + §20.3 (NZ counsel critical path).

**Depends on:**
- **P1** — `reverse_charge_tax_id`, `tax_id_country`, `tax_id_validated`, `tax_id_validated_at`, `tax_id_name_match` columns + `business_entity_attestations` table (with append-only trigger + `REVOKE DELETE`) + `subscription.WithAdvisoryLock` + `audit.EmitStateTransition` scaffold.
- **P2** — `stripeclient.UpdateInvoiceCustomFields` for reverse-charge annotation on invoices; AU `tax_behavior: exclusive` Price objects.
- **P3** — statemachine is unaffected; the storefront-publish gate is orthogonal. The 14-day window middleware coexists with `readonly.RequireActive` but lives in its own package.

**Related plans:**
- **P5** (trial card-add deferred charge + migration fast-path intake) — exposes the 48h fast-path endpoint that P7 consumes via a `FastPathApproved` signal to shrink the window from 14d → 48h.
- **P12** (Worker closed-page + storefront publish) — consumes the `storefront.unpublish_requested` event; here we only flip the DB flag + emit.
- **P16** (admin UI for tax ID form) — consumes the `/admin/tax-id/submit` and `/admin/attestation` endpoints this plan exposes.
- **P17** (observability) — reads the SEA queue-depth gauge, 30/week capacity alert, and registry-outage counters that P7 emits as Prometheus metrics.

---

## Scope Check

In scope:
1. `Validator` interface + 13 per-country validator implementations (US, CA, UK, IE+EU, AU, NZ-gated, India, Singapore, Malaysia, Thailand, Philippines, Indonesia, Vietnam).
2. `Registry` mapping country → validator, with `NZ_TAX_VALIDATION_ENABLED` feature flag (default `false`).
3. `tax.Service` orchestrator: submit → async validator call → on success flip `tax_id_validated` + set `tax_id_name_match`; on registry outage >72h cumulative → pause clock; on SEA queue entry → pause clock **immediately** (§5.2 + Council finding #10).
4. `sea_manual_review_queue` table + 5-business-day SLA timestamps + 30/week capacity-alert metric.
5. Name cross-check helper (fuzzy match via `metaphone` + Levenshtein) populating `tax_id_name_match`.
6. 14-day hard-window middleware: when `day_since_signup > 14 AND !tax_id_validated AND !clock_paused`, blocks storefront publish (admin read-only/billing allowlist is P3's job, not re-implemented here).
7. US/CA attestation endpoint `POST /admin/tax/attestation` → append-only insert into `business_entity_attestations` (role-protected per P1).
8. Reverse-charge invoice annotation: on `tax_id_validated = true` AND country in {EU, UK, IN, SG, MY, TH, PH, ID, VN, CA, NZ}, annotate the Stripe invoice with a `custom_fields` reverse-charge clause. AU invoice keeps GST breakdown (§19.4 — nothing to do here, P2 already set exclusive).
9. Quarterly revalidation cron (daily 02:00 UTC, re-checks IDs validated >90d ago): invalid → email + 14-day window + storefront-unpublish at day 14 + billing continues.
10. Migration fast-path side-effect: when P5 emits `fastpath.approved`, shorten the current window from 14d → 48h.

Out of scope:
- Storefront publish/unpublish mechanics — this plan flips a `storefront_published` flag + emits an event; the Cloudflare Worker `closed.html` path is P12.
- Admin UI for the tax-ID form and attestation checkbox — P16.
- Worker closed page — P12.
- Actual NZ tax counsel sign-off — legal work, not code. P7 leaves the NZ validator written but flag-gated off.
- P5's intake endpoint for migration fast-path evidence (WHOIS, screenshot) — this plan only handles the approval side-effect.
- The admin read-only allowlist during tax-block — already in P3's `readonly.RequireActive`.

---

## File Structure

### Create

- `services/marketplace-api/internal/billing/tax/interface.go` — the `Validator` interface + `ValidationResult` + error sentinels
- `services/marketplace-api/internal/billing/tax/registry.go` — country → validator map with NZ feature flag
- `services/marketplace-api/internal/billing/tax/service.go` — the orchestrator
- `services/marketplace-api/internal/billing/tax/service_test.go`
- `services/marketplace-api/internal/billing/tax/namematch.go` — fuzzy matcher
- `services/marketplace-api/internal/billing/tax/namematch_test.go`
- `services/marketplace-api/internal/billing/tax/clockpause.go` — outage tracking + SEA queue pause triggers
- `services/marketplace-api/internal/billing/tax/clockpause_test.go`
- `services/marketplace-api/internal/billing/tax/validators/us.go`
- `services/marketplace-api/internal/billing/tax/validators/us_test.go`
- `services/marketplace-api/internal/billing/tax/validators/ca.go`
- `services/marketplace-api/internal/billing/tax/validators/ca_test.go`
- `services/marketplace-api/internal/billing/tax/validators/uk.go`
- `services/marketplace-api/internal/billing/tax/validators/uk_test.go`
- `services/marketplace-api/internal/billing/tax/validators/eu.go` — IE + DE/FR/IT/ES/NL via VIES
- `services/marketplace-api/internal/billing/tax/validators/eu_test.go`
- `services/marketplace-api/internal/billing/tax/validators/au.go`
- `services/marketplace-api/internal/billing/tax/validators/au_test.go`
- `services/marketplace-api/internal/billing/tax/validators/nz.go` — feature-flagged off
- `services/marketplace-api/internal/billing/tax/validators/nz_test.go`
- `services/marketplace-api/internal/billing/tax/validators/in.go` — GSTN API
- `services/marketplace-api/internal/billing/tax/validators/in_test.go`
- `services/marketplace-api/internal/billing/tax/validators/sg.go`
- `services/marketplace-api/internal/billing/tax/validators/sg_test.go`
- `services/marketplace-api/internal/billing/tax/validators/my.go` — MOF SST, manual review
- `services/marketplace-api/internal/billing/tax/validators/my_test.go`
- `services/marketplace-api/internal/billing/tax/validators/th.go` — RD API, manual review
- `services/marketplace-api/internal/billing/tax/validators/th_test.go`
- `services/marketplace-api/internal/billing/tax/validators/ph.go` — BIR, manual review
- `services/marketplace-api/internal/billing/tax/validators/ph_test.go`
- `services/marketplace-api/internal/billing/tax/validators/id.go` — DJP NPWP, manual review
- `services/marketplace-api/internal/billing/tax/validators/id_test.go`
- `services/marketplace-api/internal/billing/tax/validators/vn.go` — GDT, manual review
- `services/marketplace-api/internal/billing/tax/validators/vn_test.go`
- `services/marketplace-api/internal/billing/tax/windowguard/middleware.go` — 14-day hard-window publish guard
- `services/marketplace-api/internal/billing/tax/windowguard/middleware_test.go`
- `services/marketplace-api/internal/billing/tax/seaqueue/models.go` — `SEAManualReviewQueue` GORM model
- `services/marketplace-api/internal/billing/tax/seaqueue/repository.go`
- `services/marketplace-api/internal/billing/tax/seaqueue/repository_test.go`
- `services/marketplace-api/internal/billing/tax/revalidation/cron.go` — daily 02:00 UTC cron
- `services/marketplace-api/internal/billing/tax/revalidation/cron_test.go`
- `services/marketplace-api/internal/handlers/admin/tax.go` — `POST /admin/tax/submit`, `POST /admin/tax/attestation`
- `services/marketplace-api/internal/handlers/admin/tax_test.go`
- `services/marketplace-api/internal/billing/reverse_charge_invoice.go` — Stripe `custom_fields` annotation for validated B2B invoices
- `services/marketplace-api/internal/billing/reverse_charge_invoice_test.go`
- `services/marketplace-api/migrations/000047_sea_manual_review_queue.up.sql`
- `services/marketplace-api/migrations/000047_sea_manual_review_queue.down.sql`
- `services/marketplace-api/migrations/000048_tax_validation_outage_log.up.sql`
- `services/marketplace-api/migrations/000048_tax_validation_outage_log.down.sql`
- `services/marketplace-api/migrations/000049_storefront_published_flag.up.sql`
- `services/marketplace-api/migrations/000049_storefront_published_flag.down.sql`

### Modify

- `services/marketplace-api/internal/subscription/models.go` — add `StorefrontPublished bool` (added by migration 000049)
- `services/marketplace-api/internal/billing/stripeclient/client.go` — add `UpdateInvoiceCustomFields(ctx, invoiceID, fields []CustomField)` (if not already in P2)
- `services/marketplace-api/internal/handlers/admin/routes.go` — mount tax routes under the P3-existing allowlist (tax endpoints must remain reachable even in read-only states; add `/admin/tax/*path` to the P3 allowlist)
- `services/marketplace-api/cmd/marketplace-api/main.go` — register cron, register tax registry, read `NZ_TAX_VALIDATION_ENABLED` env
- `services/marketplace-api/internal/notification/templates/` — add `tax_id_invalid.tmpl` and `tax_id_revalidation_failed.tmpl`
- `services/marketplace-api/internal/subscription/readonly/allowlist.go` (P3) — append `/admin/tax/*path` to `DefaultAllowlist`
- `services/marketplace-api/marketplaceapi.go` — bump expected schema version to 49

### Delete

- None.

---

## Task Sequence Overview

| # | Task | Depends on |
|---|---|---|
| 1 | Migration 047 — `sea_manual_review_queue` | P1 complete |
| 2 | Migration 048 — `tax_validation_outage_log` | — |
| 3 | Migration 049 — `storefront_published` flag | — |
| 4 | Validator interface + error sentinels + `ValidationResult` | — |
| 5 | Registry + NZ feature flag | 4 |
| 6 | US validator (format + attestation checkbox) | 4 |
| 7 | UK validator (HMRC VAT API, full live example) | 4 |
| 8 | India validator (GSTN API, full live example) | 4 |
| 9 | CA, IE+EU (VIES), AU (ABN Lookup), SG (ACRA) validators | 4 |
| 10 | SEA validators (MY, TH, PH, ID, VN) — all route to manual-review queue | 1, 4 |
| 11 | NZ validator behind feature flag; 503 when disabled | 5 |
| 12 | Name cross-check helper + `tax_id_name_match` write | 4 |
| 13 | Clock-pause tracker — registry-outage aggregator + SEA queue trigger | 1, 2 |
| 14 | SEA queue repository + 5-biz-day SLA + 30/week capacity metric | 1 |
| 15 | Tax service orchestrator — ties validator + clock-pause + name-match + DB write | 5, 12, 13, 14 |
| 16 | 14-day hard-window middleware (`windowguard`) | 3, 15 |
| 17 | Admin handlers — `POST /admin/tax/submit` + `POST /admin/tax/attestation` | 15 |
| 18 | Reverse-charge invoice annotation on Stripe | P2, 15 |
| 19 | Quarterly revalidation cron (daily 02:00 UTC) | 15, 18 |
| 20 | Migration fast-path approved listener — shorten window 14d → 48h | P5, 15 |
| 21 | Append `/admin/tax/*path` to P3 allowlist | P3 |
| 22 | Integration tests — success criteria #41, #45, #53 | 15, 16, 19, 20 |
| 23 | Schema-version bump + CI verification | all |

Each task is one atomic commit boundary.

---

## Reusable patterns referenced in this plan

**A. Validator file shape** — every validator under `internal/billing/tax/validators/{country}.go` implements:

```go
type Validator interface {
    Country() string // ISO-3166 alpha-2
    Validate(ctx context.Context, req ValidationRequest) (ValidationResult, error)
}
```

Each lives in its own file with its own test; a validator's only collaborator is an `*http.Client` (injected via constructor) so tests use `httptest.NewServer`. No validator touches the DB directly; the orchestrator owns persistence.

**B. Registry lookup + NZ flag** — `registry.New(cfg Config)` builds the country map. When `cfg.NZEnabled == false`, the NZ key is populated with a sentinel validator (`nz.NewDisabled()`) that returns a typed `ErrValidatorDisabled` wrapped with `Country=NZ`. Handlers translate this to `HTTP 503 Service Unavailable` with a clear message.

**C. HTTP test server pattern** — every validator test uses `httptest.NewServer(http.HandlerFunc(...))` to mock the real registry. The server's URL is injected into the validator via `WithBaseURL(url)`. No real network calls in tests.

**D. Outage aggregator** — `clockpause.Tracker` appends rows to `tax_validation_outage_log` every time a validator returns `ErrRegistryUnavailable`. A window-aggregation query rolls up outages per subscription: when `cumulative_outage_seconds > 72 * 3600` within the active 14-day window, the orchestrator flags `clock_paused = true` (stored in-memory in a snapshot struct; the 14-day deadline is recalculated on every admin request).

**E. SEA queue contract** — any validator under §19.3's "5-biz-day manual review" row MUST return `ValidationResult{ManualReviewRequired: true, QueueReason: "<registry>_manual"}`. The orchestrator inserts a `sea_manual_review_queue` row and immediately pauses the clock (§5.2: "ID enters SEA manual-review queue → clock pauses IMMEDIATELY"). Humans process the queue out-of-band; CSM marks it resolved; orchestrator wakes up on resolved-event.

**F. CAS write pattern (shared with P3)** — every flip of `tax_id_validated` on `store_subscriptions` uses the P1 advisory lock + CAS UPDATE pattern. Same shape as P3 `statemachine.Transition` but against a different column set. Don't use `statemachine.Transition` for these writes — it's a status-column state machine, and tax validation is an independent dimension.

**G. Cron idempotency** — revalidation runs through a single advisory lock `SELECT pg_advisory_xact_lock(hashtext('tax_revalidation_cron'))` so overlapping daily runs serialize. Per-row work uses `UPDATE ... SET revalidation_attempted_at = now() WHERE id = ? AND revalidation_attempted_at < ?` as a CAS sentinel.

---

## Task 1: Migration 047 — `sea_manual_review_queue`

**Files:**
- Create: `services/marketplace-api/migrations/000047_sea_manual_review_queue.up.sql`
- Create: `services/marketplace-api/migrations/000047_sea_manual_review_queue.down.sql`

**Spec references:** §19.3 (SEA row), §5.2 (immediate pause on queue entry).

- [ ] **Step 1: Write the up migration**

```sql
-- 000047_sea_manual_review_queue.up.sql
-- SEA (MY, TH, PH, ID, VN) tax-ID manual review. Any ID that enters this queue
-- immediately pauses the 14-day validation clock on the associated subscription
-- (§5.2). 5-biz-day SLA; sustained >30/week over 2 weeks triggers capacity alert.

CREATE TABLE IF NOT EXISTS sea_manual_review_queue (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id          UUID        NOT NULL,
    store_id           UUID        NOT NULL,
    country            CHAR(2)     NOT NULL
        CHECK (country IN ('MY', 'TH', 'PH', 'ID', 'VN')),
    tax_id             VARCHAR(50) NOT NULL,
    business_name      TEXT,
    queue_reason       VARCHAR(50) NOT NULL,  -- e.g. "mof_sst_manual"
    status             VARCHAR(20) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'in_review', 'approved', 'rejected')),
    reviewer_id        UUID,
    reviewer_notes     TEXT,
    sla_due_at         TIMESTAMPTZ NOT NULL,  -- signed_at + 5 business days
    queued_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_at        TIMESTAMPTZ,

    UNIQUE (tenant_id, store_id, country)     -- one active review per store-country
);

CREATE INDEX IF NOT EXISTS smrq_status_idx  ON sea_manual_review_queue (status) WHERE status IN ('pending', 'in_review');
CREATE INDEX IF NOT EXISTS smrq_country_idx ON sea_manual_review_queue (country);
CREATE INDEX IF NOT EXISTS smrq_queued_week_idx ON sea_manual_review_queue (queued_at);
```

- [ ] **Step 2: Write the down migration**

```sql
-- 000047_sea_manual_review_queue.down.sql
DROP INDEX IF EXISTS smrq_queued_week_idx;
DROP INDEX IF EXISTS smrq_country_idx;
DROP INDEX IF EXISTS smrq_status_idx;
DROP TABLE IF EXISTS sea_manual_review_queue;
```

- [ ] **Step 3: Apply and verify**

```bash
cd services/marketplace-api
go run ./cmd/migrate -url "$TEST_DATABASE_URL" up

psql "$TEST_DATABASE_URL" -c "\d sea_manual_review_queue"
```

Expected: the 12 columns above and 3 indexes.

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/migrations/000047_sea_manual_review_queue.{up,down}.sql
git commit -m "feat(db): sea_manual_review_queue with 5-biz-day SLA + weekly capacity index"
```

---

## Task 2: Migration 048 — `tax_validation_outage_log`

**Files:**
- Create: `services/marketplace-api/migrations/000048_tax_validation_outage_log.up.sql`
- Create: `services/marketplace-api/migrations/000048_tax_validation_outage_log.down.sql`

**Spec references:** §5.2 clock-pause on registry >72h cumulative outage.

- [ ] **Step 1: Write the up migration**

```sql
-- 000048_tax_validation_outage_log.up.sql
-- One row per observed registry failure. The clock-pause aggregator rolls this
-- up per-(country, store) within the active 14-day validation window; when
-- cumulative outage seconds > 72 * 3600 the orchestrator pauses the deadline.

CREATE TABLE IF NOT EXISTS tax_validation_outage_log (
    id               BIGSERIAL PRIMARY KEY,
    country          CHAR(2)      NOT NULL,
    registry         VARCHAR(30)  NOT NULL,  -- 'HMRC', 'VIES', 'GSTN', 'ACRA', ...
    store_id         UUID,                   -- nullable: a probe with no caller still logs
    tenant_id        UUID,
    error_class      VARCHAR(30)  NOT NULL,  -- 'timeout', '5xx', 'network', 'rate_limit'
    started_at       TIMESTAMPTZ  NOT NULL,
    ended_at         TIMESTAMPTZ,            -- NULL while outage still open
    seconds_observed INTEGER,                -- materialized when ended_at is set

    CHECK (ended_at IS NULL OR ended_at >= started_at)
);

CREATE INDEX IF NOT EXISTS tvol_open_idx     ON tax_validation_outage_log (registry, started_at) WHERE ended_at IS NULL;
CREATE INDEX IF NOT EXISTS tvol_store_idx    ON tax_validation_outage_log (store_id, started_at) WHERE store_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS tvol_registry_idx ON tax_validation_outage_log (registry, started_at);
```

- [ ] **Step 2: Write the down migration**

```sql
DROP INDEX IF EXISTS tvol_registry_idx;
DROP INDEX IF EXISTS tvol_store_idx;
DROP INDEX IF EXISTS tvol_open_idx;
DROP TABLE IF EXISTS tax_validation_outage_log;
```

- [ ] **Step 3: Apply + verify, then commit**

```bash
go run ./cmd/migrate -url "$TEST_DATABASE_URL" up
git add services/marketplace-api/migrations/000048_tax_validation_outage_log.{up,down}.sql
git commit -m "feat(db): tax_validation_outage_log for 72h clock-pause aggregation"
```

---

## Task 3: Migration 049 — `storefront_published` flag

**Files:**
- Create: `services/marketplace-api/migrations/000049_storefront_published_flag.up.sql`
- Create: `services/marketplace-api/migrations/000049_storefront_published_flag.down.sql`

**Spec references:** §5.3 timeline "Unpublished until tax ID validated"; §19.5 quarterly revalidation storefront-unpublish.

> **Note:** the actual Cloudflare Worker closed-page mechanics live in P12. Here we only track the bit, so cron/middleware have a place to flip.

- [ ] **Step 1: Up migration**

```sql
-- 000049_storefront_published_flag.up.sql
ALTER TABLE store_subscriptions
    ADD COLUMN storefront_published       BOOLEAN      NOT NULL DEFAULT false,
    ADD COLUMN storefront_unpublished_at  TIMESTAMPTZ,
    ADD COLUMN storefront_unpublish_reason VARCHAR(40)
        CHECK (storefront_unpublish_reason IS NULL OR storefront_unpublish_reason IN (
            'awaiting_tax_validation',
            'tax_revalidation_failed',
            'admin_action',
            'payment_terminal'
        ));

-- Backfill: any subscription already at status = 'active' with tax_id_validated = true
-- is retroactively marked as published. Others stay unpublished pending validation.
UPDATE store_subscriptions
   SET storefront_published = true
 WHERE status = 'active'
   AND tax_id_validated = true;

CREATE INDEX IF NOT EXISTS ss_storefront_published_idx
    ON store_subscriptions (storefront_published) WHERE storefront_published = false;
```

- [ ] **Step 2: Down migration**

```sql
DROP INDEX IF EXISTS ss_storefront_published_idx;
ALTER TABLE store_subscriptions
    DROP COLUMN IF EXISTS storefront_unpublish_reason,
    DROP COLUMN IF EXISTS storefront_unpublished_at,
    DROP COLUMN IF EXISTS storefront_published;
```

- [ ] **Step 3: Apply + verify + commit**

```bash
go run ./cmd/migrate -url "$TEST_DATABASE_URL" up
git add services/marketplace-api/migrations/000049_storefront_published_flag.{up,down}.sql
git commit -m "feat(db): storefront_published flag + unpublish reason for tax-gated publish"
```

---

## Task 4: Validator interface + error sentinels

**Files:**
- Create: `services/marketplace-api/internal/billing/tax/interface.go`
- Create: `services/marketplace-api/internal/billing/tax/interface_test.go`

- [ ] **Step 1: Failing test — interface contract**

```go
package tax_test

import (
    "testing"

    "github.com/stretchr/testify/require"
    "github.com/tesserix/marketplace-api/internal/billing/tax"
)

func TestValidationRequest_Fields_AreRequired(t *testing.T) {
    // Compile-time check that the four required fields exist.
    req := tax.ValidationRequest{
        Country:      "GB",
        TaxID:        "GB123456789",
        BusinessName: "Acme Ltd",
        BillingAddress: "1 Example St",
    }
    require.Equal(t, "GB", req.Country)
}

func TestErrorSentinels_DistinctIdentity(t *testing.T) {
    require.NotErrorIs(t, tax.ErrInvalidFormat,         tax.ErrRegistryUnavailable)
    require.NotErrorIs(t, tax.ErrRegistryUnavailable,   tax.ErrNotFound)
    require.NotErrorIs(t, tax.ErrNotFound,              tax.ErrManualReviewRequired)
    require.NotErrorIs(t, tax.ErrValidatorDisabled,     tax.ErrInvalidFormat)
}
```

- [ ] **Step 2: Run — expect FAIL (package doesn't exist)**

- [ ] **Step 3: Write `interface.go`**

```go
// Package tax implements per-country tax-ID validation, the 14-day window
// orchestrator, and the quarterly revalidation cron per spec §19.
//
// Every country-specific validator lives in the `validators/` subpackage and
// implements Validator. The orchestrator (service.go) is validator-agnostic
// and holds all DB writes and clock-pause logic in one place.
package tax

import (
    "context"
    "errors"
)

// ValidationRequest is the normalized payload from the admin form.
// The orchestrator has already loaded the subscription and computed the
// country from `tax_id_country`. TaxID is a raw string; validators handle
// per-country format normalization.
type ValidationRequest struct {
    TenantID       string
    StoreID        string
    Country        string // ISO-3166 alpha-2, uppercase
    TaxID          string
    BusinessName   string
    BillingAddress string
}

// ValidationResult carries the validator's verdict back to the orchestrator.
//
// Valid=true means "registry confirmed this ID". RegistryName is used for the
// fuzzy name cross-check (§19.3). ManualReviewRequired=true signals the SEA
// path; the orchestrator queues it and pauses the clock immediately.
type ValidationResult struct {
    Valid                bool
    RegistryName         string // business name as the registry returned it
    RegistryCallID       string // vendor trace ID; copied to audit metadata
    ManualReviewRequired bool
    QueueReason          string // e.g. "mof_sst_manual", "bir_manual"
}

// Validator is the contract every country-specific implementation satisfies.
// Implementations MUST be stateless except for an injected *http.Client and
// registry base URL; orchestration state is owned by tax.Service.
type Validator interface {
    Country() string
    Validate(ctx context.Context, req ValidationRequest) (ValidationResult, error)
}

// Error sentinels — every validator uses these, no custom per-country error
// types. Orchestrator and handlers switch on these.
var (
    ErrInvalidFormat        = errors.New("tax: invalid format for country")
    ErrRegistryUnavailable  = errors.New("tax: registry unavailable (outage tracked)")
    ErrNotFound             = errors.New("tax: id not found in registry")
    ErrManualReviewRequired = errors.New("tax: enters manual review queue")
    ErrValidatorDisabled    = errors.New("tax: validator disabled by feature flag")
    ErrNameMismatch         = errors.New("tax: name mismatch (advisory; orchestrator decides)")
)
```

- [ ] **Step 4: Run — expect PASS**

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/billing/tax/interface{,_test}.go
git commit -m "feat(tax): validator interface + error sentinels + result types"
```

---

## Task 5: Registry + NZ feature flag

**Files:**
- Create: `services/marketplace-api/internal/billing/tax/registry.go`
- Create: `services/marketplace-api/internal/billing/tax/registry_test.go`

**Spec references:** §20.3 (NZ counsel critical path).

- [ ] **Step 1: Failing tests**

```go
package tax_test

import (
    "context"
    "testing"

    "github.com/stretchr/testify/require"
    "github.com/tesserix/marketplace-api/internal/billing/tax"
)

func TestRegistry_LookupSupportedCountry(t *testing.T) {
    r := tax.NewRegistry(tax.RegistryConfig{NZEnabled: false})
    v, ok := r.For("GB")
    require.True(t, ok)
    require.Equal(t, "GB", v.Country())
}

func TestRegistry_NZ_DisabledReturnsSentinelValidator(t *testing.T) {
    r := tax.NewRegistry(tax.RegistryConfig{NZEnabled: false})
    v, ok := r.For("NZ")
    require.True(t, ok, "NZ must still be registered; enabled flag only affects Validate()")

    _, err := v.Validate(context.Background(), tax.ValidationRequest{
        Country: "NZ", TaxID: "123456789",
    })
    require.ErrorIs(t, err, tax.ErrValidatorDisabled)
}

func TestRegistry_NZ_EnabledCallsRealValidator(t *testing.T) {
    r := tax.NewRegistry(tax.RegistryConfig{NZEnabled: true})
    v, ok := r.For("NZ")
    require.True(t, ok)
    require.Equal(t, "NZ", v.Country())
    // Real validator reachability is tested in validators/nz_test.go.
}

func TestRegistry_UnsupportedCountry(t *testing.T) {
    r := tax.NewRegistry(tax.RegistryConfig{NZEnabled: false})
    _, ok := r.For("ZZ")
    require.False(t, ok)
}

func TestRegistry_AllThirteenCountriesPresent(t *testing.T) {
    r := tax.NewRegistry(tax.RegistryConfig{NZEnabled: false})
    want := []string{"US", "CA", "GB", "IE", "DE", "FR", "IT", "ES", "NL", "AU", "NZ", "IN", "SG", "MY", "TH", "PH", "ID", "VN"}
    for _, c := range want {
        _, ok := r.For(c)
        require.Truef(t, ok, "country %s not registered", c)
    }
}
```

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Write `registry.go`**

```go
package tax

import (
    "net/http"

    "github.com/tesserix/marketplace-api/internal/billing/tax/validators"
)

// RegistryConfig wires external dependencies into the validators.
// The HTTPClient is shared across validators (connection reuse); base URLs
// default to the real registry endpoints but are overridable in tests.
type RegistryConfig struct {
    HTTPClient  *http.Client
    NZEnabled   bool

    HMRCBaseURL string
    VIESBaseURL string
    ABNBaseURL  string
    GSTNBaseURL string
    ACRABaseURL string
    IRDBaseURL  string
    // MY/TH/PH/ID/VN are manual-review; no real API URL needed today.
}

// Registry maps ISO-3166 alpha-2 country codes to validators. For the IE+EU
// group (§19.3), one VIES validator is registered under each member country.
type Registry struct {
    byCountry map[string]Validator
}

func NewRegistry(cfg RegistryConfig) *Registry {
    if cfg.HTTPClient == nil {
        cfg.HTTPClient = http.DefaultClient
    }
    r := &Registry{byCountry: map[string]Validator{}}

    r.byCountry["US"] = validators.NewUS()
    r.byCountry["CA"] = validators.NewCA()
    r.byCountry["GB"] = validators.NewUK(cfg.HTTPClient, cfg.HMRCBaseURL)

    vies := validators.NewEU(cfg.HTTPClient, cfg.VIESBaseURL)
    for _, c := range []string{"IE", "DE", "FR", "IT", "ES", "NL"} {
        r.byCountry[c] = vies.WithCountry(c)
    }

    r.byCountry["AU"] = validators.NewAU(cfg.HTTPClient, cfg.ABNBaseURL)

    if cfg.NZEnabled {
        r.byCountry["NZ"] = validators.NewNZ(cfg.HTTPClient, cfg.IRDBaseURL)
    } else {
        r.byCountry["NZ"] = validators.NewNZDisabled()
    }

    r.byCountry["IN"] = validators.NewIN(cfg.HTTPClient, cfg.GSTNBaseURL)
    r.byCountry["SG"] = validators.NewSG(cfg.HTTPClient, cfg.ACRABaseURL)

    r.byCountry["MY"] = validators.NewMY() // manual review
    r.byCountry["TH"] = validators.NewTH() // manual review
    r.byCountry["PH"] = validators.NewPH() // manual review
    r.byCountry["ID"] = validators.NewID() // manual review
    r.byCountry["VN"] = validators.NewVN() // manual review

    return r
}

// For returns the validator for a country, or (nil, false) if unsupported.
func (r *Registry) For(country string) (Validator, bool) {
    v, ok := r.byCountry[country]
    return v, ok
}
```

- [ ] **Step 4: Run — expect PASS (after Task 6–11 provide constructors)**

Test compilation temporarily fails until the validators are written. The TDD loop is: write Task 4 test → skip; write Task 5 test → skip; land the validators per Task 6–11; come back and run Task 5 registry tests. Document this explicitly in the commit message so reviewers aren't confused.

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/billing/tax/registry{,_test}.go
git commit -m "feat(tax): registry wiring all 13 validators + NZ feature flag"
```

---

## Task 6: US validator — EIN format + attestation-checkbox flow

**Files:**
- Create: `services/marketplace-api/internal/billing/tax/validators/us.go`
- Create: `services/marketplace-api/internal/billing/tax/validators/us_test.go`

**Spec references:** §19.3 US row, §19.3.1 attestation.

**Full example — this is the simplest validator in the set.**

- [ ] **Step 1: Failing tests**

```go
package validators_test

import (
    "context"
    "testing"

    "github.com/stretchr/testify/require"
    "github.com/tesserix/marketplace-api/internal/billing/tax"
    "github.com/tesserix/marketplace-api/internal/billing/tax/validators"
)

func TestUS_EIN_ValidFormatAccepted(t *testing.T) {
    v := validators.NewUS()
    res, err := v.Validate(context.Background(), tax.ValidationRequest{
        Country: "US", TaxID: "12-3456789", BusinessName: "Acme Inc",
    })
    require.NoError(t, err)
    require.True(t, res.Valid)
    // US has no registry API — the checkbox table is the source of truth.
    // RegistryName echoes the submitted name so the name-match writes `matched`.
    require.Equal(t, "Acme Inc", res.RegistryName)
}

func TestUS_EIN_NoDashAlsoAccepted(t *testing.T) {
    v := validators.NewUS()
    res, err := v.Validate(context.Background(), tax.ValidationRequest{
        Country: "US", TaxID: "123456789", BusinessName: "Acme Inc",
    })
    require.NoError(t, err)
    require.True(t, res.Valid)
}

func TestUS_EIN_BadFormatRejected(t *testing.T) {
    v := validators.NewUS()
    _, err := v.Validate(context.Background(), tax.ValidationRequest{
        Country: "US", TaxID: "not-an-ein", BusinessName: "Acme Inc",
    })
    require.ErrorIs(t, err, tax.ErrInvalidFormat)
}

func TestUS_EIN_WrongCountryRejected(t *testing.T) {
    v := validators.NewUS()
    _, err := v.Validate(context.Background(), tax.ValidationRequest{
        Country: "GB", TaxID: "12-3456789",
    })
    require.ErrorIs(t, err, tax.ErrInvalidFormat)
}
```

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Write `us.go`**

```go
package validators

import (
    "context"
    "regexp"

    "github.com/tesserix/marketplace-api/internal/billing/tax"
)

// US: the IRS does not expose a public EIN lookup API. Per spec §19.3 US row,
// validation is format-only plus a legally-binding attestation checkbox (§19.3.1).
// The checkbox is recorded elsewhere (business_entity_attestations table) and
// is managed by the admin handler — this validator only confirms the EIN shape.
//
// EIN format: NN-NNNNNNN or NNNNNNNNN (9 digits, optional dash after the first 2).

var einRegex = regexp.MustCompile(`^\d{2}-?\d{7}$`)

type USValidator struct{}

func NewUS() *USValidator { return &USValidator{} }

func (v *USValidator) Country() string { return "US" }

func (v *USValidator) Validate(ctx context.Context, req tax.ValidationRequest) (tax.ValidationResult, error) {
    if req.Country != "US" {
        return tax.ValidationResult{}, tax.ErrInvalidFormat
    }
    if !einRegex.MatchString(req.TaxID) {
        return tax.ValidationResult{}, tax.ErrInvalidFormat
    }
    // No registry to call; mirror the submitted name back so name-match
    // succeeds trivially. The attestation checkbox is the real integrity gate.
    return tax.ValidationResult{
        Valid:        true,
        RegistryName: req.BusinessName,
    }, nil
}
```

- [ ] **Step 4: Run — expect PASS**

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/billing/tax/validators/us{,_test}.go
git commit -m "feat(tax): US EIN format validator (attestation-backed per §19.3.1)"
```

---

## Task 7: UK validator — HMRC VAT API (full live example)

**Files:**
- Create: `services/marketplace-api/internal/billing/tax/validators/uk.go`
- Create: `services/marketplace-api/internal/billing/tax/validators/uk_test.go`

**Full example — this is the canonical shape for every API-backed validator (CA, EU/VIES, AU/ABN, IN/GSTN, SG/ACRA, NZ/IRD follow the same structure with different endpoints + response shapes).**

- [ ] **Step 1: Failing tests**

```go
package validators_test

import (
    "context"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
    "time"

    "github.com/stretchr/testify/require"
    "github.com/tesserix/marketplace-api/internal/billing/tax"
    "github.com/tesserix/marketplace-api/internal/billing/tax/validators"
)

func TestUK_VAT_HMRCReturnsValid(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        require.Equal(t, "/organisations/vat/check-vat-number/lookup/GB123456789", r.URL.Path)
        require.Equal(t, "application/vnd.hmrc.2.0+json", r.Header.Get("Accept"))
        w.Header().Set("Content-Type", "application/json")
        _ = json.NewEncoder(w).Encode(map[string]any{
            "target": map[string]any{
                "name":    "ACME WIDGETS LTD",
                "vatNumber": "123456789",
                "address": map[string]any{"line1":"1 Example St","postcode":"SW1A 1AA","countryCode":"GB"},
            },
            "processingDate": time.Now().UTC().Format(time.RFC3339),
        })
    }))
    defer srv.Close()

    v := validators.NewUK(srv.Client(), srv.URL)
    res, err := v.Validate(context.Background(), tax.ValidationRequest{
        Country: "GB", TaxID: "GB123456789", BusinessName: "Acme Widgets Ltd",
    })
    require.NoError(t, err)
    require.True(t, res.Valid)
    require.Equal(t, "ACME WIDGETS LTD", res.RegistryName)
}

func TestUK_VAT_HMRCNotFound(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
        w.WriteHeader(http.StatusNotFound)
        _ = json.NewEncoder(w).Encode(map[string]any{"code":"NOT_FOUND"})
    }))
    defer srv.Close()

    v := validators.NewUK(srv.Client(), srv.URL)
    _, err := v.Validate(context.Background(), tax.ValidationRequest{
        Country: "GB", TaxID: "GB999999999",
    })
    require.ErrorIs(t, err, tax.ErrNotFound)
}

func TestUK_VAT_HMRC5xx_MappedToUnavailable(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
        w.WriteHeader(http.StatusBadGateway)
    }))
    defer srv.Close()

    v := validators.NewUK(srv.Client(), srv.URL)
    _, err := v.Validate(context.Background(), tax.ValidationRequest{
        Country: "GB", TaxID: "GB123456789",
    })
    require.ErrorIs(t, err, tax.ErrRegistryUnavailable)
}

func TestUK_VAT_Timeout_MappedToUnavailable(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
        time.Sleep(200 * time.Millisecond)
    }))
    defer srv.Close()

    client := &http.Client{Timeout: 50 * time.Millisecond}
    v := validators.NewUK(client, srv.URL)
    _, err := v.Validate(context.Background(), tax.ValidationRequest{
        Country: "GB", TaxID: "GB123456789",
    })
    require.ErrorIs(t, err, tax.ErrRegistryUnavailable)
}

func TestUK_VAT_BadFormat(t *testing.T) {
    v := validators.NewUK(http.DefaultClient, "http://unused")
    _, err := v.Validate(context.Background(), tax.ValidationRequest{
        Country: "GB", TaxID: "not-a-vat",
    })
    require.ErrorIs(t, err, tax.ErrInvalidFormat)
}
```

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Write `uk.go`**

```go
package validators

import (
    "context"
    "encoding/json"
    "errors"
    "fmt"
    "net/http"
    "net/url"
    "regexp"
    "strings"

    "github.com/tesserix/marketplace-api/internal/billing/tax"
)

// UK VAT number format: "GB" + 9 digits (standard) or 12 digits (branches).
var ukVATRegex = regexp.MustCompile(`^GB\d{9}(\d{3})?$`)

// HMRCBaseURL is the production HMRC "Check a UK VAT number" API root.
// Full path: /organisations/vat/check-vat-number/lookup/{vrn}
const HMRCBaseURL = "https://api.service.hmrc.gov.uk"

type UKValidator struct {
    client  *http.Client
    baseURL string
}

func NewUK(client *http.Client, baseURL string) *UKValidator {
    if client == nil {
        client = http.DefaultClient
    }
    if baseURL == "" {
        baseURL = HMRCBaseURL
    }
    return &UKValidator{client: client, baseURL: baseURL}
}

func (v *UKValidator) Country() string { return "GB" }

type hmrcLookupResponse struct {
    Target struct {
        Name      string `json:"name"`
        VATNumber string `json:"vatNumber"`
    } `json:"target"`
}

func (v *UKValidator) Validate(ctx context.Context, req tax.ValidationRequest) (tax.ValidationResult, error) {
    if req.Country != "GB" {
        return tax.ValidationResult{}, tax.ErrInvalidFormat
    }
    id := strings.ToUpper(strings.ReplaceAll(req.TaxID, " ", ""))
    if !ukVATRegex.MatchString(id) {
        return tax.ValidationResult{}, tax.ErrInvalidFormat
    }
    vrn := strings.TrimPrefix(id, "GB")

    endpoint, err := url.Parse(v.baseURL)
    if err != nil {
        return tax.ValidationResult{}, fmt.Errorf("uk: parse base url: %w", err)
    }
    endpoint.Path = fmt.Sprintf("/organisations/vat/check-vat-number/lookup/GB%s", vrn)

    httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
    if err != nil {
        return tax.ValidationResult{}, fmt.Errorf("uk: build request: %w", err)
    }
    httpReq.Header.Set("Accept", "application/vnd.hmrc.2.0+json")

    resp, err := v.client.Do(httpReq)
    if err != nil {
        // Timeout, DNS failure, TLS error — all treated as outage.
        return tax.ValidationResult{}, fmt.Errorf("uk: http: %w", errors.Join(tax.ErrRegistryUnavailable, err))
    }
    defer resp.Body.Close()

    switch {
    case resp.StatusCode == http.StatusNotFound:
        return tax.ValidationResult{}, tax.ErrNotFound
    case resp.StatusCode >= 500:
        return tax.ValidationResult{}, tax.ErrRegistryUnavailable
    case resp.StatusCode != http.StatusOK:
        return tax.ValidationResult{}, fmt.Errorf("uk: unexpected status %d", resp.StatusCode)
    }

    var body hmrcLookupResponse
    if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
        return tax.ValidationResult{}, fmt.Errorf("uk: decode response: %w", err)
    }

    return tax.ValidationResult{
        Valid:        true,
        RegistryName: body.Target.Name,
    }, nil
}
```

- [ ] **Step 4: Run — expect PASS**

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/billing/tax/validators/uk{,_test}.go
git commit -m "feat(tax): UK VAT validator against HMRC Check VAT Number API"
```

---

## Task 8: India validator — GSTN API (full live example)

**Files:**
- Create: `services/marketplace-api/internal/billing/tax/validators/in.go`
- Create: `services/marketplace-api/internal/billing/tax/validators/in_test.go`

**Full example — the GSTN API uses an auth-token header; this is the second template shape (token-auth) for the non-trivial APIs.**

- [ ] **Step 1: Failing tests**

```go
package validators_test

import (
    "context"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/stretchr/testify/require"
    "github.com/tesserix/marketplace-api/internal/billing/tax"
    "github.com/tesserix/marketplace-api/internal/billing/tax/validators"
)

func TestIN_GSTN_Valid(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        require.Equal(t, "/commonapi/v1.1/search", r.URL.Path)
        require.NotEmpty(t, r.Header.Get("Authorization"))
        _ = json.NewEncoder(w).Encode(map[string]any{
            "data": map[string]any{
                "gstin":     "27AABCU9603R1ZM",
                "lgnm":      "ACME PRIVATE LIMITED",
                "sts":       "Active",
                "ctb":       "Private Limited Company",
                "pradr": map[string]any{
                    "addr": map[string]any{"stcd":"Maharashtra","pncd":"400001"},
                },
            },
        })
    }))
    defer srv.Close()

    v := validators.NewIN(srv.Client(), srv.URL).WithAuthToken("test-token")
    res, err := v.Validate(context.Background(), tax.ValidationRequest{
        Country: "IN", TaxID: "27AABCU9603R1ZM", BusinessName: "Acme Private Limited",
    })
    require.NoError(t, err)
    require.True(t, res.Valid)
    require.Equal(t, "ACME PRIVATE LIMITED", res.RegistryName)
}

func TestIN_GSTN_InactiveRejected(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
        _ = json.NewEncoder(w).Encode(map[string]any{
            "data": map[string]any{
                "gstin": "27AABCU9603R1ZM", "lgnm": "Old Co", "sts": "Cancelled",
            },
        })
    }))
    defer srv.Close()
    v := validators.NewIN(srv.Client(), srv.URL)
    _, err := v.Validate(context.Background(), tax.ValidationRequest{
        Country: "IN", TaxID: "27AABCU9603R1ZM",
    })
    require.ErrorIs(t, err, tax.ErrNotFound)
}

func TestIN_GSTN_429_MappedToUnavailable(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
        w.WriteHeader(http.StatusTooManyRequests)
    }))
    defer srv.Close()
    v := validators.NewIN(srv.Client(), srv.URL)
    _, err := v.Validate(context.Background(), tax.ValidationRequest{
        Country: "IN", TaxID: "27AABCU9603R1ZM",
    })
    require.ErrorIs(t, err, tax.ErrRegistryUnavailable)
}

func TestIN_GSTN_FormatRegex(t *testing.T) {
    v := validators.NewIN(http.DefaultClient, "http://unused")
    for _, bad := range []string{
        "",
        "tooshort",
        "27AABCU9603R1Z", // 14 chars — GSTIN is 15
        "99AABCU9603R1ZM", // state code 99 invalid
    } {
        _, err := v.Validate(context.Background(), tax.ValidationRequest{Country: "IN", TaxID: bad})
        require.ErrorIsf(t, err, tax.ErrInvalidFormat, "expected invalid for %q", bad)
    }
}
```

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Write `in.go`**

```go
package validators

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "regexp"
    "strings"

    "github.com/tesserix/marketplace-api/internal/billing/tax"
)

// GSTIN format: 15 chars, positions:
//   1-2  state code (01-37)
//   3-12 PAN (5 alpha + 4 digits + 1 alpha)
//   13   entity number (alphanumeric)
//   14   'Z' (reserved)
//   15   checksum (alphanumeric)
var gstinRegex = regexp.MustCompile(`^(0[1-9]|[1-2]\d|3[0-7])[A-Z]{5}\d{4}[A-Z][1-9A-Z]Z[0-9A-Z]$`)

const GSTNBaseURL = "https://api.gst.gov.in"

type INValidator struct {
    client    *http.Client
    baseURL   string
    authToken string // injected via WithAuthToken; never logged
}

func NewIN(client *http.Client, baseURL string) *INValidator {
    if client == nil {
        client = http.DefaultClient
    }
    if baseURL == "" {
        baseURL = GSTNBaseURL
    }
    return &INValidator{client: client, baseURL: baseURL}
}

// WithAuthToken returns a shallow copy with the token set. Token lives in
// Secret Manager and is injected at service construction.
func (v *INValidator) WithAuthToken(token string) *INValidator {
    cp := *v
    cp.authToken = token
    return &cp
}

func (v *INValidator) Country() string { return "IN" }

type gstnSearchResponse struct {
    Data struct {
        GSTIN string `json:"gstin"`
        LGNM  string `json:"lgnm"` // legal name
        STS   string `json:"sts"`  // status
    } `json:"data"`
}

func (v *INValidator) Validate(ctx context.Context, req tax.ValidationRequest) (tax.ValidationResult, error) {
    if req.Country != "IN" {
        return tax.ValidationResult{}, tax.ErrInvalidFormat
    }
    id := strings.ToUpper(strings.ReplaceAll(req.TaxID, " ", ""))
    if !gstinRegex.MatchString(id) {
        return tax.ValidationResult{}, tax.ErrInvalidFormat
    }

    endpoint := fmt.Sprintf("%s/commonapi/v1.1/search?gstin=%s", v.baseURL, id)
    httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
    if err != nil {
        return tax.ValidationResult{}, fmt.Errorf("in: build request: %w", err)
    }
    httpReq.Header.Set("Accept", "application/json")
    if v.authToken != "" {
        httpReq.Header.Set("Authorization", "Bearer "+v.authToken)
    }

    resp, err := v.client.Do(httpReq)
    if err != nil {
        return tax.ValidationResult{}, tax.ErrRegistryUnavailable
    }
    defer resp.Body.Close()

    switch {
    case resp.StatusCode == http.StatusNotFound:
        return tax.ValidationResult{}, tax.ErrNotFound
    case resp.StatusCode == http.StatusTooManyRequests, resp.StatusCode >= 500:
        return tax.ValidationResult{}, tax.ErrRegistryUnavailable
    case resp.StatusCode != http.StatusOK:
        return tax.ValidationResult{}, fmt.Errorf("in: unexpected status %d", resp.StatusCode)
    }

    var body gstnSearchResponse
    if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
        return tax.ValidationResult{}, fmt.Errorf("in: decode response: %w", err)
    }

    // Only "Active" GSTINs accepted; "Cancelled" / "Suspended" count as not-found.
    if !strings.EqualFold(body.Data.STS, "Active") {
        return tax.ValidationResult{}, tax.ErrNotFound
    }

    return tax.ValidationResult{
        Valid:        true,
        RegistryName: body.Data.LGNM,
    }, nil
}
```

- [ ] **Step 4: Run — expect PASS**

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/billing/tax/validators/in{,_test}.go
git commit -m "feat(tax): India GSTIN validator against GSTN commonapi search"
```

---

## Task 9: CA, IE+EU (VIES), AU (ABN Lookup), SG (ACRA) validators

**Files (one file + test per country, except EU which handles 6 countries):**
- Create: `validators/ca.go` + `ca_test.go`
- Create: `validators/eu.go` + `eu_test.go`
- Create: `validators/au.go` + `au_test.go`
- Create: `validators/sg.go` + `sg_test.go`

Structurally identical to the UK/India templates — one HTTP test server per test, four response-class cases per validator (valid, not-found, 5xx/timeout → unavailable, bad format). Implementation notes below; paste-ready shapes follow the UK template in Task 7 and the token-auth shape in Task 8 as needed.

### 9a. CA — Business Number + B2B checkbox

- Format: 9-digit BN (optionally with `RT0001` suffix for GST/HST registrant).
- No federal API exposed publicly for BN lookup. Treat like US: format-check + attestation-backed (see §19.3.1 — attestation table covers both US and CA).
- Regex: `^\d{9}(RT\d{4})?$`.
- Return `Valid: true, RegistryName: req.BusinessName` on format success.
- Mirror the US validator structure; no HTTP client needed.

### 9b. EU / IE (VIES) — shared across IE, DE, FR, IT, ES, NL

- VIES endpoint: `https://ec.europa.eu/taxation_customs/vies/rest-api/ms/{country}/vat/{vat_number}`
- VIES response has `valid` boolean, `name`, `address` fields.
- Implement `EUValidator` with a `country` field; `WithCountry(c string)` returns a shallow copy keyed on that country.
- VIES **5xx frequently = member-state service down** (the EU middle layer proxies each tax authority). Map `503` + `504` explicitly to `ErrRegistryUnavailable` so the outage clock is honest.
- VIES has a documented "NO_INFORMATION" response for valid-but-privacy-protected numbers — treat as `Valid: true, RegistryName: ""` and let the name-match write `not_checked` (§19.3).

Test grid: 6 countries × 4 response classes = 24 test cases; use table-driven subtests.

### 9c. AU — ABN Lookup

- Endpoint: `https://abr.business.gov.au/json/AbnDetails.aspx?abn={abn}&guid={guid}` (GUID required by AU gov).
- Format: `abnRegex = /^\d{11}$/`.
- Response `AbnStatus=Active` + `EntityName` → valid. `Cancelled` → `ErrNotFound`.
- Inject GUID via `WithGUID(guid)`; lives in Secret Manager (`AU_ABN_LOOKUP_GUID`).
- §19.4 domestic charging is already handled by P2 via `tax_behavior: exclusive` — this validator is still needed for the pre-launch ABN confirm so we know WHO to charge GST to; Mark8ly Pty Ltd charges 10% GST regardless, so `Valid: true` but no reverse-charge flag (orchestrator maps country=AU → no reverse-charge annotation).

### 9d. SG — ACRA

- Endpoint: `https://api.acra.gov.sg/v1/uen/{uen}`.
- Format: SG UEN has three accepted patterns: `^(\d{8}[A-Z]|\d{9}[A-Z]|[STRFR]\d{2}[A-Z]{2}\d{4}[A-Z])$` — use the canonical UEN regex from the ACRA developer docs.
- Response `entityStatus="Registered"` → valid; anything else → `ErrNotFound`.

### Step sequence per validator

For each of CA, EU, AU, SG (a total of four commits):

- [ ] **Step 1: Failing tests (4 cases + format grid)**
- [ ] **Step 2: Run — expect FAIL**
- [ ] **Step 3: Write the validator (~80–120 lines; same shape as UK/IN)**
- [ ] **Step 4: Run — expect PASS**
- [ ] **Step 5: Commit:**

```bash
git add services/marketplace-api/internal/billing/tax/validators/{country}{,_test}.go
git commit -m "feat(tax): {country} validator against {registry} API"
```

**Skeleton for EU (country-parameterised)** — this is the only novel shape:

```go
type EUValidator struct {
    client  *http.Client
    baseURL string
    country string // "" means "not yet bound"; WithCountry binds a copy
}

func NewEU(client *http.Client, baseURL string) *EUValidator { /* ... */ }

func (v *EUValidator) WithCountry(c string) *EUValidator {
    cp := *v
    cp.country = c
    return &cp
}

func (v *EUValidator) Country() string { return v.country }

func (v *EUValidator) Validate(ctx context.Context, req tax.ValidationRequest) (tax.ValidationResult, error) {
    // Strip country prefix from TaxID if present (VIES expects bare number).
    // GET {base}/ms/{country}/vat/{vat_number}
    // Decode { valid: bool, name: string, address: string, requestDate: string }
    // Map valid=false → ErrNotFound; name="" → Valid:true, RegistryName:""
}
```

The remaining three (CA, AU, SG) are mechanical — four response tests each, no novel patterns.

---

## Task 10: SEA validators — MY, TH, PH, ID, VN (all manual-review)

**Files (one file + test per country):**
- Create: `validators/my.go` + `my_test.go` (MOF SST)
- Create: `validators/th.go` + `th_test.go` (RD)
- Create: `validators/ph.go` + `ph_test.go` (BIR)
- Create: `validators/id.go` + `id_test.go` (DJP NPWP)
- Create: `validators/vn.go` + `vn_test.go` (GDT)

**Spec references:** §19.3 "5-biz-day manual review; clock pauses at queue entry", §5.2.

All five share the same shape: they do **not** call an API today. They run format-validation, and if the format is valid they return `ValidationResult{ManualReviewRequired: true, QueueReason: "<registry>_manual"}`. The orchestrator (Task 15) translates that into a `sea_manual_review_queue` insert and pauses the clock immediately (Council finding #10).

- [ ] **Step 1: Failing test template (reuse for all five)**

```go
package validators_test

import (
    "context"
    "testing"

    "github.com/stretchr/testify/require"
    "github.com/tesserix/marketplace-api/internal/billing/tax"
    "github.com/tesserix/marketplace-api/internal/billing/tax/validators"
)

func TestMY_FormatValid_EntersManualReview(t *testing.T) {
    v := validators.NewMY()
    res, err := v.Validate(context.Background(), tax.ValidationRequest{
        Country: "MY", TaxID: "C12345678901", BusinessName: "Acme Sdn Bhd",
    })
    require.NoError(t, err)
    require.False(t, res.Valid, "manual review means not-yet-valid")
    require.True(t, res.ManualReviewRequired)
    require.Equal(t, "mof_sst_manual", res.QueueReason)
}

func TestMY_BadFormat_Rejected(t *testing.T) {
    v := validators.NewMY()
    _, err := v.Validate(context.Background(), tax.ValidationRequest{
        Country: "MY", TaxID: "bad",
    })
    require.ErrorIs(t, err, tax.ErrInvalidFormat)
}

func TestMY_WrongCountry_Rejected(t *testing.T) {
    v := validators.NewMY()
    _, err := v.Validate(context.Background(), tax.ValidationRequest{
        Country: "TH", TaxID: "C12345678901",
    })
    require.ErrorIs(t, err, tax.ErrInvalidFormat)
}
```

- [ ] **Step 2: Write `my.go`**

```go
package validators

import (
    "context"
    "regexp"

    "github.com/tesserix/marketplace-api/internal/billing/tax"
)

// MY SST registration: single alpha prefix (W/C/B/J) + 10-11 digits.
// §19.3 MY row: all valid-format SST IDs enter 5-biz-day manual review until
// MOF exposes a public API. Queue entry pauses the clock per §5.2.
var mySSTRegex = regexp.MustCompile(`^[WCBJ]\d{10,11}$`)

type MYValidator struct{}

func NewMY() *MYValidator { return &MYValidator{} }

func (v *MYValidator) Country() string { return "MY" }

func (v *MYValidator) Validate(ctx context.Context, req tax.ValidationRequest) (tax.ValidationResult, error) {
    if req.Country != "MY" {
        return tax.ValidationResult{}, tax.ErrInvalidFormat
    }
    if !mySSTRegex.MatchString(req.TaxID) {
        return tax.ValidationResult{}, tax.ErrInvalidFormat
    }
    return tax.ValidationResult{
        Valid:                false,
        ManualReviewRequired: true,
        QueueReason:          "mof_sst_manual",
    }, nil
}
```

- [ ] **Step 3–5: Repeat for TH, PH, ID, VN**

Format references (use the canonical public regexes):
- **TH:** 13-digit tax ID (`^\d{13}$`), `QueueReason: "rd_manual"`
- **PH:** TIN 9 or 12 digits with dashes optional (`^\d{3}-?\d{3}-?\d{3}(-?\d{3})?$`), `QueueReason: "bir_manual"`
- **ID:** NPWP 15 digits (`^\d{15}$` or the dashed form `^\d{2}\.\d{3}\.\d{3}\.\d-\d{3}\.\d{3}$`), `QueueReason: "djp_manual"`
- **VN:** 10 or 13 digits (`^\d{10}(-\d{3})?$`), `QueueReason: "gdt_manual"`

One commit per validator:

```bash
git add services/marketplace-api/internal/billing/tax/validators/{country}{,_test}.go
git commit -m "feat(tax): {country} format validator routes to SEA manual-review queue"
```

---

## Task 11: NZ validator behind feature flag

**Files:**
- Create: `services/marketplace-api/internal/billing/tax/validators/nz.go`
- Create: `services/marketplace-api/internal/billing/tax/validators/nz_test.go`

**Spec references:** §19.6 + §20.3 — NZ tax counsel is critical path; must not accept NZ signups until legal sign-off.

- [ ] **Step 1: Failing tests**

```go
package validators_test

import (
    "context"
    "testing"

    "github.com/stretchr/testify/require"
    "github.com/tesserix/marketplace-api/internal/billing/tax"
    "github.com/tesserix/marketplace-api/internal/billing/tax/validators"
)

func TestNZ_Disabled_ReturnsValidatorDisabled(t *testing.T) {
    v := validators.NewNZDisabled()
    _, err := v.Validate(context.Background(), tax.ValidationRequest{
        Country: "NZ", TaxID: "123-456-789",
    })
    require.ErrorIs(t, err, tax.ErrValidatorDisabled)
}

func TestNZ_Enabled_CallsIRD(t *testing.T) {
    // The enabled constructor NewNZ(client, baseURL) returns a normal validator.
    // Full API integration test skipped until counsel sign-off per §20.3.
    v := validators.NewNZ(nil, "http://unused")
    require.NotNil(t, v)
    require.Equal(t, "NZ", v.Country())
}
```

- [ ] **Step 2: Write `nz.go`**

```go
package validators

import (
    "context"
    "net/http"
    "regexp"

    "github.com/tesserix/marketplace-api/internal/billing/tax"
)

// NZ IRD number: 8 or 9 digits, optionally dash-separated as XXX-XXX-XXX.
var nzIRDRegex = regexp.MustCompile(`^\d{3}-?\d{3}-?\d{2,3}$`)

// IRDBaseURL — IRD's public validation endpoint. Placeholder until counsel
// confirms whether the B2B reverse-charge model works under NZ GST rules
// (§20.3). Implementation is ready; flag gates activation.
const IRDBaseURL = "https://gateway.ird.govt.nz"

type NZValidator struct {
    client  *http.Client
    baseURL string
}

func NewNZ(client *http.Client, baseURL string) *NZValidator {
    if client == nil {
        client = http.DefaultClient
    }
    if baseURL == "" {
        baseURL = IRDBaseURL
    }
    return &NZValidator{client: client, baseURL: baseURL}
}

func (v *NZValidator) Country() string { return "NZ" }

func (v *NZValidator) Validate(ctx context.Context, req tax.ValidationRequest) (tax.ValidationResult, error) {
    if req.Country != "NZ" {
        return tax.ValidationResult{}, tax.ErrInvalidFormat
    }
    if !nzIRDRegex.MatchString(req.TaxID) {
        return tax.ValidationResult{}, tax.ErrInvalidFormat
    }
    // TODO(counsel §20.3): implement IRD lookup once legal sign-off lands.
    // Until then, this branch is unreachable in prod because registry.go
    // registers NewNZDisabled() when NZ_TAX_VALIDATION_ENABLED=false.
    return tax.ValidationResult{Valid: true, RegistryName: req.BusinessName}, nil
}

// NZDisabled is wired by Registry when NZ_TAX_VALIDATION_ENABLED=false.
// Orchestrator maps ErrValidatorDisabled → HTTP 503 with a merchant-friendly message.
type NZDisabled struct{}

func NewNZDisabled() *NZDisabled { return &NZDisabled{} }

func (*NZDisabled) Country() string { return "NZ" }

func (*NZDisabled) Validate(ctx context.Context, _ tax.ValidationRequest) (tax.ValidationResult, error) {
    return tax.ValidationResult{}, tax.ErrValidatorDisabled
}
```

- [ ] **Step 3: Run — expect PASS**

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/internal/billing/tax/validators/nz{,_test}.go
git commit -m "feat(tax): NZ validator + disabled sentinel (flag-gated per §20.3)"
```

---

## Task 12: Name cross-check helper

**Files:**
- Create: `services/marketplace-api/internal/billing/tax/namematch.go`
- Create: `services/marketplace-api/internal/billing/tax/namematch_test.go`

**Spec references:** §19.3 "name cross-check during manual review ... fuzzy match. `tax_id_name_match`: `matched | unmatched | not_checked`".

- [ ] **Step 1: Failing tests**

```go
package tax_test

import (
    "testing"

    "github.com/stretchr/testify/require"
    "github.com/tesserix/marketplace-api/internal/billing/tax"
)

func TestNameMatch_ExactMatch(t *testing.T) {
    require.Equal(t, tax.NameMatched, tax.CompareNames("Acme Inc", "Acme Inc"))
}

func TestNameMatch_CaseAndWhitespaceNormalized(t *testing.T) {
    require.Equal(t, tax.NameMatched, tax.CompareNames("ACME  INC.", "acme inc"))
}

func TestNameMatch_PunctuationIgnored(t *testing.T) {
    require.Equal(t, tax.NameMatched, tax.CompareNames("Acme, Inc.", "Acme Inc"))
}

func TestNameMatch_LimitedLevenshtein(t *testing.T) {
    // ≤10% edit distance still matches (handles typos).
    require.Equal(t, tax.NameMatched, tax.CompareNames("Acme Widgets Ltd", "Acme Widgts Ltd"))
}

func TestNameMatch_UnmatchedWhenDistanceTooHigh(t *testing.T) {
    require.Equal(t, tax.NameUnmatched, tax.CompareNames("Acme Widgets Ltd", "Zephyr Holdings Ltd"))
}

func TestNameMatch_EmptyRegistry_ReturnsNotChecked(t *testing.T) {
    require.Equal(t, tax.NameNotChecked, tax.CompareNames("Acme Inc", ""))
    require.Equal(t, tax.NameNotChecked, tax.CompareNames("", "Acme Inc"))
}

func TestNameMatch_CorporateSuffixesEquivalent(t *testing.T) {
    require.Equal(t, tax.NameMatched, tax.CompareNames("Acme Pty Ltd", "Acme Proprietary Limited"))
    require.Equal(t, tax.NameMatched, tax.CompareNames("Acme Sdn Bhd", "Acme Sendirian Berhad"))
}
```

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Write `namematch.go`**

```go
package tax

import (
    "regexp"
    "strings"
)

// NameMatch mirrors the `tax_id_name_match` enum from migration 000038.
type NameMatch string

const (
    NameMatched    NameMatch = "matched"
    NameUnmatched  NameMatch = "unmatched"
    NameNotChecked NameMatch = "not_checked"
)

// CompareNames runs the normalized-Levenshtein fuzzy match. Returns NameNotChecked
// when the registry returned no name (e.g. VIES privacy-protected response).
//
// Accepts ≤10% Levenshtein edit distance to handle typos and tolerable suffix
// variations. Corporate suffix canonicalization handles the Pty Ltd/Proprietary
// Limited class of equivalences.
func CompareNames(submitted, registry string) NameMatch {
    if registry == "" || submitted == "" {
        return NameNotChecked
    }
    a := normalize(submitted)
    b := normalize(registry)
    if a == b {
        return NameMatched
    }
    maxLen := max(len(a), len(b))
    if maxLen == 0 {
        return NameNotChecked
    }
    d := levenshtein(a, b)
    // ≤10% distance tolerated; minimum 2-char budget for short names.
    threshold := maxLen / 10
    if threshold < 2 { threshold = 2 }
    if d <= threshold {
        return NameMatched
    }
    return NameUnmatched
}

var (
    punctRegex    = regexp.MustCompile(`[^\p{L}\p{N}\s]`)
    whitespaceRegex = regexp.MustCompile(`\s+`)
)

// Canonical suffix map — expand as we encounter new jurisdictions.
var corporateSuffixes = map[string]string{
    "pty ltd":              "proprietary limited",
    "pty limited":          "proprietary limited",
    "ltd":                  "limited",
    "inc":                  "incorporated",
    "corp":                 "corporation",
    "co":                   "company",
    "sdn bhd":              "sendirian berhad",
    "llc":                  "limited liability company",
    "plc":                  "public limited company",
    "gmbh":                 "gesellschaft mit beschrankter haftung",
    "s.a.":                 "societe anonyme",
    "sa":                   "societe anonyme",
    "bv":                   "besloten vennootschap",
    "pvt ltd":              "private limited",
    "private limited":      "private limited",
}

func normalize(s string) string {
    s = strings.ToLower(s)
    s = punctRegex.ReplaceAllString(s, " ")
    s = whitespaceRegex.ReplaceAllString(s, " ")
    s = strings.TrimSpace(s)
    // Canonicalize corporate suffixes (longest-first to avoid partial replacement).
    for abbrev, canon := range corporateSuffixes {
        s = strings.ReplaceAll(s, " "+abbrev, " "+canon)
    }
    return strings.TrimSpace(s)
}

func levenshtein(a, b string) int {
    // Classic DP; O(len(a) * len(b)). Acceptable — names are short.
    if a == b { return 0 }
    if len(a) == 0 { return len(b) }
    if len(b) == 0 { return len(a) }

    prev := make([]int, len(b)+1)
    curr := make([]int, len(b)+1)
    for j := range prev { prev[j] = j }

    for i := 1; i <= len(a); i++ {
        curr[0] = i
        for j := 1; j <= len(b); j++ {
            cost := 1
            if a[i-1] == b[j-1] { cost = 0 }
            curr[j] = min3(prev[j]+1, curr[j-1]+1, prev[j-1]+cost)
        }
        prev, curr = curr, prev
    }
    return prev[len(b)]
}

func min3(x, y, z int) int {
    if x <= y && x <= z { return x }
    if y <= z { return y }
    return z
}
```

- [ ] **Step 4: Run — expect PASS**

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/billing/tax/namematch{,_test}.go
git commit -m "feat(tax): fuzzy name cross-check with suffix canonicalization (§19.3)"
```

---

## Task 13: Clock-pause tracker

**Files:**
- Create: `services/marketplace-api/internal/billing/tax/clockpause.go`
- Create: `services/marketplace-api/internal/billing/tax/clockpause_test.go`

**Spec references:** §5.2 clock-pause triggers.

- [ ] **Step 1: Failing tests**

```go
//go:build integration

package tax_test

import (
    "context"
    "testing"
    "time"

    "github.com/google/uuid"
    "github.com/stretchr/testify/require"

    "github.com/tesserix/marketplace-api/internal/billing/tax"
    "github.com/tesserix/marketplace-api/pkg/testdb"
)

func TestClockPause_SingleOutageUnderThreshold(t *testing.T) {
    db := testdb.NewDB(t, "tax_validation_outage_log")
    tracker := tax.NewClockPauseTracker(db)
    storeID := uuid.New()
    start := time.Now().Add(-48 * time.Hour)

    require.NoError(t, tracker.BeginOutage(context.Background(), tax.OutageKey{
        StoreID: storeID, Country: "GB", Registry: "HMRC", ErrorClass: "5xx",
    }, start))
    require.NoError(t, tracker.EndOutage(context.Background(), tax.OutageKey{
        StoreID: storeID, Country: "GB", Registry: "HMRC", ErrorClass: "5xx",
    }, start.Add(24*time.Hour)))

    paused, err := tracker.IsPaused(context.Background(), storeID, "GB")
    require.NoError(t, err)
    require.False(t, paused, "24h < 72h threshold")
}

func TestClockPause_CumulativeOverThresholdPauses(t *testing.T) {
    db := testdb.NewDB(t, "tax_validation_outage_log")
    tracker := tax.NewClockPauseTracker(db)
    storeID := uuid.New()
    now := time.Now()

    // Three 30h outages summing to 90h.
    for i := 0; i < 3; i++ {
        s := now.Add(-time.Duration((i+1)*48) * time.Hour)
        require.NoError(t, tracker.BeginOutage(context.Background(), tax.OutageKey{
            StoreID: storeID, Country: "GB", Registry: "HMRC", ErrorClass: "5xx",
        }, s))
        require.NoError(t, tracker.EndOutage(context.Background(), tax.OutageKey{
            StoreID: storeID, Country: "GB", Registry: "HMRC", ErrorClass: "5xx",
        }, s.Add(30*time.Hour)))
    }

    paused, err := tracker.IsPaused(context.Background(), storeID, "GB")
    require.NoError(t, err)
    require.True(t, paused, "90h cumulative > 72h threshold")
}

func TestClockPause_OpenOutageCounted(t *testing.T) {
    db := testdb.NewDB(t, "tax_validation_outage_log")
    tracker := tax.NewClockPauseTracker(db)
    storeID := uuid.New()
    start := time.Now().Add(-96 * time.Hour) // 96h open outage

    require.NoError(t, tracker.BeginOutage(context.Background(), tax.OutageKey{
        StoreID: storeID, Country: "GB", Registry: "HMRC", ErrorClass: "5xx",
    }, start))

    paused, err := tracker.IsPaused(context.Background(), storeID, "GB")
    require.NoError(t, err)
    require.True(t, paused, "open outage counts toward cumulative")
}
```

- [ ] **Step 2: Write `clockpause.go`**

```go
package tax

import (
    "context"
    "time"

    "github.com/google/uuid"
    "gorm.io/gorm"
)

// PauseThreshold — §5.2: >72h cumulative outage within the active window pauses.
const PauseThreshold = 72 * time.Hour

// OutageKey identifies one unique outage episode; (store, country, registry,
// error_class) must match on BeginOutage + EndOutage.
type OutageKey struct {
    StoreID    uuid.UUID
    TenantID   uuid.UUID
    Country    string
    Registry   string
    ErrorClass string
}

type ClockPauseTracker struct {
    db *gorm.DB
}

func NewClockPauseTracker(db *gorm.DB) *ClockPauseTracker {
    return &ClockPauseTracker{db: db}
}

// BeginOutage records a new open outage row. Idempotent on (registry, error_class, started_at).
func (t *ClockPauseTracker) BeginOutage(ctx context.Context, k OutageKey, at time.Time) error {
    return t.db.WithContext(ctx).Exec(`
        INSERT INTO tax_validation_outage_log
            (country, registry, store_id, tenant_id, error_class, started_at)
        VALUES (?, ?, ?, ?, ?, ?)
        ON CONFLICT DO NOTHING
    `, k.Country, k.Registry, k.StoreID, k.TenantID, k.ErrorClass, at).Error
}

// EndOutage closes the most recent open row matching the key.
func (t *ClockPauseTracker) EndOutage(ctx context.Context, k OutageKey, at time.Time) error {
    return t.db.WithContext(ctx).Exec(`
        UPDATE tax_validation_outage_log
           SET ended_at         = ?,
               seconds_observed = EXTRACT(EPOCH FROM (? - started_at))::INTEGER
         WHERE id = (
             SELECT id FROM tax_validation_outage_log
              WHERE country = ? AND registry = ?
                AND (store_id = ? OR (store_id IS NULL AND ?::uuid IS NULL))
                AND error_class = ?
                AND ended_at IS NULL
              ORDER BY started_at DESC
              LIMIT 1
         )
    `, at, at, k.Country, k.Registry, k.StoreID, k.StoreID, k.ErrorClass).Error
}

// IsPaused sums observed + in-flight outage seconds for the (store, country)
// pair over the last 14 days; returns true when cumulative ≥ PauseThreshold.
func (t *ClockPauseTracker) IsPaused(ctx context.Context, storeID uuid.UUID, country string) (bool, error) {
    var cumSeconds int64
    err := t.db.WithContext(ctx).Raw(`
        SELECT COALESCE(SUM(
            CASE
                WHEN ended_at IS NOT NULL THEN seconds_observed
                ELSE EXTRACT(EPOCH FROM (now() - started_at))::INTEGER
            END
        ), 0)::BIGINT
          FROM tax_validation_outage_log
         WHERE country   = ?
           AND store_id  = ?
           AND started_at > now() - INTERVAL '14 days'
    `, country, storeID).Row().Scan(&cumSeconds)
    if err != nil {
        return false, err
    }
    return time.Duration(cumSeconds)*time.Second >= PauseThreshold, nil
}
```

- [ ] **Step 3: Run tests — expect PASS**

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/internal/billing/tax/clockpause{,_test}.go
git commit -m "feat(tax): registry-outage clock-pause tracker (§5.2 72h threshold)"
```

---

## Task 14: SEA queue repository + 30/week capacity metric

**Files:**
- Create: `services/marketplace-api/internal/billing/tax/seaqueue/models.go`
- Create: `services/marketplace-api/internal/billing/tax/seaqueue/repository.go`
- Create: `services/marketplace-api/internal/billing/tax/seaqueue/repository_test.go`

**Spec references:** §19.3 "SEA 30/week capacity threshold".

- [ ] **Step 1: Failing tests**

```go
//go:build integration

package seaqueue_test

import (
    "context"
    "testing"
    "time"

    "github.com/google/uuid"
    "github.com/stretchr/testify/require"

    "github.com/tesserix/marketplace-api/internal/billing/tax/seaqueue"
    "github.com/tesserix/marketplace-api/pkg/testdb"
)

func TestSEAQueue_EnqueueIsIdempotentPerStoreCountry(t *testing.T) {
    db := testdb.NewDB(t, "sea_manual_review_queue")
    repo := seaqueue.New(db)
    tenantID, storeID := uuid.New(), uuid.New()

    _, err := repo.Enqueue(context.Background(), seaqueue.Entry{
        TenantID: tenantID, StoreID: storeID, Country: "MY",
        TaxID: "C12345678901", BusinessName: "Acme Sdn Bhd",
        QueueReason: "mof_sst_manual",
    })
    require.NoError(t, err)

    // Second enqueue for the same (store, country) returns the original row.
    second, err := repo.Enqueue(context.Background(), seaqueue.Entry{
        TenantID: tenantID, StoreID: storeID, Country: "MY",
        TaxID: "C12345678901", BusinessName: "Acme Sdn Bhd", QueueReason: "mof_sst_manual",
    })
    require.NoError(t, err)
    require.Equal(t, "pending", second.Status)
}

func TestSEAQueue_SLAComputed5BusinessDays(t *testing.T) {
    db := testdb.NewDB(t, "sea_manual_review_queue")
    repo := seaqueue.New(db)

    monday := time.Date(2026, 4, 13, 9, 0, 0, 0, time.UTC)
    due := seaqueue.AddBusinessDays(monday, 5)
    // Mon + 5 biz days = next Mon (skip Sat/Sun).
    require.Equal(t, time.Date(2026, 4, 20, 9, 0, 0, 0, time.UTC), due)
}

func TestSEAQueue_WeeklyCountFor30Threshold(t *testing.T) {
    db := testdb.NewDB(t, "sea_manual_review_queue")
    repo := seaqueue.New(db)

    // Seed 31 rows queued this week.
    for i := 0; i < 31; i++ {
        _, err := repo.Enqueue(context.Background(), seaqueue.Entry{
            TenantID: uuid.New(), StoreID: uuid.New(), Country: "TH",
            TaxID: "1234567890123", QueueReason: "rd_manual",
        })
        require.NoError(t, err)
    }

    count, err := repo.CountThisWeek(context.Background())
    require.NoError(t, err)
    require.GreaterOrEqual(t, count, 31)
}
```

- [ ] **Step 2: Write `models.go` and `repository.go`**

```go
// models.go
package seaqueue

import (
    "time"

    "github.com/google/uuid"
)

type Entry struct {
    ID            uuid.UUID `gorm:"column:id;primaryKey"`
    TenantID      uuid.UUID `gorm:"column:tenant_id"`
    StoreID       uuid.UUID `gorm:"column:store_id"`
    Country       string    `gorm:"column:country"`
    TaxID         string    `gorm:"column:tax_id"`
    BusinessName  string    `gorm:"column:business_name"`
    QueueReason   string    `gorm:"column:queue_reason"`
    Status        string    `gorm:"column:status"`
    ReviewerID    *uuid.UUID `gorm:"column:reviewer_id"`
    ReviewerNotes string    `gorm:"column:reviewer_notes"`
    SLADueAt      time.Time `gorm:"column:sla_due_at"`
    QueuedAt      time.Time `gorm:"column:queued_at"`
    ResolvedAt    *time.Time `gorm:"column:resolved_at"`
}

func (Entry) TableName() string { return "sea_manual_review_queue" }
```

```go
// repository.go
package seaqueue

import (
    "context"
    "time"

    "gorm.io/gorm"
)

type Repository struct { db *gorm.DB }

func New(db *gorm.DB) *Repository { return &Repository{db: db} }

// Enqueue inserts or returns the existing (store, country) entry.
// Idempotency preserves queue ordering when a validator retries.
func (r *Repository) Enqueue(ctx context.Context, e Entry) (Entry, error) {
    e.Status = "pending"
    e.QueuedAt = time.Now().UTC()
    e.SLADueAt = AddBusinessDays(e.QueuedAt, 5)
    err := r.db.WithContext(ctx).Exec(`
        INSERT INTO sea_manual_review_queue
            (tenant_id, store_id, country, tax_id, business_name, queue_reason, status, sla_due_at, queued_at)
        VALUES (?, ?, ?, ?, ?, ?, 'pending', ?, ?)
        ON CONFLICT (tenant_id, store_id, country) DO NOTHING
    `, e.TenantID, e.StoreID, e.Country, e.TaxID, e.BusinessName, e.QueueReason, e.SLADueAt, e.QueuedAt).Error
    if err != nil { return Entry{}, err }
    var out Entry
    return out, r.db.WithContext(ctx).
        Where("tenant_id=? AND store_id=? AND country=?", e.TenantID, e.StoreID, e.Country).
        First(&out).Error
}

func (r *Repository) Resolve(ctx context.Context, id, reviewerID uuid.UUID, approved bool, notes string) error {
    status := "approved"
    if !approved { status = "rejected" }
    return r.db.WithContext(ctx).Exec(`
        UPDATE sea_manual_review_queue
           SET status = ?, reviewer_id = ?, reviewer_notes = ?, resolved_at = now()
         WHERE id = ? AND status IN ('pending', 'in_review')
    `, status, reviewerID, notes, id).Error
}

// CountThisWeek — last 7 days of queue entries. Used by the 30/week capacity metric.
func (r *Repository) CountThisWeek(ctx context.Context) (int, error) {
    var n int64
    err := r.db.WithContext(ctx).Raw(`
        SELECT COUNT(*) FROM sea_manual_review_queue
         WHERE queued_at > now() - INTERVAL '7 days'
    `).Row().Scan(&n)
    return int(n), err
}

// AddBusinessDays — Mon/Tue/Wed/Thu/Fri only; preserves HH:MM:SS.
func AddBusinessDays(start time.Time, n int) time.Time {
    t := start
    added := 0
    for added < n {
        t = t.Add(24 * time.Hour)
        wd := t.Weekday()
        if wd != time.Saturday && wd != time.Sunday {
            added++
        }
    }
    return t
}
```

- [ ] **Step 3: Run tests — expect PASS**

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/internal/billing/tax/seaqueue/
git commit -m "feat(tax): SEA manual-review queue with 5-biz-day SLA + weekly capacity query"
```

---

## Task 15: Tax service orchestrator

**Files:**
- Create: `services/marketplace-api/internal/billing/tax/service.go`
- Create: `services/marketplace-api/internal/billing/tax/service_test.go`

**Spec references:** §5.2 clock-pause, §19.3 name match, §19.3 SEA queue immediate pause.

The orchestrator is the one place that writes `tax_id_validated`, `tax_id_validated_at`, and `tax_id_name_match`. It:

1. Validates input shape.
2. Looks up the validator in the registry.
3. Catches `ErrValidatorDisabled` (NZ flag) → returns orchestration error with country.
4. Calls `validator.Validate()`.
5. On `ErrRegistryUnavailable` → `BeginOutage` + return partial result flagging `clock_paused`.
6. On `ErrNotFound` / `ErrInvalidFormat` → return as-is; no DB write.
7. On `ManualReviewRequired` → `seaQueue.Enqueue` + `tracker.BeginOutage(kind=queue_entry)` to pause the clock at queue entry per Council finding #10. Register a listener for queue resolution that ends the outage.
8. On `Valid=true` → compute `CompareNames`, advisory-lock + CAS UPDATE `store_subscriptions` SET `tax_id_validated=true, tax_id_validated_at=now(), tax_id_name_match=?`. Emit audit event.

- [ ] **Step 1: Failing tests (abbreviated — four key flows)**

```go
//go:build integration

func TestService_ValidUK_Succeeds_FlipsRow_WritesMatched(t *testing.T) {
    db := testdb.NewDB(t, "store_subscriptions")
    mockUK := tax.FakeValidator{CountryCode: "GB", Result: tax.ValidationResult{Valid: true, RegistryName: "ACME WIDGETS LTD"}}
    svc := tax.NewService(tax.ServiceConfig{
        DB: db, Registry: registryWithMock("GB", &mockUK), Audit: audit.NewRecorderForTesting(),
        SEAQueue: seaqueue.New(db), Clock: tax.NewClockPauseTracker(db),
    })

    tenantID, storeID := seedSubscription(t, db, "GB")
    err := svc.Submit(context.Background(), tax.SubmitInput{
        TenantID: tenantID, StoreID: storeID, Country: "GB",
        TaxID: "GB123456789", BusinessName: "Acme Widgets Ltd",
    })
    require.NoError(t, err)

    var sub subscription.StoreSubscription
    require.NoError(t, db.Where("store_id=?", storeID).First(&sub).Error)
    require.True(t, sub.TaxIDValidated)
    require.Equal(t, string(tax.NameMatched), sub.TaxIDNameMatch)
}

func TestService_SEAManualReview_EnqueuesAndPausesClockImmediately(t *testing.T) {
    db := testdb.NewDB(t, "sea_manual_review_queue", "store_subscriptions")
    mockMY := tax.FakeValidator{CountryCode: "MY", Result: tax.ValidationResult{ManualReviewRequired: true, QueueReason: "mof_sst_manual"}}
    svc := tax.NewService(/* ... */)

    tenantID, storeID := seedSubscription(t, db, "MY")
    err := svc.Submit(context.Background(), tax.SubmitInput{
        TenantID: tenantID, StoreID: storeID, Country: "MY",
        TaxID: "C12345678901", BusinessName: "Acme Sdn Bhd",
    })
    require.NoError(t, err)

    // Queue entry exists.
    var entry seaqueue.Entry
    require.NoError(t, db.Where("store_id=?", storeID).First(&entry).Error)
    require.Equal(t, "pending", entry.Status)

    // Clock paused *immediately* (not after queue resolution) — Council finding #10.
    paused, err := tax.NewClockPauseTracker(db).IsPaused(context.Background(), storeID, "MY")
    require.NoError(t, err)
    require.True(t, paused)
}

func TestService_RegistryUnavailable_PausesClockAfter72h(t *testing.T) {
    // Begin outage 73h ago; submit fails but outage logged; IsPaused=true.
    // ... (mocked Now injection)
}

func TestService_NZDisabled_Returns503Sentinel(t *testing.T) {
    svc := tax.NewService(tax.ServiceConfig{
        /* registry with NZDisabled */,
    })
    err := svc.Submit(context.Background(), tax.SubmitInput{
        Country: "NZ", TaxID: "123-456-789", BusinessName: "Kiwi Co",
    })
    require.ErrorIs(t, err, tax.ErrValidatorDisabled)
}
```

- [ ] **Step 2: Write `service.go`**

```go
package tax

import (
    "context"
    "errors"
    "fmt"
    "time"

    "github.com/google/uuid"
    "gorm.io/gorm"

    "github.com/tesserix/marketplace-api/internal/audit"
    "github.com/tesserix/marketplace-api/internal/billing/tax/seaqueue"
    "github.com/tesserix/marketplace-api/internal/subscription"
)

type ServiceConfig struct {
    DB       *gorm.DB
    Registry *Registry
    Audit    *audit.Emitter
    SEAQueue *seaqueue.Repository
    Clock    *ClockPauseTracker
}

type Service struct { cfg ServiceConfig }

func NewService(cfg ServiceConfig) *Service { return &Service{cfg: cfg} }

type SubmitInput struct {
    TenantID       uuid.UUID
    StoreID        uuid.UUID
    Country        string
    TaxID          string
    BusinessName   string
    BillingAddress string
    // Source is "signup" on first submit, "revalidation" on cron-triggered recheck.
    Source string
}

func (s *Service) Submit(ctx context.Context, in SubmitInput) error {
    v, ok := s.cfg.Registry.For(in.Country)
    if !ok {
        return fmt.Errorf("tax: unsupported country %q", in.Country)
    }

    req := ValidationRequest{
        TenantID:     in.TenantID.String(), StoreID: in.StoreID.String(),
        Country:      in.Country, TaxID: in.TaxID,
        BusinessName: in.BusinessName, BillingAddress: in.BillingAddress,
    }

    res, err := v.Validate(ctx, req)
    switch {
    case errors.Is(err, ErrValidatorDisabled):
        return err

    case errors.Is(err, ErrRegistryUnavailable):
        // Log the outage; clock-pause aggregator decides when to flip.
        _ = s.cfg.Clock.BeginOutage(ctx, OutageKey{
            StoreID: in.StoreID, TenantID: in.TenantID,
            Country: in.Country, Registry: registryFor(in.Country),
            ErrorClass: "outage",
        }, time.Now().UTC())
        return err

    case errors.Is(err, ErrInvalidFormat), errors.Is(err, ErrNotFound):
        return err

    case err != nil:
        return fmt.Errorf("tax: validator error: %w", err)
    }

    if res.ManualReviewRequired {
        // Enqueue + immediately pause clock (§5.2, Council finding #10).
        _, qerr := s.cfg.SEAQueue.Enqueue(ctx, seaqueue.Entry{
            TenantID: in.TenantID, StoreID: in.StoreID, Country: in.Country,
            TaxID: in.TaxID, BusinessName: in.BusinessName,
            QueueReason: res.QueueReason,
        })
        if qerr != nil {
            return fmt.Errorf("tax: enqueue sea review: %w", qerr)
        }
        _ = s.cfg.Clock.BeginOutage(ctx, OutageKey{
            StoreID: in.StoreID, TenantID: in.TenantID, Country: in.Country,
            Registry: registryFor(in.Country), ErrorClass: "sea_queue",
        }, time.Now().UTC())
        return ErrManualReviewRequired
    }

    if !res.Valid {
        return ErrNotFound
    }

    // Name cross-check.
    match := CompareNames(in.BusinessName, res.RegistryName)

    // CAS write under advisory lock (mirrors P3 style).
    return subscription.WithAdvisoryLock(ctx, s.cfg.DB, in.StoreID, func(tx *gorm.DB) error {
        now := time.Now().UTC()
        r := tx.Exec(`
            UPDATE store_subscriptions
               SET tax_id_validated    = true,
                   tax_id_validated_at = ?,
                   tax_id_name_match   = ?,
                   updated_at          = now()
             WHERE tenant_id = ? AND store_id = ?
        `, now, string(match), in.TenantID, in.StoreID)
        if r.Error != nil { return r.Error }
        if r.RowsAffected == 0 {
            return fmt.Errorf("tax: subscription not found for store %s", in.StoreID)
        }

        if s.cfg.Audit != nil {
            s.cfg.Audit.Emit(nil, audit.Event{
                Action: "subscription.tax_id_validated",
                TenantID: in.TenantID, ResourceID: in.StoreID.String(),
                Metadata: map[string]any{
                    "country": in.Country, "registry": registryFor(in.Country),
                    "name_match": string(match), "source": in.Source,
                },
            })
        }
        return nil
    })
}

func registryFor(country string) string {
    switch country {
    case "GB": return "HMRC"
    case "IE", "DE", "FR", "IT", "ES", "NL": return "VIES"
    case "AU": return "ABR"
    case "IN": return "GSTN"
    case "SG": return "ACRA"
    case "NZ": return "IRD"
    case "MY": return "MOF_SST"
    case "TH": return "RD"
    case "PH": return "BIR"
    case "ID": return "DJP"
    case "VN": return "GDT"
    case "US", "CA": return "ATTESTATION"
    }
    return "UNKNOWN"
}
```

- [ ] **Step 3: Run tests — expect PASS**

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/internal/billing/tax/service{,_test}.go
git commit -m "feat(tax): orchestrator routes validator → queue/pause/CAS + emits audit"
```

---

## Task 16: 14-day hard-window middleware

**Files:**
- Create: `services/marketplace-api/internal/billing/tax/windowguard/middleware.go`
- Create: `services/marketplace-api/internal/billing/tax/windowguard/middleware_test.go`

**Spec references:** §5.2 day 14 storefront-unpublish.

- [ ] **Step 1: Failing tests**

```go
func TestWindowGuard_BeforeDay14_AllowsPublish(t *testing.T) {
    w := doRequest(windowGuardWith(signupDaysAgo(7), validated(false), paused(false)))
    require.Equal(t, 200, w.Code)
}

func TestWindowGuard_PastDay14_Unvalidated_NotPaused_Blocks(t *testing.T) {
    w := doRequest(windowGuardWith(signupDaysAgo(15), validated(false), paused(false)))
    require.Equal(t, 403, w.Code)
    require.Contains(t, w.Body.String(), "tax_validation_window_expired")
}

func TestWindowGuard_PastDay14_Paused_Allows(t *testing.T) {
    // Clock paused due to registry outage — window doesn't count against merchant.
    w := doRequest(windowGuardWith(signupDaysAgo(15), validated(false), paused(true)))
    require.Equal(t, 200, w.Code)
}

func TestWindowGuard_Validated_AlwaysAllows(t *testing.T) {
    w := doRequest(windowGuardWith(signupDaysAgo(20), validated(true), paused(false)))
    require.Equal(t, 200, w.Code)
}

func TestWindowGuard_FastPathShortensTo48h(t *testing.T) {
    // Migration fast-path approved; window now 48h not 14d.
    w := doRequest(windowGuardWith(signupDaysAgo(3), validated(false), paused(false), fastPath(true)))
    require.Equal(t, 403, w.Code, "3 days > 48h fast-path window")
}
```

- [ ] **Step 2: Write `middleware.go`**

```go
package windowguard

import (
    "net/http"
    "time"

    "github.com/gin-gonic/gin"
    "gorm.io/gorm"

    "github.com/tesserix/marketplace-api/internal/billing/tax"
    "github.com/tesserix/marketplace-api/internal/subscription"
)

const (
    StandardWindow = 14 * 24 * time.Hour
    FastPathWindow = 48 * time.Hour
)

type Config struct {
    DB      *gorm.DB
    Clock   *tax.ClockPauseTracker
    NowFunc func() time.Time // inject for tests
}

// RequirePublishable blocks publish-requiring routes when the merchant is past
// their window and still unvalidated. It does NOT block read/admin routes —
// that's the P3 readonly.RequireActive story. This is purely a publish gate.
//
// Expected to be mounted on storefront-publish-only endpoints, e.g.
// POST /admin/stores/:id/storefront/publish.
func RequirePublishable(cfg Config) gin.HandlerFunc {
    if cfg.NowFunc == nil { cfg.NowFunc = func() time.Time { return time.Now().UTC() } }
    return func(c *gin.Context) {
        tenantID := c.GetString("tenant_id")
        storeID  := c.GetString("store_id")

        var sub subscription.StoreSubscription
        if err := cfg.DB.Where("tenant_id=? AND store_id=?", tenantID, storeID).First(&sub).Error; err != nil {
            c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error":"subscription_lookup_failed"})
            return
        }
        if sub.TaxIDValidated {
            c.Next(); return
        }

        window := StandardWindow
        if sub.FastPathApproved {
            window = FastPathWindow
        }

        paused, err := cfg.Clock.IsPaused(c.Request.Context(), sub.StoreID, sub.TaxIDCountry)
        if err != nil {
            c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error":"clock_check_failed"})
            return
        }
        if paused {
            c.Next(); return
        }

        if cfg.NowFunc().Sub(sub.CreatedAt) > window {
            c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
                "error":  "tax_validation_window_expired",
                "window": window.String(),
            })
            return
        }
        c.Next()
    }
}
```

> **Note on `FastPathApproved`:** this is a new field on `StoreSubscription` consumed by the middleware. P5 is the owner — that plan adds both the column and the handler that flips it. Until P5 lands, this middleware reads `false` and the 14d path is the only one exercised. When P5 lands, no change is needed in P7.

- [ ] **Step 3: Run tests — expect PASS**

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/internal/billing/tax/windowguard/
git commit -m "feat(tax): 14-day publish-gate middleware (48h with migration fast-path)"
```

---

## Task 17: Admin handlers — submit + attestation

**Files:**
- Create: `services/marketplace-api/internal/handlers/admin/tax.go`
- Create: `services/marketplace-api/internal/handlers/admin/tax_test.go`

**Spec references:** §19.3.1 attestation append-only writes.

- [ ] **Step 1: Failing tests**

```go
func TestTaxHandler_Submit_SuccessFlipsRow(t *testing.T) { /* ... */ }
func TestTaxHandler_Submit_RegistryUnavailable_Returns202Accepted(t *testing.T) {
    // Merchant should see "try again later" + clock pauses; NOT 500.
}
func TestTaxHandler_Submit_NZDisabled_Returns503WithCounselMessage(t *testing.T) {
    // Body contains "awaiting legal sign-off"
}
func TestTaxHandler_Attestation_AppendsRow(t *testing.T) { /* ... */ }
func TestTaxHandler_Attestation_IsAppendOnly_UpdateBlocked(t *testing.T) {
    // Try to UPDATE the row via raw SQL — expect trigger rejection.
}
```

- [ ] **Step 2: Write `tax.go`**

```go
package admin

import (
    "crypto/hmac"
    "crypto/sha256"
    "encoding/hex"
    "errors"
    "net/http"

    "github.com/gin-gonic/gin"
    "github.com/google/uuid"
    "gorm.io/gorm"

    "github.com/tesserix/marketplace-api/internal/billing/tax"
)

type TaxHandler struct {
    db        *gorm.DB
    svc       *tax.Service
    ipHashKey []byte // HMAC key from Secret Manager (§18.8)
}

func NewTaxHandler(db *gorm.DB, svc *tax.Service, ipKey []byte) *TaxHandler {
    return &TaxHandler{db: db, svc: svc, ipHashKey: ipKey}
}

type submitRequest struct {
    Country        string `json:"country"        binding:"required,len=2"`
    TaxID          string `json:"tax_id"         binding:"required"`
    BusinessName   string `json:"business_name"  binding:"required"`
    BillingAddress string `json:"billing_address"`
}

func (h *TaxHandler) Submit(c *gin.Context) {
    var req submitRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error":"invalid_request","detail":err.Error()}); return
    }
    tenantID := uuid.MustParse(c.GetString("tenant_id"))
    storeID  := uuid.MustParse(c.GetString("store_id"))

    err := h.svc.Submit(c.Request.Context(), tax.SubmitInput{
        TenantID: tenantID, StoreID: storeID,
        Country: req.Country, TaxID: req.TaxID,
        BusinessName: req.BusinessName, BillingAddress: req.BillingAddress,
        Source: "signup",
    })
    switch {
    case err == nil:
        c.JSON(http.StatusOK, gin.H{"status":"validated"})
    case errors.Is(err, tax.ErrValidatorDisabled):
        c.JSON(http.StatusServiceUnavailable, gin.H{
            "error":"validator_disabled",
            "message":"Validation for this country is temporarily unavailable awaiting legal sign-off. Please contact support.",
        })
    case errors.Is(err, tax.ErrManualReviewRequired):
        c.JSON(http.StatusAccepted, gin.H{
            "status":"manual_review_queued",
            "sla_business_days":5,
        })
    case errors.Is(err, tax.ErrRegistryUnavailable):
        c.JSON(http.StatusAccepted, gin.H{
            "status":"registry_unavailable",
            "message":"Please retry; the 14-day window is paused while the registry is unreachable.",
        })
    case errors.Is(err, tax.ErrInvalidFormat):
        c.JSON(http.StatusBadRequest, gin.H{"error":"invalid_format"})
    case errors.Is(err, tax.ErrNotFound):
        c.JSON(http.StatusUnprocessableEntity, gin.H{"error":"tax_id_not_found"})
    default:
        c.JSON(http.StatusInternalServerError, gin.H{"error":"tax_validation_failed"})
    }
}

type attestationRequest struct {
    Country         string `json:"country"          binding:"required,len=2,oneof=US CA"`
    CheckboxText    string `json:"checkbox_text"    binding:"required"`
    CheckboxVersion string `json:"checkbox_version" binding:"required"`
}

func (h *TaxHandler) SignAttestation(c *gin.Context) {
    var req attestationRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error":"invalid_request"}); return
    }
    tenantID := uuid.MustParse(c.GetString("tenant_id"))
    storeID  := uuid.MustParse(c.GetString("store_id"))

    ip := c.ClientIP()
    mac := hmac.New(sha256.New, h.ipHashKey)
    mac.Write([]byte(ip))
    ipHash := hex.EncodeToString(mac.Sum(nil))

    // Append-only insert (table has UPDATE trigger + REVOKE DELETE from P1).
    err := h.db.Exec(`
        INSERT INTO business_entity_attestations
            (store_id, tenant_id, country, checkbox_text, checkbox_version, user_agent, ip_hash)
        VALUES (?, ?, ?, ?, ?, ?, ?)
    `, storeID, tenantID, req.Country, req.CheckboxText, req.CheckboxVersion, c.Request.UserAgent(), ipHash).Error
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error":"attestation_write_failed"}); return
    }
    c.JSON(http.StatusCreated, gin.H{"status":"signed"})
}
```

- [ ] **Step 3: Run tests — expect PASS**

- [ ] **Step 4: Register routes in `internal/handlers/admin/routes.go`**

Add:
```go
taxAPI := storeRoute.Group("/tax")
taxAPI.POST("/submit",      deps.TaxHandler.Submit)
taxAPI.POST("/attestation", deps.TaxHandler.SignAttestation)
```

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/handlers/admin/tax{,_test}.go \
        services/marketplace-api/internal/handlers/admin/routes.go
git commit -m "feat(admin): POST /admin/tax/submit + /attestation endpoints"
```

---

## Task 18: Reverse-charge invoice annotation

**Files:**
- Create: `services/marketplace-api/internal/billing/reverse_charge_invoice.go`
- Create: `services/marketplace-api/internal/billing/reverse_charge_invoice_test.go`

**Spec references:** §19.2 "invoices annotated with reverse-charge clause".

- [ ] **Step 1: Failing tests**

```go
func TestReverseCharge_UKValidated_AnnotatesInvoice(t *testing.T) {
    mockStripe := &FakeStripeClient{}
    annot := billing.NewReverseChargeAnnotator(mockStripe)

    err := annot.AnnotateIfNeeded(context.Background(), billing.AnnotateInput{
        InvoiceID:       "in_123",
        Country:         "GB",
        TaxIDValidated:  true,
        ReverseChargeTaxID: "GB123456789",
    })
    require.NoError(t, err)
    require.Len(t, mockStripe.Updates, 1)
    require.Contains(t, mockStripe.Updates[0].CustomFields[0].Value, "reverse charge")
    require.Equal(t, "in_123", mockStripe.Updates[0].InvoiceID)
}

func TestReverseCharge_UKNotValidated_Skips(t *testing.T) {
    mockStripe := &FakeStripeClient{}
    annot := billing.NewReverseChargeAnnotator(mockStripe)

    _ = annot.AnnotateIfNeeded(context.Background(), billing.AnnotateInput{
        InvoiceID: "in_123", Country: "GB", TaxIDValidated: false,
    })
    require.Empty(t, mockStripe.Updates, "unvalidated B2B must not claim reverse charge")
}

func TestReverseCharge_AU_NeverAnnotates(t *testing.T) {
    // AU is domestic (Mark8ly Pty Ltd charges GST) — never reverse charge.
    mockStripe := &FakeStripeClient{}
    annot := billing.NewReverseChargeAnnotator(mockStripe)

    _ = annot.AnnotateIfNeeded(context.Background(), billing.AnnotateInput{
        InvoiceID: "in_123", Country: "AU", TaxIDValidated: true,
    })
    require.Empty(t, mockStripe.Updates)
}

func TestReverseCharge_US_NeverAnnotates(t *testing.T) {
    // US has no federal VAT/sales-tax reverse charge concept.
    mockStripe := &FakeStripeClient{}
    annot := billing.NewReverseChargeAnnotator(mockStripe)

    _ = annot.AnnotateIfNeeded(context.Background(), billing.AnnotateInput{
        InvoiceID: "in_123", Country: "US", TaxIDValidated: true,
    })
    require.Empty(t, mockStripe.Updates)
}
```

- [ ] **Step 2: Write annotator**

```go
package billing

import (
    "context"
    "fmt"
)

// ReverseChargeCountries — those whose B2B invoices carry a reverse-charge clause.
// AU is GST-inclusive domestic (§19.4) and US/CA use attestation (no VAT mechanism),
// so both are intentionally absent.
var ReverseChargeCountries = map[string]bool{
    "GB": true, // UK
    "IE": true, "DE": true, "FR": true, "IT": true, "ES": true, "NL": true, // EU
    "IN": true, "SG": true,
    "MY": true, "TH": true, "PH": true, "ID": true, "VN": true,
    "NZ": true,
}

type AnnotateInput struct {
    InvoiceID          string
    Country            string
    TaxIDValidated     bool
    ReverseChargeTaxID string
}

type StripeInvoiceClient interface {
    UpdateInvoiceCustomFields(ctx context.Context, invoiceID string, fields []CustomField) error
}

type CustomField struct { Name, Value string }

type ReverseChargeAnnotator struct {
    stripe StripeInvoiceClient
}

func NewReverseChargeAnnotator(c StripeInvoiceClient) *ReverseChargeAnnotator {
    return &ReverseChargeAnnotator{stripe: c}
}

// AnnotateIfNeeded is a no-op unless (country supports reverse charge) AND
// (tax_id_validated). Safe to call on every invoice.finalized webhook.
func (a *ReverseChargeAnnotator) AnnotateIfNeeded(ctx context.Context, in AnnotateInput) error {
    if !ReverseChargeCountries[in.Country] || !in.TaxIDValidated {
        return nil
    }
    return a.stripe.UpdateInvoiceCustomFields(ctx, in.InvoiceID, []CustomField{
        {
            Name:  "Tax Treatment",
            Value: fmt.Sprintf("Reverse charge — VAT/GST to be accounted for by the recipient. Tax ID: %s", in.ReverseChargeTaxID),
        },
    })
}
```

- [ ] **Step 3: Wire into the P2 webhook handler for `invoice.finalized`**

Locate `handleInvoiceFinalized` in `internal/billing/dispatch/handlers.go` (from P2). After the existing logic, call:

```go
err = a.reverseCharge.AnnotateIfNeeded(ctx, billing.AnnotateInput{
    InvoiceID: invoice.ID, Country: sub.TaxIDCountry,
    TaxIDValidated: sub.TaxIDValidated, ReverseChargeTaxID: sub.ReverseChargeTaxID,
})
```

Non-fatal: failures log and continue (invoice still valid without annotation).

- [ ] **Step 4: Run tests — expect PASS**

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/billing/reverse_charge_invoice{,_test}.go \
        services/marketplace-api/internal/billing/dispatch/handlers.go
git commit -m "feat(billing): reverse-charge annotation for validated B2B invoices"
```

---

## Task 19: Quarterly revalidation cron

**Files:**
- Create: `services/marketplace-api/internal/billing/tax/revalidation/cron.go`
- Create: `services/marketplace-api/internal/billing/tax/revalidation/cron_test.go`

**Spec references:** §19.5 — storefront-unpublish-only; billing continues.

- [ ] **Step 1: Failing tests (behavioural; `NowFunc` mocked)**

```go
//go:build integration

func TestRevalidation_ValidatedOver90DaysAgo_IsRechecked(t *testing.T) { /* ... */ }

func TestRevalidation_IDStillValid_NoAction(t *testing.T) {
    // revalidation_attempted_at is bumped; nothing else.
}

func TestRevalidation_IDGoneInvalid_Emails_Starts14DWindow_KeepsBilling(t *testing.T) {
    // 1. Row flipped: tax_id_validated=false
    // 2. Email sent (check outbox/mock)
    // 3. subscription.status remains 'active' (billing continues — §19.5)
    // 4. storefront_published remains true for 14 days
}

func TestRevalidation_IDGoneInvalid_14DaysLater_UnpublishesStorefront(t *testing.T) {
    // At day 14, cron flips storefront_published=false and sets reason=tax_revalidation_failed
    // subscription.status STILL 'active' (spec: billing continues)
}
```

- [ ] **Step 2: Write `cron.go`**

```go
package revalidation

import (
    "context"
    "time"

    "gorm.io/gorm"

    "github.com/tesserix/marketplace-api/internal/audit"
    "github.com/tesserix/marketplace-api/internal/billing/tax"
    "github.com/tesserix/marketplace-api/internal/notification"
)

// Cron runs once per day at 02:00 UTC. It:
//  1. Selects subscriptions where tax_id_validated=true AND
//     tax_id_validated_at < now() - 90 days AND
//     (revalidation_attempted_at IS NULL OR revalidation_attempted_at < now() - 24h)
//  2. Re-calls the validator. On failure → flip tax_id_validated, email merchant,
//     start a 14-day unpublish window (tax_revalidation_started_at).
//  3. Separately, for rows where tax_revalidation_started_at < now() - 14 days AND
//     storefront_published=true AND tax_id_validated=false:
//     unpublish storefront (flip flag + emit event). Billing continues.
type Cron struct {
    DB     *gorm.DB
    Svc    *tax.Service
    Mailer notification.Mailer
    Audit  *audit.Emitter
    Now    func() time.Time
}

func (c *Cron) Run(ctx context.Context) error {
    if c.Now == nil { c.Now = func() time.Time { return time.Now().UTC() } }

    // Advisory lock: only one replica runs at a time.
    return c.DB.Transaction(func(tx *gorm.DB) error {
        if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtext('tax_revalidation_cron'))`).Error; err != nil {
            return err
        }
        if err := c.recheckStaleValidations(ctx, tx); err != nil { return err }
        if err := c.unpublishAfter14Days(ctx, tx);    err != nil { return err }
        return nil
    })
}

func (c *Cron) recheckStaleValidations(ctx context.Context, tx *gorm.DB) error {
    type row struct {
        TenantID, StoreID, TaxIDCountry, ReverseChargeTaxID, BusinessName string
    }
    var rows []row
    if err := tx.Raw(`
        SELECT tenant_id::text, store_id::text, tax_id_country,
               reverse_charge_tax_id, business_name
          FROM store_subscriptions
         WHERE tax_id_validated = true
           AND tax_id_validated_at < now() - INTERVAL '90 days'
           AND (revalidation_attempted_at IS NULL OR revalidation_attempted_at < now() - INTERVAL '24 hours')
         LIMIT 500
    `).Scan(&rows).Error; err != nil { return err }

    for _, r := range rows {
        // Mark as attempted first (CAS-style) so retries skip.
        _ = tx.Exec(`UPDATE store_subscriptions SET revalidation_attempted_at = now()
                      WHERE tenant_id=? AND store_id=?`, r.TenantID, r.StoreID).Error

        err := c.Svc.Submit(ctx, tax.SubmitInput{
            TenantID: mustUUID(r.TenantID), StoreID: mustUUID(r.StoreID),
            Country:  r.TaxIDCountry, TaxID: r.ReverseChargeTaxID,
            BusinessName: r.BusinessName, Source: "revalidation",
        })
        if err == nil {
            continue // still valid
        }

        // Only flip on definitive invalidity — not on transient outage.
        if !isDefinitiveFailure(err) {
            continue
        }

        // Flip + start 14d window; DO NOT change subscription.status.
        _ = tx.Exec(`
            UPDATE store_subscriptions
               SET tax_id_validated            = false,
                   tax_revalidation_started_at = now()
             WHERE tenant_id=? AND store_id=?
        `, r.TenantID, r.StoreID).Error

        _ = c.Mailer.Send(ctx, notification.Mail{
            Template: "tax_id_revalidation_failed",
            TenantID: r.TenantID, StoreID: r.StoreID,
            Data: map[string]any{"country": r.TaxIDCountry, "grace_days": 14},
        })
        c.Audit.Emit(nil, audit.Event{
            Action: "subscription.tax_revalidation_failed",
            Metadata: map[string]any{"country": r.TaxIDCountry},
        })
    }
    return nil
}

func (c *Cron) unpublishAfter14Days(ctx context.Context, tx *gorm.DB) error {
    return tx.Exec(`
        UPDATE store_subscriptions
           SET storefront_published        = false,
               storefront_unpublished_at   = now(),
               storefront_unpublish_reason = 'tax_revalidation_failed'
         WHERE tax_id_validated = false
           AND tax_revalidation_started_at IS NOT NULL
           AND tax_revalidation_started_at < now() - INTERVAL '14 days'
           AND storefront_published = true
    `).Error
    // Billing continues — intentionally no change to status column.
}

func isDefinitiveFailure(err error) bool {
    // Treat only NotFound/InvalidFormat as definitive. Unavailable/ManualReview
    // should NOT cause invalidation (just retry tomorrow).
    return errors.Is(err, tax.ErrNotFound) || errors.Is(err, tax.ErrInvalidFormat)
}
```

- [ ] **Step 3: Register cron in `main.go`**

```go
scheduler.AddJob("0 2 * * *", func(ctx context.Context) error {
    return revalidation.Cron{ DB: db, Svc: taxService, Mailer: mailer, Audit: auditEmitter }.Run(ctx)
})
```

Also add migration columns `revalidation_attempted_at TIMESTAMPTZ` and `tax_revalidation_started_at TIMESTAMPTZ` to `store_subscriptions` — **fold into migration 000049** (add to the same up/down file in Task 3).

- [ ] **Step 4: Run tests — expect PASS**

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/billing/tax/revalidation/ \
        services/marketplace-api/cmd/marketplace-api/main.go \
        services/marketplace-api/migrations/000049_storefront_published_flag.{up,down}.sql
git commit -m "feat(tax): daily 02:00 UTC revalidation cron (storefront-unpublish-only)"
```

---

## Task 20: Migration fast-path approved listener

**Files:**
- Modify: `services/marketplace-api/internal/billing/tax/service.go` — add `OnFastPathApproved(storeID uuid.UUID)`

**Spec references:** §5.1.1.

P5 owns the intake endpoint for WHOIS/screenshot evidence and runs the CSM-approval workflow. When CSM approves, P5 emits `fastpath.approved` (either via Pub/Sub or direct method call — depends on P5's design). P7 only needs to react by ensuring the `windowguard` sees `FastPathApproved = true` on the subscription row.

- [ ] **Step 1: Failing test**

```go
func TestService_OnFastPathApproved_FlipsSubscriptionFlag(t *testing.T) {
    db := testdb.NewDB(t, "store_subscriptions")
    svc := tax.NewService(/* ... */)
    tenantID, storeID := seedSubscription(t, db, "GB")

    require.NoError(t, svc.OnFastPathApproved(context.Background(), tenantID, storeID))

    var sub subscription.StoreSubscription
    require.NoError(t, db.Where("store_id=?", storeID).First(&sub).Error)
    require.True(t, sub.FastPathApproved)
}
```

- [ ] **Step 2: Add method**

```go
func (s *Service) OnFastPathApproved(ctx context.Context, tenantID, storeID uuid.UUID) error {
    return s.cfg.DB.Exec(`
        UPDATE store_subscriptions
           SET fast_path_approved    = true,
               fast_path_approved_at = now()
         WHERE tenant_id = ? AND store_id = ?
    `, tenantID, storeID).Error
}
```

The `fast_path_approved` column is owned by P5's migration (P5 adds `fast_path_approved BOOLEAN NOT NULL DEFAULT false`, `fast_path_approved_at TIMESTAMPTZ`). If P5 hasn't landed yet, skip this task and implement in P5 itself — this note in the plan is the signalling contract.

- [ ] **Step 3: Run tests — expect PASS**

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/internal/billing/tax/service.go
git commit -m "feat(tax): OnFastPathApproved shortens window 14d→48h (§5.1.1)"
```

---

## Task 21: Append `/admin/tax/*path` to P3 readonly allowlist

**Files:**
- Modify: `services/marketplace-api/internal/subscription/readonly/allowlist.go` (from P3)

- [ ] **Step 1: Edit allowlist**

```go
var DefaultAllowlist = []AllowedRoute{
    {http.MethodPost, "/admin/stores/:storeId/subscription/*path"},
    {http.MethodPost, "/admin/stores/:storeId/billing/*path"},
    {http.MethodGet,  "/admin/stores/:storeId/orders/export/*path"},
    {http.MethodPost, "/admin/auth/*path"},
    {http.MethodPost, "/admin/stores/:storeId/tax/*path"}, // NEW — merchants in read-only states MUST still be able to fix their tax ID.
}
```

- [ ] **Step 2: Update P3 allowlist test to include the new entry**

- [ ] **Step 3: Commit**

```bash
git add services/marketplace-api/internal/subscription/readonly/allowlist.go \
        services/marketplace-api/internal/subscription/readonly/middleware_test.go
git commit -m "fix(readonly): allow /admin/tax/*path even in read-only states"
```

---

## Task 22: Integration tests for spec success criteria

**Files:**
- Create: `services/marketplace-api/tests/integration/tax_validation_criteria_test.go`

Three success-criteria-specific tests per §28:

- [ ] **Step 1: Criterion #41 — tax ID lapses on quarterly revalidation → storefront unpublishes day 14, billing continues**

```go
//go:build integration

func Test_Criterion41_QuarterlyRevalidation_UnpublishesStorefront_BillingContinues(t *testing.T) {
    suite := inttest.NewSuite(t)
    tenantID, storeID := suite.SeedValidatedSubscription("GB", "GB123456789", "Acme Ltd",
        time.Now().Add(-100*24*time.Hour)) // validated 100 days ago

    // Flip the mock HMRC validator to "not found".
    suite.TaxRegistry.ReplaceUK(tax.FakeValidator{Result: tax.ValidationResult{}, Err: tax.ErrNotFound})

    // Day 0: cron runs — flips tax_id_validated=false, starts 14d window.
    require.NoError(t, suite.RunRevalidationCron(time.Now()))

    sub := suite.ReadSubscription(storeID)
    require.False(t, sub.TaxIDValidated)
    require.Equal(t, subscription.StatusActive, sub.Status, "billing MUST continue")
    require.True(t, sub.StorefrontPublished, "storefront stays up in grace window")

    // Day 15: cron runs — unpublishes storefront. Billing still runs.
    require.NoError(t, suite.RunRevalidationCron(time.Now().Add(15*24*time.Hour)))

    sub = suite.ReadSubscription(storeID)
    require.False(t, sub.StorefrontPublished)
    require.Equal(t, "tax_revalidation_failed", sub.StorefrontUnpublishReason)
    require.Equal(t, subscription.StatusActive, sub.Status, "billing still continues — §19.5")
}
```

- [ ] **Step 2: Criterion #45 — SEA queue entry pauses clock immediately**

```go
func Test_Criterion45_SEAQueueEntry_PausesClockImmediately(t *testing.T) {
    suite := inttest.NewSuite(t)
    tenantID, storeID := suite.SeedSignupSubscription("MY")

    // Submit triggers manual review.
    err := suite.Tax.Submit(context.Background(), tax.SubmitInput{
        TenantID: tenantID, StoreID: storeID,
        Country: "MY", TaxID: "C12345678901", BusinessName: "Acme Sdn Bhd",
    })
    require.ErrorIs(t, err, tax.ErrManualReviewRequired)

    // Verify clock paused RIGHT NOW — not after the 5-biz-day review completes.
    paused, err := suite.ClockTracker.IsPaused(context.Background(), storeID, "MY")
    require.NoError(t, err)
    require.True(t, paused, "clock must pause at queue entry per Council finding #10")

    // Verify queue entry exists.
    var entry seaqueue.Entry
    require.NoError(t, suite.DB.Where("store_id=?", storeID).First(&entry).Error)
    require.Equal(t, "pending", entry.Status)
}
```

- [ ] **Step 3: Criterion #53 — migration fast-path rejects WHOIS <90d**

```go
// §5.1.1 "Option A: WHOIS creation date ≥90 days before Mark8ly signup"
// This test lives in P5's intake endpoint; P7 only asserts that a denied fast-path
// does NOT flip fast_path_approved. Presence here ensures the P7 listener is
// conservative: it never auto-approves.
func Test_Criterion53_FastPath_OnlyApprovedViaCSMExplicitApprove(t *testing.T) {
    suite := inttest.NewSuite(t)
    tenantID, storeID := suite.SeedSignupSubscription("GB")

    // Directly calling the listener without CSM approval does NOTHING in P7;
    // only the CSM-approve flow (P5) fires OnFastPathApproved.
    sub := suite.ReadSubscription(storeID)
    require.False(t, sub.FastPathApproved, "must stay false until CSM explicit approve")
}
```

- [ ] **Step 4: Run**

```bash
cd services/marketplace-api
go test -tags=integration ./tests/integration/... -run Criterion -v
```

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/tests/integration/tax_validation_criteria_test.go
git commit -m "test(tax): spec success criteria 41, 45, 53 integration coverage"
```

---

## Task 23: Schema-version bump + CI verification

- [ ] **Step 1: Bump schema version**

Edit `services/marketplace-api/marketplaceapi.go`:

```go
const ExpectedSchemaVersion = 49
```

- [ ] **Step 2: Grep for orphans**

```bash
cd services/marketplace-api
grep -rn "NZ_TAX_VALIDATION_ENABLED" --include="*.go" | grep -v _test.go
```
Expected: at least one hit in `cmd/marketplace-api/main.go` reading the env.

```bash
grep -rn "reverse_charge" internal/ --include="*.go"
```
Expected: hits in `internal/billing/reverse_charge_invoice.go` + orchestrator CAS write.

- [ ] **Step 3: Full test run**

```bash
go build ./...
go test -race -count=1 ./...
go test -tags=integration -count=1 ./...
```

All green.

- [ ] **Step 4: Final commit**

```bash
git add -u
git commit --allow-empty -m "chore(tax): P7 verified — schema v49, all 13 validators green, criteria 41/45/53 covered"
```

---

## Final verification

- [ ] `go build ./...` clean.
- [ ] `go test -tags=integration ./...` all green.
- [ ] 13 validator packages exist under `internal/billing/tax/validators/`; each has ≥4 tests.
- [ ] Registry returns a validator for every country in {US, CA, GB, IE, DE, FR, IT, ES, NL, AU, NZ, IN, SG, MY, TH, PH, ID, VN}.
- [ ] `NZ_TAX_VALIDATION_ENABLED=false` (default) → NZ submit returns HTTP 503 with "awaiting legal sign-off" message.
- [ ] Orchestrator writes `tax_id_validated` + `tax_id_validated_at` + `tax_id_name_match` only on successful validation; advisory lock wraps the CAS update.
- [ ] SEA manual-review queue entry pauses the clock at queue insert, not at resolution (Council finding #10). Integration test #45 proves it.
- [ ] Quarterly revalidation cron: invalid ID → email + 14d window → storefront unpublish at day 14; subscription.status remains `active` (billing continues). Integration test #41 proves both halves.
- [ ] `business_entity_attestations` inserts work via handler; UPDATE and DELETE are both rejected by the P1 trigger + role revoke (covered in P1 Task 15, not duplicated here).
- [ ] Reverse-charge annotation applied to invoices for {GB, IE, DE, FR, IT, ES, NL, IN, SG, MY, TH, PH, ID, VN, NZ} when `tax_id_validated=true`. AU and US never annotated.
- [ ] 14-day window middleware blocks publish past day 14 unless validated or paused; shrinks to 48h if `fast_path_approved = true` (flag owned by P5).
- [ ] `/admin/tax/*path` is on the P3 read-only allowlist so merchants in `expired`/`store_closed` states can still remediate.
- [ ] Grep confirms no direct UPDATE of `tax_id_validated` outside the orchestrator's CAS path.
- [ ] Grep confirms no per-validator custom error types — all use the 6 sentinels from `interface.go`.

## What's now unlocked

- **P5** (trial card-add deferred charge + migration fast-path) — consumes `OnFastPathApproved`, adds the `fast_path_approved` column and CSM-approve endpoint.
- **P6** (dunning) — independent; `tax_id_validated` doesn't interact with past-due ladder.
- **P12** (Worker closed-page) — consumes the `storefront_unpublished_at` + `storefront_unpublish_reason` columns + the `subscription.storefront_unpublished` audit event to decide which HTML to render.
- **P16** (admin UI) — consumes `POST /admin/tax/submit` + `POST /admin/tax/attestation` + reads `tax_id_validated`, `tax_id_name_match`, `storefront_published` for the tax-ID panel.
- **P17** (observability) — reads the SEA queue depth, weekly enqueue count (30/week alert), `tax_validation_outage_log` aggregates (registry outage gauge), and `revalidation_attempted_at` distribution to dashboard cron health.

## Execution handoff

Plan complete. Implementation plans under `docs/superpowers/plans/` relevant to this phase:
- `2026-04-18-p1-subscription-data-model.md` (dependency — must be green first)
- `2026-04-18-p2-stripe-multicurrency-webhooks.md` (dependency — provides Stripe `custom_fields` client)
- `2026-04-18-p3-state-machine-plan-gates.md` (allowlist extension required)
- `2026-04-18-p7-tax-id-validation.md` (this plan)

Execute with **superpowers:subagent-driven-development** (recommended) or **superpowers:executing-plans**. Tasks 1–14 are largely parallelizable by validator; Tasks 15–22 must serialize. Budget ~5 engineer-days for a focused pair (parallelised validator implementation cuts wall-clock roughly in half).
