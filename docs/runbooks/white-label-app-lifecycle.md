# Runbook — White-Label App Lifecycle Stuck

**Alert:** `WhiteLabelLifecycleQuietFor48h`
**Severity:** info
**Owner:** platform / marketplace-api

---

## What the alert says

`white_label_app:lifecycle_transitions:rate1h` has been `absent` (no
samples) for 48 consecutive hours. Either:

1. **Benign** — no merchants currently sunsetting their white-label
   app. Zero is the expected steady state during quiet periods.
2. **Stuck cron** — the daily 05:00 UTC advancer has errored on every
   run for 48h.
3. **Stuck external API** — Apple ASC / Google Play / Firebase
   returning errors that the advancer keeps retrying.

## First 5 minutes — rule out the benign case

```bash
# How many sunset_scheduled rows exist right now?
kubectl exec -n mark8ly mark8ly-postgres-1 -c postgres -- \
  psql -U marketplace_user -d mark8ly_marketplace_api -tAc "
    SELECT status, COUNT(*) FROM white_label_app_state
    GROUP BY status;"
```

- If every row is `credentials_purged` (or the table is empty): benign.
  Silence the alert for 7d.
- If there are rows in non-terminal states: continue to §next.

## Check the cron

```bash
# Last invocation of the advancer tick (look for the log line).
kubectl logs -n marketplace deploy/marketplace-api --since=2d \
  | grep -E "P15 white-label lifecycle|lifecycle: advance" \
  | tail -20
```

- **Zero log lines** in 48h: the cron isn't firing. Check
  `cfg.WhiteLabelLifecycleCron` in the running pod's env (default `0 5 * * *`).
- **Log lines with "advance row failed"**: copy the `store_id` and
  jump to §stuck-row.

## Stuck row diagnosis

```bash
# Inspect the due row.
kubectl exec -n mark8ly mark8ly-postgres-1 -c postgres -- \
  psql -U marketplace_user -d mark8ly_marketplace_api -c "
    SELECT store_id, status, scheduled_at, next_action_at, merchant_initiated,
           apple_app_id, google_package, firebase_project_id
    FROM white_label_app_state
    WHERE next_action_at <= now()
    ORDER BY next_action_at
    LIMIT 5;"
```

Match the status to the action:

| Current status | Next action at day | External API called |
|---|---|---|
| `sunset_scheduled` | +7 (banner), +30 (block) | Apple + Google (block downloads) |
| `downloads_blocked` | +60 (pull) | Apple + Google (pull app) |
| `pulled` | +60+1min (archive) | Firebase (archive project) |
| `firebase_archived` | +90 (purge) | Firebase delete + Secret Manager purge |

Cross-reference with the log: find the specific error (Apple 4xx?
Google `ErrNotWired`? Firebase 500?).

- **Google/Firebase `ErrNotWired`** — expected today (P15 T8 stubs);
  advancer logs + swallows. Not actually stuck — the status just stays
  in a state like `downloads_blocked` because Apple succeeded but
  Google is deferred. No action required; alert will auto-resolve when
  the next Apple-driven transition fires.
- **Apple 4xx (401/403)** — credentials rotated/expired. Re-upload via
  `POST /admin/stores/:id/app-credentials/apple` and wait for the next
  tick.
- **Apple 5xx** — transient ASC outage. Retry next tick. Escalate if
  >24h.

## Nuclear option — force advance

**DO NOT** run in production without a ticket. For staging debugging
only:

```sql
-- Pull next_action_at forward by N days so the advancer picks it up
-- immediately. Log the before/after in the ticket.
UPDATE white_label_app_state
SET next_action_at = now()
WHERE store_id = '<store-id>';
```

Then trigger a cron tick via:

```bash
kubectl exec -n marketplace deploy/marketplace-api -- \
  /bin/sh -c 'curl -sf -X POST http://localhost:8087/internal/lifecycle/advance-now'
```

(This endpoint does not exist today — add only if debugging in-prod
stalls becomes a recurring need.)

## Silencing

If confirmed benign (no merchants sunsetting), silence via
Alertmanager for 7 days. Re-check weekly — the silence is not a fix,
just noise reduction.

## Day 60 on Google Play — a manual step, every time, forever

**This is not a stall and force-advancing will not help.** It is the one
teardown step the platform cannot perform, and it needs a person in the
Play Console.

### The signal

```promql
increase(white_label_app_lifecycle_step_skipped_total{surface="google_play",step="pull_app"}[24h]) > 0
```

Each increment is one merchant whose Play listing is still live after the
platform finished its day-60 work. Unlike every other counter here, **this one
rising is the expected steady state, not a fault** — see the note in
`metrics.go`. Alert on the *other* surfaces rising; treat this one as a work
queue.

The matching audit row records the reason rather than asserting a success. It is in
the append-only log (`white_label_app_lifecycle`), which is the table with a `reason`
column — `white_label_app_state` is the mutable current-state row and carries no
failure text:

```sql
SELECT l.tenant_id, l.store_id, l.created_at, l.reason, s.google_package
  FROM white_label_app_lifecycle l
  LEFT JOIN white_label_app_state s ON s.store_id = l.store_id
 WHERE l.status = 'pulled'
   AND l.reason LIKE '%requires a manual Play Console unpublish%'
 ORDER BY l.created_at;
```

The `LIKE` matches the exact suffix `advancer.go` appends when the cause is
`googleplay.ErrUnpublishNotSupported`, so a row that failed for any *other* Play
reason will not be picked up here — that one is a genuine fault and belongs in the
stuck-row diagnosis above.

### Why the platform cannot do it

The Android Publisher v3 API has no unpublish. There is no `unpublished`
release status (`draft`, `inProgress`, `halted`, `completed`,
`statusUnspecified`), `edits.tracks.update` only manages releases *within* a
track, and the `applications` resource has exactly one method, `dataSafety`.
`googleplay.Client.PullApp` returns `ErrUnpublishNotSupported` rather than
claiming a success it did not earn. See tesserix/mark8ly#862.

### The manual step

1. Confirm the merchant is genuinely at day 60 — check `scheduled_at` on
   `white_label_app_state` (the sunset anchor the advancer counts from), not the
   alert alone. Note `merchant_initiated = true` compresses the schedule to 7 days,
   so day 60 arrives sooner for those rows.
2. Play Console -> the merchant's app -> **Advanced settings -> App
   availability -> Unpublish**.
3. Record it against the merchant. There is no automated confirmation, so an
   unrecorded action is indistinguishable from one nobody did.

Apple needs nothing here: `apple.Client.PullApp` removes the app through App
Store Connect and the day-60 transition completes on its own.

### If the counter is zero and you expected work

Neither Play step can run in production yet — `discovery.go` leaves
`GooglePackage` empty because nothing captures a package identifier. Until
that is decided, both the day-30 halt and this day-60 step are unreachable and
the counter stays flat. That is a known gap, not evidence the teardown worked.

## References

- `services/marketplace-api/internal/whitelabel/lifecycle/advancer.go`
- `services/marketplace-api/internal/whitelabel/googleplay/client.go` — `ErrUnpublishNotSupported`
- `services/marketplace-api/internal/whitelabel/metrics/metrics.go` — why the skip counter is expected to rise
- Spec §13.5, including the day-60 correction note
- Migration `000076_white_label_app_state`
