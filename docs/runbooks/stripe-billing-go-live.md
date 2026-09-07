# Stripe billing go-live checklist

**Issue:** #366 (the record), #371 (the execution).
**Status:** not scheduled. #366's recorded decision is **"not yet"** — production stays on
the test key until the open billing correctness work is closed, because those defects are
latent *only* because `store_subscriptions` has 0 rows.

This document is the thing #366 asks for: what changes meaning the moment the key is
swapped, written down before it is swapped rather than after.

---

## 0. Measured starting state

Verified 2026-09-08 against GCP Secret Manager and the running cluster. Only key prefixes
were ever read; no key value appears here or anywhere in the repo.

| Fact | Value |
|---|---|
| Secret the cluster consumes | `prod-mark8ly-stripe-billing-secret-key-sandbox` |
| Webhook secret consumed | `prod-mark8ly-stripe-billing-webhook-secret-sandbox` |
| `CONSOLE_CATALOG_MODE` on the admin pod | `test` |
| `store_subscriptions` | **0 rows** |
| `stores` / `orders` | 4 / 3 |

Secrets that exist: `…-secret-key-sandbox`, `…-secret-key-test`,
`…-webhook-secret-sandbox`, `…-webhook-secret-test`, plus the two `uat` ones.
The un-suffixed `prod-mark8ly-stripe-billing-secret-key` **404s** — it was renamed after
the 2026-09-03 incident in which a live key was added as a new *version* of the mode-less
secret and the admin pod ran on it for six minutes.

### 0.1 An inconsistency to resolve BEFORE the swap, not during

The cluster runs the **sandbox** key while the catalog mode is **`test`**. Those are two
different Stripe surfaces: a sandbox account's objects are not the main account's
test-mode objects, and the console publishes a catalog per mode.

Whether this is currently harmful depends on which Stripe account the console's `test`
catalog was published against — that has **not** been verified here and must be, because
it is the same class of mismatch this checklist exists to prevent at the live boundary.
Today nothing is charged, so nothing is visibly broken; that is not the same as correct.

### 0.2 The uat secrets still carry the trap that was fixed for prod

`prod-mark8ly-uat-stripe-billing-secret-key` and `…-webhook-secret` have **no mode
suffix**. That is precisely the naming that allowed the 2026-09-03 incident. Rename them
before go-live, or the next person adds a live version to a mode-less name again.

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
- [ ] **The live account needs its own Price objects** with **byte-identical lookup keys**.
      Stripe objects are per-mode; nothing carries over. A missing or differently-keyed
      price fails every subscribe and every plan change at price resolution.

## 2. Preconditions — the recorded gate

#366's decision is that these close first. They are latent today only because nobody is
subscribed; the first live subscription makes each one real.

- [ ] **#699** — AU prices are `tax_behavior: exclusive` and nothing adds GST.
      **Check the framing before fixing:** Tesserix is not GST-registered, so it must
      **not** charge GST. If that still holds, the fix is removing the "Plus GST"
      disclosure, not adding tax. Charging GST while unregistered is the worse outcome.
- [ ] **#702** — Pro+App cancellation never tears down the white-label app.
      Note this is **not** just an unconstructed consumer: the emitted event carries no
      app identifiers, and `apple_app_id` / `google_package` / `firebase_project_id` exist
      *only* in `white_label_app_state`, the table the consumer writes. Nothing captures
      them, so the chain is unimplementable as designed and needs a source decided first.
      (`white_label_app_state`: 0 rows.)
- [ ] **#704** — `radar.early_fraud_warning` is acknowledged and discarded, and
      `arbitrage_flag` cannot be set by any code path.
- [ ] **#620 / #795** — promo redemption and console promo-catalog ingest.

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

1. [ ] Create live Price objects with lookup keys byte-identical to the test set; verify
       by resolving each key against the live account.
2. [ ] Publish the console catalog for `live`.
3. [ ] Create the live webhook endpoint; capture its signing secret into
       `prod-mark8ly-stripe-billing-webhook-secret-live`.
4. [ ] Create `prod-mark8ly-stripe-billing-secret-key-live`. **A new secret, never a new
       version of an existing one** — that is what the 2026-09-03 incident was.
5. [ ] Flip the ExternalSecret and `CONSOLE_CATALOG_MODE` **in one change**.
6. [ ] Verify: a real subscribe end to end, then a plan change, then a cancellation.

## 5. Rollback

Point the ExternalSecret back at the `-test`/`-sandbox` pair and revert
`CONSOLE_CATALOG_MODE` in the same change. This is clean **only while
`store_subscriptions` is empty**: once a live subscription exists, rolling back to test
mode strands a real Stripe customer and subscription id that the test account cannot
resolve.

**That is the argument for doing this while the tables are empty** — and equally the
argument for closing §2 first, because after the first live subscription both the defects
and the rollback stop being cheap.
