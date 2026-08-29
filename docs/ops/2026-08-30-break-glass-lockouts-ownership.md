# break_glass_lockouts ownership fix — 2026-08-30

Database: `mark8ly_marketplace_api`, cluster `mark8ly-postgres` (primary
`mark8ly-postgres-3`). Closes #457.

## Symptom

```
GET /api/v1/platform/admin/break-glass → 500
platform break-glass list failed
err="breakglass platform list: ERROR: permission denied for table
     break_glass_lockouts (SQLSTATE 42501)"
```

Auth was never the problem: the request passed the HMAC check and the
`rotate-credentials` capability gate and reached the handler. Only the
database read failed.

## The reported 500 was one symptom of three

`marketplace_api` held **no privileges at all** on the table — confirmed via
`information_schema.role_table_grants` (only `postgres` and
`mark8ly_platform_admin` appeared) and by `SET ROLE marketplace_api; SELECT …`
returning `permission denied`.

Two more consequences follow, both in the break-glass **login** path, and
neither was reported:

1. **The durable 24h lockout had never been persisted.** `recordFailure` →
   `Repo.LockIP` → `INSERT` → denied. The error is logged at `Warn` and
   swallowed.
2. **An existing lockout would never have been enforced.** The fast-path
   `IsIPLocked` read fails open — the error is logged and `locked` stays
   `false`.

So brute-force protection on break-glass login was, in production, entirely
the in-memory per-pod limiter: it resets on every deploy and does not span
replicas. The "durable decision" layer the code describes had never
functioned since #333 shipped it.

Not unprotected — three strikes per hour per pod still applied, and login
requires password **and** TOTP — but the documented 24h hard lockout did not
exist.

## Cause

Migration `000073_break_glass_lockouts.up.sql` creates the table. Migrations
run as `marketplace_api` (the migrate init container reads
`mark8ly-postgres-marketplace-api`), so the migrating role owns what it
creates. Every sibling table from the same migration run confirms it:

```
break_glass_accounts  -> marketplace_api
enterprise_api_keys   -> marketplace_api
tenant_sso_configs    -> marketplace_api
break_glass_lockouts  -> postgres        ← the anomaly
```

The table was therefore created out-of-band by `postgres` at some point, not
by the migration. **This is a production-only anomaly**: a fresh environment
running `000073` as `marketplace_api` owns the table and never has this bug.

## Why this was not a migration

A role can only `GRANT` on a table it owns. Migrations connect as
`marketplace_api`; the table was owned by `postgres`. A migration carrying
the fix would fail with `permission denied` and block the rollout — and
would be pointless anyway, since no other environment is affected.

Same shape as the 2026-07-29 sequence-ownership fix, and applied the same
way: privileged SQL by hand, written up here.

## Applied

```sql
-- as postgres, on the primary (mark8ly-postgres-3)
ALTER TABLE break_glass_lockouts OWNER TO marketplace_api;
```

Ownership rather than `GRANT SELECT, INSERT` deliberately: it makes
production match every sibling table and match a fresh environment, so the
anomaly stops existing instead of being papered over. The narrower grant
would have left prod permanently different from everywhere else.

## Verified

```
break_glass_accounts -> marketplace_api
break_glass_lockouts -> marketplace_api
SET ROLE marketplace_api; SELECT count(*) FROM break_glass_lockouts;  -- 0, no error
```

End to end, via the admin-conformance suite (which probes this endpoint
because contract v3 declares it):

```
break-glass  (2 passed, 0 failed, 3 skipped)
  PASS  §4.1  responds with the { data, pagination } envelope
  PASS  §4.5  returns 200 with an empty array when there are no rows
```

It returned 500 before this change.

## Rollback

```sql
ALTER TABLE break_glass_lockouts OWNER TO postgres;
```

Rolling back restores the 500 and re-breaks lockout persistence.

## Consequences for the tenant purge

`tenantpurge` excluded this table because `marketplace_api` had no DELETE and
including it aborted the single-tx purge. That reason is now gone, and the
comments in `purge.go` and `purge_test.go` have been corrected — they
asserted a privilege state that is no longer true.

The exclusion **stands**, on different grounds: `tenant_id` is NULLABLE, so
rows with no tenant are IP-level lockouts belonging to nobody and a
tenant-scoped purge must not touch them; and the rows are ephemeral and
self-expiring via `locked_until`.

Re-adding it as `tenantScoped()` is now possible and is a deliberate decision
— it deletes a tenant's lockout rows on purge, defensible under art.17 and a
change to rate-limit state. Filed separately rather than folded in here.

## Not fixed here

The login path's **fail-open on a lockout-read error** is a separate defect:
a security control that silently degrades when its storage is unavailable is
why this went unnoticed for so long. Filed separately.
