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

## References

- `services/marketplace-api/internal/whitelabel/lifecycle/advancer.go`
- Spec §13.5
- Migration `000076_white_label_app_state`
