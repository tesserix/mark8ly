# #259 — GDPR erasure execution: design proposal

**Status:** proposal, for approval before any code is written.
**Scope:** `marketplace-api`. The control-plane half is `tesserix/tesserix-home#140`.
**Date:** 2026-08-28

This is a design document, not a plan. It records what was verified about the data model, states the decisions that have to be made before anything is built, and recommends an answer to each. Nothing here has been implemented.

---

## 1. Corrections to the issue

Three things in #259 do not match the code. Two of them change the design.

**1.1 The status vocabulary is wrong.** The issue says *"`processing`, `completed` and `failed` exist in the schema and nothing sets them."* The actual constraint (`migrations/000059_customer_erasure_requests.up.sql:12`) is:

```sql
CHECK (status IN ('pending', 'processed', 'rejected'))
```

There is no `processing`, no `completed`, no `failed`. So there is **no in-flight state at all** — a worker cannot mark a request as being worked on without a migration. This matters: the state machine the issue asks for does not partially exist, it does not exist.

**1.2 The subject is identified by email only.** The table has `customer_email TEXT NOT NULL` and **no `customer_id`, no `gip_uid`**. Everything downstream follows from this — see §3.

**1.3 The dedupe comment is inaccurate.** `migrations/000059:16-17` says *"a second request just re-sets the clock; support dedupes manually."* The insert is `ON CONFLICT (store_id, customer_email) DO NOTHING` (`internal/customerportal/erasure.go:31-44`), so a second request does **not** re-set the clock — it is silently discarded. The 30-day statutory clock therefore runs from the *first* request, which is arguably correct, but the comment describes behaviour the code does not have.

The issue's central claim — that nothing has ever processed a request — is **confirmed**: a repo-wide search for `customer_erasure_requests` finds zero `UPDATE` statements.

---

## 2. What already exists

**The request is captured.** `POST /storefront/stores/:storeSlug/customer-erasure` (`internal/customerportal/handler.go:141+`), unauthenticated, body `{email}`.

**It surfaces to operators, but cannot be acted on.** `internal/inbox/erasure.go` lists pending rows and declares two actions, `process` (marked `Destructive: true`) and `reject`. Neither is executable: `cmd/marketplace-api/inbox_wiring.go:61-77` registers only the migration fast-path executor, and every other kind answers 501. The wiring comment states the reason plainly:

> *"erasure_request's `process` action is irreversible destruction of customer data. Neither should get a one-click path before the behaviour beneath it is settled."*

**This proposal is that settling.** The inbox surface is already built and waiting for an executor.

**There is an idempotency mechanism ready to reuse.** `inbox_action_idempotency` (migration 000107) is keyed on `idempotency_key TEXT PRIMARY KEY` with an `outcome JSONB`, and the inbox action handler already enforces `Idempotency-Key` on destructive actions and replays the stored outcome with `"replayed": true`. Its own migration comment diagnoses exactly #259's retry problem:

> *"a domain write matching `status='pending'` returns ErrNotFound on a second call, which is a failure response for an operation that in fact succeeded."*

---

## 3. The identity problem — decide this first

**There is no global customer identity.** `customer_profiles` is one row per `(store, email)`:

- `UNIQUE (store_id, email)` — migration 000013:22
- `UNIQUE (store_id, gip_uid) WHERE gip_uid IS NOT NULL` — migration 000084:13-15
- `gip_uid` is **nullable** (password-only signups), so it cannot serve as a join key
- `customer_profiles` has no FK to any tenant-level person record

So the same human in three stores is three unrelated rows, and **email is the only cross-store correlator** — which is exactly what the erasure table records. But `cer_store_email_unique` is *also* store-scoped, so a person with accounts in three stores must file three requests. The console's "group by person" (tesserix-home#140) is an application-layer convention over an email string with no backing constraint.

**A complication that must be resolved before building:** `orders.customer_id` exists but has **no FK** to `customer_profiles.id`, and orders are placeable by guests. Whether `customer_id` is reliably populated for logged-in customers was **not verified**. If it is not, erasure must be driven by `customer_email` throughout, not by profile id.

> **Open question 1.** Is erasure scoped to `(store_id, email)` — one request, one store — or to an email across every store in the tenant? Recommendation: **`(store_id, email)`**, matching the table's own unique constraint and the storefront endpoint that creates the row. Cross-store grouping stays a console presentation concern, as #259 already argues. Anything else invents a person-level identity the schema does not have.

> **Open question 2 (blocking, needs a data check).** Is `orders.customer_id` reliably set for logged-in customers? This decides whether the erasure walks the graph by profile id or by email. Cheap to answer with one query against production.

---

## 4. The footprint

Every table holding data attributable to one customer, from an exhaustive `information_schema` sweep plus the full FK graph.

**Direct — the row names the customer:**

`customer_profiles` (identity: email, name, phone, avatar, notes) · `customer_addresses` (FK, CASCADE) · `orders` (`customer_id` no FK, `customer_email`, `customer_name`) · `abandoned_carts` · `reviews` (FK + denormalised email/name) · `review_reactions` (FK) · `wishlists` (FK) · `storefront_push_tokens` (no FK) · `product_notify_subscriptions` (no FK) · `customer_loyalties` (email) · `coupon_usage` (email) · `campaign_recipients` (email) · `promo_redemptions` (email) · `gift_cards` (**four** person roles per row: sender, recipient, purchaser, plus a free-text message) · `customer_erasure_requests` itself

**Mixed subject — customer *or* staff, discriminated by an `author_type`/`actor_type` column:** `review_replies` · `support_tickets` / `support_ticket_replies` · `tickets` / `ticket_replies` · `order_events` (`actor_email`) · `audit_logs` (`actor_email`, `ip_address`, `user_agent`)

**Indirect — reachable only by join:** `order_items` · **`order_addresses`** (name, street, phone — postal PII with *no* direct customer column) · `order_tax_lines` · `payment_transactions` · `refund_transactions` · `platform_fee_ledger` · `shipments` (`ship_to` blob) · `returns` (`pickup_details` blob) · `return_items` · `gift_card_transactions` · `loyalty_transactions` · `referrals` · `review_media`

**Unclassified — flagged rather than guessed:**
- `notifications.recipient_user_id` — varchar, no FK. (Separately established while fixing #350: this column holds `customer_profiles.id`. So it **is** customer data and belongs in the direct list.)
- **JSONB blobs, none content-inspected:** `payment_transactions.metadata`, `stripe_webhook_events.payload`, `webhook_events.payload`, `abandoned_carts.items_snapshot`, `shipments.ship_from`/`ship_to`, `returns.pickup_details`, `audit_logs.metadata`, `outbox_events.payload`, `idempotency_keys.response`. Any may embed an email or address.
- `email_sends` (migration 000108) has `recipient TEXT NOT NULL` — a direct customer-email column in a table **no purge plan covers**.

### 4.1 Why `tenantpurge` cannot be reused

`internal/tenantpurge/purge.go` is the closest existing deletion map, and its structure is worth copying — a pure `purgePlan()` function returning ordered `deleteStep`s, with `countPlan` mechanically derived from it so the two enumerations cannot drift.

But **it cannot be borrowed**, because a large part of its coverage is implicit: these tables are never named in a step and are swept only by the group-6 `stores` CASCADE — `customer_profiles`, `customer_addresses`, `gift_cards`, `gift_card_transactions`, `campaigns`, `campaign_recipients`, `tickets`, `ticket_replies`.

A per-customer erasure deletes no store, so it gets no CASCADE. **Every one of those tables needs an explicit, customer-scoped delete that does not exist anywhere in the codebase today.** That is the bulk of the build.

---

## 5. The retain-versus-erase tension

This is the substance of the issue, and there is a gap worth naming.

**What is actually documented in this repo:**

| Obligation | Basis | Covers |
|---|---|---|
| 7 years after hard-delete | *"legal-obligation basis"*, §23.2 | `billing_archive` — `migrations/000046:24` |
| 7 years from `created_at` | reuses billing_archive's basis (#365) | `audit_logs WHERE actor_type='operator'` |
| Plan-derived (90d/365d/unlimited) | product promise | store-scoped `audit_logs` |

**The gap:** an exhaustive grep of every migration for `retain|retention|7 year|legal-obligation|statutory|tax|accounting` finds retention text in **exactly one place** — the two lines in migration 000046.

**There is no documented retention obligation on `orders`, `order_items`, `order_addresses`, `payment_transactions`, `refund_transactions`, `platform_fee_ledger` or `returns`.**

The premise that these cannot be erased is legally well-founded — most jurisdictions require invoice and transaction records for 6–10 years — but it is **not written down anywhere in this repo**. That is a gap to close as part of this work, not a fact to cite. Whoever approves this proposal should state the obligation and its basis, and it should land as a migration comment in the same place the code enforces it.

The codebase already draws this line explicitly. `internal/audit/prune_cron.go:227-229`:

> *"That's exactly why operator rows need their own retention path rather than riding on GDPR erasure."*

### 5.1 There is no anonymisation precedent — this proposal must invent it

Searched for `anonymi|pseudonym|redact|scrub|tombstone|obfuscat` across the whole repo. Everything found is **secret redaction in API responses or log-line masking**, never a mutation of a stored PII column. The closest stylistic reference is `maskEmailForLog` (`internal/handlers/storefront/mobile_stubs.go:14-35`, `alice.smith@example.com` → `a***@example.com`) — but it is in-memory, for logs, and never written to a column.

One supporting design signal: `migrations/000108_email_sends.up.sql:32-38` deliberately omits subject and body because *"Subject lines are interpolated customer content"*, citing three prior console endpoints that excluded free text. The estate has a consistent minimise-what-you-store instinct; there is simply no persistence-layer anonymisation to follow.

---

## 6. Proposed mechanism

### 6.1 Three dispositions, not two

Each table in §4 gets exactly one disposition, declared in one place:

- **DELETE** — the row exists only to serve the customer. `wishlists`, `abandoned_carts`, `storefront_push_tokens`, `product_notify_subscriptions`, `review_reactions`, `campaign_recipients`, `notifications`, `customer_addresses`, `customer_profiles`.
- **ANONYMISE** — the row must survive for financial or integrity reasons, but its personal fields must not. `orders` (`customer_email`, `customer_name`, and `customer_id` → NULL), `order_addresses`, `order_events.actor_email`, `payment_transactions`, `refund_transactions`, `returns`, `shipments`, `coupon_usage`, `promo_redemptions`, `customer_loyalties`, `gift_cards` (per-role), `reviews` (see below).
- **RETAIN UNTOUCHED** — with a stated basis. `billing_archive`, operator `audit_logs`, the attestation tables.

The anonymisation token should be deterministic per `(store_id, email)` — e.g. `erased+<hmac16>@erased.invalid` — so referential grouping survives (two orders by the same erased person still group) while the person does not. `.invalid` is reserved by RFC 2606 and can never be routed.

> **Open question 3.** Reviews: delete outright, or anonymise to "A customer"? Anonymising keeps the merchant's aggregate rating honest; deleting changes historical ratings. Recommendation: **anonymise** (`customer_name` → "A customer", `customer_email` → token, `customer_profile_id` → NULL), and drop `review_media` outright since customer-uploaded photographs are not aggregate-bearing.

### 6.2 The state machine needs a migration

Given §1.1, add: `processing`, `completed`, `failed` to the CHECK — or replace `processed` with them. Also needed: `processed_at` (exists, never written), `notes` (exists, never written), and an `attempts` counter. Recommend keeping `processed`/`rejected` as terminal aliases rather than rewriting existing rows.

### 6.3 Idempotency

Reuse what exists rather than inventing:
- `pg_advisory_xact_lock(hashtext(...))` via `subscription.WithAdvisoryLock`, keyed on the request id — the established pattern (used by `statemachine`, `ticket`, `billing/dispatch`, `billing/appaddon`).
- `inbox_action_idempotency` for the operator-triggered path — it already replays a stored outcome.
- Every step must be a `WHERE` clause that matches zero rows on a retry, exactly as `tenantpurge` documents (`purge.go:128-130`).

Anonymisation is naturally idempotent: re-anonymising an already-anonymised row is a no-op, because the token is deterministic.

### 6.4 The evidence record

#259 requires "an audit record of what was deleted and what was retained". Recommendation: write a **per-table counts** record — the same shape `tenantpurge.Report` already produces — into the request's `notes` (or a new `erasure_receipts` table), plus an operator `audit_log` row. It must record what was **retained and why**, not only what was destroyed: that is the half that answers a regulator.

Note the trap: that audit row must not itself carry the erased person's email. It should reference the request id, not the subject.

---

## 7. The console contract

Per #259 and tesserix-home#160, the console records intent; `marketplace-api` executes. Recommended contract:

- The console never issues deletes over its cross-DB grant.
- `marketplace-api` exposes the executor through the **existing inbox action surface** — `process` and `reject` are already declared on the erasure item and already answer 501, so this is filling in a socket that exists, not adding an endpoint.
- Status transitions are owned by `marketplace-api`; the console reads them.
- The coupling test from tesserix-home#160 holds: with the console down, erasure must still be runnable — so the executor must also be invocable as a worker/CLI, not only via the inbox.

---

## 8. Decisions needed before any code

1. **Scope** — `(store_id, email)` per request? *(recommended: yes)*
2. **`orders.customer_id` reliability** — one production query; decides id-driven vs email-driven traversal. **Blocking.**
3. **Retention obligations** — name the basis and duration for orders/payments/refunds/ledger, to be written into the schema. **Blocking** — the whole anonymise-vs-delete split rests on it.
4. **Reviews** — anonymise or delete? *(recommended: anonymise)*
5. **JSONB blobs** — in scope for this pass, or explicitly deferred with the risk recorded? A `stripe_webhook_events.payload` can contain the customer's email and address. *(recommended: explicitly defer, with a documented follow-up — inspecting nine blob columns is its own effort)*
6. **`email_sends`** — covered by no purge plan at all. Fix here, or file separately? *(recommended: file separately; it is also a tenant-purge gap, not just an erasure gap)*
7. **Anonymisation token format** — confirm `erased+<hmac16>@erased.invalid` and where the HMAC key lives.

## 9. Suggested sequencing

Not a plan, but the shape one would take:

1. Answer 2 and 3 (the blocking questions), and land the retention documentation as migration comments.
2. Migration: status vocabulary + `attempts`.
3. A pure `erasurePlan(storeID, email) []step` function with per-table dispositions — pure, so it is unit-testable without a database, exactly as `tenantpurge.purgePlan` is. **This is the reviewable artefact**; get it right before any execution code exists.
4. The executor, behind the advisory lock, with the receipt.
5. Wire it into the existing inbox action, replacing the 501.
6. A CLI/worker entry point for the console-down case.

**A note on step 3, drawn from this milestone's experience:** the dangerous failure mode is not a bug in the executor, it is a table missing from the plan. The tenant purge guards against that with a hand-maintained list plus `purge_test.go`'s `protectedTables`, and `purge.go:47-54` records that an earlier version of its own comment was *false* about what the database enforced. An erasure plan needs the same treatment — a test that enumerates every table in `information_schema` and fails when one has no declared disposition, so a table added next year cannot silently escape erasure.

---

## Appendix: how this was verified

Live schema queried via `information_schema` (92 tables) plus the full FK graph; migrations read directly; repo-wide greps for retention, anonymisation and `customer_erasure_requests` usage. No tests were run. Claims are marked in the source survey as verified-by-reading, verified-by-query, or inferred; the three items in §4 marked "unclassified" are genuinely unresolved and are flagged rather than guessed.

One environment note found along the way, relevant to anyone testing this: **the dev database is at schema version 106 while the migration files go to 110.** `inbox_action_idempotency` (000107) is therefore absent from it — which is the real cause of the two `internal/handlers/platformadmin` integration failures previously recorded as a "migration gap in the test environment". The gap is that the dev database has not been migrated, not that a migration is missing.

---

## Corrections found during implementation (2026-08-29)

Implementing the plan meant verifying every column against the live schema, which found four errors in the survey above. They are recorded here rather than silently edited, because the survey's method — and where it fell short — is the useful part.

1. **`promo_redemptions` is NOT customer data.** §4 lists it under "Direct — the row names the customer". It is the *subscription* promo-code engine (§7): `promo.Service` writes `normaliseEmail(in.MerchantEmail)` (`internal/promo/service.go:156`) and the row carries `subscription_id NOT NULL`, not an order. Its `email` is the **merchant's billing address**. Anonymising it during a customer erasure would have rewritten a merchant's own billing record. It is now a declared exclusion with that justification.

2. **Four of the five "anonymise via `order_id`" tables have no personal column at all.** `payment_transactions`, `refund_transactions`, `platform_fee_ledger` and `shipments` hold none — their only PII is inside JSONB (`payment_transactions.metadata`, `shipments.ship_from`/`ship_to`), which is out of scope per decision 5. They therefore carry **no erasure step**. They still take the 7-year retention `COMMENT` from migration 000113, because the retention basis applies to the rows regardless.

3. **`returns.pickup_details` is `text`, not JSONB.** §4 lists it among the uninspected blobs. It is a free-text customer pickup address and nullable, so it is **in scope** and is cleared.

4. **Two tables have no name column.** §6.1 groups "`coupon_usage`, `promo_redemptions`, `customer_loyalties`: email → token, name → RedactedName". Only `customer_loyalties` has a name column. `coupon_usage` has only `customer_email`.

Also worth recording, since it changes how the console should think about scoping: `campaign_recipients` has no `store_id` (only `tenant_id` + `campaign_id`, scoped through `campaigns`), and `storefront_push_tokens` / `product_notify_subscriptions` carry `store_slug` rather than `store_id` and have no FK to `customer_profiles`.

**The blocking question in §3 is answered.** `orders.customer_id` is populated only when a logged-in profile is in request context (`internal/handlers/storefront/checkout.go:175-181`); guest checkouts leave it NULL while `customer_email` is `NOT NULL`. Erasure therefore keys on **email**, matching `customer_id` only as a supplementary predicate.
