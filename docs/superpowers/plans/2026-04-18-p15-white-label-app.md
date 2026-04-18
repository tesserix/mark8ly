# P15 — White-Label Mobile App Add-on: Purchase, Credentials, Teardown Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship the entire server-side lifecycle of the $199/mo + $2,000-setup white-label mobile-app add-on: a Pro-gated purchase endpoint that co-terminates the add-on with the Pro annual renewal, a hardened Secret-Manager-backed credential store for Apple App Store Connect API keys + Google Play service-account JSON with a single choke-point for reads + audit logging, a daily cron that advances the §13.5 sunset lifecycle (day 0 → 7 → 30 → 60 → 90) against Apple / Google / Firebase real APIs, an append-only `app_contract_attestations` table capturing the Apple 4.2.6 acknowledgment at purchase, and the observability counters that will feed P17 dashboards.

**Architecture:** Three new packages with tight boundaries.

1. `internal/billing/appaddon/` — purchase endpoint. Computes proration (`(remaining_days_of_pro_year / 365) × $199 × 12 + $2000`) against the Pro anchor subscription's `current_period_end`. Creates a Stripe Invoice via the P2 Stripe client. The `invoice.paid` webhook already flows through P2 dispatch; P15 adds a `handleInvoicePaidForAppAddOn` that flips `has_white_label_app_add_on = true` and writes the attestation row. **Not** a subscription state-machine transition — the add-on lives on the same `store_subscriptions` row as a boolean, orthogonal to `SubscriptionStatus` (§13.5 explicitly separates these).

2. `internal/billing/appcreds/` — **THE** credential package. Single public API: `Store(ctx, tenantID, creds)`, `Load(ctx, tenantID, credType)`, `Delete(ctx, tenantID, credType)`. All reads go through this one file. Every call emits an `audit.EmitCredentialAccess` event with `(actor, tenant_id, credential_type, operation)`. The package is the only module in the codebase authorized to call `cloud.google.com/go/secretmanager`. CI enforces this via a lint rule in Task 12.

3. `internal/whitelabel/lifecycle/` — the teardown cron. A `robfig/cron/v3` scheduler (daily 05:00 UTC) queries `white_label_app_lifecycle` for rows where `next_action_at <= now()`, dispatches to a small state table keyed on `WhiteLabelAppStatus`, and advances each row. Day-30 calls Apple ASC (`App.availability` field) and Google Play (`edits.tracks.update`) to block downloads. Day-60 pulls the apps. Day-90 deletes Firebase + purges all four Secret Manager paths for the tenant. Merchant-initiated immediate pull skips the gradual schedule and compresses to 7 days. Cron is triggered by the P11 `subscription.pro_app_cancelled` event landing a row in `white_label_app_lifecycle` with `status='sunset_scheduled'`; the cron does the rest.

A fourth, thinner package, `internal/billing/attestations/`, owns the `app_contract_attestations` table. It mirrors P1's `business_entity_attestations` pattern exactly: append-only via UPDATE-blocking trigger + role-level `REVOKE DELETE`. One `Record(ctx, input)` function. That's it.

**Tech Stack:** Go 1.26, Gin, GORM, PostgreSQL 15, `github.com/go-jose/go-jose/v4` (Apple ASC JWT signing ES256 from the `.p8` key), `firebase.google.com/go/v4` (Firebase Admin — project archive + delete), `cloud.google.com/go/secretmanager` (credential I/O), `golang.org/x/oauth2/google` (Google Play Android Publisher API auth from service-account JSON), `github.com/robfig/cron/v3` (scheduler), existing `internal/audit` emitter, P1's `subscription.WithAdvisoryLock` for concurrency on `has_white_label_app_add_on` flips, P2's Stripe invoice client.

**Spec:** [`docs/superpowers/specs/2026-04-17-subscription-model-design.md`](../specs/2026-04-17-subscription-model-design.md) — §3.4 (add-on price, co-termination rule), §13 (teardown lifecycle), §14.2 (Pro+App onboarding flow), §18.9 (credential storage paths + IAM).

**Depends on:**
- **P1** — `WhiteLabelAppStatus` enum, `white_label_app_lifecycle` table (migration 046), `has_white_label_app_add_on` column on `store_subscriptions`, `business_entity_attestations` pattern we mirror for `app_contract_attestations`.
- **P2** — Stripe invoice client (`internal/billing/stripeclient.Invoice.Create`) and the webhook dispatcher we register a new handler on.
- **P3** — `statemachine.Transition` (referenced but **not modified**; app lifecycle is deliberately orthogonal to `SubscriptionStatus` per §13.5).
- **P11** — emits `subscription.pro_app_cancelled` event that seeds a `sunset_scheduled` row in `white_label_app_lifecycle` via a tiny consumer we add in Task 9.

**Out of scope:**
- Mobile-app build pipeline itself (Fastlane, xcodebuild, gradle release).
- Per-tenant Firebase project **provisioning** Terraform (we only delete from here — creation is `tesserix-infra/terraform/07-firebase-tenants/` in a separate phase).
- Admin UI for credential upload + lifecycle status (P16).
- Alerting rules (P17).
- IAM wiring for CI/CD SA + eng staff (we document the required bindings; actual Terraform lands in `tesserix-infra/`).

**Related plans:**
- **P11** (cancellation + save-offer) — emits `subscription.pro_app_cancelled`; we consume it.
- **P16** (admin frontend) — wraps the credential upload + lifecycle view endpoints here.
- **P17** (observability) — reads the counters + audit events this plan emits.

---

## Scope Check

In scope:

1. `POST /admin/stores/:storeId/subscription/add-on/white-label-app` — Pro-gated, computes prorated + setup Stripe invoice, returns hosted URL.
2. Webhook extension: `handleInvoicePaidForAppAddOn` — flips `has_white_label_app_add_on=true`, writes attestation row.
3. Co-termination: prorated amount aligns add-on charge to Pro annual renewal; next renewal bundles Pro + App at combined rate (priced out of P2's renewal invoice builder, not this plan).
4. `POST /admin/stores/:storeId/app-credentials/apple` — upload `.p8` + issuer_id + key_id, P8 format validation, write to Secret Manager.
5. `POST /admin/stores/:storeId/app-credentials/google` — upload service-account JSON, validate shape, write to Secret Manager.
6. `internal/billing/appcreds/` package — single choke-point for ALL credential reads, with audit emit + observability counter on every access.
7. `app_contract_attestations` table migration — append-only with UPDATE-blocking trigger + role-level `REVOKE DELETE`, same pattern as P1 Task 7.
8. Daily 05:00 UTC lifecycle cron — advances rows in `white_label_app_lifecycle` through `sunset_scheduled → downloads_blocked → pulled → firebase_archived → credentials_purged`.
9. `internal/whitelabel/apple/` — Apple ASC API client (JWT signing + availability/app-level endpoints used at day 30 + 60).
10. `internal/whitelabel/googleplay/` — Google Play Android Publisher client (track update for day 30, app suspension for day 60).
11. `internal/whitelabel/firebase/` — Firebase Admin wrapper (project archive at day 60, project delete at day 90).
12. Consumer for `subscription.pro_app_cancelled` — seeds a `sunset_scheduled` row; accepts a merchant-initiated variant that compresses to 7 days.
13. Observability counters: `white_label_app.lifecycle_transition{from,to}` and `white_label_app.credential_accessed{type}` via Prometheus.
14. IAM policy doc (committed as `docs/ops/white-label-app-iam.md`) for the Terraform handoff.

Out of scope (reiterated):

- Mobile-app build pipeline, per-tenant Firebase Terraform creation, admin UI (P16), alert wiring (P17).
- Any change to `subscription.SubscriptionStatus` or the P3 state machine — app lifecycle is orthogonal.
- Migrating from `app_contract_attestations` to a generalized attestation table. Mirror P1's shape; unification is a v2 concern.

---

## File Structure

### Create

**Attestation table:**
- `services/marketplace-api/internal/db/migrations/050_app_contract_attestations.sql`
- `services/marketplace-api/internal/billing/attestations/attestations.go`
- `services/marketplace-api/internal/billing/attestations/attestations_test.go`

**Credential store (SECURITY CRITICAL):**
- `services/marketplace-api/internal/billing/appcreds/appcreds.go` — the only module that imports `cloud.google.com/go/secretmanager`.
- `services/marketplace-api/internal/billing/appcreds/paths.go` — path builders — pure, tested.
- `services/marketplace-api/internal/billing/appcreds/validate.go` — P8 + JSON format validators.
- `services/marketplace-api/internal/billing/appcreds/appcreds_test.go`
- `services/marketplace-api/internal/billing/appcreds/validate_test.go`
- `services/marketplace-api/internal/billing/appcreds/paths_test.go`

**Purchase endpoint:**
- `services/marketplace-api/internal/billing/appaddon/proration.go` — pure math, no I/O.
- `services/marketplace-api/internal/billing/appaddon/handler.go` — `POST /admin/stores/:storeId/subscription/add-on/white-label-app`.
- `services/marketplace-api/internal/billing/appaddon/handler_test.go`
- `services/marketplace-api/internal/billing/appaddon/proration_test.go`
- `services/marketplace-api/internal/billing/appaddon/webhook.go` — `handleInvoicePaidForAppAddOn` registered into P2 dispatch.
- `services/marketplace-api/internal/billing/appaddon/webhook_test.go`

**Credential upload endpoints:**
- `services/marketplace-api/internal/handlers/admin/app_credentials.go` — the two `POST` handlers.
- `services/marketplace-api/internal/handlers/admin/app_credentials_test.go`

**External API clients:**
- `services/marketplace-api/internal/whitelabel/apple/client.go` — ASC JWT + HTTP client.
- `services/marketplace-api/internal/whitelabel/apple/client_test.go`
- `services/marketplace-api/internal/whitelabel/googleplay/client.go` — Android Publisher API wrapper.
- `services/marketplace-api/internal/whitelabel/googleplay/client_test.go`
- `services/marketplace-api/internal/whitelabel/firebase/client.go` — Firebase Admin wrapper.
- `services/marketplace-api/internal/whitelabel/firebase/client_test.go`

**Lifecycle cron:**
- `services/marketplace-api/internal/whitelabel/lifecycle/scheduler.go` — `robfig/cron/v3` wiring.
- `services/marketplace-api/internal/whitelabel/lifecycle/advancer.go` — state-table-driven row advancer.
- `services/marketplace-api/internal/whitelabel/lifecycle/advancer_test.go`
- `services/marketplace-api/internal/whitelabel/lifecycle/scheduler_test.go`
- `services/marketplace-api/internal/whitelabel/lifecycle/pro_app_cancelled_consumer.go` — consumes P11 event, seeds row.
- `services/marketplace-api/internal/whitelabel/lifecycle/pro_app_cancelled_consumer_test.go`

**Observability:**
- `services/marketplace-api/internal/whitelabel/metrics/metrics.go` — `white_label_app.*` counter definitions.

**Ops docs:**
- `docs/ops/white-label-app-iam.md` — IAM binding list for Terraform.

### Modify

- `services/marketplace-api/internal/billing/dispatch/dispatcher.go` — register `handleInvoicePaidForAppAddOn` into the `invoice.paid` handler chain.
- `services/marketplace-api/internal/handlers/admin/routes.go` — mount the three new admin endpoints.
- `services/marketplace-api/cmd/marketplace-api/main.go` — start the lifecycle cron, wire the three external clients, wire `appcreds.Store`.

### Delete

None.

---

## Task Sequence Overview

| # | Task | Depends on |
|---|---|---|
| 1 | `app_contract_attestations` migration (append-only) + attestations pkg | P1 |
| 2 | `appcreds` package — paths + validation (pure) | — |
| 3 | `appcreds` package — Secret Manager I/O + audit emit | 2, P1 audit emitter |
| 4 | Apple + Google credential upload handlers | 3 |
| 5 | Proration math (pure) | — |
| 6 | Add-on purchase handler — Stripe invoice create | 5, P2 Stripe client |
| 7 | `invoice.paid` webhook extension — flips add-on, writes attestation | 1, 6, P2 dispatcher |
| 8 | Apple ASC + Google Play + Firebase client wrappers | 3 |
| 9 | `pro_app_cancelled` consumer seeds lifecycle row | P11 |
| 10 | Lifecycle advancer — state table + per-status handlers | 3, 8 |
| 11 | Cron scheduler + main wiring | 10 |
| 12 | Observability counters + lint rule restricting Secret Manager imports | 3 |

---

## Reusable patterns

**A. Credential choke-point.** Every read of Apple / Google credentials flows through `appcreds.Load(ctx, tenantID, credType)`. That function:
1. Builds the Secret Manager path from `paths.go`.
2. Calls `secretmanager.AccessSecretVersion`.
3. Emits an audit event via `audit.EmitCredentialAccess(actor, tenantID, credType, "read")`.
4. Increments the `white_label_app.credential_accessed{type=…}` Prometheus counter.
5. Returns the raw bytes.

No caller is permitted to skip steps 3 + 4. Task 12 adds a CI lint (`go vet` custom analyzer OR a simple `grep` in `ci.yml`) that fails the build if any file outside `internal/billing/appcreds/` imports `cloud.google.com/go/secretmanager`.

**B. Path builder pattern.** `appcreds.paths.go` is pure and string-only:

```go
type CredType string
const (
    CredTypeAppleP8        CredType = "apple-asc-api-key"
    CredTypeAppleIssuerID  CredType = "apple-asc-issuer-id"
    CredTypeAppleKeyID     CredType = "apple-asc-key-id"
    CredTypeGooglePlayJSON CredType = "google-play-service-account"
)

func Path(projectID, tenantID string, t CredType) string {
    return fmt.Sprintf("projects/%s/secrets/merchant_%s_%s",
        projectID, tenantID, string(t))
}
```

Note the path encoding: Secret Manager doesn't allow `/` in secret names, so the logical §18.9 path `/secrets/merchant/{tenant_id}/apple-asc-api-key` flattens to the physical secret name `merchant_{tenant_id}_apple-asc-api-key`. The logical-vs-physical distinction is documented in `paths.go` and asserted in `paths_test.go`.

**C. Attestation table — P1 mirror.** `app_contract_attestations` mirrors `business_entity_attestations` (P1 Task 7) exactly: UUID primary key, tenant_id, store_id, subscription_id, `attestation_type` (= `'apple_4_2_6'`), `attested_at`, `attested_by_user_id`, `attestation_text` (the exact UI copy the user clicked OK on), `ip_address`, `user_agent`, `stripe_invoice_id`. Append-only is enforced by:
1. BEFORE UPDATE trigger that raises `P0001` with `"append-only: updates forbidden on app_contract_attestations"`.
2. Role-level `REVOKE DELETE ON app_contract_attestations FROM marketplace_api_rw;` — the app role cannot delete; only the DDL-owning migrator role can.

Both guards must be present; the trigger alone is not sufficient because the trigger can be disabled. Both `pg_policy` inspection and an integration test (Task 1 Step 4) assert both guards are live.

**D. Proration formula.** §3.4 specifies `(remaining_days_of_pro_year / 365) × $199 × 12 + $2000`. Implemented as pure integer-cents math in `proration.go` with half-even rounding. Zero day remainder = pay zero proration + $2000 setup (weirdly, if they buy on the exact renewal day). Renewal day boundary test included.

**E. Lifecycle state table.** `advancer.go` defines:

```go
type lifecycleStep struct {
    From            WhiteLabelAppStatus
    To              WhiteLabelAppStatus
    DaysFromSunset  int
    Action          func(ctx, row) error // side effects — Apple/Google/Firebase/Secret Manager
    Severity        audit.Severity
}

var lifecycleTable = []lifecycleStep{
    {From: StatusSunsetScheduled,  To: StatusSunsetScheduled,  DaysFromSunset: 7,  Action: emitBannerEvent,       Severity: audit.SeverityInfo},
    {From: StatusSunsetScheduled,  To: StatusDownloadsBlocked, DaysFromSunset: 30, Action: blockDownloads,         Severity: audit.SeverityWarning},
    {From: StatusDownloadsBlocked, To: StatusPulled,           DaysFromSunset: 60, Action: pullApps,               Severity: audit.SeverityWarning},
    {From: StatusPulled,           To: StatusFirebaseArchived, DaysFromSunset: 60, Action: archiveFirebase,        Severity: audit.SeverityWarning},
    {From: StatusFirebaseArchived, To: StatusCredentialsPurged, DaysFromSunset: 90, Action: deleteFirebaseAndPurge, Severity: audit.SeverityError},
}
```

The advancer iterates rows where `scheduled_at + DaysFromSunset days <= now()` and applies each step idempotently (Apple/Google/Firebase APIs are re-callable; Secret Manager delete is a no-op on 404).

**F. Idempotency for external API calls.** Apple + Google + Firebase calls must be safe to re-run. Apple ASC `PATCH /v1/apps/{id}` is idempotent. Google Play `edits.tracks.update` is idempotent (you're overwriting a track config). Firebase `projects.delete` 404s are swallowed. Every action function returns `nil` on 404s from upstream; callers don't distinguish "already applied" from "applied just now".

**G. Advisory lock on add-on flip.** `has_white_label_app_add_on = true` flip in Task 7 wraps inside `subscription.WithAdvisoryLock(ctx, db, storeID, fn)` (from P1) to serialize concurrent webhook replays of `invoice.paid`.

---

## Task 1: `app_contract_attestations` migration + attestations pkg

**Files:**
- Create: `services/marketplace-api/internal/db/migrations/050_app_contract_attestations.sql`
- Create: `services/marketplace-api/internal/billing/attestations/attestations.go`
- Create: `services/marketplace-api/internal/billing/attestations/attestations_test.go`

**Spec references:** §13.2 (Apple 4.2.6 ack), §14.2 (contract capture at purchase), P1 Task 7 (append-only pattern to mirror).

- [ ] **Step 1: Failing test — record + fetch round-trip**

```go
//go:build integration

package attestations_test

import (
    "context"
    "testing"

    "github.com/google/uuid"
    "github.com/stretchr/testify/require"

    "github.com/tesserix/marketplace-api/internal/billing/attestations"
    "github.com/tesserix/marketplace-api/pkg/testdb"
)

func TestRecord_RoundTrip(t *testing.T) {
    db := testdb.NewDB(t, "app_contract_attestations")
    tenantID, storeID, userID, subID := uuid.New(), uuid.New(), uuid.New(), uuid.New()

    id, err := attestations.Record(context.Background(), db, attestations.Input{
        TenantID: tenantID, StoreID: storeID, SubscriptionID: subID,
        AttestationType: attestations.TypeApple426,
        AttestedByUserID: userID,
        AttestationText: "I acknowledge Apple Guideline 4.2.6 may cause first-review rejection…",
        IPAddress: "10.0.0.1", UserAgent: "curl/8",
        StripeInvoiceID: "in_abc",
    })
    require.NoError(t, err)

    got, err := attestations.FindByStripeInvoice(context.Background(), db, "in_abc")
    require.NoError(t, err)
    require.Equal(t, id, got.ID)
    require.Equal(t, storeID, got.StoreID)
}
```

- [ ] **Step 2: Failing test — UPDATE rejected by trigger**

```go
func TestAttestation_UpdateForbidden(t *testing.T) {
    db := testdb.NewDB(t, "app_contract_attestations")
    tenantID, storeID, userID, subID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
    id, err := attestations.Record(context.Background(), db, attestations.Input{
        TenantID: tenantID, StoreID: storeID, SubscriptionID: subID,
        AttestationType: attestations.TypeApple426,
        AttestedByUserID: userID, AttestationText: "x",
        StripeInvoiceID: "in_1",
    })
    require.NoError(t, err)

    err = db.Exec(`UPDATE app_contract_attestations SET attestation_text='tampered' WHERE id=?`, id).Error
    require.Error(t, err, "UPDATE must be blocked by trigger")
    require.Contains(t, err.Error(), "append-only")
}
```

- [ ] **Step 3: Failing test — DELETE rejected at role level**

```go
func TestAttestation_DeleteForbiddenByRoleRevoke(t *testing.T) {
    db := testdb.NewDB(t, "app_contract_attestations")
    // testdb connects as marketplace_api_rw (same role prod uses).
    tenantID, storeID, userID, subID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
    _, err := attestations.Record(context.Background(), db, attestations.Input{
        TenantID: tenantID, StoreID: storeID, SubscriptionID: subID,
        AttestationType: attestations.TypeApple426, AttestedByUserID: userID,
        AttestationText: "x", StripeInvoiceID: "in_1",
    })
    require.NoError(t, err)

    err = db.Exec(`DELETE FROM app_contract_attestations`).Error
    require.Error(t, err, "DELETE must be blocked by role-level REVOKE")
    require.Contains(t, err.Error(), "permission denied")
}
```

- [ ] **Step 4: Run tests — expect FAIL (table + package don't exist)**

```bash
cd services/marketplace-api
go test -tags=integration ./internal/billing/attestations/... -v
```

- [ ] **Step 5: Write migration `050_app_contract_attestations.sql`**

Mirror `business_entity_attestations` from P1 Task 7. Fields:
- `id UUID PRIMARY KEY DEFAULT gen_random_uuid()`
- `tenant_id UUID NOT NULL`
- `store_id UUID NOT NULL`
- `subscription_id UUID NOT NULL`
- `attestation_type TEXT NOT NULL CHECK (attestation_type IN ('apple_4_2_6'))`
- `attested_at TIMESTAMPTZ NOT NULL DEFAULT now()`
- `attested_by_user_id UUID NOT NULL`
- `attestation_text TEXT NOT NULL`
- `ip_address INET`
- `user_agent TEXT`
- `stripe_invoice_id TEXT NOT NULL UNIQUE`
- Index on `(tenant_id, store_id)`.

Then the two guards (critical — both required):

```sql
CREATE OR REPLACE FUNCTION app_contract_attestations_no_update() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'append-only: updates forbidden on app_contract_attestations'
      USING ERRCODE = 'P0001';
END;
$$;

CREATE TRIGGER app_contract_attestations_no_update
    BEFORE UPDATE ON app_contract_attestations
    FOR EACH ROW EXECUTE FUNCTION app_contract_attestations_no_update();

REVOKE DELETE ON app_contract_attestations FROM marketplace_api_rw;
```

- [ ] **Step 6: Write `attestations.go`**

```go
package attestations

import (
    "context"
    "time"

    "github.com/google/uuid"
    "gorm.io/gorm"
)

type Type string

const TypeApple426 Type = "apple_4_2_6"

type Input struct {
    TenantID        uuid.UUID
    StoreID         uuid.UUID
    SubscriptionID  uuid.UUID
    AttestationType Type
    AttestedByUserID uuid.UUID
    AttestationText string
    IPAddress       string
    UserAgent       string
    StripeInvoiceID string
}

type Record struct {
    ID              uuid.UUID
    TenantID        uuid.UUID
    StoreID         uuid.UUID
    SubscriptionID  uuid.UUID
    AttestationType Type
    AttestedAt      time.Time
    AttestedByUserID uuid.UUID
    AttestationText string
    IPAddress       string
    UserAgent       string
    StripeInvoiceID string
}

// TableName ensures GORM maps to the unprefixed table name.
func (Record) TableName() string { return "app_contract_attestations" }

// Record inserts a new attestation. Returns the new row ID.
func Record(ctx context.Context, db *gorm.DB, in Input) (uuid.UUID, error) {
    r := Record{
        ID: uuid.New(), TenantID: in.TenantID, StoreID: in.StoreID,
        SubscriptionID: in.SubscriptionID, AttestationType: in.AttestationType,
        AttestedAt: time.Now().UTC(), AttestedByUserID: in.AttestedByUserID,
        AttestationText: in.AttestationText, IPAddress: in.IPAddress,
        UserAgent: in.UserAgent, StripeInvoiceID: in.StripeInvoiceID,
    }
    if err := db.WithContext(ctx).Create(&r).Error; err != nil {
        return uuid.Nil, err
    }
    return r.ID, nil
}

func FindByStripeInvoice(ctx context.Context, db *gorm.DB, invoiceID string) (Record, error) {
    var r Record
    err := db.WithContext(ctx).Where("stripe_invoice_id=?", invoiceID).First(&r).Error
    return r, err
}
```

- [ ] **Step 7: Run tests — expect PASS**

- [ ] **Step 8: Commit**

```bash
git add services/marketplace-api/internal/db/migrations/050_app_contract_attestations.sql \
        services/marketplace-api/internal/billing/attestations/
git commit -m "feat(attestations): app_contract_attestations append-only table + record/find API"
```

---

## Task 2: `appcreds` package — paths + validation (pure)

**Files:**
- Create: `services/marketplace-api/internal/billing/appcreds/paths.go`
- Create: `services/marketplace-api/internal/billing/appcreds/validate.go`
- Create: `services/marketplace-api/internal/billing/appcreds/paths_test.go`
- Create: `services/marketplace-api/internal/billing/appcreds/validate_test.go`

**Spec references:** §18.9.

- [ ] **Step 1: Failing test — path builder matches §18.9**

```go
package appcreds_test

import (
    "testing"

    "github.com/stretchr/testify/require"
    "github.com/tesserix/marketplace-api/internal/billing/appcreds"
)

func TestPath_MatchesSpec18_9(t *testing.T) {
    tenantID := "11111111-1111-1111-1111-111111111111"
    project := "tesserix-prod"

    require.Equal(t,
        "projects/tesserix-prod/secrets/merchant_11111111-1111-1111-1111-111111111111_apple-asc-api-key",
        appcreds.Path(project, tenantID, appcreds.CredTypeAppleP8))
    require.Equal(t,
        "projects/tesserix-prod/secrets/merchant_11111111-1111-1111-1111-111111111111_apple-asc-issuer-id",
        appcreds.Path(project, tenantID, appcreds.CredTypeAppleIssuerID))
    require.Equal(t,
        "projects/tesserix-prod/secrets/merchant_11111111-1111-1111-1111-111111111111_apple-asc-key-id",
        appcreds.Path(project, tenantID, appcreds.CredTypeAppleKeyID))
    require.Equal(t,
        "projects/tesserix-prod/secrets/merchant_11111111-1111-1111-1111-111111111111_google-play-service-account",
        appcreds.Path(project, tenantID, appcreds.CredTypeGooglePlayJSON))
}

func TestAllCredTypes_Enumerated(t *testing.T) {
    // For teardown — we iterate this list at day 90.
    require.ElementsMatch(t, []appcreds.CredType{
        appcreds.CredTypeAppleP8,
        appcreds.CredTypeAppleIssuerID,
        appcreds.CredTypeAppleKeyID,
        appcreds.CredTypeGooglePlayJSON,
    }, appcreds.AllCredTypes())
}
```

- [ ] **Step 2: Failing test — P8 + JSON validators**

```go
func TestValidateP8_AcceptsEC256PrivateKey(t *testing.T) {
    // Generated with `openssl ecparam -genkey -name prime256v1 -out key.pem` then
    // `openssl pkcs8 -topk8 -nocrypt -in key.pem`.
    valid := []byte(`-----BEGIN PRIVATE KEY-----
MIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQgX…
-----END PRIVATE KEY-----`)
    require.NoError(t, appcreds.ValidateP8(valid))
}

func TestValidateP8_RejectsRSAKey(t *testing.T) {
    require.Error(t, appcreds.ValidateP8([]byte("-----BEGIN RSA PRIVATE KEY-----\nABC\n-----END RSA PRIVATE KEY-----")))
}

func TestValidateP8_RejectsGarbage(t *testing.T) {
    require.Error(t, appcreds.ValidateP8([]byte("not a pem file")))
}

func TestValidateGooglePlayJSON_AcceptsServiceAccount(t *testing.T) {
    valid := []byte(`{"type":"service_account","project_id":"x","private_key_id":"y","private_key":"z","client_email":"x@x.iam.gserviceaccount.com","client_id":"1","token_uri":"https://oauth2.googleapis.com/token"}`)
    require.NoError(t, appcreds.ValidateGooglePlayJSON(valid))
}

func TestValidateGooglePlayJSON_RejectsUserCred(t *testing.T) {
    userCred := []byte(`{"type":"authorized_user","client_id":"x","client_secret":"y","refresh_token":"z"}`)
    require.Error(t, appcreds.ValidateGooglePlayJSON(userCred))
}
```

- [ ] **Step 3: Run — expect FAIL**

- [ ] **Step 4: Write `paths.go`**

```go
package appcreds

import "fmt"

type CredType string

const (
    CredTypeAppleP8        CredType = "apple-asc-api-key"
    CredTypeAppleIssuerID  CredType = "apple-asc-issuer-id"
    CredTypeAppleKeyID     CredType = "apple-asc-key-id"
    CredTypeGooglePlayJSON CredType = "google-play-service-account"
)

// AllCredTypes is the iteration order used at day-90 purge.
func AllCredTypes() []CredType {
    return []CredType{
        CredTypeAppleP8, CredTypeAppleIssuerID, CredTypeAppleKeyID, CredTypeGooglePlayJSON,
    }
}

// Path returns the Secret Manager fully-qualified secret name. Secret Manager
// disallows '/' in names, so the logical §18.9 path
//   /projects/{project}/secrets/merchant/{tenant_id}/{cred_type}
// is flattened to the physical name
//   projects/{project}/secrets/merchant_{tenant_id}_{cred_type}
func Path(projectID, tenantID string, t CredType) string {
    return fmt.Sprintf("projects/%s/secrets/merchant_%s_%s",
        projectID, tenantID, string(t))
}
```

- [ ] **Step 5: Write `validate.go`**

Use `encoding/pem` + `crypto/x509` to parse the `.p8` and assert `ecdsa.PrivateKey` with P-256 curve. Reject anything else. For Google Play, `json.Unmarshal` into a minimal struct with `Type string json:"type"` and assert `== "service_account"` plus presence of `private_key`, `client_email`, `project_id`.

- [ ] **Step 6: Run tests — expect PASS**

- [ ] **Step 7: Commit**

```bash
git add services/marketplace-api/internal/billing/appcreds/paths.go \
        services/marketplace-api/internal/billing/appcreds/validate.go \
        services/marketplace-api/internal/billing/appcreds/paths_test.go \
        services/marketplace-api/internal/billing/appcreds/validate_test.go
git commit -m "feat(appcreds): Secret Manager path builder + P8/JSON validators"
```

---

## Task 3: `appcreds` Secret Manager I/O + audit emit

**Files:**
- Create: `services/marketplace-api/internal/billing/appcreds/appcreds.go`
- Create: `services/marketplace-api/internal/billing/appcreds/appcreds_test.go`

**Spec references:** §18.9 (audit on every access + IAM scoping + deletion at teardown).

- [ ] **Step 1: Failing test — Store/Load/Delete round trip against fake**

```go
package appcreds_test

import (
    "context"
    "testing"

    "github.com/google/uuid"
    "github.com/stretchr/testify/require"

    "github.com/tesserix/marketplace-api/internal/audit"
    "github.com/tesserix/marketplace-api/internal/billing/appcreds"
)

func TestStore_LoadRoundTrip(t *testing.T) {
    fake := appcreds.NewFakeSM() // injectable fake
    rec := audit.NewRecorderForTesting()
    em := audit.NewEmitter(rec)

    svc := appcreds.NewService(appcreds.Config{
        ProjectID: "tesserix-prod", SM: fake, Emitter: em,
    })

    tenantID := uuid.New()
    p8 := []byte("-----BEGIN PRIVATE KEY-----\n…\n-----END PRIVATE KEY-----")
    require.NoError(t, svc.Store(context.Background(),
        appcreds.StoreInput{
            TenantID: tenantID, CredType: appcreds.CredTypeAppleP8,
            Payload: p8, Actor: "user:abc",
        }))

    got, err := svc.Load(context.Background(),
        appcreds.LoadInput{
            TenantID: tenantID, CredType: appcreds.CredTypeAppleP8,
            Actor: "system:build-pipeline",
        })
    require.NoError(t, err)
    require.Equal(t, p8, got)

    em.FlushForTesting()
    // Store + Load each emit one audit event.
    require.Len(t, rec.Events(), 2)
    require.Equal(t, "write", rec.Events()[0].Metadata["operation"])
    require.Equal(t, "read",  rec.Events()[1].Metadata["operation"])
    require.Equal(t, "apple-asc-api-key", rec.Events()[1].Metadata["credential_type"])
}
```

- [ ] **Step 2: Failing test — cross-tenant Load returns not-found (tenant scoping)**

```go
func TestLoad_CrossTenant_NotFound(t *testing.T) {
    fake := appcreds.NewFakeSM()
    em := audit.NewEmitter(audit.NewRecorderForTesting())
    svc := appcreds.NewService(appcreds.Config{ProjectID: "p", SM: fake, Emitter: em})

    tenantA, tenantB := uuid.New(), uuid.New()
    require.NoError(t, svc.Store(context.Background(),
        appcreds.StoreInput{TenantID: tenantA, CredType: appcreds.CredTypeAppleP8, Payload: []byte("A")}))

    _, err := svc.Load(context.Background(),
        appcreds.LoadInput{TenantID: tenantB, CredType: appcreds.CredTypeAppleP8, Actor: "user:x"})
    require.ErrorIs(t, err, appcreds.ErrNotFound)
}
```

- [ ] **Step 3: Failing test — Delete is idempotent (double delete does not error)**

```go
func TestDelete_Idempotent(t *testing.T) {
    fake := appcreds.NewFakeSM()
    em := audit.NewEmitter(audit.NewRecorderForTesting())
    svc := appcreds.NewService(appcreds.Config{ProjectID: "p", SM: fake, Emitter: em})

    tenantID := uuid.New()
    require.NoError(t, svc.Store(context.Background(),
        appcreds.StoreInput{TenantID: tenantID, CredType: appcreds.CredTypeAppleP8, Payload: []byte("x")}))
    require.NoError(t, svc.Delete(context.Background(),
        appcreds.DeleteInput{TenantID: tenantID, CredType: appcreds.CredTypeAppleP8, Actor: "system:cron"}))
    require.NoError(t, svc.Delete(context.Background(),
        appcreds.DeleteInput{TenantID: tenantID, CredType: appcreds.CredTypeAppleP8, Actor: "system:cron"}),
        "second delete must swallow 404")
}
```

- [ ] **Step 4: Failing test — PurgeAll deletes all four cred types**

```go
func TestPurgeAll_RemovesAllFourCredentials(t *testing.T) {
    fake := appcreds.NewFakeSM()
    em := audit.NewEmitter(audit.NewRecorderForTesting())
    svc := appcreds.NewService(appcreds.Config{ProjectID: "p", SM: fake, Emitter: em})

    tenantID := uuid.New()
    for _, ct := range appcreds.AllCredTypes() {
        require.NoError(t, svc.Store(context.Background(),
            appcreds.StoreInput{TenantID: tenantID, CredType: ct, Payload: []byte("x")}))
    }
    require.NoError(t, svc.PurgeAll(context.Background(), tenantID, "system:cron:day_90"))

    for _, ct := range appcreds.AllCredTypes() {
        _, err := svc.Load(context.Background(),
            appcreds.LoadInput{TenantID: tenantID, CredType: ct, Actor: "test"})
        require.ErrorIs(t, err, appcreds.ErrNotFound)
    }
}
```

- [ ] **Step 5: Run — expect FAIL**

- [ ] **Step 6: Write `appcreds.go`**

The `Service` struct wraps a narrow interface (`SM`) so tests inject a fake. In production, `SM` is backed by `cloud.google.com/go/secretmanager`. Key methods:

```go
type SM interface {
    CreateSecret(ctx context.Context, name string) error
    AddSecretVersion(ctx context.Context, name string, payload []byte) error
    AccessSecretVersion(ctx context.Context, name string) ([]byte, error)
    DeleteSecret(ctx context.Context, name string) error
}

type Service struct {
    projectID string
    sm        SM
    emitter   *audit.Emitter
    counter   *metrics.Counter // increment on every Load
}

func (s *Service) Store(ctx context.Context, in StoreInput) error { … }
func (s *Service) Load(ctx context.Context, in LoadInput) ([]byte, error) { … }
func (s *Service) Delete(ctx context.Context, in DeleteInput) error { … }
func (s *Service) PurgeAll(ctx context.Context, tenantID uuid.UUID, actor string) error {
    for _, ct := range AllCredTypes() {
        if err := s.Delete(ctx, DeleteInput{TenantID: tenantID, CredType: ct, Actor: actor}); err != nil {
            return err
        }
    }
    return nil
}
```

Every method emits:
```go
s.emitter.EmitCredentialAccess(ctx, audit.CredentialAccess{
    TenantID: in.TenantID, CredentialType: string(in.CredType),
    Operation: "read"|"write"|"delete", Actor: in.Actor,
})
```
and (for `Load`) increments `s.counter.WithLabels(string(in.CredType)).Inc()`.

`ErrNotFound` is returned when `AccessSecretVersion` returns a `NotFound` gRPC status. Cross-tenant scoping is structural — the tenant ID is in the path, so `tenantB` can never resolve `tenantA`'s path.

- [ ] **Step 7: Implement the real GCP SM adapter**

```go
type gcpSM struct{ cli *secretmanager.Client }
// Thin shim — each method maps 1:1 onto the SDK call.
```

- [ ] **Step 8: Run tests — expect PASS**

- [ ] **Step 9: Commit**

```bash
git add services/marketplace-api/internal/billing/appcreds/appcreds.go \
        services/marketplace-api/internal/billing/appcreds/appcreds_test.go
git commit -m "feat(appcreds): Service with Store/Load/Delete/PurgeAll + audit + metrics"
```

---

## Task 4: Apple + Google credential upload handlers

**Files:**
- Create: `services/marketplace-api/internal/handlers/admin/app_credentials.go`
- Create: `services/marketplace-api/internal/handlers/admin/app_credentials_test.go`

**Spec references:** §14.2 step "Apple ASC + Google Play credentials collected", §18.9.

- [ ] **Step 1: Failing test — POST /app-credentials/apple accepts multipart**

```go
//go:build integration

func TestPostAppleCredentials_Succeeds(t *testing.T) {
    suite := inttest.NewSuite(t)
    tenantID, storeID := suite.SeedStore(subscription.StatusActive, subscription.PlanPro)
    suite.SetHasWhiteLabelAppAddOn(storeID, true)

    body := suite.Multipart(map[string]any{
        "p8":         []byte("-----BEGIN PRIVATE KEY-----\n…\n-----END PRIVATE KEY-----"),
        "issuer_id":  "69a6de7e-…",
        "key_id":     "ABCD1234",
    })

    resp := suite.AdminPOST(tenantID, storeID,
        "/admin/stores/"+storeID.String()+"/app-credentials/apple", body)
    require.Equal(t, 204, resp.Code)

    // All three credentials exist in the fake Secret Manager.
    for _, ct := range []appcreds.CredType{
        appcreds.CredTypeAppleP8, appcreds.CredTypeAppleIssuerID, appcreds.CredTypeAppleKeyID,
    } {
        _, err := suite.AppCreds.Load(context.Background(),
            appcreds.LoadInput{TenantID: tenantID, CredType: ct, Actor: "test"})
        require.NoError(t, err)
    }
}
```

- [ ] **Step 2: Failing test — rejects non-Pro store (403)**

```go
func TestPostAppleCredentials_RejectsStarterPlan(t *testing.T) {
    suite := inttest.NewSuite(t)
    tenantID, storeID := suite.SeedStore(subscription.StatusActive, subscription.PlanStarter)

    body := suite.Multipart(map[string]any{"p8": []byte("x"), "issuer_id": "a", "key_id": "b"})
    resp := suite.AdminPOST(tenantID, storeID,
        "/admin/stores/"+storeID.String()+"/app-credentials/apple", body)
    require.Equal(t, 403, resp.Code)
    require.Contains(t, resp.Body.String(), "add_on_not_active")
}
```

- [ ] **Step 3: Failing test — invalid P8 returns 400**

```go
func TestPostAppleCredentials_InvalidP8_400(t *testing.T) {
    suite := inttest.NewSuite(t)
    tenantID, storeID := suite.SeedStore(subscription.StatusActive, subscription.PlanPro)
    suite.SetHasWhiteLabelAppAddOn(storeID, true)

    body := suite.Multipart(map[string]any{"p8": []byte("garbage"), "issuer_id": "a", "key_id": "b"})
    resp := suite.AdminPOST(tenantID, storeID,
        "/admin/stores/"+storeID.String()+"/app-credentials/apple", body)
    require.Equal(t, 400, resp.Code)
    require.Contains(t, resp.Body.String(), "invalid_p8_format")
}
```

- [ ] **Step 4: Failing test — POST /app-credentials/google**

```go
func TestPostGoogleCredentials_Succeeds(t *testing.T) {
    suite := inttest.NewSuite(t)
    tenantID, storeID := suite.SeedStore(subscription.StatusActive, subscription.PlanPro)
    suite.SetHasWhiteLabelAppAddOn(storeID, true)

    sa := []byte(`{"type":"service_account","project_id":"p","private_key":"k","client_email":"x@y.iam.gserviceaccount.com"}`)
    body := suite.Multipart(map[string]any{"service_account_json": sa})
    resp := suite.AdminPOST(tenantID, storeID,
        "/admin/stores/"+storeID.String()+"/app-credentials/google", body)
    require.Equal(t, 204, resp.Code)
}
```

- [ ] **Step 5: Run — expect FAIL**

- [ ] **Step 6: Write `app_credentials.go`**

```go
package admin

import (
    "io"
    "net/http"

    "github.com/gin-gonic/gin"

    "github.com/tesserix/marketplace-api/internal/billing/appcreds"
)

type AppCredentialsHandler struct {
    creds *appcreds.Service
}

// POST /admin/stores/:storeId/app-credentials/apple
// multipart form: p8 (file), issuer_id (string), key_id (string)
func (h *AppCredentialsHandler) PostApple(c *gin.Context) {
    if !mustHaveAppAddOn(c) { return } // 403 add_on_not_active if missing

    tenantID := uuid.MustParse(c.GetString("tenant_id"))
    actor := "user:" + c.GetString("user_id")

    p8, issuerID, keyID, err := readAppleMultipart(c)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_payload"})
        return
    }
    if err := appcreds.ValidateP8(p8); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_p8_format"})
        return
    }
    // Write all three under one handler call — they are one logical credential set.
    for _, p := range []struct {
        t appcreds.CredType
        v []byte
    }{
        {appcreds.CredTypeAppleP8, p8},
        {appcreds.CredTypeAppleIssuerID, []byte(issuerID)},
        {appcreds.CredTypeAppleKeyID, []byte(keyID)},
    } {
        if err := h.creds.Store(c.Request.Context(), appcreds.StoreInput{
            TenantID: tenantID, CredType: p.t, Payload: p.v, Actor: actor,
        }); err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": "store_failed"})
            return
        }
    }
    c.Status(http.StatusNoContent)
}

// POST /admin/stores/:storeId/app-credentials/google
// multipart form: service_account_json (file)
func (h *AppCredentialsHandler) PostGoogle(c *gin.Context) {
    if !mustHaveAppAddOn(c) { return }
    tenantID := uuid.MustParse(c.GetString("tenant_id"))
    actor := "user:" + c.GetString("user_id")

    f, _, err := c.Request.FormFile("service_account_json")
    if err != nil { c.JSON(400, gin.H{"error": "invalid_payload"}); return }
    defer f.Close()
    payload, err := io.ReadAll(f)
    if err != nil { c.JSON(400, gin.H{"error": "invalid_payload"}); return }

    if err := appcreds.ValidateGooglePlayJSON(payload); err != nil {
        c.JSON(400, gin.H{"error": "invalid_service_account_json"})
        return
    }
    if err := h.creds.Store(c.Request.Context(), appcreds.StoreInput{
        TenantID: tenantID, CredType: appcreds.CredTypeGooglePlayJSON,
        Payload: payload, Actor: actor,
    }); err != nil {
        c.JSON(500, gin.H{"error": "store_failed"})
        return
    }
    c.Status(http.StatusNoContent)
}

// mustHaveAppAddOn 403s unless subscription plan=pro AND has_white_label_app_add_on=true.
// Reads from Gin context keys set by StoreMiddleware (P3 Task 7).
func mustHaveAppAddOn(c *gin.Context) bool { … }
```

- [ ] **Step 7: Register routes in `handlers/admin/routes.go`**

```go
acHandler := &AppCredentialsHandler{creds: deps.AppCreds}
storeRoute.POST("/app-credentials/apple",  acHandler.PostApple)
storeRoute.POST("/app-credentials/google", acHandler.PostGoogle)
```

- [ ] **Step 8: Run tests — expect PASS**

- [ ] **Step 9: Commit**

```bash
git add services/marketplace-api/internal/handlers/admin/app_credentials.go \
        services/marketplace-api/internal/handlers/admin/app_credentials_test.go \
        services/marketplace-api/internal/handlers/admin/routes.go
git commit -m "feat(admin): Apple + Google white-label credential upload endpoints"
```

---

## Task 5: Proration math (pure)

**Files:**
- Create: `services/marketplace-api/internal/billing/appaddon/proration.go`
- Create: `services/marketplace-api/internal/billing/appaddon/proration_test.go`

**Spec references:** §3.4.

- [ ] **Step 1: Failing tests — the three boundary cases**

```go
package appaddon_test

import (
    "testing"
    "time"

    "github.com/stretchr/testify/require"
    "github.com/tesserix/marketplace-api/internal/billing/appaddon"
)

func TestProrationCents_FullYear_Remaining(t *testing.T) {
    // 365 days remaining = pay the full $199 × 12 + $2000 setup.
    renewalAt := time.Now().Add(365 * 24 * time.Hour)
    got := appaddon.ProrationCents(time.Now(), renewalAt)
    // $199 × 12 = $2388 prorated + $2000 setup = $4388 = 438_800 cents.
    require.Equal(t, int64(438_800), got)
}

func TestProrationCents_HalfYear_Remaining(t *testing.T) {
    // 183 days remaining ≈ 0.5014 × $2388 = $1197.33 + $2000 ≈ $3197 (rounded half-even).
    renewalAt := time.Now().Add(183 * 24 * time.Hour)
    got := appaddon.ProrationCents(time.Now(), renewalAt)
    require.InDelta(t, 319_733, got, 2) // allow ±2c for rounding
}

func TestProrationCents_SameDay_OnlySetupFee(t *testing.T) {
    // 0 days remaining → zero proration + $2000 setup = $2000 = 200_000 cents.
    now := time.Now()
    require.Equal(t, int64(200_000), appaddon.ProrationCents(now, now))
}

func TestProrationCents_NegativeRemaining_ClampedToZero(t *testing.T) {
    // Defensive: renewal in the past shouldn't give negative proration.
    now := time.Now()
    require.Equal(t, int64(200_000), appaddon.ProrationCents(now, now.Add(-24*time.Hour)))
}
```

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Write `proration.go`**

```go
package appaddon

import "time"

const (
    appAnnualCents = 2388_00  // $199 × 12
    setupFeeCents  = 2000_00
)

// ProrationCents returns the total up-front charge in USD cents for an
// add-on purchase at `now` with the Pro anchor renewing at `renewalAt`.
// Formula (§3.4): (remaining_days / 365) × $2388 + $2000.
func ProrationCents(now, renewalAt time.Time) int64 {
    remaining := renewalAt.Sub(now).Hours() / 24.0
    if remaining < 0 { remaining = 0 }
    if remaining > 365 { remaining = 365 }
    prorated := float64(appAnnualCents) * (remaining / 365.0)
    // Half-even rounding.
    roundedProrated := int64(prorated + 0.5)
    return roundedProrated + setupFeeCents
}
```

- [ ] **Step 4: Run tests — expect PASS**

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/billing/appaddon/proration.go \
        services/marketplace-api/internal/billing/appaddon/proration_test.go
git commit -m "feat(appaddon): pure proration math for co-termination with Pro renewal"
```

---

## Task 6: Add-on purchase handler — Stripe invoice

**Files:**
- Create: `services/marketplace-api/internal/billing/appaddon/handler.go`
- Create: `services/marketplace-api/internal/billing/appaddon/handler_test.go`

**Spec references:** §3.4 + §14.2 (attestation captured at purchase).

- [ ] **Step 1: Failing test — Pro store with add-on flag unset returns hosted URL**

```go
//go:build integration

func TestPurchaseAppAddOn_CreatesStripeInvoice(t *testing.T) {
    suite := inttest.NewSuite(t)
    tenantID, storeID := suite.SeedStore(subscription.StatusActive, subscription.PlanPro)
    suite.SetProAnnualRenewalAt(storeID, time.Now().Add(183*24*time.Hour))

    body := map[string]any{
        "apple_4_2_6_acknowledged": true,
        "attestation_text": "I acknowledge Apple 4.2.6…",
    }
    resp := suite.AdminPOST(tenantID, storeID,
        "/admin/stores/"+storeID.String()+"/subscription/add-on/white-label-app", body)
    require.Equal(t, 201, resp.Code)

    var out struct{ HostedInvoiceURL string `json:"hosted_invoice_url"`; StripeInvoiceID string `json:"stripe_invoice_id"` }
    suite.DecodeJSON(resp, &out)
    require.NotEmpty(t, out.HostedInvoiceURL)
    require.NotEmpty(t, out.StripeInvoiceID)

    // The Stripe fake was invoked with the right amount (~$3197).
    inv := suite.FakeStripe.LastInvoice()
    require.InDelta(t, 319_733, inv.AmountDue, 2)
    require.Equal(t, "usd", inv.Currency)

    // has_white_label_app_add_on stays false until invoice.paid fires.
    sub := suite.LoadSubscription(storeID)
    require.False(t, sub.HasWhiteLabelAppAddOn)
}
```

- [ ] **Step 2: Failing test — non-Pro rejected 403**

```go
func TestPurchaseAppAddOn_StudioRejected(t *testing.T) {
    suite := inttest.NewSuite(t)
    tenantID, storeID := suite.SeedStore(subscription.StatusActive, subscription.PlanStudio)
    resp := suite.AdminPOST(tenantID, storeID,
        "/admin/stores/"+storeID.String()+"/subscription/add-on/white-label-app",
        map[string]any{"apple_4_2_6_acknowledged": true, "attestation_text": "x"})
    require.Equal(t, 403, resp.Code)
    require.Contains(t, resp.Body.String(), "pro_plan_required")
}
```

- [ ] **Step 3: Failing test — missing attestation rejected**

```go
func TestPurchaseAppAddOn_MissingAttestationRejected(t *testing.T) {
    suite := inttest.NewSuite(t)
    tenantID, storeID := suite.SeedStore(subscription.StatusActive, subscription.PlanPro)
    resp := suite.AdminPOST(tenantID, storeID,
        "/admin/stores/"+storeID.String()+"/subscription/add-on/white-label-app",
        map[string]any{"apple_4_2_6_acknowledged": false})
    require.Equal(t, 400, resp.Code)
    require.Contains(t, resp.Body.String(), "apple_4_2_6_ack_required")
}
```

- [ ] **Step 4: Run — expect FAIL**

- [ ] **Step 5: Write `handler.go`**

```go
package appaddon

import (
    "net/http"

    "github.com/gin-gonic/gin"

    "github.com/tesserix/marketplace-api/internal/billing/stripeclient"
    "github.com/tesserix/marketplace-api/internal/subscription"
)

type Handler struct {
    db     *gorm.DB
    stripe stripeclient.InvoiceAPI
}

type PurchaseRequest struct {
    Apple426Acknowledged bool   `json:"apple_4_2_6_acknowledged"`
    AttestationText      string `json:"attestation_text"`
}

type PurchaseResponse struct {
    HostedInvoiceURL string `json:"hosted_invoice_url"`
    StripeInvoiceID  string `json:"stripe_invoice_id"`
    AmountDueCents   int64  `json:"amount_due_cents"`
    Currency         string `json:"currency"`
}

// POST /admin/stores/:storeId/subscription/add-on/white-label-app
func (h *Handler) Purchase(c *gin.Context) {
    var req PurchaseRequest
    if err := c.BindJSON(&req); err != nil { c.JSON(400, gin.H{"error":"invalid_body"}); return }
    if !req.Apple426Acknowledged {
        c.JSON(400, gin.H{"error":"apple_4_2_6_ack_required"}); return
    }

    sub, err := h.loadSubscription(c)
    if err != nil { c.JSON(500, gin.H{"error":"subscription_lookup_failed"}); return }
    if sub.Plan != subscription.PlanPro {
        c.JSON(403, gin.H{"error":"pro_plan_required"}); return
    }
    if sub.HasWhiteLabelAppAddOn {
        c.JSON(409, gin.H{"error":"already_active"}); return
    }

    amountCents := ProrationCents(time.Now().UTC(), sub.CurrentPeriodEnd)

    inv, err := h.stripe.CreateInvoice(c.Request.Context(), stripeclient.InvoiceInput{
        Customer: sub.StripeCustomerID,
        Currency: "usd",
        LineItems: []stripeclient.LineItem{{
            Description: "White-label app add-on (prorated + $2000 setup)",
            AmountCents: amountCents,
        }},
        Metadata: map[string]string{
            "tenant_id":   sub.TenantID.String(),
            "store_id":    sub.StoreID.String(),
            "kind":        "white_label_app_add_on",
        },
        AutoAdvance: true,
    })
    if err != nil {
        c.JSON(500, gin.H{"error":"stripe_invoice_failed"}); return
    }

    // Record attestation NOW (purchase intent + Apple 4.2.6 ack). The
    // has_white_label_app_add_on flip waits for invoice.paid (Task 7).
    _, err = attestations.Record(c.Request.Context(), h.db, attestations.Input{
        TenantID: sub.TenantID, StoreID: sub.StoreID, SubscriptionID: sub.ID,
        AttestationType: attestations.TypeApple426,
        AttestedByUserID: uuid.MustParse(c.GetString("user_id")),
        AttestationText: req.AttestationText,
        IPAddress: c.ClientIP(), UserAgent: c.GetHeader("User-Agent"),
        StripeInvoiceID: inv.ID,
    })
    if err != nil {
        // Attestation failure shouldn't leave a ghost invoice; void Stripe invoice.
        _ = h.stripe.VoidInvoice(c.Request.Context(), inv.ID)
        c.JSON(500, gin.H{"error":"attestation_record_failed"}); return
    }

    c.JSON(201, PurchaseResponse{
        HostedInvoiceURL: inv.HostedInvoiceURL,
        StripeInvoiceID:  inv.ID,
        AmountDueCents:   amountCents,
        Currency:         "usd",
    })
}
```

- [ ] **Step 6: Register route in `handlers/admin/routes.go`**

```go
addon := &appaddon.Handler{db: deps.DB, stripe: deps.StripeInvoiceAPI}
storeRoute.POST("/subscription/add-on/white-label-app", addon.Purchase)
```

- [ ] **Step 7: Run tests — expect PASS**

- [ ] **Step 8: Commit**

```bash
git add services/marketplace-api/internal/billing/appaddon/handler.go \
        services/marketplace-api/internal/billing/appaddon/handler_test.go \
        services/marketplace-api/internal/handlers/admin/routes.go
git commit -m "feat(appaddon): Pro-gated add-on purchase endpoint with Stripe invoice + attestation"
```

---

## Task 7: `invoice.paid` webhook extension

**Files:**
- Create: `services/marketplace-api/internal/billing/appaddon/webhook.go`
- Create: `services/marketplace-api/internal/billing/appaddon/webhook_test.go`
- Modify: `services/marketplace-api/internal/billing/dispatch/dispatcher.go`

**Spec references:** §3.4 (co-termination).

- [ ] **Step 1: Failing test — invoice.paid with kind=white_label_app_add_on flips flag**

```go
func TestWebhook_InvoicePaidForAddOn_FlipsFlag(t *testing.T) {
    db := testdb.NewDB(t, "store_subscriptions", "app_contract_attestations")
    em := audit.NewEmitter(audit.NewRecorderForTesting())

    tenantID, storeID := uuid.New(), uuid.New()
    require.NoError(t, db.Create(&subscription.StoreSubscription{
        TenantID: tenantID, StoreID: storeID, StripeCustomerID: "cus_x",
        Plan: subscription.PlanPro, Status: subscription.StatusActive,
        HasWhiteLabelAppAddOn: false,
    }).Error)

    raw := []byte(`{
      "id":"evt_1","type":"invoice.paid",
      "data":{"object":{
        "id":"in_addon","customer":"cus_x","status":"paid","amount_paid":319733,
        "metadata":{
          "kind":"white_label_app_add_on",
          "tenant_id":"` + tenantID.String() + `",
          "store_id":"` + storeID.String() + `"
        }
      }}
    }`)

    d := dispatch.NewWithAddOnHandler(em)
    require.NoError(t, d.Dispatch(context.Background(), db, webhookevents.StripeWebhookEvent{
        EventID: "evt_1", EventType: "invoice.paid", Payload: raw,
    }))

    var sub subscription.StoreSubscription
    require.NoError(t, db.Where("store_id=?", storeID).First(&sub).Error)
    require.True(t, sub.HasWhiteLabelAppAddOn)
}
```

- [ ] **Step 2: Failing test — replay is idempotent (advisory lock, CAS on flag)**

```go
func TestWebhook_InvoicePaidForAddOn_IdempotentReplay(t *testing.T) {
    // Two concurrent invoke() calls with same event_id must leave has=true, no error.
}
```

- [ ] **Step 3: Run — expect FAIL**

- [ ] **Step 4: Write `webhook.go`**

```go
package appaddon

// handleInvoicePaidForAppAddOn is registered after the generic invoice.paid
// handler in dispatch/dispatcher.go. It no-ops unless the invoice metadata
// carries kind=white_label_app_add_on.
func HandleInvoicePaidForAppAddOn(
    ctx context.Context, tx *gorm.DB, emitter *audit.Emitter, raw []byte,
) error {
    var e struct {
        Data struct {
            Object struct {
                ID       string            `json:"id"`
                Customer string            `json:"customer"`
                Metadata map[string]string `json:"metadata"`
            } `json:"object"`
        } `json:"data"`
    }
    if err := json.Unmarshal(raw, &e); err != nil { return err }
    obj := e.Data.Object
    if obj.Metadata["kind"] != "white_label_app_add_on" {
        return nil // not our invoice
    }
    storeID, err := uuid.Parse(obj.Metadata["store_id"])
    if err != nil { return fmt.Errorf("addon webhook: store_id: %w", err) }

    return subscription.WithAdvisoryLock(ctx, tx, storeID, func(tx *gorm.DB) error {
        res := tx.Exec(`
            UPDATE store_subscriptions
            SET has_white_label_app_add_on = TRUE, updated_at = now()
            WHERE store_id = ?
              AND plan = 'pro'
              AND has_white_label_app_add_on = FALSE`, storeID)
        if res.Error != nil { return res.Error }
        // RowsAffected == 0 is fine: replay; already flipped.
        return nil
    })
}
```

- [ ] **Step 5: Register handler in P2 dispatcher**

Modify `dispatch/dispatcher.go` `handleInvoicePaid` to call `appaddon.HandleInvoicePaidForAppAddOn` **before or after** the existing `payment_action_required → active` transition. Both are safe together: the add-on handler short-circuits when metadata.kind is absent. Use a slice of sub-handlers for clarity:

```go
for _, sub := range d.invoicePaidSubs {
    if err := sub(ctx, tx, d.emitter, raw); err != nil { return err }
}
```

where `d.invoicePaidSubs = []subhandler{existing, appaddon.HandleInvoicePaidForAppAddOn}`.

- [ ] **Step 6: Run tests — expect PASS**

- [ ] **Step 7: Commit**

```bash
git add services/marketplace-api/internal/billing/appaddon/webhook.go \
        services/marketplace-api/internal/billing/appaddon/webhook_test.go \
        services/marketplace-api/internal/billing/dispatch/dispatcher.go
git commit -m "feat(appaddon): invoice.paid webhook flips has_white_label_app_add_on"
```

---

## Task 8: Apple ASC + Google Play + Firebase client wrappers

**Files:**
- Create: `services/marketplace-api/internal/whitelabel/apple/client.go`
- Create: `services/marketplace-api/internal/whitelabel/apple/client_test.go`
- Create: `services/marketplace-api/internal/whitelabel/googleplay/client.go`
- Create: `services/marketplace-api/internal/whitelabel/googleplay/client_test.go`
- Create: `services/marketplace-api/internal/whitelabel/firebase/client.go`
- Create: `services/marketplace-api/internal/whitelabel/firebase/client_test.go`

**Spec references:** §13.5 (day 30 blocks downloads via Apple/Google; day 60 pull; day 90 Firebase delete).

- [ ] **Step 1: Failing tests — Apple client JWT signs with ES256**

```go
func TestAppleClient_JWT_HasES256Header(t *testing.T) {
    p8 := testP8Fixture(t)
    tok, err := apple.SignJWT(p8, "issuer-abc", "key-xyz")
    require.NoError(t, err)

    // Decode the JWS header.
    obj, err := jose.ParseSigned(tok)
    require.NoError(t, err)
    require.Equal(t, jose.ES256, obj.Signatures[0].Protected.Algorithm)
    require.Equal(t, "key-xyz", obj.Signatures[0].Protected.KeyID)
}
```

- [ ] **Step 2: Failing tests — Apple client `BlockDownloads` calls the right endpoint**

```go
func TestApple_BlockDownloads_CallsAvailabilityEndpoint(t *testing.T) {
    fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        require.Equal(t, "PATCH", r.Method)
        require.Contains(t, r.URL.Path, "/v1/apps/")
        require.Contains(t, r.URL.Path, "/availability")
        w.WriteHeader(204)
    }))
    defer fake.Close()

    cli := apple.New(apple.Config{BaseURL: fake.URL, Signer: stubSigner()})
    require.NoError(t, cli.BlockDownloads(context.Background(), "app-id-1"))
}
```

- [ ] **Step 3: Failing tests — Google Play + Firebase analogs (same shape)**

- [ ] **Step 4: Run — expect FAIL**

- [ ] **Step 5: Write `apple/client.go`**

```go
package apple

import (
    "context"
    "crypto/ecdsa"
    "crypto/x509"
    "encoding/pem"
    "encoding/json"
    "fmt"
    "net/http"
    "time"

    "github.com/go-jose/go-jose/v4"
    "github.com/go-jose/go-jose/v4/jwt"
)

// SignJWT produces the short-lived App Store Connect API JWT (ES256, 20-min TTL).
func SignJWT(p8PEM []byte, issuerID, keyID string) (string, error) {
    block, _ := pem.Decode(p8PEM)
    if block == nil { return "", fmt.Errorf("apple: no PEM block") }
    parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
    if err != nil { return "", err }
    priv, ok := parsed.(*ecdsa.PrivateKey)
    if !ok { return "", fmt.Errorf("apple: expected ECDSA key") }

    sig, err := jose.NewSigner(
        jose.SigningKey{Algorithm: jose.ES256, Key: priv},
        (&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", keyID),
    )
    if err != nil { return "", err }

    claims := jwt.Claims{
        Issuer: issuerID,
        Audience: jwt.Audience{"appstoreconnect-v1"},
        Expiry: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
    }
    return jwt.Signed(sig).Claims(claims).CompactSerialize()
}

type Client struct { base string; http *http.Client; signer Signer }

// BlockDownloads marks the app "not available" in all territories.
// Used at day 30 of the §13.5 teardown.
func (c *Client) BlockDownloads(ctx context.Context, appleAppID string) error { … }

// PullApp removes the app listing (soft-unpublish). Used at day 60.
func (c *Client) PullApp(ctx context.Context, appleAppID string) error { … }
```

- [ ] **Step 6: Write `googleplay/client.go` using `golang.org/x/oauth2/google` + direct HTTPS to `https://androidpublisher.googleapis.com/androidpublisher/v3/applications/{packageName}/edits`**

- [ ] **Step 7: Write `firebase/client.go` using `firebase.google.com/go/v4`. Two operations: `ArchiveProject(ctx, projectID)` (marks IAM read-only via a service-account policy flip) and `DeleteProject(ctx, projectID)`. Per-tenant Firebase project IDs live on `white_label_app_lifecycle.firebase_project_id`.**

- [ ] **Step 8: Run tests — expect PASS**

- [ ] **Step 9: Commit**

```bash
git add services/marketplace-api/internal/whitelabel/apple/ \
        services/marketplace-api/internal/whitelabel/googleplay/ \
        services/marketplace-api/internal/whitelabel/firebase/
git commit -m "feat(whitelabel): Apple ASC + Google Play + Firebase admin client wrappers"
```

---

## Task 9: `pro_app_cancelled` consumer seeds lifecycle row

**Files:**
- Create: `services/marketplace-api/internal/whitelabel/lifecycle/pro_app_cancelled_consumer.go`
- Create: `services/marketplace-api/internal/whitelabel/lifecycle/pro_app_cancelled_consumer_test.go`

**Spec references:** §13.5 + §15.5.

- [ ] **Step 1: Failing test — graceful 60-day path**

```go
func TestConsumer_SeedsSunsetScheduledRow_GracefulPath(t *testing.T) {
    db := testdb.NewDB(t, "white_label_app_lifecycle")
    c := lifecycle.NewProAppCancelledConsumer(db)
    storeID := uuid.New()

    require.NoError(t, c.Handle(context.Background(), lifecycle.ProAppCancelledEvent{
        TenantID: uuid.New(), StoreID: storeID,
        MerchantInitiatedImmediate: false,
    }))

    var row lifecycle.Row
    require.NoError(t, db.Where("store_id=?", storeID).First(&row).Error)
    require.Equal(t, lifecycle.StatusSunsetScheduled, row.Status)
    require.WithinDuration(t, time.Now(), row.ScheduledAt, 5*time.Second)
}
```

- [ ] **Step 2: Failing test — merchant-initiated immediate pull compresses to 7 days**

```go
func TestConsumer_MerchantInitiatedImmediate_CompressesTo7Days(t *testing.T) {
    db := testdb.NewDB(t, "white_label_app_lifecycle")
    c := lifecycle.NewProAppCancelledConsumer(db)
    storeID := uuid.New()

    require.NoError(t, c.Handle(context.Background(), lifecycle.ProAppCancelledEvent{
        TenantID: uuid.New(), StoreID: storeID,
        MerchantInitiatedImmediate: true,
    }))

    var row lifecycle.Row
    require.NoError(t, db.Where("store_id=?", storeID).First(&row).Error)
    // scheduled_at rolled back by 53 days so the advancer sees "53 days elapsed"
    // immediately and transitions within the 7-day window.
    require.WithinDuration(t, time.Now().Add(-53*24*time.Hour), row.ScheduledAt, 1*time.Minute)
    require.True(t, row.MerchantInitiated)
}
```

- [ ] **Step 3: Run — expect FAIL**

- [ ] **Step 4: Write `pro_app_cancelled_consumer.go`**

The consumer is a pub/sub handler registered in `main.go`. On merchant-initiated immediate pull, `scheduled_at` is backdated by 53 days so that the advancer's clock-based step matching treats day-60 + day-90 as "already due" within 7 days.

- [ ] **Step 5: Run tests — expect PASS**

- [ ] **Step 6: Commit**

```bash
git add services/marketplace-api/internal/whitelabel/lifecycle/pro_app_cancelled_consumer.go \
        services/marketplace-api/internal/whitelabel/lifecycle/pro_app_cancelled_consumer_test.go
git commit -m "feat(lifecycle): consume subscription.pro_app_cancelled + seed sunset row"
```

---

## Task 10: Lifecycle advancer — state table + per-status handlers

**Files:**
- Create: `services/marketplace-api/internal/whitelabel/lifecycle/advancer.go`
- Create: `services/marketplace-api/internal/whitelabel/lifecycle/advancer_test.go`

**Spec references:** §13.5 (day 0/7/30/60/90 timeline), §18.9 (day-90 credential purge).

- [ ] **Step 1: Failing test — day-30 advances sunset_scheduled → downloads_blocked + calls Apple + Google**

```go
func TestAdvancer_Day30_BlocksDownloads(t *testing.T) {
    db := testdb.NewDB(t, "white_label_app_lifecycle", "store_subscriptions")
    appleCli := &apple.FakeClient{}
    googleCli := &googleplay.FakeClient{}
    firebaseCli := &firebase.FakeClient{}
    credsSvc := appcreds.NewService(testCredsConfig(t))

    adv := lifecycle.NewAdvancer(lifecycle.Config{
        DB: db, Apple: appleCli, Google: googleCli, Firebase: firebaseCli,
        Creds: credsSvc, Clock: func() time.Time { return time.Now() },
    })

    storeID := uuid.New()
    // Seed a row "30 days old" in sunset_scheduled.
    require.NoError(t, db.Create(&lifecycle.Row{
        StoreID: storeID, Status: lifecycle.StatusSunsetScheduled,
        ScheduledAt: time.Now().Add(-30 * 24 * time.Hour),
        AppleAppID: "apple-1", GooglePackage: "com.store.x",
        FirebaseProjectID: "firebase-proj-1",
    }).Error)

    require.NoError(t, adv.AdvanceDue(context.Background()))

    var row lifecycle.Row
    require.NoError(t, db.Where("store_id=?", storeID).First(&row).Error)
    require.Equal(t, lifecycle.StatusDownloadsBlocked, row.Status)
    require.Equal(t, 1, appleCli.BlockDownloadsCallCount)
    require.Equal(t, 1, googleCli.BlockDownloadsCallCount)
}
```

- [ ] **Step 2: Failing test — day-60 pulls apps + archives Firebase**

```go
func TestAdvancer_Day60_PullsAndArchives(t *testing.T) {
    // Seed a row already at downloads_blocked with scheduled_at = now-60d.
    // Assert transition → pulled → firebase_archived, Apple.PullApp + Google.PullApp + Firebase.ArchiveProject all called.
}
```

- [ ] **Step 3: Failing test — day-90 deletes Firebase + purges ALL four credentials (success criterion 52)**

```go
func TestAdvancer_Day90_PurgesAllFourCredentials(t *testing.T) {
    db := testdb.NewDB(t, "white_label_app_lifecycle")
    fakeSM := appcreds.NewFakeSM()
    creds := appcreds.NewService(appcreds.Config{ProjectID:"p", SM: fakeSM, Emitter: audit.NewEmitter(audit.NewRecorderForTesting())})

    tenantID, storeID := uuid.New(), uuid.New()
    for _, ct := range appcreds.AllCredTypes() {
        require.NoError(t, creds.Store(context.Background(),
            appcreds.StoreInput{TenantID: tenantID, CredType: ct, Payload: []byte("x")}))
    }

    firebaseCli := &firebase.FakeClient{}
    adv := lifecycle.NewAdvancer(lifecycle.Config{
        DB: db, Apple: &apple.FakeClient{}, Google: &googleplay.FakeClient{},
        Firebase: firebaseCli, Creds: creds,
        Clock: time.Now,
    })

    require.NoError(t, db.Create(&lifecycle.Row{
        TenantID: tenantID, StoreID: storeID, Status: lifecycle.StatusFirebaseArchived,
        ScheduledAt: time.Now().Add(-90 * 24 * time.Hour),
        FirebaseProjectID: "fb-proj-1",
    }).Error)

    require.NoError(t, adv.AdvanceDue(context.Background()))

    var row lifecycle.Row
    require.NoError(t, db.Where("store_id=?", storeID).First(&row).Error)
    require.Equal(t, lifecycle.StatusCredentialsPurged, row.Status)

    // All four credentials gone from Secret Manager.
    for _, ct := range appcreds.AllCredTypes() {
        _, err := creds.Load(context.Background(),
            appcreds.LoadInput{TenantID: tenantID, CredType: ct, Actor: "test"})
        require.ErrorIs(t, err, appcreds.ErrNotFound)
    }
    require.Equal(t, 1, firebaseCli.DeleteProjectCallCount)
}
```

- [ ] **Step 4: Failing test — day-7 emits banner event (no state change)**

```go
func TestAdvancer_Day7_EmitsBannerOnly(t *testing.T) {
    // Row stays at sunset_scheduled; audit event "white_label_app.banner_deployed" emitted.
}
```

- [ ] **Step 5: Run — expect FAIL**

- [ ] **Step 6: Write `advancer.go`**

```go
package lifecycle

type Advancer struct {
    db       *gorm.DB
    apple    apple.ClientAPI
    google   googleplay.ClientAPI
    firebase firebase.ClientAPI
    creds    *appcreds.Service
    emitter  *audit.Emitter
    metrics  *metrics.Counter
    clock    func() time.Time
}

// AdvanceDue scans rows with next_action_at <= now() and advances each.
// Idempotent: Apple/Google/Firebase calls tolerate re-application;
// Secret Manager deletes swallow NotFound.
func (a *Advancer) AdvanceDue(ctx context.Context) error {
    var rows []Row
    if err := a.db.Where("next_action_at <= ?", a.clock()).Find(&rows).Error; err != nil {
        return err
    }
    for _, r := range rows {
        if err := a.advanceOne(ctx, r); err != nil {
            // Log and continue — one bad row must not stall others.
            log.WithError(err).WithField("store_id", r.StoreID).Error("lifecycle advance failed")
        }
    }
    return nil
}

func (a *Advancer) advanceOne(ctx context.Context, r Row) error {
    daysElapsed := int(a.clock().Sub(r.ScheduledAt).Hours() / 24)
    step, ok := nextStepFor(r.Status, daysElapsed)
    if !ok { return nil } // no step due yet

    if err := step.Action(ctx, a, r); err != nil { return err }

    return a.db.Transaction(func(tx *gorm.DB) error {
        if err := tx.Model(&Row{}).Where("store_id=?", r.StoreID).Updates(map[string]any{
            "status": step.To, "updated_at": a.clock(),
            "next_action_at": nextActionAt(step.To, r.ScheduledAt),
        }).Error; err != nil { return err }

        a.emitter.EmitLifecycleTransition(ctx, audit.LifecycleTransition{
            TenantID: r.TenantID, StoreID: r.StoreID,
            From: string(r.Status), To: string(step.To),
            Actor: "system:cron:lifecycle",
        })
        a.metrics.WithLabels(string(r.Status), string(step.To)).Inc()
        return nil
    })
}

// Per-status action functions — each idempotent against upstream.
func blockDownloads(ctx context.Context, a *Advancer, r Row) error {
    if err := a.apple.BlockDownloads(ctx, r.AppleAppID); err != nil { return err }
    return a.google.BlockDownloads(ctx, r.GooglePackage)
}
func pullApps(ctx context.Context, a *Advancer, r Row) error { … }
func archiveFirebase(ctx context.Context, a *Advancer, r Row) error { … }
func deleteFirebaseAndPurge(ctx context.Context, a *Advancer, r Row) error {
    if err := a.firebase.DeleteProject(ctx, r.FirebaseProjectID); err != nil { return err }
    return a.creds.PurgeAll(ctx, r.TenantID, "system:cron:day_90")
}
func emitBannerEvent(ctx context.Context, a *Advancer, r Row) error {
    a.emitter.EmitBusinessEvent(ctx, audit.BusinessEvent{
        Kind: "white_label_app.banner_deployed", TenantID: r.TenantID, StoreID: r.StoreID,
    })
    return nil
}
```

The credentials-upload endpoints from Task 4 call `appcreds.Store`; the advancer calls `appcreds.PurgeAll`. Both go through the one package — no other module touches Secret Manager.

- [ ] **Step 7: Run tests — expect PASS**

- [ ] **Step 8: Commit**

```bash
git add services/marketplace-api/internal/whitelabel/lifecycle/advancer.go \
        services/marketplace-api/internal/whitelabel/lifecycle/advancer_test.go
git commit -m "feat(lifecycle): advancer executes day 7/30/60/90 actions + purges credentials"
```

---

## Task 11: Cron scheduler + main wiring

**Files:**
- Create: `services/marketplace-api/internal/whitelabel/lifecycle/scheduler.go`
- Create: `services/marketplace-api/internal/whitelabel/lifecycle/scheduler_test.go`
- Modify: `services/marketplace-api/cmd/marketplace-api/main.go`

**Spec references:** §13.5 (daily cadence).

- [ ] **Step 1: Failing test — scheduler invokes advancer once per tick**

```go
func TestScheduler_InvokesAdvancerOnTick(t *testing.T) {
    fakeAdv := &fakeAdvancer{}
    sched := lifecycle.NewScheduler(fakeAdv, "* * * * *") // every minute for test
    ctx, cancel := context.WithCancel(context.Background())
    go sched.Run(ctx)
    time.Sleep(65 * time.Second)
    cancel()
    require.GreaterOrEqual(t, fakeAdv.Count(), 1)
}
```

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Write `scheduler.go`**

```go
package lifecycle

import (
    "context"

    "github.com/robfig/cron/v3"
)

type Scheduler struct {
    adv      AdvancerAPI
    cronSpec string
    cron     *cron.Cron
}

func NewScheduler(adv AdvancerAPI, spec string) *Scheduler {
    return &Scheduler{adv: adv, cronSpec: spec, cron: cron.New(cron.WithLocation(time.UTC))}
}

func (s *Scheduler) Run(ctx context.Context) error {
    _, err := s.cron.AddFunc(s.cronSpec, func() {
        if err := s.adv.AdvanceDue(ctx); err != nil {
            log.WithError(err).Error("lifecycle advance tick failed")
        }
    })
    if err != nil { return err }
    s.cron.Start()
    <-ctx.Done()
    s.cron.Stop()
    return nil
}
```

Production cron spec: `"0 5 * * *"` (05:00 UTC daily). Configured via env var `WHITE_LABEL_LIFECYCLE_CRON`.

- [ ] **Step 4: Wire `main.go`**

```go
// After constructing appcreds, apple/google/firebase clients, audit emitter:
lifecycleMetrics := whitelabelmetrics.NewCounter(registry, "white_label_app_lifecycle_transition_total")
adv := lifecycle.NewAdvancer(lifecycle.Config{
    DB: db, Apple: appleCli, Google: googleCli, Firebase: firebaseCli,
    Creds: credsSvc, Emitter: auditEmitter, Metrics: lifecycleMetrics,
    Clock: time.Now,
})
sched := lifecycle.NewScheduler(adv, cfg.WhiteLabelLifecycleCron)
go func() {
    if err := sched.Run(ctx); err != nil {
        log.WithError(err).Fatal("lifecycle scheduler crashed")
    }
}()
```

- [ ] **Step 5: Run tests — expect PASS**

- [ ] **Step 6: Commit**

```bash
git add services/marketplace-api/internal/whitelabel/lifecycle/scheduler.go \
        services/marketplace-api/internal/whitelabel/lifecycle/scheduler_test.go \
        services/marketplace-api/cmd/marketplace-api/main.go
git commit -m "feat(lifecycle): daily 05:00 UTC cron drives white-label app sunset sequence"
```

---

## Task 12: Observability counters + Secret-Manager import lint

**Files:**
- Create: `services/marketplace-api/internal/whitelabel/metrics/metrics.go`
- Create: `docs/ops/white-label-app-iam.md`
- Modify: `.github/workflows/ci.yml` — add import-lint step.

- [ ] **Step 1: Define the two counters**

```go
package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
    LifecycleTransition = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "white_label_app_lifecycle_transition_total",
            Help: "Count of white-label app lifecycle transitions, labeled by from/to status.",
        },
        []string{"from", "to"},
    )
    CredentialAccessed = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "white_label_app_credential_accessed_total",
            Help: "Count of white-label credential reads, labeled by credential type.",
        },
        []string{"type"},
    )
)

func MustRegister(reg prometheus.Registerer) {
    reg.MustRegister(LifecycleTransition, CredentialAccessed)
}
```

- [ ] **Step 2: CI import-lint step**

Add to `ci.yml` `test` job, before `go test`:

```yaml
- name: Enforce Secret Manager choke-point
  run: |
    set -euo pipefail
    offenders=$(grep -RlE '"cloud\.google\.com/go/secretmanager' \
      services/marketplace-api/internal services/marketplace-api/cmd \
      --include='*.go' | grep -v '^services/marketplace-api/internal/billing/appcreds/' || true)
    if [ -n "$offenders" ]; then
      echo "ERROR: Secret Manager imports outside internal/billing/appcreds/:"
      echo "$offenders"
      exit 1
    fi
```

- [ ] **Step 3: Write `docs/ops/white-label-app-iam.md`**

One page. Lists:
- CI/CD SA (`tesserix-ci@tesserix-prod.iam.gserviceaccount.com`): `roles/secretmanager.secretAccessor` scoped to `secrets/merchant_*` via resource-name condition.
- Eng staff group (`gcp-eng-whitelabel@tesserix.com`, ≤2 people): same role, audit-logged, documented rotation procedure.
- Implementation: `tesserix-infra/terraform/03-secrets/whitelabel-app-iam.tf` (Terraform handoff — not in this plan).

- [ ] **Step 4: Run tests + CI locally**

```bash
cd services/marketplace-api
go build ./...
go test -tags=integration ./...
bash -c 'offenders=$(grep -RlE "cloud.google.com/go/secretmanager" internal cmd --include="*.go" | grep -v "internal/billing/appcreds/" || true); test -z "$offenders"'
```

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/whitelabel/metrics/metrics.go \
        docs/ops/white-label-app-iam.md \
        .github/workflows/ci.yml
git commit -m "feat(observability): white-label lifecycle + credential counters; CI enforces appcreds choke-point"
```

---

## Final verification

- [ ] `go build ./...` clean.
- [ ] `go test -tags=integration ./...` all green.
- [ ] `app_contract_attestations` table present; integration test proves UPDATE rejected by trigger AND DELETE rejected by role revoke.
- [ ] `internal/billing/appcreds/` is the **only** package importing `cloud.google.com/go/secretmanager` — CI lint enforces.
- [ ] Every `appcreds.{Store,Load,Delete}` emits `audit.EmitCredentialAccess` + increments `white_label_app_credential_accessed_total`.
- [ ] Cross-tenant integration test: `GET /admin/stores/:storeOfB/app-credentials/apple` by tenant A returns **404** (not 403 — 403 leaks "credential exists"). Implementation must ensure `appcreds.Load` with wrong tenant returns `ErrNotFound`, rendered as 404.
- [ ] Security test: DELETE on `app_contract_attestations` as the `marketplace_api_rw` role raises `permission denied` (role revoke) — **not** "append-only" (trigger) — proving both guards live independently.
- [ ] Security test: teardown day-90 integration test asserts all four Secret Manager paths (`apple-asc-api-key`, `apple-asc-issuer-id`, `apple-asc-key-id`, `google-play-service-account`) return NotFound after `AdvanceDue` runs — **success criterion #52**.
- [ ] Co-termination test: `ProrationCents(now, now + 183d) ≈ 319_733` cents (half-year remaining) — **success criterion #44**.
- [ ] Merchant-initiated immediate pull test: seed a sunset row with `MerchantInitiatedImmediate=true`; run `AdvanceDue` → row transitions through `downloads_blocked → pulled → firebase_archived → credentials_purged` within 7 simulated days.
- [ ] Webhook replay test: calling `HandleInvoicePaidForAppAddOn` twice with the same event leaves `has_white_label_app_add_on=true` and does not error.
- [ ] Apple 4.2.6 attestation is recorded at purchase time (in `app_contract_attestations`), not at `invoice.paid`, so a user who abandons Stripe Checkout still has an auditable record of their ack.
- [ ] `WhiteLabelAppStatus` lifecycle is orthogonal to `SubscriptionStatus` — no call site transitions the subscription state machine from this plan's code.
- [ ] Pro-gated: `POST /subscription/add-on/white-label-app` returns 403 for Starter/Studio/Trial plans.
- [ ] `handleInvoicePaidForAppAddOn` is registered after the generic `invoice.paid` handler so the `payment_action_required → active` transition in P3 runs first, then the add-on flag flip runs, then both commit in the same webhook transaction.
- [ ] Observability counters `white_label_app_lifecycle_transition_total{from,to}` and `white_label_app_credential_accessed_total{type}` are reachable via `/metrics`.

## What's now unlocked

- **P16** (admin frontend) can build:
  - Pro+App checkout flow (calls `POST /subscription/add-on/white-label-app`, displays hosted Stripe URL).
  - Credential upload forms (Apple `.p8` + Google service-account JSON) posting to the Task 4 endpoints.
  - Lifecycle-status display reading `white_label_app_lifecycle` (new `GET /admin/stores/:storeId/white-label-app/status` endpoint — small P16 addition).
- **P17** (observability) has two ready-made counter series to alert on:
  - `white_label_app_credential_accessed_total{type}` spikes → possible credential-dump attempt.
  - `white_label_app_lifecycle_transition_total{from="firebase_archived",to="credentials_purged"}` → day-90 completions; wire to a success notification for the CSM.
- **P11** (cancellation) — its `subscription.pro_app_cancelled` event now has a consumer that seeds the lifecycle row; P11 only needs to emit.
- **P2 invoice builder** — can render renewal invoices that bundle Pro + App at the combined rate, because `has_white_label_app_add_on=true` is now authoritative on the subscription row.
- **Terraform `tesserix-infra/terraform/03-secrets/whitelabel-app-iam.tf`** — the IAM binding doc (`docs/ops/white-label-app-iam.md`) is the spec for that Terraform module.

## Execution handoff

Plan complete. This plan is the server-side foundation for the white-label mobile app add-on. Execute it after P1/P2/P3/P11 have landed. Dependencies on P11 (the `subscription.pro_app_cancelled` event emitter) are the only thing that prevents Task 9 from being merged standalone — Tasks 1–8 + 10–12 can be implemented and merged before P11 completes; Task 9's consumer wiring is the final merge gate.

Recommended execution: **superpowers:subagent-driven-development** (preferred — per-task parallelization is safe because tasks 1–8 touch disjoint packages). Tasks 9–11 depend on 10 and must serialize.

After merge, hand off to:
- **Terraform** (`tesserix-infra/`) — implement `docs/ops/white-label-app-iam.md` as IAM bindings.
- **P16** — build the admin UI against the three endpoints from Tasks 4 + 6.
- **P17** — add dashboards + alerts for the two counters from Task 12.
