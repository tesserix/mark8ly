# Dev database: orphan `mk_seq_*` sequence sweep

**Date:** 2026-08-29
**Database:** `marketplace_db` on the shared dev Postgres (`dev-postgres-1`)
**Issue:** [#436](https://github.com/tesserix/mark8ly/issues/436)
**Scope:** dev only. Production was never affected — see "Production" below.

## Why

Migration `000004_orders_seq_eager` installs an `AFTER INSERT ON stores`
trigger that creates two Postgres sequences per store,
`mk_seq_order_<id>` and `mk_seq_return_<id>`. Nothing dropped them:

- `pkg/testdb.SeedStore`'s cleanup deleted the `stores` row but issued no
  `DROP SEQUENCE`, and `NewDB`'s `TRUNCATE ... CASCADE` does not reach a
  catalog relation. Every `NewDB`-based integration test leaked two
  sequences per seeded store, permanently. (Tests using `testdb.NewTx` did
  not leak — the DDL rolls back with the fixture.)
- `internal/tenantpurge` had no sequence handling at all, so a purged
  tenant orphaned two catalog objects per store.

Both are fixed in code alongside this sweep. This document records the
one-off cleanup of the residue that had already accumulated.

## Production

Not an incident. `docs/ops/2026-07-29-bondi-sequence-ownership.md` records
16 `mk_seq_order_*` sequences in production as of 2026-07-29 — 16 stores,
32 sequences, proportional to real stores. The runaway count was a
property of the shared dev database, which absorbs every integration run.

## Orphan predicate

A sequence is an orphan iff its embedded uuid matches no live `stores` row.
The name is parsed back to a uuid by stripping the kind prefix and
translating `_` to `-`:

```sql
SELECT c.relname
FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE c.relkind = 'S' AND n.nspname = 'public'
  AND (c.relname LIKE 'mk\_seq\_order\_%' OR c.relname LIKE 'mk\_seq\_return\_%')
  AND NOT EXISTS (
    SELECT 1 FROM stores s
    WHERE s.id::text = translate(regexp_replace(c.relname, '^mk_seq_(order|return)_', ''), '_', '-')
  );
```

The predicate was verified in both directions before anything was dropped,
because "orphan" was about to authorise a `DROP` on a database shared with
other work:

- **Negative half:** `live_store_seqs` = 0 against `stores` = 0 rows, so no
  sequence belonging to a live store was in the drop set.
- **Positive half:** inside a rolled-back transaction, a probe store was
  inserted; the predicate then classified exactly 2 sequences as live and
  excluded them from the orphan set. Without this check a predicate that
  simply matched everything would have looked identical against an empty
  `stores` table.

## Execution

Batched at 2000 drops per transaction. A single transaction was not an
option: `max_locks_per_transaction` is 64 with `max_connections` 100
(~6,400 lock slots), and `DROP SEQUENCE` takes a lock per relation, so
28k drops in one transaction would have exhausted the lock table — the
same limit that made `pg_dump` fail in the first place. Orphan status is
recomputed inside each batch, so a store created concurrently by other
work could never fall into a later batch's drop set.

## Result

| | Before | After |
|---|---|---|
| `mk_seq_order_*` + `mk_seq_return_*` | 27,990 | 0 |
| All sequences in `public` | 27,991 | 1 |
| `stores` rows | 0 | 0 |

Every one of the 27,990 was residue. The one remaining sequence is
unrelated to this trigger and was never in the drop set.

`pg_dump -U dev -d marketplace_db` was run afterwards and succeeded
(0.25s, 214,940 bytes), confirming the lock-exhaustion symptom that
opened #436 is gone.

## Residual leak sources — the fixes above are NOT the whole leak

Measured, not assumed. After the sweep the count was 0; running the
marketplace-api integration suite once put **4,152** back. `testdb.SeedStore`
was one leak path, not the leak.

Nineteen test files run `INSERT INTO stores` directly; only three drop the
sequences afterwards (`internal/order/testhelpers_integration_test.go`,
`internal/order/lifecycle_integration_test.go`, and
`internal/tenantpurge/purge_integration_test.go`). The other sixteen do not
go through `testdb.SeedStore` at all, so fixing that one fixture cannot
reach them:

```
cmd/backfill-email/main_integration_test.go
internal/outbox/publisher_integration_test.go
internal/subscription/repository_crosstenant_integration_test.go
internal/subscription/harddelete/sweep_via_parent_integration_test.go
internal/subscription/lifecycle/winback_integration_test.go
internal/subscription/planchange/planchange_integration_test.go
internal/audit/prune_cron_storeless_integration_test.go
internal/campaign/segment_delete_integration_test.go
internal/orderrefund/resolver_integration_test.go
internal/giftcard/repository_integration_test.go
internal/giftcard/repository_status_integration_test.go
internal/handlers/storefront/webhooks_capture_integration_test.go
internal/handlers/storefront/storefront_integration_test.go
internal/handlers/internalsvc/storefront_status_test.go
internal/billing/trial/expiring_integration_test.go
internal/billing/trial/extend_stripelive_test.go
```

Handler tests that create a store through the HTTP API leak the same way.

Per-fixture drops are the wrong shape for sixteen files and counting: the
next one added leaks again, silently, and nothing fails. The durable fix is
a single teardown that sweeps orphans — the predicate above is exactly it —
run once per package or once per suite, so a fixture cannot opt out by
forgetting. That is a design decision, not a cleanup, and is left for #436's
follow-up rather than folded into this sweep.

Until then this sweep is repeatable: re-run the batched drop above. It is
safe at any time, because orphan status is recomputed inside each batch and
a live store's sequences are never selected.

## Recurrence

Closed on two of the paths, with a mutation-verified regression test on
each:

- `pkg/testdb.SeedStore` now drops both sequences in its cleanup
  (`TestSeedStore_CleanupDropsPerStoreSequences`, which must use `NewDB` —
  under `NewTx` it would pass either way and prove nothing).
- `tenantpurge.Purge` now drops them inside the purge transaction
  (`TestIntegration_Purge_DropsPerStoreSequences`).

The count will still grow from the sixteen fixtures listed above until the
suite-level sweep lands.
