# Mark8ly Subscription Model — Design

**Status:** v2.1 — addresses round-2 reviewer MEDIUMs, Pro restructure, PPP pricing
**Date:** 2026-04-17 (revised 2026-04-18)
**Scope:** Final pricing, plans, trial mechanics, billing infrastructure, feature matrix, tax model, and enforcement rules. Includes state machine, concurrency controls, security requirements, observability, and DR. Implementation-ready.

---

## 1. Summary

Mark8ly ships a **B2B-only** subscription model across 18 countries with PPP-adjusted pricing for emerging markets:

- **Trial** — 3 months free, no card, email-verified + reCAPTCHA gated, default on signup
- **Starter** — from $19/mo (developed markets) / ₹999 (India PPP-adjusted), self-serve, 2 stores
- **Studio** — from $49/mo / ₹2,499 (India), self-serve, 5 stores, custom CSS, 50k emails, 12-month audit retention, read-only API
- **Pro** — **from $99/mo** / ₹5,499 (India), contact-sales with **visible "Starts at" floor**, 10 stores, SSO, full read/write API, custom code injection, priority support, unlimited
- **White-label mobile app add-on** — **$199/mo + $2,000 non-refundable setup**, Pro-only, bundles named CSM + SLA + onboarding concierge + branded iOS/Android apps under tenant's developer accounts
- **Marketplace** — placeholder in code, hidden from UI (future)

Mark8ly charges zero transaction fees on merchant GMV — merchants bring their own payment gateway keys (Stripe, Razorpay, PayPal) for *their* storefront checkout. Mark8ly's own billing runs through a single Stripe Australia account serving all 18 countries. Prices are geo-localized in round native numbers; 12 developed markets priced at USD parity, 6 emerging markets (IN, MY, TH, PH, ID, VN) get ~33% PPP discount.

Because every merchant is a business entity, Mark8ly operates under the **B2B reverse-charge tax model** (EU/UK/SEA/India) — merchants self-account for VAT/GST. Mark8ly validates tax IDs at onboarding (VIES for EU, HMRC for UK, GSTN for India, local validators for SEA) and issues reverse-charge-compliant invoices. No consumer signups permitted.

Substantial scaffolding exists already: `plangate` (Go package with feature-gating middleware), `store_subscriptions` table, `internal/payment/stripe.go` client, shipping carriers (ShipEngine, NinjaVan, Delhivery) covering all 18 launch countries. This spec collapses the existing Free/Starter/Pro matrix into the new 4-tier model, fixes a latent multi-tenant security defect, adds Stripe webhook + state-machine specifications, and defines failed-payment/dunning + cancellation flows that were missing.

---

## 2. Existing infrastructure audit

| Artifact | Location | Status | Gap |
|---|---|---|---|
| Feature matrix | `services/marketplace-api/internal/plangate/gate.go` | 22 features × 5 plans (Free/Starter/Pro/Enterprise/Marketplace) | Rewrite to 4 plans (Trial/Starter/Studio/Pro) + Marketplace hidden; update all limits to match §9 |
| Subscription model | `services/marketplace-api/internal/subscription/models.go` | `StoreSubscription` struct + `SubscriptionPlan` enum (4 values, missing `marketplace`) | Update plan enum; add `ReverseChargeTaxID`, `TaxIDCountry`, `TaxIDValidated`, `TaxIDValidatedAt`, `HasWhiteLabelAppAddOn` fields |
| Subscription repository | `services/marketplace-api/internal/subscription/repository.go` | `GetByStoreID(ctx, db, storeID)` | **SECURITY BLOCKER: `tenant_id` missing from WHERE clause.** Change signature to `GetByStoreID(ctx, db, tenantID, storeID)`. Audit all call sites. |
| Migration | `services/marketplace-api/migrations/000015_subscriptions.up.sql` | `store_subscriptions` table with Stripe IDs, period windows, plan, status, unique `(store_id)` | Add new migration: tax ID fields, plan enum rename (free→trial; starter/pro collapse → starter/studio/pro), add-on tracking, soft-delete tracking |
| Stripe client | `services/marketplace-api/internal/payment/stripe.go` | Partial — used for merchant storefront checkout only; signature verification exists; idempotency keys missing; error-body logging leaks PII | Add Stripe Billing subscription flow, Checkout sessions, Customer Portal, webhook dispatcher; fix log sanitization; add idempotency keys |
| Admin subscription handler | `services/marketplace-api/internal/handlers/admin/subscription.go` | Exists | Update DTOs to new plan lineup; add plan-switch, upgrade, cancel endpoints; enforce `RequireActive` middleware |
| Shipping carriers | `services/marketplace-api/internal/shipping/{shipengine,delhivery,ninjavan}.go` | 15 countries hardcoded in `SupportedCountries()` | Add IE, NZ to ShipEngine list; add VN to NinjaVan list; defer AE (Aramex) to post-v1 |
| Auth | Google Identity Platform + OpenFGA | Already multi-tenant; SAML + OIDC federation available | Build per-tenant SSO config for Pro (§12) |

**Audit finding:** the existing `GetByStoreID` query does not include `tenant_id` in its WHERE clause. A handler that trusts a URL-path `storeId` without verifying it belongs to the caller's `tenant_id` would leak another tenant's Stripe customer ID. This must be fixed as part of this spec's implementation — it is not a future improvement, it is a security patch owed to existing code.

---

## 3. Plan lineup

### 3.1 Tiers

| Plan | Visibility | Pricing (developed markets) | Billing | Audience |
|---|---|---|---|---|
| **Trial** | Default on signup, email-verified + reCAPTCHA | $0 for 90 days | N/A | Everyone B2B |
| **Starter** | Pricing page, self-serve | $19/mo or $182/yr (20% off) | Monthly or annual | Single-brand merchants, up to 2 stores |
| **Studio** | Pricing page, self-serve | $49/mo or $470/yr (20% off) | Monthly or annual | Growing merchants, custom CSS, larger marketing volume, up to 5 stores |
| **Pro** | Pricing page with **visible "Starts at $99/mo"** floor, "Contact sales" CTA | From $99/mo + optional $2,000 setup if add-on | Annual only (monthly available on request); quarterly invoicing for enterprise procurement | Mid-market brands, SSO, multi-store, full API, custom code injection — up to 10 stores |
| **Pro + White-label mobile app add-on** | Pro-only add-on, sales-quoted | +$199/mo + **$2,000 non-refundable setup** | Annual only, renews with Pro subscription | Brands wanting branded iOS + Android apps, includes named CSM + SLA + onboarding concierge |
| **Marketplace** | *Hidden in UI, placeholder in code* | TBD (future phase) | TBD | Future multi-operator marketplace |

### 3.2 Pricing ladder rationale

Developed-market ladder:

```
$19  →  $49  →  $99  →  $298 (Pro + App)
      2.6×      2.0×    3.0×
```

- **2.6×** Starter → Studio: real feature + limit upgrade (custom CSS, 3× emails, 2.5× stores)
- **2.0×** Studio → Pro: features serious brands need (SSO, multi-store, full API, custom code injection, priority support)
- **3.0×** Pro → Pro + App: the white-label app plus CSM/SLA bundle — aligned with real ongoing cost (app maintenance + dedicated human success contact)

No price cliffs. Each transition has a clear upgrade driver and a proportional price step.

### 3.3 Pro "contact sales" with visible floor

Pro is contact-sales-led **but with a published floor price**. The pricing page Pro card displays:

- Feature list (SSO, multi-store, full API, custom code injection, priority support)
- **"Starts at $99/mo"** — visible floor, localized per country
- Primary CTA: "Contact sales"
- Secondary CTA: "Download Pro brief" (PDF one-pager)

Rationale for visible floor rather than pure "Contact sales":
- Self-qualifies leads — prospects who balk at $99 filter themselves out pre-discovery-call
- Easier competitive comparison for prospects evaluating mark8ly vs Tapcart / Vajro / Shopify Plus
- Retains SEO ranking on "[tool] pricing" queries
- Preserves full negotiation ceiling upward (price floor, not ceiling)

Contact form captures: business name, tax ID, country, annual GMV range, intended store count, timeline, need for white-label app (yes/no). Submissions auto-create a Notion record in `Sales Pipeline` and Slack notification to `#sales-inbox`. Response SLA 24h business hours.

### 3.4 White-label mobile app add-on

Sold only as an add-on to Pro. Cannot be purchased on Starter or Studio. The **$199/mo + $2,000 non-refundable setup** covers:

- Apple Developer + Google Play Console setup guidance (tenant owns the accounts; see §13 for Apple app-factory policy context)
- Branded iOS + Android app build (tenant's icon, splash, colors, deep links into storefront)
- First submission to both stores (5–10 business days typical)
- Firebase push notification infrastructure per tenant
- **Named Customer Success Manager** (CSM — 1h/month dedicated account management)
- **Uptime SLA (99.9%)** with service credits
- **Onboarding concierge**
- 60 days post-launch issue support
- Ongoing per-major-OS update maintenance (2×/year)

**Why CSM + SLA are bundled with the app add-on (not Pro base):** the customer investing in a branded mobile app is the one who genuinely needs dedicated human support (Apple submission coordination, OS update windows, push campaign strategy). Pro-without-app customers are typically mid-market teams with internal ops — they prefer fast priority email support over scheduled check-ins. Aligning the CSM cost with the customer who values it keeps Pro base accessible at $99.

---

## 4. Pricing — localized for 18 countries

### 4.1 Developed markets — USD parity

12 countries priced at USD-equivalent local numbers. Geo-detected on the pricing page from billing address or IP. Charged in USD by Stripe AU; card network handles FX at the consumer's end.

| Country | Shipping carrier | Starter monthly | Starter annual | Studio monthly | Studio annual | Pro from |
|---|---|---|---|---|---|---|
| US, CA | ShipEngine | $19 | $182 | $49 | $470 | $99 |
| UK (GB) | ShipEngine | £15 | £144 | £39 | £375 | £79 |
| Ireland (IE) *new carrier config* | ShipEngine | €17 | €163 | €45 | €432 | €89 |
| EU (DE, FR, IT, ES, NL) | ShipEngine | €17 | €163 | €45 | €432 | €89 |
| Australia | ShipEngine | A$29 | A$278 | A$75 | A$719 | A$149 |
| New Zealand *new carrier config* | ShipEngine | NZ$29 | NZ$278 | NZ$75 | NZ$719 | NZ$149 |
| Singapore | NinjaVan | S$25 | S$239 | S$65 | S$623 | S$129 |

### 4.1.1 Emerging markets — PPP-adjusted (~33% off USD parity)

6 countries priced ~33% below USD parity to reflect purchasing-power differences. This is industry standard (Figma, Notion, Canva, Spotify, Netflix all do this). Without PPP adjustment, Starter at ₹1,499/mo would represent 3% of revenue for a typical indie Indian DTC brand doing ₹50k/mo GMV — an accessibility problem that excludes the long-tail merchant segment Mark8ly wants to serve.

| Country | Shipping carrier | Starter monthly | Starter annual | Studio monthly | Studio annual | Pro from |
|---|---|---|---|---|---|---|
| India | Delhivery | ₹999 | ₹9,599 | ₹2,499 | ₹23,999 | ₹5,499 |
| Malaysia | NinjaVan | RM 59 | RM 569 | RM 149 | RM 1,429 | RM 299 |
| Thailand | NinjaVan | ฿499 | ฿4,799 | ฿1,199 | ฿11,519 | ฿2,399 |
| Philippines | NinjaVan | ₱749 | ₱7,199 | ₱1,899 | ₱18,239 | ₱3,799 |
| Indonesia | NinjaVan | Rp 199,000 | Rp 1,919,000 | Rp 499,000 | Rp 4,799,000 | Rp 999,000 |
| Vietnam *new NinjaVan country* | NinjaVan | ₫329,000 | ₫3,169,000 | ₫799,000 | ₫7,699,000 | ₫1,649,000 |

### 4.1.2 Add-on — white-label mobile app

Priced **globally in USD, no localization: $199/mo + $2,000 non-refundable setup.** Reason: underlying cost base (Apple Developer fee, Firebase, build/submission labor) is USD-denominated; local-currency discounting would break unit economics.

**Deferred to v2:** UAE (AE) — requires Aramex carrier integration (~1 week build). Launch Mark8ly waitlist for AE now; enable in follow-up milestone.

### 4.2 FX handling

- Merchant sees localized price on pricing page and admin
- Stripe charges the merchant's card in USD (cross-border transaction)
- Stripe AU settles to Mark8ly in AUD after FX at Stripe's ~1.5% spread
- Card network handles FX at consumer end (merchant's bank statement shows their local currency)
- Mark8ly bears USD→AUD exposure on received revenue

### 4.3 Price-table review cadence

Every 6 months. Hard-update any currency row if USD has moved >10% against that currency since last review. PPP discounts re-evaluated annually (not every 6 months) — emerging-market currencies are more volatile, rapid re-pricing hurts trust.

### 4.4 Billing period changes

A merchant can change their billing period at any time via admin → Subscription:

- **Monthly → Annual:** takes effect **immediately** with pro-rata credit. Formula: `credit = (days_remaining_in_month / days_in_month) × monthly_price`. Charge now is `annual_price - credit`. New billing cycle starts today; next renewal in 12 months.
- **Annual → Monthly:** takes effect **at end of current annual period, not immediately**. Merchant retains annual price through the full 12 months. On renewal, subscription switches to monthly at current monthly price.
- Switching tier (Starter→Studio, Studio→Pro) is governed by §4.5, not this section.

### 4.5 Plan upgrades and downgrades

- **Upgrade immediately, prorate.** Starter→Studio charges `studio_remaining_days_price − starter_remaining_days_credit` on the spot. Upgrade effective instantly, all Studio features unlock.
- **Downgrade at end of period** (with prerequisite check for excess resources).
  - **Studio → Starter blocked if merchant has more than 2 stores.** Admin displays blocking dialog: "You have 5 stores. Starter allows 2. Choose which stores to close or delete before downgrading." No auto-suspension, no silent data loss — merchant explicitly selects.
  - If resources fit the target tier: downgrade scheduled for end of period. Merchant keeps current tier features until then.
  - Downgrade reversal allowed anytime before period end.
- **Studio → Pro is always sales-led** via the contact form. There is no self-serve "upgrade to Pro" button. This is intentional: Pro requires tax docs, setup call, and is annual-only.

### 4.6 Country change mid-subscription

Merchant relocates or updates billing address to a different country:

- Billing-country change takes effect at **next renewal**, never mid-period
- Merchant receives email 14 days before renewal: "Your billing country is changing to X. Your next invoice will be [localized_price_for_X]"
- If new country is India + billing period is annual: re-evaluate RBI e-mandate applicability (§4.7)
- If new country moves merchant from emerging-market PPP zone to developed-market zone (or vice versa): price change applies at next renewal per §4.1/§4.1.1
- If new country is not yet supported (e.g. AE pre-v2): block the change and offer migration path (wait or cancel)

### 4.7 RBI e-mandate fallback for Indian annual subscribers

Annual charges for Indian subscribers (₹9,599 and above — Studio and Pro) near the ₹15,000 international-card recurring threshold. Starter annual at ₹9,599 sits safely below; Studio annual at ₹23,999 and Pro floor ₹5,499×12 = ₹65,988/yr exceed the threshold.

Operational rules:
- Indian merchants on **monthly billing** (all plans): safe, standard Stripe recurring flow
- Indian merchants on **annual billing** AND charge >₹15,000: automatic switch to `billing_cycle_mode = "invoice_based"`:
  1. 14 days before renewal, create a Stripe Invoice (not Subscription charge)
  2. Email merchant with hosted invoice URL + enabled local payment methods (UPI, NetBanking)
  3. Reminder emails at T-14, T-7, T-1
  4. On `invoice.paid` webhook, extend subscription by 12 months
  5. If unpaid at T+0: flip subscription to `past_due` and enter dunning (§16)
- Country-of-record for RBI detection: `customer.billing_address.country` on Stripe Customer (not signup-time IP geo)

---

## 5. Trial mechanics

### 5.1 Signup gates (v1 launch)

The 3-month no-card trial needs hard gates against abuse:

- **Email verification** required before storefront publishes. Merchant can set up admin, design themes, add products during verification pending; storefront remains unpublished (returns store-closed page to public).
- **reCAPTCHA Enterprise** on the signup form at Cloudflare Worker edge layer. Free-tier scoring; block scores <0.5.
- **Rate-limit signup endpoint**: 3 signups per IP per 24h; 1 signup per email (case-insensitive, stripped of `+` aliases).
- **Disposable email blocklist**: reject known disposable domains (use `disposable-email-domains` upstream list, refreshed weekly).
- **First 7 days of trial**: campaign email sending is **disabled** (warming period). Transactional emails work. This eliminates email-relay abuse motivation while low-friction for legitimate merchants.
- **Signup volume alert**: Cloud Monitoring alert when trial signups exceed 50/day.

### 5.2 Tax ID validation at signup — hard 14-day window

Mark8ly is B2B-only (§19). Signup requires a valid tax ID for the merchant's business entity:

- Form asks: business name, country, tax ID (format validated client-side per country)
- Server-side validation against country-specific registry:
  - EU: VIES API (free, asynchronous — return immediate "pending" if VIES is slow, accept provisionally)
  - UK: HMRC VAT API
  - India: GSTN lookup API (requires Mark8ly registration + API key)
  - SEA (TH, ID, PH, VN, MY): local registry APIs; §19.3 specifies fallback for unavailable APIs
  - AU: ABN Lookup API
  - US, CA: format check + legally-binding business-entity checkbox (§19.3)
  - NZ: IRD lookup API
- **If validation fails or is pending**: signup completes but store is marked `tax_id_pending`. Admin unlocks for setup; **storefront does NOT publish until validated.**
- **Hard 14-day deadline for first-time validation.** If tax ID not validated within 14 days of signup:
  - Day 7: reminder email
  - Day 12: escalation email
  - Day 14: admin locked to read-only + billing actions only; store remains unpublished
  - Day 30: signup cancelled, data purged (60-day soft-delete per §5.3)
- Tightened from the earlier 90-day window to close the "live storefront on fake tax ID" attack surface.
- If validation passes: `tax_id_validated_at` timestamped; quarterly revalidation cron re-checks (§19.5).

### 5.3 Timeline

| Day | Event | Admin | Storefront |
|---|---|---|---|
| 0 | Signup, email verification complete, reCAPTCHA pass, tax ID submitted | Full access | **Unpublished** until tax ID validated |
| 0–14 | Tax ID validation window | Full access except campaign send | Publishes once tax ID validated |
| 0–6 | Normal use | Full access except campaign send | Full (if tax ID validated) |
| 7 | Campaign send unlocked (if tax ID validated) | Full access | Full |
| 0–59 | Normal use | Full access | Full |
| 60 | Banner appears | "Add a card before day 90. Tax ID status: verified" | Full |
| 75 | Banner escalates | Amber accent, email reminder | Full |
| 85 | Final nudge | Countdown "5 days remaining" + second email | Full |
| 90 (no card) | Trial expires | Read-only (admin allowlist per §17.3) | Flip to editorial "store closed" page |
| 90–149 | Grace / soft-delete window | Read-only | "Store closed" |
| 150 | Hard delete | Store record removed; billing archive retained (§23) | 404 |
| Any day card added | Subscription activates | Normal | Normal |

### 5.4 Store-closed storefront page

Serving rule: expired subscription → Cloudflare Worker serves a **branded, editorial "store closed"** page from CF assets. This bypasses Mark8ly's Knative storefront pods entirely — no origin hit, no compute cost, no cold-start delay.

Implementation:
- `tenant-router-service` exposes `GET /status/:storeSlug → {status, store_name, logo_url}` (cached 60s at Worker)
- Worker checks this status before routing to storefront origin
- If `status = closed` or `status = unpublished`: Worker serves `/assets/closed.html` with merchant's logo + name interpolated, 307 SEO-safe status code
- Cache invalidation via POST from marketplace-api when subscription status changes

---

## 6. Transaction fee model

**Mark8ly takes zero transaction fees on merchant GMV.** Merchants bring their own payment-gateway keys (Stripe, Razorpay, PayPal, Stripe Connect) into their store's payment configuration. Funds flow directly from customer to merchant's gateway account. Mark8ly never touches the money.

Pricing page copy: **"Your store, your payments, no middleman fees."** This is a deliberate differentiator vs Shopify's per-transaction fees on non-Shopify-Payments gateways. The pricing page must explicitly call it out.

---

## 7. Promo codes

### 7.1 Rules

| Rule | Value |
|---|---|
| When applicable | **After** the 90-day trial only. Cannot extend or discount the trial. |
| Max discount depth | 50% off |
| Max duration (post-trial monthly discount) | 6 months |
| Annual promo | One-shot, year-1 only, never recurring |
| Stacking | One active promo per subscription. Monthly + annual promos cannot combine. |
| Retroactive application | No. Promo must be entered at checkout. |

### 7.2 Allowed shapes

- **Post-trial monthly discount** — e.g. `FOUNDER50`: first 3 months after trial at 50% off, then snap to full price
- **Annual upfront discount** — e.g. `FOUNDER20`: 20% off year-1 annual
- **Grandfathered launch rate** — at launch campaign only, specific slots. Implemented as `grandfathered_price` override on `store_subscriptions`, NOT via promo code table. One-shot, closed permanently.

### 7.3 Abuse prevention

- Each email address can redeem a given promo code at most once
- Promo codes have explicit `starts_at` and `expires_at`
- All redemptions logged to `promo_redemptions` table for audit
- **Rate limit on promo validation**: 5 attempts per IP per hour; 10 per email per 24h
- **Response is timing-safe**: both "code not found" and "code expired" return the same generic `promo_code_invalid` error message with identical response time (no enumeration signal)
- **Minimum code length**: 12 characters mixed-case alphanumeric, avoiding visually ambiguous characters (no `0/O`, `1/l/I`)
- Codes stored as Stripe Coupon IDs as the canonical backend

---

## 8. Refunds

### 8.1 Policy

- **14-day cooling-off from first charge after trial** → full refund, no questions. Compliant with EU CRD Article 9, UK Consumer Contracts Regulations, AU ACL.
- **After 14 days** → cancel anytime, access retained until end of paid period, no pro-rated refund
- **Pro setup fee ($2,000 USD)** → never refundable (applies only when Pro + White-label add-on is purchased)
- **Chargebacks via card network** → Mark8ly contests based on access logs + refund policy in TOS

### 8.2 Refund fraud prevention

- **Card fingerprint tracking**: Stripe returns `fingerprint` on every PaymentMethod. Stored on `store_subscriptions`.
- **One refund per card fingerprint, lifetime.** Second refund attempt from same fingerprint requires manual CSM approval.
- **Device fingerprint logged** at card-add time (IP, user-agent hash, ASN) for correlation.

### 8.3 Refund flow (v1)

Minimal viable: Mark8ly staff processes refund directly in Stripe Dashboard, logs reason + context into `refund_audit` table via internal endpoint. No dedicated admin UI in v1. v2 adds `tesserix-home` refund-management UI with SQL-backed eligibility check.

### 8.4 TOS copy

Terms of Service must clearly state:
- 14-day window and its precise start point (first charge after trial, not signup)
- Non-refundability of Pro + White-label app add-on setup fee
- How to request a refund (email to support with order + card last-4)
- Legal-review required before launch

---

## 9. Feature matrix

| Feature | Trial | Starter | Studio | Pro | Pro + App add-on |
|---|---|---|---|---|---|
| **Limits** | | | | | |
| Stores | 1 | 2 | **5** | up to 10 | up to 10 |
| Products, categories, orders | ∞ | ∞ | ∞ | ∞ | ∞ |
| Staff seats | ∞ | ∞ | ∞ | ∞ | ∞ |
| Images per product | 25 | 25 | **50** | Unlimited | Unlimited |
| Image file size (all plans) | 10 MB | 10 MB | 10 MB | 10 MB | 10 MB |
| Audit log retention | 90 days | 90 days | **12 months** | Forever | Forever |
| Campaign emails/month | 5,000 | 15,000 | **50,000** | Negotiated | Negotiated |
| Transactional emails | ∞ (100k/mo fair-use) | ∞ (same) | ∞ (same) | Negotiated ceiling | Negotiated ceiling |
| Active coupons, loyalty programs, campaigns created | ∞ | ∞ | ∞ | ∞ | ∞ |
| **Storefront** | | | | | |
| Custom domain | ✓ | ✓ | ✓ | ✓ | ✓ |
| Full color palette | ✓ | ✓ | ✓ | ✓ | ✓ |
| Announcement bar | ✓ | ✓ | ✓ | ✓ | ✓ |
| Remove "Powered by Mark8ly" | ✓ | ✓ | ✓ | ✓ | ✓ |
| **Custom CSS + fonts** | — | — | **✓** | ✓ | ✓ |
| Custom code injection (JS/HTML) | — | — | — | ✓ | ✓ |
| **White-label iOS + Android app** | — | — | — | — | **✓** |
| **Platform** | | | | | |
| CSV import/export | ✓ | ✓ | ✓ | ✓ | ✓ |
| Shipping labels | ✓ | ✓ | ✓ | ✓ | ✓ |
| Returns | ✓ | ✓ | ✓ | ✓ | ✓ |
| Reviews | ✓ | ✓ | ✓ | ✓ | ✓ |
| Support tickets | ✓ | ✓ | ✓ | ✓ | ✓ |
| Gift cards | ✓ | ✓ | ✓ | ✓ | ✓ |
| Read-only API + webhooks (rate-limited) | — | — | **✓** | ✓ | ✓ |
| Full read/write API | — | — | — | ✓ | ✓ |
| SSO (SAML / OIDC via GIP) | — | — | — | ✓ | ✓ |
| **Uptime SLA (99.9%)** | — | — | — | — | **✓** |
| **Support** | | | | | |
| Standard email support (24h response) | ✓ | ✓ | ✓ | — | — |
| **Priority email support (4h response)** | — | — | — | **✓** | ✓ |
| **Named CSM + onboarding concierge** | — | — | — | — | **✓** |

### 9.1 Features NOT in the matrix (disclose in pricing-page FAQ)

- **Multi-language storefronts** — future phase, not in v1
- **Multi-currency storefronts** (customer-side) — future phase, not in v1
- **Inventory transfer between Paid stores** — available in Studio and Pro; not applicable to Starter (1 store)
- **Analytics data retention** — Starter: 12 months; Studio: 24 months; Pro: forever
- **Staff permission granularity** — role-based (admin, staff, read-only); no custom role editor in v1

---

## 10. Email enforcement

### 10.1 Architecture — atomic-decrement budget pattern (TOCTOU-safe)

**The per-month quota is not computed from meter sums at send time.** That pattern has a TOCTOU race (two concurrent campaigns both pass the check, both insert, total exceeds cap). Instead:

```sql
CREATE TABLE campaign_email_budget (
    store_id   UUID         NOT NULL,
    month      DATE         NOT NULL,  -- first of UTC month
    remaining  INT          NOT NULL,
    limit_set  INT          NOT NULL,  -- plan limit snapshot
    PRIMARY KEY (store_id, month)
);
```

**Pre-send enforcement (atomic):**
```sql
UPDATE campaign_email_budget
SET remaining = remaining - :recipient_count
WHERE store_id = :store_id
  AND month = date_trunc('month', now() at time zone 'utc')
  AND remaining >= :recipient_count
RETURNING remaining;
```

If the UPDATE returns 0 rows: either the budget row doesn't exist (lazy-init it at `remaining = plan_limit`) or remaining < recipient_count → reject with 403 and upgrade message.

No race is possible: PostgreSQL's per-row locking serializes concurrent UPDATEs. The check-and-decrement is atomic.

### 10.2 Monthly meter reset

- Budget row key: `(store_id, first_of_utc_month)`
- On month roll (UTC midnight on day 1): scheduled job creates new row with `remaining = limit_set` per plan
- No rollover
- Limit updates on plan change: plan-change webhook handler recomputes `limit_set` and `remaining` inside the transaction that writes the subscription change
- Merchant-visible counter in admin: "X of Y emails used this month" (reads `limit_set - remaining`)

### 10.3 Per-store concurrent send limit

Max **3 concurrent** campaign sends per store. Enforced via Redis INCR + TTL or PostgreSQL advisory lock on `store_id`.

### 10.4 Transactional email handling

Transactional emails (order confirmations, shipping updates, password resets) go through a separate SendGrid template pipeline. **Not metered** against plan caps. Platform-wide fair-use of **100,000 transactional emails/store/month** exists for abuse detection.

### 10.5 SendGrid cost posture

| Scale | SendGrid plan | Per-merchant approx cost |
|---|---|---|
| 0–100 merchants | Essentials ($14.95) | $0.15/merchant/mo (virtually free) |
| 100–500 merchants | Pro 100k ($89.95) | $0.90–$1.80/merchant/mo |
| 500–2,000 merchants | Pro 700k ($449) | $0.22–$0.90/merchant/mo |
| 2,000+ merchants | **Migrate to Amazon SES** at ~$0.10/merchant/mo | $0.10/merchant/mo |

Trigger: **evaluate SES migration at 500 paid merchants.** Migration work: custom template system, deliverability tuning, bounce/complaint handling = 3–4 weeks.

---

## 11. Image and file caps

### 11.1 Per-product image cap

Enforced at `POST /admin/stores/:storeId/products/:productId/media`:

```go
count := r.CountMedia(productID)
limit := plangate.GetLimit(plan, FeatureImagesPerProduct)
if limit != plangate.Unlimited && count >= limit {
    return 403 { "error":"plan_limit", "feature":"images_per_product", "limit":N, "used":count }
}
```

### 11.2 Per-file size cap (all plans)

**10 MB maximum per uploaded image file.** Enforced at upload-URL issuance and rechecked server-side. Platform safety — applies identically to all plans.

---

## 12. SSO (Pro and Pro + App)

### 12.1 Protocols

- **SAML 2.0** — Okta, Azure AD, OneLogin, Ping
- **OIDC** — Google Workspace, Auth0, modern IdPs

Both leverage existing Google Identity Platform federation — not building an IdP.

### 12.2 Per-tenant config

New `tenant_sso_configs` table stores: `tenant_id PK`, `provider_type`, `metadata_url`, `issuer`, `client_id`, `client_secret_ref` (GCP Secret Manager path). Pro admin pastes their IdP metadata via admin UI.

### 12.3 Just-in-time provisioning

New employee signs in via IdP → if user record doesn't exist for this `(tenant_id, email)`, create with default `staff` role. Admin adjusts roles. No SCIM in v1.

### 12.4 Break-glass admin

Each Pro tenant has **exactly one** non-SSO Mark8ly-native admin account for emergencies:

- Password: machine-generated, **20 characters minimum**, from CSPRNG. Never user-chosen.
- Stored in GCP Secret Manager under `projects/tesserix-prod/secrets/break-glass-{tenant_id}`.
- **MFA: TOTP mandatory** (Google Authenticator or equivalent). No SMS.
- **Rotation: 90 days** OR immediately after use.
- **Use triggers immediate alert** to `#security-alerts` Slack channel + log to audit service.
- Access to GCP Secret Manager requires IAM role held by ≤2 Mark8ly staff at any time.

---

## 13. White-label mobile app add-on (Pro-only)

### 13.1 Apple "app factory" policy

Apple rejects batch-submitted apps from a single publisher. Industry workaround:

- Apps submitted under the **tenant's own Apple Developer account** ($99/yr per account, tenant-owned)
- Apps submitted under the **tenant's own Google Play Console** ($25 one-time, tenant-owned)
- Tenant provides API access via App Store Connect API keys + Google Play service account credentials
- Mark8ly handles build, submission, and updates under the tenant's accounts

### 13.2 What the $2,000 setup fee covers

- Apple + Google dev-account setup guidance
- Branded iOS + Android build (tenant icon, splash, colors, deep links)
- First submission to both stores
- Firebase push notification project per tenant
- Named CSM onboarding + 60 days post-launch issue support

### 13.3 Ongoing (included in $199/mo add-on)

- **Named Customer Success Manager (CSM)** — 1 hour/month of dedicated account management (renewal conversations, strategic guidance, app campaign help, escalation path)
- **Uptime SLA 99.9%** — service credits on breach per TOS
- **Onboarding concierge** — guided first-90-days program for app go-live
- OS-release-driven app updates (iOS/Android 2×/year)
- Push notification infrastructure
- Crash monitoring + incident response
- Tenant retains Apple $99/yr dev-account fee + Google one-time $25

### 13.4 Build toolchain — v1 decision deferred

Three options to evaluate during first deal:
- **Capacitor** (pragmatic v1, shipped in weeks)
- **React Native** (best UX, biggest investment)
- **Native Swift + Kotlin** (premium UX, highest maintenance)

**Automation threshold:** before signing the **second** Pro+App deal, build a per-tenant build automation pipeline (bundle ID parameterization, Firebase provisioning automation, credential vault per tenant).

---

## 14. Pro onboarding process

### 14.1 Pro base (no white-label app)

Lighter-touch sales process since there's no CSM commitment:

| Stage | Owner | Duration target | Deliverable |
|---|---|---|---|
| Contact form submitted | Marketing form → Notion | Immediate | Sales record, #sales-inbox notified |
| Sales-qualification email reply | Sales | 24h | Pricing confirmation, requirements check |
| Self-serve annual checkout link sent | Sales | 1 day | Stripe Checkout URL for Pro annual at quoted price |
| Tenant provisioned | Engineering | 2 business days post-payment | Pro plan active, SSO skeleton ready |
| Setup call (optional) | Sales/Support | On request | 30-min orientation |
| Time-to-value: store live on Pro | Buyer | Target 30 days from contact form | Merchant operating on Pro |

### 14.2 Pro + White-label App add-on — full concierge

Higher-touch because of app submission + CSM commitment:

| Stage | Owner | Duration target | Deliverable |
|---|---|---|---|
| Contact form submitted | Marketing form → Notion | Immediate | Sales record, #sales-inbox notified |
| Discovery call | Sales | 3 days to schedule | 45-min call; requirements captured |
| Quote issued | Sales | 2 business days post-call | Signed PDF with price, scope, setup fee, SLA, MSA link |
| MSA + DPA signed | Buyer + Sales | 2 weeks typical | DocuSign completed |
| Setup fee invoice | Finance | 1 business day | Invoice via Stripe; NET-15 terms |
| Setup fee paid | Buyer | 15 business days max | Stripe confirmation |
| Tenant provisioned | Engineering | 2 business days post-payment | Pro + App plan active |
| Onboarding call with CSM | CSM | 1 week post-provisioning | Goals set, success criteria documented |
| White-label app build | Engineering | 4–6 weeks | App live in both stores |
| Time-to-value: store configured + first order | Buyer + CSM | Target 60 days from contact form | Merchant live |

### 14.3 Procurement prerequisites (before launch)

- **MSA (Master Services Agreement) template** — drafted, legally reviewed, versioned
- **DPA (Data Processing Agreement) template** — GDPR Article 28 compliant, lists subprocessors
- **SOC 2 posture** — v1 launch without SOC 2 is acceptable for sub-$100k deals. Budget SOC 2 Type I audit ($25k–50k) when first $100k+ deal arrives.
- **Cyber liability insurance** — $1M minimum policy. Required by many enterprise buyers.

---

## 15. Cancellation flow

### 15.1 Merchant-facing

- **Entry point**: admin → Subscription → "Cancel subscription"
- **Confirmation dialog**: lists what cancellation means (access until period end, data retention 60 days post-expiry, store-closed page)
- **Exit survey** (required, 1 question + optional comment): "Why are you leaving?"
  - Radio options: *Too expensive*, *Missing features*, *Taking a break*, *Switched to competitor*, *Closed my business*, *Other*
  - Optional free-text comment
  - Submits to `cancellation_surveys` table for retention analysis
- **Save offer** (shown after survey):
  - If reason = "Too expensive" and plan ∈ {Starter, Studio}: offer 50% off for next 3 months (one-time, tracked in `promo_redemptions`)
  - If accepted: cancellation reversed (state transition `cancel_scheduled → active` per §17.2), promo applied
  - If declined: proceed to cancellation
- **Final state**: `subscription_status = cancel_scheduled` with `cancels_at = current_period_end`. Storefront remains live. Admin remains editable until `cancels_at`.

### 15.2 Post-cancellation

- At `cancels_at`: subscription transitions to `expired` (see state machine §17)
- 60-day data retention window begins (same as trial grace period)
- Add-card flow available to restore anytime in the 60-day window

### 15.3 Win-back

At day 30 post-cancellation (still in grace window): send one win-back email with:
- Confirmation of data retention deadline
- Optional 20% off for 6 months if they return

---

## 16. Failed payment / dunning flow

### 16.1 Trigger

Stripe webhook `invoice.payment_failed` arrives → subscription transitions to `past_due`.

### 16.2 Grace period + retry schedule

- **Day 0 (payment fail):** subscription `past_due`. Storefront stays live. Admin stays fully editable. First email sent: "Payment failed, please update your card."
- **Day 1, 3, 5 (Stripe Smart Retries):** Stripe auto-retries. If successful, subscription returns to `active`.
- **Day 5:** if still failing, second email: "Your payment has failed 3 times. Update your card by day 7 to keep your store live."
- **Day 7:** final retry + final email: "Last reminder — your store will go read-only tomorrow."
- **Day 8:** subscription transitions to `expired`. Admin goes read-only (allowlisted routes only, §17.3). Storefront stays live through day 14 (brand protection window).
- **Day 14:** storefront flips to "store closed" page.
- **Day 90 post-payment-fail:** hard-delete per §5.3 timeline.

### 16.3 Recovery at any point

Adding a valid card at any point before day 90 restores subscription immediately. Subscription status goes to `active`. All grace-period email reminders cancelled.

### 16.4 Dunning copy tone

Editorial/calm — not urgent/threatening. Branded with Mark8ly hairline rules. Example template:

> "Hi [first_name], your last payment didn't go through — likely an expired card. We'll retry in a few days. If you'd like to update your card now, [link]. — The Mark8ly team"

### 16.5 14-day dunning vs 14-day refund window

A merchant who's just charged (triggering the 14-day cooling-off refund window per §8.1) and then experiences a payment failure sits at an edge: their account can be both "in cooling-off window" and "in dunning." Resolution: **refund window always dominates.** A merchant in their first 14 days can request a refund regardless of dunning state; dunning is only material for charges outside the cooling-off window.

---

## 17. State transitions & concurrency

### 17.1 Subscription state enum

```go
type SubscriptionStatus string

const (
    StatusSignup           SubscriptionStatus = "signup"            // pre-email-verification
    StatusTrialing         SubscriptionStatus = "trialing"          // active 3-month trial
    StatusActive           SubscriptionStatus = "active"            // paying, happy path
    StatusPastDue          SubscriptionStatus = "past_due"          // payment failed, in retry window
    StatusCancelScheduled  SubscriptionStatus = "cancel_scheduled"  // cancelled, access until cancels_at
    StatusExpired          SubscriptionStatus = "expired"           // trial expired OR dunning failed — read-only
    StatusStoreClosed      SubscriptionStatus = "store_closed"      // 14+ days expired, storefront dark
    StatusPendingHardDelete SubscriptionStatus = "pending_hard_delete" // day 90 post-expiry, scheduled deletion
    StatusHardDeleted      SubscriptionStatus = "hard_deleted"      // terminal, billing archive only
)
```

### 17.2 Allowed transitions

```
signup → trialing (email verified)
trialing → active (card added before day 90)
trialing → expired (day 90, no card)
active → past_due (invoice.payment_failed)
active → cancel_scheduled (merchant-initiated cancel)
past_due → active (payment retry succeeds)
past_due → expired (day 8 of dunning, final retry failed)
cancel_scheduled → active (merchant reverses cancellation via save offer or add-card, §15.1)
cancel_scheduled → expired (current_period_end reached)
expired → active (card re-added during grace window)
expired → store_closed (day 14 post-expiry)
expired → pending_hard_delete (hard-delete scheduler fires direct from expired if storefront dark window already passed)
store_closed → active (card re-added during grace window)
store_closed → pending_hard_delete (day 90 post-expiry)
pending_hard_delete → hard_deleted (deletion job run)
```

No other transitions are allowed. Any other state change is a bug.

### 17.3 Read-only mode — admin allowlist

When `subscription_status ∈ {expired, store_closed}`, admin routes are blocked by a new middleware `subscription.RequireActive()` EXCEPT for:

- `GET /admin/**` (view-only)
- `POST /admin/stores/:storeId/billing/*` (add card, manage payment method)
- `POST /admin/stores/:storeId/subscription/*` (upgrade, restore)
- `GET /admin/stores/:storeId/orders/export/*` (data export)
- `POST /admin/auth/**` (sign in, sign out, password reset)

Middleware ordering: `IstioAuth → TenantMiddleware → RequireActive → RequireFeature → handler`.

Returns HTTP **402 Payment Required** with body: `{"error":"subscription_inactive","current_status":"expired","restore_url":"/admin/billing"}`.

### 17.4 Concurrency controls

Every write to `store_subscriptions` must:
1. Take a PostgreSQL advisory lock: `SELECT pg_advisory_xact_lock(hashtext(store_id::text))` at start of transaction
2. Re-read the current row after lock acquisition (no stale snapshots)
3. Execute update with CAS guard: `UPDATE ... WHERE status = :expected_status AND updated_at = :expected_updated_at` — if 0 rows affected, loudly fail and retry from step 2
4. Log state transition to `subscription_state_log` with actor (system | webhook_event_id | admin_user_id)

### 17.5 Cron must be idempotent

The daily trial-expiry and dunning-advance crons must be pure functions of time + subscription row. Re-running any cron job must produce identical state. No imperative step-through that can half-complete on pod restart.

### 17.6 Stripe webhook event catalog

**All of these must be handled:**

| Event | Purpose |
|---|---|
| `checkout.session.completed` | New subscription created via Stripe Checkout |
| `customer.subscription.created` | Subscription record created |
| `customer.subscription.updated` | Plan change, cancel-at-period-end toggle, discount application |
| `customer.subscription.deleted` | Final cancel (period end reached on `cancel_scheduled`) |
| `customer.updated` | Billing address change → may trigger §4.6 re-evaluation |
| `invoice.created` | Upcoming renewal, India annual workflow |
| `invoice.finalized` | Invoice finalized, about to auto-charge |
| `invoice.paid` | Renewal success, extend period |
| `invoice.payment_failed` | Dunning start → §16 |
| `invoice.payment_action_required` | 3DS / SCA challenge |
| `charge.refunded` | Refund confirmation |
| `payment_method.attached` | Card added → possibly exit past_due |
| `payment_method.detached` | Card removed → update UI |
| `radar.early_fraud_warning` | Fraud alert — flag subscription for review |

### 17.7 Webhook idempotency

Every inbound webhook:
1. Verify signature (existing `verifyStripeSignature`, raw body read before Gin JSON binding)
2. Reject with 400 if signature invalid
3. Extract `event.id`
4. `INSERT INTO stripe_webhook_events (event_id, ...) ON CONFLICT (event_id) DO NOTHING` — if conflict, load existing row; if `processed_at IS NOT NULL`, return 200 (replay)
5. Process event within advisory-locked transaction on the relevant `store_id` (or advisory lock on `event_id` if store unresolvable yet)
6. Update `stripe_webhook_events.processed_at = now()` in same transaction
7. Return 200

Schema:
```sql
CREATE TABLE stripe_webhook_events (
    event_id      VARCHAR(100) PRIMARY KEY,
    event_type    VARCHAR(100) NOT NULL,
    store_id      UUID,  -- nullable for events not yet linked
    payload       JSONB       NOT NULL,
    received_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    processed_at  TIMESTAMPTZ,
    processing_error TEXT
);
```

Orphan-handling for nullable `store_id`: unlinked events must be linked within one processing attempt. Any event unlinked after processing is flagged for manual review. Advisory lock taken on `event_id` in orphan cases to prevent replay.

### 17.8 Idempotency keys on outbound Stripe calls

Every Stripe mutating call passes `Idempotency-Key`:

- Create customer: `customer:{store_id}`
- Create checkout session: `checkout:{store_id}:{plan}:{period}:{day_bucket}`
- Create subscription: `subscription:{store_id}:{plan}:{billing_period}`
- Create portal session: `portal:{store_id}:{hour_bucket}`
- Create refund: `refund:{invoice_id}`

Keys are server-generated (never caller-supplied).

---

## 18. Security & compliance requirements

### 18.1 PCI-A scope rules (hard constraints)

Using Stripe Checkout (hosted) + Customer Portal keeps Mark8ly in SAQ-A. Forbidden patterns:

- ✗ Do not store full card number, CVV, expiry date, or PAN anywhere
- ✗ Do not log raw Stripe API responses or webhook payloads after processing
- ✗ Do not pass card details through Mark8ly API routes — all card collection on Stripe's hosted page
- ✓ If displaying "card ending in 4242" UX, fetch `card.last4` from Stripe's PaymentMethod API; do not store locally
- ✓ `store_subscriptions` stores only: `stripe_customer_id`, `stripe_subscription_id`, `card_fingerprint`, no card data

### 18.2 Multi-tenant data isolation (BLOCKER fix)

**Existing defect in `subscription.Repository.GetByStoreID`**: query filters on `store_id` only, missing `tenant_id`. An authenticated user from Tenant A with knowledge of Tenant B's store UUID could call `GET /admin/stores/{tenant-b-store-id}/subscription` and receive Tenant B's Stripe customer ID.

**Fix:** every subscription repository query double-keyed on `(tenant_id, store_id)`. Signature becomes:

```go
GetByStoreID(ctx context.Context, db *gorm.DB, tenantID, storeID uuid.UUID) (*StoreSubscription, error)
```

Call sites that extract `tenant_id` from `AuthContext` must pass both. Handler-level audit required as part of implementation.

### 18.3 Stripe secret key management

| Secret | Location | Rotation |
|---|---|---|
| Mark8ly's Stripe AU secret key (live) | GCP Secret Manager `/projects/tesserix-prod/secrets/stripe-au-secret-key-live` | 90 days OR staff-leave-triggered |
| Mark8ly's Stripe AU secret key (test) | `/projects/tesserix-dev/secrets/stripe-au-secret-key-test` | Loose; refresh quarterly |
| Stripe webhook endpoint secret (live) | `/projects/tesserix-prod/secrets/stripe-au-webhook-secret-live` | Rotate whenever webhook URL changes |
| Merchant-provided Stripe/Razorpay keys (BYO gateway) | `/projects/tesserix-prod/secrets/merchant/{tenant_id}/{provider}-secret` | Merchant-controlled; deleted on hard-delete |

Access:
- Workload Identity Federation (WIF) only — no service account JSON keys on pods
- GCP Secret Manager audit log monitored; alert on unexpected `accessSecretVersion` calls
- Access to GCP Secret Manager requires named IAM role held by ≤3 Mark8ly staff

### 18.4 Enterprise API key security (Pro only)

- Keys: 32 bytes of entropy, base64url encoded, prefixed `mk8_live_` (prod) or `mk8_test_`
- Storage: bcrypt or argon2 hash; prefix-only stored plaintext for UI display
- Shown exactly once at creation
- **Per-key scopes**: `read:orders`, `write:orders`, `read:products`, `write:products`, etc.
- **Per-key rate limit**: default 1,000 req/min per key; configurable per contract
- **Tenant binding**: each key is bound to `(tenant_id, store_id)`
- **Revocation**: immediate, via admin UI
- **Key rotation**: merchant can rotate anytime; old key works for 24h for zero-downtime rotation, then invalidated

### 18.5 Webhook endpoint hardening

- **Rate limit at Istio VirtualService**: 100 req/min per source IP on the webhook path
- **Body size cap**: `http.MaxBytesReader` at 512 KB
- **Event type allowlist**: after signature verification, `event.type` validated against §17.6 allowlist; unknown types logged + 200-accepted
- **No body logged in full** at any error level

### 18.6 Log sanitization

- Stripe API error bodies: parse and log `error.code` + `error.type` only; never full body string
- Webhook payloads: log `event.id` and `event.type` only; never raw payload
- Customer email, billing address, GSTIN/VAT: redact from any error log

### 18.7 Account recovery

- Self-service card-add during grace window: allowed
- Post-hard-delete recovery: requires email verification + director-level approval ticket
- Support staff cannot restore hard-deleted tenants without two-person approval

### 18.8 Geo-pricing anti-arbitrage (VPN mitigation)

Because emerging-market pricing is ~33% below developed-market pricing, the price gap creates a VPN-based arbitrage vector (merchant spoofs IP/billing to get India pricing while actually located in the US).

**Mitigation triangulation at subscription creation:**

1. **Card-country check** — Stripe returns `card.country` on every PaymentMethod. If card was issued in a developed market but billing address is in an emerging market, apply **developed-market pricing** (upgrade price) unless merchant manually requests review.
2. **IP + billing + card triangulation** — all three logged at subscription creation. Mismatch does NOT auto-block (legitimate cross-border cases: diaspora merchants, expats, holding companies); mismatch IS flagged for audit review in `subscription_arbitrage_audit` table.
3. **Quarterly anti-arbitrage audit** — internal report on price-tier vs card-country mismatches; spot-check flagged accounts.
4. **Edge case handling** — if card-country and billing-country disagree, billing-country wins for pricing BUT `subscription_arbitrage_flag = true` is set for later review. Blatant fraud (e.g., prepaid EUR card + IN billing + US IP) gets manual escalation.

Adds ~2 days to implementation plan. Acceptable cost.

---

## 19. Tax compliance — B2B reverse charge

### 19.1 Model

Mark8ly sells B2B SaaS **only to registered business entities** in 18 countries. No consumer customers.

This permits the **reverse-charge mechanism** in EU, UK, India, and most of SEA where buyer is VAT/GST-registered:
- Mark8ly does NOT collect VAT/GST on sale
- Buyer (merchant) self-accounts for VAT/GST in their jurisdiction
- Mark8ly's invoice is annotated with "Reverse charge applicable under Article 196 of EU VAT Directive" (or local equivalent)
- Mark8ly is NOT required to register as foreign VAT/GST payer in most jurisdictions — **as long as B2B-only is strictly enforced**

### 19.2 Enforcement

- Signup form requires valid business tax ID (per §5.2)
- Tax ID validated in real-time against country registry
- Consumer/individual signups rejected at form level
- Invoices issued include buyer's tax ID + reverse-charge annotation

### 19.3 Per-country specifics

| Country | Tax type | Validator | Reverse charge | Fallback if validation fails |
|---|---|---|---|---|
| US | None federal; state nexus eventually | EIN format check + **legally-binding business-entity checkbox** (§19.3.1) | N/A | Accept if checkbox signed |
| CA | GST/HST 5-15% | Business Number format check + **legally-binding business-entity checkbox** | Yes for registered GST/HST | Accept if checkbox signed |
| UK | VAT 20% | HMRC VAT API | Yes for B2B | Provisional 48h revalidation |
| Ireland + EU (DE/FR/IT/ES/NL) | VAT 19-25% | VIES | Yes for B2B | Provisional revalidation |
| Australia | GST 10% — Mark8ly AU entity must charge | ABN Lookup | N/A (domestic) | Strict, block |
| NZ | GST 15% | IRD lookup | Yes for B2B *if counsel confirms pre-launch* | Provisional |
| India | GST 18% under OIDAR | GSTN API | Yes for B2B | Provisional; RBI mandate rules §4.7 |
| Singapore | GST 9% | ACRA check | Yes for B2B | Provisional |
| Malaysia | SST 8% | MOF SST registry | Partial — monitor | Provisional; **5-biz-day manual-review fallback** if API unavailable |
| Thailand | VAT 7% | RD API | Yes for B2B | Provisional; **5-biz-day manual-review fallback** if API enrollment blocked |
| Philippines | VAT 12% | BIR lookup | Yes for B2B | Provisional; **5-biz-day manual-review fallback** if API enrollment blocked |
| Indonesia | VAT 11% | DJP NPWP | Yes for B2B | Provisional; **5-biz-day manual-review fallback** if API enrollment blocked |
| Vietnam | VAT 10% | GDT lookup | Yes for B2B | Provisional; **5-biz-day manual-review fallback** if API unavailable |

### 19.3.1 US/CA legally-binding business-entity checkbox

Since US has no federal tax-ID validator and CA's Business Number lookup is not authoritative for B2B status, the signup form for these countries must include a **legally-binding checkbox**:

> "☐ I confirm that the purchase is being made by a registered business entity (not an individual consumer) and I have the authority to enter into this agreement on behalf of the business. I understand this affirmation is legally binding and used by Mark8ly for tax-reporting purposes."

This checkbox is archived with the signup record and referenced in invoices. Legal review confirms this meets reverse-charge B2B declaration requirements in US/CA.

### 19.4 Australia-specific (Mark8ly Pty Ltd is AU entity)

For Australian merchants, reverse charge does NOT apply — Mark8ly charges 10% GST and remits to ATO. AU GST is a domestic obligation regardless of B2B status. Pricing page for AU shows **GST-exclusive** with "Plus GST" shown below the price card; invoice breaks out GST separately.

### 19.5 Quarterly revalidation

Scheduled job re-checks tax IDs against registries quarterly. If invalid → email merchant, allow 14 days to update; if not updated → subscription pauses billing until resolved.

### 19.6 Pre-launch tax counsel confirmation

Required before launch per §20:
- EU/UK/India reverse-charge applicability for B2B SaaS
- NZ specifically — reverse-charge applicability under IRD guidance (flagged by solution architect as non-obvious)
- AU GST registration status confirmed

---

## 20. Legal & TOS requirements

### 20.1 TOS must include

- 14-day cooling-off refund terms with precise start-point
- Non-refundability of Pro + White-label app add-on setup fee
- Jurisdictional notices for all 18 countries
- Subprocessor list (Stripe, SendGrid, GCP, Firebase, Cloudflare, OpenFGA if external)
- GDPR Articles 13/14 disclosure (Mark8ly as processor + controller dual role)
- India DPDP Act: grievance officer designation, consent-based processing statement
- Right-to-erasure exemption for billing records (legal basis: AU Income Tax Assessment Act s 382-5 — 5-year retention; EU VAT Article 242 — 10-year retention)
- Uptime SLA definition for Pro + White-label App (99.9% measured over calendar month; service credits on breach)
- Acceptable Use Policy (prohibit spam, illegal goods, copyright infringement)
- Legally-binding business-entity attestation for US/CA reverse-charge basis (§19.3.1)
- AU GST inclusivity disclosure for AU merchants

### 20.2 DPA template (Pro + App)

GDPR Article 28 compliant. Pro + App customers sign DPA as part of MSA. Template drafted separately, legally reviewed.

### 20.3 Pre-launch legal checklist

- [ ] TOS drafted and legally reviewed (AU, UK, EU, IN counsel)
- [ ] DPA template drafted
- [ ] Cookie policy + GDPR consent banner for pricing page
- [ ] Privacy Policy aligned with DPDP Act + GDPR
- [ ] MSA template drafted for Pro + App
- [ ] Cyber liability insurance $1M minimum procured
- [ ] NZ tax counsel confirmation on reverse-charge applicability
- [ ] EU/UK/India tax counsel confirmation on reverse-charge registration thresholds

### 20.4 Post-launch legal work (not blockers)

- SOC 2 Type I audit (budget $25–50k, trigger at first $100k+ deal)
- EU/UK tax counsel confirmation of reverse-charge application at 100 merchants/country
- India GST OIDAR registration evaluation when Indian monthly revenue crosses ₹20 lakh

---

## 21. Observability requirements

### 21.1 Metrics (Cloud Monitoring custom)

- `subscription.state.count{status}` — gauge, per status
- `subscription.mrr_usd` — gauge, computed from active subscriptions with USD normalization
- `subscription.trial.expired_today` — counter
- `subscription.payment_failed` — counter
- `subscription.arbitrage_flagged` — counter (§18.8)
- `campaign.email.sent{store_id}` — counter (for fair-use detection)
- `webhook.processed{event_type}` — counter
- `webhook.failed{event_type, reason}` — counter

### 21.2 Logs (structured, JSON)

Every subscription state transition logs:
```json
{
  "event": "subscription_state_transition",
  "store_id": "...",
  "tenant_id": "...",
  "from_status": "trialing",
  "to_status": "active",
  "actor": "webhook:evt_ABC...",
  "plan_before": "trial",
  "plan_after": "starter",
  "timestamp": "..."
}
```

### 21.3 Alerts (Cloud Monitoring)

- **Trial scheduler dead-man's-switch**: page if trial-expiry cron has not run in 25 hours
- **Failed payment spike**: alert if `invoice.payment_failed` volume > 5% of active subscriptions in 24h window
- **Webhook processing latency**: alert if P95 > 5s
- **Webhook failure rate**: alert if >1% failures in 1h window
- **Trial signup anomaly**: alert if >50 trial signups/day
- **Break-glass admin use**: immediate Slack alert to `#security-alerts`
- **Arbitrage flag spike**: alert if `subscription_arbitrage_flag` increments >5× baseline

### 21.4 Dashboards

Cloud Monitoring dashboard "Subscription Health":
- MRR gauge
- Active subscription count by plan
- Today's failed payments vs 7-day average
- Upcoming trial expirations (next 7 days)
- Webhook processing latency + error rate

---

## 22. Disaster recovery

### 22.1 CNPG (Cloud Native Postgres)

Mark8ly runs CNPG in `mark8ly-postgres` namespace. Backup posture:

- **Point-in-time recovery (PITR)**: enabled on `mark8ly-postgres` cluster. 7-day window.
- **Daily snapshots**: to GCS bucket `tesserix-mark8ly-backups` via CNPG `backup` resource. 30-day retention.
- **Subscription-critical tables exported daily**: `store_subscriptions`, `stripe_webhook_events`, `promo_redemptions`, `billing_archive` via Cloud Scheduler → `pg_dump` → GCS. 90-day retention.
- **No cross-region replica in v1** (cost-prohibitive). Revisit at $1M ARR.

### 22.2 Recovery scenarios

| Scenario | RTO | RPO | Mechanism |
|---|---|---|---|
| Single pod restart | 30s | 0 | Knative auto-restart |
| CNPG primary failure (w/ standby, ≥2k merchants) | 2 min | 0 | CNPG failover |
| CNPG primary failure (<2k merchants, no standby) | 4h | 24h | GCS snapshot restore |
| Accidental table drop | 1h | 5 min | PITR restore |
| Full cluster loss | 4h | 24h | GCS snapshot restore |
| Data-center loss | 24h+ | 24h | GCS snapshot → new cluster in new region |

RTO at the sub-2000-merchant tier assumes single-instance CNPG. Adding a read standby at 100 merchants (see §24) brings RTO down to ~2 min.

### 22.3 Stripe as reconciliation source

If `store_subscriptions` is fully lost, Stripe subscription + customer objects can reconstruct most state. `stripe_subscription_id` + `stripe_customer_id` are reconciliation keys.

**Important: `promo_redemptions` and `refund_audit` are NOT reconstructible from Stripe alone** (internal Mark8ly data). GCS backups are the only recovery path for those. Documented in runbook.

---

## 23. Audit logging & billing archive

### 23.1 Subscription mutations audit

Every write to `store_subscriptions` emits a structured event to the existing `audit-service`:

- Subscription created
- Plan upgraded / downgraded
- Status changed
- Card added / changed / removed
- Refund issued
- Promo code applied
- Hard-delete scheduled
- SSO config changed (Pro / Pro + App)
- White-label App add-on purchased or cancelled

### 23.2 Billing archive — 7-year retention

Billing records survive subscription hard-delete:

```sql
CREATE TABLE billing_archive (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    original_store_id    UUID NOT NULL,
    original_tenant_id   UUID NOT NULL,
    business_name        VARCHAR(500) NOT NULL,
    tax_id               VARCHAR(50),
    tax_id_country       CHAR(2),
    billing_country      CHAR(2),
    stripe_customer_id   VARCHAR(100) NOT NULL,
    all_invoices         JSONB NOT NULL,
    total_revenue_usd    NUMERIC(12,2),
    hard_deleted_at      TIMESTAMPTZ NOT NULL,
    archive_expires_at   TIMESTAMPTZ NOT NULL
);
```

### 23.3 GDPR erasure reconciliation

- Live-table PII purged per standard delete flow
- `billing_archive` PII retained under legal-obligation basis (tax law)
- Stripe customer deleted via `DELETE /v1/customers/:id`
- Merchant notified in writing which data was retained under which legal basis

---

## 24. Database sizing — CNPG staircase

`mark8ly-postgres` CNPG cluster resource profile by merchant count:

| Merchant count | CPU request | Memory request | Storage | Standby | Notes |
|---|---|---|---|---|---|
| 0–100 | 0.5 CPU | 2 GiB | 50 GiB | No | Current v1 sizing; single instance (4h RTO) |
| 100–500 | 1 CPU | 4 GiB | 200 GiB | No | PgBouncer connection pool added |
| 500–2,000 | 2 CPU | 8 GiB | 500 GiB | **Yes (sync standby)** | RTO drops to 2 min with standby |
| 2,000–10,000 | 4 CPU | 16 GiB | 1 TiB | Yes | WAL archiving tuned |
| 10,000+ | Evaluate horizontal partitioning | 32+ GiB | 2+ TiB | Yes + async replica | Multi-tenant sharding or Cloud Spanner evaluation |

**Upgrade triggers:**
- >80% CPU utilization sustained 1 hour
- >80% memory utilization sustained 1 hour
- Query latency P95 > 500ms
- Approaching 80% of storage

---

## 25. Implementation gaps

| Area | Rough effort |
|---|---|
| **Backend — plans & billing** | |
| Update `plangate.go` to 4-plan matrix per §9 | 1 day |
| Add new migration: tax ID fields, enum rename, add-on tracking | 1 day |
| Fix `GetByStoreID` to require `tenant_id`; audit all call sites | 2 days |
| Stripe Billing subscription flow (Checkout + Customer Portal) | 1 week |
| Stripe webhook dispatcher (§17.6, §17.7) | 1 week |
| State machine + advisory locking + idempotency keys | 4 days |
| Plan upgrade/downgrade endpoints + Stripe proration + excess-store downgrade block | 4 days |
| Campaign email budget pattern + atomic decrement (§10.1) | 3 days |
| Read-only middleware + allowlisted routes (§17.3) | 2 days |
| Trial expiry + dunning crons (idempotent, §16, §17.5) | 1 week |
| RBI e-mandate fallback flow (§4.7) | 4 days |
| Promo codes + abuse prevention (§7.3) | 1 week |
| Refund flow (v1: manual via Stripe Dashboard + `refund_audit` table) | 2 days |
| Store-closed page at Cloudflare Worker layer (§5.4) | 3 days |
| Tax ID validation per country (12 validators, §19.3) | 2 weeks |
| US/CA legally-binding business-entity checkbox (§19.3.1) | 1 day |
| Reverse-charge invoice template + logic (§19) | 1 week |
| Billing archive table + hard-delete hook (§23.2) | 2 days |
| Geo-pricing anti-arbitrage triangulation (§18.8) | 2 days |
| White-label App add-on handling (purchase flow, CSM assignment) | 3 days |
| **Backend — shipping** | |
| Add IE, NZ to `ShipEngineCarrier.SupportedCountries()` | 1 day |
| Add VN to `NinjaVanCarrier.SupportedCountries()` | 1 day |
| AE / Aramex new carrier (deferred to post-v1) | 1 week |
| **Frontend — admin** | |
| Pricing page (geo-localized, 18 countries with PPP + visible Pro floor) | 1 week |
| Plan management UI (upgrade/downgrade, billing period switch) | 1 week |
| Cancellation flow with survey + save offer | 3 days |
| Email usage counter in campaigns UI | 1 day |
| Tax ID field in onboarding wizard + 14-day reminder banners | 3 days |
| Failed-payment banner + add-card flow | 2 days |
| Contact-sales form for Pro (Notion integration + Slack notification) | 2 days |
| White-label App add-on purchase flow (Pro-only) | 2 days |
| **Security + compliance** | |
| PCI-A forbidden-pattern code audit | 2 days |
| GCP Secret Manager paths + WIF wiring | 2 days |
| Break-glass admin provisioning + TOTP + rotation cron | 3 days |
| Enterprise API key management (Pro-only, §18.4) | 1 week |
| Webhook endpoint hardening (Istio + body cap + log sanitize) | 2 days |
| **Observability** | |
| Subscription state custom metrics | 2 days |
| Cloud Monitoring dashboard | 1 day |
| Alerts wiring (§21.3) | 2 days |
| **SSO (Pro / Pro + App)** | 2-3 weeks |
| **White-label mobile app (first deal)** | 4-6 weeks |

**Core Starter + Studio + Trial self-serve + Pro contact-sales (no app):** ~6 weeks back-end + ~2.5 weeks front-end = **~8–10 engineer-weeks for v1 self-serve launch**.

**Pro + App add-on:** adds ~4-6 weeks for first deal (with SSO and app build).

---

## 26. Risks & open questions

### 26.1 Pre-launch blockers

1. **Legal review of TOS, Privacy, DPA** — required in AU, UK, EU, IN. Estimated $15–30k or equivalent staff lawyer time.
2. **Stripe India RBI e-mandate** for annual Indian subscribers (§4.7) — validate flow pre-launch with a test Indian card.
3. **Tax ID validators for 12 countries** — build per §19.3. SEA validator APIs may require enrollment with finite lead times.
4. **Cyber liability insurance** — $1M minimum policy required before first Pro + App deal.
5. **NZ reverse-charge tax counsel opinion** — specific ambiguity in NZ GST for cross-border B2B SaaS.

### 26.2 Strategic risks

1. **SendGrid cost at 500+ merchants** — SES migration evaluated before this scale trigger.
2. **Stripe AU acquiring-rate gap for US/UK cards** — 1-3% lower authorization rate. Revisit at $500k ARR if merchant mix skews US/UK.
3. **White-label mobile app build automation** — critical before second Pro+App deal.
4. **Apple app-factory policy tightening** — if Apple rejects current workaround, fallback is a master Mark8ly Merchant app with tenant switching.
5. **Trial abuse despite gates** — add card-bin verification if v1 gates prove insufficient.
6. **Geo-pricing arbitrage** — §18.8 triangulation catches most, but diaspora/expat cases will require manual review. Accept as operational overhead.

### 26.3 Open questions

1. **SOC 2 posture timing** — budget for Type I audit ($25–50k) trigger on first $100k deal or 18 months whichever first?
2. **Pro tier discovery-call tooling** — Calendly + Notion + Zoom as the default.
3. **Grandfathered launch rate** — running this campaign at v1 launch? If yes, how many slots?

### 26.4 Out of scope

- Marketplace plan features (placeholder only, future phase)
- Payment gateway aggregation (Mark8ly is NEVER an aggregator; merchants BYO keys always)
- Customer-facing discounts on merchant storefront (separate product — `coupons`, `gift_cards`)
- Multi-language storefronts
- Multi-currency (customer-side) storefronts
- WhatsApp Business API integration for support (future)
- AI-powered merchant tooling (future)

---

## 27. Decision log

| Decision | Rationale |
|---|---|
| 3-month trial (not 14 or 30) | Merchants take real time to launch. Generous trial attracts serious merchants; conversion quality > rate. |
| No card at signup | Editorial/calm positioning, low funnel friction, trusts product to earn the card by day 60. |
| B2B-only, reverse-charge tax model | Eliminates Mark8ly VAT/GST registration burden in 12+ jurisdictions while remaining fully compliant. Matches reality (merchants are businesses). |
| 4-tier pricing (not 3 or 5) | Trial + Starter + Studio + Pro covers indie → growing → mid-market. Studio solves the $19→$299 cliff flagged in round 1 review. |
| $99 Pro base (not $149 or $299) | CSM + SLA moved to add-on (where they belong with the white-label app customer); Pro base accessible for mid-market brands wanting features without full enterprise treatment. Unit economics remain positive at $99. |
| Pro visible price floor ($99) | Self-qualifies prospects pre-discovery call; preserves negotiation ceiling; matches Tapcart / Shopify Plus transparency norms. |
| White-label App add-on bundles CSM + SLA | Aligns dedicated human cost with the customer who needs and values it (app submission coordination, OS update windows, push campaign strategy). |
| PPP discount (~33%) for IN / MY / TH / PH / ID / VN | Accessibility to emerging-market merchants. Industry standard (Figma, Notion, Canva, Spotify). Without PPP, Starter was 3% of revenue for typical indie Indian DTC brand — unaffordable. |
| USD parity for US / CA / UK / IE / EU / AU / NZ / SG | Developed-market willingness-to-pay supports USD parity; PPP discount would leave revenue on the table. |
| Zero transaction fee | Genuinely differentiated from Shopify. BYO gateway matches "your store, your money" brand. |
| Flat monthly model | Matches editorial positioning; no per-transaction drip. Rejected GMV-tier model on brand grounds (v2.1 conversation). |
| 14-day cooling-off refund | Legally bulletproof in 18 countries; practically irrelevant post-90-day trial; simple accounting. |
| Pro contact-sales with visible floor | Best-of-both: self-qualification + flexibility. |
| White-label App setup fee $2,000 non-refundable | Covers real work (Apple + Google submission labor); self-qualifies serious buyers. |
| Mobile app via tenant-owned dev accounts | Apple's app-factory policy. Industry standard. |
| 25 images/product Starter, 50 Studio, ∞ Pro | Per-product cap merchant-intuitive, upgrade nudge at high-intent moment. |
| 2 stores Starter, 5 Studio, 10 Pro | Meaningful multi-store on Studio; Pro genuinely for multi-brand operators. Excess-store downgrade blocked (not auto-suspended). |
| Unlimited staff on all plans | Mark8ly is not Slack. Staff scale ≠ value signal. |
| 5k/15k/50k campaign email caps | Tight enough for margin; realistic at typical utilization; splits marketing from transactional. |
| Custom CSS in Studio+ (not Starter) | Real support burden; genuinely higher-value request. |
| SSO in Pro only | Gate criterion for enterprise procurement. |
| Monthly→annual immediate prorate; annual→monthly at renewal | Standard SaaS pattern; Stripe supports natively. |
| Country change at next renewal (not immediate) | Avoids mid-period repricing confusion. |
| Cancellation survey + save offer | Retention lever + product signal. |
| 14-day tax-ID validation hard deadline | Closes attack surface where unvalidated stores run live for 90 days on fake tax IDs. |
| US/CA legally-binding business-entity checkbox | Closes B2B enforcement gap where no federal validator exists. |
| 5-biz-day manual-review fallback for SEA validators | Handles API enrollment delays without indefinite provisional state. |
| Geo-pricing anti-arbitrage via card-country triangulation | Prevents exploitation of the ~33% PPP gap without blocking legitimate expats/diaspora. |
| Single Stripe AU account for all 18 countries | Operational simplicity; revisit at $500k ARR if US/UK mix >40%. |

---

## 28. Success criteria (annotated by test type)

| # | Criterion | Test type |
|---|---|---|
| 1 | New merchant signs up → email verified → reCAPTCHA pass → tax ID validated → trial store live | Integration (e2e) |
| 2 | Trial merchant on day 91 without card: admin read-only, storefront "closed" page | Integration (time-mocked) |
| 3 | Trial merchant on day 91 adds card: subscription `active`, admin + storefront fully restored | Integration |
| 4 | Merchant upgrades Starter → Studio mid-month: immediate prorate, Studio features active | Integration |
| 5 | Merchant downgrades Studio → Starter with 5 stores: blocking dialog appears, downgrade prevented until resolved | Integration |
| 6 | Merchant cancels annual in month 7: access until month 12, no refund, cancellation survey captured | Integration |
| 7 | Pro contact form submission → Notion record + Slack notification within 60s | Integration |
| 8 | First payment fails: `past_due`, 3 Stripe retries, 3 email reminders, day 8 read-only | Integration |
| 9 | Campaign send at 14,001 emails on Starter in month: blocked with upgrade message | Integration |
| 10 | Create 11th product media on Starter: blocked with upgrade message | Integration (boundary) |
| 11 | Tenant A cannot read Tenant B's subscription via URL manipulation | **Security test (required)** |
| 12 | Stripe webhook replay: idempotent, no double-processing | **Security test** |
| 13 | 14-day cooling-off refund: Stripe Dashboard refund + `refund_audit` row created | Manual UAT |
| 14 | Indian annual subscriber: invoice-based flow with UPI/NetBanking link | Manual UAT (requires Indian test card) |
| 15 | Break-glass admin login: TOTP required, alert fires to #security-alerts | Integration |
| 16 | Pricing page displays localized prices for all 18 countries with PPP correct | Manual UAT |
| 17 | Support team can issue refund via Stripe Dashboard with eligibility check | Operational runbook |
| 18 | MRR metric updates within 60s of subscription state change | Integration |
| 19 | Trial scheduler dead-man's-switch fires if cron skipped | Chaos/SRE test |
| 20 | B2B-only enforced: US/CA signup rejected without business-entity checkbox | Integration |
| 21 | Hard-delete → `billing_archive` row persisted with 7-year retention | Integration |
| 22 | Tax-ID unvalidated at day 14: admin read-only, storefront unpublished | Integration (time-mocked) |
| 23 | India merchant with card issued in US: pricing resolves to developed-market tier + arbitrage flag set | Integration |
| 24 | Merchant reverses cancellation via save offer: state transitions cancel_scheduled → active | Integration |

---

## 29. Next step

After user approval of this spec, proceed to **implementation plan** via `writing-plans` skill. The plan will decompose §25's gap list into atomic, sequenced tasks with tests-first structure and atomic commit boundaries, targeting ~8–10 engineer-weeks for v1 Starter + Studio + Trial self-serve launch, plus ~4–6 weeks for the first Pro + App customer.
