# stock-hold-sweep-cron

Deletes `stock_holds` rows that expired more than an hour ago (#229/#231).

## It is housekeeping, not correctness

Availability is computed, never stored:

```sql
available = variant_stock.quantity
          - SUM(qty) FILTER (WHERE state = 'held' AND expires_at > now())
```

An expired hold therefore already reduces availability by **nothing**. If this
job never runs again, stock, pricing and checkout all stay correct — only dead
rows accumulate.

That is deliberate. A stored `reserved` counter would have made this sweeper
load-bearing for correctness, and a missed run would silently strand stock.
Read an alert from this job accordingly: it means rows are piling up, not that
anything is wrong with what customers can buy.

## Why expired rows are kept for an hour

They reduce availability by nothing, and they are the evidence of what happened
when someone asks why a cart lost its unit. The grace window is
`sweepGrace` in `internal/stockhold`.

## Environment

| var | required | default | notes |
|---|---|---|---|
| `DATABASE_URL` | yes | — | |
| `STOCK_HOLD_SWEEP_BATCH` | no | 500 | must be a positive integer; a bad value exits non-zero rather than silently falling back |

## Exit codes

Non-zero only on infrastructure failure (no `DATABASE_URL`, bad batch size,
DB unreachable, query error). **A sweep that deletes zero rows is a normal,
successful run** — the usual case once the backlog is drained.

## Concurrency

The delete runs `FOR UPDATE SKIP LOCKED` in bounded batches, so overlapping
runs on multiple replicas neither block each other nor double-delete, and a
large backlog drains over several runs instead of one long-held lock.
