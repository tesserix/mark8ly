# Cashfree webhook — payment and refund status push

> Part of the Cashfree payment integration.
> For how the adapter itself behaves (amounts, refunds, idempotency), read
> the header comment in
> [`services/marketplace-api/internal/payment/cashfree.go`](../services/marketplace-api/internal/payment/cashfree.go).
> For the Razorpay equivalent of the client-callback half, see
> [`payment_verify.go`](../services/marketplace-api/internal/handlers/storefront/payment_verify.go).

`marketplace-api` exposes a public, per-store webhook endpoint that Cashfree
POSTs to on every payment and refund state change. This document is the manual
runbook for wiring one merchant by hand; it exists to be **automated away** —
see [Automating this](#automating-this) for exactly which steps a provisioning
job would take over and what makes each one safe to script.

## What the webhook is and isn't responsible for

Read this first, because it changes how urgently a broken webhook needs fixing.

Checkout does **not** depend on the webhook. Cashfree's JS SDK returns no signed
receipt, so after the payment sheet closes the storefront calls
`POST /api/v1/storefront/stores/{slug}/orders/{id}/confirm-payment`, and the
server asks Cashfree directly what was captured
(`GET /pg/orders/{order_id}/payments`) before marking the order paid. An order
therefore confirms even if no webhook ever arrives.

The webhook is the asynchronous backstop, and it is the **only** path for two
things:

| Event | Why the webhook is needed |
|---|---|
| `REFUND_STATUS_WEBHOOK` | A Cashfree refund can settle as `PENDING`/`ONHOLD` and finish minutes later. Nothing else re-checks it, so without this the refund ledger row never reaches `succeeded`. |
| Payments completed off-session | UPI collect requests approved after the buyer closed the tab, or any flow where the browser never returns to run `confirm-payment`. |

So: checkout works without it, refund accounting does not.

## Endpoint

```
POST https://api.mark8ly.com/api/v1/webhooks/<slug>/cashfree
```

- `api.mark8ly.com` is the public host that Istio routes to
  `mark8ly-marketplace-api-storefront` (an `authority: exact` match on the
  `mark8ly-wildcard` VirtualService).
- `<slug>` is the store's slug — the subdomain of its storefront. If admin is
  `my-god-admin.mark8ly.com`, the slug is `my-god`.
- The trailing `cashfree` is the provider path segment the router matches. It
  must be exactly that string; it is what selects the Cashfree adapter.

To list slugs:

```bash
export KUBECONFIG=~/.kube/gke-prod
kubectl exec -n mark8ly mark8ly-postgres-2 -- \
  psql -d mark8ly_marketplace_api \
  -c "SELECT slug, name, country_code, currency_code FROM stores WHERE status='active' ORDER BY slug;"
```

### Why the URL is store-scoped

There is also a legacy unscoped route (`/api/v1/legacy-webhooks/cashfree`) that
resolves the gateway config by provider alone. **Do not use it.** It returns
`200 ambiguous` and drops the event whenever more than one tenant has an active
config for the provider — deliberately, because guessing which merchant a
signed payload belongs to is how one tenant's webhook settles another tenant's
order. Always configure the `/<slug>/` form.

## Configuring on merchant.cashfree.com

1. Log in to <https://merchant.cashfree.com>.
2. Click **Switch to Test** (top right). Do this first — a webhook added while
   the dashboard is in production scope will not fire for sandbox payments.
   Production activation being "in review" does not block test-mode work.
3. Open **Developers** (top nav) → **Webhooks** under the Payment Gateway
   section.
4. **Add Webhook Endpoint**, and for **Endpoint URL** paste the URL from above
   with a real slug substituted:
   ```
   https://api.mark8ly.com/api/v1/webhooks/my-god/cashfree
   ```
   (`my-god` is the IN/INR test store — see
   [`testing-payments.md`](./testing-payments.md).)
   Cashfree validates the field, so a literal `<slug>` placeholder is rejected
   with "Please enter a valid URL" — the angle brackets are not legal URL
   characters.
5. **Select Webhook Version:** choose **`2023-08-01`**.

   This is not cosmetic. The adapter pins `x-api-version: 2023-08-01` and parses
   that generation's envelope — `type` at the top level, ids nested under
   `data.order`, `data.payment` and `data.refund`. An older version sends a
   flatter payload in which the parser finds no order id: the signature still
   verifies, and the event then resolves to an empty order and updates nothing.
   That failure is silent, which is what makes the dropdown worth care.
6. Subscribe to exactly these four events — the only ones
   `normalizeCashfreeEvent` maps:

   | Cashfree event | Normalized to | Effect |
   |---|---|---|
   | `PAYMENT_SUCCESS_WEBHOOK` | `payment.succeeded` | Order confirmed, payment transaction → `captured` |
   | `PAYMENT_FAILED_WEBHOOK` | `payment.failed` | Payment transaction → `failed` |
   | `PAYMENT_USER_DROPPED_WEBHOOK` | `payment.failed` | Same handling — buyer abandoned the sheet, order stays reserved |
   | `REFUND_STATUS_WEBHOOK` | `refund.succeeded` | Refund ledger row settled |

   Any other event type is logged as unhandled and ignored, so subscribing to
   more is harmless noise rather than an error.
7. Save. If Cashfree offers a per-endpoint **signing secret**, copy it now — you
   will paste it into admin in the next section. If it does not, Cashfree signs
   with your Secret Key and the backend already falls back to that.

## Configuring the mark8ly side

Admin → **Settings → Payments → Cashfree**:

| Field | Value |
|---|---|
| API key | Cashfree **App ID** (`x-client-id`), from Developers → API Keys |
| Secret key | Cashfree **Secret Key** (`x-client-secret`) |
| Webhook secret | The per-endpoint secret from step 7, or blank to fall back to the Secret Key |
| Mode | **test** for sandbox keys, **live** for production keys |

Then toggle the provider **active**.

`mode` selects the API **host**, not just the credentials
(`sandbox.cashfree.com` vs `api.cashfree.com`), so a mismatch is not a subtle
degradation — live keys in test mode cannot reach production at all, and test
keys in live mode fail authentication. That is deliberately safer than
Razorpay's single-host, key-prefix scheme.

Credentials are written to GCP Secret Manager and the row stores a `gsm://`
reference. For out-of-band provisioning (UAT seeding, first merchant, recovery)
use [`scripts/sql/cashfree-enable.sql`](../services/marketplace-api/scripts/sql/cashfree-enable.sql),
whose header documents the plaintext-until-first-read caveat.

### Prerequisite: the country allowlist

`supported_countries.payment_providers` gates both the admin write path and the
storefront read path. Migration `000099_supported_countries_cashfree` adds
`cashfree` to India's list. Without it, admin rejects the provider outright and
the storefront never offers it — so a webhook configured first would arrive for
a store that has no Cashfree config and be dropped.

Array **order is preference**: the payment-methods endpoint sorts its response
by position (`sortByPreference`), and the storefront pre-selects the first entry
and badges it *Recommended*.

**Razorpay is the default in India, not Cashfree.** 000099 originally promoted
Cashfree to the head; `000100_razorpay_preferred_in_india` reversed that, so the
live order is `{razorpay,paypal,cashfree}` — Cashfree is fully configured and
selectable, it is simply not pre-selected. Buyers reach it by choosing it at
checkout. Because the ordering is read from data rather than named in code,
flipping the default back is a one-row migration, not a deploy.

## Signature verification

Cashfree signs `base64(HMAC-SHA256(timestamp + rawBody, secret))` and sends:

- `x-webhook-signature` — the base64 digest
- `x-webhook-timestamp` — epoch seconds, **part of the signed material**

Two consequences worth knowing when debugging:

- The **raw** body must be hashed. A re-marshalled parse produces a different
  byte sequence and will never match.
- Because the timestamp is signed, a captured delivery could otherwise be
  replayed verbatim forever. Verification rejects anything older than
  **30 minutes** (`cashfreeWebhookMaxSkew`), on top of the
  `webhook_events (provider, provider_event_id)` unique index that makes a
  genuine redelivery a no-op.

The `Gateway.VerifyWebhook` interface takes one signature string, so the route
packs both headers as `"<timestamp>.<signature>"` via
`payment.CashfreeWebhookSignature` — the same "composite signature" approach
PayPal already uses for its header set.

## Verifying it works

Use the **Test** button on the Cashfree webhook endpoint, then:

```bash
psql "$DATABASE_URL" -v store_slug=my-god \
  -f services/marketplace-api/scripts/sql/cashfree-verify.sql
```

Section 6 lists received webhook events by type; sections 2–3 report credential
and configuration problems. Live logs:

```bash
export KUBECONFIG=~/.kube/gke-prod
kubectl logs -n mark8ly -l app=mark8ly-marketplace-api-storefront --tail=200 | grep -i cashfree
```

| Symptom | Cause |
|---|---|
| `signature mismatch` | Webhook secret in admin ≠ the dashboard's signing secret. If you left admin blank, the dashboard must be signing with the Secret Key. |
| `stale or invalid timestamp` | Delivery older than 30 minutes, or a malformed `x-webhook-timestamp`. Replays fail here by design. |
| `no active gateway config for (store, provider)` | Slug in the URL doesn't match a store with an active Cashfree config. |
| `webhook: unhandled event type` | Subscribed to an event outside the four above. Harmless. |
| 404 from the endpoint | Wrong path. Check `/api/v1/webhooks/<slug>/cashfree`, not `/webhooks/...`. |
| Verifies but nothing updates | Almost always the wrong **webhook version** — the payload parsed but carried no order id. See step 5. |

## Automating this

The manual steps split cleanly into two halves, and only one of them needs
Cashfree's API.

**Already scriptable today** (no Cashfree involvement):

- Country allowlist — migration, done.
- Gateway config row — `scripts/sql/cashfree-enable.sql`, idempotent on
  `(store_id, provider)`, so re-running rotates rather than duplicating.
- Verification — `scripts/sql/cashfree-verify.sql`.

**Needs Cashfree's API**: creating the webhook endpoint itself. Cashfree does
not expose webhook-endpoint management under the same PG credentials the adapter
uses, so an automation would either drive the partner/onboarding API (if the
account is enabled for it) or keep this as a documented one-time manual step per
merchant.

Two properties make the rest safe to automate whenever that lands:

1. **The URL is derivable.** It is a pure function of the store slug, so
   provisioning can compute it without any lookup or human choice.
2. **Re-running is safe.** The endpoint is idempotent at the event level via the
   `webhook_events` unique index, so re-registering the same URL cannot cause
   double-processing.

The honest constraint: the **webhook version** and the **event subscription
list** are the two settings a script must assert rather than assume, because
both fail silently when wrong — a bad version parses to an empty order id, and a
missing `REFUND_STATUS_WEBHOOK` subscription leaves refunds pending forever with
no error anywhere.
