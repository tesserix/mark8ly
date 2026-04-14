# Tenant / Vendor / Store Architecture

**Date:** 2026-04-14
**Status:** Draft — pending review
**Scope:** Mark8ly admin, storefront, marketplace-api, platform-api

## Goal

Refactor the data model and admin UX from the current flat **Model B**
(store-scoped everything) to **Model C** — a three-level hierarchy that
cleanly supports single-brand merchants, multi-region brands, **and** a
future multi-vendor marketplace without schema rework later.

## Why now

- App is not live with real customers. Migration risk is at its minimum —
  no order history, no active carts, no payouts in flight to protect.
- Settings and orders are the weakest tested surfaces today. A future
  migration would hit exactly those surfaces with real data behind them.
- The copy-product workaround is already in place, which signals the
  merchant need for a shared catalog. Shipping Model C removes the need
  for that workaround.
- Vendor as a concept is already in the schema (`products.vendor_id`
  nullable). The bones exist — we need to finish them.

## Current state (Model B)

- Everything operational is store-scoped: products, categories,
  customers, orders, payments, shipping, tax, coupons, gift cards,
  loyalty, reviews. Each has a `store_id` column.
- Tenant-scoped: team/memberships, subscription, owner email.
- Two stores under the same tenant = two fully independent catalogs,
  customer lists, order books. Sharing requires the copy-product
  feature.
- Admin is per-store via subdomain (`{store-slug}-admin.mark8ly.com`).
- Storefront is per-store (`{store-slug}.mark8ly.com`).

## Proposed model (Model C)

A three-level entity hierarchy:

```
Tenant (platform operator / company)
  └── Vendor (seller; default tenants have 1 "self" vendor)
        └── Product  ← canonical, vendor-owned
              └── ProductListing  ← product × store overlay
                    └── Store (regional storefront)
```

### Why three levels

- **Single-brand merchant:** tenant has 1 self-vendor, 1 store. The
  vendor layer is invisible. Schema grows under the user.
- **Multi-region brand** (e.g. Acme US / Acme EU / Acme IN): 1 vendor,
  N stores, products authored once and listed into each store with
  per-region price / stock / availability.
- **Marketplace operator:** N vendors, each owning their product slice.
  Listings control which vendor products appear in which store.
- **Vendor user:** sees only their own vendor's scope.

The existence of a Vendor layer lets all four cases share the same
schema and UX primitives.

## Entity split

| Entity | Tenant | Vendor | Store | Notes |
|---|---|---|---|---|
| Category taxonomy | ✅ | — | — | Shared across all stores under a tenant |
| Media library | ✅ | ✅ (vendor-owned) | — | Upload once, reuse |
| Product (canonical SKU, title, description, variants, specs) | via vendor | ✅ owner | — | Authored once |
| Product → category assignment | — | ✅ on product | — | Product-level, inherited by listings |
| ProductListing (price, availability, localized copy, store-specific policies) | — | — | ✅ | Product × Store overlay |
| Inventory / stock | — | ✅ or per-(vendor, store) | ✅ | Depends on fulfillment model (see below) |
| Orders | — | (split lines per vendor) | ✅ primary | Orders are store-scoped; line items can carry `vendor_id` |
| Customers | — | — | ✅ | Separate account per store (privacy-safe) |
| Cart / Checkout | — | — | ✅ | Always store-scoped |
| Payment gateways (customer-facing) | — | — | ✅ | Per-region (Razorpay/Stripe) |
| Payout destinations | — | ✅ | — | Vendor bank, Stripe Connect, UPI |
| Shipping carriers (service offered at checkout) | — | — | ✅ | Per-region operator decision |
| Warehouses / fulfillment origin | — | ✅ | — | Vendor's physical origin |
| Tax provider config | — | — | ✅ | Per-jurisdiction |
| Tax class (on product) | ✅ (taxonomy) | ✅ on product | — | Product-level attribute |
| Vendor tax identity (GST/VAT) | — | ✅ | — | For invoicing / payouts |
| Coupons, campaigns, loyalty, gift cards | — | — | ✅ | Regional promotions |
| Reviews | — | — | ✅ (on listing) | Per-store listing, not per-product |
| Themes / branding | — | — | ✅ | Pure storefront |
| Custom domains | — | — | ✅ | Per store |
| Locale (country, currency, timezone) | — | — | ✅ | Per store |
| Platform team | ✅ (with optional store/vendor overlay) | — | — | Inherits via FGA |
| Vendor team | — | ✅ | — | Vendors manage their own |
| Subscription / platform billing | ✅ | — | — | Tenant pays the platform |
| Marketplace fees / policies | ✅ | — | — | Operator-defined |
| Audit log | ✅ (write-once) | ✅ (filtered view) | ✅ (filtered view) | |

### Key product/listing split

Fields on `Product` (canonical, authored by vendor):
- SKU, title, description, variants, images, base specs, weight,
  dimensions, tax class, category assignment, vendor owner.

Fields on `ProductListing` (overlay per `(product_id, store_id)`):
- Price, currency (inherited from store), availability (enabled? Y/N),
  inventory, localized title/description (optional overrides),
  store-specific policies (return window override, visibility).

Rule: **every product must live in its vendor's canonical catalog**.
Store-only products are not allowed (keeps model honest; enable/disable
per store via listing).

## Settings UX — three-layer differentiation

Admin must always make scope visible. Three layers work together:

### Layer 1 — Sidebar header switchers

- `SWITCH COMPANY` (tenant switcher) — shown if user has 2+ memberships.
- `SWITCH STORE` — shown if current tenant has 2+ stores.
- `VIEWING AS` (vendor context) — shown only if current tenant has 2+
  vendors. Single-brand merchants never see this.

### Layer 2 — Sidebar nav grouped by scope

Sections carry an eyebrow label declaring their scope:

```
— NAVIGATION —
  Dashboard / Products / Orders / Customers / Marketing

— STORE: {store name} —
  Themes, Domains, Payments (gateways), Shipping (carriers), Tax (provider), Team (store overlay)

— VENDOR: {vendor name} —        (hidden in single-brand)
  Payouts, Warehouses, Tax identity, Vendor team

— PLATFORM: {tenant name} —
  Categories, Media library, Platform team, Subscription, Audit logs
```

Eyebrow always tells user what entity they're editing. Navigating inside
a section keeps that scope.

### Layer 3 — Page scope badge

Every scoped page shows a pill under the title naming its scope
(`Store: India Store`, `Vendor: Acme`, `Platform: Acme Inc`). Safety net
against accidental edits.

### Per-user-type collapse

- **Solo merchant** (1 tenant, 1 self-vendor, 1 store): no switchers,
  no VENDOR eyebrow, flat nav. UX ≈ today.
- **Multi-region brand** (1 tenant, 1 self-vendor, N stores): store
  switcher + `— STORE —` eyebrow. No vendor surface.
- **Marketplace operator** (1 tenant, N vendors, M stores): store
  switcher + "Vendors" top-level nav → drill into a vendor's scope.
  `— VENDOR —` eyebrow becomes visible.
- **Vendor user**: single vendor context, sees only their products,
  payouts, warehouses, and orders for their listings.

### Routing and scope

- Tenant scope → `admin.mark8ly.com` or any store subdomain (global)
- Store scope → `{store-slug}-admin.mark8ly.com/{section}`
- Vendor scope → `{store-slug}-admin.mark8ly.com/vendors/{vendor-id}/{section}`
  (later: vendor-owned users get their own subdomain)

Scope changes are always a full navigation, not in-place state toggles.

## Migration phases

Ordered by blast radius, lowest → highest:

### Phase 1 — Add Vendor table + self-vendor per tenant

- New `vendors` table with `tenant_id`, `name`, `slug`, `status`.
- Auto-create a "self" vendor for every existing and new tenant.
- Make `products.vendor_id` NOT NULL, backfill to the self-vendor.
- Zero user impact. No UI change yet.

### Phase 2 — Add ProductListing + backfill

- New `product_listings` table keyed on `(product_id, store_id)`.
- Backfill: create one listing per existing product in its current
  store. Copy `price`, `stock`, `is_available` from product.
- Dual-write: admin writes to both old and new columns.
- Reads still come from the product columns. No user-visible change.

### Phase 3 — Switch reads to listings

- Storefront, cart, checkout, product pages, search — all read from
  listings.
- Feature-flagged per-store so we can canary one store at a time.
- Orders / cart smoke tests required before flipping the flag.
- **Highest-risk phase.** Dedicated test pass on products → cart →
  checkout → order placement.
- Once stable everywhere, remove old columns.

### Phase 4 — Migrate Categories from store → tenant

- New `categories` table at tenant scope with `tenant_id`.
- De-duplicate across stores in single-tenant case (same category tree
  name = same row).
- Update product assignments to reference tenant-level category IDs.
- Admin UI: move Categories out of per-store settings into Platform
  section.

### Phase 5 — Split Payments (gateway vs payout)

- Split existing payment config into `payment_gateways` (store-scoped,
  customer-facing: Razorpay/Stripe keys, enabled methods) and
  `vendor_payouts` (vendor-scoped: bank/Stripe Connect destination).
- Admin: `/settings/payments` → store scope, new `/settings/payouts` →
  vendor scope.
- Write settings E2E tests as part of this phase.

### Phase 6 — Split Shipping (carriers vs warehouses)

- Split into `shipping_carriers` (store-scoped, customer-facing carrier
  offerings and credentials) and `warehouses` (vendor-scoped, vendor
  fulfillment origin with address).
- Admin: `/settings/shipping` → store scope, new `/settings/warehouses`
  → vendor scope.
- Orders flow updates to resolve a warehouse per line item.

### Phase 7 — Order line vendor scoping + split orders

- Add `vendor_id` to order line items (even before multi-vendor so
  single-vendor works consistently).
- Introduce "sub-order per vendor" split logic for when orders span
  multiple vendors. Defer full payout split until a real vendor lands.

### Phase 8 — Admin UX rollout

- Ship the sidebar scope grouping, page scope badges, and the Vendors
  top-level nav.
- Feature-flagged to the platform tenant first; enable broadly once
  validated.
- Single-brand users see the flat nav via the progressive-disclosure
  collapse rules.

Phases 1–4 land in a single milestone because they form one logical
refactor on the read path. 5–6 land as a second milestone. 7–8 land as
needed.

## Derisking principles

Each principle prevents a specific failure mode:

1. **Backfill before behavior change.** Every schema change lands in
   two deploys: (a) add table + dual-write + backfill, (b) switch
   reads. No broken intermediate state.
2. **Shim old shape at the API boundary.** Admin and storefront APIs
   keep the old response shape (`product.price`, `product.stock`) until
   the read switch. Frontend changes independently.
3. **Feature-flag new admin UIs.** Vendor section, per-store pricing,
   warehouse UI — all flagged. Canary on our own tenant.
4. **Single-brand UX unchanged during migration.** 1 tenant → 1
   self-vendor → 1 listing per product → warehouses default to
   tenant's onboarding address. Admin looks identical throughout.
5. **Feature-flag reads per store** in Phase 3. Canary one store,
   verify orders match, then roll to all.

## Impact by feature area

| Feature | Impact | Risk |
|---|---|---|
| Auth, login, onboarding | None | Safe |
| Themes, domains, store switcher | None | Safe |
| Dashboard, account, notifications, audit logs | None | Safe |
| Products (admin + storefront) | Read path changes | Moderate |
| Cart / Checkout | Reads listings instead of products | Moderate |
| Inventory | Column moves product → listing | Moderate |
| Categories | Scope changes store → tenant | Moderate |
| Coupons, gift cards, loyalty, customers | None | Safe |
| Reviews | Keyed to listing | Moderate |
| Marketing campaigns | None | Safe |
| Settings / Payments | Split into gateway + payout | Significant |
| Settings / Shipping | Split into carriers + warehouses | Significant |
| Settings / Tax | Split across provider + product + vendor | Significant |
| Orders | Adds vendor_id on line items; order splits | Significant |
| Team / roles | Additive (vendor scope on top of tenant) | Moderate |

## Open questions / deferred

- **Defaults + overrides pattern.** Several settings should have
  tenant-level defaults that stores inherit unless overridden (return
  window, default carrier, default tax class). Should be designed in
  from the first settings-split phase rather than retrofitted. Need a
  shared `scope + parent_id + overrides` table convention.
- **Cross-store aggregates.** Product catalog is shared; should
  analytics be cross-store too? Out of scope for this refactor — handled
  in a future reporting layer.
- **Vendor-owned subdomains.** When vendor users log in independently,
  do they get a `{vendor-slug}.vendor.mark8ly.com` or similar? Deferred
  to when the first real marketplace vendor signs up.
- **Shared customers across stores?** Today separate accounts by design
  (GDPR-safe). Revisit if merchants complain about duplicate customer
  records; possible future feature is opt-in customer linking.
- **Inventory at the vendor vs listing level.** Single warehouse
  serving many stores = inventory is vendor-level; per-store warehouses
  = inventory is listing-level. Need merchant input before Phase 6.
- **Marketplace commission policy primitives.** Flat % per vendor, per
  category, per product tier? Design with Phase 7 when we have real
  vendors to validate against.

## Success criteria

- Phase 1 complete: every tenant has exactly one self-vendor, every
  product has a non-null `vendor_id`.
- Phase 3 complete: all reads go through listings; old columns removed;
  zero regressions in product / cart / checkout / orders verified via
  E2E suite.
- Phase 4 complete: single category tree per tenant; admin Categories
  page lives under Platform.
- Phase 5–6 complete: `/settings/payouts` and `/settings/warehouses`
  exist and are populated for the self-vendor; customer-facing pages
  continue to work off `/settings/payments` and `/settings/shipping`.
- Phase 7 complete: every order line carries `vendor_id`; orders
  correctly split when (eventually) a cart includes multiple vendors.
- Phase 8 complete: sidebar shows scoped eyebrows + page badges; solo
  merchants see no added complexity.
- Zero customer-data loss at every phase boundary.
- Each phase is independently revertible via feature flag or migration
  rollback.
