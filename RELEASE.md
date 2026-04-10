# mark8ly — Release Plan

## v1.0 — Launch Release

### Shipped
- [x] Products catalog (list, detail, variants, media, categories, CSV import/export)
- [x] Orders (create, confirm, fulfill, cancel, refund, returns, abandoned carts)
- [x] Payments (Stripe, Razorpay, PayPal + webhooks)
- [x] Shipping (ShipEngine, Delhivery, NinjaVan + rate calculation)
- [x] Tax (flat rate, India GST, US TaxJar)
- [x] Storefront (product browse, search, categories, variant selector, media gallery, pagination)
- [x] Cart + Checkout (add to cart, qty controls, extended checkout with payment/shipping/tax)
- [x] Admin settings (store general, storefront, stores, team, payments, shipping, tax)

### Planned — Marketing (M1–M4)
- [ ] M1: Coupons (CRUD, validate, checkout integration, rate limiting)
- [ ] M2: Gift Cards (issue, balance check, checkout as payment method)
- [ ] M3: Loyalty Program (points, tiers, referrals, expiry worker)
- [ ] M4: Campaigns (email campaigns, segments, send worker, templates)

### Planned — Customers & Reviews (C1–C3)
- [ ] C1: Storefront Auth (GIP mp-customer, session middleware, /account)
- [ ] C2: Customer Profiles (admin list with aggregated stats, detail, block/unblock)
- [ ] C3: Reviews (1-5 stars, photos, helpful, featured, merchant reply, moderation)
- [ ] C4: Wishlists (save products, storefront wishlist page, account tab)

### Planned — Settings (S1–S5)
- [ ] S1: Account & Security (profile, MFA via auth-bff, sessions, delete tenant)
- [ ] S2: Custom Domains (Cloudflare API DNS management, verification worker)
- [ ] S3: Subscription/Billing (Stripe Billing, plan display, portal)
- [ ] S4: Audit Logs (read-only viewer, search, export CSV)
- [ ] S5: Notifications (preferences, notification table, bell dropdown, 30s poll)

### Planned — Dashboard & Support (D1–D3)
- [ ] D1: Dashboard (stat cards, revenue sparkline, setup checklist, recent orders, top products, low stock)
- [ ] D2: Support Tickets (create, list, reply, resolve/close)
- [ ] D3: Help Center (markdown articles, search, contextual links)

### Planned — Branding & Subscriptions (B1–B3)
- [ ] B1: Storefront Branding (logo, full color palette, fonts, layout, footer, announcement bar, live preview)
- [ ] B2: Subscription Tiers (Free/Starter/Pro/Enterprise/Marketplace, feature gating, regional pricing, soft downgrade)
- [ ] B3: React Native Mobile App (native UI, per-merchant builds, Enterprise plan)

### Planned — Production Readiness (P0–P3)
- [ ] P0: Critical Security (secrets cleanup, API key encryption, webhook scoping, security headers, CORS)
- [ ] P1: Observability (Prometheus metrics, Sentry error tracking, logging audit)
- [ ] P2: Performance (Next.js Image optimization, N+1 query audit, bundle monitoring)
- [ ] P3: Dependencies & Tooling (dependency updates, Dependabot, migration runbook, API docs, load testing)

---

## v2.0 — Marketplace Release

### Multi-Vendor Marketplace
- [ ] Vendor onboarding portal
- [ ] Per-vendor dashboard (products, orders, earnings)
- [ ] Configurable commission rates per vendor
- [ ] Auto payout splits (Stripe Connect / Razorpay Route)
- [ ] Multi-vendor cart (single checkout, items from multiple vendors)
- [ ] Vendor ratings/reviews

---

## Subscription Tiers

| | Free | Starter | Pro | Enterprise | Marketplace |
|---|---|---|---|---|---|
| **USD/month** | $0 | $9.99 | $29.99 | $99.99 | $249.99 |
| **INR/month** | ₹0 | ₹499 | ₹1,499 | ₹4,999 | ₹14,999 |
| **Transaction fees** | 0% | 0% | 0% | 0% | 0% |
| **Products** | 25 | 500 | Unlimited | Unlimited | Unlimited |
| **Mobile app** | — | — | — | Yes (+$499 setup) | Yes (+$499 setup) |
| **Custom domain** | — | — | Yes | Yes | Yes |

---

## Specs & Plans

All specs and implementation plans live in `docs/superpowers/`:

| Area | Spec | Plans |
|------|------|-------|
| Marketing | `specs/2026-04-10-marketing-features-design.md` | `plans/2026-04-10-marketing-m1-coupons.md` through `m4-campaigns.md` |
| Customers | `specs/2026-04-10-customers-reviews-design.md` | `plans/2026-04-10-customers-c1-storefront-auth.md` through `c3-reviews.md` |
| Settings | `specs/2026-04-10-settings-tier1-tier2-design.md` | `plans/2026-04-10-settings-s1-account-security.md` through `s5-notifications.md` |
| Dashboard | `specs/2026-04-10-dashboard-support-help-design.md` | `plans/2026-04-10-dashboard-d1-dashboard.md` through `d3-help-center.md` |
| Branding | `specs/2026-04-10-branding-subscriptions-mobile-design.md` | `plans/2026-04-10-branding-b1-storefront-branding.md` through `b3-mobile-app.md` |
| Prod Readiness | `specs/2026-04-10-production-readiness-design.md` | `plans/2026-04-10-prod-p0-critical-security.md` through `p3-dependencies-tooling.md` |
| Reviews | `reviews/2026-04-10-orders-payments-shipping-tax-review.md` | — |
