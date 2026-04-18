# P5 — Trial Lifecycle + Deferred-Charge Card-Add Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the trial loop. A merchant signs up through a hardened endpoint (email verification + reCAPTCHA + disposable-email blocklist + per-IP/per-email rate limits), stays in `trialing` for up to 90 days, can add a card at any point without being charged immediately (§4.7 Council finding #11), gets calm day‑60/75/85 nudges, and — if day 90 arrives without a card — transitions `trialing → expired` via the P3 state machine. Migration fast-path (§5.1.1) lets CSMs accept prior-platform evidence and shorten the tax-ID window from 14d to 48h. The P2 `handleCheckoutSessionCompleted` raw UPDATE (left in place by P3) is finally retired and routed through `statemachine.Transition`.

**Architecture:** A new `internal/signup` package owns the signup HTTP handler and its three gates (`verification`, `recaptcha`, `blocklist`) plus two rate limiters (per-IP 3/24h, per-email 1-ever). A new `internal/billing/trial` package owns the deferred-charge Stripe flow: given a store + plan + currency choice, it calls `stripe.CreateSubscription` with `trial_end = signup_date + 90d` and `proration_behavior=none`, keyed with P2's `SubscriptionIdempotencyKey(storeID, plan, period)`. A new `internal/billing/migration` package owns the fast-path review queue (merchant submit + CSM approve/reject) and the tax-ID window shortener. Four crons wire into the existing `robfig/cron/v3` scheduler P2 already registered: `trial_banner_cron`, `trial_expiry_cron`, `signup_anomaly_cron`, `trial_activation_cron`. Every cron is a pure function of `(signup_date, now(), row state)` and is safe to re-run (§17.5). `handleCheckoutSessionCompleted` is refactored to call `statemachine.Transition(signup → trialing)` — the raw UPDATE P3 deferred to us is deleted.

**Tech Stack:** Go 1.26, Gin, GORM, PostgreSQL, `robfig/cron/v3` (wired by P2), Redis (for rate-limit INCR), SendGrid (for nudge + expiry emails). No new infra.

**Spec:** [`docs/superpowers/specs/2026-04-17-subscription-model-design.md`](../specs/2026-04-17-subscription-model-design.md) — §4.7 (Stripe-native fallback + trial card-add), §5 (trial mechanics), §5.1 (signup gates), §5.1.1 (migration fast-path + 48h expedited), §5.3 (timeline), §5.4 (store-closed reference — Worker is P12), §17.5 (cron idempotency).

**Depends on:** P1 (data model, `WithAdvisoryLock`, `EmitStateTransition`), P2 (Stripe `Client` + `SubscriptionIdempotencyKey` + webhook dispatcher), P3 (`statemachine.Transition`, `RequireActive` already correctly excluding `payment_action_required`).

**Related plans:**
- **P6** dunning — owns `active → past_due → active|expired`; P5 hands off a clean `trialing → active` or `trialing → expired`.
- **P7** tax-ID validation — owns registry lookups; P5 fast-path only shortens the window, does NOT waive validation.
- **P9** campaign-email ramp + drip — owns marketing emails; P5 owns only three transactional trial nudges + the expiry notice.
- **P12** Cloudflare Worker `closed.html` — serves the storefront page for expired stores.
- **P16** admin UI — renders the banner, migration form, and add-card CTA; P5 only ships JSON.
- **P17** observability — reads `subscription.trial.product_created_day_30` + `signup_anomaly_log`; P5 only emits.

---

## Scope Check

In scope:
1. `POST /api/signup` hardening — email-verification token, reCAPTCHA Enterprise verify, disposable-email blocklist (weekly refresh), rate-limit `3/IP/24h` + `1/email` (one-ever).
2. `POST /admin/stores/:storeId/billing/subscription` — deferred-charge Stripe subscription create with `trial_end = signup_date + 90d`, `proration_behavior=none`, idempotency `subscription:<store>:<plan>:<period>`. Persists `stripe_subscription_id`; does NOT flip status (webhook owns the transition).
3. `trial_banner_cron` — daily; nudges at day 60, 75, 85 via SendGrid; writes `trial_banner_state` column for admin UI.
4. `trial_expiry_cron` — daily; for every `trialing` store where `now() - signup_date >= 90d` AND `stripe_subscription_id IS NULL`, call `statemachine.Transition(trialing → expired)` and send expiry email.
5. `POST /admin/stores/:storeId/migration-fast-path/submit` + `POST /internal/csm/migration-fast-path/:id/review` — CSM queue; on approve, sets `tax_id_window_shortened_at` (14d → 48h). Does NOT waive validation (P7 owns that).
6. Refactor `handleCheckoutSessionCompleted` — replace raw UPDATE (documented as "P5 owns this" in P3 Task 3 Step 4) with `statemachine.Transition(signup → trialing)` / `signup → active`.
7. `signup_anomaly_cron` — daily; emits Slack alert + Prometheus counter if yesterday's signup count > 50.
8. `trial_activation_cron` — daily; increments `subscription.trial.product_created_day_30` for every trialing store reaching day 30 with ≥1 product. Counter only — dashboards are P17.

Out of scope:
- Campaign-email trial ramp (day 3→4, 7→8) — **P9.**
- Actual tax-ID validation (registry lookups, SEA manual-review queue) — **P7.**
- Dunning ladder — **P6.**
- Cloudflare Worker `closed.html` — **P12.**
- Admin UI for banner, migration form, card-add CTA — **P16.**
- Trial dashboards — **P17** reads the counter; we only emit.
- Save-offer / cancel on trial exit — **P11.**

---

## File Structure

### Create

- `services/marketplace-api/internal/signup/handler.go` + `_test.go`
- `services/marketplace-api/internal/signup/recaptcha.go` + `_test.go`
- `services/marketplace-api/internal/signup/blocklist.go` + `_test.go`
- `services/marketplace-api/internal/signup/blocklist_refresh.go`
- `services/marketplace-api/internal/signup/ratelimit.go` + `_test.go`
- `services/marketplace-api/internal/signup/anomaly_cron.go` + `_test.go`
- `services/marketplace-api/internal/billing/trial/subscribe.go` + `_test.go`
- `services/marketplace-api/internal/billing/trial/banner_cron.go` + `_test.go`
- `services/marketplace-api/internal/billing/trial/expiry_cron.go` + `_test.go`
- `services/marketplace-api/internal/billing/trial/activation_cron.go` + `_test.go`
- `services/marketplace-api/internal/billing/trial/e2e_deferred_charge_test.go`
- `services/marketplace-api/internal/billing/migration/repository.go`
- `services/marketplace-api/internal/billing/migration/handler.go` + `_test.go`
- `services/marketplace-api/internal/billing/stripe/subscription_create.go` + `_test.go` — add `CreateSubscription` (P2 only shipped `GetSubscription`)
- `services/marketplace-api/internal/email/templates/{trial_day_60,trial_day_75,trial_day_85,trial_expired,migration_fast_path_approved,migration_fast_path_rejected}.{txt,html}`
- `services/marketplace-api/migrations/0050_migration_fast_path_reviews.{up,down}.sql`
- `services/marketplace-api/migrations/0051_trial_banner_state.{up,down}.sql`
- `services/marketplace-api/migrations/0052_tax_id_window_shortened_at.{up,down}.sql`
- `services/marketplace-api/migrations/0053_signup_anomaly_log.{up,down}.sql`
- `services/marketplace-api/migrations/0054_trial_activation_marker.{up,down}.sql`
- `data/disposable-emails.txt` — vendored copy of [disposable-email-domains](https://github.com/disposable-email-domains/disposable-email-domains) (MIT)
- `services/marketplace-api/scripts/verify-no-raw-status-updates.sh`

### Modify

- `services/marketplace-api/internal/billing/dispatch/handlers.go` + `_test.go` — retire `handleCheckoutSessionCompleted` raw UPDATE
- `services/marketplace-api/internal/subscription/readonly/allowlist.go` — add `GET /admin/stores/:storeId/migration-fast-path/*path`
- `services/marketplace-api/cmd/marketplace-api/main.go` — wire the four crons + new HTTP routes
- `services/marketplace-api/internal/handlers/admin/routes.go` — add `/billing/subscription` + `/migration-fast-path/*`

### Delete

None. Every P5 change is additive or refactors an explicitly-deferred P3 leave-behind.

---

## Task Sequence Overview

| # | Task | Depends on |
|---|---|---|
| 1 | Migrations 0050–0052 (+0053/0054 inline with their tasks) | — |
| 2 | `stripe.CreateSubscription` helper with `trial_end` + idempotency | P2 |
| 3 | Signup rate limiter (per-IP 3/24h + per-email 1-ever) | — |
| 4 | Signup reCAPTCHA Enterprise verifier | — |
| 5 | Disposable-email blocklist + weekly refresh cron | — |
| 6 | `POST /api/signup` handler composing gates + limiters | 3, 4, 5 |
| 7 | `trial.Subscribe` — deferred-charge flow | 2 |
| 8 | `POST /admin/stores/:storeId/billing/subscription` | 7 |
| 9 | `trial_banner_cron` + day 60/75/85 nudge emails | 1 |
| 10 | `trial_expiry_cron` + expired email | 1, P3 |
| 11 | Migration fast-path repo + submit + CSM review | 1 |
| 12 | Refactor `handleCheckoutSessionCompleted` → state machine | P3 |
| 13 | `signup_anomaly_cron` | — |
| 14 | `trial_activation_cron` | — |
| 15 | Main-wiring — register crons + routes | 9, 10, 13, 14 |
| 16 | E2E integration: card day 45 → charge day 90 (criterion #46) | 7, 8, 12 |

---

## Reusable patterns

**A. Advisory-lock around every state-machine call.** Every `statemachine.Transition` in P5 runs inside `subscription.WithAdvisoryLock(ctx, db, storeID, fn)` (P1 Task 13). Crons are especially exposed — two pods may both elect to expire the same store. The lock serialises; the loser sees `ErrCASConflict` and treats as no-op. No extra distributed lock.

**B. Cron idempotency shape (§17.5).** Every cron is:
```
rows = SELECT … FROM store_subscriptions WHERE <pure time predicate> AND <row-state predicate>
for each row: WithAdvisoryLock(storeID) { statemachine.Transition OR CAS UPDATE on a nudge-state column }
```
Time predicate is always `date_trunc('day', signup_date) = date_trunc('day', now() - interval 'N day')` (exact day, never `>=`). Running twice same UTC day touches each row at most once — after first pass the row either transitions out of `trialing` or flips its banner-state column, and both predicates exclude it.

**C. Stripe subscription idempotency.** Reuse P2's `stripe.SubscriptionIdempotencyKey(storeID, plan, period)` — collision-free across retries. Adding the card twice at the same plan reuses the key; Stripe returns the existing subscription.

**D. Three gates + two limiters + one handler.** Strict order, fail-fast: `ratelimit.CheckIP → blocklist.Check → recaptcha.Verify → ratelimit.CheckEmail → verifier.SendSignupVerification → Create row`. Every gate maps to the same generic `{"error":"signup_blocked"}` response so attackers cannot probe which gate tripped.

**E. Email templates = text + HTML parallel.** Under `internal/email/templates/`, named by lifecycle event. Copy is editorial/calm per brand voice — no exclamation marks, no countdowns, no "ACT NOW". One moss link per message.

**F. Crons on the shared `*cron.Cron`.** P2 already constructed the scheduler and registered the orphan resolver. P5 calls `scheduler.AddFunc(Spec, ...)` four more times in `cmd/marketplace-api/main.go`. Spec strings live as package constants at the top of each cron file.

**G. Internal auth for CSM endpoint.** The `/internal/csm/…` review endpoint uses P2's `auth.HeaderTrustAuth(cfg.InternalSecret)` middleware. No FGA — the trust boundary is network-level (only services behind Istio can reach `/internal/`).

---

## Task 1: Migrations

**Files:** `migrations/0050_migration_fast_path_reviews.{up,down}.sql`, `0051_trial_banner_state.{up,down}.sql`, `0052_tax_id_window_shortened_at.{up,down}.sql`. (0053 and 0054 land with their respective tasks.)

**Spec references:** §5.1.1, §5.3.

- [ ] **Step 1: Write 0050**

```sql
-- up
CREATE TABLE migration_fast_path_reviews (
    id              UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID         NOT NULL,
    store_id        UUID         NOT NULL REFERENCES store_subscriptions(store_id) ON DELETE CASCADE,
    evidence_type   TEXT         NOT NULL CHECK (evidence_type IN ('whois_domain', 'platform_screenshot')),
    evidence_url    TEXT         NOT NULL,                         -- signed GCS URL
    prior_platform  TEXT         NULL,                             -- "shopify" | "woocommerce" | "bigcommerce"
    whois_domain    TEXT         NULL,                             -- only when evidence_type = whois_domain
    status          TEXT         NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending', 'approved', 'rejected')),
    reviewer_id     UUID         NULL,
    reviewer_notes  TEXT         NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
    reviewed_at     TIMESTAMPTZ  NULL,
    CONSTRAINT only_one_open_per_store
        EXCLUDE (store_id WITH =) WHERE (status = 'pending')
);
CREATE INDEX idx_mfpr_pending ON migration_fast_path_reviews (status, created_at) WHERE status = 'pending';
CREATE INDEX idx_mfpr_store   ON migration_fast_path_reviews (store_id);
-- down
DROP TABLE IF EXISTS migration_fast_path_reviews;
```

- [ ] **Step 2: Write 0051**

```sql
-- up: banner state column doubles as the cron's idempotency marker (§17.5).
ALTER TABLE store_subscriptions
    ADD COLUMN trial_banner_state        TEXT        NULL
        CHECK (trial_banner_state IN ('none', 'day_60', 'day_75', 'day_85')),
    ADD COLUMN trial_banner_state_set_at TIMESTAMPTZ NULL;
-- down
ALTER TABLE store_subscriptions
    DROP COLUMN IF EXISTS trial_banner_state_set_at,
    DROP COLUMN IF EXISTS trial_banner_state;
```

- [ ] **Step 3: Write 0052**

```sql
-- up: NULL = normal 14d window; non-NULL = 48h shortened window. P7 reads this.
-- Clock is the merchant's signup_date, NOT approval time — we honour the original window origin.
ALTER TABLE store_subscriptions
    ADD COLUMN tax_id_window_shortened_at TIMESTAMPTZ NULL;
-- down
ALTER TABLE store_subscriptions DROP COLUMN IF EXISTS tax_id_window_shortened_at;
```

- [ ] **Step 4: Apply + commit**

```bash
cd services/marketplace-api
migrate -path migrations -database "$DATABASE_URL" up
git add migrations/005{0,1,2}_*.sql
git commit -m "feat(migrations): migration fast-path reviews + trial banner state + tax-ID window shortener"
```

---

## Task 2: `stripe.CreateSubscription` helper

**Files:** `internal/billing/stripe/subscription_create.go` + `_test.go`

**Spec references:** §4.7 Council finding #11.

P2 only shipped `GetSubscription`. The create side is ours — the one caller (trial card-add) is a P5 concept.

- [ ] **Step 1: Failing test — `trial_end` sent, idempotency key stable**

```go
func TestCreateSubscription_SendsTrialEndAndIdempotencyKey(t *testing.T) {
    trialEnd := time.Date(2026, 7, 17, 0, 0, 0, 0, time.UTC).Unix()
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        require.Equal(t, "/v1/subscriptions", r.URL.Path)
        require.Equal(t, "subscription:store_abc:starter:monthly", r.Header.Get("Idempotency-Key"))
        body, _ := io.ReadAll(r.Body)
        form, _ := url.ParseQuery(string(body))
        require.Equal(t, "cus_123", form.Get("customer"))
        require.Equal(t, "price_starter_monthly_gbp", form.Get("items[0][price]"))
        require.Equal(t, "none", form.Get("proration_behavior"))
        require.Equal(t, strconv.FormatInt(trialEnd, 10), form.Get("trial_end"))
        _, _ = w.Write([]byte(`{"id":"sub_created","status":"trialing"}`))
    }))
    defer srv.Close()

    c := stripe.New("sk_test_x"); c.SetBaseURLForTesting(srv.URL)
    s, err := stripe.CreateSubscription(context.Background(), c, stripe.CreateSubscriptionInput{
        StoreID: "store_abc", Plan: "starter", Period: "monthly",
        CustomerID: "cus_123", PriceID: "price_starter_monthly_gbp", TrialEnd: trialEnd,
    })
    require.NoError(t, err)
    require.Equal(t, "sub_created", s.ID)
}

func TestCreateSubscription_IdempotencyKeyStableAcrossRetries(t *testing.T) {
    // Two calls same (store, plan, period) → identical Idempotency-Key header.
}
```

- [ ] **Step 2: Run — expect FAIL; then implement `subscription_create.go`**

```go
package stripe

import (
    "context"
    "encoding/json"
    "fmt"
    "net/url"
    "strconv"
)

type CreateSubscriptionInput struct {
    StoreID    string // for idempotency key
    Plan       string // "starter"|"studio"|"pro"
    Period     string // "monthly"|"annual"
    CustomerID string
    PriceID    string
    TrialEnd   int64  // Unix seconds — always signup_date + 90d for trial card-add
}

// CreateSubscription POSTs /v1/subscriptions with trial_end, proration_behavior=none,
// and a stable idempotency key. Stripe returns a "trialing" subscription whose first
// invoice is scheduled at trial_end. No charge at call time.
//
// Callers persist the returned Subscription.ID onto store_subscriptions.stripe_subscription_id.
// The state transition (signup|trialing → active) happens asynchronously via the
// customer.subscription.created webhook routed through statemachine.Transition in P2.
func CreateSubscription(ctx context.Context, c *Client, in CreateSubscriptionInput) (*Subscription, error) {
    if in.CustomerID == "" || in.PriceID == "" {
        return nil, fmt.Errorf("stripe: CreateSubscription: customer + price required")
    }
    if in.TrialEnd <= 0 {
        return nil, fmt.Errorf("stripe: CreateSubscription: trial_end required (signup_date + 90d)")
    }
    v := url.Values{}
    v.Set("customer", in.CustomerID)
    v.Set("items[0][price]", in.PriceID)
    v.Set("proration_behavior", "none")
    v.Set("trial_end", strconv.FormatInt(in.TrialEnd, 10))
    v.Set("metadata[mark8ly_store_id]", in.StoreID)
    v.Set("metadata[mark8ly_plan]", in.Plan)
    v.Set("metadata[mark8ly_period]", in.Period)

    key := SubscriptionIdempotencyKey(in.StoreID, in.Plan, in.Period)
    body, err := c.PostForm(ctx, "/v1/subscriptions", key, v)
    if err != nil { return nil, err }
    var s Subscription
    if err := json.Unmarshal(body, &s); err != nil {
        return nil, fmt.Errorf("stripe: decode subscription: %w", err)
    }
    return &s, nil
}
```

- [ ] **Step 3: Run — expect PASS; commit**

```bash
git add internal/billing/stripe/subscription_create{,_test}.go
git commit -m "feat(billing): stripe.CreateSubscription with trial_end + stable idempotency key"
```

---

## Task 3: Signup rate limiter (Redis)

**Files:** `internal/signup/ratelimit.go` + `_test.go`

**Spec references:** §5.1 — "Rate-limit: 3/IP/24h, 1/email".

- [ ] **Step 1: Failing tests — IP 3/24h, email 1-ever, case-insensitive, 24h reset, different IPs independent**

```go
func TestCheckIP_AllowsThree_RejectsFourth(t *testing.T) { /* …3 pass, 4th → ErrIPLimitExceeded… */ }
func TestCheckIP_DifferentIPsIndependent(t *testing.T)    { /* …ip A hits limit, ip B passes… */ }
func TestCheckIP_ResetsAfter24h(t *testing.T)             { /* …testredis.Advance(25h); next call passes… */ }
func TestCheckEmail_OncePerEmailEver(t *testing.T)        { /* …first ok; second → ErrEmailAlreadyUsed… */ }
func TestCheckEmail_CaseInsensitive(t *testing.T)         { /* …"A@B" blocks "a@b"… */ }
func TestReleaseEmail_AllowsRetry(t *testing.T)           { /* …Release then CheckEmail passes… */ }
```

- [ ] **Step 2: Run — expect FAIL; then implement `ratelimit.go`**

```go
package signup

import (
    "context"
    "errors"
    "fmt"
    "strings"
    "time"

    "github.com/redis/go-redis/v9"
)

var (
    ErrIPLimitExceeded  = errors.New("signup: IP rate limit exceeded (3/24h)")
    ErrEmailAlreadyUsed = errors.New("signup: email has already completed a signup")
)

type RateLimiter struct {
    r   *redis.Client
    now func() time.Time
}

func NewRateLimiter(r *redis.Client, now func() time.Time) *RateLimiter {
    if now == nil { now = time.Now }
    return &RateLimiter{r: r, now: now}
}

// CheckIP: atomic INCR; if count > 3 → ErrIPLimitExceeded. TTL 25h so UTC edge is safe.
func (rl *RateLimiter) CheckIP(ctx context.Context, ip string) error {
    if ip == "" { return fmt.Errorf("signup: ratelimit: empty IP") }
    key := fmt.Sprintf("signup:ip:%s:%s", ip, rl.now().UTC().Format("20060102"))
    count, err := rl.r.Incr(ctx, key).Result()
    if err != nil { return fmt.Errorf("signup: ratelimit incr: %w", err) }
    if count == 1 { _ = rl.r.Expire(ctx, key, 25*time.Hour).Err() }
    if count > 3 { return ErrIPLimitExceeded }
    return nil
}

// CheckEmail: 1-ever via SETNX. TTL is long (10y). Also re-verified against
// store_subscriptions.email in the handler as a belt.
func (rl *RateLimiter) CheckEmail(ctx context.Context, email string) error {
    if email == "" { return fmt.Errorf("signup: ratelimit: empty email") }
    norm := strings.ToLower(strings.TrimSpace(email))
    ok, err := rl.r.SetNX(ctx, "signup:email:"+norm, "1", 10*365*24*time.Hour).Result()
    if err != nil { return fmt.Errorf("signup: ratelimit setnx: %w", err) }
    if !ok { return ErrEmailAlreadyUsed }
    return nil
}

// ReleaseEmail rolls back a CheckEmail claim. Called on any downstream failure
// so one transient SendGrid 500 doesn't burn the email forever.
func (rl *RateLimiter) ReleaseEmail(ctx context.Context, email string) {
    _ = rl.r.Del(ctx, "signup:email:"+strings.ToLower(strings.TrimSpace(email))).Err()
}
```

- [ ] **Step 3: Run — expect PASS; commit**

```bash
git add internal/signup/ratelimit{,_test}.go
git commit -m "feat(signup): Redis-backed per-IP 3/24h + per-email 1-ever rate limiter"
```

---

## Task 4: reCAPTCHA Enterprise verifier

**Files:** `internal/signup/recaptcha.go` + `_test.go`

**Spec references:** §5.1.

- [ ] **Step 1: Failing tests — valid pass, low score rejected, action mismatch rejected**

```go
func TestRecaptcha_Verify_Passes(t *testing.T)                { /* valid token, score 0.9 → nil */ }
func TestRecaptcha_Verify_LowScoreFails(t *testing.T)         { /* score 0.2 → ErrRecaptchaLowScore */ }
func TestRecaptcha_Verify_WrongActionFails(t *testing.T)      { /* action "login" vs expected "signup" → ErrRecaptchaActionMismatch */ }
func TestRecaptcha_Verify_InvalidTokenFails(t *testing.T)     { /* tokenProperties.valid=false → ErrRecaptchaInvalidToken */ }
```

- [ ] **Step 2: Implement `recaptcha.go`**

```go
package signup

import (
    "bytes"
    "context"
    "encoding/json"
    "errors"
    "fmt"
    "net/http"
    "time"
)

var (
    ErrRecaptchaInvalidToken   = errors.New("signup: recaptcha token invalid")
    ErrRecaptchaLowScore       = errors.New("signup: recaptcha risk score below threshold")
    ErrRecaptchaActionMismatch = errors.New("signup: recaptcha action mismatch")
)

type RecaptchaConfig struct {
    Project, SiteKey, APIKey string
    BaseURL                  string // default: https://recaptchaenterprise.googleapis.com
    MinScore                 float64 // default: 0.5
    ExpectedAction           string  // "signup"
    HTTPClient               *http.Client
}

type RecaptchaVerifier struct{ cfg RecaptchaConfig }

func NewRecaptchaVerifier(cfg RecaptchaConfig) *RecaptchaVerifier {
    if cfg.BaseURL == "" { cfg.BaseURL = "https://recaptchaenterprise.googleapis.com" }
    if cfg.MinScore == 0 { cfg.MinScore = 0.5 }
    if cfg.HTTPClient == nil { cfg.HTTPClient = &http.Client{Timeout: 5 * time.Second} }
    return &RecaptchaVerifier{cfg: cfg}
}

func (v *RecaptchaVerifier) Verify(ctx context.Context, token string) error {
    body, _ := json.Marshal(map[string]any{
        "event": map[string]any{"token": token, "siteKey": v.cfg.SiteKey, "expectedAction": v.cfg.ExpectedAction},
    })
    u := fmt.Sprintf("%s/v1/projects/%s/assessments?key=%s", v.cfg.BaseURL, v.cfg.Project, v.cfg.APIKey)
    req, _ := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    resp, err := v.cfg.HTTPClient.Do(req)
    if err != nil { return fmt.Errorf("signup: recaptcha: %w", err) }
    defer resp.Body.Close()
    if resp.StatusCode >= 400 { return fmt.Errorf("signup: recaptcha: http %d", resp.StatusCode) }

    var out struct {
        TokenProperties struct { Valid bool; Action string } `json:"tokenProperties"`
        RiskAnalysis    struct { Score float64 }              `json:"riskAnalysis"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&out); err != nil { return err }
    if !out.TokenProperties.Valid                  { return ErrRecaptchaInvalidToken }
    if out.TokenProperties.Action != v.cfg.ExpectedAction { return ErrRecaptchaActionMismatch }
    if out.RiskAnalysis.Score < v.cfg.MinScore     { return ErrRecaptchaLowScore }
    return nil
}
```

- [ ] **Step 3: Run — expect PASS; commit**

```bash
git add internal/signup/recaptcha{,_test}.go
git commit -m "feat(signup): reCAPTCHA Enterprise verifier with score + action checks"
```

---

## Task 5: Disposable-email blocklist + weekly refresh

**Files:** `internal/signup/blocklist.go` + `_test.go`, `internal/signup/blocklist_refresh.go`, `data/disposable-emails.txt`

**Spec references:** §5.1 — "Disposable email blocklist refreshed weekly".

- [ ] **Step 1: Vendor seed file**

Copy `disposable_email_blocklist.conf` from [disposable-email-domains/disposable-email-domains](https://github.com/disposable-email-domains/disposable-email-domains) (MIT) to `data/disposable-emails.txt`, one domain per line, lowercased.

- [ ] **Step 2: Failing tests**

```go
func TestBlocklist_RejectsKnownDisposable(t *testing.T)        { /* mailinator.com → ErrDisposableEmail */ }
func TestBlocklist_AllowsLegitimate(t *testing.T)               { /* gmail.com, brand.co.uk → nil */ }
func TestBlocklist_CaseInsensitive(t *testing.T)                { /* User@MAILINATOR.COM blocked */ }
func TestBlocklist_MalformedEmailRejected(t *testing.T)          { /* "not-an-email" → ErrMalformedEmail */ }
func TestBlocklist_HotReplace_Atomic(t *testing.T)               { /* Replace() swaps set without read-lock */ }
```

- [ ] **Step 3: Implement `blocklist.go` — atomic.Pointer swap, no read lock needed**

```go
package signup

import (
    "bufio"
    "errors"
    "io"
    "strings"
    "sync/atomic"
)

var (
    ErrDisposableEmail = errors.New("signup: disposable email domain rejected")
    ErrMalformedEmail  = errors.New("signup: malformed email")
)

type Blocklist struct{ set atomic.Pointer[map[string]struct{}] }

func NewBlocklistFromReader(r io.Reader) *Blocklist { b := &Blocklist{}; b.Replace(r); return b }

func (b *Blocklist) Replace(r io.Reader) {
    m := make(map[string]struct{}, 10_000)
    sc := bufio.NewScanner(r)
    for sc.Scan() {
        line := strings.TrimSpace(strings.ToLower(sc.Text()))
        if line == "" || strings.HasPrefix(line, "#") { continue }
        m[line] = struct{}{}
    }
    b.set.Store(&m)
}

func (b *Blocklist) Check(email string) error {
    at := strings.LastIndex(email, "@")
    if at < 1 || at == len(email)-1 { return ErrMalformedEmail }
    domain := strings.ToLower(email[at+1:])
    m := b.set.Load()
    if m == nil { return nil } // not yet loaded — fail open, log at startup
    if _, ok := (*m)[domain]; ok { return ErrDisposableEmail }
    return nil
}
```

- [ ] **Step 4: Implement `blocklist_refresh.go` — weekly cron, remote with local fallback**

```go
package signup

import (
    "context"
    "net/http"
    "os"
    "time"

    "github.com/robfig/cron/v3"
)

const WeeklyRefreshSpec = "0 3 * * MON"
const RefreshSource = "https://raw.githubusercontent.com/disposable-email-domains/disposable-email-domains/master/disposable_email_blocklist.conf"

type BlocklistRefresher struct {
    b     *Blocklist
    local string
    http  *http.Client
}

func NewBlocklistRefresher(b *Blocklist, localPath string) *BlocklistRefresher {
    return &BlocklistRefresher{b: b, local: localPath, http: &http.Client{Timeout: 10 * time.Second}}
}

// Refresh tries remote; on any failure falls back to the vendored file.
// Never returns an error — a stale blocklist is better than a crashed service.
func (rf *BlocklistRefresher) Refresh(ctx context.Context) {
    req, _ := http.NewRequestWithContext(ctx, http.MethodGet, RefreshSource, nil)
    if resp, err := rf.http.Do(req); err == nil && resp.StatusCode == 200 {
        defer resp.Body.Close()
        rf.b.Replace(resp.Body)
        return
    }
    if f, err := os.Open(rf.local); err == nil {
        defer f.Close()
        rf.b.Replace(f)
    }
}

func (rf *BlocklistRefresher) Register(c *cron.Cron) error {
    _, err := c.AddFunc(WeeklyRefreshSpec, func() {
        ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
        defer cancel()
        rf.Refresh(ctx)
    })
    return err
}
```

- [ ] **Step 5: Run — expect PASS; commit**

```bash
git add internal/signup/blocklist{,_test,_refresh}.go data/disposable-emails.txt
git commit -m "feat(signup): disposable-email blocklist with weekly remote refresh + local fallback"
```

---

## Task 6: `POST /api/signup` handler

**Files:** `internal/signup/handler.go` + `_test.go`

**Spec references:** §5.1 + §5.3 (day 0 row).

Gates + limiters in strict order. Rejections always return generic `{"error":"signup_blocked"}` so attackers cannot probe which gate tripped.

- [ ] **Step 1: Failing tests**

```go
func TestSignup_HappyPath(t *testing.T)                              { /* 200, verification_email_sent, row created */ }
func TestSignup_RateLimitedAfter3(t *testing.T)                      { /* 4th attempt same IP → 429, signup_blocked */ }
func TestSignup_DisposableEmailRejected(t *testing.T)                { /* mailinator.com → 400, signup_blocked */ }
func TestSignup_RecaptchaLowScoreRejected(t *testing.T)              { /* low-score stub → 400, signup_blocked */ }
func TestSignup_DuplicateEmailRejected(t *testing.T)                 { /* second signup same email → 400 */ }
func TestSignup_DownstreamFailureReleasesEmailClaim(t *testing.T)    { /* sender err → email released + row deleted → retry passes */ }
```

- [ ] **Step 2: Implement `handler.go`**

```go
package signup

import (
    "net/http"
    "strings"
    "time"

    "github.com/gin-gonic/gin"
    "github.com/google/uuid"
    "gorm.io/gorm"

    "github.com/tesserix/marketplace-api/internal/subscription"
    "github.com/tesserix/marketplace-api/internal/verification"
)

type Request struct {
    Email          string `json:"email" binding:"required,email"`
    Password       string `json:"password" binding:"required,min=10"`
    RecaptchaToken string `json:"recaptcha_token" binding:"required"`
    StoreName      string `json:"store_name" binding:"required,min=2,max=80"`
    Country        string `json:"country" binding:"required,len=2"`
}

type Handler struct {
    db        *gorm.DB
    limiter   *RateLimiter
    blocklist *Blocklist
    recaptcha *RecaptchaVerifier
    verifier  verification.Sender
    clock     func() time.Time
}

func NewHandler(db *gorm.DB, rl *RateLimiter, bl *Blocklist, rc *RecaptchaVerifier, v verification.Sender, clock func() time.Time) *Handler {
    if clock == nil { clock = time.Now }
    return &Handler{db: db, limiter: rl, blocklist: bl, recaptcha: rc, verifier: v, clock: clock}
}

// Signup — POST /api/signup. Cheapest gate first, most expensive last.
func (h *Handler) Signup(c *gin.Context) {
    var req Request
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "signup_blocked"}); return
    }
    email := strings.ToLower(strings.TrimSpace(req.Email))
    ip := clientIP(c)
    ctx := c.Request.Context()

    // 1. IP rate limit (Redis hit only)
    if err := h.limiter.CheckIP(ctx, ip); err != nil {
        c.JSON(http.StatusTooManyRequests, gin.H{"error": "signup_blocked"}); return
    }
    // 2. Blocklist (in-memory map)
    if err := h.blocklist.Check(email); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "signup_blocked"}); return
    }
    // 3. reCAPTCHA (network)
    if err := h.recaptcha.Verify(ctx, req.RecaptchaToken); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "signup_blocked"}); return
    }
    // 4. Email 1-ever — CLAIM before any side effect
    if err := h.limiter.CheckEmail(ctx, email); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "signup_blocked"}); return
    }

    storeID, tenantID := uuid.New(), uuid.New()
    now := h.clock().UTC()
    row := subscription.StoreSubscription{
        TenantID: tenantID, StoreID: storeID, Email: email,
        Status: subscription.StatusSignup, Plan: subscription.PlanTrial,
        SignupDate: now, Country: strings.ToUpper(req.Country),
    }
    if err := h.db.WithContext(ctx).Create(&row).Error; err != nil {
        h.limiter.ReleaseEmail(ctx, email)
        c.JSON(http.StatusInternalServerError, gin.H{"error": "signup_blocked"}); return
    }

    if err := h.verifier.SendSignupVerification(ctx, email, storeID); err != nil {
        h.limiter.ReleaseEmail(ctx, email)
        _ = h.db.WithContext(ctx).Where("store_id = ?", storeID).Delete(&subscription.StoreSubscription{}).Error
        c.JSON(http.StatusInternalServerError, gin.H{"error": "signup_blocked"}); return
    }

    c.JSON(http.StatusOK, gin.H{"status": "verification_email_sent"})
}

// Cloudflare Tunnel → Istio ingress sets CF-Connecting-IP.
func clientIP(c *gin.Context) string {
    if ip := c.GetHeader("CF-Connecting-IP"); ip != "" { return ip }
    if ip := c.GetHeader("X-Real-IP");       ip != "" { return ip }
    return c.ClientIP()
}
```

- [ ] **Step 3: Run — expect PASS; commit**

```bash
git add internal/signup/handler{,_test}.go
git commit -m "feat(signup): POST /api/signup with three gates + rate limits + rollback on failure"
```

---

## Task 7: `trial.Subscribe` — deferred-charge flow

**Files:** `internal/billing/trial/subscribe.go` + `_test.go`

**Spec references:** §4.7 Council finding #11, §5.3 ("Any day ≤90, card added"), criterion #46.

- [ ] **Step 1: Failing tests**

```go
func TestSubscribe_TrialEndIsSignupPlus90Days(t *testing.T)       { /* clock=day45, trial_end sent == signup+90d (NOT day45+90d) */ }
func TestSubscribe_PersistsStripeSubscriptionID(t *testing.T)     { /* row.StripeSubscriptionID == "sub_x" after call */ }
func TestSubscribe_NoImmediateStatusMutation(t *testing.T)        { /* row.Status stays signup — webhook owns transitions */ }
func TestSubscribe_IdempotentOnReplay(t *testing.T)               { /* 2 calls, same subscription_id; no drift */ }
func TestSubscribe_BlockedWhenAlreadyActive(t *testing.T)         { /* active → ErrSubscriptionAlreadyActive (P4 owns upgrades) */ }
func TestSubscribe_MissingStripeCustomer(t *testing.T)            { /* empty cus id → ErrMissingStripeCustomer */ }
```

- [ ] **Step 2: Implement `subscribe.go`**

```go
package trial

import (
    "context"
    "errors"
    "fmt"
    "time"

    "github.com/google/uuid"
    "gorm.io/gorm"

    "github.com/tesserix/marketplace-api/internal/billing/stripe"
    "github.com/tesserix/marketplace-api/internal/subscription"
)

const TrialDays = 90 // §5.3

var (
    ErrSubscriptionAlreadyActive = errors.New("trial: already active — use upgrade flow (P4)")
    ErrMissingStripeCustomer     = errors.New("trial: store has no Stripe customer")
)

// PriceResolver (P2 ships this) maps (plan, period, currency) → Stripe price id.
type PriceResolver interface {
    Resolve(ctx context.Context, plan, period, currency string) (string, error)
}

// StripeAPI is the narrow subset Subscribe needs — tests stub without the full Client.
type StripeAPI interface {
    CreateSubscription(ctx context.Context, in stripe.CreateSubscriptionInput) (*stripe.Subscription, error)
}

type Subscriber struct {
    db     *gorm.DB
    stripe StripeAPI
    prices PriceResolver
    clock  func() time.Time
}

func NewSubscriber(db *gorm.DB, s StripeAPI, p PriceResolver, clock func() time.Time) *Subscriber {
    if clock == nil { clock = func() time.Time { return time.Now().UTC() } }
    return &Subscriber{db: db, stripe: s, prices: p, clock: clock}
}

type SubscribeInput struct {
    TenantID uuid.UUID
    StoreID  uuid.UUID
    Plan     string // "starter" | "studio" | "pro"
    Period   string // "monthly" | "annual"
    Currency string // iso 4217 lowercase
}

type SubscribeResult struct {
    StripeSubscriptionID string
    TrialEndUnix         int64
}

// Subscribe creates the Stripe subscription with trial_end = signup_date + 90d.
// It does NOT flip subscription.status — that's the webhook's job, routed through
// statemachine.Transition in P2's dispatcher. Keeping this a pure "provision Stripe
// object + persist its id" is what makes webhook-replay safety trivial.
func (s *Subscriber) Subscribe(ctx context.Context, in SubscribeInput) (*SubscribeResult, error) {
    var out SubscribeResult
    err := subscription.WithAdvisoryLock(ctx, s.db, in.StoreID, func(tx *gorm.DB) error {
        var row subscription.StoreSubscription
        if err := tx.Where("tenant_id=? AND store_id=?", in.TenantID, in.StoreID).First(&row).Error; err != nil {
            return fmt.Errorf("trial: load store: %w", err)
        }
        if row.Status == subscription.StatusActive { return ErrSubscriptionAlreadyActive }
        if row.StripeCustomerID == ""              { return ErrMissingStripeCustomer }

        // trial_end = signup_date + 90d (NOT now() + 90d). Criterion #46.
        trialEnd := row.SignupDate.Add(TrialDays * 24 * time.Hour).Unix()

        priceID, err := s.prices.Resolve(ctx, in.Plan, in.Period, in.Currency)
        if err != nil { return fmt.Errorf("trial: resolve price: %w", err) }

        sub, err := s.stripe.CreateSubscription(ctx, stripe.CreateSubscriptionInput{
            StoreID: in.StoreID.String(), Plan: in.Plan, Period: in.Period,
            CustomerID: row.StripeCustomerID, PriceID: priceID, TrialEnd: trialEnd,
        })
        if err != nil { return fmt.Errorf("trial: create stripe subscription: %w", err) }

        res := tx.Exec(`
            UPDATE store_subscriptions
            SET stripe_subscription_id = ?, updated_at = now()
            WHERE tenant_id = ? AND store_id = ?`,
            sub.ID, in.TenantID, in.StoreID,
        )
        if res.Error != nil { return fmt.Errorf("trial: persist subscription id: %w", res.Error) }
        out = SubscribeResult{StripeSubscriptionID: sub.ID, TrialEndUnix: trialEnd}
        return nil
    })
    if err != nil { return nil, err }
    return &out, nil
}
```

- [ ] **Step 3: Run — expect PASS; commit**

```bash
git add internal/billing/trial/subscribe{,_test}.go
git commit -m "feat(trial): deferred-charge Stripe subscribe (trial_end = signup_date + 90d)"
```

---

## Task 8: `POST /admin/stores/:storeId/billing/subscription`

**Files:** `internal/handlers/admin/billing.go` + `_test.go` (modify)

This endpoint is inside `/admin/stores/:storeId/billing/*` — already on P3's `RequireActive` allowlist.

- [ ] **Step 1: Failing tests**

```go
func TestAdminBilling_Subscribe_CallsTrialSubscriber(t *testing.T) { /* 200, response carries stripe_subscription_id */ }
func TestAdminBilling_Subscribe_RefusesActiveStore(t *testing.T)   { /* 409 — upgrades route through P4, not here */ }
func TestAdminBilling_Subscribe_MissingCustomer412(t *testing.T)   { /* 412 — signup never completed checkout */ }
```

- [ ] **Step 2: Implement**

```go
type SubscribeRequest struct {
    Plan     string `json:"plan"     binding:"required,oneof=starter studio pro"`
    Period   string `json:"period"   binding:"required,oneof=monthly annual"`
    Currency string `json:"currency" binding:"required,len=3"`
}

func (h *BillingHandler) Subscribe(c *gin.Context) {
    tenantID := c.MustGet("tenant_id").(uuid.UUID)
    storeID  := uuid.MustParse(c.Param("storeId"))
    var req SubscribeRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(400, gin.H{"error": "invalid_request"}); return
    }
    res, err := h.subscriber.Subscribe(c.Request.Context(), trial.SubscribeInput{
        TenantID: tenantID, StoreID: storeID,
        Plan: req.Plan, Period: req.Period, Currency: strings.ToLower(req.Currency),
    })
    switch {
    case errors.Is(err, trial.ErrSubscriptionAlreadyActive):
        c.JSON(409, gin.H{"error": "already_active"}); return
    case errors.Is(err, trial.ErrMissingStripeCustomer):
        c.JSON(412, gin.H{"error": "missing_stripe_customer"}); return
    case err != nil:
        c.JSON(500, gin.H{"error": "subscribe_failed"}); return
    }
    c.JSON(200, gin.H{"stripe_subscription_id": res.StripeSubscriptionID, "trial_end_unix": res.TrialEndUnix})
}
```

- [ ] **Step 3: Run — expect PASS; commit**

```bash
git add internal/handlers/admin/billing{,_test}.go
git commit -m "feat(admin): POST /admin/stores/:storeId/billing/subscription — deferred-charge card-add"
```

---

## Task 9: `trial_banner_cron` + nudge emails

**Files:** `internal/billing/trial/banner_cron.go` + `_test.go`, `internal/email/templates/trial_day_{60,75,85}.{txt,html}`

**Spec references:** §5.3 rows 60 / 75 / 85.

- [ ] **Step 1: Failing tests**

```go
func TestBannerCron_SetsDay60StateAndSendsEmail(t *testing.T)       { /* day-60 store → trial_banner_state='day_60', "trial_day_60" sent */ }
func TestBannerCron_Idempotent(t *testing.T)                        { /* second run same day → no extra email */ }
func TestBannerCron_SkipsStoresWithCards(t *testing.T)              { /* stripe_subscription_id != NULL → skip */ }
func TestBannerCron_NoDayMismatchOnOtherDays(t *testing.T)          { /* days 59/61/74/76/84/86 → no-op */ }
func TestBannerCron_TransitionsCorrectlyAt75And85(t *testing.T)     { /* state advances day_60 → day_75 → day_85 */ }
```

- [ ] **Step 2: Implement `banner_cron.go`**

```go
package trial

import (
    "context"
    "fmt"
    "time"

    "gorm.io/gorm"

    "github.com/tesserix/marketplace-api/internal/email"
    "github.com/tesserix/marketplace-api/internal/subscription"
)

// BannerSpec 09:00 UTC — early enough for start-of-day in most timezones.
const BannerSpec = "0 9 * * *"

type BannerCron struct {
    db     *gorm.DB
    mailer email.Mailer
    clock  func() time.Time
}

func NewBannerCron(db *gorm.DB, m email.Mailer, clock func() time.Time) *BannerCron {
    if clock == nil { clock = func() time.Time { return time.Now().UTC() } }
    return &BannerCron{db: db, mailer: m, clock: clock}
}

type bannerTarget struct { day int; state, template string }

var bannerTargets = []bannerTarget{
    {60, "day_60", "trial_day_60"},
    {75, "day_75", "trial_day_75"},
    {85, "day_85", "trial_day_85"},
}

func (c *BannerCron) Run(ctx context.Context) error {
    for _, t := range bannerTargets {
        if err := c.processDay(ctx, t); err != nil {
            return fmt.Errorf("banner cron day %d: %w", t.day, err)
        }
    }
    return nil
}

func (c *BannerCron) processDay(ctx context.Context, t bannerTarget) error {
    now := c.clock().UTC()
    target := now.AddDate(0, 0, -t.day)
    dayStart := time.Date(target.Year(), target.Month(), target.Day(), 0, 0, 0, 0, time.UTC)
    dayEnd := dayStart.Add(24 * time.Hour)

    var rows []subscription.StoreSubscription
    err := c.db.WithContext(ctx).
        Where("status = ?", subscription.StatusTrialing).
        Where("stripe_subscription_id IS NULL").
        Where("signup_date >= ? AND signup_date < ?", dayStart, dayEnd).
        Where("trial_banner_state IS DISTINCT FROM ?", t.state).
        Find(&rows).Error
    if err != nil { return err }

    for _, row := range rows {
        _ = c.processOne(ctx, row, t) // log + continue — one failure must not block others
    }
    return nil
}

func (c *BannerCron) processOne(ctx context.Context, row subscription.StoreSubscription, t bannerTarget) error {
    return subscription.WithAdvisoryLock(ctx, c.db, row.StoreID, func(tx *gorm.DB) error {
        // CAS on state column — exactly-one-writer across pods.
        res := tx.Exec(`
            UPDATE store_subscriptions
            SET trial_banner_state = ?, trial_banner_state_set_at = now(), updated_at = now()
            WHERE store_id = ?
              AND status = 'trialing'
              AND stripe_subscription_id IS NULL
              AND (trial_banner_state IS NULL OR trial_banner_state <> ?)`,
            t.state, row.StoreID, t.state,
        )
        if res.Error != nil     { return res.Error }
        if res.RowsAffected == 0 { return nil } // another pod won
        return c.mailer.Send(ctx, email.Message{
            Template: t.template,
            To:       row.Email,
            Data:     map[string]any{"store_name": row.StoreName, "day": t.day},
        })
    })
}
```

- [ ] **Step 3: Email templates (editorial, calm — text shown; HTML mirrors with shared `_layout.html`)**

```
# trial_day_60.txt
Subject: Your Mark8ly trial turns 60 today

Sixty days in. Your store, your products, your customers — all in place. Thirty days remain before renewal decisions.

When you're ready to continue, add a card in the admin. We don't charge today — the first invoice lands on day 90, in your local currency.

— The Mark8ly team
```

```
# trial_day_75.txt
Subject: Fifteen days until your first renewal

Your trial renews in fifteen days. Adding a card today still keeps day 90 as your first charge date — add early, pay later.

If you'd rather not continue, no action needed. Your store simply pauses on day 90 and stays paused for sixty days before archival.

— The Mark8ly team
```

```
# trial_day_85.txt
Subject: Five days

Five days until your Mark8ly trial completes. A card added now means the first charge lands on day 90 — five days from today — in your local currency.

If you'd like to continue after day 90, do one thing this week: Billing → Add card.

— The Mark8ly team
```

- [ ] **Step 4: Run — expect PASS; commit**

```bash
git add internal/billing/trial/banner_cron{,_test}.go \
        internal/email/templates/trial_day_{60,75,85}.{txt,html}
git commit -m "feat(trial): banner cron day 60/75/85 with editorial nudge emails"
```

---

## Task 10: `trial_expiry_cron` + expired email

**Files:** `internal/billing/trial/expiry_cron.go` + `_test.go`, `internal/email/templates/trial_expired.{txt,html}`

**Spec references:** §5.3 day 90; §17.2 `trialing → expired`.

- [ ] **Step 1: Failing tests**

```go
func TestExpiryCron_TransitionsDay90StoresWithoutCard(t *testing.T) { /* trialing → expired + audit event emitted */ }
func TestExpiryCron_SkipsStoresWithStripeSubscription(t *testing.T) { /* subscription id != NULL → skip (webhook owns) */ }
func TestExpiryCron_Idempotent(t *testing.T)                        { /* re-run finds expired row → no-op; 1 audit event total */ }
func TestExpiryCron_LosesCASRaceGracefully(t *testing.T)             { /* two racers → exactly one succeeds; loser sees ErrCASConflict */ }
```

- [ ] **Step 2: Implement**

```go
package trial

import (
    "context"
    "errors"
    "time"

    "gorm.io/gorm"

    "github.com/tesserix/marketplace-api/internal/audit"
    "github.com/tesserix/marketplace-api/internal/email"
    "github.com/tesserix/marketplace-api/internal/subscription"
    "github.com/tesserix/marketplace-api/internal/subscription/statemachine"
)

// ExpirySpec 00:15 UTC — late enough that any last-second card-add webhook
// from the prior day has settled; early enough that merchants see expiry at
// start-of-day in most timezones.
const ExpirySpec = "15 0 * * *"

type ExpiryCron struct {
    db      *gorm.DB
    emitter *audit.Emitter
    mailer  email.Mailer
    clock   func() time.Time
}

func NewExpiryCron(db *gorm.DB, em *audit.Emitter, m email.Mailer, clock func() time.Time) *ExpiryCron {
    if clock == nil { clock = func() time.Time { return time.Now().UTC() } }
    return &ExpiryCron{db: db, emitter: em, mailer: m, clock: clock}
}

func (c *ExpiryCron) Run(ctx context.Context) error {
    cutoff := c.clock().UTC().AddDate(0, 0, -TrialDays)
    var rows []subscription.StoreSubscription
    err := c.db.WithContext(ctx).
        Where("status = ?", subscription.StatusTrialing).
        Where("stripe_subscription_id IS NULL").
        Where("signup_date < ?", cutoff).
        Find(&rows).Error
    if err != nil { return err }

    for _, row := range rows { c.expireOne(ctx, row) }
    return nil
}

func (c *ExpiryCron) expireOne(ctx context.Context, row subscription.StoreSubscription) {
    err := statemachine.Transition(ctx, statemachine.TransitionInput{
        DB: c.db, Emitter: c.emitter,
        TenantID: row.TenantID, StoreID: row.StoreID,
        From: subscription.StatusTrialing, To: subscription.StatusExpired,
        Actor: "system:cron:trial_expiry", Reason: "day_90_no_card",
    })
    switch {
    case err == nil:
        _ = c.mailer.Send(ctx, email.Message{
            Template: "trial_expired", To: row.Email,
            Data: map[string]any{"store_name": row.StoreName},
        })
    case errors.Is(err, statemachine.ErrCASConflict):
        // Another writer moved it — likely webhook from a last-second card-add. Intended; no email.
    default:
        // Log + continue.
    }
}
```

- [ ] **Step 3: Template**

```
# trial_expired.txt
Subject: Your Mark8ly trial has completed

Your trial window ended today. Your storefront is paused; your admin stays open for billing, orders export, and account management.

If you'd like to reopen the store, add a card under Billing. Your catalog, customers, and settings wait for you for sixty days before the store archives.

— The Mark8ly team
```

- [ ] **Step 4: Run — expect PASS; commit**

```bash
git add internal/billing/trial/expiry_cron{,_test}.go \
        internal/email/templates/trial_expired.{txt,html}
git commit -m "feat(trial): day-90 expiry cron via statemachine.Transition + expired email"
```

---

## Task 11: Migration fast-path — repo + submit + CSM review

**Files:** `internal/billing/migration/repository.go`, `internal/billing/migration/handler.go` + `_test.go`, `internal/email/templates/migration_fast_path_{approved,rejected}.{txt,html}`

**Spec references:** §5.1.1.

Two endpoints:
- `POST /admin/stores/:storeId/migration-fast-path/submit` — merchant; writes row in `pending`. Uniqueness enforced by `EXCLUDE (store_id) WHERE status='pending'`.
- `POST /internal/csm/migration-fast-path/:id/review` — CSM approve/reject. On approve, sets `tax_id_window_shortened_at`. Does NOT waive tax-ID validation (P7 owns that).

- [ ] **Step 1: Repository tests**

```go
func TestRepo_CreatePending(t *testing.T)            { /* status pending, row persisted */ }
func TestRepo_OnlyOneOpenPerStore(t *testing.T)      { /* second CreatePending → ErrAlreadyPending */ }
func TestRepo_Approve_ShortensTaxWindow(t *testing.T){ /* approve writes tax_id_window_shortened_at */ }
func TestRepo_Reject_NoSideEffect(t *testing.T)      { /* reject leaves tax_id_window_shortened_at NULL */ }
```

- [ ] **Step 2: Handler tests**

```go
func TestSubmit_Merchant(t *testing.T)                       { /* 200 */ }
func TestSubmit_RejectsWhoisYoungerThan90Days(t *testing.T)  { /* stub validator rejects → 400 whois_too_young */ }
func TestCSMReview_InternalAuthRequired(t *testing.T)        { /* no internal secret → 401 */ }
func TestCSMReview_ApproveShortensWindow(t *testing.T)       { /* approve → 200 + tax_id_window_shortened_at set */ }
func TestCSMReview_RejectDoesNotShortenWindow(t *testing.T)  { /* reject → 200 + column still NULL */ }
```

- [ ] **Step 3: Implement `repository.go`**

```go
package migration

import (
    "context"
    "errors"
    "strings"
    "time"

    "github.com/google/uuid"
    "gorm.io/gorm"
)

var (
    ErrAlreadyPending = errors.New("migration: pending review already exists for this store")
    ErrNotFound       = errors.New("migration: review not found")
)

type Review struct {
    ID            uuid.UUID
    TenantID      uuid.UUID
    StoreID       uuid.UUID
    EvidenceType  string
    EvidenceURL   string
    PriorPlatform string
    WhoisDomain   string
    Status        string
    ReviewerID    *uuid.UUID
    ReviewerNotes string
    CreatedAt     time.Time
    ReviewedAt    *time.Time
}

type CreatePendingInput struct {
    TenantID      uuid.UUID
    StoreID       uuid.UUID
    EvidenceType  string
    EvidenceURL   string
    PriorPlatform string
    WhoisDomain   string
}

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

func (r *Repository) CreatePending(ctx context.Context, in CreatePendingInput) (*Review, error) {
    review := Review{
        ID: uuid.New(), TenantID: in.TenantID, StoreID: in.StoreID,
        EvidenceType: in.EvidenceType, EvidenceURL: in.EvidenceURL,
        PriorPlatform: in.PriorPlatform, WhoisDomain: in.WhoisDomain,
        Status: "pending", CreatedAt: time.Now().UTC(),
    }
    if err := r.db.WithContext(ctx).Create(&review).Error; err != nil {
        if isUniqueViolation(err) { return nil, ErrAlreadyPending }
        return nil, err
    }
    return &review, nil
}

func (r *Repository) Approve(ctx context.Context, id, reviewerID uuid.UUID, notes string) error {
    return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
        var review Review
        if err := tx.First(&review, "id = ? AND status = 'pending'", id).Error; err != nil {
            if errors.Is(err, gorm.ErrRecordNotFound) { return ErrNotFound }
            return err
        }
        now := time.Now().UTC()
        if err := tx.Exec(`
            UPDATE migration_fast_path_reviews
            SET status='approved', reviewer_id=?, reviewer_notes=?, reviewed_at=?
            WHERE id=? AND status='pending'`,
            reviewerID, strings.TrimSpace(notes), now, id,
        ).Error; err != nil { return err }

        // 14d → 48h shortcut. We do NOT waive validation — P7's tax-ID lookup
        // still runs; only the storefront-publish window shortens.
        return tx.Exec(`
            UPDATE store_subscriptions
            SET tax_id_window_shortened_at = ?, updated_at = now()
            WHERE store_id = ? AND tax_id_window_shortened_at IS NULL`,
            now, review.StoreID,
        ).Error
    })
}

func (r *Repository) Reject(ctx context.Context, id, reviewerID uuid.UUID, notes string) error {
    return r.db.WithContext(ctx).Exec(`
        UPDATE migration_fast_path_reviews
        SET status='rejected', reviewer_id=?, reviewer_notes=?, reviewed_at=now()
        WHERE id=? AND status='pending'`,
        reviewerID, strings.TrimSpace(notes), id,
    ).Error
}
```

- [ ] **Step 4: Implement `handler.go`**

```go
package migration

import (
    "context"
    "errors"
    "net/http"

    "github.com/gin-gonic/gin"
    "github.com/google/uuid"

    "github.com/tesserix/marketplace-api/internal/email"
)

// PriorPlatformValidator is implemented by P7's tax-ID package; here we only
// need the WHOIS-age check. Until P7 ships, a stub satisfies the interface.
type PriorPlatformValidator interface {
    ValidateWhoisAge(ctx context.Context, domain string, minAgeDays int) error
}

type Handler struct {
    repo      *Repository
    validator PriorPlatformValidator
    mailer    email.Mailer
}

type submitRequest struct {
    EvidenceType  string `json:"evidence_type"  binding:"required,oneof=whois_domain platform_screenshot"`
    EvidenceURL   string `json:"evidence_url"   binding:"required,url"`
    PriorPlatform string `json:"prior_platform" binding:"omitempty,oneof=shopify woocommerce bigcommerce"`
    WhoisDomain   string `json:"whois_domain"`
}

func (h *Handler) Submit(c *gin.Context) {
    tenantID := c.MustGet("tenant_id").(uuid.UUID)
    storeID  := uuid.MustParse(c.Param("storeId"))
    var req submitRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(400, gin.H{"error": "invalid_request"}); return
    }
    if req.EvidenceType == "whois_domain" {
        if req.WhoisDomain == "" {
            c.JSON(400, gin.H{"error": "whois_domain_required"}); return
        }
        if err := h.validator.ValidateWhoisAge(c.Request.Context(), req.WhoisDomain, 90); err != nil {
            c.JSON(400, gin.H{"error": "whois_too_young"}); return
        }
    }
    review, err := h.repo.CreatePending(c.Request.Context(), CreatePendingInput{
        TenantID: tenantID, StoreID: storeID,
        EvidenceType: req.EvidenceType, EvidenceURL: req.EvidenceURL,
        PriorPlatform: req.PriorPlatform, WhoisDomain: req.WhoisDomain,
    })
    if errors.Is(err, ErrAlreadyPending) { c.JSON(409, gin.H{"error": "already_pending"}); return }
    if err != nil                         { c.JSON(500, gin.H{"error": "submit_failed"}); return }
    c.JSON(http.StatusOK, gin.H{"review_id": review.ID, "status": "pending"})
}

type reviewRequest struct {
    Decision string `json:"decision" binding:"required,oneof=approve reject"`
    Notes    string `json:"notes"    binding:"required,min=3,max=2000"`
}

func (h *Handler) Review(c *gin.Context) {
    reviewerID := c.MustGet("user_id").(uuid.UUID)
    id, err := uuid.Parse(c.Param("id"))
    if err != nil { c.JSON(400, gin.H{"error": "invalid_id"}); return }
    var req reviewRequest
    if err := c.ShouldBindJSON(&req); err != nil { c.JSON(400, gin.H{"error": "invalid_request"}); return }

    ctx := c.Request.Context()
    var repoErr error
    switch req.Decision {
    case "approve": repoErr = h.repo.Approve(ctx, id, reviewerID, req.Notes)
    case "reject":  repoErr = h.repo.Reject(ctx, id, reviewerID, req.Notes)
    }
    switch {
    case errors.Is(repoErr, ErrNotFound): c.JSON(404, gin.H{"error": "not_found"}); return
    case repoErr != nil:                   c.JSON(500, gin.H{"error": "review_failed"}); return
    }

    _ = h.mailer.Send(ctx, email.Message{
        Template: "migration_fast_path_" + req.Decision + "d",
        To:       lookupEmail(h.repo, id),
        Data:     map[string]any{"notes": req.Notes},
    })
    c.JSON(http.StatusOK, gin.H{"status": req.Decision + "d"})
}
```

- [ ] **Step 5: Templates**

```
# migration_fast_path_approved.txt
Subject: Your Mark8ly migration fast-path is approved

Thanks for the evidence. Your storefront can publish within 48 hours of tax-ID validation (instead of the standard 14-day window). Tax-ID validation itself runs the same as everyone else.

Notes from review: {{.notes}}

— The Mark8ly CSM team
```

```
# migration_fast_path_rejected.txt
Subject: Migration fast-path — additional evidence needed

Your submission didn't meet the evidence bar for expedited review. Notes:

{{.notes}}

Standard 14-day tax-ID window applies. You can resubmit with stronger evidence anytime.

— The Mark8ly CSM team
```

- [ ] **Step 6: Run — expect PASS; commit**

```bash
git add internal/billing/migration/ internal/email/templates/migration_fast_path_*.{txt,html}
git commit -m "feat(migration): fast-path submit + CSM review queue + 14d→48h window shortener"
```

---

## Task 12: Refactor `handleCheckoutSessionCompleted` → state machine

**Files:** `internal/billing/dispatch/handlers.go` + `_test.go` (modify), `scripts/verify-no-raw-status-updates.sh` (create)

**Spec references:** P3 Task 3 Step 4 — "will be rewritten to call `statemachine.Transition` in **P5**".

Two transition paths, both starting from `signup`:
- Checkout `mode=subscription` with trial → `signup → trialing`
- Checkout `mode=subscription` with `metadata[mark8ly_no_trial]=true` → `signup → active` (defensive; we don't expose this path to merchants)

- [ ] **Step 1: Failing tests**

```go
func TestCheckoutSessionCompleted_UsesStateMachine_SignupToTrialing(t *testing.T) {
    // Dispatch event → row.Status == trialing; audit event from=signup to=trialing.
}
func TestCheckoutSessionCompleted_IdempotentOnReplay(t *testing.T) {
    // Second dispatch → no second audit event; ErrCASConflict swallowed.
}
func TestCheckoutSessionCompleted_NoRawStatusUPDATE(t *testing.T) {
    // Parse handlers.go with go/parser; assert no literal "UPDATE store_subscriptions SET status" inside handleCheckoutSessionCompleted.
}
```

- [ ] **Step 2: Refactor the handler**

Previous (P2-era, kept in place by P3):

```go
// OLD — raw UPDATE kept by P3 as P5's responsibility
tx.Exec(`UPDATE store_subscriptions SET
    status=CASE WHEN stripe_subscription_id IS NULL THEN 'trialing' ELSE status END,
    stripe_subscription_id=COALESCE(stripe_subscription_id, ?), …
    WHERE stripe_customer_id=?`, …)
```

Replace with:

```go
func (d *Dispatcher) handleCheckoutSessionCompleted(ctx context.Context, tx *gorm.DB, raw []byte) error {
    var e struct {
        Data struct {
            Object struct {
                Customer     string            `json:"customer"`
                Mode         string            `json:"mode"`
                Subscription string            `json:"subscription"`
                Currency     string            `json:"currency"`
                Metadata     map[string]string `json:"metadata"`
            } `json:"object"`
        } `json:"data"`
    }
    if err := json.Unmarshal(raw, &e); err != nil {
        return fmt.Errorf("checkout.session.completed: unmarshal: %w", err)
    }
    if e.Data.Object.Customer == "" || e.Data.Object.Mode != "subscription" {
        return nil // unexpected shape — no-op
    }

    var row subscription.StoreSubscription
    if err := tx.Where("stripe_customer_id=?", e.Data.Object.Customer).First(&row).Error; err != nil {
        return fmt.Errorf("checkout.session.completed: load store: %w", err)
    }

    // Non-status columns — safe to raw-UPDATE.
    if err := tx.Exec(`
        UPDATE store_subscriptions
        SET stripe_subscription_id = COALESCE(stripe_subscription_id, ?),
            billing_currency       = COALESCE(billing_currency, ?),
            plan                   = COALESCE(NULLIF(plan, 'trial'), ?, plan),
            updated_at             = now()
        WHERE tenant_id = ? AND store_id = ?`,
        e.Data.Object.Subscription, e.Data.Object.Currency,
        e.Data.Object.Metadata["mark8ly_plan"],
        row.TenantID, row.StoreID,
    ).Error; err != nil { return err }

    target := subscription.StatusTrialing
    if e.Data.Object.Metadata["mark8ly_no_trial"] == "true" {
        target = subscription.StatusActive
    }
    err := statemachine.Transition(ctx, statemachine.TransitionInput{
        DB: tx, Emitter: d.emitter,
        TenantID: row.TenantID, StoreID: row.StoreID,
        From: row.Status, To: target,
        Actor: "system:webhook:stripe", Reason: "checkout.session.completed",
        StripeEventID: eventIDFromCtx(ctx),
    })
    // Replay-safe: CAS conflict means another replay already moved us;
    // invalid transition means the row is already past signup.
    if errors.Is(err, statemachine.ErrCASConflict) || errors.Is(err, statemachine.ErrInvalidTransition) {
        return nil
    }
    return err
}
```

- [ ] **Step 3: Update scrub script — remove P3's exemption**

```bash
#!/usr/bin/env bash
# scripts/verify-no-raw-status-updates.sh
set -euo pipefail
cd "$(dirname "$0")/.."
hits=$(grep -RnE 'UPDATE\s+store_subscriptions\s+SET[^;]*status\s*=' internal/ \
    | grep -v "_test.go" | grep -v statemachine/ || true)
if [ -n "$hits" ]; then
    echo "FAIL: direct status UPDATEs found — route through statemachine.Transition"
    echo "$hits"; exit 1
fi
echo "OK: no raw status UPDATEs outside statemachine package"
```

- [ ] **Step 4: Run — expect PASS**

```bash
go test -tags=integration ./internal/billing/dispatch/... -v
bash scripts/verify-no-raw-status-updates.sh
```

- [ ] **Step 5: Commit**

```bash
git add internal/billing/dispatch/handlers{,_test}.go scripts/verify-no-raw-status-updates.sh
git commit -m "refactor(dispatch): route checkout.session.completed through statemachine.Transition"
```

---

## Task 13: `signup_anomaly_cron` — >50/day alert

**Files:** `internal/signup/anomaly_cron.go` + `_test.go`, `migrations/0053_signup_anomaly_log.{up,down}.sql`

**Spec references:** §5.1 — "Signup volume alert: >50 trial signups/day".

- [ ] **Step 1: Migration 0053**

```sql
-- up
CREATE TABLE signup_anomaly_log (
    alert_date   DATE         NOT NULL,
    signup_date  DATE         NOT NULL,
    count        INT          NOT NULL,
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
    PRIMARY KEY (alert_date, signup_date)
);
-- down
DROP TABLE IF EXISTS signup_anomaly_log;
```

- [ ] **Step 2: Failing tests**

```go
func TestAnomalyCron_QuietUnder50(t *testing.T)         { /* 49 signups → no Slack send */ }
func TestAnomalyCron_AlertsOver50(t *testing.T)         { /* 75 signups → 1 Slack send, counter +=1 */ }
func TestAnomalyCron_IdempotentSameDay(t *testing.T)    { /* 2 runs → still 1 Slack send (ON CONFLICT guard) */ }
```

- [ ] **Step 3: Implement**

```go
package signup

import (
    "context"
    "fmt"
    "time"

    "github.com/prometheus/client_golang/prometheus"
    "gorm.io/gorm"
)

const AnomalySpec = "0 5 * * *" // 05:00 UTC — 5h buffer for edge rows
const AnomalyThreshold = 50

type SlackNotifier interface {
    Send(ctx context.Context, text string) error
}

type AnomalyCron struct {
    db      *gorm.DB
    slack   SlackNotifier
    counter prometheus.Counter
    clock   func() time.Time
}

func NewAnomalyCron(db *gorm.DB, s SlackNotifier, c prometheus.Counter, clock func() time.Time) *AnomalyCron {
    if clock == nil { clock = func() time.Time { return time.Now().UTC() } }
    return &AnomalyCron{db: db, slack: s, counter: c, clock: clock}
}

// Run counts signups in the previous UTC day. Idempotent via signup_anomaly_log
// unique key (alert_date, signup_date) + ON CONFLICT DO NOTHING.
func (c *AnomalyCron) Run(ctx context.Context) error {
    now := c.clock().UTC()
    yesterdayStart := time.Date(now.Year(), now.Month(), now.Day()-1, 0, 0, 0, 0, time.UTC)
    yesterdayEnd := yesterdayStart.Add(24 * time.Hour)

    var count int64
    if err := c.db.WithContext(ctx).Table("store_subscriptions").
        Where("signup_date >= ? AND signup_date < ?", yesterdayStart, yesterdayEnd).
        Count(&count).Error; err != nil {
        return fmt.Errorf("signup anomaly: count: %w", err)
    }
    if count <= AnomalyThreshold { return nil }

    res := c.db.WithContext(ctx).Exec(`
        INSERT INTO signup_anomaly_log (alert_date, signup_date, count, created_at)
        VALUES (?, ?, ?, now())
        ON CONFLICT DO NOTHING`,
        now.Format("2006-01-02"), yesterdayStart, count,
    )
    if res.Error != nil { return fmt.Errorf("signup anomaly: dedup insert: %w", res.Error) }
    if res.RowsAffected == 0 { return nil } // already alerted today

    c.counter.Inc()
    return c.slack.Send(ctx, fmt.Sprintf(
        "[mark8ly] %d signups yesterday (threshold %d). Check %s",
        count, AnomalyThreshold, yesterdayStart.Format("2006-01-02"),
    ))
}
```

- [ ] **Step 4: Run — expect PASS; commit**

```bash
git add internal/signup/anomaly_cron{,_test}.go migrations/0053_signup_anomaly_log.*.sql
git commit -m "feat(signup): >50/day anomaly cron with Slack alert + Prometheus counter"
```

---

## Task 14: `trial_activation_cron` — product_created_day_30 counter

**Files:** `internal/billing/trial/activation_cron.go` + `_test.go`, `migrations/0054_trial_activation_marker.{up,down}.sql`

**Spec references:** §15 — `subscription.trial.product_created_day_30`.

Counter-only; dashboards are P17. Marker column on the row guards against double-increment.

- [ ] **Step 1: Migration 0054**

```sql
-- up
ALTER TABLE store_subscriptions ADD COLUMN trial_activation_marked_at TIMESTAMPTZ NULL;
-- down
ALTER TABLE store_subscriptions DROP COLUMN IF EXISTS trial_activation_marked_at;
```

- [ ] **Step 2: Failing tests**

```go
func TestActivationCron_IncrementsAtDay30WithProducts(t *testing.T) { /* counter +=1; marker set */ }
func TestActivationCron_SkipsStoresWithoutProducts(t *testing.T)    { /* counter unchanged */ }
func TestActivationCron_IdempotentOnReRun(t *testing.T)             { /* second run: marker present → skip */ }
```

- [ ] **Step 3: Implement**

```go
package trial

import (
    "context"
    "time"

    "github.com/google/uuid"
    "github.com/prometheus/client_golang/prometheus"
    "gorm.io/gorm"
)

const ActivationSpec = "30 0 * * *"

type ActivationCron struct {
    db      *gorm.DB
    counter prometheus.Counter
    clock   func() time.Time
}

func NewActivationCron(db *gorm.DB, c prometheus.Counter, clock func() time.Time) *ActivationCron {
    if clock == nil { clock = func() time.Time { return time.Now().UTC() } }
    return &ActivationCron{db: db, counter: c, clock: clock}
}

func (c *ActivationCron) Run(ctx context.Context) error {
    now := c.clock().UTC()
    target := now.AddDate(0, 0, -30)
    dayStart := time.Date(target.Year(), target.Month(), target.Day(), 0, 0, 0, 0, time.UTC)
    dayEnd := dayStart.Add(24 * time.Hour)

    rows, err := c.db.WithContext(ctx).Raw(`
        SELECT ss.store_id
        FROM store_subscriptions ss
        WHERE ss.status = 'trialing'
          AND ss.signup_date >= ? AND ss.signup_date < ?
          AND ss.trial_activation_marked_at IS NULL
          AND EXISTS (SELECT 1 FROM products p WHERE p.store_id = ss.store_id LIMIT 1)`,
        dayStart, dayEnd,
    ).Rows()
    if err != nil { return err }
    defer rows.Close()

    for rows.Next() {
        var storeID uuid.UUID
        if err := rows.Scan(&storeID); err != nil { continue }
        res := c.db.WithContext(ctx).Exec(`
            UPDATE store_subscriptions
            SET trial_activation_marked_at = now(), updated_at = now()
            WHERE store_id = ? AND trial_activation_marked_at IS NULL`,
            storeID,
        )
        if res.Error == nil && res.RowsAffected == 1 { c.counter.Inc() }
    }
    return rows.Err()
}
```

- [ ] **Step 4: Run — expect PASS; commit**

```bash
git add internal/billing/trial/activation_cron{,_test}.go migrations/0054_trial_activation_marker.*.sql
git commit -m "feat(trial): activation cron emits product_created_day_30 counter"
```

---

## Task 15: Main-wiring — register crons + routes

**Files:** `cmd/marketplace-api/main.go`, `internal/handlers/admin/routes.go`, `internal/subscription/readonly/allowlist.go` (modify)

- [ ] **Step 1: Register crons on the shared scheduler**

```go
// After stripeClient + auditEmitter + scheduler are in place (P2 owns construction):

trialSubscriber := trial.NewSubscriber(db, stripeClient, priceResolver, nil)
bannerCron      := trial.NewBannerCron(db, mailer, nil)
expiryCron      := trial.NewExpiryCron(db, auditEmitter, mailer, nil)
activationCron  := trial.NewActivationCron(db, metrics.TrialProductCreatedDay30, nil)

must(scheduler.AddFunc(trial.BannerSpec,     func() { bannerCron.Run(context.Background()) }))
must(scheduler.AddFunc(trial.ExpirySpec,     func() { expiryCron.Run(context.Background()) }))
must(scheduler.AddFunc(trial.ActivationSpec, func() { activationCron.Run(context.Background()) }))

blocklist := signup.NewBlocklistFromReader(mustOpen("data/disposable-emails.txt"))
refresher := signup.NewBlocklistRefresher(blocklist, "data/disposable-emails.txt")
must(refresher.Register(scheduler))

anomalyCron := signup.NewAnomalyCron(db, slackNotifier, metrics.SignupAnomalyAlert, nil)
must(scheduler.AddFunc(signup.AnomalySpec, func() { anomalyCron.Run(context.Background()) }))
```

- [ ] **Step 2: Wire HTTP routes**

```go
// Public signup — no auth chain beyond the rate limiter.
signupHandler := signup.NewHandler(db, rateLimiter, blocklist, recaptchaVerifier, verificationSender, nil)
router.POST("/api/signup", signupHandler.Signup)

// Admin billing subscribe + migration submit — inside the admin group.
// /billing/* is already on RequireActive allowlist; /migration-fast-path/submit is trial-time only.
billingHandler   := admin.NewBillingHandler(trialSubscriber)
migrationHandler := migration.NewHandler(migrationRepo, priorPlatformValidator, mailer)
storeRoute.POST("/billing/subscription",            billingHandler.Subscribe)
storeRoute.POST("/migration-fast-path/submit",      migrationHandler.Submit)

// CSM review — internal trust boundary.
internal := router.Group("/internal", auth.HeaderTrustAuth(cfg.InternalSecret))
internal.POST("/csm/migration-fast-path/:id/review", migrationHandler.Review)
```

- [ ] **Step 3: Update `RequireActive` allowlist**

Migrants must be able to read their submission history even in read-only states. POST submit is NOT on the allowlist — submission is trial-time only.

```go
// internal/subscription/readonly/allowlist.go — append:
{http.MethodGet, "/admin/stores/:storeId/migration-fast-path/*path"},
```

- [ ] **Step 4: Build + smoke**

```bash
cd services/marketplace-api
go build ./...
go test -tags=integration ./... -count=1
```

- [ ] **Step 5: Commit**

```bash
git add cmd/marketplace-api/main.go \
        internal/handlers/admin/routes.go \
        internal/subscription/readonly/allowlist.go
git commit -m "feat(wiring): register P5 crons + signup/billing/migration routes"
```

---

## Task 16: E2E — card day 45 → charge day 90 (criterion #46)

**Files:** `internal/billing/trial/e2e_deferred_charge_test.go`

**Purpose:** Assert spec criterion #46 — "Trial merchant adds card day 45: subscription created, first charge NOT immediate, deferred to day 90."

- [ ] **Step 1: Write the test**

```go
//go:build integration

func TestE2E_CardAddedDay45_FirstChargeDeferredToDay90(t *testing.T) {
    suite := inttest.NewSuite(t); defer suite.Close()

    // 1. Signup at a fixed date.
    signupDate := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
    suite.SetClock(signupDate)
    resp := suite.PublicPOST("/api/signup", map[string]any{
        "email":"founder@brand.co","password":"strongpass123","recaptcha_token":"good",
        "store_name":"Brand","country":"GB",
    })
    require.Equal(t, 200, resp.Code)

    storeID := suite.StoreIDFor("founder@brand.co")
    suite.SimulateEmailVerified(storeID)

    // 2. Advance clock to day 45 and add a card.
    suite.SetClock(signupDate.Add(45 * 24 * time.Hour))
    resp = suite.AdminPOST(suite.TenantID(), storeID,
        "/admin/stores/"+storeID.String()+"/billing/subscription",
        map[string]any{"plan":"starter","period":"monthly","currency":"gbp"})
    require.Equal(t, 200, resp.Code)

    // 3. Stripe fake: trial_end == signup + 90d, NOT day45 + 90d.
    stripeCall := suite.StripeFake().LastSubscriptionCreate()
    require.Equal(t, signupDate.Add(90*24*time.Hour).Unix(), stripeCall.TrialEnd)
    require.Equal(t, "none", stripeCall.ProrationBehavior)

    // 4. No charge at day 45.
    require.Empty(t, suite.StripeFake().ChargesFor(storeID))

    // 5. customer.subscription.created webhook → signup → trialing (via state machine).
    suite.DispatchWebhook("customer.subscription.created", storeID)
    require.Equal(t, subscription.StatusTrialing, suite.StatusOf(storeID))

    // 6. Day 90 - 1s: still trialing, no charge.
    suite.SetClock(signupDate.Add(90*24*time.Hour - time.Second))
    require.Equal(t, subscription.StatusTrialing, suite.StatusOf(storeID))
    require.Empty(t, suite.StripeFake().ChargesFor(storeID))

    // 7. Day 91: Stripe auto-invoices → invoice.paid webhook → trialing → active.
    suite.SetClock(signupDate.Add(91 * 24 * time.Hour))
    suite.StripeFake().FireInvoicePaid(storeID, 999 /* pence */)

    require.Equal(t, subscription.StatusActive, suite.StatusOf(storeID))
    require.Len(t, suite.StripeFake().ChargesFor(storeID), 1)
    require.Equal(t, 999, suite.StripeFake().ChargesFor(storeID)[0].AmountMinor)
}

func TestE2E_Day90NoCard_TransitionsToExpired(t *testing.T) {
    // Same setup; merchant never adds a card.
    // Day 90 cron fires: statemachine.Transition(trialing → expired) + expired email sent.
}
```

- [ ] **Step 2: Run + commit**

```bash
go test -tags=integration ./internal/billing/trial/... -run TestE2E -v
git add internal/billing/trial/e2e_deferred_charge_test.go
git commit -m "test(trial): e2e — card day 45 defers first charge to day 90 (criterion #46)"
```

---

## Final verification

- [ ] `go build ./...` clean.
- [ ] `go test -tags=integration ./...` all green.
- [ ] `bash scripts/verify-no-raw-status-updates.sh` — clean (no exemption left).
- [ ] Criterion #46 passes: card-add at day 45 → `trial_end = signup_date + 90d`; no charge at day 45; charge at day 90.
- [ ] §5.3 timeline: day 60 nudge, day 75 escalation, day 85 final nudge, day 90 `trialing → expired` + expired email.
- [ ] Criterion #53 direction-of-travel: WHOIS <90d rejected, ≥90d accepted (via stubbed validator; P7 ships the real lookup).
- [ ] `statemachine.Transition(signup → trialing)` is the only route in `handleCheckoutSessionCompleted`.
- [ ] Four crons register on boot: `trial_banner_cron` (09:00), `trial_expiry_cron` (00:15), `trial_activation_cron` (00:30), `signup_anomaly_cron` (05:00) — plus `blocklist_refresh` (Mon 03:00).
- [ ] `POST /api/signup` wired; all three gate-failure paths return generic `{"error":"signup_blocked"}`.
- [ ] Migration fast-path submit + CSM review writes `tax_id_window_shortened_at` only on approve; does not waive tax-ID validation.
- [ ] Email templates load from `internal/email/templates/` — text + HTML pairs exist for every send site.

## What's now unlocked

- **P6** dunning receives a clean handoff: trial stores either exit via `trialing → active` (card + charge) or `trialing → expired` (no card). P6 owns `active → past_due → active|expired`.
- **P7** tax-ID validation reads `tax_id_window_shortened_at` to pick 48h vs 14d.
- **P9** campaign-email ramp + drip can assume every trial row has a valid `signup_date` and a defined banner state.
- **P11** cancel + save-offer can trust `trialing → expired` is the only system-driven exit from trial.
- **P16** admin UI reads `trial_banner_state`, `stripe_subscription_id`, `tax_id_window_shortened_at`, and `migration_fast_path_reviews.status`.
- **P17** observability reads `subscription.trial.product_created_day_30` and `signup_anomaly_log`.

## Execution handoff

Plan complete. Execute with **superpowers:subagent-driven-development** (recommended) or **superpowers:executing-plans**, in order P1 → P2 → P3 → P5 (P4 is independent of P5 and can run in parallel after P3).
