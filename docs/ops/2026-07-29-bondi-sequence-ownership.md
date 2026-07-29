# Bondi Store — order/return sequence ownership fix — 2026-07-29

Database: `mark8ly_marketplace_api` on `mark8ly-postgres-2`.
Store `8b69eea9-2537-4d36-9d99-bafcbad02dbc` (The Bondi Store),
tenant `8c302556-b647-4824-8ce4-73f547ca456e`, owner `demo@mark8ly.com`.

## Symptom

Creating an order on the Bondi store failed with a 500 from every path:

```
order: nextval mk_seq_order_8b69eea9_2537_4d36_9d99_bafcbad02dbc:
ERROR: permission denied for sequence ... (SQLSTATE 42501)
```

Raised at `internal/order/number.go:49`, surfaced by
`internal/handlers/admin/errors.go:91`.

## Cause

Per-store sequences are created by the eager-creation trigger added in
migration `000004_orders_seq_eager.up.sql`. That migration contains no
`GRANT` and the trigger function is not `SECURITY DEFINER`, so each
sequence is owned by whichever role created it.

15 of the 16 `mk_seq_order_*` sequences were owned by `marketplace_api`
— the role the API connects as. The Bondi pair was owned by `postgres`:

```
mk_seq_order_8b69eea9_… | postgres | {postgres=rwU/postgres,mark8ly_platform_admin=wU/postgres}
mk_seq_return_8b69eea9_… | postgres | {postgres=rwU/postgres,mark8ly_platform_admin=wU/postgres}
```

`marketplace_api` held no grant on either, so `nextval()` was denied.
The store was hand-seeded from a `postgres` session rather than created
through the app, so its sequences inherited that ownership.

This is the same class as the `break_glass_lockouts` ownership bug that
wedged the Phase 5 tenant purge: an object created by a manual `postgres`
session that the runtime role then cannot touch.

## What changed

```sql
ALTER SEQUENCE mk_seq_order_8b69eea9_2537_4d36_9d99_bafcbad02dbc  OWNER TO marketplace_api;
ALTER SEQUENCE mk_seq_return_8b69eea9_2537_4d36_9d99_bafcbad02dbc OWNER TO marketplace_api;
```

Both now match the other 15 stores exactly. After the change,
`public` holds zero `postgres`-owned sequences.

The return sequence had the identical defect and would have 500'd
customer returns (`POST /storefront/.../orders/:id/returns`) the same
way; fixed at the same time rather than left as a known landmine.

## Rollback

```sql
ALTER SEQUENCE mk_seq_order_8b69eea9_2537_4d36_9d99_bafcbad02dbc  OWNER TO postgres;
ALTER SEQUENCE mk_seq_return_8b69eea9_2537_4d36_9d99_bafcbad02dbc OWNER TO postgres;
```

Rolling back restores the 500.

## Deliberately NOT changed

`break_glass_lockouts` is still `postgres`-owned. It is the only
remaining `postgres`-owned object in `public`. The tenant purge already
handles it by exclusion, so changing ownership here without re-checking
that fix would be a change with no owner.

## Seeded order

With the grant in place, one order was created through the real service
layer (`POST /api/v1/admin/stores/:storeId/orders`, in-cluster) so the
mobile admin's Confirm flow could finally be exercised — no pending
order had existed for five sessions.

```
M-THE-260729-00001 | pending | payment pending | AUD 149
id 7893e9d8-ae50-4ce1-a2f6-7add1ed4c77c
1x Bondi Linen Beach Shirt (TBS-BLBS-M)
```

Left pending deliberately as a standing fixture. To remove it later,
cancel it through the admin rather than deleting rows.

Note: the real code path numbers it `M-THE-260729-00001`, whereas the
hand-seeded demo orders are `BND-1001`–`BND-1006`. The `THE` comes from
the store-prefix derivation taking the first word of "The Bondi Store".
Cosmetic, but the demo store now shows two different numbering schemes.

## Still broken — not fixed here

Storefront checkout on this store still 500s before it ever reaches the
sequence:

```
checkout_ext: unhandled error
err="shipping calculation failed: no active carrier config: record not found"
```

`internal/handlers/storefront/checkout_ext.go:828`. The store also has
no payment methods (`/payment-methods` → `{"data":[]}`). Both need real
carrier-aggregator and payment-gateway credentials, which is why this
was left. Any customer reaching checkout on the demo store gets a hard
500 — and today's catalogue cleanup fixed stock, so they can now get
that far.
