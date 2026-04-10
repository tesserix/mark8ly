# mark8ly — Release Plan

## v1.0 Release Status

| # | Feature | Status | Plan |
|---|---------|--------|------|
| **Core (Shipped)** |
| 1 | Products catalog (list, detail, variants, media, categories, CSV) | Done | — |
| 2 | Orders (create, confirm, fulfill, cancel, refund, returns, abandoned carts) | Done | — |
| 3 | Payments (Stripe, Razorpay, PayPal + webhooks) | Done | — |
| 4 | Shipping (ShipEngine, Delhivery, NinjaVan + rates) | Done | — |
| 5 | Tax (flat rate, India GST, US TaxJar) | Done | — |
| 6 | Storefront (browse, search, categories, variants, gallery, pagination) | Done | — |
| 7 | Cart + Checkout (cart, extended checkout with payment/shipping/tax) | Done | — |
| 8 | Admin settings (general, storefront, stores, team, payments, shipping, tax) | Done | — |
| **Marketing** |
| 9 | M1: Coupons (CRUD, validate, checkout, rate limiting) | Done | `marketing-m1-coupons.md` |
| 10 | M2: Gift Cards (issue, balance, checkout as payment) | Done | `marketing-m2-gift-cards.md` |
| 11 | M3: Loyalty Program (points, tiers, referrals, expiry) | Done | `marketing-m3-loyalty.md` |
| 12 | M4: Campaigns (email, segments, send worker, templates) | Done | `marketing-m4-campaigns.md` |
| **Customers & Reviews** |
| 13 | C1: Storefront Auth (GIP, session middleware, /account) | Done | `customers-c1-storefront-auth.md` |
| 14 | C2: Customer Profiles (admin list, detail, block/unblock) | Done | `customers-c2-profiles.md` |
| 15 | C3: Reviews (stars, photos, helpful, featured, moderation) | Done | `customers-c3-reviews.md` |
| 16 | C4: Wishlists (save products, heart icon, account page) | Done | `customers-c4-wishlists.md` |
| **Settings** |
| 17 | S1: Account & Security (profile, MFA, sessions, delete) | Done | `settings-s1-account-security.md` |
| 18 | S2: Custom Domains (Cloudflare DNS, verification worker) | Done | `settings-s2-custom-domains.md` |
| 19 | S3: Subscription/Billing (Stripe Billing, portal) | Done | `settings-s3-subscription.md` |
| 20 | S4: Audit Logs (viewer, search, CSV export) | Done | `settings-s4-audit-logs.md` |
| 21 | S5: Notifications (preferences, bell dropdown, poll) | Done | `settings-s5-notifications.md` |
| **Dashboard & Support** |
| 22 | D1: Dashboard (stats, sparkline, checklist, recent orders) | Done | `dashboard-d1-dashboard.md` |
| 23 | D2: Support Tickets (create, list, reply, resolve) | Done | `dashboard-d2-tickets.md` |
| 24 | D3: Help Center (markdown articles, search, contextual links) | Done | `dashboard-d3-help-center.md` |
| **Branding & Subscriptions** |
| 25 | B1: Storefront Branding (logo, colors, fonts, preview) | Planned | `branding-b1-storefront-branding.md` |
| 26 | B2: Subscription Tiers (gating, pricing, soft downgrade) | Planned | `branding-b2-subscription-tiers.md` |
| 27 | B3: React Native Mobile App (Enterprise) | Planned | `branding-b3-mobile-app.md` |
| **Production Readiness** |
| 28 | P0: Critical Security (encryption, headers, CORS) | Planned | `prod-p0-critical-security.md` |
| 29 | P1: Observability (Prometheus, Sentry, logging) | Planned | `prod-p1-observability.md` |
| 30 | P2: Performance (Next.js Image, N+1 audit, bundles) | Planned | `prod-p2-performance.md` |
| 31 | P3: Dependencies & Tooling (updates, Dependabot, docs) | Planned | `prod-p3-dependencies-tooling.md` |

**Progress: 24 of 31 done (77%)**

---

## Recommended Execution Order

| Phase | Items | Focus |
|-------|-------|-------|
| **Now** | P0 + D1 | Security fixes + Dashboard |
| **Week 1-2** | P0 + D1 | Security fixes + Dashboard (first thing merchants see) |
| **Week 3-4** | B1 + C1 | Branding (store identity) + Customer auth (shopper accounts) |
| **Week 5-6** | C2 + C3 + C4 | Customer profiles, reviews, wishlists |
| **Week 7-8** | S1 + S3 + S5 | Account security, billing, notifications |
| **Week 9-10** | D2 + D3 + S2 | Tickets, help center, custom domains |
| **Week 11-12** | B2 + P1 + P2 | Subscription gating, observability, performance |
| **Week 13+** | S4 + P3 + B3 | Audit logs, tooling, mobile app |

---

## v2.0 — Marketplace Release (separate brainstorm)

| Feature | Estimate |
|---------|----------|
| Vendor onboarding + approval | 4 weeks |
| Multi-vendor cart + order splitting | 4 weeks |
| Stripe Connect / Razorpay Route payouts | 4 weeks |
| Vendor analytics + marketplace dashboard | 3 weeks |
| Polish (vendor ratings, advanced commission) | 2 weeks |
| **Total** | **~17 weeks** |

---

## Subscription Tiers

| | Free | Starter | Pro | Enterprise | Marketplace |
|---|---|---|---|---|---|
| **USD/month** | $0 | $9.99 | $29.99 | $99.99 | $249.99 |
| **INR/month** | ₹0 | ₹499 | ₹1,499 | ₹4,999 | ₹14,999 |
| **SEA/month** | $0 | $6.99 | $19.99 | $49.99 | $124.99 |
| **Annual discount** | — | 20% off | 17% off | 16% off | 16% off |
| **Transaction fees** | 0% | 0% | 0% | 0% | 0% |
| **Products** | 25 | 500 | Unlimited | Unlimited | Unlimited |
| **Staff** | 1 | 3 | 10 | Unlimited | Unlimited |
| **Custom domain** | — | — | Yes | Yes | Yes |
| **Mobile app** | — | — | — | Yes (+$499) | Yes (+$499) |
| **Trial** | Permanent | 3 months | 14 days | Contact | Contact |

---

## Specs & Plans

All documentation in `docs/superpowers/`:

| Area | Spec | Plans |
|------|------|-------|
| Marketing | `specs/2026-04-10-marketing-features-design.md` | `m1-coupons` `m2-gift-cards` `m3-loyalty` `m4-campaigns` |
| Customers | `specs/2026-04-10-customers-reviews-design.md` | `c1-storefront-auth` `c2-profiles` `c3-reviews` `c4-wishlists` |
| Settings | `specs/2026-04-10-settings-tier1-tier2-design.md` | `s1-account` `s2-domains` `s3-subscription` `s4-audit` `s5-notifications` |
| Dashboard | `specs/2026-04-10-dashboard-support-help-design.md` | `d1-dashboard` `d2-tickets` `d3-help-center` |
| Branding | `specs/2026-04-10-branding-subscriptions-mobile-design.md` | `b1-branding` `b2-subscription-tiers` `b3-mobile-app` |
| Prod Readiness | `specs/2026-04-10-production-readiness-design.md` | `p0-security` `p1-observability` `p2-performance` `p3-tooling` |
| Reviews | `reviews/2026-04-10-orders-payments-shipping-tax-review.md` | — |
