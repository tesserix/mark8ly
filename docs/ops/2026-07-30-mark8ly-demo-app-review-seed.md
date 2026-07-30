# Mark8ly Demo store — App Review demo-data seed — 2026-07-30

Database: `mark8ly_marketplace_api` on `mark8ly-postgres-2` (namespace `mark8ly`).

| | |
|---|---|
| Store | `815ee993-37cc-4c4e-a49e-46000a23f508` — "Mark8ly Demo", slug `mark8ly-demo`, AU/AUD, active |
| Tenant | `fb5ceef7-335e-4ebd-96e4-f75ded263c86` |
| Reviewer account | `appreview@mark8ly.com`, GIP uid `BhU8cLt3LWaHGEbu79jP4d7j3vG2` |

These are the credentials given to Apple App Review and Google Play for
mobile-admin 1.0.0 (iOS build 10 / Android versionCode 8, commit `08ae4167`).
Deliberately **not** the Bondi store, to protect its fixtures.

## Why

The store existed but was almost empty, while the store-listing screenshots
uploaded for both platforms show a populated store (dashboard revenue + chart,
a 7-order inbox, an order detail with a customer, a full catalogue). A reviewer
would have seen empty states on every business screen — App Store guideline
2.3.3 (screenshots must reflect the app), plus a plausible 2.1 "appears
non-functional".

## Access was already sound — checked first, nothing to fix

The worse failure mode (the recorded "Couldn't load your store" dead-end, whose
Retry can never succeed) does **not** apply here:

- store row exists and is `active`;
- GIP custom claim is present and correctly array-shaped:
  `{"tenant_id":["fb5ceef7-…"]}` — the shape the verifier expects after the
  `4f2fbf67` fix. A scalar here 401s every request;
- OpenFGA tuple `tenant:fb5ceef7-… owner user:BhU8cLt3LWaHGEbu79jP4d7j3vG2`
  exists, so `RequireTenantRelation` passes;
- account is enabled, password provider, and signed in successfully
  2026-07-30 12:05 local.

## State before

```
products 3 (with real CDN photos)   categories 1 (named "54084")
orders 0   customer_profiles 0   reviews 0   coupons 0   gift_cards 0
campaigns 0   customer_segments 0
```

Dashboard therefore returned `$0` for today/week/month, a flat 7-day trend, no
top products, no low stock, and an empty NEEDS YOU queue.

## What was seeded

**8 customer profiles** — realistic AU names/addresses/phones, `created_at`
spread over ~5 weeks so "new this week" reads 2 rather than 8.

**8 orders, created through the real admin API** (`POST
/api/v1/admin/stores/:storeId/orders`), not hand-written SQL, so per-store
numbering, line items, addresses and totals all come from the production code
path. Statuses advanced with the real `/confirm` and `/fulfill` endpoints.
Idempotency keys `seed-demo-20260730-01` … `-08`, so a re-run is a no-op.

| Order | Customer | Status / payment | Total | Placed |
|---|---|---|---|---|
| M-MAR-260719-00001 | Ava Whitfield | fulfilled / paid | 154.85 | 19 Jul |
| M-MAR-260721-00051 | Marcus Delaney | fulfilled / paid | 108.94 | 21 Jul |
| M-MAR-260723-00052 | Priya Raman | fulfilled / paid | 162.84 | 23 Jul |
| M-MAR-260726-00053 | Joel Nakamura | confirmed / pending | 181.80 | 26 Jul |
| M-MAR-260727-00002 | Sienna Blackwood | confirmed / pending | 90.80 | 27 Jul |
| M-MAR-260728-00054 | Tomas Vidal | confirmed / paid | 108.94 | 28 Jul |
| M-MAR-260729-00055 | Hannah Ellery | pending / pending | 154.85 | 29 Jul |
| M-MAR-260730-00056 | Declan Moore | pending / pending | 135.89 | today |

Two left **pending** on purpose: the dashboard NEEDS YOU queue is fed by pending
orders, and the "Confirm order" sticky bar only appears on a pending order — the
exact screen in frame 3 of both screenshot sets.

`created_at`/`placed_at`/`updated_at` were backdated in SQL afterwards (the API
stamps `now()`), which is what gives the 7-day chart its shape and a non-zero
month. The date embedded in `order_number` was rewritten to match, so the number
and the placed date agree. Note `now()` is UTC here — today's order uses a
relative offset because a fixed clock time would have landed in the future.

**Categories** — the junk category literally named `54084` renamed to
`Homeware`; added `Bags & Luggage` and `Audio`; all three products filed via
`product_categories` + `primary_category_id`.

**Stock** — mug 7 (threshold 12) and backpack 4 (threshold 6) are genuinely
low so the queue has low-stock rows; headphones left healthy at 38 so the
catalogue does not look abandoned. Low stock is
`inventory_quantity <= COALESCE(low_stock_threshold, 10) AND > 0`.

**5 reviews** — 4 approved + 1 pending (the pending one gives the review queue a
row), all `verified_purchase`, each linked to the matching `customer_profile_id`.

## Pre-existing defect found and fixed: variants were soft-deleted

All three `product_variants` had `deleted_at` set (2026-07-25 11:06–11:12,
minutes after creation) while their parent products stayed live — so each
product had zero live variants.

The admin product read path does **not** filter `deleted_at` on variants, so the
product list and detail happily returned price, stock, media and category, which
is why this was invisible from the app. The dashboard's low-stock query **does**
filter it, which is what exposed it: two variants under threshold produced zero
low-stock rows. Cleared `deleted_at` on all three — this aligns the rows with
what the API already reports, and makes the products genuinely live rather than
live-looking.

Not systemic: Bondi's 54 soft-deleted variants belong to products archived by
the 2026-07-29 catalogue cleanup, and its live products all have live variants.

## Verified after seeding (via the admin API, not read off the DB)

```
revenue today 135.89 | week 672.28 | month 1098.91 | change +57.6%
trend [0, 0, 181.8, 90.8, 108.94, 154.85, 135.89]
orders pending 2 / fulfilled 3 / cancelled 0
customers 8 (2 new this week) | pending reviews 1
recent orders 5, all with images
top products all 3 (387.00 / 359.96 / 171.50)
low stock 2 — backpack 4/6, mug 7/12
customers list: 8, total_spent populated (so the order↔customer link took)
reviews list: 5, one pending
```

The mobile app calls `/api/v1/mobile/admin/...`, which is GIP-bearer-authed;
verification above used the internal-auth `/api/v1/admin/...` path. Per
`mobile_routes.go:28` these are the **same handlers, same authz, different
auth**, so the payloads are the same — but the mobile path itself was not
exercised here (it needs the account password).

## Notes, not fixed

- The order-detail DTO does not expose `image_url` on line items even though
  `order_items.image_url` is populated, so order line items render without
  thumbnails. Pre-existing, affects Bondi identically, and a DTO change cannot
  reach an already-submitted build.
- The mobile dashboard does not render the setup checklist at all, so the
  store's incomplete setup (3/9) is invisible to the reviewer. No branding,
  payment or shipping config was seeded.

## Rollback

Order timestamps/numbers as they were before backdating are in
`2026-07-30-mark8ly-demo-app-review-seed-orders-rollback.csv`.

```sql
-- remove everything seeded here (orders cascade to items/addresses)
DELETE FROM reviews  WHERE store_id = '815ee993-37cc-4c4e-a49e-46000a23f508';
DELETE FROM orders   WHERE store_id = '815ee993-37cc-4c4e-a49e-46000a23f508'
                       AND idempotency_key LIKE 'seed-demo-20260730-%';
DELETE FROM customer_profiles
      WHERE store_id = '815ee993-37cc-4c4e-a49e-46000a23f508'
        AND email LIKE '%@example.com';

-- categories
DELETE FROM product_categories WHERE category_id IN (
  SELECT id FROM categories WHERE store_id = '815ee993-37cc-4c4e-a49e-46000a23f508'
    AND slug IN ('bags-luggage','audio','homeware'));
UPDATE products SET primary_category_id = NULL
 WHERE store_id = '815ee993-37cc-4c4e-a49e-46000a23f508';
DELETE FROM categories WHERE store_id = '815ee993-37cc-4c4e-a49e-46000a23f508'
   AND slug IN ('bags-luggage','audio');
UPDATE categories SET name = '54084', slug = '54084', description = NULL, position = 0
 WHERE store_id = '815ee993-37cc-4c4e-a49e-46000a23f508' AND slug = 'homeware';

-- stock
UPDATE product_variants SET inventory_quantity = 60, low_stock_threshold = NULL
 WHERE sku = 'ceramic-coffee-mug-default'           AND store_id = '815ee993-37cc-4c4e-a49e-46000a23f508';
UPDATE product_variants SET inventory_quantity = 25, low_stock_threshold = NULL
 WHERE sku = 'classic-leather-backpack-default'     AND store_id = '815ee993-37cc-4c4e-a49e-46000a23f508';
UPDATE product_variants SET inventory_quantity = 40, low_stock_threshold = NULL
 WHERE sku = 'wireless-over-ear-headphones-default' AND store_id = '815ee993-37cc-4c4e-a49e-46000a23f508';

-- variant soft-delete (restoring this re-breaks low stock; almost certainly
-- NOT what you want — it was a defect, not a setting)
UPDATE product_variants SET deleted_at = '2026-07-25 11:12:22.061491+00' WHERE id = '75620484-1a18-46cd-bb0b-60712e8be27a';
UPDATE product_variants SET deleted_at = '2026-07-25 11:06:08.606573+00' WHERE id = '1d5c2906-d94b-4fdb-9f5b-4839d7b11603';
UPDATE product_variants SET deleted_at = '2026-07-25 11:08:41.951841+00' WHERE id = 'f9be7f1d-587d-48c1-a567-0b618230ee9c';
```
