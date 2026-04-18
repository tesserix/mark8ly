# P8 — Geo-Pricing Anti-Arbitrage + HMAC-SHA256 IP Hashing + Self-Service Appeal Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire the flag-based geo-pricing anti-arbitrage check into subscription creation. When a PPP-tier subscription lands with a `card_country` or `ip_country` that points at a developed market, triangulate the three signals (Stripe card country + billing address country + Cloudflare `CF-IPCountry` header), write a `subscription_arbitrage_audit` row, set `arbitrage_flag = true` — and **never block the merchant**. Hash the raw IP with HMAC-SHA256 keyed by a rotating 256-bit secret in GCP Secret Manager; never store the raw address. Provide the merchant a self-service appeal endpoint so false positives clear themselves instead of piling up in the `billing-ops` queue.

**Architecture:** A new `internal/arbitrage` package owns the triangulation check, the HMAC client, the appeals handler, and the key-rotation cron. `arbitrage.Evaluate(ctx, input) -> Decision` is a pure function — given three country codes, the resolved plan tier, and the hashed IP, it produces either `NoFlag` or `Flag{MismatchReason}`. `arbitrage.Recorder` persists flagged rows into `subscription_arbitrage_audit` and toggles `store_subscriptions.arbitrage_flag`. The P2 webhook dispatcher calls `Recorder.RecordIfFlagged` from both `customer.subscription.created` and `checkout.session.completed` handlers, so triangulation runs once per subscription lifecycle entry. Key rotation lives in a separate `cmd/arbitrage-rotator` main that Cloud Scheduler pokes nightly; it consults Secret Manager's version metadata, creates a new version when the latest is ≥30 days old, and disables versions ≥61 days old (retained for in-flight correlation, then severed). A Gin handler at `POST /admin/stores/:storeId/arbitrage-appeal` lets merchants submit jurisdiction evidence that writes directly into the `reviewed_*` + `resolution` fields of the existing audit row — no new schema. The admin store-read path returns `arbitrage_flag` plus the latest audit row so the frontend can render the banner (frontend rendering itself is P16).

**Tech Stack:** Go 1.26, Gin, GORM, PostgreSQL (no new migrations; reuses P1's migration 042 `subscription_arbitrage_audit`), `cloud.google.com/go/secretmanager/apiv1` (via existing go-shared secret client if present, else direct), `crypto/hmac` + `crypto/sha256`, `go-shared/messaging` (for the `billing-ops` queue publish), existing `internal/audit` emitter, existing media/GCS upload path for the optional appeal document, Cloud Scheduler + Cloud Run Job for rotation.

**Spec:** [`docs/superpowers/specs/2026-04-17-subscription-model-design.md`](../specs/2026-04-17-subscription-model-design.md) — §4.1.3 (PPP disclosure policy; support escalation path to billing-ops), §18.8 (triangulation, HMAC-SHA256, 30d rotation + 31d overlap, flag-not-block, `ip_country` as durable join), §18.8.1 (self-service appeal form + 5-biz-day SLA + resolution enum), §28 success criterion #51.

**Depends on:**
- **P1** — `subscription_arbitrage_audit` table (migration 042), `arbitrage_flag` column on `store_subscriptions` (migration 038), `ip_hash` + `ip_country` columns, and `internal/arbitrage/models.go` with the `SubscriptionArbitrageAudit` GORM model. All present.
- **P2** — Webhook dispatcher (`internal/billing/dispatch/handlers.go`) runs this check inside the `customer.subscription.created` and `checkout.session.completed` handlers. The dispatcher already loads the subscription row by `stripe_customer_id`; we just add a post-load hook.
- **P3** — `statemachine.Transition` is **not** used for merchant-initiated appeal denials that result in re-invoicing at the developed-market tier. Those run through the normal renewal flow (invoice at new tier on `current_period_end`), which is not a subscription state change.

**Related plans:**
- **P11** (cancellation + save-offer) — the "refusal to pay → cancellation at next renewal" path delegates to P11's `cancel_at_period_end` mechanism; nothing new here.
- **P16** (admin UI) — consumes `arbitrage_flag` + latest audit row from the store-read payload we extend.
- **P17** (observability) — wires the `subscription.arbitrage_flagged` + `subscription.arbitrage_false_positive_cleared` counters we emit to dashboards + the >5× baseline alert. We publish the metrics here; alert wiring is P17.

---

## Scope Check

In scope:
1. Triangulation at subscription creation: extract `(card_country, billing_country, ip_country)`, compute mismatch, flag PPP subscriptions whose card or IP resolves to a developed market.
2. HMAC-SHA256 IP hashing with a 256-bit key from GCP Secret Manager at `/projects/tesserix-prod/secrets/arbitrage-ip-hmac-key`. Raw IP never persisted.
3. Nightly key-rotation Go job: checks secret age, writes a new version at ≥30 days, disables versions at ≥61 days. Overlap window = 31 days. Run as a Cloud Run Job triggered by Cloud Scheduler.
4. Insert one `subscription_arbitrage_audit` row per subscription-creation event where a mismatch is detected; toggle `store_subscriptions.arbitrage_flag` to `true`.
5. Self-service appeal endpoint `POST /admin/stores/:storeId/arbitrage-appeal` that populates `reviewed_by` / `reviewed_at` / `resolution` on the existing audit row (no new table) and enqueues a `billing-ops` review message.
6. Enforcement action flow — 14-day merchant notice, re-invoice at developed-market tier at next renewal, cancellation on refusal (delegated to P11), escalation on second-flag-after-resolution.
7. Quarterly cron that clears stale `resolution = 'ongoing'` flags under advisory lock (§18.8 concurrency rule).
8. Admin store-read payload exposes `arbitrage_flag` + the latest audit row so P16's banner has a data contract.
9. Observability counters: `subscription.arbitrage_flagged`, `subscription.arbitrage_false_positive_cleared`.
10. PII access logging: every read of `subscription_arbitrage_audit` emits an audit-service event; IAM enforced to `billing-ops` role.

Out of scope:
- Admin UI rendering (banner markup, appeal form) — **P16**.
- Cancellation mechanics when an appeal is denied and the merchant refuses the new tier — delegated to **P11**.
- Actual GCP Secret Manager resource creation + IAM binding (ops/Terraform task — this plan documents the required secret path + service-account role so infra can create them; code consumes by path).
- Cloud Scheduler + Cloud Run Job deployment manifests — Task 12 emits the job binary + an ArgoCD/K8s manifest stub, but the schedule + IAM binding land in the tesserix-k8s repo via a separate follow-up.
- Alert wiring for "counter spike >5× baseline" — **P17**.
- GeoIP database maintenance — we trust Cloudflare's `CF-IPCountry` header exclusively; we do not run a MaxMind lookup.
- Storefront/customer IP tracking — this is merchant-subscription only.

---

## File Structure

### Create

- `services/marketplace-api/internal/arbitrage/evaluator.go` — pure triangulation (`Evaluate(input) -> Decision`)
- `services/marketplace-api/internal/arbitrage/evaluator_test.go`
- `services/marketplace-api/internal/arbitrage/countries.go` — developed-vs-emerging country lookup table
- `services/marketplace-api/internal/arbitrage/countries_test.go`
- `services/marketplace-api/internal/arbitrage/hmac.go` — `Hasher` with versioned HMAC keys fetched from Secret Manager
- `services/marketplace-api/internal/arbitrage/hmac_test.go`
- `services/marketplace-api/internal/arbitrage/keyloader.go` — `KeyLoader` that reads `/projects/tesserix-prod/secrets/arbitrage-ip-hmac-key` and caches the latest 2 enabled versions (for cross-rotation read correlation)
- `services/marketplace-api/internal/arbitrage/keyloader_test.go`
- `services/marketplace-api/internal/arbitrage/ipcountry.go` — `CFIPCountryFromGin(c *gin.Context) (string, string)` — reads `CF-IPCountry` header + returns the raw IP (for hashing only; caller must not persist it)
- `services/marketplace-api/internal/arbitrage/ipcountry_test.go`
- `services/marketplace-api/internal/arbitrage/recorder.go` — `Recorder.RecordIfFlagged(ctx, tx, evt) -> error` writes the audit row + toggles `arbitrage_flag` + emits counter
- `services/marketplace-api/internal/arbitrage/recorder_test.go`
- `services/marketplace-api/internal/arbitrage/appeal.go` — service layer: `AppealService.Submit(ctx, input) -> error` updates the existing audit row's `reviewed_*` fields + publishes to `billing-ops` queue
- `services/marketplace-api/internal/arbitrage/appeal_test.go`
- `services/marketplace-api/internal/arbitrage/quarterly_cron.go` — `QuarterlyAudit.Run(ctx)` clears `ongoing` flags with advisory lock
- `services/marketplace-api/internal/arbitrage/quarterly_cron_test.go`
- `services/marketplace-api/internal/arbitrage/pii_access.go` — `LogPIIAccess(ctx, event)` wrapper that emits an audit-service event on every audit-row read
- `services/marketplace-api/internal/handlers/admin/arbitrage_appeal.go` — Gin handler for `POST /admin/stores/:storeId/arbitrage-appeal`
- `services/marketplace-api/internal/handlers/admin/arbitrage_appeal_test.go`
- `services/marketplace-api/cmd/arbitrage-rotator/main.go` — rotation binary run as Cloud Run Job nightly
- `services/marketplace-api/cmd/arbitrage-rotator/main_test.go`
- `services/marketplace-api/deploy/arbitrage-rotator.k8s.yaml` — Cloud Run Job / CronJob stub (documented; infra team wires IAM)

### Modify

- `services/marketplace-api/internal/billing/dispatch/handlers.go` (P2) — call `arbitrage.Recorder.RecordIfFlagged` from `handleCustomerSubscriptionCreated` and `handleCheckoutSessionCompleted`
- `services/marketplace-api/internal/handlers/admin/subscription.go` — extend `GetSubscription` response payload with `arbitrage_flag` + `latest_arbitrage_audit` (nullable)
- `services/marketplace-api/internal/handlers/admin/routes.go` — register the appeal endpoint
- `services/marketplace-api/cmd/marketplace-api/main.go` — wire `arbitrage.KeyLoader`, `arbitrage.Recorder`, `arbitrage.AppealService`, `arbitrage.QuarterlyAudit` into the dependency graph
- `services/marketplace-api/internal/arbitrage/models.go` (P1) — add `TableName()` check + helper constructors if missing

### Delete

- None.

---

## Task Sequence Overview

| # | Task | Depends on |
|---|---|---|
| 1 | Developed-vs-emerging country table (pure) + tests | — |
| 2 | `arbitrage.Evaluate` pure triangulation function + tests | 1 |
| 3 | `CF-IPCountry` extraction helper + test injection path | — |
| 4 | `KeyLoader` — Secret Manager client with version cache | — |
| 5 | `Hasher` — HMAC-SHA256 keyed by `KeyLoader.Latest()` | 4 |
| 6 | `Recorder.RecordIfFlagged` — persists audit row + toggles flag + counter | 2, 3, 5, P1 |
| 7 | Wire `Recorder` into P2 dispatcher at two webhook events | 6, P2 |
| 8 | `AppealService.Submit` — updates audit row + `billing-ops` publish | 6, P1 |
| 9 | `POST /admin/stores/:storeId/arbitrage-appeal` Gin handler | 8 |
| 10 | Extend `GetSubscription` payload with flag + latest audit | 6 |
| 11 | `QuarterlyAudit.Run` — clears ongoing flags under advisory lock | 6, P1 |
| 12 | `cmd/arbitrage-rotator/main.go` + K8s stub | 4 |
| 13 | `LogPIIAccess` wrapper on every audit-row read | 6, 8, 10 |
| 14 | End-to-end success-criterion #51 test | 4, 5 |
| 15 | Final scrub: grep raw-IP persistence + reader bypasses | all |

Each task is one atomic commit boundary.

---

## Reusable patterns

**A. HMAC construction** — one canonical call site.

```go
// CORRECT: crypto/hmac with SHA-256; prevents length-extension; the
// right primitive for keyed hashing.
mac := hmac.New(sha256.New, key)
_, _ = mac.Write([]byte(rawIP))
digest := mac.Sum(nil)
hexHash := hex.EncodeToString(digest) // 64 chars, fits VARCHAR(64)

// WRONG (and explicitly rejected in spec §18.8):
//   h := sha256.Sum256(append([]byte(salt), rawIP...))
// SHA-256 alone is not a MAC; concatenated-salt is vulnerable to
// length-extension and collisions on attacker-controlled input.
```

Every consumer of IP-hashing must call `arbitrage.Hasher.Hash(ctx, rawIP)`. No other file in the service may import `crypto/hmac` for IP work (Task 15 greps for this).

**B. `CF-IPCountry` is trusted** — we do **not** run a local GeoIP lookup. Cloudflare Tunnel terminates in front of Istio + Knative, so every request hits our handler with the header pre-populated. In tests, the header is injected via `gin.Context.Request.Header.Set("CF-IPCountry", "US")`. Missing header → `ip_country = "??"` → treated as `unknown` (no flag, because we can't triangulate without it; a separate observability signal counts header-missing events).

**C. Secret Manager version semantics** — we use native Secret Manager versioning, not a home-rolled `key_v1 / key_v2` scheme.

- **Writes** always use the `latest` alias (Hasher only hashes with current).
- **Reads** iterate `ListSecretVersions` for every enabled version ≤61 days old. A subscription row's `ip_hash` was produced by whatever key was `latest` at the time; correlating two rows from different rotation windows requires re-hashing one candidate under every retained key. Beyond 61 days, the older version is disabled, and correlation is intentionally severed (spec §18.8 accepted limitation).
- The `KeyLoader` caches `{latest, previous}` in memory with a 5-minute TTL so the hot path never hits Secret Manager per request.

**D. Flag, don't block** — the triangulation path returns a `Decision`, never an error. A `Flag` decision causes a write and a counter increment; it does **not** short-circuit subscription creation. The handler in P2 ignores `Recorder.RecordIfFlagged` errors when they are not storage errors (an evaluation failure must not break Stripe webhook idempotency).

**E. Advisory lock for quarterly audit** — same `subscription.WithAdvisoryLock(ctx, db, storeID, fn)` helper from P1. The quarterly cron wraps its per-row `UPDATE ... SET resolution = 'false_positive_cleared'` in an advisory lock over `store_id`, so a merchant-initiated appeal in flight cannot collide with a cron-driven clear.

**F. Audit-service PII access logging** — every read of `subscription_arbitrage_audit` (not `store_subscriptions`; only the audit table carries the residual PII) goes through `arbitrage.LogPIIAccess(ctx, Event{...})`. The wrapper emits a structured audit event before returning the row. In practice this means every exported function on `Recorder` and `AppealService` that returns audit data wraps its query in `LogPIIAccess`.

**G. Response envelope** — the appeal endpoint uses the standard `{data, meta, error}` envelope (per project API convention). Error codes: `arbitrage_appeal_no_open_flag`, `arbitrage_appeal_document_too_large`, `arbitrage_appeal_invalid_jurisdiction`.

---

## Task 1: Developed-vs-emerging country lookup

**Files:**
- Create: `services/marketplace-api/internal/arbitrage/countries.go`
- Create: `services/marketplace-api/internal/arbitrage/countries_test.go`

**Spec references:** §4.1.3 (PPP tier eligibility aligns with emerging-market jurisdictions), §18.8 (triangulation on card_country / billing_country / ip_country).

**Purpose:** A pure lookup — no DB, no Secret Manager — that the evaluator consumes. The list is small enough to encode as a literal map; no need for config.

- [ ] **Step 1: Write failing tests**

In `countries_test.go`:
- `TestIsDevelopedMarket` — asserts `US, GB, DE, FR, AU, NZ, CA, IE, NL, IT, ES, JP, SG` return `true`; `IN, ID, VN, PH, TH, MY, BR, MX` return `false`; `"??"` and `""` return `false` (never flag on ambiguity).
- `TestNormalizeCountry` — `"us" → "US"`, `" US " → "US"`, `"" → "??"`, `"USA" → "??"` (not ISO-2), `"1" → "??"` (not alpha).

- [ ] **Step 2: Run — expect FAIL**

```bash
cd services/marketplace-api
go test ./internal/arbitrage/... -run TestIsDevelopedMarket -v
```

- [ ] **Step 3: Implement `countries.go`**

```go
// Package arbitrage encodes the geo-pricing triangulation check per spec §18.8.
// This file contains only the developed-vs-emerging market classifier —
// zero I/O, safe to call in a hot path.
package arbitrage

import (
    "strings"
    "unicode"
)

// developedMarkets is the exhaustive list that anti-arbitrage treats as
// "standard-tier eligible". Countries NOT in this set are either PPP-eligible
// or require a judgment call routed to billing-ops (§4.1.3 support escalation).
//
// The list aligns with the 13 billing currencies in §4.2.1 minus emerging
// markets. It is deliberately small — over-including countries risks flagging
// legitimate merchants; under-including risks missing arbitrage attempts. When
// the spec adds a currency, this list must be audited in the same PR.
var developedMarkets = map[string]struct{}{
    "US": {}, "CA": {}, "GB": {}, "IE": {},
    "DE": {}, "FR": {}, "IT": {}, "ES": {}, "NL": {},
    "AU": {}, "NZ": {}, "JP": {}, "SG": {},
}

// IsDevelopedMarket returns true for ISO-3166-1 alpha-2 codes in the developed
// set. Unknown, empty, or sentinel "??" input returns false — on ambiguity
// we err on the side of NOT flagging.
func IsDevelopedMarket(code string) bool {
    _, ok := developedMarkets[NormalizeCountry(code)]
    return ok
}

// NormalizeCountry returns an uppercased 2-letter code, or "??" when input is
// missing/invalid. Callers should propagate "??" into the audit row so the
// billing-ops reviewer can see which signal was missing.
func NormalizeCountry(code string) string {
    c := strings.ToUpper(strings.TrimSpace(code))
    if len(c) != 2 {
        return "??"
    }
    for _, r := range c {
        if !unicode.IsLetter(r) {
            return "??"
        }
    }
    return c
}
```

- [ ] **Step 4: Run — expect PASS**

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/arbitrage/countries{,_test}.go
git commit -m "feat(arbitrage): developed-vs-emerging market classifier"
```

---

## Task 2: `arbitrage.Evaluate` pure triangulation

**Files:**
- Create: `services/marketplace-api/internal/arbitrage/evaluator.go`
- Create: `services/marketplace-api/internal/arbitrage/evaluator_test.go`

**Spec references:** §18.8 — flag PPP-tier subscriptions where card_country or ip_country resolves to a developed market; billing_country is surface evidence but is not sufficient alone to clear a mismatch.

**Purpose:** One pure function that takes the three countries + the resolved price tier and returns a `Decision`. No DB, no HTTP — exhaustively unit-testable.

- [ ] **Step 1: Failing tests**

In `evaluator_test.go`:
- `TestEvaluate_PPPTierWithDevelopedCardIsFlagged` — `PriceTier=ppp, CardCountry=US, Billing=IN, IP=IN` → `Flagged=true`, reason contains `"card_country=US"` and `"developed"`.
- `TestEvaluate_PPPTierWithDevelopedIPIsFlagged` — `PriceTier=ppp, CardCountry=IN, Billing=IN, IP=GB` → `Flagged=true`, reason contains `"ip_country=GB"`.
- `TestEvaluate_PPPTierFullyEmergingIsClean` — all three countries emerging → `Flagged=false`, empty reason.
- `TestEvaluate_DevelopedTierNeverFlags` — `PriceTier=developed` with any country combo → `Flagged=false`. No arbitrage incentive when paying full price.
- `TestEvaluate_MissingIPCountryDoesNotFlag` — `IP="??"` → `Flagged=false`, `Note==ReasonIPUnknown`. Don't flag on card alone; travelers + dual-citizens would swamp the queue.
- `TestEvaluate_IsPure` — same input twice → `==` decisions.

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Implement `evaluator.go`**

```go
package arbitrage

import (
    "fmt"
    "strings"

    "github.com/tesserix/marketplace-api/internal/subscription"
)

// Input is the full triangulation surface. All country codes are ISO-3166-1
// alpha-2 after Normalize; "??" means missing.
type Input struct {
    PriceTier      subscription.PriceTier
    CardCountry    string
    BillingCountry string
    IPCountry      string
}

// Decision is what Evaluate returns. Flagged = write the audit row. Note is a
// non-flag observability hint (e.g. IP signal missing); MismatchReason is the
// human-readable rationale for a flag.
type Decision struct {
    Flagged        bool
    MismatchReason string
    Note           string
}

const (
    // ReasonIPUnknown — the IP country was "??" so we didn't have enough
    // signal to flag; billing-ops dashboards track these separately.
    ReasonIPUnknown = "ip_country_unknown"
)

// Evaluate is a pure function: no DB, no clock, no external calls. Given the
// three country signals + the resolved price tier, it decides whether to flag.
//
// Rules (§18.8):
//   - Developed-tier subscriptions are never flagged (no arbitrage incentive).
//   - PPP-tier subscriptions are flagged iff card_country OR ip_country points
//     at a developed market. billing_country alone is NOT sufficient to flag,
//     because a legitimate merchant may list a registered office in an
//     emerging market while paying via a developed-country corporate card —
//     that's exactly the kind of case billing-ops decides, not the code.
//   - Missing IP country ("??") downgrades to a Note; we don't flag on card
//     alone because travelers and dual-citizens would swamp the queue.
func Evaluate(in Input) Decision {
    if in.PriceTier != subscription.PriceTierPPP {
        return Decision{}
    }
    card := NormalizeCountry(in.CardCountry)
    ip := NormalizeCountry(in.IPCountry)

    if ip == "??" {
        return Decision{Note: ReasonIPUnknown}
    }

    reasons := make([]string, 0, 2)
    if IsDevelopedMarket(card) {
        reasons = append(reasons, fmt.Sprintf("card_country=%s (developed)", card))
    }
    if IsDevelopedMarket(ip) {
        reasons = append(reasons, fmt.Sprintf("ip_country=%s (developed)", ip))
    }
    if len(reasons) == 0 {
        return Decision{}
    }
    return Decision{
        Flagged:        true,
        MismatchReason: "PPP tier with " + strings.Join(reasons, "; "),
    }
}
```

- [ ] **Step 4: Run — expect PASS**

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/arbitrage/evaluator{,_test}.go
git commit -m "feat(arbitrage): pure triangulation evaluator — flag PPP+developed-card|ip"
```

---

## Task 3: `CF-IPCountry` extraction helper

**Files:**
- Create: `services/marketplace-api/internal/arbitrage/ipcountry.go`
- Create: `services/marketplace-api/internal/arbitrage/ipcountry_test.go`

**Spec references:** §18.8 (we trust the Cloudflare header; no local GeoIP).

**Purpose:** Single, well-named function that handlers call to get `(ipCountry, rawIP)`. The raw IP must be used only for hashing and must never be stored or logged.

- [ ] **Step 1: Failing tests**

In `ipcountry_test.go`, using a `newCtx(headers, remoteAddr)` helper that builds a `*gin.Context` with `httptest.NewRecorder` + `httptest.NewRequest`:

- `TestCFIPCountryFromGin_TrustsHeader` — `{CF-IPCountry:US, CF-Connecting-IP:203.0.113.42}` → `("US", "203.0.113.42")`. Prefer `CF-Connecting-IP` over `RemoteAddr`.
- `TestCFIPCountryFromGin_MissingHeader` — no CF headers, `RemoteAddr=10.0.0.1:12345` → `("??", "10.0.0.1")`. Fall back to `RemoteAddr`.
- `TestCFIPCountryFromGin_NormalizesCase` — `{CF-IPCountry:us}` → `"US"`.
- `TestCFIPCountryFromGin_RejectsXX` — `{CF-IPCountry:XX}` (Cloudflare emits `XX` for anonymous networks) → `"??"`.
- `TestCFIPCountryFromGin_RejectsT1` — `{CF-IPCountry:T1}` (Tor exit) → `"??"`.

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Implement `ipcountry.go`**

```go
package arbitrage

import (
    "net"

    "github.com/gin-gonic/gin"
)

// cfOpaqueCodes are Cloudflare-specific sentinels that are not real ISO codes.
// We treat them as "unknown" so the evaluator downgrades to a Note rather
// than flagging on bogus data.
var cfOpaqueCodes = map[string]struct{}{
    "XX": {}, // anonymous networks
    "T1": {}, // Tor exit
}

// CFIPCountryFromGin returns (ipCountry, rawIP). It reads the `CF-IPCountry`
// header (populated by Cloudflare Tunnel in front of Istio) and the
// `CF-Connecting-IP` header (the client's public IP as seen by Cloudflare).
//
// IMPORTANT: the rawIP is returned only for HMAC hashing. Callers MUST NOT
// persist, log, or forward it. Task 15 greps for violations.
func CFIPCountryFromGin(c *gin.Context) (ipCountry string, rawIP string) {
    ipCountry = NormalizeCountry(c.GetHeader("CF-IPCountry"))
    if _, opaque := cfOpaqueCodes[ipCountry]; opaque {
        ipCountry = "??"
    }

    rawIP = c.GetHeader("CF-Connecting-IP")
    if rawIP == "" {
        // Fall back to the transport address — useful for tests + local dev.
        host, _, err := net.SplitHostPort(c.Request.RemoteAddr)
        if err == nil {
            rawIP = host
        } else {
            rawIP = c.Request.RemoteAddr
        }
    }
    return ipCountry, rawIP
}
```

- [ ] **Step 4: Run — expect PASS**

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/arbitrage/ipcountry{,_test}.go
git commit -m "feat(arbitrage): CF-IPCountry extraction with XX/T1 opaque-code handling"
```

---

## Task 4: `KeyLoader` — Secret Manager client

**Files:**
- Create: `services/marketplace-api/internal/arbitrage/keyloader.go`
- Create: `services/marketplace-api/internal/arbitrage/keyloader_test.go`

**Spec references:** §18.8 (Secret Manager path, 30d rotation, 31d overlap, native version IDs).

**Purpose:** One client-shaped component that owns Secret Manager access. Exposes `Latest() ([]byte, string)` returning `(key, versionName)` and `All() []Version` returning every enabled version ≤61 days old. The latter is used by correlation queries; the hot path only ever calls `Latest()`.

**Note on dependency injection:** the real implementation takes a `*secretmanager.Client`. For tests we inject a `versionsSource` interface so we can stub out the cloud call.

- [ ] **Step 1: Failing tests**

Define a `stubSource` helper in `keyloader_test.go`:

```go
type stubSource struct {
    versions []arbitrage.KeyVersion
    err      error
    calls    int
}
func (s *stubSource) ListEnabled(context.Context) ([]arbitrage.KeyVersion, error) {
    s.calls++
    return s.versions, s.err
}
```

Tests:
- `TestKeyLoader_LatestReturnsNewest` — two versions (today, 30d ago) → `Latest()` returns the newest payload + version name.
- `TestKeyLoader_AllReturnsBothForCorrelation` — same two versions → `All()` length 2, newest-first.
- `TestKeyLoader_CachesBetweenCalls` — two `Latest()` calls within TTL → `src.calls == 1` (second call served from cache).
- `TestKeyLoader_TTLExpiryRefetches` — TTL = 1ns, sleep 2ns between calls → `src.calls == 2`.

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Implement `keyloader.go`**

```go
package arbitrage

import (
    "context"
    "errors"
    "sort"
    "sync"
    "time"
)

// KeyVersion is one row of Secret Manager metadata + payload.
type KeyVersion struct {
    Name      string // projects/.../secrets/arbitrage-ip-hmac-key/versions/N
    Payload   []byte
    CreatedAt time.Time
}

// VersionsSource is the injection point. Production is backed by
// secretmanager.Client; tests inject a stub.
type VersionsSource interface {
    ListEnabled(ctx context.Context) ([]KeyVersion, error)
}

// KeyLoader caches versions in-process with TTL. It never blocks on startup —
// first call on a cold cache is the one that fetches.
type KeyLoader struct {
    src VersionsSource
    ttl time.Duration

    mu          sync.RWMutex
    cached      []KeyVersion
    cachedAt    time.Time
}

func NewKeyLoader(src VersionsSource, ttl time.Duration) *KeyLoader {
    return &KeyLoader{src: src, ttl: ttl}
}

// Latest returns the newest enabled version's payload + name. Used by Hasher
// for every write.
func (l *KeyLoader) Latest(ctx context.Context) ([]byte, string, error) {
    vs, err := l.get(ctx)
    if err != nil {
        return nil, "", err
    }
    if len(vs) == 0 {
        return nil, "", errors.New("arbitrage: no enabled key versions in Secret Manager")
    }
    return vs[0].Payload, vs[0].Name, nil
}

// All returns every enabled version newest-first. Used by correlation queries.
func (l *KeyLoader) All(ctx context.Context) ([]KeyVersion, error) {
    return l.get(ctx)
}

func (l *KeyLoader) get(ctx context.Context) ([]KeyVersion, error) {
    l.mu.RLock()
    if len(l.cached) > 0 && time.Since(l.cachedAt) < l.ttl {
        out := l.cached
        l.mu.RUnlock()
        return out, nil
    }
    l.mu.RUnlock()

    l.mu.Lock()
    defer l.mu.Unlock()
    // Double-check under write lock.
    if len(l.cached) > 0 && time.Since(l.cachedAt) < l.ttl {
        return l.cached, nil
    }

    vs, err := l.src.ListEnabled(ctx)
    if err != nil {
        return nil, err
    }
    sort.Slice(vs, func(i, j int) bool { return vs[i].CreatedAt.After(vs[j].CreatedAt) })
    l.cached = vs
    l.cachedAt = time.Now()
    return vs, nil
}
```

- [ ] **Step 4: Implement production Secret Manager adapter**

Add to the same file:

```go
// SecretManagerSource is the production VersionsSource. It implements
// ListEnabled against `cloud.google.com/go/secretmanager/apiv1`.
//
// The adapter is intentionally thin: no caching (KeyLoader handles that) and
// no filtering-by-age (every enabled version is returned; callers apply their
// own age policy). Secret Manager's `Disabled` state is our boundary — the
// rotation job disables versions >61d old, and this source only returns
// enabled ones.
type SecretManagerSource struct {
    Client     SecretManagerClient // interface, real type is *secretmanager.Client
    SecretPath string              // "projects/tesserix-prod/secrets/arbitrage-ip-hmac-key"
}

// SecretManagerClient is the minimal slice of *secretmanager.Client we use.
// Full type lives in cloud.google.com/go/secretmanager/apiv1; we depend on
// the interface so tests can skip the cloud import.
type SecretManagerClient interface {
    ListSecretVersions(ctx context.Context, req *smpb.ListSecretVersionsRequest, opts ...gax.CallOption) SecretVersionIterator
    AccessSecretVersion(ctx context.Context, req *smpb.AccessSecretVersionRequest, opts ...gax.CallOption) (*smpb.AccessSecretVersionResponse, error)
}

func (s *SecretManagerSource) ListEnabled(ctx context.Context) ([]KeyVersion, error) {
    // Implementation: iterate ListSecretVersions, filter State == ENABLED,
    // AccessSecretVersion for each payload, build KeyVersion slice.
    // Full implementation in the file; elided here for plan brevity.
    return nil, errors.New("not yet implemented — see keyloader.go")
}
```

**Note:** the full `SecretManagerSource.ListEnabled` body is ~40 lines of cloud-client boilerplate; the integration test in Task 14 covers it.

- [ ] **Step 5: Run — expect PASS**

- [ ] **Step 6: Commit**

```bash
git add services/marketplace-api/internal/arbitrage/keyloader{,_test}.go
git commit -m "feat(arbitrage): KeyLoader with Secret Manager source + TTL cache"
```

---

## Task 5: `Hasher` — HMAC-SHA256 keyed by `KeyLoader.Latest()`

**Files:**
- Create: `services/marketplace-api/internal/arbitrage/hmac.go`
- Create: `services/marketplace-api/internal/arbitrage/hmac_test.go`

**Spec references:** §18.8 (HMAC-SHA256, length-extension-safe), §28 criterion #51.

- [ ] **Step 1: Failing tests**

In `hmac_test.go` (reusing `stubSource` from Task 4):

- `TestHasher_DeterministicUnderSameKey` — hash `"203.0.113.1"` twice under the same key → same hex digest, length exactly 64.
- `TestHasher_DifferentKeysProduceDifferentDigests` — same IP hashed under `keyA` vs `keyB` → different digests. (Rotation severs correlation.)
- `TestHasher_RecordsKeyVersion` — returned `HashResult.KeyVersion` matches the Secret Manager resource name of the newest version.
- `TestHasher_EmptyIPReturnsEmpty` — empty IP → `HashResult{}` (no fabricated hash).

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Implement `hmac.go`**

```go
package arbitrage

import (
    "context"
    "crypto/hmac"
    "crypto/sha256"
    "encoding/hex"
)

// HashResult is what Hasher returns. KeyVersion is stored on the audit row
// (out-of-band in metadata, not ip_hash itself) so correlation code knows
// which key to use when comparing two rows.
type HashResult struct {
    Hex        string // HMAC-SHA256 hex, always 64 chars, or "" if no IP
    KeyVersion string // Secret Manager version resource name
}

// Hasher is the one-and-only path for IP → ip_hash. It must be used for every
// write of subscription_arbitrage_audit.ip_hash. Task 15 enforces this.
type Hasher struct {
    loader *KeyLoader
}

func NewHasher(loader *KeyLoader) *Hasher {
    return &Hasher{loader: loader}
}

// Hash computes HMAC-SHA256(key=latest_secret_version, data=rawIP) and returns
// the hex digest + the key's Secret Manager version name.
//
// This is the *correct* construction: hmac.New(sha256.New, key) is keyed-hash
// by design, length-extension-safe, and is what the spec's security review
// reasoned about. Do NOT replace with sha256.Sum256(append(salt, ip...)).
func (h *Hasher) Hash(ctx context.Context, rawIP string) (HashResult, error) {
    if rawIP == "" {
        return HashResult{}, nil
    }
    key, version, err := h.loader.Latest(ctx)
    if err != nil {
        return HashResult{}, err
    }
    mac := hmac.New(sha256.New, key)
    _, _ = mac.Write([]byte(rawIP))
    digest := mac.Sum(nil)
    return HashResult{
        Hex:        hex.EncodeToString(digest),
        KeyVersion: version,
    }, nil
}
```

- [ ] **Step 4: Run — expect PASS**

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/arbitrage/hmac{,_test}.go
git commit -m "feat(arbitrage): HMAC-SHA256 hasher with Secret-Manager-versioned key"
```

---

## Task 6: `Recorder.RecordIfFlagged` — persist audit + toggle flag

**Files:**
- Create: `services/marketplace-api/internal/arbitrage/recorder.go`
- Create: `services/marketplace-api/internal/arbitrage/recorder_test.go`

**Spec references:** §18.8 (write audit row; `arbitrage_flag = true`); §28 criterion #51.

- [ ] **Step 1: Failing integration tests** (`//go:build integration`)

In `recorder_test.go`:

- `TestRecorder_FlagWritesAuditRowAndTogglesFlag` — seed a `StoreSubscription{PriceTier: PPP}`, call `RecordIfFlagged` with `CardCountry=US, Billing=IN, IP=IN, RawIP="203.0.113.9"`. Assert: one audit row exists (card/billing/ip country match, `IPHash` length 64, `Resolution="ongoing"`, `MismatchReason` contains `"card_country=US"`), and `reloaded.ArbitrageFlag == true`.
- `TestRecorder_NoFlagIsNoOp` — `PriceTier=Developed` with any country mix → zero audit rows, `ArbitrageFlag=false`.
- `TestRecorder_IncrementsCounter` — flagged path increments `spyCounter.flaggedCalls` by 1; no-flag path leaves it at 0.

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Implement `recorder.go`**

```go
package arbitrage

import (
    "context"
    "errors"
    "fmt"

    "github.com/google/uuid"
    "gorm.io/gorm"

    "github.com/tesserix/marketplace-api/internal/subscription"
)

// Counter is the observability interface. Production wires to Prometheus;
// tests spy on calls. See P17 for alert rules over these counters.
type Counter interface {
    IncArbitrageFlagged()
    IncArbitrageFalsePositiveCleared()
}

// RecordInput carries every field a triangulation decision needs.
type RecordInput struct {
    SubscriptionID uuid.UUID
    TenantID       uuid.UUID
    StoreID        uuid.UUID
    PriceTier      subscription.PriceTier
    CardCountry    string
    BillingCountry string
    IPCountry      string
    RawIP          string // hashed then dropped; never persisted
}

type Recorder struct {
    db     *gorm.DB
    hasher *Hasher
    count  Counter
}

func NewRecorder(db *gorm.DB, hasher *Hasher, count Counter) *Recorder {
    return &Recorder{db: db, hasher: hasher, count: count}
}

// RecordIfFlagged evaluates + persists + toggles + counts. On a clean decision
// it's a no-op.
//
// Called from: P2 webhook dispatcher handleCustomerSubscriptionCreated +
// handleCheckoutSessionCompleted. Must be safe to call twice (Stripe redelivers
// on transient failure). Duplicate audit rows are permitted — billing-ops
// treats them as part of the same investigation; we do NOT unique on
// (subscription_id) because a later webhook event could legitimately surface
// new triangulation signal.
func (r *Recorder) RecordIfFlagged(ctx context.Context, in RecordInput) error {
    dec := Evaluate(Input{
        PriceTier:      in.PriceTier,
        CardCountry:    in.CardCountry,
        BillingCountry: in.BillingCountry,
        IPCountry:      in.IPCountry,
    })
    if !dec.Flagged {
        return nil
    }
    hash, err := r.hasher.Hash(ctx, in.RawIP)
    if err != nil {
        // Hashing failed (Secret Manager unavailable). We prefer to persist
        // the flag with an empty ip_hash than to silently drop the signal —
        // billing-ops still has card_country + ip_country + billing_country
        // to act on.
        hash = HashResult{}
    }

    return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
        row := SubscriptionArbitrageAudit{
            SubscriptionID:    in.SubscriptionID,
            TenantID:          in.TenantID,
            StoreID:           in.StoreID,
            CardCountry:       NormalizeCountry(in.CardCountry),
            BillingCountry:    NormalizeCountry(in.BillingCountry),
            IPCountry:         NormalizeCountry(in.IPCountry),
            IPHash:            hash.Hex,
            ResolvedPriceTier: string(in.PriceTier),
            MismatchReason:    dec.MismatchReason,
            Resolution:        "ongoing",
        }
        if err := tx.Create(&row).Error; err != nil {
            return fmt.Errorf("insert arbitrage audit: %w", err)
        }
        res := tx.Model(&subscription.StoreSubscription{}).
            Where("id = ? AND tenant_id = ?", in.SubscriptionID, in.TenantID).
            Update("arbitrage_flag", true)
        if res.Error != nil {
            return fmt.Errorf("toggle arbitrage_flag: %w", res.Error)
        }
        if res.RowsAffected == 0 {
            return errors.New("arbitrage: subscription not found or tenant mismatch")
        }
        return nil
    })
    // NOTE: counter increment is after the commit so we never count a write
    // that rolled back. Immediate-after pattern:
    //   if err := tx.Commit(); err == nil { r.count.IncArbitrageFlagged() }
    // GORM's Transaction wrapper commits-or-rolls-back before returning,
    // so the post-return branch is safe.
}
```

Append the counter increment after the transaction block:

```go
    if err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
        // ... (body above)
    }); err != nil {
        return err
    }
    r.count.IncArbitrageFlagged()
    return nil
}
```

- [ ] **Step 4: Run — expect PASS**

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/arbitrage/recorder{,_test}.go
git commit -m "feat(arbitrage): Recorder.RecordIfFlagged persists audit row and toggles flag"
```

---

## Task 7: Wire `Recorder` into P2 dispatcher

**Files:**
- Modify: `services/marketplace-api/internal/billing/dispatch/handlers.go`
- Modify: `services/marketplace-api/cmd/marketplace-api/main.go`

**Spec references:** §18.8 (triangulation "at subscription creation"); §17.7 (webhook events).

**Purpose:** Slot `Recorder.RecordIfFlagged` into the two webhook handlers that see subscription creation. The call must be post-persistence (so we have a `subscription_id`) and pre-return (so an error surfaces back to Stripe for redelivery if the DB write fails).

- [ ] **Step 1: Read P2 handler structure**

```bash
grep -n "handleCustomerSubscriptionCreated\|handleCheckoutSessionCompleted" \
  services/marketplace-api/internal/billing/dispatch/handlers.go
```

- [ ] **Step 2: Modify `handleCustomerSubscriptionCreated`**

Insert after the subscription row is persisted:

```go
// Triangulation check per §18.8. Flag-only; never short-circuit the webhook.
// Errors from RecordIfFlagged roll back into the handler return so Stripe
// redelivers; the triangulation row is itself idempotent-ish (duplicate rows
// are permitted — billing-ops groups by subscription_id).
cardCountry := ""
if s.PaymentMethod != nil && s.PaymentMethod.Card != nil {
    cardCountry = s.PaymentMethod.Card.Country
}
billingCountry := ""
if s.Customer != nil && s.Customer.Address != nil {
    billingCountry = s.Customer.Address.Country
}
ipCountry, rawIP := arbitrage.CFIPCountryFromGin(ginCtx)

if err := h.arbitrage.RecordIfFlagged(ctx, arbitrage.RecordInput{
    SubscriptionID: sub.ID,
    TenantID:       sub.TenantID,
    StoreID:        sub.StoreID,
    PriceTier:      sub.PriceTier,
    CardCountry:    cardCountry,
    BillingCountry: billingCountry,
    IPCountry:      ipCountry,
    RawIP:          rawIP,
}); err != nil {
    // Log + return so Stripe redelivers. The subscription row itself is
    // already committed in the outer transaction, so redelivery is
    // idempotent via the stripe_webhook_events dedupe table (P2).
    return fmt.Errorf("arbitrage record: %w", err)
}
```

- [ ] **Step 3: Do the same in `handleCheckoutSessionCompleted`**

The signal is often richer at checkout.session.completed (billing_details has already been filled in), so we run the check at both entry points. Duplicate flagged rows are permitted and grouped by `subscription_id` in the billing-ops UI.

- [ ] **Step 4: Wire dependency in `cmd/marketplace-api/main.go`**

```go
// Arbitrage pipeline.
smClient, err := secretmanager.NewClient(ctx)
if err != nil {
    return fmt.Errorf("secret manager client: %w", err)
}
keySrc := &arbitrage.SecretManagerSource{
    Client:     smClient,
    SecretPath: os.Getenv("ARBITRAGE_HMAC_SECRET_PATH"), // /projects/tesserix-prod/secrets/arbitrage-ip-hmac-key
}
keyLoader := arbitrage.NewKeyLoader(keySrc, 5*time.Minute)
hasher := arbitrage.NewHasher(keyLoader)
recorder := arbitrage.NewRecorder(db, hasher, prom.ArbitrageCounter)

// Pass to dispatch handlers.
dispatchHandler := dispatch.NewHandler(db, ..., recorder)
```

- [ ] **Step 5: Integration test — subscription.created webhook flags**

```go
//go:build integration

func TestDispatchHandler_FlagsOnPPPPlusDevelopedCard(t *testing.T) {
    // Seed an active PPP subscription, then replay the customer.subscription.created
    // webhook with card_country=US. Assert the arbitrage row exists + flag toggled.
    // (Body elided — uses testdb + the P2 dispatch setup.)
}
```

- [ ] **Step 6: Run — expect PASS**

- [ ] **Step 7: Commit**

```bash
git add services/marketplace-api/internal/billing/dispatch/handlers.go
git add services/marketplace-api/cmd/marketplace-api/main.go
git commit -m "feat(arbitrage): wire Recorder into Stripe subscription.created + checkout.completed"
```

---

## Task 8: `AppealService.Submit` — resolve via existing audit row

**Files:**
- Create: `services/marketplace-api/internal/arbitrage/appeal.go`
- Create: `services/marketplace-api/internal/arbitrage/appeal_test.go`

**Spec references:** §18.8.1 (self-service appeal; 5-biz-day SLA; resolution enum `false_positive_cleared` | `reprice_developed` | `ongoing`).

**Purpose:** Write `reviewed_by`, `reviewed_at`, `resolution` on the *existing* `subscription_arbitrage_audit` row. No new schema. Publish to the `billing-ops` Pub/Sub queue so the review task goes through the normal ops dashboard. The merchant-submitted jurisdiction + optional document upload path is stored in the audit row's `mismatch_reason` (appended under a delimiter, not replacing the original triangulation rationale) — billing-ops reviewers read both.

- [ ] **Step 1: Failing integration tests** (`//go:build integration`)

In `appeal_test.go`:

- `TestAppealService_MarksAuditRowUnderReview` — seed a `StoreSubscription{PriceTier:PPP, ArbitrageFlag:true}` + `SubscriptionArbitrageAudit{Resolution:"ongoing"}`. Call `Submit` with `Jurisdiction="IN"`, a justification string, and a `gs://` doc URL. Assert: `ReviewedBy == merchantUserID`, `ReviewedAt` non-nil, `Resolution` still `"ongoing"` (billing-ops closes it), `MismatchReason` contains `"MERCHANT_APPEAL"` and `"IN"`.
- `TestAppealService_RejectsNoOpenFlag` — empty DB → `ErrNoOpenFlag`.
- `TestAppealService_PublishesToBillingOpsQueue` — seed row + submit → spy publisher recorded one message with `subscription_id` + `jurisdiction`.

- [ ] **Step 2: Run — expect FAIL**

- [ ] **Step 3: Implement `appeal.go`**

```go
package arbitrage

import (
    "context"
    "errors"
    "fmt"
    "time"

    "github.com/google/uuid"
    "gorm.io/gorm"
)

var (
    ErrNoOpenFlag = errors.New("arbitrage: no open arbitrage flag for store")
)

// Publisher abstracts go-shared/messaging so tests can inject a spy.
type Publisher interface {
    Publish(ctx context.Context, topic string, payload any) error
}

// AppealInput is the merchant-submitted form (§18.8.1).
type AppealInput struct {
    TenantID     uuid.UUID
    StoreID      uuid.UUID
    Jurisdiction string    // ISO-2 the merchant claims to operate from
    Justification string   // free text (trimmed to 1000 chars)
    DocumentURL  string    // gs:// URI, optional
    ActorUserID  uuid.UUID // the admin user submitting the appeal
}

type AppealService struct {
    db        *gorm.DB
    publisher Publisher
    piiLogger PIILogger
}

func NewAppealService(db *gorm.DB, pub Publisher, pii PIILogger) *AppealService {
    return &AppealService{db: db, publisher: pub, piiLogger: pii}
}

// Submit updates the latest ongoing audit row in place and queues a
// billing-ops review. Resolution STAYS "ongoing" — only a billing-ops reviewer
// can move it to false_positive_cleared or reprice_developed.
func (s *AppealService) Submit(ctx context.Context, in AppealInput) error {
    s.piiLogger.LogPIIAccess(ctx, PIIAccessEvent{
        Actor:     in.ActorUserID,
        StoreID:   in.StoreID,
        TenantID:  in.TenantID,
        Operation: "arbitrage_appeal_submit",
    })

    now := time.Now().UTC()
    var row SubscriptionArbitrageAudit
    q := s.db.WithContext(ctx).
        Where("tenant_id = ? AND store_id = ? AND resolution = 'ongoing'", in.TenantID, in.StoreID).
        Order("flagged_at DESC").
        Limit(1)
    if err := q.First(&row).Error; err != nil {
        if errors.Is(err, gorm.ErrRecordNotFound) {
            return ErrNoOpenFlag
        }
        return fmt.Errorf("load audit row: %w", err)
    }

    appended := row.MismatchReason + "\n---\nMERCHANT_APPEAL jurisdiction=" + NormalizeCountry(in.Jurisdiction)
    if in.Justification != "" {
        if len(in.Justification) > 1000 {
            in.Justification = in.Justification[:1000]
        }
        appended += " justification=" + in.Justification
    }
    if in.DocumentURL != "" {
        appended += " doc=" + in.DocumentURL
    }

    actor := in.ActorUserID
    if err := s.db.WithContext(ctx).
        Model(&SubscriptionArbitrageAudit{}).
        Where("id = ?", row.ID).
        Updates(map[string]any{
            "reviewed_by":     &actor,
            "reviewed_at":     &now,
            "mismatch_reason": appended,
            // resolution stays "ongoing" until billing-ops closes it.
        }).Error; err != nil {
        return fmt.Errorf("update audit row: %w", err)
    }

    // Enqueue billing-ops review. Non-fatal if publish fails — billing-ops
    // also polls the table directly via the `ongoing` partial index.
    _ = s.publisher.Publish(ctx, "billing-ops.arbitrage-appeal", map[string]any{
        "audit_id":       row.ID,
        "subscription_id": row.SubscriptionID,
        "tenant_id":      row.TenantID,
        "store_id":       row.StoreID,
        "jurisdiction":   NormalizeCountry(in.Jurisdiction),
        "document_url":   in.DocumentURL,
        "submitted_at":   now,
    })
    return nil
}
```

- [ ] **Step 4: Run — expect PASS**

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/arbitrage/appeal{,_test}.go
git commit -m "feat(arbitrage): self-service AppealService — updates audit row + queues billing-ops"
```

---

## Task 9: `POST /admin/stores/:storeId/arbitrage-appeal` handler

**Files:**
- Create: `services/marketplace-api/internal/handlers/admin/arbitrage_appeal.go`
- Create: `services/marketplace-api/internal/handlers/admin/arbitrage_appeal_test.go`
- Modify: `services/marketplace-api/internal/handlers/admin/routes.go`

**Spec references:** §18.8.1 (form: jurisdiction + optional document upload).

- [ ] **Step 1: Failing test**

```go
func TestArbitrageAppealHandler_Success(t *testing.T) {
    // Set up Gin test context with tenant + user, POST JSON body,
    // assert 200 + response envelope + service called exactly once.
}

func TestArbitrageAppealHandler_RejectsMissingJurisdiction(t *testing.T) {
    // POST with no jurisdiction → 400, error code `arbitrage_appeal_invalid_jurisdiction`.
}

func TestArbitrageAppealHandler_ReturnsNoFlagError(t *testing.T) {
    // Service returns ErrNoOpenFlag → handler returns 404 with code `arbitrage_appeal_no_open_flag`.
}

func TestArbitrageAppealHandler_EnforcesRole(t *testing.T) {
    // Non-admin user → 403.
}
```

- [ ] **Step 2: Implement the handler**

```go
package admin

import (
    "errors"
    "net/http"

    "github.com/gin-gonic/gin"
    "github.com/google/uuid"

    "github.com/tesserix/marketplace-api/internal/apperror"
    "github.com/tesserix/marketplace-api/internal/arbitrage"
    "github.com/tesserix/marketplace-api/internal/authz"
)

type ArbitrageAppealHandler struct {
    svc *arbitrage.AppealService
}

func NewArbitrageAppealHandler(svc *arbitrage.AppealService) *ArbitrageAppealHandler {
    return &ArbitrageAppealHandler{svc: svc}
}

type appealBody struct {
    Jurisdiction  string `json:"jurisdiction" binding:"required,len=2"`
    Justification string `json:"justification"`
    DocumentURL   string `json:"document_url"`
}

// POST /admin/stores/:storeId/arbitrage-appeal
func (h *ArbitrageAppealHandler) Submit(c *gin.Context) {
    storeID, err := uuid.Parse(c.Param("storeId"))
    if err != nil {
        apperror.Write(c, http.StatusBadRequest, "invalid_store_id", "")
        return
    }
    tenantID, _ := c.Get("tenant_id")
    userID, _ := c.Get("user_id")

    var body appealBody
    if err := c.ShouldBindJSON(&body); err != nil {
        apperror.Write(c, http.StatusBadRequest, "arbitrage_appeal_invalid_jurisdiction", err.Error())
        return
    }
    if !arbitrage.IsKnownCountry(body.Jurisdiction) {
        apperror.Write(c, http.StatusBadRequest, "arbitrage_appeal_invalid_jurisdiction", "")
        return
    }

    err = h.svc.Submit(c.Request.Context(), arbitrage.AppealInput{
        TenantID:      uuid.MustParse(tenantID.(string)),
        StoreID:       storeID,
        Jurisdiction:  body.Jurisdiction,
        Justification: body.Justification,
        DocumentURL:   body.DocumentURL,
        ActorUserID:   uuid.MustParse(userID.(string)),
    })
    switch {
    case errors.Is(err, arbitrage.ErrNoOpenFlag):
        apperror.Write(c, http.StatusNotFound, "arbitrage_appeal_no_open_flag", "")
    case err != nil:
        apperror.Write(c, http.StatusInternalServerError, "arbitrage_appeal_failed", err.Error())
    default:
        c.JSON(http.StatusOK, gin.H{"data": gin.H{"status": "submitted"}, "error": nil})
    }
}

// Route registration — add alongside other admin subscription endpoints.
func RegisterArbitrageAppeal(r *gin.RouterGroup, h *ArbitrageAppealHandler, fgaMw *authz.Middleware) {
    r.POST("/stores/:storeId/arbitrage-appeal",
        fgaMw.RequireStorePermission(authz.MPCanManageSubscription),
        h.Submit)
}
```

- [ ] **Step 3: Document upload path**

The appeal reuses the **existing** media/GCS upload endpoint (`POST /admin/stores/:storeId/media/upload` if present, else the existing document-service proxy). The client uploads the document first to get a `gs://` URL, then posts it as `document_url` in the appeal body. We do NOT accept multipart form data on the appeal endpoint — that keeps the handler stateless and lets the media path own the virus-scan/quota checks.

- [ ] **Step 4: Wire route into `routes.go`**

```go
arbitrageAppealH := admin.NewArbitrageAppealHandler(deps.ArbitrageAppeal)
admin.RegisterArbitrageAppeal(adminGroup, arbitrageAppealH, fgaMw)
```

- [ ] **Step 5: Run — expect PASS**

- [ ] **Step 6: Commit**

```bash
git add services/marketplace-api/internal/handlers/admin/arbitrage_appeal{,_test}.go
git add services/marketplace-api/internal/handlers/admin/routes.go
git commit -m "feat(arbitrage): POST /admin/stores/:storeId/arbitrage-appeal endpoint"
```

---

## Task 10: Extend `GetSubscription` payload with flag + latest audit

**Files:**
- Modify: `services/marketplace-api/internal/handlers/admin/subscription.go`
- Modify: `services/marketplace-api/internal/handlers/admin/subscription_test.go`

**Spec references:** §18.8.1 (admin UI banner data contract).

**Purpose:** P16 needs a response shape to render the banner. Expose `arbitrage_flag` on the top level + `latest_arbitrage_audit` nested (nullable, single row, no PII beyond country codes).

- [ ] **Step 1: Failing test**

```go
func TestGetSubscription_IncludesArbitrageFields(t *testing.T) {
    // Seed a flagged subscription + audit row.
    // GET /admin/stores/:storeId/subscription → JSON includes
    //   arbitrage_flag=true + latest_arbitrage_audit{card_country, ip_country, billing_country, resolution, flagged_at}
    // MUST NOT expose ip_hash (that's PII-adjacent; reviewers see it via internal tooling).
}

func TestGetSubscription_OmitsArbitrageWhenClean(t *testing.T) {
    // Clean subscription → arbitrage_flag=false, latest_arbitrage_audit=nil.
}
```

- [ ] **Step 2: Define response struct**

```go
type subscriptionResponse struct {
    // ... existing fields
    ArbitrageFlag         bool                    `json:"arbitrage_flag"`
    LatestArbitrageAudit  *arbitrageAuditPayload  `json:"latest_arbitrage_audit,omitempty"`
}

type arbitrageAuditPayload struct {
    CardCountry     string    `json:"card_country"`
    BillingCountry  string    `json:"billing_country"`
    IPCountry       string    `json:"ip_country"`
    Resolution      string    `json:"resolution"`
    FlaggedAt       time.Time `json:"flagged_at"`
    MismatchReason  string    `json:"mismatch_reason"` // public summary — reviewer notes live elsewhere
    // Deliberately omitted: ip_hash, reviewed_by, reviewed_at —
    // those are billing-ops-only via a separate internal endpoint.
}
```

- [ ] **Step 3: Load the latest audit row in the handler**

```go
var audit arbitrage.SubscriptionArbitrageAudit
err := h.db.WithContext(c).
    Where("tenant_id=? AND store_id=?", tenantID, storeID).
    Order("flagged_at DESC").Limit(1).First(&audit).Error

if errors.Is(err, gorm.ErrRecordNotFound) {
    resp.LatestArbitrageAudit = nil
} else if err != nil {
    // Degrade gracefully — arbitrage is not load-bearing for the read path.
    logger.WithError(err).Warn("arbitrage audit load failed; omitting from response")
} else {
    // Every read of this table is PII-adjacent — log the access.
    h.piiLogger.LogPIIAccess(c.Request.Context(), arbitrage.PIIAccessEvent{
        Actor: userID, StoreID: storeID, TenantID: tenantID,
        Operation: "arbitrage_audit_read_admin_subscription",
    })
    resp.LatestArbitrageAudit = &arbitrageAuditPayload{
        CardCountry: audit.CardCountry, BillingCountry: audit.BillingCountry,
        IPCountry: audit.IPCountry, Resolution: audit.Resolution,
        FlaggedAt: audit.FlaggedAt, MismatchReason: audit.MismatchReason,
    }
}
```

- [ ] **Step 4: Run — expect PASS**

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/handlers/admin/subscription{,_test}.go
git commit -m "feat(arbitrage): expose arbitrage_flag + latest audit on admin subscription payload"
```

---

## Task 11: `QuarterlyAudit.Run` — clear ongoing flags under advisory lock

**Files:**
- Create: `services/marketplace-api/internal/arbitrage/quarterly_cron.go`
- Create: `services/marketplace-api/internal/arbitrage/quarterly_cron_test.go`

**Spec references:** §18.8 ("quarterly-audit job that clears flags takes `store_id` advisory lock").

**Purpose:** Every quarter, a job walks the `subscription_arbitrage_audit` table, reads all `ongoing` rows older than N days (config; default 90), applies a policy (billing-ops-approved list of merchants to clear), and toggles `resolution` + `arbitrage_flag` under a per-store advisory lock so a merchant-initiated appeal cannot collide.

For v1, "policy" is a flat allowlist file loaded from Secret Manager — billing-ops edits it via a separate admin flow (out of scope for this plan). This task wires the cron shell + advisory-lock mechanics; the allowlist consumer is a trivial `bool` that plans can swap in later.

- [ ] **Step 1: Failing tests** (`//go:build integration`)

- `TestQuarterlyAudit_ClearsAllowlistedFlags` — seed a flagged subscription + a 120-day-old `ongoing` audit row. Allowlist returns `true` for all. `Run` → audit `Resolution=="false_positive_cleared"`, subscription `ArbitrageFlag==false`, and per-store advisory lock was taken.
- `TestQuarterlyAudit_SkipsNonAllowlisted` — same setup, allowlist returns `false` → rows untouched.

- [ ] **Step 2: Implement `quarterly_cron.go`**

```go
package arbitrage

import (
    "context"
    "fmt"
    "time"

    "github.com/google/uuid"
    "gorm.io/gorm"

    "github.com/tesserix/marketplace-api/internal/subscription"
)

// AllowlistFunc is the billing-ops-approved set. Production is backed by a
// Secret Manager-managed list; tests inject a predicate directly.
type AllowlistFunc func(subscriptionID uuid.UUID) bool

type QuarterlyAudit struct {
    db         *gorm.DB
    allowlist  AllowlistFunc
    counter    Counter
    piiLogger  PIILogger
    maxAgeDays int
}

func NewQuarterlyAudit(db *gorm.DB, allowlist AllowlistFunc, counter Counter, pii PIILogger) *QuarterlyAudit {
    return &QuarterlyAudit{db: db, allowlist: allowlist, counter: counter, piiLogger: pii, maxAgeDays: 90}
}

// Run walks every `ongoing` audit row older than maxAgeDays. For each,
// checks the allowlist; if allowed, takes per-store advisory lock + clears
// the flag + toggles `arbitrage_flag = false`.
func (a *QuarterlyAudit) Run(ctx context.Context) error {
    cutoff := time.Now().Add(-time.Duration(a.maxAgeDays) * 24 * time.Hour)

    rows := make([]SubscriptionArbitrageAudit, 0, 128)
    if err := a.db.WithContext(ctx).
        Where("resolution = 'ongoing' AND flagged_at < ?", cutoff).
        Find(&rows).Error; err != nil {
        return fmt.Errorf("load ongoing audits: %w", err)
    }

    for _, row := range rows {
        if !a.allowlist(row.SubscriptionID) {
            continue
        }
        row := row // capture for closure
        err := subscription.WithAdvisoryLock(ctx, a.db, row.StoreID, func(tx *gorm.DB) error {
            if err := tx.Model(&SubscriptionArbitrageAudit{}).
                Where("id = ?", row.ID).
                Updates(map[string]any{
                    "resolution":  "false_positive_cleared",
                    "reviewed_at": time.Now().UTC(),
                }).Error; err != nil {
                return err
            }
            return tx.Model(&subscription.StoreSubscription{}).
                Where("id = ? AND tenant_id = ?", row.SubscriptionID, row.TenantID).
                Update("arbitrage_flag", false).Error
        })
        if err != nil {
            // Per-row errors log + continue; one bad row must not block the batch.
            a.piiLogger.LogPIIAccess(ctx, PIIAccessEvent{
                Actor: uuid.Nil, StoreID: row.StoreID, TenantID: row.TenantID,
                Operation: "arbitrage_quarterly_clear_failed", Note: err.Error(),
            })
            continue
        }
        a.counter.IncArbitrageFalsePositiveCleared()
    }
    return nil
}
```

- [ ] **Step 3: Run — expect PASS**

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/internal/arbitrage/quarterly_cron{,_test}.go
git commit -m "feat(arbitrage): QuarterlyAudit.Run clears ongoing flags under advisory lock"
```

---

## Task 12: `cmd/arbitrage-rotator` — nightly key-rotation job

**Files:**
- Create: `services/marketplace-api/cmd/arbitrage-rotator/main.go`
- Create: `services/marketplace-api/cmd/arbitrage-rotator/main_test.go`
- Create: `services/marketplace-api/deploy/arbitrage-rotator.k8s.yaml`

**Spec references:** §18.8 (30d rotation, 31d overlap → 61d total retention); §28 criterion #51.

**Purpose:** A small Go main that Cloud Scheduler pokes nightly. On each run:

1. List all versions of the secret.
2. If the newest enabled version is ≥30 days old → create a new version with a fresh 256-bit CSPRNG payload.
3. For every enabled version ≥61 days old → disable (not destroy; destroy is a separate quarterly cleanup so accidental disables are reversible within 30 days).
4. Exit 0 on success; non-zero on Secret Manager failure (Cloud Scheduler retries with backoff).

The binary is wired into a Cloud Run Job + Cloud Scheduler trigger; the K8s manifest stub lands in this repo but the infra team finalizes IAM in tesserix-k8s.

- [ ] **Step 1: Failing tests**

In `main_test.go`:

- `TestShouldRotate_NewestOver30d` — single version aged 31d → `true`.
- `TestShouldRotate_NewestUnder30d` — single version aged 5d → `false`.
- `TestShouldRotate_EmptyForcesFirstWrite` — empty slice → `true` (bootstrap).
- `TestVersionsToDisable` — versions aged `{10d, 40d, 62d, 100d}`, maxDays=61 → disable list contains names of the 62d + 100d versions only.

- [ ] **Step 2: Implement `main.go`**

```go
// Command arbitrage-rotator rotates the HMAC key used by the anti-arbitrage
// IP hasher. Runs nightly as a Cloud Run Job triggered by Cloud Scheduler.
//
// Rotation policy (spec §18.8):
//   - New version when the newest enabled version is ≥30 days old.
//   - Old versions retained for 31 days past their replacement (so any
//     ip_hash produced under the old key can still be correlated during the
//     overlap window). After 61 days total, the old version is DISABLED.
//   - Disabled != Destroyed. A separate quarterly cleanup (follow-up)
//     destroys disabled versions.
package main

import (
    "context"
    "crypto/rand"
    "flag"
    "fmt"
    "log/slog"
    "os"
    "sort"
    "time"

    secretmanager "cloud.google.com/go/secretmanager/apiv1"
    smpb "cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"

    "github.com/tesserix/marketplace-api/internal/arbitrage"
)

const (
    rotationAgeDays   = 30
    maxRetentionDays  = 61
    hmacKeyBytes      = 32 // 256-bit CSPRNG
)

func main() {
    secretPath := flag.String("secret", "", "projects/.../secrets/arbitrage-ip-hmac-key")
    flag.Parse()
    if *secretPath == "" {
        slog.Error("missing --secret flag")
        os.Exit(2)
    }

    ctx := context.Background()
    client, err := secretmanager.NewClient(ctx)
    if err != nil {
        slog.Error("secret manager client", "err", err)
        os.Exit(1)
    }
    defer client.Close()

    if err := run(ctx, client, *secretPath); err != nil {
        slog.Error("rotation failed", "err", err)
        os.Exit(1)
    }
    slog.Info("rotation complete")
}

func run(ctx context.Context, client *secretmanager.Client, secretPath string) error {
    src := &arbitrage.SecretManagerSource{Client: client, SecretPath: secretPath}
    vs, err := src.ListEnabled(ctx)
    if err != nil {
        return fmt.Errorf("list versions: %w", err)
    }
    sort.Slice(vs, func(i, j int) bool { return vs[i].CreatedAt.After(vs[j].CreatedAt) })

    if shouldRotate(vs) {
        payload := make([]byte, hmacKeyBytes)
        if _, err := rand.Read(payload); err != nil {
            return fmt.Errorf("csprng: %w", err)
        }
        _, err := client.AddSecretVersion(ctx, &smpb.AddSecretVersionRequest{
            Parent:  secretPath,
            Payload: &smpb.SecretPayload{Data: payload},
        })
        if err != nil {
            return fmt.Errorf("add version: %w", err)
        }
        slog.Info("added new version", "secret", secretPath)
    }

    for _, old := range versionsToDisable(vs, maxRetentionDays) {
        _, err := client.DisableSecretVersion(ctx, &smpb.DisableSecretVersionRequest{
            Name: old.Name,
        })
        if err != nil {
            return fmt.Errorf("disable %s: %w", old.Name, err)
        }
        slog.Info("disabled old version", "version", old.Name)
    }
    return nil
}

func shouldRotate(vs []arbitrage.KeyVersion) bool {
    if len(vs) == 0 {
        return true // first-time bootstrap
    }
    return time.Since(vs[0].CreatedAt) >= rotationAgeDays*24*time.Hour
}

func versionsToDisable(vs []arbitrage.KeyVersion, maxDays int) []arbitrage.KeyVersion {
    cutoff := time.Now().Add(-time.Duration(maxDays) * 24 * time.Hour)
    out := make([]arbitrage.KeyVersion, 0)
    for _, v := range vs {
        if v.CreatedAt.Before(cutoff) {
            out = append(out, v)
        }
    }
    return out
}
```

- [ ] **Step 3: K8s manifest stub**

`deploy/arbitrage-rotator.k8s.yaml` documents the required resource for the infra team:

```yaml
# Cloud Run Job triggered nightly by Cloud Scheduler.
# IAM:
#   - Cloud Run Job service account: roles/secretmanager.secretVersionAdder +
#     roles/secretmanager.secretVersionManager on /projects/tesserix-prod/secrets/arbitrage-ip-hmac-key
#   - Cloud Scheduler service account: roles/run.invoker on this job.
#
# Infra team: this file is a documentation stub. Canonical resource lives
# in tesserix-k8s via Terraform + ArgoCD.
apiVersion: batch/v1
kind: CronJob
metadata:
  name: arbitrage-rotator
  namespace: marketplace
spec:
  schedule: "0 3 * * *"  # 03:00 UTC daily
  concurrencyPolicy: Forbid
  successfulJobsHistoryLimit: 3
  failedJobsHistoryLimit: 3
  jobTemplate:
    spec:
      backoffLimit: 2
      template:
        spec:
          serviceAccountName: arbitrage-rotator
          restartPolicy: Never
          containers:
            - name: rotator
              image: asia-south1-docker.pkg.dev/tesserix-app/services/arbitrage-rotator:latest
              args:
                - --secret=projects/tesserix-prod/secrets/arbitrage-ip-hmac-key
              resources:
                requests: { cpu: 50m, memory: 64Mi }
                limits:   { cpu: 200m, memory: 128Mi }
```

- [ ] **Step 4: Run — expect PASS**

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/cmd/arbitrage-rotator/*.go
git add services/marketplace-api/deploy/arbitrage-rotator.k8s.yaml
git commit -m "feat(arbitrage): nightly key-rotation job with 30d/61d retention policy"
```

---

## Task 13: `LogPIIAccess` on every audit-row read

**Files:**
- Create: `services/marketplace-api/internal/arbitrage/pii_access.go`
- Modify: call sites in `recorder.go`, `appeal.go`, `quarterly_cron.go`, `handlers/admin/subscription.go`

**Spec references:** §18.8 ("Access scoped to `billing-ops` IAM role; every read logged").

**Purpose:** Centralise the audit-service emit so the compliance story is one package. Every exported method that reads `subscription_arbitrage_audit` must call `LogPIIAccess` first. Task 15 enforces this via grep.

- [ ] **Step 1: Implement `pii_access.go`**

```go
package arbitrage

import (
    "context"

    "github.com/google/uuid"
)

// PIIAccessEvent describes one read of the arbitrage audit table.
// The audit-service is the SoR; we just emit.
type PIIAccessEvent struct {
    Actor     uuid.UUID // billing-ops user; uuid.Nil for system jobs
    StoreID   uuid.UUID
    TenantID  uuid.UUID
    Operation string // e.g. "arbitrage_audit_read_admin_subscription"
    Note      string // optional free text
}

// PIILogger is the injection point; production wires to audit-service client.
type PIILogger interface {
    LogPIIAccess(ctx context.Context, evt PIIAccessEvent)
}

// NopPIILogger is a silent implementation useful in tests where PII logging
// is not under assertion.
type NopPIILogger struct{}

func (NopPIILogger) LogPIIAccess(context.Context, PIIAccessEvent) {}

// AuditServicePIILogger wraps the existing go-shared audit client.
type AuditServicePIILogger struct {
    Emitter AuditEmitter // matches audit.EmitStateTransition shape
    Source  string       // "marketplace-api/arbitrage"
}

// AuditEmitter is the minimal shape of internal/audit.Emitter that this
// package uses. Avoids pulling the whole package in just for one method.
type AuditEmitter interface {
    Emit(ctx context.Context, evt AuditEmittable)
}

type AuditEmittable struct {
    Kind     string
    Severity string
    Metadata map[string]string
    TenantID uuid.UUID
    StoreID  uuid.UUID
    ActorID  uuid.UUID
}

func (l AuditServicePIILogger) LogPIIAccess(ctx context.Context, evt PIIAccessEvent) {
    md := map[string]string{
        "operation": evt.Operation,
    }
    if evt.Note != "" {
        md["note"] = evt.Note
    }
    l.Emitter.Emit(ctx, AuditEmittable{
        Kind:     "arbitrage_pii_access",
        Severity: "info",
        Metadata: md,
        TenantID: evt.TenantID,
        StoreID:  evt.StoreID,
        ActorID:  evt.Actor,
    })
}
```

- [ ] **Step 2: Thread `PIILogger` into `Recorder`, `AppealService`, `QuarterlyAudit`, and the admin subscription handler**

Each constructor grows a `pii PIILogger` parameter. Every `db.Where(...).Find(&audit)` call grows a preceding `s.piiLogger.LogPIIAccess(ctx, PIIAccessEvent{...})` line.

- [ ] **Step 3: Test — `AuditServicePIILogger.LogPIIAccess` emits once with correct shape**

```go
func TestAuditServicePIILogger_EmitsOnce(t *testing.T) {
    spy := &spyEmitter{}
    logger := arbitrage.AuditServicePIILogger{Emitter: spy}
    logger.LogPIIAccess(context.Background(), arbitrage.PIIAccessEvent{
        Operation: "test", TenantID: uuid.New(), StoreID: uuid.New(), Actor: uuid.New(),
    })
    require.Len(t, spy.events, 1)
    require.Equal(t, "arbitrage_pii_access", spy.events[0].Kind)
}
```

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/internal/arbitrage/pii_access.go
git add -u
git commit -m "feat(arbitrage): LogPIIAccess audit emit on every arbitrage audit row read"
```

---

## Task 14: End-to-end success criterion #51 test

**Files:**
- Create: `services/marketplace-api/internal/arbitrage/criterion_51_test.go`

**Spec references:** §28 success criterion #51 — "IP hash: HMAC-SHA256 with Secret Manager key; salt rotation 30d preserves 31-day overlap; cross-window correlation severed beyond 31d."

**Purpose:** One integration test that proves the full HMAC + rotation + correlation story end-to-end. This is the single regression guard for the security claim.

- [ ] **Step 1: Write the test** (`//go:build integration`)

`TestCriterion51_HMACRotationPreservesOverlap` — one test proving the full chain:

1. **Setup:** `stubSource` with two versions (`v3` at day 0, `v2` at day -35). Build a `Hasher`.
2. **Hash twice under the same rotation window:** `r1 = hasher.Hash("203.0.113.42")` and assert `r1.KeyVersion` references `v3`.
3. **Rotate:** prepend a fresh `v4` (day +1h) to the source; rebuild loader + hasher with TTL=1ns, sleep 2ns.
4. **Assert rotation severs correlation:** `r2 = hasher2.Hash("203.0.113.42")`; `r2.Hex != r1.Hex`.
5. **Assert overlap window preserves correlation:** compute `manualHMAC(v3.Payload, "203.0.113.42")` using `hmac.New(sha256.New, key)` directly; it equals `r1.Hex` — proves that during the 61d window a retained version can reproduce the earlier digest.
6. **Assert past-retention severance:** shrink the source to `[v4]` (simulates rotator having disabled older versions); `KeyLoader.All()` returns length 1 — correlation with any ip_hash produced under v3/v2 is now unrecoverable.

This single test is the regression guard for spec §28 criterion #51.

- [ ] **Step 2: Run — expect PASS**

- [ ] **Step 3: Commit**

```bash
git add services/marketplace-api/internal/arbitrage/criterion_51_test.go
git commit -m "test(arbitrage): end-to-end criterion #51 — HMAC rotation + overlap + sever"
```

---

## Task 15: Final scrub — raw IP persistence + HMAC call-site audit

- [ ] **Step 1: Grep for raw IP writes**

```bash
cd services/marketplace-api
grep -RnE '\braw_ip\b|\bRemoteAddr\b' internal/ cmd/ \
  | grep -v "_test.go" \
  | grep -v "internal/arbitrage/ipcountry.go" \
  | grep -v "internal/arbitrage/hmac.go" \
  | grep -v "internal/arbitrage/recorder.go"
```
Expected: empty. The only legitimate references are in the three files that *consume* raw IP and never persist it.

- [ ] **Step 2: Grep for HMAC construction outside `arbitrage/hmac.go`**

```bash
grep -Rn "hmac.New" internal/ cmd/ \
  | grep -v "_test.go" \
  | grep -v "internal/arbitrage/hmac.go"
```
Expected: empty. Any other HMAC usage is a bug — route it through `arbitrage.Hasher`.

- [ ] **Step 3: Grep for the rejected pattern**

```bash
grep -RnE 'sha256\.(Sum|Sum256).*salt|salt.*sha256' internal/ cmd/
```
Expected: empty. The spec-rejected "SHA-256 with concatenated salt" construction must not appear anywhere.

- [ ] **Step 4: Grep for `subscription_arbitrage_audit` reads without `LogPIIAccess`**

```bash
grep -Rn "SubscriptionArbitrageAudit" internal/ \
  | grep -v "_test.go" \
  | grep -v "LogPIIAccess" \
  | grep -vE "internal/arbitrage/(models|evaluator|countries|hmac|keyloader|ipcountry)\.go"
```
Manually review any remaining hits — every one must be immediately preceded by a `LogPIIAccess` call in the same function.

- [ ] **Step 5: Run full suite**

```bash
go test -tags=integration ./... -count=1
go build ./...
```
Expected: green.

- [ ] **Step 6: Final commit**

```bash
git add -u
git commit --allow-empty -m "chore(arbitrage): final scrub — no raw IP persistence, no rogue HMAC call sites"
```

---

## Final verification

- [ ] `go build ./...` clean.
- [ ] `go test -tags=integration ./...` all green.
- [ ] `arbitrage.Evaluate` is pure — same input yields same output (verified in `evaluator_test.go`).
- [ ] Every webhook handler that sees subscription creation calls `Recorder.RecordIfFlagged`.
- [ ] Every read of `subscription_arbitrage_audit` outside the package is preceded by `LogPIIAccess`.
- [ ] The HMAC hot path calls `hmac.New(sha256.New, key)` — no `sha256.Sum256(append(salt, ip...))` anywhere.
- [ ] `KeyLoader.Latest()` returns the newest enabled version; `KeyLoader.All()` returns every version in the 61-day window.
- [ ] `cmd/arbitrage-rotator` emits a new version at ≥30d + disables versions at ≥61d.
- [ ] `GET /admin/stores/:storeId/subscription` payload includes `arbitrage_flag` + `latest_arbitrage_audit` when flagged; `arbitrage_flag=false, latest_arbitrage_audit=null` otherwise.
- [ ] `POST /admin/stores/:storeId/arbitrage-appeal` returns 200 on a valid submission, 404 on no-open-flag, 400 on bad jurisdiction.
- [ ] `QuarterlyAudit.Run` clears `ongoing` flags under per-store advisory lock.
- [ ] Counter `subscription.arbitrage_flagged` increments on every `RecordIfFlagged` that writes; `subscription.arbitrage_false_positive_cleared` increments on every `QuarterlyAudit`-cleared row.
- [ ] Raw IP string appears in memory only inside `CFIPCountryFromGin` callers and `Hasher.Hash` — never persisted, never logged.
- [ ] Spec success criterion #51 test passes: same IP hashes differently across rotation; within 61d retention, earlier hashes re-producible; beyond retention, severed.

## What's now unlocked

- **P11** (cancellation + save-offer) can consume `arbitrage_flag = true` + `resolution = reprice_developed` as a cancellation trigger on merchant refusal.
- **P16** (admin UI) reads `arbitrage_flag` + `latest_arbitrage_audit` from the subscription payload to render the banner + appeal form.
- **P17** (observability) consumes `subscription.arbitrage_flagged` + `subscription.arbitrage_false_positive_cleared` for dashboards + the >5× baseline alert; reads `arbitrage_pii_access` audit events for compliance auditing.
- **Billing-ops tooling** (separate project) reads the `subscription_arbitrage_audit` partial index `WHERE resolution = 'ongoing'` for the review queue; can `UPDATE ... SET resolution = 'false_positive_cleared' | 'reprice_developed'` to close a case. Writes MUST go through an IAM-gated internal endpoint that emits `LogPIIAccess` on every mutation (follow-up ticket).

## Execution handoff

Plan complete. Execute with **superpowers:subagent-driven-development** (recommended) or **superpowers:executing-plans**. Runs after P1 (data model) and P2 (webhook dispatcher) are green; independent of P3/P4/P5/P6 but integrates cleanly with each.

Before kickoff, confirm infra has:
1. Created the Secret Manager secret at `/projects/tesserix-prod/secrets/arbitrage-ip-hmac-key` (initially empty — the rotator creates the first version on first run).
2. Bound the `marketplace-api` workload-identity service account to `roles/secretmanager.secretAccessor` on that secret.
3. Bound the `arbitrage-rotator` service account to `roles/secretmanager.secretVersionManager` + `roles/secretmanager.secretVersionAdder` on the same secret.
4. Confirmed Cloudflare Tunnel + Istio pass the `CF-IPCountry` and `CF-Connecting-IP` headers through to the handler unmodified.

If any of those are missing, stop and file the infra ticket before writing code — the HMAC hot path cannot boot without (1) + (2), and the triangulation signal is wrong without (4).
