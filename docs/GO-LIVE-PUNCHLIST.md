# Mark8ly go-live punchlist

**Assessed 2026-09-21** across five domains — production infra/ops, billing, product
correctness + CI, mobile, and legal/GTM. Every item below was verified against the running
cluster, the live databases, read-only Stripe reads, or quoted code. Where something could
not be verified it says so.

This supersedes `docs/superpowers/plans/2026-04-19-p20-prod-launch-readiness.md` as the
status document. That plan remains useful as a *requirements* source — its legal and tax
workstreams are still right — but 0 of its 218 boxes were ever ticked, so it is not a
record of anything.

**Scope of "go live":** real merchants on the platform, Stripe billing on live money,
mobile apps in the stores, and a public launch. All four.

---

## 0. Already actioned in this pass

| | Change |
|---|---|
| ✅ | **Checkout priced from the catalog** — `unit_price` and the tax fields no longer come from the request (#883, `9f8f43af`) |
| ✅ | **Billing writes gated to 503** until cancellation reaches Stripe (#885, `ca3a9fbd`) |
| ✅ | **Cancellation now reaches Stripe** (§1.5) — `cancel_at_period_end` is set and reversed at Stripe, the period end is persisted at subscribe time, and a Stripe failure refuses the cancellation instead of recording a local-only one. The gate stays closed pending a live-mode end-to-end run and the `BILLING_WRITES_ENABLED` decision. |
| ✅ | AU Stripe Tax instructions reversed in 4 documents; go-live runbook status corrected (#885) |
| ✅ | Base image digests bumped **and repinned by dated tag** so Renovate can see them (#883) — containers and both e2e suites green again after 11 days dead |
| ✅ | `required_status_checks: CI gate` added to the `main` ruleset |
| ✅ | **`/internal/*` 404 at the ingress gateway** — tesserix-k8s #1063 merged 2026-09-21; `POST https://auth.mark8ly.com/internal/mint-session` now answers 404, not 401. **`MARKETPLACE_INTERNAL_AUTH_SECRET` must still be rotated** — it was internet-reachable and must be treated as exposed (#888 added the rotation script; restart auth-bff and marketplace-api-admin together). |
| ✅ | **Cross-tenant IDOR cluster closed** (§1.3) — orders/returns/shipments/abandoned-carts in #887, then campaigns, segments, reviews, csv-imports and loyalty members. `knownUnscoped` in `store_scope_arch_test.go` is now empty. |

Two caveats on the ruleset change: the existing bypass actor (admin role, `bypass_mode: always`)
can still merge past the required check, and `required_approving_review_count` is still `0`.
Only `CI gate` is required — the three e2e workflows are path-filtered, and requiring a
path-filtered check permanently blocks any PR that does not touch those paths.

---

## 1. Must fix before a single real merchant touches this

Ordered by what I would do first. Everything here is live today.

### 1.1 Checkout charged a client-supplied price — PR #883
`CheckoutItemRequest.UnitPrice` came from the request body and nothing loaded a catalog
price anywhere in the checkout path. `POST {"unit_price": 0.01}` bought anything in any
live store for a cent. The block commented `── C2 fix: Recompute subtotal server-side ──`
recomputed the arithmetic from the client's own price, which is why it read as a safeguard.
`tax_rate_override` had the same shape and is also fixed.

### 1.2 `/internal/*` on auth-bff is reachable from the internet — PR #1063
`POST https://auth.mark8ly.com/internal/mint-session` answers **401, not 404** — armed at
the edge. One static shared header is the only guard; no mTLS, no rate limit, no lockout.
Whoever holds it mints a session for **any user in any tenant**.

**After merging: rotate `MARKETPLACE_INTERNAL_AUTH_SECRET`.** It must be treated as
exposed. Restart auth-bff and marketplace-api-admin *together* or the internal calls break.
Still missing afterwards: any rate limit or lockout on that comparison.

### 1.3 Cross-tenant IDOR cluster in the admin API
`StoreMiddleware` proves only that `:storeId` belongs to your tenant; it constrains no other
path id, and nine route groups never re-check. Worst two, because they act on the outside
world:
- `POST .../orders/:id/shipments/:shipmentId/cancel` — real carrier cancel/RTO on **any**
  shipment by UUID (`admin/shipments.go:181`; repo query is `WHERE id = ?`). Every *other*
  shipment route checks; this one was missed.
- `POST .../returns/:id/refunded` — real gateway refund on another tenant's order.

Also: loyalty read/adjust, campaigns read/edit/**send**/delete, segments, review
moderation, csv-import job read/cancel, abandoned-cart recovery email (emails another
tenant's customer), returns approve/reject/received. Most are mirrored under
`/api/v1/mobile/admin/...`, doubling the surface.

The correct guard already exists in-repo at `admin/returns.go:91`. Apply it uniformly, or
push `storeId` into each repository query.

**Closed.** #887 scoped orders, returns, shipments and abandoned-carts and added
`store_scope_arch_test.go`, which lists every remaining unscoped handler as debt that may
only shrink. The rest — campaigns (read/edit/delete/schedule/pause/resume and **send**),
segments, review moderation, csv-import job read/cancel/errors, and loyalty member
read/adjust — now each load the row through a `require*InStore` helper and 404 when it is
not this store's, so `knownUnscoped` is empty. The mobile mirrors are the same handler
functions behind the same middleware, so they close with them. The settings three were
never actually unscoped: they resolve the store through `storeFromCtx` and filter on it,
which the arch test now recognises. Wire-level proof, including that no write, send or
points adjust happens on the refused call, is in `store_scope_test.go`.

### 1.4 platform-api fails **open** to "owner" when OpenFGA is slow
`cmd/server/main.go:132-157` — 5s timeout on store discovery, then a `Warn` and `fga = nil`
rather than a panic. `tenant/handler.go:172` then returns `{"role":"owner"}` unconditionally
and skips the `can_edit_settings` check. The admin BFF uses that endpoint to decide the
caller's role, so **a slow OpenFGA start promotes every user to owner**. marketplace-api
gets this right and exits 1. Five-line fix.

### 1.5 Billing: a live key with no working cancellation — PR #882 (stopgap)
Live Stripe key since 2026-09-09; nothing charged yet (0 customers/subscriptions/invoices/
charges). But there is **no `Subscriptions.Cancel` anywhere in marketplace-api** —
cancellation is local-only, so a real subscriber loses access at the next finalize tick and
keeps being billed. PR #882 gates the write routes to 503.

**The gate comes off only when:** cancellation reaches Stripe, `current_period_end` is
persisted at subscribe time (today it is written only from webhooks, so `FinalizeCron` sees
NULL and expires the row), and the merchant-facing copy is re-checked.

**All three conditions are now met in code; the gate is still closed.**
- Cancellation reaches Stripe through `cancel.StripeCanceller` (`CancelAtPeriodEnd`), and
  **refuses** — 503, nothing changed locally — when Stripe cannot be reached or is not
  wired for a row Stripe is billing. Accepting the save offer reverses the schedule at
  Stripe as well, which the local-only reversal never did.
- `current_period_end` and `current_period_start` are written at subscribe time from the
  created subscription. An integration test runs `FinalizeCron` after a cancellation and
  requires the row to survive until the period actually ends.
- The cancellation flow's final copy no longer renders "Your plan ends on ." when no date
  is known.

**Found on the way, and NOT decided: §15 and §17.2 disagree about cancelling a trial.**
`cancel.IsCancellableStatus` admits trialing — "active and trialing are the only
cancellable states (§15)" — while §17.2's transition table has no
`trialing → cancel_scheduled` move. A merchant cancelling during a trial passed the guard,
fell out of the state machine, and got a **500**. It is now refused up front as
"not cancellable" (409) and no Stripe call is made for a state the local row cannot
record. That is deliberately not an answer to whether a trial *should* be cancellable —
it is only a refusal to answer with a server error. Someone has to pick: add the
transition to §17.2, or drop trialing from §15. Nobody is hitting it today because the
route is 503.

What remains is a decision, not code: an end-to-end run against a live-mode test
subscription — subscribe, cancel, un-cancel, let a period roll — and then
`BILLING_WRITES_ENABLED=true` in the chart. Until someone does that, subscribe and cancel
stay 503 and nothing can be charged.

### 1.6 The production database has been unmonitored for 15 days
Every `mark8ly-postgres` alert evaluates over an **absent metric series** — mark8ly is the
only CNPG cluster in the estate with no metrics (`cnpg_collector_up` returns NO DATA;
direct scrape times out, `NetworkPolicy mark8ly-postgres-ingress` the likely cause). The
rules report `health: ok`, which is why it looks fine. Instance loss, WAL-archive failure,
connection exhaustion and disk fill are all unalertable.

### 1.7 Nothing pages
Alertmanager's default receiver is `null`; only five narrow routes escape. **Every
`severity: warning` in mark8ly is discarded.** The P17 routing tree that would add
PagerDuty is rejected every ~5 min because the `slack-webhooks` secret does not exist.
`ZitadelLoginDown` has been firing **18 days** unacknowledged with zero silences — it is a
false positive, but nobody triaged it, which is the real signal. There is also **zero
synthetic monitoring**: nothing checks that checkout works.

### 1.8 No log aggregation at all
Cloud Logging and Cloud Monitoring are **disabled** on the production cluster
(`loggingConfig: {}`), and the in-cluster `logging` namespace is empty. Logs live only in
each node's container ring buffer and vanish on restart. When a merchant reports a failed
order tomorrow, there is nothing to read. One cluster-update flag — do it before anything
else here, because it makes every other fix verifiable.

### 1.9 Async replication with unsupervised auto-failover
`synchronous_commit = local`, `synchronous_standby_names` empty, `minSyncReplicas: 0`,
`primaryUpdateStrategy: unsupervised`. P19 was supposed to land before public launch and
did not. `docs/runbooks/cnpg-primary-failover.md` still advertises **RPO 0**, and its own
verification step fails in production. An automatic failover drops unreplicated committed
orders.

### 1.10 Backup retention is 3 days and no restore has ever run
WAL archiving works, so PITR is possible within 3 days. Corruption introduced Friday and
noticed Monday is unrecoverable. No CNPG cluster in the estate was ever bootstrapped from
recovery. Recent migrations do irreversible destruction (`DROP TABLE`, 8 dropped columns,
`DELETE FROM`) — down-migrations cannot restore dropped rows, so a bad migration *is* a
restore event.

### 1.11 Storefront merchant storefronts have no shopper privacy notice
No code path creates default legal pages at store creation (the Bondi demo's were
hand-seeded by SQL). The storefront footer renders only merchant-configured CMS links. The
platform privacy policy scopes itself to "the Mark8ly website and account". Meanwhile
**OpenPanel runs on every storefront under one shared client ID**, aggregating shopper
behaviour across all merchants — a controller-level activity disclosed nowhere and not
covered by the DPA. This scales with every new store.

---

## 2. Must fix before a public launch

### Legal — start now, it is the critical path
- **No document has been lawyer-reviewed.** The spec says so explicitly; `docs/legal/`
  does not exist. 4–8 weeks. **The documents themselves are good** — nine real, live,
  correctly-identified documents — so this is a *review*, not a drafting job. Budget
  accordingly and send the existing drafts.
- **No GDPR Art 27 EU representative, no UK representative**, while DE/ES/FR/IT/NL/GB
  merchants can sign up today and Australia has no adequacy decision. Either appoint them
  (~€200–500/yr each) or drop EU/UK from the allowlist for v1.
- **NZ GST opinion — the single longest pole.** 1–2 weeks for the opinion, then **4–8 weeks
  of IRD processing** if registration is required. NZ is one of the 11 live onboarding
  countries. **Removing NZ from the allowlist is a one-line change that reclaims ~10 weeks.**
- **$2,000 setup fee advertised with no contractual term**, and `/refunds` currently makes
  it refundable for 14 days — the spec required non-refundable. A merchant can pay, have
  custom app work begin, and demand a full refund on day 13.
- **Four published documents name Google Identity Platform**, which was decommissioned —
  including the Art 28 sub-processor register and a security page claiming Google holds
  passwords. Text edits, ~1 hour, and the cheapest diligence risk you will ever retire.
- **The cookie policy says analytics are "rolling out in 2026"** while they have been
  running the whole time. OpenPanel uses sessionStorage not cookies (so the literal
  sentence survives), but it is disclosed in no sub-processor list and there is no consent
  UI anywhere.

### Observability — you cannot tell if launch day worked
- **Zero funnel instrumentation.** `.track(`/`.capture(`/`useOpenPanel` return **no matches
  repo-wide**; signup, activation and purchase emit nothing; there is no UTM or referrer
  capture. Three events plus UTM persistence onto the store record — ~2 days, and the
  highest value-per-hour item in this document. Without it, none of #153's targets are
  computable and you cannot attribute a single one of the 259 leads.
- **Sentry looks configured and is not** — `SentryDSN` is declared, reserved in
  `secrets.example.yaml`, and never read; no SDK in any manifest.
- **No payment/order/auth metrics from mark8ly.** Only the two marketplace-api deployments
  are scraped. The four "payment" alerts evaluate over empty recording rules and can never
  fire. No auth-failure signal of any kind.

### Correctness
- **CSV product import has never worked** (#881): wrong API path, a literal `__STORE_ID__`,
  a unit test pinning the wrong URL — **plus a fourth defect not in the issue**
  (`/errors.csv` vs `/errors`). It has seven green test files. `exportProductsCsv` has no
  caller and `/products/import` has no inbound nav link. Fix it end to end — it is the
  cleanest proof that the e2e stack does its job.
- **Ungated `/internal` routes in marketplace-api**, including
  `POST /internal/stores/upsert`, which **rewrites the store→tenant projection that
  `StoreMiddleware` reads for isolation**. Only network policy stands behind it.
- **Fail-open secrets not required at boot**: `MARKETPLACE_STOREFRONT_KEY` and
  `AUDIT_INGEST_SECRET` both no-op their middleware when empty, opening the storefront API,
  audit ingest, and the destructive tenant purge.
- **`reconciliation-cron` is not in the Docker image** and has no CronJob — while the live
  `StripeReconciliationDrift` alert depends on it and can therefore never fire.
- **No startup key-mode/account assertion**, and `CONSOLE_CATALOG_MODE` **fails open**: an
  unknown value 404s, falls back to the *compiled* test-mode catalog, and serves baked test
  amounts against a live account. The chart renders it with no `| default`, and envconfig's
  default applies only to unset, not empty.
- ~~**Webhook events are silently dropped**~~ **— fixed.** Dispatch failure returned 200 so
  Stripe never retried, while the orphan-recovery query excluded the row forever. All 31
  historical events sat unprocessed at exhausted retries, including 7 `invoice.paid`.

  Four mechanisms were nominally in place and none of them ran. The handler resolves the
  store and calls `SetStoreID` **before** dispatching, so an ordinary handler failure came
  back with `store_id` populated — and both the recovery query and the stale alert filtered
  on `store_id IS NULL`, so it was neither retried nor alerted on. Stripe's own three days
  of redelivery were discarded by the 200; and had they not been, the handler answered
  every redelivery "duplicate" on the strength of the row existing, without asking whether
  it had ever processed. `manual_review_required` was a one-way door with no code that
  cleared it, which is where the 31 ended up.

  Now: recovery and the stale alert select on `processed_at IS NULL` alone; a genuine
  failure answers 503 so Stripe redelivers; a redelivery of an unprocessed event dispatches
  again; past the retry cap the event is flagged and answered 200 so Stripe stops;
  `processed_at` is stamped **inside** the dispatch transaction rather than after it; events
  whose type is not allowlisted are stamped processed rather than left to churn; and
  `cmd/webhook-replay` lists what is stuck and puts chosen events back in front of the
  resolver. **The 31 stuck events are recovered by running that tool** — `-list` first.

### Email
- **No "you have a new order" email to the merchant** — the only signal is in-app + device
  push, and the ICP is Instagram sellers who may never install the app. Payment-failure
  dunning *does* exist and is wired (that worry was unfounded).
- **No "subscription cancelled" confirmation** for a paid cancel.
- **Single provider, no fallback** — `RESEND_API_KEY` is set nowhere. A SendGrid incident
  takes out verification and password reset, i.e. signup itself.

### Mobile
- **DECIDED 2026-09-23: `mobile-storefront` is deferred.** It was never tested, and the
  white-label app it exists for is later work. Nothing can ship it by accident — it has no
  CI and no release workflow; only `mobile-admin` has one (`mobile-admin-build.yml`,
  `mobile-admin-ios-release.yml`). Its `com.example.shop` bundle id and missing auth stop
  being launch risks and become that later project's first tasks.

  **It is not free to leave sitting there, though.** It is the other half of the two-Expo
  -SDK problem below — `mobile-admin` is on Expo 56, `mobile-storefront` on Expo 52 — and
  the root hoists the older pins, which is exactly what `mobile-admin`'s four jest
  `moduleNameMapper` hacks exist to undo ("the monorepo root hoists an older major
  (pinned by another app in the workspace)"). So a shelved app is degrading the test
  fidelity of the one that ships. Dropping it from the npm `workspaces` array until it is
  picked up again would remove the hacks and the divergence in one move.
- ~~**Ship `mobile-admin`, hold `mobile-storefront`.**~~ Superseded by the decision above.
  The original reasoning: the storefront app has no auth at all (sign-in deliberately
  disabled, no replacement endpoint) yet ships five screens that cannot function plus a
  `com.example.shop` bundle id — an Apple 2.1/2.3.1 rejection that would draw scrutiny
  onto the admin app under the same account.
- **Universal links are declared but not served** — AASA and `assetlinks.json` both 404, so
  Android App Links verification fails at install on every device. Either serve them or set
  `autoVerify: false`; nothing in the app depends on them today. **This one is
  `mobile-admin`'s** (`app.config.js:74` declares `applinks:admin.mark8ly.com` with
  `autoVerify: true`), so deferring the storefront app does not retire it.
- **No crash reporting in any mobile app.** Highest-regret omission for an app that has
  never run on a stranger's device.
- **130 test files and 50 routes with zero CI** — every turbo script filters mobile-admin
  out (root `package.json` excludes `@repo/mobile-admin` from build, lint, test and
  check-types), and it has no ESLint at all. Its only workflows are the tag-triggered EAS
  build and iOS release. This is now the **largest** open mobile item, because it belongs
  to the app that actually ships.
- **Two Expo SDK majors in one workspace**, papered over by four jest resolution hacks — so
  jest and Metro test different dependency graphs, exactly how #719 shipped green. The
  second major is the deferred `mobile-storefront` (Expo 52 vs 56) — see above.
- Store consoles still need App Privacy labels, Data Safety, age rating and screenshots by
  hand. Account deletion **does** exist in-app and is reachable, which satisfies both
  stores' hard requirement.

### Deploy safety
- **Every deploy takes the storefront API fully down**: 1 replica, `maxSurge: 0`,
  `maxUnavailable: 1`, so the only pod is terminated before its replacement starts. The
  admin HPA is pinned `min 2 / max 2` and has been maxed since 2026-09-08. Three pods sit
  on a node that has been **cordoned for 21 days** — a drain is pending by definition.
- **Break-glass is armed with zero accounts** — route live, issuer real, migrations
  applied, and `break_glass_accounts` is empty. A locked door with no key.
- **MFA is enforced correctly in code but inert unless Zitadel says so.** The
  implementation is the strongest thing in the repo (deny is the zero value, pinned by
  archtests), but it only engages when `ForceMFA` is set on the tenant's org login policy,
  and **nothing provisions or asserts that**. Could not verify the live policy — no admin
  credentials. Check this before the first merchant.

---

## 3. Known-broken, can ship, must be written down

- **Multi-store switching**: create-store does not switch to the new store. Measured,
  deliberate, spec left red.
- **Ireland is excluded by accident.** The live country gate is an unintended intersection
  of a 20-country seed with a hardcoded 12-code allowlist; `IE` is in the allowlist but not
  the seed and silently drops, giving 11. Nobody chose 11. Five different counts exist
  across the codebase (20/15/19/12/11). **No user-facing copy states any count**, so there
  is nothing to correct in marketing — the contradiction is entirely in code.
- **Adding a second store bypasses the country gate** — the admin store-create page calls
  `listCountries()` unfiltered, so a merchant can pick a country with no working carrier.
- **The storefront undersells ShipEngine coverage** — a hand-maintained map lists 4
  countries where the carrier supports 11.
- **The 14-day tax-validation window is unenforced** (`windowguard.RequirePublishable` is
  mounted on no route) while the cron that punishes failure is live. Means B2B
  reverse-charge eligibility cannot be substantiated.
- **A reverse-charge clause is printed on real invoices** on validations that do not
  validate — US and CA return valid on regex alone.
- **Marketing plan is 139 days stale**; every date in it has expired and it still carries
  `<TODO: co-founder name>` placeholders.
- **The CRM is built and empty** — six tables, real UI, RBAC, conversion attribution, and
  **zero DM templates authored**. The 259 leads land in "Drifting" not "Due" because a
  never-worked lead has no `next_action_at`, and there are no bulk actions for triaging 259
  records one at a time. This is the only item in this document that makes money rather
  than removing risk.

---

## 4. Things that are better than believed

Recorded because they change where effort should go.

- **Legal prose is good** — nine real documents, live, correct ACN/ABN, no placeholders.
- **Transactional email is well built** — welcome, verification, OTP, password reset,
  invites, order/invoice/receipt/refund, dispatch, trial reminders, **and payment-failure
  dunning**, all with compiled-in Go fallbacks behind the DB rows.
- **GIP removal is genuinely complete.** Zitadel is the only issuer; verification is real
  JWKS with an audience check; no `InsecureSkipVerify`/`SKIP_AUTH` anywhere.
- **Mobile auth is correct across all three IDP hops** — the historical silent-drop risk is
  clean, and the https bridge uses a hardcoded callback with a param allowlist.
- **Admin BFF auth and route authz are solid** — tenant/user headers always come from the
  validated session, never client input; authz fails closed and 404s rather than 403s.
- **Backend checkout is well covered** — 14 integration test files that actually run.
- **No Cloud SQL capacity risk** — the DB is in-cluster CNPG, 18 MB, 1% disk, 24/400
  connections. (The `db-f1-micro` constraint in CLAUDE.md is stale.)
- **The 11 critical Go CVE alerts are noise** — all on `packages/platformauth`, a local
  `replace`; every deployed service pins patched versions and no Dockerfile copies
  `go.work`.
- **`base-digest-auto-merge.yml` was never the problem** — it is carefully built and gates
  on authoritative PR authorship. Renovate simply never opened the PRs, because the
  Dockerfiles pinned bare digests while the shared preset matches dated tags.

---

## 5. Sequencing

1. **Rotate `MARKETPLACE_INTERNAL_AUTH_SECRET`** — tesserix-k8s#1063 is merged and the
   route is 404 at the edge, but the secret was internet-reachable and is still the one
   that was exposed. Restart auth-bff and marketplace-api-admin together.
2. **Turn on Cloud Logging** — one flag, and it makes everything else verifiable.
3. ~~IDOR cluster and platform-api fail-open~~ — both closed (§1.3, `9c93b4ab`).
4. **Restore postgres metrics, create `slack-webhooks`/`pagerduty-keys`, clear the 18-day
   false positive** so the alert channel is trustworthy again.
5. **Engage counsel this week** and decide NZ in or out — that decision alone is worth ~10
   weeks.
6. **Instrument the funnel** before launch, not after.
7. Then the rest of §2.

**Two blockers have no owner in this document and cannot be closed by code:** counsel, and
whoever decides whether NZ ships. Both should start before any of the engineering above.
