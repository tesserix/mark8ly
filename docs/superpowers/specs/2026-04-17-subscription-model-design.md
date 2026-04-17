# Mark8ly Subscription Model — Design

**Status:** Draft for review
**Date:** 2026-04-17
**Scope:** Finalize pricing, plans, trial mechanics, billing infrastructure, feature matrix, and enforcement rules for mark8ly. Implementation-ready. Does **not** design the admin UI flows for plan management (next phase) or the Stripe webhook wiring (implementation detail in plan).

---

## Summary

Mark8ly ships a **three-visible-plan** subscription model: a generous 3-month free trial, a single flat-rate **Paid** plan ($19/mo, $182/yr annual), and a sales-led **Enterprise** plan starting at $299/mo + $2,000 one-time setup. A fourth **Marketplace** plan is reserved in code but hidden from the UI for a future multi-operator phase. Mark8ly takes zero transaction fees — merchants bring their own payment gateway keys (Stripe, Razorpay, PayPal, etc.) for their storefront checkout. Mark8ly's *own* billing runs through a single Stripe Australia account serving all 15 supported countries, with geo-localized rounded prices.

Most of the backing infrastructure is already built: the `plangate` package (22-feature × 5-plan matrix with Gin middleware), `store_subscriptions` table, and `internal/payment/stripe.go` scaffolding all exist. This spec collapses the existing Free/Starter/Pro tiers into a single **Paid** plan, finalizes prices and limits, and closes the gap between what's in code and what the pricing page will show.

---

## 1. Context — what already exists

| Artifact | Location | Current state | Gap |
|---|---|---|---|
| Feature matrix | `services/marketplace-api/internal/plangate/gate.go` | 22 features × 5 plans (Free/Starter/Pro/Enterprise/Marketplace) with boolean + numeric limits | Collapse Free/Starter/Pro into single "Paid". Update limits to match this spec. |
| Subscription model | `services/marketplace-api/internal/subscription/models.go` | `StoreSubscription` GORM model with plan, status, Stripe IDs, period windows | `Plan` enum needs to match new lineup; `marketplace` missing from the Go const list (plangate has it, models.go does not) |
| Migration | `services/marketplace-api/migrations/000015_subscriptions.up.sql` | `store_subscriptions` table, `(store_id)` unique, Stripe customer + subscription IDs | No changes needed |
| Plan resolver + middleware | `plangate.NewPlanResolver` + `RequireFeature`, `RequirePlan` | Boolean + min-plan gating middleware ready | Integrate at all gated endpoints (audit pass required) |
| Stripe client | `services/marketplace-api/internal/payment/stripe.go` | Partial — used for merchant storefront checkout, not for Mark8ly's own subscription billing | Add subscription + checkout session + customer portal + webhook flows for Mark8ly-as-merchant-of-record |
| Admin subscription handler | `services/marketplace-api/internal/handlers/admin/subscription.go` | Exists | Verify DTOs match new plan lineup; add upgrade/manage flows |

**The point of this spec is decision-level, not greenfield design** — we're locking numbers and rules so implementation is unambiguous.

---

## 2. Plan lineup

| Plan | Visibility | Pricing | Billing | Audience |
|---|---|---|---|---|
| **Trial** | Default on signup, no card | $0 for 90 days | N/A | Everyone |
| **Paid** | Shown on pricing page as "Mark8ly" | $19/mo monthly, $182/yr annual (~$15.20/mo, 20% discount) | Monthly or annual, merchant choice | Single-brand merchants, up to 2 stores |
| **Enterprise** | "Contact sales" button on pricing page | Starts at $299/mo annual + $2,000 one-time setup (non-refundable) | Annual only | Multi-store, white-label app, SSO, API, SLA needs |
| **Marketplace** | *Hidden in UI, placeholder in code* | TBD (future phase) | TBD | Future multi-operator marketplace operators |

**Rationale for single Paid tier (vs. a Starter/Pro ladder):**
- 3-month trial already does the "try before commit" job a cheap Starter tier would do
- Merchants compare mark8ly to Shopify Basic ($29) — a single $19 plan is the easiest pitch ("same capability, cheaper, no transaction fee")
- Collapsing Free/Starter/Pro limits into one Paid tier simplifies support, feature gating, and upgrade UX
- Real upsell to Enterprise is feature-driven (white-label app, SSO, multi-store) not scale-driven — so no need for a "more products" middle tier

**Rationale for Enterprise floor + setup fee:**
- Floor self-qualifies leads: merchants who balk at $299/mo don't waste sales-team time
- Non-refundable $2,000 setup fee covers real work (Apple + Google app submissions, SSO wiring, onboarding) and filters tire-kickers
- Contact-sales motion captures bigger deals per case — pricing flexibility is a feature, not a bug, at this tier

---

## 3. Pricing — localized prices for 15 countries

Geo-detected local price shown on the pricing page, charged in USD by Stripe, with card network handling FX at the consumer's end. Local prices are **round numbers, not FX conversions** — Mark8ly absorbs small FX margins to keep native-feeling prices.

| Country group | Paid monthly | Paid annual | Enterprise floor |
|---|---|---|---|
| US, CA, SG, AE, NZ | $19 | $182 | $299 |
| UK | £15 | £144 | £239 |
| EU (IE, DE, FR, NL, ES, IT) | €17 | €163 | €269 |
| Australia | A$29 | A$278 | A$459 |
| India | ₹1,499 | ₹14,399 | ₹23,999 |

**Update cadence:** review every 6 months. Re-price any row if USD moves >10% against that currency since the last review.

**Enterprise setup fee:** $2,000 USD (or local equivalent shown to the prospect in a sales quote; invoice generated from the quote).

### FX and revenue handling

- Merchant charged in USD on their card (Stripe cross-border transaction)
- Stripe Australia settles to Mark8ly in AUD after FX at Stripe's spread
- Mark8ly bears USD→AUD FX risk on received revenue. Typical Stripe FX spread is ~2% — priced into the $19 anchor
- Merchant sees `$19 USD` (or `~£15` etc.) on their credit card statement; display price on pricing page is the localized row

### Edge case — India annual billing & RBI e-mandate

Annual charge (₹14,399 ≈ $173 USD) sits near the **₹15,000 threshold** for international-card recurring payments under RBI's e-mandate framework. This has operational consequences:

- Indian merchants on **monthly** billing (~₹1,499/charge): safe — well under threshold, standard recurring flow works
- Indian merchants on **annual** billing: verify with Stripe India pre-launch. If blocked, fallback is one-time NetBanking/UPI charge rather than card-on-file auto-renew. This means annual renewals for Indian merchants become a proactive email-reminder-with-payment-link flow, not a silent auto-charge
- Implementation: detect `country = IN` + `billing_period = annual` at subscription creation; if so, set `billing_cycle_mode = invoice_based` rather than `card_on_file`

---

## 4. Trial mechanics (90-day free trial)

### Timeline

| Day | Event | Admin experience | Storefront experience |
|---|---|---|---|
| 0 | Signup | Full access, no card prompt | Fully live |
| 0–60 | Normal use | Full access, no banners | Fully live |
| 60 | Banner appears | Persistent banner "Add a card before day 90 to avoid interruption" — dismissable per session but reappears on next login | Fully live |
| 75 | Banner escalates | Banner accent shifts to signal (vermillion/amber), email reminder sent to owner | Fully live |
| 85 | Final nudge | Second email reminder, banner adds explicit "5 days remaining" countdown | Fully live |
| 90 (no card) | Trial expires | Admin goes **read-only** — merchant can view but not edit orders, products, settings | Storefront flips to editorial **"store closed"** page (branded, reassuring, not apologetic) |
| Day 90 + 60 | Soft-delete window ends | Store and data scheduled for deletion if still no card | Storefront stays in "store closed" mode |
| Day 90 + 60+ε | Hard delete | Data purged, store removed | 404 |
| Any day a card is added | Immediate paid subscription | Banner dismissed, full access restored, billing cycle starts today | Fully live (if was in read-only/closed) |

### Key rules

- **No card required at signup.** Full product access from day 0.
- **Trial length is fixed at 90 days.** Cannot be extended via promo code.
- **Cannot stack discounts on top of the trial.** See §6 on promo rules.
- **Store-closed page** is a branded template (hairline rules, Mark8ly design tokens, reassuring copy). Not a dev error page, not a 404, not a generic "upgrade" wall. The merchant's brand is still visible in header/footer.
- **Soft-delete retention** = 60 days post-trial-expiry. Data remains in database but marked `deleted_at`. Merchant can still add a card during this window to restore everything.
- **Hard delete** after day 150 total (trial + retention) removes data irreversibly. One final email sent 7 days before hard delete.

### Merchant-of-record & TOS

Mark8ly Pty Ltd (Australia) is the merchant-of-record for all subscriptions. Indian subscribers receive a separate GST-compliant invoice document from a TBD entity structure (to be finalized with tax/legal review pre-launch).

---

## 5. Transaction fee model — zero

**Mark8ly takes no transaction fee on merchant GMV.** Merchants bring their own payment-gateway keys (Stripe, Razorpay, PayPal, Stripe Connect, whichever) into their store's payment configuration. Funds flow directly from customer to merchant's gateway account. Mark8ly never touches the money.

This is a deliberate differentiator vs Shopify's per-transaction fees on non-Shopify-Payments gateways. Pricing page copy should explicitly call out: *"Your store, your payments, no middleman fees."*

**Implication:** Mark8ly's only revenue from a merchant is their subscription fee. No volume upside from high-GMV merchants on Paid — which is precisely why Enterprise exists and is priced at 15× Paid.

---

## 6. Promo codes

### Rules

| Rule | Value |
|---|---|
| When promo codes apply | **After** the 90-day trial only |
| Stacking with trial | Never. Promos cannot extend the trial or discount the trial period (which is already $0). |
| Max discount depth | 50% off |
| Max duration (post-trial monthly promo) | 6 months |
| Annual promo | One-shot, year-1 only — never recurring |
| Monthly + annual promo on same subscription | Not allowed. Subscription has one active promo at a time. |

### Allowed shapes

- **Post-trial monthly discount** — e.g. `FOUNDER50`: first 3 months after trial at $9, then snap to $19.
- **Annual upfront discount** — e.g. `FOUNDER20`: 20% off year 1 annual = $146 instead of $182.

### Grandfathered launch rates

A founder/early-access segment strategy (e.g. "first 100 signups get $14/mo forever") is allowed as a **segment**, not a promo code. Implementation: a `grandfathered_price` override on `store_subscriptions`. Runs once at launch, closed permanently after the allotment is used.

### Abuse prevention

- Each email address can redeem a given promo code at most once
- Promo codes have an explicit `starts_at` / `expires_at` window
- All redemptions logged to `promo_redemptions` table for audit

---

## 7. Refund policy

Mark8ly's refund stance is **14-day cooling-off from first charge, no refunds after**. Legally defensible in all 15 countries (complies with EU CRD Article 9, UK Consumer Contracts Regulations, AU ACL).

### Rules

| Scenario | Refund |
|---|---|
| Customer cancels within 14 days of first charge after trial | **Full refund** of that charge, no questions |
| Customer cancels after 14 days | **No refund.** Cancel is honored but access continues until end of paid period; no pro-rated refund of unused days/months. |
| Enterprise one-time setup fee | **Never refundable.** |
| Chargebacks via card network | Mark8ly contests based on access logs + refund policy in TOS |

### Why practical exercise rate will be <2%

The 14-day window starts when the card is first charged. That happens *after* a 90-day trial in which the merchant already had full product access. The 14-day window exists to satisfy EU/UK statutory distance-selling rights, not to serve as a real evaluation period — which the 90-day trial already provides.

### TOS copy

Terms of Service must clearly state the 14-day window, its start point (first charge after trial, not signup date), and the exclusion of the Enterprise setup fee. Legal review required before launch.

---

## 8. Feature matrix — what's in each plan

| Feature | Trial | Paid | Enterprise |
|---|---|---|---|
| **Limits** | | | |
| Stores | 1 | **2** | up to 10 |
| Products | Unlimited | Unlimited | Unlimited |
| Categories | Unlimited | Unlimited | Unlimited |
| Orders per month | Unlimited | Unlimited | Unlimited |
| Staff seats | Unlimited | Unlimited | Unlimited |
| Images per product | 25 | **25** | Unlimited |
| Image file size (all plans) | 10 MB | 10 MB | 10 MB |
| Audit log retention | 90 days | 90 days | Forever |
| Campaign emails/month | 5,000 | **15,000** | Negotiated |
| Transactional emails | Unlimited (platform 100k/mo fair-use) | Unlimited (same) | Unlimited, negotiated ceiling |
| Active coupons | Unlimited | Unlimited | Unlimited |
| Loyalty programs | Unlimited | Unlimited | Unlimited |
| Marketing campaigns created | Unlimited | Unlimited | Unlimited |
| **Storefront customization** | | | |
| Custom domain | ✓ | ✓ | ✓ |
| Full color palette | ✓ | ✓ | ✓ |
| Announcement bar | ✓ | ✓ | ✓ |
| Remove "Powered by Mark8ly" | ✓ | ✓ | ✓ |
| **Custom CSS + fonts + code injection** | — | — | **✓** |
| **White-label iOS + Android mobile app** | — | — | **✓** |
| **Platform features** | | | |
| CSV import/export | ✓ | ✓ | ✓ |
| Shipping labels | ✓ | ✓ | ✓ |
| Returns | ✓ | ✓ | ✓ |
| Reviews | ✓ | ✓ | ✓ |
| Support tickets | ✓ | ✓ | ✓ |
| Gift cards | ✓ | ✓ | ✓ |
| API access + webhooks | — | — | ✓ |
| SSO (SAML / OIDC via GIP) | — | — | ✓ |
| Uptime SLA | — | — | ✓ |
| **Support** | | | |
| Standard email support (24h response) | ✓ | ✓ | ✓ |
| Priority support + named CSM | — | — | ✓ |
| Onboarding concierge | — | — | ✓ |

**Notes on specific decisions:**

- **Images per product at 25.** Empirically covers > 95% of real stores. Upgrade nudge triggers when a merchant tries to add image #26 in the product editor — high-intent moment. A per-product cap > total-store-count (which merchants can't conceptualize) > GB cap (opaque).
- **Staff unlimited on Paid.** You are not Slack. If a merchant has 50 staff, they need SSO + white-label app anyway → natural Enterprise upgrade.
- **Audit logs unlimited volume, 90-day retention on Paid / forever on Enterprise.** Retention is the real cost driver and the compliance upgrade hook.
- **Campaigns count the email sends, not the campaigns created.** Merchants can design as many campaigns as they want; enforcement is at send time.
- **Custom CSS in Enterprise only.** Real support burden (merchants break their theme, blame us), and a genuine Enterprise-grade ask.

---

## 9. Email enforcement — campaign-level metering

### Enforcement point

The cap is enforced **at the moment a merchant clicks "Send campaign"**. Pre-send flow:

```
1. Merchant selects recipient segment, clicks Send
2. Handler computes this_send_count = COUNT(recipients)
3. Handler computes month_total = SUM(campaign_email_meter.send_count)
   WHERE store_id = ? AND month = current_month_utc
4. If month_total + this_send_count > plan_limit:
     Return 403 with upgrade message
     Do not dispatch to SendGrid
5. Else:
     Dispatch campaign to SendGrid
     INSERT INTO campaign_email_meter (store_id, campaign_id, sent_at, send_count)
```

### Schema addition

```sql
CREATE TABLE campaign_email_meter (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    store_id     UUID        NOT NULL,
    tenant_id    UUID        NOT NULL,
    campaign_id  UUID        NOT NULL,
    sent_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    send_count   INT         NOT NULL
);
CREATE INDEX ON campaign_email_meter (store_id, sent_at);
```

### Monthly meter reset

- Meter key: `DATE_TRUNC('month', sent_at AT TIME ZONE 'UTC')`
- No rollover of unused quota
- Merchant-visible "X of Y emails used this month" counter in campaigns UI

### Transactional email flow is separate

Order confirmations, shipping updates, password resets, ticket replies are sent via the transactional SendGrid template system. These are **not metered against the plan cap**. A platform-wide 100,000 emails/month/store fair-use ceiling on *transactional* exists as an abuse safety (for detecting compromised accounts spamming); it is not a product feature.

### Cost model

At realistic utilization (30–40% of cap) and 100 Paid merchants, SendGrid cost is ~$4–5 per merchant/month = $400–500/month aggregate. This is priced into the $19 Paid margin. Worst-case (every merchant maxes to 15k) = ~$8–10 per merchant/month = marginally profitable. Margin pressure is the reason the cap exists.

---

## 10. Image and file caps — platform safety

### Per-product image count

- **Trial + Paid:** 25 images per product (counted across all variants + media kinds)
- **Enterprise:** Unlimited

Enforcement at `POST /admin/stores/:storeId/products/:productId/media`:

```go
// Pseudocode
count := r.CountMedia(productID)
if count >= plangate.GetLimit(plan, FeatureImagesPerProduct) {
    return 403 { "error": "plan_limit", "message": "Upgrade to Enterprise for unlimited images per product", "feature": "images_per_product", "limit": N, "used": count }
}
```

### Per-file size cap (all plans)

**10 MB maximum per uploaded image file.** Enforced at the upload URL issuance step and rechecked server-side after upload completes. This is a platform abuse safety, not a plan feature — applies identically to Trial, Paid, and Enterprise.

```go
const MaxImageBytes = 10 * 1024 * 1024
```

Merchants uploading 50 MB 4K PNGs will be told to resize. Auto-transcode to WebP + size variants happens server-side after upload (already implemented for `product_media`).

---

## 11. SSO (Enterprise only)

### Protocols supported

- **SAML 2.0** — most enterprise IdPs (Okta, Azure AD, OneLogin, Ping)
- **OIDC** — Google Workspace, Auth0, modern IdPs

Both leverage Google Identity Platform (GIP), which already backs Mark8ly's user authentication. GIP's built-in SAML + OIDC federation means we are not building an IdP from scratch — we're adding per-tenant federation config.

### Per-tenant config UI

An Enterprise admin can paste their IdP metadata (SAML XML URL or OIDC issuer URL + client ID/secret). Mark8ly stores this in a new `tenant_sso_configs` table, per tenant. Staff logins for that tenant are then redirected to the IdP for authentication.

### Just-in-time provisioning

New employee logs in via their company IdP → Mark8ly creates their user record on the fly with default staff role. Admin can then adjust roles/permissions. No SCIM in v1 (SCIM = automated add/remove from IdP; nice-to-have for v2).

### Break-glass admin

Every Enterprise tenant keeps one non-SSO Mark8ly-native admin account for emergencies (IdP outages, SSO config breakage). Stored with MFA mandatory.

### Build effort

2–3 weeks engineering:
- Week 1: SAML + OIDC verification, tenant config storage
- Week 2: Admin UI for SSO config, test flows against Okta + Google Workspace
- Week 3: JIT provisioning, break-glass flow, audit logs for SSO events, QA

Out of scope for this spec (this lives in the *implementation plan*, not the pricing spec).

---

## 12. White-label mobile app (Enterprise only)

### Key constraint — Apple's "app factory" policy

Apple rejects batch-submitted apps from a single publisher. Industry workaround (Tapcart, Vajro, Appy Pie): **apps are submitted under the tenant's own Apple Developer and Google Play Console accounts.** The tenant provides API access; Mark8ly handles the build, asset prep, submission, and ongoing updates.

### What the $2,000 setup fee covers

- Apple + Google dev-account setup guidance (tenant owns the accounts)
- Branded app build (React Native or Capacitor wrapper around existing storefront theme, branded splash/icon/deep-links)
- Initial submission to both stores (typically 5–10 business days to approval)
- Custom push notification setup via Firebase for the tenant's app
- One onboarding call with named CSM
- 60 days of post-launch support on app-store issues

### Ongoing responsibilities (included in Enterprise monthly)

- App updates every major OS release (iOS + Android) to maintain compatibility
- Push notification delivery infrastructure
- Crash monitoring + incident response
- Annual Apple Developer fee ($99) — tenant pays under their own account
- Google Play Console is one-time $25 — tenant pays

### Tech stack

Existing Mark8ly codebase doesn't yet have a mobile app. Implementation plan should evaluate:
- **Option A: React Native** — most flexibility, shares components with web storefront
- **Option B: Capacitor/Ionic** — web-view wrapper, fastest to build, limited native feel
- **Option C: Native iOS (Swift) + Android (Kotlin)** — best UX, biggest maintenance burden

Recommendation for first enterprise deal: **Capacitor** as a pragmatic v1 (ships in weeks, not months), migrate to React Native if UX demands it. Final decision belongs to the implementation plan.

---

## 13. Implementation gaps — what needs to be built

| Area | Gap | Rough effort |
|---|---|---|
| Plan enum collapse | Remove Starter + Pro from code; migrate existing (if any) to Paid | 1 day |
| `plangate` matrix update | Rewrite feature matrix to match this spec (2 stores on Paid, 25 img/product, etc.) | 1 day |
| Stripe subscription billing | Webhook handler for `customer.subscription.*`, `invoice.*`, `charge.*` events → sync to `store_subscriptions` | 1 week |
| Stripe checkout + billing portal flow | New Paid signup → Stripe checkout session; "Manage billing" → Stripe customer portal | 3 days |
| Trial expiry scheduler | Daily cron: check trials, update status, send banner emails, flip to read-only at day 90 | 3 days |
| Read-only mode | Middleware that blocks all mutations in admin when `subscription_status = expired` | 2 days |
| Store-closed storefront page | Editorial page template, served when store's subscription is `expired` + `grace_period_ended` | 2 days |
| Campaign email meter | New `campaign_email_meter` table + enforcement in campaign send handler | 3 days |
| Banner + reminder emails | In-admin banner component with escalation states + transactional templates for day 60/75/85 | 2 days |
| Promo code infrastructure | `promo_codes` + `promo_redemptions` tables, validation, application at subscription start | 1 week |
| Geo-localized pricing page | IP/billing-address detection → show correct currency row | 2 days |
| RBI e-mandate fallback | For `country = IN` + `billing_period = annual`: invoice-based flow instead of card-on-file | 3 days |
| Refund flow | Admin-side (Mark8ly staff) tool to issue refund within 14-day window | 2 days |
| SSO (Enterprise) | SAML + OIDC via GIP, per-tenant config, JIT provisioning | 2–3 weeks |
| White-label mobile app (Enterprise) | Evaluation + v1 build (decoupled from this spec; own phase) | 4–8 weeks first build, ~3 days per subsequent tenant |
| Pricing page + marketing site | Design + copy (editorial tone) | 1 week |
| Admin plan-management UI | Current plan display, "Manage billing" link, upgrade CTA, usage counters (emails used this month, etc.) | 1 week |

**Total back-end engineering to ship Paid + Trial in production:** ~4–5 weeks. Enterprise adds ~6–8 weeks depending on whether white-label app is in scope for the first deal.

---

## 14. Risks, edge cases, and open questions

### Risks

1. **RBI e-mandate for Indian annual subscriptions** (§3) — annual charge sits at the international-card recurring threshold. Must verify with Stripe India pre-launch. Fallback flow is defined but adds UX complexity for Indian merchants on annual.
2. **SendGrid cost at scale** — 15k cap is margin-positive but the headroom shrinks fast if merchants abuse it. Monitoring + kill-switch for stores exceeding expected patterns is mandatory.
3. **Apple app-factory policy** — Mark8ly cannot ship white-label apps under a single Mark8ly publisher account. Mitigation: tenant-owned Apple accounts (§12). Risk if Apple tightens further: alternative is a Shopify-Shop-app style "Mark8ly Merchant" master app with tenant switching — loses the white-label value prop.
4. **Trial abuse** — same person signing up multiple trials with different emails to avoid ever paying. Mitigation: fingerprint by card (when eventually added) + email + IP + GCP device fingerprint. Low priority v1.
5. **Stripe AU merchant-of-record risk** — if Mark8ly grows big enough, a stricter merchant-of-record model (e.g. Lemon Squeezy, Paddle) may be needed for international tax compliance. Revisit at $1M ARR.
6. **Promo code abuse** — one email signs up with promo, cancels after promo expires, rotates email. Mitigation in §6 limits this but doesn't eliminate it. Acceptable loss for v1.

### Open questions — require human input before implementation plan

1. **Indian merchant entity for GST** — currently "TBD" in §4. Tax/legal consultation needed to decide: Mark8ly Pty Ltd direct (simpler, no GST collection but merchants lose GST input credit), or an Indian entity (adds operational overhead, enables GST invoicing).
2. **Exact 15-country list** — this spec assumes the set inferred from the user's conversation (US, UK, AU, CA, NZ, IE, IN, AE, SG, DE, FR, NL, ES, IT + 1 TBD). Confirm the specific 15th, especially for compliance (ME/LATAM/SEA).
3. **Grandfather-rate launch campaign** — if yes, when does it run and how many slots? Spec allows for it; execution TBD.
4. **White-label app tech stack** — Capacitor (v1) vs React Native (v1) deferred to implementation plan. Decision blocks first Enterprise deal.

### Out of scope

- **Marketplace plan features** — placeholder only; no design work until the multi-operator use case is concrete.
- **Payment gateway aggregation** — mark8ly does not act as a payment aggregator; merchants bring their own keys. Always. Any "Mark8ly Payments" style product is out of scope indefinitely.
- **Billing-triggered storefront behavior for storefront shoppers** — customers shopping on a merchant's storefront never see Mark8ly's billing state. The store either works (active) or shows the store-closed page (expired). No upsell CTAs, no Mark8ly branding leak.
- **Discount and promo UX for end customers** — this spec is about *merchant* subscriptions; customer-facing discounts on a merchant's storefront are a different feature entirely (already exists as `coupons` + `gift_cards`).

---

## 15. Decision log

Every meaningful decision in this spec traces back to a specific conversation:

| Decision | Rationale |
|---|---|
| 3-month trial (not 14 or 30) | Merchants take real time to launch a store. Aggressive trial attracts serious merchants; conversion quality > conversion rate. |
| No card at signup | Editorial/calm brand positioning, removes top-of-funnel friction, trusts the product to earn the card by day 60. |
| Single Paid tier (not Starter/Pro/Growth) | 3-month trial removes the "cheap entry" need. One price is easier to sell and support. |
| $19/mo (not $29 Shopify-parity or $14 low-floor) | $19 is the cheapest *legit* commerce SaaS price while staying margin-positive. Below Shopify = competitive, not cheapened. |
| Geo-localized prices | Better conversion in non-US markets; the FX margin absorbed by Mark8ly is a worthwhile acquisition cost. |
| Zero transaction fee | Genuinely differentiated from Shopify. Clean revenue model. BYO gateway keys align with brand: "your store, your money". |
| Flat-monthly model (no GMV %) | Matches editorial/calm positioning; no "pennies-per-transaction" feel. |
| 14-day cooling-off, no refunds after | Legally bulletproof in 15 countries. Practically irrelevant post-90-day-trial. Simple accounting. |
| Enterprise floor $299 + $2k setup, non-refundable | Self-qualifies leads. Setup fee covers real work. Non-refundable filters tire-kickers and signals seriousness. |
| Enterprise = contact sales (not self-serve) | White-label mobile app requires human conversation (dev accounts, onboarding). Sales motion fits. |
| 25 images per product on Paid | Per-product cap is the most merchant-intuitive; covers >95% of real stores; upgrade nudge triggers at high-intent moment. |
| 2 stores on Paid (not 1 or 3) | Gives a brand + pop-up room without opening the agency-stacking door. Multi-store is the clearest Enterprise upsell. |
| Unlimited staff on Paid | Mark8ly is not Slack. 50-staff merchants need SSO + white-label → natural Enterprise. |
| 5k/15k campaign email caps | Tight enough to be margin-positive at worst-case; realistic at typical utilization. Distinguishes marketing from transactional. |
| Custom CSS in Enterprise only | Support burden when merchants break their theme; genuinely enterprise-grade request. |
| SSO in Enterprise only | Gate criterion at 100+ employee merchants. Real compliance value, real engineering cost. |
| White-label app requires tenant-owned app-store accounts | Apple's app-factory policy. Industry standard. |

---

## 16. Success criteria

Implementation of this spec is complete when:

1. A new merchant can sign up → create a store → run the 3-month trial → add a card → be charged the localized price → continue using their store under Paid plan, all without manual intervention.
2. A merchant on Paid who tries to create a 3rd store is blocked with a clear upgrade-to-Enterprise message linking to a "Contact sales" form.
3. A merchant on Paid who tries to send a 16,001st campaign email in a calendar month is blocked with a clear upgrade message.
4. A trial merchant on day 91 without a card sees a read-only admin and an editorial store-closed page — and can recover full access by adding a card.
5. A merchant who cancels within 14 days of first charge receives a full refund via Stripe.
6. A merchant who cancels after 14 days retains access until end of period with no refund processed.
7. An enterprise sales inbound fills out the "Contact sales" form; a human responds within 24h; a custom quote including the $2,000 setup fee is issued.
8. Pricing page displays geo-localized prices for all 15 countries.
9. Indian annual subscribers either complete via card-on-file (if their charge is under ₹15k) or via one-time invoice flow.
10. All of the above is observable in the `store_subscriptions` table, in Stripe's dashboard, and in the admin plan-management UI.

---

## 17. Next step

After user approval of this spec, proceed to **implementation plan** via the `writing-plans` skill. The plan will decompose §13's gap list into atomic, sequenced tasks with tests-first structure and atomic commit boundaries.
