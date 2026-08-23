# Admin KPIs — Design

**Issue:** #282. Part of the platform console integration series (#260), after #274, #275, #276, #277, #279, #283.

**Goal:** Give the console's product tile a set of headline counters for mark8ly, and — just as importantly — a way to say "this metric is not instrumented" that cannot be mistaken for zero.

## The failure this endpoint exists to prevent

The console's existing KPI route falls through to an empty object for products it does not recognise. One product has therefore rendered four em-dashes since launch: **dashes that look like zeroes**.

"Not instrumented" and "zero" are different answers. A tile showing `0` active tenants is an emergency; a tile showing "not instrumented" is a backlog item. Collapsing them costs someone an afternoon.

So: **no key is ever omitted to mean "unavailable", and no unavailable value is ever rendered as `0`.**

## The key registry

Every key mark8ly knows is declared in one place, each marked instrumented or not. Both the `200` payload and the `501` responses are driven by that declaration rather than by conditionals scattered through a handler.

This makes three states distinguishable, which is the whole point:

| state | response |
|---|---|
| known and instrumented | `200`, the value |
| known, not instrumented | `501 not_implemented`, naming the key |
| not a key mark8ly knows | `501 not_implemented`, naming the key |

The last two share a status deliberately — the console cannot act differently on them, and distinguishing them would leak our roadmap into an error code. They differ in the message, which is for a human reading logs.

## v1 keys

All counters. Money keys are declared but uninstrumented (see below).

```json
{ "data": {
    "tenants_active": 4,
    "stores_active": 5,
    "onboarding_in_flight": 0,
    "trials_expiring": 2
} }
```

| key | definition | source |
|---|---|---|
| `tenants_active` | `tenants.status = 'active'` | platform-api |
| `stores_active` | `stores.status = 'active'` | platform-api |
| `onboarding_in_flight` | not completed, idle **less than 24h** — #283's exact definition | platform-api onboarding funnel |
| `trials_expiring` | `subscriptions.status = 'trialing'` and `current_period_end` within **7 days** | marketplace-api |

**`onboarding_in_flight` reuses #283's funnel rather than recomputing.** One definition, one implementation — otherwise the KPI tile and the funnel screen can show different numbers for the same word, which is the same class of bug as counters disagreeing with rows inside #283.

**`trials_expiring` is per store, not per tenant.** `subscriptions.store_id` is unique; a tenant with three stores can have three trials. The key counts subscriptions, and its name says `trials`, not `tenants_on_trial`.

### The 7-day horizon is shared, not local

**#285** (`GET /admin/billing/trials — expiring, with dunning state`) will need its own notion of "expiring". If it picks a different horizon, the console shows two different numbers for the same word on two screens.

The horizon lives in one exported constant in the subscription package. #285 must reuse it rather than declare its own.

## Money: pinned now, instrumented later

`gmv_today` and `gmv_month` are **declared in the registry and marked uninstrumented**. Requesting either returns `501`.

That is not a placeholder — it is the honest answer. GMV across the estate is not computable today:

- Currency is per store (`stores.currency_code`) and per order (`orders.currency_code`), so a cross-tenant sum spans however many currencies the estate uses.
- **There is no FX rate source anywhere in the workspace.** Summing INR and GBP into one number would be arithmetic on incompatible units.

Returning `0`, or omitting the key, would both be worse than `501` — the first is a lie and the second is ambiguous.

**The money contract is still pinned here**, so the first implementation cannot invent a third convention:

```json
{ "gmv_today": { "amount": 984200, "currency": "INR" } }
```

Integer **minor units**, explicit currency, never a bare number.

Note for whoever instruments it: `orders.grand_total` is `numeric(12,2)` — **major** units. Conversion is required, and multiplying by 100 is correct only for 2-decimal currencies. Read the currency's exponent rather than assuming.

## API

`GET /admin/kpis` — all instrumented keys.

`GET /admin/kpis?keys=tenants_active,gmv_today` — the named subset. If **any** named key is uninstrumented or unknown, the response is `501` naming that key. A caller asking a definite question gets a definite answer rather than a quietly shorter object.

```json
{ "error": "not_implemented",
  "message": "kpi \"gmv_today\" is not instrumented",
  "key": "gmv_today" }
```

The `key` field is machine-readable so the console can mark exactly one tile rather than the whole panel.

With no `keys` parameter, uninstrumented keys are simply absent — the console's registry knows what it expected, and `?keys=` is how it turns absence into a definite answer. This is the one place absence is acceptable, because the caller did not ask about a specific key.

## Failure is not zero

If platform-api is unreachable, `GET /admin/kpis` returns **`503`** for the whole request. It does **not** return the subset it managed to reach.

A partial object is indistinguishable from a complete one, so a missing counter would read as absent-therefore-uninstrumented, or worse be rendered as `0`. That is precisely the em-dashes failure. One unreachable dependency makes the whole answer untrustworthy, and the endpoint says so.

A non-404 4xx from platform-api becomes `500` — that means our configuration is broken, and retrying never helps.

## Architecture

Same path as the three endpoints before it.

| Layer | Responsibility |
|---|---|
| `platform-api` repository + internal endpoint | `GET /internal/estate/counts` → `{tenants_active, stores_active}`, on the **strict** auth group (estate-wide data, so an unconfigured deploy must refuse) |
| `marketplace-api` client | A new `estatecounts` client, modelled on `tenantdirectory` |
| `marketplace-api` subscription repository | The `trials_expiring` count |
| `marketplace-api` handler | The registry, the composition, the wire shape |

The handler composes three sources: the new estate-counts client, the existing `onboardingfunnel` client (for `in_flight`), and its own subscription repository.

## Testing

- Registry-driven: every key in the registry either returns a value or `501`; no key can be silently dropped by adding one to the registry and forgetting the handler.
- An unknown key and a known-uninstrumented key both `501`, and the response names the key.
- `?keys=` with a mix of instrumented and uninstrumented returns `501`, not a partial `200`.
- **`onboarding_in_flight` equals what the funnel endpoint reports** for the same instant — the anti-drift assertion.
- An unreachable platform-api yields `503` and **never** a partial object. Assert on the raw body that no counter keys are present.
- Every instrumented value is an integer, never a float or a string.
- Golden fixture proven by mutation against a field rename and a field addition.

## Out of scope

- Instrumenting GMV. Needs a reporting currency or an FX source; neither exists. The registry entry and the money contract are the deliverable here.
- Caching. The counters are cheap and the tile is not hot. Adding a cache would introduce a staleness question nobody has asked.
- Per-tenant KPIs. This is the estate-wide product tile.
