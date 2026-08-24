# The Bondi Store catalogue cleanup — 2026-07-29

Demo tenant `8c302556-b647-4824-8ce4-73f547ca456e` (The Bondi Store,
owner `demo@mark8ly.com`) had 162 live products. Only 12 were the curated
coastal catalogue. The other 150 were leftover generic seed data
(tablets, action cameras, boxing gloves), each triplicated, and two of
them were `active` and showing on the public storefront homepage.

This matters because the demo tenant is what prospects are pointed at.

## What changed

Database: `mark8ly_marketplace_api` on `mark8ly-postgres-2`.

1. **Archived + soft-deleted 150 off-brand products.** Set
   `status='archived'`, then `deleted_at=now()`. 2 were `active`
   (Indoor Basketball, Insulated Sport Bottle 1L) and publicly visible;
   148 were `draft`.
2. **Renamed** `Bondi Linen Beach Shirt 2` back to
   `Bondi Linen Beach Shirt` (id `972a8469-1641-4f82-8b9d-2434e465e150`).
   The `2` suffix came from re-running the catalogue seed after the
   original was soft-deleted on 2026-05-04.
3. **Set stock on all 41 live variants.** Every variant was
   `inventory_quantity = 0` with `inventory_policy = 'deny'`, meaning the
   entire shop refused add-to-cart. Set to a spread of 8 to 45 per
   variant via `8 + (abs(hashtext(sku)) % 38)`, mirrored into
   `variant_stock.quantity`.

Verified safe first: no `order_items` referenced any archived product, and
no test, fixture, or seed in the repo references the sports catalogue.

Note: the stock UPDATE filtered on `status='active'` but not
`deleted_at IS NULL`, so it also set quantities on 12 variants belonging
to previously soft-deleted products. Those rows are invisible to every
view; no action needed.

## Rollback

`2026-07-29-bondi-catalogue-cleanup-rollback.csv` holds `id,status` for
all 150 products as they were before the change. To restore:

```sql
UPDATE products SET status = :status, deleted_at = NULL
WHERE id = :id;
```

Stock is not covered by the rollback file: every affected variant was
`0` beforehand, so reverting means setting `inventory_quantity` and
`variant_stock.quantity` back to `0`.

## Follow-up

- Product images are associated with the wrong products in the seed
  (a foam roller carried a Nike shoe photo). Mostly moot now that those
  products are gone, but `apps/admin/tests/e2e/seed-product-images.spec.ts`
  looks like the likely source and is worth a look.
- Live SKUs (`BND-01-momxrusl`) do not match the curated seed in
  `docs/marketing/seeds/bondi-product-catalog.sql` (`TBS-BLBS-M`), so the
  live data came from a regenerated seed rather than that file.

---

## Follow-on: customers, reviews and orders (same day)

4. **Deleted personal data.** The tenant's only customer profile was
   `mahesh.sangawar@gmail.com` (no name, from a storefront signup test), and
   two `approved` reviews were signed with that name and address, publicly
   visible on the storefront. One had the body text "Test". All removed.
5. **Added 4 demo customers** on `example.com` (IANA-reserved, undeliverable):
   Ella Whitmore, Priya Raman, Tom Kelleher, Sofia Marchetti, with staggered
   join dates and mixed `marketing_opt_in`.
6. **Added 5 reviews** by those customers, linked via `customer_profile_id`,
   ratings 5/5/4/5/4, one featured.
7. **Added 6 orders** (`BND-1001` to `BND-1006`) across the four customers:
   4 fulfilled + paid, 1 confirmed + unfulfilled, 1 cancelled + refunded.
   Each has order items, shipping and billing addresses, a GST tax line,
   order events, and a payment transaction. Totals verified: every
   `subtotal` equals the sum of its line totals and every `grand_total`
   equals `subtotal + shipping_total + tax_total`. Free shipping over
   A$150 is honoured, matching the storefront banner. Inventory was
   decremented for non-cancelled orders, and reviews by customers who
   bought the product were set `verified_purchase = true`.

### Known gaps

- **No payment gateway is configured for any tenant**, and before this change
  there were zero orders platform-wide. A real checkout on the demo store
  cannot complete. The orders above were inserted directly, not placed
  through the checkout flow.
- Because they bypassed the API, **no analytics events were emitted**. The
  dashboard header reads A$312 from the orders table while the Analytics
  panel below it reads A$0 from its own source. Worth reconciling.
- The `variant_stock` sync ran without a tenant filter and touched all 383
  variants in the database. The only other tenant with stock ("Mark8ly Demo",
  3 variants at 60/25/40) ended consistent with `product_variants` and has
  no orders, so impact is nil.
