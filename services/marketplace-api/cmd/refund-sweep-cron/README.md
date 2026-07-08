# refund-sweep-cron

Re-drives `refund_transactions` rows stuck in `pending` — the never-lost
guarantee for refunds. A row goes stuck when the gateway call succeeded but
the process crashed (or the DB blipped) before the finalize transaction
committed. The sweeper re-calls the gateway with the **same idempotency
key** (a provider no-op if the money already moved) and completes the DB
finalize + bookkeeping via `orderrefund.Coordinator.ResumePending`.

## Deployment

Run as a **Cloud Run Job** (mirroring `reconciliation-cron`) triggered by
**Cloud Scheduler every ~5 minutes**. Not a long-lived service — each
invocation processes the current backlog of stuck rows and exits.

- K8s alternative: a `CronJob` on the same `*/5 * * * *` schedule if the
  environment isn't Cloud Run-based.
- Timeout: the job self-bounds to 2 minutes per run; schedule accordingly
  so overlapping runs stay rare (idempotency keys make overlap safe either
  way).

## Configuration

- `DATABASE_URL` (required) — same Cloud SQL instance as `marketplace-api`.
- The job always constructs its `Coordinator` with `enabled=true`: the
  sweep IS the recovery path for the `REFUND_GATEWAY_ENABLED` kill switch,
  so it must re-drive pending rows regardless of the main API's flag state.

## Exit behavior

Exits non-zero **only** on infrastructure failure (missing `DATABASE_URL`,
DB connection failure, or an unexpected error from `ResumePending`). A run
that resumes zero rows is a normal, successful exit — do not alert on
"nothing to do."

## Relationship to REFUND_GATEWAY_ENABLED

`REFUND_GATEWAY_ENABLED` gates the main API's `orderrefund.Coordinator`
(admin + storefront). Refunds only flow end-to-end when it is `true` on
`marketplace-api`. The sweeper is deploy-independent of that flag — it
should always be running so any refund that got stuck while the flag was
on (or during a flip) gets recovered.
