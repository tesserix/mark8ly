# Stripe billing go-live checklist

**Issue:** #366 (the record), #371 (the execution).
**Status:** not scheduled. #366's recorded decision is **"not yet"** — production stays on
the test key until the open billing correctness work is closed.

This document is the thing #366 asks for: what changes meaning the moment the key is
swapped, written down before it is swapped rather than after.

**Re-measured 2026-09-08.** Most of §2 has closed and two steps of §4 turn out to be
already done, so the remaining work is smaller than this document said. Corrections are
marked **UPDATED** where a claim changed rather than being silently rewritten — the point
of a go-live record is that a reader can see what moved.

---

## 0. Measured starting state

Verified 2026-09-08 against GCP Secret Manager and the running cluster. Only key prefixes
were ever read; no key value appears here or anywhere in the repo.

| Fact | Value |
|---|---|
| Secret the cluster consumes | `prod-mark8ly-stripe-billing-secret-key-sandbox` |
| Webhook secret consumed | `prod-mark8ly-stripe-billing-webhook-secret-sandbox` |
| `CONSOLE_CATALOG_MODE` on the admin pod | `test` |
| `store_subscriptions` | **4 rows, none carrying a Stripe id** — UPDATED, see below |
| `stores` / `orders` | 4 / 3 |
| `white_label_app_state` | 0 rows |
| GSM secrets ending `-live` | **none yet** — §4.3 and §4.4 are outstanding |

### 0.0 UPDATED — "`store_subscriptions` has 0 rows" is no longer the right check

It returns **4**, and every earlier version of this document and of #366, #699 and #704
argues from the zero. Read literally today, the check now says go-live has already
happened and rollback is unsafe. It has not, and it is not.

All four rows are `plan = trial`, `status = trialing`, created at onboarding between
2026-04-29 and 2026-09-05, and **every one has an empty `stripe_subscription_id` and
`stripe_customer_id`**. Nothing has ever been created in Stripe.

So the property the reasoning actually rests on is not the row count — it is that **no row
references a Stripe object**. That is what makes the billing defects latent and the
rollback in §5 clean, and it survives more onboarding where the row count does not. The
check to run is:

```sql
SELECT count(*) FROM store_subscriptions
 WHERE coalesce(stripe_subscription_id, '') <> ''
    OR coalesce(stripe_customer_id, '') <> '';
-- 0 today. The moment this is non-zero, §5's rollback strands a real Stripe object.
```

Secrets that exist: `…-secret-key-sandbox`, `…-secret-key-test`,
`…-webhook-secret-sandbox`, `…-webhook-secret-test`, plus the two `uat` ones.
The un-suffixed `prod-mark8ly-stripe-billing-secret-key` **404s** — it was renamed after
the 2026-09-03 incident in which a live key was added as a new *version* of the mode-less
secret and the admin pod ran on it for six minutes.

### 0.1 RESOLVED — the sandbox key and the `test` catalog are the same account

This section previously flagged the cluster running a **sandbox** key while the catalog
mode read **`test`**, and said the account the console's `test` catalog was published
against had not been verified. It has been, twice and independently:

- **#696** (closed) established the intended target for `test` is the sandbox account
  `acct_1SgwbhE1SGxvhzVd`, "because that is where the console publishes test prices", and
  its fix is precisely why the cluster now mounts `…-secret-key-sandbox` rather than
  `…-secret-key-test`.
- The console's own chart records the same account for its test write key, verified
  2026-09-05 by resolving `mark8ly_pro_monthly_developed_v1` against it (1 match — the
  live account's test mode holds none).

The naming remains genuinely ambiguous: `CONSOLE_CATALOG_MODE=test` and a secret suffixed
`-sandbox` describe the same surface under two vocabularies. That is a legibility problem,
not a correctness one, and #696 already recommends naming the suffix for the account
surface rather than the mode. Nothing here blocks the swap.

### 0.2 The uat secrets still carry the trap that was fixed for prod

`prod-mark8ly-uat-stripe-billing-secret-key` and `…-webhook-secret` have **no mode
suffix**. That is precisely the naming that allowed the 2026-09-03 incident. Rename them
before go-live, or the next person adds a live version to a mode-less name again.

Still true, re-confirmed 2026-09-08 — the eight `mark8ly` Stripe secrets in GSM are the
four `prod-…-{sandbox,test}` pair members, these two mode-less `uat` ones, and two
unrelated per-tenant payment keys. **This is the only item in §0 that is still open**, and
it is the cheapest one on the page.

---

## 1. What silently changes meaning at the swap

Each of these changes behaviour with **no code change**, which is why they are listed
rather than trusted to review.

- [ ] **Every Stripe write becomes real money.** Trial-end pushes, plan changes,
      cancellations and refunds stop being sandbox operations.
- [ ] **`CONSOLE_CATALOG_MODE` must move in the same change.** If the key goes live and
      the mode does not, marketplace-api resolves prices from the **test** catalog while
      charging the **live** account — two independent object sets that may disagree on
      amount. This is a single atomic change, not two.
- [ ] **The webhook secret must change with it.** `prod-mark8ly-stripe-billing-webhook-secret-*`
      is per-endpoint and per-mode. A live key with a test webhook secret means every
      inbound event fails signature verification — and the failure is silent from the
      merchant's side.
- [x] **The live account needs its own Price objects** with **byte-identical lookup keys**.
      Stripe objects are per-mode; nothing carries over. A missing or differently-keyed
      price fails every subscribe and every plan change at price resolution.
      **SATISFIED** — see §4.1 and §4.2. Kept on this list rather than deleted, because it
      is still a thing that changes meaning at the swap: what makes it safe is a nightly
      check that can start failing, not a fact that was true once.

## 2. Preconditions — the recorded gate

#366's decision is that these close first. They are latent today only because no
subscription references a Stripe object (§0.0); the first live subscription makes each one
real.

**UPDATED 2026-09-08: four of the five are closed.** Only #699 remains, and what is left
of it is two lines of admin pricing copy.

- [ ] **#699 — OPEN.** AU prices are `tax_behavior: exclusive` and nothing adds GST.
      **The framing inverted on the issue and this entry had it backwards:** Tesserix is
      not GST-registered, so it must **not** charge GST — the charging behaviour is already
      correct, and the defect is the "Plus GST" disclosure on `apps/admin/app/pricing/
      PricingClient.tsx:51` stating a charge that does not happen and cannot lawfully
      happen. The fix is removing the disclosure and the two tests pinning it.
      **Leave `tax_behavior: "exclusive"` on the AUD rows** — GST registration is expected
      later, and the issue explicitly reversed an earlier suggestion to change it. This is
      deferred, not cancelled; the A$75,000 threshold gives today's answer an expiry date.
- [x] **#702 — CLOSED** (#860). Pro+App cancellation and the white-label teardown.
- [x] **#704 — CLOSED.** `radar.early_fraud_warning` and the unreachable `arbitrage_flag`.
- [x] **#620 / #795 — CLOSED.** Promo redemption and console promo-catalog ingest.

## 3. Startup assertion — decided shape, still to build

#366 asks whether startup should assert key mode. **Yes**, and the 2026-09-03 incident is
the argument. Two findings constrain how:

- [ ] The key is **restricted** (`rk_`), and restricted keys are **permission-denied on
      `/v1/account`** (`more_permissions_required`). An assertion cannot identify the
      account or mode that way.
- [ ] **Mode alone is insufficient.** There are two test surfaces — the sandbox account
      and the main account's own test mode — and mode-checking cannot tell them apart.
      Wrong-*account* was the root of #696.

**Assert by resolving a known `lookup_key` against the configured account.** It is one
API call, needs no elevated permission, and fails on the wrong account as well as the
wrong mode — which prefix inspection cannot do. Prefix inspection (`rk_test_` / `sk_test_`
vs `_live_`) is still worth doing first because it is free and needs no network.

## 4. Execution order (#371)

**UPDATED 2026-09-08: steps 1 and 2 are already done.** They are the two this document
called the most likely to be discovered at the worst possible moment, and neither is
outstanding. The remaining work is three steps and a verification.

- [x] **1. Live Price objects with byte-identical lookup keys.** Done. #696 measured the
      live mode of `acct_1SgwbFCyiazmanuP` holding all 42, `mark8ly_pro_monthly_developed_v1`
      included, at matching amounts.
- [x] **2. Publish the console catalog for `live`.** Done — published 2026-09-03
      (revision `fb9c1667…`). Its content is **identical to the test publication**: 42
      prices and 78 currency amounts on each side, with zero rows present in one and not
      the other.

      This is continuously evidenced rather than measured once. The console's nightly
      parity job reports `mode=live source=mark8ly outcome=clean differenceCount=0`
      alongside test, `tesserix_console_stripe_parity_window_satisfied` is `1`, and two
      Prometheus rules (`ConsoleStripeParityDifference`, `ConsoleStripeParityStale`) alert
      on both a difference and on the check going quiet.
- [ ] **3. Create the live webhook endpoint**; capture its signing secret into
      `prod-mark8ly-stripe-billing-webhook-secret-live`.
- [ ] **4. Create `prod-mark8ly-stripe-billing-secret-key-live`.** **A new secret, never a
      new version of an existing one** — that is what the 2026-09-03 incident was.
      Confirmed 2026-09-08: no `-live` secret of either kind exists yet.
- [ ] **5. Flip the ExternalSecret and `CONSOLE_CATALOG_MODE` in one change.**
- [ ] **6. Verify**: a real subscribe end to end, then a plan change, then a cancellation.

**One consequence of step 1 being satisfied, recorded on #696 and worth repeating here:**
the live key is the first key that would make price resolution succeed at all, so the swap
does not merely change money mode — it repairs the flow. The first thing that starts
working is also the first thing that charges someone.

## 5. Rollback

Point the ExternalSecret back at the `-test`/`-sandbox` pair and revert
`CONSOLE_CATALOG_MODE` in the same change. This is clean **only while no
`store_subscriptions` row carries a Stripe id** (UPDATED — the table is no longer empty,
see §0.0, but no row references a Stripe object). Once a live subscription exists, rolling
back to test mode strands a real Stripe customer and subscription id that the test account
cannot resolve.

**That is the argument for doing this while no Stripe object is referenced** — and equally
the argument for closing §2 first, because after the first live subscription both the
defects and the rollback stop being cheap.
