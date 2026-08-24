# Design — `GET /admin/health` (#289)

**Status:** approved, not yet implemented
**Issue:** #289 · **Umbrella:** #260 · **Date:** 2026-08-24

## What this endpoint answers

*"Is this product working?"* — as distinct from `/health` (is the process alive) and
`/ready` (can it serve). Those two are correctly scoped and do not change.

It is also not a substitute for cluster telemetry. Prometheus already covers CPU,
memory and pod health. This reports only what the application itself can see.

## Where it mounts

`GET /admin/health` on the `platformadmin` group → **`/api/v1/platform/admin/health`**.

A read: it requires the HMAC signature and replay defence like every route on this
surface, but per the enforcement matrix it needs no operator identity and no
capability.

**The mount is mode-gated, and this matters to the semantics.** `platformadmin`
registers only under `MODE=admin` (production) and `MODE=both` (local). The outbox
publisher is gated on *exactly the same* modes (`main.go`, "Outbox publisher — runs in
admin and both modes"). So the process answering this endpoint is always a process
that runs the workers being reported on. The production admin Deployment is
`replicas: 1`, pinned deliberately for the in-memory rate limiter — so "this process"
and "the estate" currently coincide.

That coincidence is a fact about today's manifest, not a guarantee. Every measurement
in the instrumented set is therefore read **from the database**, not from in-process
counters, so the answer stays correct if the replica pin is ever lifted.

## Response

Envelope `{"data": {...}}` with **no `pagination`** — this is not a list. Same shape
decision as `/admin/kpis`.

```json
{
  "data": {
    "checked_at": "2026-08-24T09:31:04Z",
    "dependencies": [
      {
        "name": "outbox",
        "status": "ok",
        "metrics": { "pending": 0, "oldest_pending_age_seconds": 0, "errored": 0 }
      },
      {
        "name": "csv_import_jobs",
        "status": "degraded",
        "metrics": { "queued": 3, "running_stale_heartbeat": 1 }
      },
      {
        "name": "campaign_sends",
        "status": "ok",
        "metrics": { "sending": 0, "sending_stale_heartbeat": 0 }
      },
      {
        "name": "stripe_webhooks",
        "status": "ok",
        "metrics": { "unprocessed": 0, "oldest_unprocessed_age_seconds": 0,
                     "manual_review_required": 0 }
      },
      { "name": "scheduled_jobs",  "status": "not_instrumented" },
      { "name": "platform_api",    "status": "not_instrumented" },
      { "name": "stripe_api",      "status": "not_instrumented" },
      { "name": "email_delivery",  "status": "not_instrumented" },
      { "name": "object_storage",  "status": "not_instrumented" }
    ]
  }
}
```

- `dependencies` is an **array in registry order**, allocated with
  `make([]dependencyRow, 0, len(DependencyRegistry))` — a nil slice marshals to
  `null` and defeats the caller's `?? []`.
- `checked_at` and any timestamp are RFC3339 UTC with offset.
- A `not_instrumented` entry carries **no `metrics` key at all**. Not `{}`, not zeroes.

### Statuses

| status | meaning |
|---|---|
| `ok` | measured, and within threshold |
| `degraded` | measured, and outside threshold |
| `unknown` | the check ran and **failed** — the query errored |
| `not_instrumented` | mark8ly cannot measure this today |

`unknown` exists so a failed query can never be rendered as `ok`. It is the same rule
the platform-api clients enforce with `ErrUnavailable`: an error and an empty result
must never collapse into each other.

`not_instrumented` is a separate value from `unknown` on purpose. "We did not look"
and "we looked and the lookup broke" are different facts about the system.

## The registry

A `DependencyRegistry` names every dependency mark8ly knows about, each with an
`Instrumented bool`, and drives the payload — the handler does not decide membership
with conditionals. This copies `KPIRegistry` (#282) for the same reason: a dependency
must not be able to fall silently out of the response.

One deliberate divergence from `/admin/kpis`. Kpis answers `501` for an uninstrumented
key because the caller named that key. Health takes no `keys` parameter and always
reports the whole set, so uninstrumented dependencies appear **inline** as entries
with `status: "not_instrumented"`. They are never omitted and never `"ok"`.

### Instrumented (4)

Each is backed by a table that records work the system actually did.

| name | source | metrics |
|---|---|---|
| `outbox` | `outbox_events` | `pending` (`published_at IS NULL`), `oldest_pending_age_seconds`, `errored` (`error IS NOT NULL`) |
| `csv_import_jobs` | `csv_import_jobs` | `queued` (`status='queued'`), `running_stale_heartbeat` (`status='running'` and `heartbeat_at` older than the orphan window) |
| `campaign_sends` | `campaigns` | `sending` (`status='sending'`), `sending_stale_heartbeat` (`heartbeat_at` older than `campaign.StaleDuration`) |
| `stripe_webhooks` | `stripe_webhook_events` | `unprocessed` (`processed_at IS NULL`), `oldest_unprocessed_age_seconds`, `manual_review_required` |

### Not instrumented (5)

| name | why not |
|---|---|
| `scheduled_jobs` | The scheduled crons and workers constructed in `main.go` — the trial, dunning, lifecycle, loyalty, audit-prune, signup-anomaly, downgrade-recheck, dispatch-orphan and revalidation jobs, plus the nonce sweep — persist **no last-run anywhere**. Established by searching `internal/` for `last_run`/`heartbeat`: the *only* two subsystems that persist one are `csv_import_jobs` and `campaigns`, which is precisely why those two are instrumented above and these are not. Reporting these `ok` would assert a liveness nothing in the system records. |
| `platform_api` | The three clients (`tenantdirectory`, `onboardingfunnel`, `estatecounts`) are exercised only when another endpoint calls them. Nothing records the outcome, and probing them here is out of scope (see below). |
| `stripe_api` | **Outbound** calls. Distinct from `stripe_webhooks`, which is inbound: receiving webhooks normally proves nothing about whether our own API calls are succeeding. No outcome log exists. |
| `email_delivery` | No delivery-outcome table. The dunning and trial mailers log locally and move on. |
| `object_storage` | GCS. Same — no persisted outcome. |

**Configuration presence is not health.** `STRIPE_BILLING_SECRET_KEY` or `GCS_BUCKET`
being set says only that a deploy was configured. Deriving `ok` from a non-empty env
var would be a fabricated status, and is explicitly not done.

## Thresholds

Thresholds are policy. They are defined in **one place** in the handler package, named
in this spec, and the raw measurement ships alongside the status so the console can
re-derive rather than trust.

| dependency | `degraded` when | anchor |
|---|---|---|
| `csv_import_jobs` | a `running` job's `heartbeat_at` is older than **15 min** | **Not a chosen number**, but not yet a shared one either: `main.go` passes a bare `15*time.Minute` literal to `RecoverOrphanedJobs`, and `csvjob` exports no constant. Implementation extracts `csvjob.OrphanWindow = 15 * time.Minute`, replaces the literal at that call site, and reads it here — so the endpoint and the recovery scan cannot drift into disagreeing about the same job. Until that extraction lands, the shared definition does not exist. |
| `campaign_sends` | a `sending` campaign's `heartbeat_at` is older than **15 min** | **Not a chosen number.** `campaign.StaleDuration = 15 * time.Minute` is an exported constant already governing `RecoverStuckCampaigns`. Reused for the same reason as the csv window. |
| `outbox` | oldest pending older than **5 min**, or any `errored` row | Chosen. The publisher ticks every 2s with a batch of 100, so 5 min is ~150 ticks of headroom — comfortably past transient lag, well short of a stall going unnoticed. |
| `stripe_webhooks` | any `manual_review_required`, or oldest unprocessed older than **15 min** | Chosen. `manual_review_required` is already the system's own "a human must look" flag, so any non-zero value is degraded by that table's own definition. |

If the reused csv constant is ever changed, both call sites change together — that is
the point of reusing it rather than copying `15 * time.Minute` into the handler.

## Error handling — deliberately the inverse of `/admin/kpis`

Kpis aborts the entire request with `503` when any upstream fails, because a partial
KPI set is a lie about the estate.

Health does the opposite, because **acceptance criterion 2 requires that a degraded
dependency does not make the endpoint itself fail**. A check whose query errors yields
`200` with that one dependency marked `unknown`; every other dependency still reports.

The error text is **logged server-side and never echoed to the caller** — the same
discipline `/ready` already applies, so DSN fragments and driver error text do not
leave the process.

The endpoint itself fails only for surface-level reasons, all inherited from the
group and none specific to this handler: a bad or missing signature, a replayed nonce,
or `503 not_configured` when the platform-admin secret is unset.

## Wiring

`platformadmin.Register` is called at **two sites** in `cmd/marketplace-api/main.go` —
the `mode.Both` engine and the `mode.Admin` engine. Any new `Deps` field must be
populated at both, or the two deployments differ silently. This is the exact shape of
follow-up **#323**, so it is a task in the plan with its own test, not a footnote.

This handler needs only `DB` and `Logger`, both already on `Deps`. If implementation
finds it needs no new field, the equivalence test is still written — it is the cheap
part, and #323 records five instances of this failing.

## Testing

Aimed at the specific failure modes this milestone has actually produced.

- **Golden fixture**, proved by mutation to fail on a field **rename** *and* a field
  **addition**. A fixture that only catches omissions is theatre (see #276).
- **Boundary fixtures on the exact instant.** For each threshold, seed a row at
  precisely the boundary — not near it. Trap 6 cost seven instances of fixtures that
  sat beside the property instead of on it. "Close to the edge" is not the edge.
- **Distinct non-zero values per stub.** A payload assembled by lookup returns the
  zero value for a missing key, so presence-assertions pass on a fabricated `0`.
  Assert values, not keys.
- **A test that fails if `not_instrumented` entries are dropped** from the payload —
  the registry's whole purpose is that a dependency cannot silently vanish.
- **A test that fails if a failed check renders as `ok`** rather than `unknown`.
- **Both-`Register`-sites equivalence test** (#323).
- Integration runs use `-p 1` (trap 5) and the LAN IP DSN, not `localhost`.

## Out of scope, and what it would take

**Active probing of platform-api** (rejected for this endpoint). It would buy a
genuinely exercised dependency, but puts a live network call with its own timeout
budget on a path whose acceptance criteria require it not to fail — and "reachable
right now from this pod" is closer to what Prometheus already covers than to what this
endpoint is for.

**Cron heartbeat persistence** — the real gap. Making `scheduled_jobs` individually
reportable needs a `cron_runs` table written by every one of those call sites. That is the honest fix
and it is not slim; **file as a follow-up issue** off #289 rather than folding it in.
Until it exists, `scheduled_jobs` reports `not_instrumented`, which is the truth.

## Verification after deploy

Per the milestone's discipline, separate checks that carry information from checks
that merely mean "no data reached this code":

- **Data-independent, carries information:** the route answers under signature; an
  unsigned request is rejected; the payload contains all nine registry entries; the
  five uninstrumented entries carry no `metrics` key.
- **Data-dependent, proves less:** production has 4 tenants and 4 stores, no merchant
  has entered the billing flow, and `store_subscriptions` is empty. Expect the three
  instrumented dependencies to report `ok` with zero counts. **A zero here proves the
  query ran, not that it would ever return non-zero** — the #282 defect shipped
  "verified" on exactly that reading. The boundary fixtures, not the production
  response, are what establish the counters can move.

Deploys are Kargo-gated (CI → ghcr → Warehouse → Freight → Promotion → rollout);
expect 10–20 minutes from freight appearing.
