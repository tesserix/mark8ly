# Stripe billing webhook — the three lists

An event only does work if it appears in **all three** of these. Two live in
this repo and are kept in step by `TestAllowlistMatchesHandlers`
(`internal/billing/dispatch/allowlist_parity_test.go`). The third is a
dashboard setting that nothing in CI can see, which is why it is written down
here.

| # | List | Where |
|---|------|-------|
| 1 | Enabled events on the Stripe endpoint | Stripe Dashboard → Event destinations |
| 2 | `STRIPE_ALLOWED_EVENT_TYPES` | `pkg/config/config.go` (default; no chart overrides it) |
| 3 | Dispatcher handler map | `internal/billing/dispatch/dispatcher.go` |

## The subscription set

Every destination pointed at the billing webhook must enable exactly these
thirteen:

```
charge.refunded
checkout.session.completed
customer.subscription.deleted
customer.subscription.updated
customer.updated
invoice.finalized
invoice.paid
invoice.payment_action_required
invoice.payment_failed
payment_method.attached
payment_method.detached
radar.early_fraud_warning.created
radar.early_fraud_warning.updated
```

## Endpoint requirements

- **Host must be `api.mark8ly.com`.** The route is mounted on
  marketplace-api (`cmd/marketplace-api/main.go`). `admin.mark8ly.com` routes
  to the admin frontend (tesserix-k8s `virtualservice-wildcard.yaml`) and has
  no such path — a destination pointed there is a 404 that reports a 0% error
  rate until something is actually sent.
- **Path is `/webhooks/stripe-billing`.** Not `/api/v1/...`; that prefix
  belongs to the per-store Connect webhook provisioned by
  `internal/payment/stripewebhook/provision.go`, which is a different
  subsystem.
- **Payload style must be Snapshot.** Every handler reads `data.object`
  (`internal/webhookevents/customerid.go`, `dispatch/handlers.go`). There is
  no thin-event code path; a Thin destination parses to nothing.
- **One destination per mode.** Sandbox destinations must not point at the
  production host. They would fail signature verification against the live
  `STRIPE_BILLING_WEBHOOK_SECRET` — correct, but by luck of mismatched
  secrets, not by design.

## Why a 0% error rate is not evidence

Stripe reports errors as a fraction of deliveries. A destination that has
never been sent anything — because the event type is not in list (1) — shows
0% forever, with a flat activity sparkline. Read the sparkline, not the
percentage.

This is how two defects survived: `invoice.finalized` had a handler but was
missing from list (2), and `radar.early_fraud_warning` sat in lists (2) and
(3) under a name Stripe does not emit. Both were answered `200` and recorded
as processed. Neither had ever run.
