# Outbound webhooks

Mark8ly can notify your systems when things happen in your store — an order
is placed, a return is approved, a product changes — by making a signed HTTP
POST to an endpoint you register. This page is the merchant-facing reference
for integrating with them: the event catalogue, the payload shape, how to
verify the signature, and what your endpoint needs to handle.

Webhooks are available on every plan. There is no gating by tier.

**Completing the loop needs the read-only API.** A delivery body carries only
the event name, the aggregate id and a timestamp — deliberately, so no customer
data is sent to a merchant-supplied URL and a retry can never deliver a stale
copy of an entity. To act on an event you fetch the entity from the REST API
using the id. The read-only API is available on **Starter, Studio and Pro**
(widened to Starter in #585 precisely so every plan that can register a webhook
can also resolve what it delivers).

On **Trial**, the read-only API is not enabled. Trial webhooks still work as
pure signals — ping a Slack channel, increment a counter, prompt someone to
open admin — but a Trial integration cannot fetch the order behind an
`order.placed` until the store is on a paid plan.

Subscriptions, deliveries, replay and test sends are all managed from
**Admin → Settings → Webhooks**.

## Event catalogue

A subscription selects one or more of the following 18 event types (from
`internal/outbox`, the transactional event log every subscription reads
from):

| Event | Fires when |
|---|---|
| `product.created` | A product is created |
| `product.updated` | A product is updated |
| `product.deleted` | A product is deleted |
| `category.created` | A category is created |
| `category.updated` | A category is updated |
| `category.deleted` | A category is deleted |
| `order.placed` | An order is placed |
| `order.confirmed` | An order is confirmed |
| `order.fulfilled` | An order is fully fulfilled |
| `order.partially_fulfilled` | An order is partially fulfilled |
| `order.cancelled` | An order is cancelled |
| `order.refunded` | An order is refunded |
| `return.requested` | A return is requested |
| `return.approved` | A return is approved |
| `return.received` | A returned item is received |
| `return.refunded` | A return is refunded |
| `return.rejected` | A return is rejected |
| `abandoned_cart.recovery_email` | An abandoned-cart recovery email is sent |

A subscription may select up to 32 event types (`webhook.MaxEventTypes`) —
comfortably more than the 18 that exist today, so the cap is only there to
bound a malformed request, not to make you ration event types.

## Payload shape

Every delivery body is exactly three fields — "notify-and-fetch":

```json
{
  "event": "order.placed",
  "id": "3fa85f64-5717-4562-b3fc-2c963f66afa6",
  "occurred_at": "2026-09-02T14:03:11Z"
}
```

- `event` — the event type, one of the values above.
- `id` — the aggregate id (the order id, product id, etc.). Call the REST
  API with this id to fetch the current state of the entity.
- `occurred_at` — when the underlying domain event happened (UTC), not when
  the delivery attempt was made.

That's it — no order lines, no customer name or email, no addresses. Two
reasons the payload is kept this thin, deliberately:

1. **No customer PII reaches a merchant-supplied URL.** The delivery target
   is arbitrary, external, and outside Mark8ly's control. A thin,
   identifier-only payload means a webhook endpoint never becomes a place
   customer data leaks to.
2. **A retry can't deliver a stale entity.** Deliveries retry over hours (see
   below). If the payload carried a snapshot of the order, a retried
   delivery could hand you an order that has since changed — refunded,
   cancelled, fulfilled — while the payload still claims the state at the
   time of the original event. Fetching fresh from the API means you always
   see the current truth, never a stale copy.

Call the corresponding REST endpoint with the `id` to get the full entity.

## Verifying the signature

Every delivery carries an `X-Mark8ly-Signature` header, formatted:

```
t=<unix timestamp>,v1=<hex-encoded HMAC-SHA256>
```

`v1` is `HMAC-SHA256(secret, "<t>.<raw request body>")` — the timestamp is
concatenated with a `.` and the exact raw JSON body, and that concatenation
is what's HMAC'd with your subscription's signing secret.

The timestamp is part of the **signed material itself**, not a sibling
header you check separately. That is what makes rejecting a stale timestamp
trustworthy: an attacker replaying an old, captured delivery cannot rewrite
`t` to make it look recent without invalidating `v1`, because `t` is baked
into the value the signature covers.

Your signing secret is shown once, at subscription-creation time, and never
returned by the API again — store it somewhere you can read back at
verification time.

### Worked example (Node.js)

```js
const crypto = require("crypto");

// Replace with the secret shown when you created the subscription.
// NEVER commit a real signing secret to source control.
const WEBHOOK_SECRET = "whsec_REPLACE_ME";

// Reject anything older than 5 minutes to defeat replay of a captured
// delivery. Tune to your clock skew tolerance, but keep it tight — the
// whole point of checking `t` is that it's recent.
const MAX_AGE_SECONDS = 5 * 60;

function verifyMark8lySignature(header, rawBody, secret) {
  const parts = Object.fromEntries(
    header.split(",").map((kv) => kv.split("=")),
  );
  const t = parts.t;
  const v1 = parts.v1;
  if (!t || !v1) {
    throw new Error("malformed signature header");
  }

  // Reject stale deliveries BEFORE trusting anything else. This works only
  // because `t` is signed material — see above.
  const age = Math.abs(Date.now() / 1000 - Number(t));
  if (age > MAX_AGE_SECONDS) {
    throw new Error("stale webhook timestamp — possible replay");
  }

  // Recompute the HMAC over "<t>.<raw body>" — the exact bytes received,
  // before any JSON.parse.
  const expected = crypto
    .createHmac("sha256", secret)
    .update(`${t}.${rawBody}`)
    .digest("hex");

  // Constant-time comparison — a naive `expected === v1` string compare
  // leaks timing information an attacker can use to guess the signature
  // byte by byte.
  const a = Buffer.from(expected, "hex");
  const b = Buffer.from(v1, "hex");
  if (a.length !== b.length || !crypto.timingSafeEqual(a, b)) {
    throw new Error("signature mismatch");
  }
}

// Express example — must use the RAW body, not a parsed one, since the
// signature covers the exact bytes sent.
app.post(
  "/webhooks/mark8ly",
  express.raw({ type: "application/json" }),
  (req, res) => {
    try {
      verifyMark8lySignature(
        req.header("X-Mark8ly-Signature"),
        req.body, // Buffer — raw bytes
        WEBHOOK_SECRET,
      );
    } catch (err) {
      return res.status(401).send("invalid signature");
    }

    const payload = JSON.parse(req.body);
    // ... handle payload.event / payload.id / payload.occurred_at ...
    res.sendStatus(200);
  },
);
```

The same shape works in any language: split the header on `,` and `=`,
reject a stale `t`, recompute HMAC-SHA256 over `"<t>.<raw body>"` with your
secret, and compare with a constant-time comparison function (not `==`/
`===`/`memcmp`-without-constant-time).

## Delivery behaviour

**At-least-once, not exactly-once.** A delivery can be sent more than once —
after a retry whose success response was lost in transit, for example. Your
endpoint must be idempotent: dedupe on the delivery, not just act on receipt.
The delivery id is available in the admin delivery log; if you need a
dedupe key from the payload itself, `event` + `id` + `occurred_at` together
identify a specific delivery attempt's underlying event.

**Retry with backoff.** A failed attempt (non-2xx response, or the request
otherwise failing) is retried with exponential backoff: roughly 30s, 2m,
8m, 32m, then ~2h before the final attempt — five retries after the
initial send, six attempts total. (The backoff formula itself is capped at
4h between attempts, but with only 6 attempts total that cap is never
actually reached — the delay before the last attempt tops out around 2h.)
This is spread over hours so a merchant endpoint that's briefly down (a
deploy, a restart) has time to recover before attempts run out.

**Dead-lettering.** After 6 attempts (`webhook.MaxAttempts`) a delivery is
marked `failed` and kept — visible and replayable from admin — but no longer
retried automatically.

**Retention.** Delivery rows (successful and failed) are kept for 30 days
(`webhook.RetentionWindow`) on every plan, then pruned. This is not a paid
tier's extended log — treat the delivery log as a debugging aid for recent
activity, not a permanent audit trail.

**Auto-disable.** If a subscription accumulates 10 **consecutive** failures
(`webhook.FailureThreshold`) — across different events, not just repeated
attempts at the same one — the subscription is disabled automatically. The
threshold is
deliberately higher than the per-delivery attempt cap so a single bad
delivery can't take down an otherwise-working endpoint; it takes sustained
failure. Any deliveries still queued for that subscription are marked
`failed` rather than sent, so a disabled webhook stops making requests to
your endpoint immediately.

**Where a disabled webhook shows up.** Nothing is emailed to you today, so
check admin: a disabled subscription appears in **Settings → Webhooks**
marked as disabled, with the reason it was disabled and when. If deliveries
stopped arriving and you are not sure why, that page is the place to look
first. Once you've fixed your endpoint, re-enable the subscription from
admin and replay the failed deliveries you want retried.

## Endpoint requirements

- **HTTPS only.** Plain `http://` URLs are rejected at registration.
- **Publicly resolvable.** The hostname is resolved both when you register
  the URL and again immediately before every delivery, and rejected if it
  resolves to a private, loopback, link-local, or cloud-metadata address.
  This means `localhost`, addresses on your office LAN, and cloud metadata
  endpoints (e.g. `169.254.169.254`) can never be used as a webhook target —
  including if a hostname that used to resolve publicly is later repointed
  at one of those addresses.
- **No redirects followed.** A 3xx response from your endpoint is not
  followed. If your endpoint moves, update the registered URL directly.
- **Respond 2xx to acknowledge.** Any status outside 200–299 is treated as a
  failure and retried per the backoff schedule above.
- **Respond promptly.** Requests are made with a short timeout; a slow
  endpoint is indistinguishable from a failing one and will be retried the
  same way.
- **Response bodies are read but bounded.** On a failing response, a bounded
  snippet of the body is captured and shown in your delivery log to help you
  debug — but it is never logged server-side, since it's arbitrary remote
  content. On success, the body is discarded unread beyond what's needed to
  reuse the connection. Don't rely on returning data in the response body;
  fetch state from the REST API instead.

## Summary checklist

- [ ] Endpoint is HTTPS, publicly resolvable, and does not redirect
- [ ] Endpoint returns 2xx promptly on success
- [ ] Signature is verified against the **raw** request body, not a
      re-serialized/parsed one
- [ ] `t` is checked for staleness before anything else
- [ ] Comparison of the computed and received signature is constant-time
- [ ] Handler is idempotent — safe to receive the same delivery twice
- [ ] Handler calls the REST API for entity detail rather than assuming the
      payload carries it
