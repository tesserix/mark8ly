# Outbound webhooks: merchant subscriptions over the existing outbox

Design for #562. Written 2026-09-02, after #560 and #565 removed "webhooks"
from the pricing table — it had been advertised on Studio for some time
while no outbound delivery existed anywhere in the service (#544).

## What this delivers

A merchant registers an HTTPS endpoint, picks which events they care about,
and receives a signed POST when those events happen. Failures retry, then
dead-letter. A permanently broken endpoint is disabled and the merchant is
told. Every delivery is visible in admin and replayable.

## The expensive half already exists

`outbox_events` is a transactional outbox: rows are written in the same
transaction as the mutation that produces them (`EnqueueInTx`), and an
in-process publisher drains them with `SELECT … FOR UPDATE SKIP LOCKED`.

It already carries eighteen domain events, all of them written by real
producers in `internal/order`, `internal/product` and `internal/category`:

```
order.placed          order.confirmed    order.fulfilled
order.partially_fulfilled                order.cancelled     order.refunded
return.requested      return.approved    return.received
return.refunded       return.rejected
product.created       product.updated    product.deleted
category.created      category.updated   category.deleted
abandoned_cart.recovery_email
```

So this feature captures nothing. It is a **consumer** of an event stream
that is already reliable and already transactional — which is what makes it
a contained piece of work rather than the multi-week build it looks like.

## Product decisions

Decided 2026-09-02. Recorded with their rejected alternatives, because each
has a cheaper option a later reader might assume was overlooked.

### 1. Notify-and-fetch payloads

The delivery body carries the event name, the aggregate id, and a timestamp.
The merchant calls the REST API for detail.

Rejected: full payloads. They would ship customer PII to merchant-supplied
URLs, and a retry hours later would deliver a stale snapshot of an order that
has since changed. Thin payloads also match what the outbox already stores —
its payloads are identifiers plus `store_id`, not entities.

The cost is a second round trip, and a hard dependency on the merchant having
API access. See decision 4.

### 2. Strict URL validation, re-resolved at delivery

HTTPS only. The hostname is resolved at registration **and again immediately
before each delivery**, and the request is rejected if it resolves to a
private, loopback, link-local, or cloud-metadata address. Redirects are not
followed.

Rejected: validating only at registration. That is the common shortcut and it
is defeated by DNS rebinding — a hostname that resolves public when saved and
private when delivered. The re-resolution is the part that actually matters.

This is the first place merchant input becomes an egress target from the
cluster. Every other outbound integration (payment gateways, carriers, email
providers) goes to fixed, configured endpoints.

The cost is real: a merchant cannot point a webhook at `localhost` to test,
and "why won't it accept my URL" becomes a support question. Accepted.

### 3. Retry, dead-letter, then auto-disable

Exponential backoff over a few hours. A delivery that exhausts its attempts
is recorded as failed and kept. After a threshold of consecutive failures
across *different* events, the subscription is disabled. They fix the
endpoint, re-enable, and replay failed deliveries. Deliveries already
pending when a subscription is disabled are retired as failed rather than
sent, so disabling actually stops outbound traffic instead of only stopping
new deliveries being created.

**What ships today, and what does not.** The merchant is NOT emailed. The
worker has the notification hook (`webhook.NewWorker`'s `notify` callback,
which fires exactly once, on the transition to disabled) but it is wired to
nil: the email path is its own piece of work and has not been built. What a
merchant actually gets is the disabled subscription in admin settings with
its reason and timestamp, and a server-side warning log. That is weaker than
this decision describes — a merchant who is not looking at admin is not told
— and the email remains outstanding. Recorded here rather than left as an
aspiration the code does not meet.

Rejected: retrying forever. A dead endpoint would have the cluster making
outbound requests indefinitely, and on a shared `db-f1-micro` with a small
node pool a handful of those is not free.

Rejected: dropping silently. A webhook that stopped working without saying so
is worse for a merchant than one that told them — especially for order events.

### 4. Available on every plan

Rejected: gating to Studio+. The events already exist; gating mainly creates
support friction, and webhooks are not the differentiator the plan copy
implied.

### 5. Read-only API widened to Starter

Decision 1 makes a webhook useless without API access to fetch the detail,
and the read-only API is currently Studio+. A Starter merchant would get a
doorbell with the door locked.

**This is a plan-entitlement change, not a webhook change**, and it needs its
own issue: it alters what Studio sells and requires matching pricing copy on
both public surfaces (`Pricing.tsx` and `admin/lib/copy/pricing.ts`, kept
honest by `pricing-surfaces-truth.spec.ts`). It must land before or with
webhooks, or decision 1 is incoherent for Starter merchants.

## Architecture

Two loops and one new table, running in-process beside the existing
`outbox.Publisher`.

```
outbox_events ──(dispatcher: own cursor)──▶ webhook_deliveries ──(worker)──▶ merchant endpoint
                                                    ▲
                                      webhook_subscriptions
```

**Dispatcher.** Polls `outbox_events` against its own cursor — deliberately
not the publisher's watermark — matches each event to enabled subscriptions
for that tenant and event type, and inserts one `webhook_deliveries` row per
(event, subscription). Idempotent on that pair.

**Delivery worker.** Polls `webhook_deliveries WHERE status='pending' AND
next_attempt_at <= now()` with `FOR UPDATE SKIP LOCKED`, sends, records the
outcome. Retries, dead-lettering, replay and the merchant-facing delivery log
all fall out of this one table.

### Why not extend the existing publisher

Fanning out inside `outbox.Publisher.ProcessBatch` would give exactly-once
fan-out for free, in the same transaction that marks the row published. It is
rejected because it welds untrusted-network work with unbounded latency into
the watermark publisher — the component whose failure semantics
`outbox/models.go` documents at length (#336) as subtle and awkward to
recover from. A merchant's dead endpoint must never stall internal
bookkeeping.

The exactly-once property that coupling would buy is recovered by making
fan-out idempotent on `(outbox_event_id, subscription_id)`.

### Why in-process rather than a separate workload

Both loops run as goroutines in the admin-mode pod, matching how
`outbox.Publisher` already runs. `FOR UPDATE SKIP LOCKED` makes this safe
across KEDA-scaled replicas.

Rejected: a dedicated Deployment. It buys isolation — merchant endpoints
could not touch API latency — at the cost of another chart, another ArgoCD
Application, and another pod's memory on a cluster that has already had
rollouts deadlock under memory pressure (the `maxSurge: 0` workaround in the
mark8ly charts exists because of exactly that). Adding a permanent pod for a
feature with no users yet is the wrong first move.

Rejected: a CronJob. Minimum granularity is 60 seconds, which undercuts the
point of a webhook.

The risk accepted is that a burst of slow endpoints ties up goroutines in a
pod that also serves admin API requests. Bounded by a small worker pool and a
short per-request timeout, so the blast radius is capped rather than absent.

**This is a reversible decision.** Because the fan-out table already
decouples dispatch from delivery, extracting the delivery loop into its own
workload later is a deployment change, not a redesign.

## Schema

`webhook_subscriptions` — tenant and store scoped, URL, selected event types,
signing secret, enabled flag with a disabled-reason and disabled-at, plus the
consecutive-failure counter that drives auto-disable.

`webhook_deliveries` — subscription id, outbox event id, event type, status
(`pending` / `delivered` / `failed`), attempt count, `next_attempt_at`, last
response status and a truncated response body for the merchant-facing log,
timestamps. Unique on `(outbox_event_id, subscription_id)` — that constraint
is what makes dispatch idempotent.

Both tables carry `tenant_id`, unlike `journal_subscribers` (#153), because a
webhook subscription genuinely belongs to a merchant.

Delivery rows are pruned on a schedule, following `audit/prune_cron.go`.
Retention is **30 days on every plan**, deliberately not tied to
`FeatureAuditRetentionDays` — "forever" retention of delivery bodies on Pro
is a storage cost on a `db-f1-micro` with no corresponding merchant value.

## Delivery semantics

Each request carries an HMAC signature over a timestamp and the raw body,
using the subscription's secret, in a header alongside that timestamp. The
timestamp is inside the signed material so a captured delivery cannot be
replayed later. The verification recipe is documented for merchants, with a
worked example, and the secret is shown once at creation and never again.

A 2xx is success. Anything else, a timeout, or a connection failure is a
retryable failure until attempts are exhausted.

## Admin surface

Register, edit and delete a subscription; pick event types; send a test
event; view recent deliveries with their status and response code; replay a
failed delivery. Follows the existing admin settings patterns.

## Testing

The SSRF guard is the piece most worth getting right and is built as a
separately-testable unit, including a DNS-rebinding case where registration
and delivery resolve differently.

Beyond that: dispatch is idempotent under repeated runs; retry and backoff
scheduling; auto-disable fires on consecutive failures and not on isolated
ones; signature verification; and a test asserting that a stalled webhook
delivery cannot block the outbox watermark publisher — the failure this
architecture exists to prevent.

Integration tests run against a throwaway `postgres:15`, with the **full**
suite run before opening a PR, not just the touched packages. The last two
features here each tripped a guard in an untouched package (the customer
erasure coverage guard, the CI contract test) that a narrow run would have
missed.

## Out of scope

**Custom code injection** — the other half of #544. Deliberately not being
built: arbitrary `<script>` in merchant storefronts is XSS against their
customers by design, and shipping it permanently weakens the storefront CSP.

**Restoring "webhooks" as a Studio pricing bullet.** It is on every plan, so
it is not a tier differentiator. `pricing-surfaces-truth.spec.ts` currently
fails on any pricing surface mentioning webhooks; that guard should be
narrowed rather than deleted when this ships, so the code-injection half stays
pinned.

## Open items

- The plan-entitlement change in decision 5 needs its own issue before this
  can ship coherently.
- Retry schedule, attempt count and the auto-disable threshold are left to
  the implementation plan; nothing in the design turns on the exact numbers.
