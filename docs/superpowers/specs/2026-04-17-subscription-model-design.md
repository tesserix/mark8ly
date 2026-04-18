# Mark8ly Subscription Model — Design

**Status:** v2.3 — final polish — Council APPROVED (GPT-5.4 + Gemini 3.1 Pro + Claude Opus 4.6 + Grok 4.20) + round 4 specialist review + Codex critique. Implementation-ready **pending NZ tax counsel confirmation** (see §20.3 critical path).
**Date:** 2026-04-17 (revised 2026-04-18)
**Scope:** Final pricing, plans, trial mechanics, billing infrastructure, feature matrix, tax model, enforcement rules. Includes state machine, concurrency controls, security requirements, observability, DR, and credential management for Pro + White-label App.

---

## 1. Summary

Mark8ly ships a **B2B-only** subscription model across 18 countries with **native local-currency billing** and PPP-adjusted pricing for emerging markets:

- **Trial** — 3 months free, no card required, email-verified + reCAPTCHA gated, default on signup
- **Starter** — from $19/mo (developed markets) / ₹999 (India PPP), self-serve, 2 stores
- **Studio** — from $49/mo / ₹2,499 (India), self-serve, 5 stores, custom CSS, 50k emails, 12-month audit retention, read-only API
- **Pro** — **from $1,188/yr ($99/mo equivalent), billed annually**; monthly available on request at +20% premium ($119/mo); ₹14,399 annual in India. Contact-sales with visible floor. 10 stores, SSO, full R/W API, custom code injection, priority support, unlimited.
- **Pro + White-label mobile app add-on** — $199/mo + $2,000 non-refundable setup; Pro-only; bundles named CSM + SLA + onboarding concierge + branded iOS/Android apps under tenant's developer accounts
- **Marketplace** — placeholder in code, hidden from UI

Mark8ly charges zero transaction fees on merchant GMV. Mark8ly's own billing runs through a single Stripe Australia account with **`currency_options`-based multi-currency Price objects** (8 Price objects total: 4 plans × 2 billing periods). Each merchant is charged in their own local currency from signup through the billing term.

B2B-only enforcement via tax ID validation at onboarding; 14-day hard window before storefront publishes; SEA manual-review clock pauses at queue entry. Reverse-charge tax model eliminates Mark8ly VAT/GST registration burden in 12+ jurisdictions.

---

## 2. Existing infrastructure audit

| Artifact | Location | Status | Gap |
|---|---|---|---|
| Feature matrix | `services/marketplace-api/internal/plangate/gate.go` | 22 features × 5 plans | Rewrite to 4 plans per §9 |
| Subscription model | `services/marketplace-api/internal/subscription/models.go` | `StoreSubscription` struct | Add: `ReverseChargeTaxID`, `TaxIDCountry`, `TaxIDValidated`, `TaxIDValidatedAt`, `BillingCurrency`, `PriceTier`, `HasWhiteLabelAppAddOn`, `ArbitrageFlag`, `AppLifecycleStatus` |
| Subscription repository | `services/marketplace-api/internal/subscription/repository.go` | `GetByStoreID` missing `tenant_id` | **SECURITY FIX:** require `tenant_id` parameter; audit all call sites |
| Migration | `000015_subscriptions.up.sql` | Base table exists | Add migration: tax ID fields, plan enum rename, `billing_currency`, `price_tier`, add-on tracking, `arbitrage_flag`, `app_lifecycle_status`; new tables (§17.7 `stripe_webhook_events`, §18.8 `subscription_arbitrage_audit`, §19.3.1 `business_entity_attestations` with `REVOKE DELETE`, §23.2 `billing_archive`, §10.1 `campaign_email_budget`, §13.5 `white_label_app_lifecycle`) |
| Stripe client | `services/marketplace-api/internal/payment/stripe.go` | Partial, idempotency missing, error-body logging leaks PII | Add multi-currency `currency_options` Price objects, Checkout, Customer Portal, webhook dispatcher; fix log sanitization; idempotency keys; `tax_behavior: exclusive` for AU |
| Admin subscription handler | `services/marketplace-api/internal/handlers/admin/subscription.go` | Exists | Update DTOs for 4-plan model; plan-switch, upgrade, cancel, close-before-downgrade endpoints |
| Shipping carriers | `services/marketplace-api/internal/shipping/{shipengine,delhivery,ninjavan}.go` | 15 countries | Add IE, NZ to ShipEngine; VN to NinjaVan; defer AE (Aramex) |
| Auth | GIP + OpenFGA | Multi-tenant; SAML/OIDC available | Per-tenant SSO for Pro (§12) |

---

## 3. Plan lineup

### 3.1 Tiers

| Plan | Visibility | Pricing (developed markets) | Billing | Audience |
|---|---|---|---|---|
| **Trial** | Default on signup | $0 for 90 days | N/A | All B2B signups |
| **Starter** | Pricing page, self-serve | $19/mo or $182/yr (20% off) | Monthly or annual | Single-brand, up to 2 stores |
| **Studio** | Pricing page, self-serve | $49/mo or $470/yr (20% off) | Monthly or annual | Growing merchants, up to 5 stores |
| **Pro** | Pricing page with **"From $1,188/yr ($99/mo equivalent), billed annually"**, "Contact sales" CTA | **$1,188/yr annual baseline ($99/mo equivalent); monthly on request at +20% premium ($119/mo)** | Annual preferred; monthly available with premium | Mid-market, up to 10 stores |
| **Pro + White-label mobile app add-on** | Pro-only add-on, sales-quoted | +$199/mo + $2,000 non-refundable setup | Annual only, co-terminates with Pro renewal | Brands wanting branded apps; includes named CSM + SLA + onboarding concierge |
| **Marketplace** | Hidden in UI | TBD | TBD | Future multi-operator |

**Monthly Pro premium rationale (Council finding):** without the +20% monthly premium, monthly Pro at $99/mo becomes cheap SSO/API access for 1-month bursts before churn. $119/mo monthly vs $99/mo annual-equivalent matches the Starter/Studio +20%-for-monthly convention.

### 3.2 Pricing ladder rationale

```
$19  →  $49  →  $99  →  $298 (Pro + App)
      2.6×      2.0×    3.0×
```

### 3.3 Pro "contact sales" with visible floor

Pro card on pricing page displays:
- Feature list (SSO, multi-store, full R/W API, custom code injection, priority support)
- **"From $1,188/yr ($99/mo equivalent), billed annually. Monthly available at $119/mo."** — explicit annual-first framing
- Primary CTA: "Contact sales"
- Secondary CTA: "Download Pro brief"

**Pricing-page FAQ entry** explicitly distinguishes Pro priority support from CSM-level engagement: "Pro customers receive priority email support (4-hour response). Dedicated Customer Success Manager (CSM), uptime SLA, and onboarding concierge are included with the White-label App add-on for brands that want dedicated account management."

**Strategic watch-out** (revisit 3-month post-launch): if trial-to-Pro conversion lags, consider making Pro base self-serve, keeping only the White-label App add-on sales-led.

### 3.4 White-label mobile app add-on

Sold only as add-on to Pro. **$199/mo + $2,000 non-refundable setup** covers Apple/Google setup guidance, branded build, first submission, Firebase per tenant, named CSM (1h/month), SLA 99.9%, onboarding concierge, 60 days post-launch support, OS-release updates (2×/yr).

**Co-termination (Council finding):** mid-cycle add-on purchase is **prorated to co-terminate with the Pro annual renewal date**. Merchant pays `(remaining_days_of_pro_year / 365) × $199 × 12` up front; next renewal bundles Pro + App at full combined price.

**Loss-leader acknowledgment:** first 2-3 Pro+App deals are deliberate loss leaders. Unit economics positive only after §13.4 build automation pipeline lands.

---

## 4. Pricing — localized for 18 countries

### 4.1 Developed markets — USD parity

| Country | Shipping | Starter mo | Starter yr | Studio mo | Studio yr | Pro from (annual) |
|---|---|---|---|---|---|---|
| US | ShipEngine | $19 | $182 | $49 | $470 | $1,188/yr ($99/mo eq) |
| **Canada (CAD)** | ShipEngine | **C$25** | **C$239** | **C$65** | **C$625** | **C$1,619/yr (C$135/mo eq)** |
| UK (GB) | ShipEngine | £15 | £144 | £39 | £375 | £948/yr (£79/mo eq) |
| Ireland + EU (IE, DE, FR, IT, ES, NL) | ShipEngine | €17 | €163 | €45 | €432 | €1,068/yr (€89/mo eq) |
| Australia *GST-exclusive; see §19.4* | ShipEngine | A$29 + GST | A$278 + GST | A$75 + GST | A$719 + GST | A$1,788/yr + GST |
| New Zealand | ShipEngine | NZ$29 | NZ$278 | NZ$75 | NZ$719 | NZ$1,788/yr |
| Singapore | NinjaVan | S$25 | S$239 | S$65 | S$623 | S$1,548/yr |

Canada billed in **CAD** (consistent with native local-currency policy, resolving the §4.1 vs §4.2.2 ambiguity from Codex + Council review).

### 4.1.1 Emerging markets — PPP-adjusted (~33% off USD parity)

| Country | Carrier | Starter mo | Starter yr | Studio mo | Studio yr | Pro from (annual) |
|---|---|---|---|---|---|---|
| India | Delhivery | ₹999 | ₹9,599 | ₹2,499 | ₹23,999 | ₹65,999/yr (₹5,499/mo eq) |
| Malaysia | NinjaVan | RM 59 | RM 569 | RM 149 | RM 1,429 | RM 3,588/yr |
| Thailand | NinjaVan | ฿499 | ฿4,799 | ฿1,199 | ฿11,519 | ฿28,788/yr |
| Philippines | NinjaVan | ₱749 | ₱7,199 | ₱1,899 | ₱18,239 | ₱45,588/yr |
| Indonesia | NinjaVan | Rp 199,000 | Rp 1,919,000 | Rp 499,000 | Rp 4,799,000 | Rp 11,988,000/yr |
| Vietnam | NinjaVan | ₫329,000 | ₫3,169,000 | ₫799,000 | ₫7,699,000 | ₫19,788,000/yr |

### 4.1.2 White-label app add-on

**$199/mo + $2,000 non-refundable setup, USD globally, no PPP localization** — underlying Apple/Google/Firebase cost base is USD-denominated.

### 4.1.3 PPP disclosure policy

- Geo-served pricing pages do NOT cross-display regional rates
- Pricing is not negotiable on the basis of another region's rate
- Pricing-page FAQ: "We adjust prices to match local purchasing power in emerging markets, consistent with industry practice (Spotify, Figma, Notion)"
- **Support escalation path (BA finding):** pricing-tier disputes involving legal entity jurisdiction evidence route to **billing-ops, not support**. Support does not approve pricing exceptions.

**Deferred:** UAE (AE) requires Aramex; post-v1.

### 4.2 Multi-currency billing model

Each merchant is charged in their own local currency from subscription creation through the entire term. Displayed price = charged price = statement amount.

#### 4.2.1 Architecture

- **Commit to `currency_options` on single Price object** (SA finding): 8 Price objects total (4 plans × 2 billing periods), each with `currency_options` for all 13 billing currencies. PPP tiers use separate Price IDs since `currency_options` amounts must be consistent per subscriber.
- **Binding event:** `billing_currency` is set at **`checkout.session.completed`** webhook (the reliable signal that checkout succeeded), stored on `store_subscriptions.billing_currency`, locked for the entire billing period.
- **Country change only at next renewal** (§4.6).
- **Invoices + admin UI show exact currency + amount**, never USD-equivalent estimates.
- **MRR/ARR reporting normalizes to USD** via daily mid-market rate snapshot for internal analytics only; merchant-facing displays always native.

#### 4.2.2 Split-currency Stripe settlement — designed, **post-$200k ARR activation**

**Design:** Stripe AU supports split-currency settlement. USD charges → USD-denominated Australian business bank account (bypasses ~1.5% USD→AUD spread). AUD → AUD account. Other currencies → AUD.

**v1 activation:** **Optional; deferred to $200k ARR or first meaningful US/UK/EU revenue concentration.** At v1 scale (~100 merchants), 1.5% FX savings ≈ $22-36/mo; operational cost of multi-currency reconciliation and second bank account exceeds savings. Council praised this as "brilliant FinOps" for design elegance; SA review correctly flagged v1 timing. Design stays in the spec; implementation is triggered, not immediate.

**Accounting approach:** AR tracked per currency. Monthly reconciliation runs per-currency against Stripe settlement reports. Revenue recognition in USD via mid-market FX rate snapshot for reporting.

**Not-app-secret note:** USD-AU bank account credentials are bank credentials, not app secrets. Protected at the banking layer (2FA, hardware token), not stored in GCP Secret Manager or application config.

#### 4.2.3 FX risk exposure

Mark8ly bears Stripe spread (~1.5%) on each non-AUD/USD settlement. Diversified across 11 currencies. Priced into anchor prices. Re-review if any currency moves >10% vs USD.

### 4.3 Price-table review cadence

Developed markets: every 6 months. PPP discounts: annually. Hard-update any row if USD moves >10% since last review.

### 4.4 Billing period changes

- **Monthly → Annual:** immediate with pro-rata credit. Currency unchanged (locked per §4.2.1). Monthly Pro switching to annual releases the +20% premium.
- **Annual → Monthly:** at end of current annual period. Switch to monthly triggers +20% Pro premium if applicable. No currency change mid-term.

### 4.5 Plan upgrades and downgrades

- **Upgrade immediately, prorate** (using locked `billing_currency`).
- **Downgrade at end of period**, gated by §4.5.1 checks.

#### 4.5.1 Downgrade-block checks

**Store-count block (Studio → Starter requires ≤2 stores):**

UI blocking dialog lists all stores (active + soft-deleted-but-restorable ≤60 days). For each store with >0 in-flight orders: red warning banner + open-order count + "Download orders CSV" link. Merchant explicitly chooses:

- **Close** store: storefront dark, data retained, **counts toward plan slot** (explicit UX copy: "Closing does NOT free a plan slot; to reduce store count, Delete")
- **Delete** store: hard-delete with 60-day soft-delete grace, then purged

**Concurrency (Architect finding):** advisory lock on **`store_id`** (aligning with §17.4 writes, not `tenant_id`). Count includes soft-deleted-but-restorable stores.

**Cron re-check at downgrade execution** (end-of-period). If over-quota at execution time (e.g., merchant restored a soft-deleted store during grace window) (**Council finding #2**):
- **Downgrade blocked**: subscription remains on current (higher) plan
- **Renewal charges at higher plan rate**
- **Alert email** to merchant explaining why downgrade didn't execute + what to do
- Subscription state stays `active` (no `cancel_scheduled` misroute)

**Image-limit grandfathering (Studio → Starter):** existing products retain their 50-image counts; new uploads cap at 25. Admin UI shows "Grandfathered" badge on affected products.

### 4.6 Country change mid-subscription

Takes effect at next renewal. Email 14 days before renewal with new country/currency/amount. India + annual → §4.7 RBI-via-webhook check. Unsupported country → block + migration path.

### 4.7 Stripe-native challenge fallback (replaces hardcoded thresholds)

**Do not hardcode ₹15,000 threshold.** Listen for Stripe `invoice.payment_action_required` webhook. Generalizes across RBI/SCA/3DS.

**Trial card-add timing (Council finding #11):** adding a card during trial **creates the Stripe subscription but defers the first charge to day 90** (Shopify model). Punishing early card-adds with immediate charge kills conversion. Explicit flow:

1. Day N < 90 during trial, merchant adds card via Stripe Checkout
2. Mark8ly creates Stripe Subscription with `trial_end = signup_date + 90d`
3. Stripe auto-invoices on day 90 at native billing currency
4. Merchant receives confirmation: "Card saved. First charge on [date], ₹999 (or local)"
5. Merchant can cancel anytime in trial without charge

**Fallback flow (card challenge):**
1. Webhook `invoice.payment_action_required` → subscription → `payment_action_required`
2. Create Stripe Invoice with hosted URL, enable local payment methods (UPI/NetBanking/3DS link)
3. Reminder emails T-14, T-7, T-1
4. `invoice.paid` → extend period, return to `active`
5. Unpaid at T+0 → `past_due`, enter dunning §16

---

## 5. Trial mechanics

### 5.1 Signup gates (v1)

- Email verification before storefront publishes
- reCAPTCHA Enterprise at Cloudflare Worker
- Rate-limit: 3/IP/24h, 1/email
- Disposable email blocklist refreshed weekly
- **Campaign send volume ramp** (replaces 7-day hard block):

| Trial day | Max campaign emails/day |
|---|---|
| Day 1–3 | 500 |
| Day 4–7 | 2,000 |
| Day 8+ | Plan allowance (Starter 15k/mo, Studio 50k/mo, Trial 5k/mo) |

**Volume-ramp mechanism (Architect finding):** daily `trial_ramp_cron` runs 00:00 UTC. Computes current trial day per store; updates `campaign_email_budget.limit_set` via atomic UPDATE. Subscription at day 3→4 transition jumps `limit_set` from 500 to 2,000; day 7→8 jumps to full plan allowance. Pure function of `(signup_date, now())` — idempotent. Budget grain stays `(store_id, month)`; trial ramps override the monthly allowance within first 7 days.

- Signup volume alert: >50 trial signups/day

### 5.1.1 Migration fast-path — 48h expedited validation

Merchants migrating from existing platforms can request expedited tax-ID validation. **Evidence requirements (Security finding):**

- **Option A:** Submitted prior-storefront domain has WHOIS creation date ≥90 days before Mark8ly signup, AND resolves to a live product catalog (not parked page)
- **Option B:** Screenshot of existing-platform admin (Shopify/WooCommerce/BigCommerce) showing store creation date ≥90 days prior

**Performed by CSM** (not general support) — 5-business-day SLA for review. Fast-path reduces the 14-day storefront-publish hold to 48 hours. **Fast-path does NOT waive tax ID validation** — only shortens the storefront-publish window.

### 5.2 Tax ID validation — 14-day hard window

Form asks: business name, country, tax ID, billing address. Server-side validation per §19.3.

**Clock-pause triggers (Council finding #10 + v2.2 clock-pause):**
- Registry API unavailable >72h cumulatively within 14-day window → clock pauses
- **ID enters SEA manual-review queue → clock pauses IMMEDIATELY** (not after review completes). Otherwise merchant locked during internal ops backlog.
- Clock resumes when API reachable AND/OR review queue resolved

Day 14 without validation: admin read-only + billing; storefront unpublished.

### 5.3 Timeline

| Day | Event | Admin | Storefront |
|---|---|---|---|
| 0 | Signup, email verify, reCAPTCHA pass, tax ID submit | Full access, campaigns capped to ramp D1-3 | Unpublished until tax ID validated |
| 0–14 (paused on registry outage or SEA review queue) | Tax ID validation window | Full access | Publishes on validation |
| 0–3 | Volume ramp A | 500 campaign emails/day | Full (if validated) |
| 4–7 | Volume ramp B | 2,000/day | Full |
| 8–59 | Normal | Full access | Full |
| 60 | Banner | "Add card before day 90" | Full |
| 75 | Banner escalates | Amber + email | Full |
| 85 | Final nudge | "5 days" + email | Full |
| Any day ≤90, card added | Subscription created with `trial_end = day 90` | Normal | Normal |
| 90 (no card) | Expires | Read-only (allowlist §17.3) | "Store closed" page |
| 90–149 | Grace | Read-only | "Store closed" |
| 150 | Hard delete | Data purged; billing archive retained (§23) | 404 |

### 5.4 Store-closed storefront page

Cloudflare Worker serves `/assets/closed.html` with merchant branding interpolated. **HTTP 200 OK + `X-Robots-Tag: noindex`** (not 307 redirect — Worker serves HTML directly).

---

## 6. Transaction fee model

**Zero transaction fees on merchant GMV.** Merchants BYO gateway keys. "Your store, your payments, no middleman fees" — pricing-page copy.

---

## 7. Promo codes

### 7.1 Rules

| Rule | Value |
|---|---|
| When applicable | Post-trial only |
| Max depth | 50% off |
| Max duration (monthly) | 6 months |
| Annual promo | One-shot, year-1 only |
| Stacking | One active promo per subscription |

### 7.2 Allowed shapes

- **Post-trial monthly discount** — e.g. `MARK8LY50EARLY` (13 chars, compliant)
- **Annual upfront discount** — e.g. `MARK8LY20ANNUAL`
- **Grandfathered launch rate** — `grandfathered_price` override, one-shot segment strategy

### 7.3 Abuse prevention

- Each email redeems a given code once
- **Rate limit**: 5/IP/hour; 10/email/24h
- **Timing-safe generic response** for both "not found" and "expired"
- **Min 12 characters** mixed-case alphanumeric, no visually ambiguous chars
- Stripe Coupon IDs as canonical backend

### 7.4 Promo floor price (Council finding #5)

**Promo codes cannot reduce effective price below an absolute floor per plan per currency.** If a promo would drop effective price below the floor, Stripe Coupon validation rejects it.

**Floors (developed markets):** Starter $12/mo, Studio $30/mo, Pro $75/mo.
**Floors (emerging markets, PPP-adjusted):** Starter ₹800, Studio ₹1,800, Pro ₹4,200 (India); comparable for other emerging markets.

**Rationale:** at 50% off ₹999 Starter = ₹500 ≈ $6 USD. SendGrid cost at 15k campaign emails ≈ $4-5/merchant. Margin negative. Floor prevents promo codes from being mathematically loss-generating.

Implementation: `promo_codes` table has `min_effective_price_per_currency` JSONB column; code validation checks floor before applying.

---

## 8. Refunds

**14-day cooling-off** from first charge → full refund, compliant EU CRD / UK / AU ACL. After 14 days: cancel anytime, access to period end, no refund. Pro+App setup fee never refundable.

**Refund fraud prevention:** card fingerprint + one refund per fingerprint lifetime + device-fingerprint logged.

**v1 flow:** Stripe Dashboard + `refund_audit` table.

---

## 9. Feature matrix

| Feature | Trial | Starter | Studio | Pro | Pro + App |
|---|---|---|---|---|---|
| **Limits** | | | | | |
| Stores | 1 | 2 | 5 | up to 10 | up to 10 |
| Products/categories/orders | ∞ | ∞ | ∞ | ∞ | ∞ |
| Staff seats | ∞ | ∞ | ∞ | ∞ | ∞ |
| Images per product | 25 | 25 | 50 | Unlimited | Unlimited |
| Image file size (all) | 10 MB | 10 MB | 10 MB | 10 MB | 10 MB |
| Audit log retention | 90d | 90d | 12mo | Forever | Forever |
| Campaign emails/mo | 5k (ramp §5.1) | 15k | 50k | Negotiated | Negotiated |
| Transactional emails | ∞ (100k fair-use) | ∞ | ∞ | Negotiated ceiling | Negotiated ceiling |
| Coupons/loyalty/campaigns | ∞ | ∞ | ∞ | ∞ | ∞ |
| **Storefront** | | | | | |
| Custom domain | ✓ | ✓ | ✓ | ✓ | ✓ |
| Full color palette | ✓ | ✓ | ✓ | ✓ | ✓ |
| Announcement bar | ✓ | ✓ | ✓ | ✓ | ✓ |
| Remove "Powered by Mark8ly" | ✓ | ✓ | ✓ | ✓ | ✓ |
| Custom CSS + fonts | — | — | ✓ | ✓ | ✓ |
| Custom code injection (JS/HTML) | — | — | — | ✓ | ✓ |
| White-label iOS + Android app | — | — | — | — | ✓ |
| **Platform** | | | | | |
| CSV import/export, shipping labels, returns, reviews, tickets, gift cards | ✓ | ✓ | ✓ | ✓ | ✓ |
| Read-only API + webhooks (rate-limited) | — | — | ✓ | ✓ | ✓ |
| Full read/write API | — | — | — | ✓ | ✓ |
| SSO (SAML/OIDC via GIP) | — | — | — | ✓ | ✓ |
| Uptime SLA (99.9%) | — | — | — | — | ✓ |
| **Support** | | | | | |
| Standard email support (24h) | ✓ | ✓ | ✓ | — | — |
| Priority email support (4h) | — | — | — | ✓ | ✓ |
| Named CSM + onboarding concierge | — | — | — | — | ✓ |

### 9.1 Features NOT in matrix (pricing-page FAQ)

- Multi-language/multi-currency storefronts — future
- Inventory transfer between stores — available wherever merchant has ≥2 stores
- Analytics retention — Starter 12mo / Studio 24mo / Pro forever
- Staff permission granularity — role-based (admin/staff/read-only); no custom role editor v1

---

## 10. Email enforcement

### 10.1 Atomic-decrement budget + trial ramp

```sql
CREATE TABLE campaign_email_budget (
    store_id   UUID         NOT NULL,
    month      DATE         NOT NULL,
    remaining  INT          NOT NULL,
    limit_set  INT          NOT NULL,  -- mutated by trial-ramp cron for first 7 days of trial
    PRIMARY KEY (store_id, month)
);
```

**Pre-send enforcement:**
```sql
UPDATE campaign_email_budget
SET remaining = remaining - :recipient_count
WHERE store_id = :store_id
  AND month = date_trunc('month', now() at time zone 'utc')
  AND remaining >= :recipient_count
RETURNING remaining;
```

0 rows = reject with 403 + upgrade message.

**Trial-ramp daily cron (Architect finding):** runs 00:00 UTC. For each subscription where `(now() - signup_date)` is a transition day (3→4 or 7→8), atomically update `limit_set`. At day 3→4, `limit_set = GREATEST(remaining, 2000)` (preserve already-consumed). At 7→8, `limit_set = plan_allowance`. Plan-change webhook also recomputes `limit_set` in same transaction as subscription write.

### 10.2 Monthly reset, concurrent-send limit, transactional separation

First-of-month scheduler creates new budget row. Max 3 concurrent sends via Redis INCR or advisory lock. Transactional emails separate pipeline, 100k/store/month fair-use.

### 10.3 SendGrid cost

Evaluate SES migration at **500 paid merchants**.

---

## 11. Image/file caps

Per-product cap enforced at upload + server-side recheck. 10MB per-file platform-wide. Studio→Starter downgrade: existing products grandfathered, new uploads enforced.

---

## 12. SSO (Pro / Pro + App)

SAML 2.0 + OIDC via GIP. `tenant_sso_configs` per-tenant.

### 12.4 Break-glass admin

- Password: CSPRNG 20 chars
- Stored in GCP Secret Manager `/projects/tesserix-prod/secrets/break-glass-{tenant_id}`
- **TOTP mandatory** (not SMS)
- Rotation: 90d OR immediately after use
- Use triggers Slack alert to `#security-alerts` + audit log
- IAM: ≤2 staff

---

## 13. White-label mobile app add-on

### 13.1 Apple app-factory workaround

Apps submitted under **tenant's own Apple Developer + Google Play Console** accounts. Mark8ly handles build + submit under tenant accounts. See §18.9 for credential management.

### 13.2 Apple 4.2.6 UI-variety requirement

~30% first-review rejection rate without UI differentiation. CSM concierge enforces before submission:
- Distinct splash screen per tenant
- Custom app icon (tenant brand, not Mark8ly-default)
- ≥3 brand customizations (colors, fonts, custom CSS)
- Custom onboarding flow

**Contractual acknowledgment:** Pro+App onboarding contract states: "First Apple App Store review may be rejected under Apple Guideline 4.2.6. Resubmission after UI variety updates typically succeeds within 1-2 cycles. Timeline budget: 2-4 weeks from submission to approval."

### 13.3 Setup fee covers (one-time $2,000 USD)

Dev-account setup guidance, branded build, first + resubmission, Firebase per tenant, named CSM onboarding + 60 days post-launch support.

### 13.4 Ongoing (in $199/mo)

Named CSM (1h/mo), SLA 99.9%, onboarding concierge, OS-release updates (2×/yr), push infra + crash monitoring. Tenant retains Apple $99/yr + Google one-time $25.

### 13.5 Teardown sequence on churn

Separate lifecycle enum from subscription state (Architect finding):

```go
type WhiteLabelAppStatus string

const (
    AppStatusActive          WhiteLabelAppStatus = "active"
    AppStatusSunsetScheduled WhiteLabelAppStatus = "sunset_scheduled"
    AppStatusDownloadsBlocked WhiteLabelAppStatus = "downloads_blocked"
    AppStatusPulled          WhiteLabelAppStatus = "pulled"
    AppStatusFirebaseArchived WhiteLabelAppStatus = "firebase_archived"
    AppStatusCredentialsPurged WhiteLabelAppStatus = "credentials_purged"
)
```

Stored on `white_label_app_lifecycle(store_id, status, scheduled_at, actor, reason)`. Orthogonal to `SubscriptionStatus` — app lifecycle continues independently of subscription state during churn.

**Default graceful 60-day sunset (merchant cannot choose "app-only $49/mo continuity" at day 0 — that tier is a v2 placeholder; Council finding #12):**

| Day | Action | Comms owner |
|---|---|---|
| 0 | Sunset scheduled. Firebase project → read-only. | **CSM** — formal teardown notice email, personal call |
| 7 | In-app banner deploys: "Service ending in 53 days" | Automated (CSM CC'd) |
| 30 | New downloads blocked (app marked unavailable); existing installs still open, storefront shows "closed" | Automated |
| 60 | App pulled from both stores; Firebase project archived | Automated (CSM CC'd) |
| 90 | Firebase project deleted; **tenant Apple ASC API key + Google Play service account credentials deleted from Secret Manager** (§18.9) | Automated + CSM confirms |

Merchant-initiated immediate pull: apps pulled within 7 days, Firebase archived immediately.

---

## 14. Pro onboarding process

### 14.1 Pro base (no white-label app)

| Stage | Owner | Duration | Deliverable |
|---|---|---|---|
| Contact form submitted | Marketing → Notion | Immediate | Sales record, #sales-inbox |
| Sales-qualification reply | Sales | 24h | Pricing confirmation |
| Self-serve annual checkout | Sales | 1 day | Stripe Checkout URL |
| Tenant provisioned | Engineering | 2 days post-payment | Pro active |
| Time-to-value | Buyer | 30 days from contact | Merchant on Pro |

### 14.2 Pro + App add-on

| Stage | Owner | Duration | Deliverable |
|---|---|---|---|
| Contact form | Marketing | Immediate | Sales record |
| Discovery call | Sales | 3 days to schedule | 45-min call |
| Quote issued | Sales | 2 days | PDF with price, scope, setup fee, SLA, MSA |
| MSA + DPA signed | Buyer + Sales | 2 weeks | DocuSign |
| Setup fee invoice | Finance | 1 day | Stripe invoice NET-15 |
| **Apple ASC + Google Play credentials collected** | CSM | Before build starts | Credentials stored per §18.9 |
| Setup fee paid | Buyer | 15 days | Stripe confirmation |
| Tenant provisioned | Engineering | 2 days | Pro + App active |
| CSM onboarding call | CSM | 1 week | Goals documented |
| UI customization gate (§13.2) | CSM | Before submission | 3+ brand customizations verified |
| App build + first submission | Engineering | 4-6 weeks | Apps live |
| Time-to-value | Buyer + CSM | 60 days | Live with apps |

### 14.3 Procurement prerequisites

MSA + DPA templates, cyber liability insurance $1M, SOC 2 Type I triggered at first $100k+ deal.

---

## 15. Cancellation flow

### 15.1 Merchant-facing

- Entry: admin → Subscription → "Cancel"
- Confirmation dialog + exit survey + save offer
- **Save-offer promo timing (BA finding):** discount applies **starting with the NEXT invoice cycle, not retroactively to the current period**. Explicit confirmation text: "Your 50% discount applies to your next 3 charges after your current billing cycle ends. No credit is issued for the current period."
- Save-offer acceptance → state `cancel_scheduled → active` (§17.2), promo applied
- Final: `subscription_status = cancel_scheduled` with `cancels_at = current_period_end`

### 15.2 Post-cancellation

At `cancels_at` → `expired`. 60-day data retention. Add-card restores.

### 15.3 Win-back

Day 30 post-cancellation: win-back email with 20% off 6 months.

### 15.4 GDPR rights on closed/deleted stores (BA finding)

Customer-generated data on closed/deleted stores (reviews, order history) is **customer-subject data** under GDPR Art. 17 right-to-erasure and Art. 20 portability — merchant cannot arbitrarily delete it without honoring customer rights.

**Policy:**
- **Customer reviews** on a closed store: retained under legitimate-interest basis during closed state; customers retain erasure rights individually via Mark8ly support channel (not through merchant admin)
- **Customer order history**: customers of a closed store retain order-history access via Mark8ly-hosted `/my-orders/:email/:order_token` portal for **90 days** after store closure; beyond 90 days, archived (customer can request export via support)
- **On hard-delete (day 150 post-cancellation)**: customer-subject data is purged alongside merchant data; billing archive retains only merchant-side billing records per §23
- **Merchant cannot bypass Mark8ly for customer erasure requests** — customer emails Mark8ly support, Mark8ly honors + notifies merchant

### 15.5 Pro+App cancellation

Triggers white-label app teardown sequence §13.5.

---

## 16. Failed payment / dunning

### 16.1 Trigger

`invoice.payment_failed` → `past_due`.

### 16.2 Schedule

Day 0 → past_due, storefront live, admin editable. Day 1/3/5 retries. Day 5 second email. Day 7 final. Day 8 → expired, admin read-only, storefront lives day 8-13. Day 14 storefront → "closed". Day 90 hard-delete path.

### 16.3 Recovery

Card-add anytime before day 90 → active.

### 16.4 Tone

Editorial/calm. Not threatening.

### 16.5 Dunning vs refund

Refund window always dominates within first 14 days from first charge.

---

## 17. State transitions & concurrency

### 17.1 State enum

```go
type SubscriptionStatus string

const (
    StatusSignup                 = "signup"
    StatusTrialing               = "trialing"
    StatusActive                 = "active"
    StatusPastDue                = "past_due"
    StatusPaymentActionRequired  = "payment_action_required"
    StatusCancelScheduled        = "cancel_scheduled"
    StatusExpired                = "expired"
    StatusStoreClosed            = "store_closed"
    StatusPendingHardDelete      = "pending_hard_delete"
    StatusHardDeleted            = "hard_deleted"
)
```

### 17.2 Allowed transitions — sequential hard-delete path (Council finding #4)

```
signup → trialing (email verified)
trialing → active (card added; first charge deferred to day 90 per §4.7)
trialing → expired (day 90, no card)
active → past_due (invoice.payment_failed)
active → payment_action_required (invoice.payment_action_required)
active → cancel_scheduled (merchant cancel)
past_due → active (retry succeeds)
past_due → expired (dunning final fail)
payment_action_required → active (invoice.paid via invoice flow)
payment_action_required → past_due (invoice unpaid past reminder)
cancel_scheduled → active (save-offer reversal §15.1, or card re-added)
cancel_scheduled → expired (current_period_end reached)
expired → active (card re-added during grace window)
expired → store_closed (day 14 post-expiry)  -- ONLY path out of expired
store_closed → active (card re-added during grace window)
store_closed → pending_hard_delete (day 90 post-expiry)  -- ONLY path into pending_hard_delete
pending_hard_delete → hard_deleted (deletion job)
```

**Expired routes sequentially through `store_closed` before `pending_hard_delete`.** No direct `expired → pending_hard_delete` shortcut. This eliminates the ambiguous "ONLY if 14d AND 90d both elapsed" language from v2.2 — simpler, no branch confusion, matches merchant experience (every expired subscription goes through a visible 76-day storefront-closed window).

### 17.3 Read-only mode — admin allowlist

Blocked-by-middleware `subscription.RequireActive()` when `subscription_status ∈ {expired, store_closed, pending_hard_delete}`. **`payment_action_required` is NOT in this list (Council finding #3)** — merchants in that state must retain full admin + storefront access to complete 3DS/UPI authentication.

Allowed routes in read-only:
- `GET /admin/**` (view-only)
- `POST /admin/stores/:storeId/billing/*`
- `POST /admin/stores/:storeId/subscription/*`
- `GET /admin/stores/:storeId/orders/export/*`
- `POST /admin/auth/**`

Middleware order: `IstioAuth → TenantMiddleware → RequireActive → RequireFeature → handler`.

HTTP **402 Payment Required** response. CSM/SLA gate: only if `HasWhiteLabelAppAddOn = true` AND subscription `active`.

### 17.4 Concurrency controls

Every subscription write: `pg_advisory_xact_lock(hashtext(store_id::text))` + re-read + CAS guard + log to `subscription_state_log`. §4.5.1 downgrade-block uses same `store_id` lock.

### 17.5 Idempotent crons

Pure functions of time + row state.

### 17.6 Stripe webhook event catalog

15 events: `checkout.session.completed` (triggers `billing_currency` binding per §4.2.1), `customer.subscription.*`, `customer.updated`, `invoice.created/finalized/paid/payment_failed/payment_action_required`, `charge.refunded`, `payment_method.attached/detached`, `radar.early_fraud_warning`.

### 17.7 Webhook idempotency + orphan handling

```sql
CREATE TABLE stripe_webhook_events (
    event_id         VARCHAR(100) PRIMARY KEY,
    event_type       VARCHAR(100) NOT NULL,
    store_id         UUID,
    payload          JSONB NOT NULL,
    received_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    processed_at     TIMESTAMPTZ,
    processing_error TEXT,
    retry_count      INT NOT NULL DEFAULT 0
);
```

Flow: verify signature (raw body first) → `INSERT ON CONFLICT (event_id) DO NOTHING` → process in advisory-locked tx → set `processed_at` in same tx → return 200.

**Orphan resolution SLA:**
- Re-attempt cron every 5 min for events with `store_id IS NULL AND processed_at IS NULL`
- Per-event retry cap: `retry_count ≤ 6` (30 min total) before auto-transitioning to `manual_review_required`
- 1-hour unresolved → PagerDuty page on-call
- 24-hour unresolved → `manual_review_required` flag + observability dashboard; cron stops retrying

### 17.8 Idempotency keys — outbound Stripe calls

- Create customer: `customer:{store_id}`
- Checkout: `checkout:{store_id}:{plan}:{period}:{day_bucket}`
- Subscription: `subscription:{store_id}:{plan}:{billing_period}`
- **Customer Portal: `portal:{store_id}:{5_min_bucket}`** (Council finding #1 — hour_bucket returned expired session URL; 5-min bucket matches portal URL lifetime)
- Refund: `refund:{invoice_id}`

Server-generated keys only.

---

## 18. Security & compliance

### 18.1 PCI-A scope

SAQ-A. Forbid card/CVV/PAN storage, raw body logging, card details in Mark8ly API routes. Display `card.last4` from Stripe API; don't store.

### 18.2 Multi-tenant isolation

`GetByStoreID(ctx, db, tenantID, storeID)` — `tenant_id` required. Call-site audit in implementation.

### 18.3 Stripe secret key management

GCP Secret Manager with WIF. Live secret rotated 90d; webhook secret rotated on URL change. Merchant BYO gateway keys namespaced per tenant, deleted on hard-delete. IAM ≤3 staff.

**USD-AU bank account credentials (§4.2.2) note:** bank credentials, not app secrets. Protected at banking layer (2FA, hardware token), not in Secret Manager or app config.

### 18.4 Enterprise API keys (Pro)

32 bytes entropy, `mk8_live_` prefix, bcrypt hash, per-key scopes + rate limits + tenant binding. Immediate revocation; 24h rotation overlap.

### 18.5 Webhook endpoint hardening

Istio rate limit 100 req/min/IP; `http.MaxBytesReader` 512 KB; event-type allowlist post-signature; no body logging.

### 18.6 Log sanitization

Stripe errors: `error.code` + `error.type` only. Webhook: `event.id` + `event.type`. Redact email, billing address, GSTIN/VAT, raw IP.

### 18.7 Account recovery

Self-service card-add during grace. Post-hard-delete: email verify + director-approval + two-person review.

### 18.8 Geo-pricing anti-arbitrage

Card-country + IP + billing triangulation at subscription creation. Flag-based, not block-based.

**Schema:**

```sql
CREATE TABLE subscription_arbitrage_audit (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    subscription_id      UUID NOT NULL REFERENCES store_subscriptions(id),
    tenant_id            UUID NOT NULL,
    store_id             UUID NOT NULL,
    card_country         CHAR(2),
    billing_country      CHAR(2),
    ip_country           CHAR(2),      -- derived, NOT raw IP
    ip_hash              VARCHAR(64),  -- HMAC-SHA256(key=Secret-Manager-salt, data=raw_ip)
    resolved_price_tier  VARCHAR(20) NOT NULL,
    mismatch_reason      VARCHAR(100),
    flagged_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    reviewed_by          UUID,
    reviewed_at          TIMESTAMPTZ,
    resolution           VARCHAR(30),
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ON subscription_arbitrage_audit (flagged_at) WHERE resolution = 'ongoing';
```

**IP hashing (Security finding):**
- Algorithm: **HMAC-SHA256** (not SHA-256 with concatenated salt; HMAC prevents length-extension and is the correct construction for keyed hashing)
- Key: 256-bit from CSPRNG
- Stored in GCP Secret Manager at `/projects/tesserix-prod/secrets/arbitrage-ip-hmac-key` as a versioned secret
- Rotation: every 30 days via Cloud Scheduler writing a new secret version (old version retained for 31-day overlap window to allow in-flight correlation)
- **Accepted limitation:** cross-rotation correlation beyond 30 days is intentionally severed. For longer-window correlation, `ip_country` (not rotated) is the durable join field.

**PII handling:** raw IP never stored. 2-year retention uncleared; 7-year if enforcement action taken. Access scoped to `billing-ops` IAM role; every read logged.

**Enforcement action on confirmed arbitrage:**
1. Re-invoice at developed-market tier at next renewal; 14-day merchant notice
2. Merchant dispute path → §18.8.1 self-service appeal
3. Refusal to pay → cancellation at next renewal
4. Second flag after first resolution → management escalation; account termination possible

**Concurrency:** quarterly-audit job that clears flags takes `store_id` advisory lock.

#### 18.8.1 False-positive self-service appeal

Admin UI shows banner if `arbitrage_flag = true`: "We've noted a discrepancy in your billing details — [Resolve]"

Form: "Which jurisdiction best describes your business?" + optional document upload. Routes to `billing-ops` queue for 5-biz-day review. Resolution options: `false_positive_cleared` | `reprice_developed` | `ongoing`.

### 18.9 White-label app credential management (Security HIGH finding)

Apple Developer and Google Play credentials for Pro+App tenants are **publisher-account-level privileged**. Loss = every tenant's app publishing compromised.

**Credential types accepted:**
- **Apple: App Store Connect API key** (`.p8` file + issuer ID + key ID). Apple ID password **NOT accepted** — it cannot be scoped.
- **Google: Google Play Developer service account JSON** with `releasemanager` role scoped to tenant's Developer Account.

**Storage:**
- `/projects/tesserix-prod/secrets/merchant/{tenant_id}/apple-asc-api-key` — the `.p8` content
- `/projects/tesserix-prod/secrets/merchant/{tenant_id}/apple-asc-issuer-id`
- `/projects/tesserix-prod/secrets/merchant/{tenant_id}/apple-asc-key-id`
- `/projects/tesserix-prod/secrets/merchant/{tenant_id}/google-play-service-account` — JSON content

**IAM access:** CI/CD build pipeline service account + ≤2 engineering staff. Audit log on every access.

**Rotation:** revoke + re-issue on engineer departure or suspected breach. Documented runbook.

**Teardown:** at day 90 of the §13.5 sunset sequence, **all four credentials above are deleted from Secret Manager** (`white_label_app_lifecycle.status = credentials_purged`).

**Audit:** every access logged to audit-service + GCP Cloud Logging; alert on unexpected patterns.

---

## 19. Tax compliance — B2B reverse charge

### 19.1 Model

B2B only, 18 countries, reverse-charge mechanism in EU/UK/India/SEA. Mark8ly does not collect VAT/GST except Australia (domestic).

### 19.2 Enforcement

Tax ID required at signup, validated per-country, invoices annotated with reverse-charge clause.

### 19.3 Per-country specifics

| Country | Tax | Validator | Reverse charge | Fallback |
|---|---|---|---|---|
| US | Federal: none | EIN format + **legally-binding attestation checkbox (§19.3.1)** | N/A | Checkbox signed |
| CA | GST/HST 5-15% | Business Number + checkbox | Yes for B2B | Checkbox signed |
| UK | VAT 20% | HMRC VAT API | Yes for B2B | Provisional; clock pauses if API >72h down |
| IE + EU (DE/FR/IT/ES/NL) | VAT 19-25% | VIES | Yes for B2B | Provisional |
| AU | GST 10% (MARK8LY AU charges) | ABN Lookup | N/A domestic | Block |
| NZ | GST 15% | IRD | Yes for B2B *(pending counsel §20.3)* | Provisional |
| India | GST 18% OIDAR | GSTN API | Yes for B2B | Provisional; §4.7 challenge fallback |
| Singapore | GST 9% | ACRA | Yes for B2B | Provisional |
| Malaysia | SST 8% | MOF SST | Partial | **5-biz-day manual review; clock pauses at queue entry** |
| Thailand | VAT 7% | RD API | Yes for B2B | **5-biz-day manual review; clock pauses at queue entry** |
| Philippines | VAT 12% | BIR | Yes for B2B | **5-biz-day manual review; clock pauses at queue entry** |
| Indonesia | VAT 11% | DJP NPWP | Yes for B2B | **5-biz-day manual review; clock pauses at queue entry** |
| Vietnam | VAT 10% | GDT | Yes for B2B | **5-biz-day manual review; clock pauses at queue entry** |

**SEA 30/week capacity threshold:** sustained >30 manual reviews/week over 2 weeks triggers ops capacity review + commit to API enrollment within 30 days.

**Name cross-check during manual review:** tax IDs verified against registry-returned business name (fuzzy match). Mismatch flagged. `tax_id_name_match` column: `matched | unmatched | not_checked`.

### 19.3.1 US/CA business-entity attestation

```sql
CREATE TABLE business_entity_attestations (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    store_id        UUID NOT NULL,
    tenant_id       UUID NOT NULL,
    country         CHAR(2) NOT NULL,
    checkbox_text   TEXT NOT NULL,
    checkbox_version VARCHAR(20) NOT NULL,
    user_agent      TEXT,
    ip_hash         VARCHAR(64),  -- HMAC-SHA256 per §18.8
    signed_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- Append-only. No UPDATE (trigger) AND no DELETE (role-level revoke).
CREATE TRIGGER business_entity_no_update BEFORE UPDATE ON business_entity_attestations
    FOR EACH ROW EXECUTE FUNCTION raise_immutable_exception();
REVOKE DELETE ON business_entity_attestations FROM marketplace_api_user;
```

Both the UPDATE-blocking trigger AND the `REVOKE DELETE` at the role level are required (Security finding). Trigger alone is not sufficient — a `DROP TRIGGER` followed by DELETE would bypass it. Role-level revocation closes that path.

### 19.4 Australia-specific

Mark8ly Pty Ltd charges 10% GST + remits ATO. Pricing page shows **GST-exclusive** with "Plus GST" below. Invoice breaks out GST separately.

**Stripe configuration:** AU Price objects use `tax_behavior: exclusive`. Stripe Tax enabled for AU GST registration. **Default `tax_behavior: inclusive` must be explicitly overridden.**

### 19.5 Quarterly revalidation — storefront-unpublish-only (Council finding #6)

Scheduled job re-checks tax IDs. If invalid:
- Email merchant, **14-day update window**
- **Storefront unpublishes** at day 14 if unresolved (matches onboarding enforcement)
- **Billing continues** (merchant pays normally)
- This removes the perverse incentive where merchants let tax IDs lapse to get free service: now they continue paying but lose their storefront, forcing them to resolve

### 19.6 Pre-launch tax counsel — NZ is critical path

Required before launch:
- EU/UK/India reverse-charge B2B confirmation (1-2 weeks)
- **NZ reverse-charge + possible GST registration — critical path** (1-2 weeks opinion + 4-8 weeks registration processing if required)
- AU GST registration confirmed

---

## 20. Legal & TOS

### 20.1 TOS must include

14-day cooling-off terms, non-refundable Pro+App setup fee, jurisdictional notices, subprocessor list, GDPR/DPDP disclosures, right-to-erasure carve-out, SLA definition (Pro+App 99.9%), AUP, US/CA business-entity attestation, AU GST inclusivity, PPP pricing disclosure.

### 20.2 DPA template (Pro+App)

GDPR Article 28 compliant.

### 20.3 Pre-launch checklist

| Item | Lead time | Critical path? |
|---|---|---|
| TOS drafted + legally reviewed | 4-8 weeks | No |
| DPA template | 1-2 weeks | No |
| Cookie policy + GDPR consent | 1 week | No |
| Privacy Policy (DPDP + GDPR) | 2-3 weeks | Operational: India grievance officer |
| MSA template for Pro+App | 2-3 weeks | No |
| Cyber liability insurance $1M | 2-4 weeks | Yes if first Pro+App deal imminent |
| **NZ tax counsel + potential GST registration** | **1-2 weeks opinion + 4-8 weeks registration** | **YES — critical path** |
| EU/UK/India tax counsel | 1-2 weeks | No |

### 20.4 Post-launch

SOC 2 Type I at first $100k+ deal. EU/UK tax re-confirm at 100 merchants/country. India GST OIDAR at ₹20 lakh/mo.

---

## 21. Observability

### 21.1 Metrics

- `subscription.state.count{status}` — gauge per status
- `subscription.mrr_{currency}` — gauge per currency
- `subscription.trial.expired_today` — counter
- `subscription.trial.activated_day_30` — counter (activation funnel)
- `subscription.trial.product_created_day_30` — counter (key activation milestone)
- `subscription.payment_failed` — counter
- `subscription.payment_action_required` — counter
- `subscription.arbitrage_flagged` — counter
- `subscription.arbitrage_false_positive_cleared` — counter
- `subscription.downgrade_blocked_at_cron` — counter (§4.5.1 failure case)
- `campaign.email.sent{store_id}` — counter
- `webhook.processed{event_type}` — counter
- `webhook.failed{event_type, reason}` — counter
- `webhook.orphan_resolved_after_seconds` — histogram

### 21.2 Logs

Structured JSON per state transition: actor, from_status, to_status, plan, timestamp.

### 21.3 Alerts

- Trial scheduler dead-man's-switch >25h stale
- Failed payment spike >5% in 24h
- Webhook P95 >5s
- Webhook failure >1% in 1h
- Trial signup anomaly >50/day
- Break-glass admin use → immediate Slack
- Arbitrage flag spike >5× baseline
- **Orphan webhook >1h unresolved → PagerDuty**
- Trial activation <30% Day-30 milestone rolling 30d → consider trial shortening

### 21.4 Dashboards

"Subscription Health" dashboard: MRR by currency, active count by plan, failed-payment trend, upcoming expirations, webhook health, trial activation funnel.

---

## 22. Disaster recovery

### 22.1 CNPG backup

PITR 7d. Daily GCS snapshots 30d. Subscription-critical tables daily export 90d.

### 22.2 Recovery

| Scenario | RTO | RPO | Mechanism |
|---|---|---|---|
| Pod restart | 30s | 0 | Knative |
| CNPG primary fail (≥100 merchants, sync standby) | **2 min** | 0 | CNPG failover |
| CNPG primary fail (<100 merchants) | 4h | 24h | GCS snapshot |
| Table drop | 1h | 5min | PITR |
| Cluster loss | 4h | 24h | GCS → new cluster |
| Region loss | 24h+ | 24h | GCS → new region |

**Sync standby at 100-merchant tier** (not 500) — churn risk > ~$15/mo cost.

### 22.3 Stripe + GCS reconciliation

Stripe reconstructs subscription state but NOT `promo_redemptions`, `refund_audit`, `subscription_arbitrage_audit`, `business_entity_attestations`, `white_label_app_lifecycle`. GCS snapshots are the only recovery path for these.

---

## 23. Audit logging + billing archive

### 23.1 Subscription mutations → audit-service

Every write emits structured event: created, plan change, status, card events, refund, promo, hard-delete, SSO config, add-on purchased/cancelled, app lifecycle transitions.

### 23.2 Billing archive — 7-year retention

```sql
CREATE TABLE billing_archive (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    original_store_id    UUID NOT NULL,
    original_tenant_id   UUID NOT NULL,
    business_name        VARCHAR(500) NOT NULL,
    tax_id               VARCHAR(50),
    tax_id_country       CHAR(2),
    billing_country      CHAR(2),
    billing_currency     CHAR(3),
    stripe_customer_id   VARCHAR(100) NOT NULL,
    all_invoices         JSONB NOT NULL,
    total_revenue_usd    NUMERIC(12,2),
    hard_deleted_at      TIMESTAMPTZ NOT NULL,
    archive_expires_at   TIMESTAMPTZ NOT NULL
);
```

### 23.3 GDPR erasure

Live PII purged. `billing_archive` retained under legal-obligation basis. Stripe customer deleted via API. Customer-subject data purged per §15.4. Merchant notified of retained data.

---

## 24. Database sizing — CNPG staircase

| Merchant count | CPU | Memory | Storage | Sync standby | Notes |
|---|---|---|---|---|---|
| 0–100 | 0.5 | 2 GiB | 50 GiB | No | Launch; 4h RTO |
| **100–500** | **1** | **4 GiB** | **200 GiB** | **Yes** | **RTO 2 min; PgBouncer** |
| 500–2,000 | 2 | 8 GiB | 500 GiB | Yes | WAL archiving tuned |
| 2,000–10,000 | 4 | 16 GiB | 1 TiB | Yes + async | — |
| 10,000+ | Evaluate horizontal partitioning | 32+ | 2+ TiB | Yes + async | Cloud Spanner evaluation |

**CNPG sync replication requirement (SA finding):** `synchronous_commit = on` in `Cluster.spec.postgresql.parameters` — required for RPO=0 at the 100-merchant tier. `instances: 2` in Cluster spec, `primaryUpdateStrategy: unsupervised` for auto-failover. Default CNPG `synchronous_commit = local` does NOT provide RPO=0 — this configuration must be explicit. Update happens in `tesserix-k8s` ArgoCD overlay, not code.

**Upgrade triggers:** >80% CPU/memory sustained 1h; P95 query latency >500ms; 80% storage.

---

## 25. Implementation gaps

**v1 effort table excludes items deferred or optional.** Split-currency USD-AU settlement account (§4.2.2) is optional v1 / post-$200k ARR activation; not in v1 effort.

| Area | Effort |
|---|---|
| **Backend — plans & billing** | |
| Update `plangate.go` to 4-plan matrix | 1 day |
| Migration: all new tables + columns (§2) | 2 days |
| Fix `GetByStoreID` + call-site audit | 2 days |
| Stripe multi-currency `currency_options` Price objects (8 total) + Checkout + Portal | 1.5 weeks |
| Webhook dispatcher (§17.6, §17.7) + orphan 5-min re-attempt cron + dead-letter | 1 week |
| State machine + advisory locking + idempotency + Customer Portal 5-min bucket | 5 days |
| Plan upgrade/downgrade + store-block + image grandfathering + cron failure path | 5 days |
| Campaign budget + trial-ramp daily cron (§10.1, §5.1) | 4 days |
| Read-only middleware + allowlist (excluding `payment_action_required` per §17.3) | 2 days |
| Trial + dunning crons | 1 week |
| Trial card-add deferred-charge flow (§4.7) | 2 days |
| `invoice.payment_action_required` fallback (§4.7) | 4 days |
| Promo codes + abuse prevention + **absolute floor per plan/currency** (§7.4) | 1 week |
| Refund flow v1 + Stripe Dashboard + `refund_audit` | 2 days |
| Store-closed Worker page (200 + noindex) | 3 days |
| Tax ID validation (12 validators + clock-pause on API + queue entry) | 2 weeks |
| US/CA business-entity checkbox + immutable storage (REVOKE DELETE + trigger) | 2 days |
| Reverse-charge invoice template | 1 week |
| AU Stripe `tax_behavior: exclusive` + Stripe Tax AU | 1 day |
| Tax revalidation storefront-unpublish flow (§19.5) | 2 days |
| Billing archive + hard-delete hook | 2 days |
| Geo-pricing anti-arbitrage + HMAC-SHA256 IP hashing + self-service appeal | 4 days |
| SEA manual-review with name cross-check + capacity monitoring | 3 days |
| White-label app teardown sequence (§13.5) + separate lifecycle enum | 3 days |
| **White-label app credential management (§18.9)** — storage, IAM, rotation, teardown purge | 2 days |
| Pro monthly premium logic (+20%) | 1 day |
| App add-on co-termination logic | 2 days |
| Save-offer prospective-only promo logic | 1 day |
| Customer-subject data portal (GDPR closed-store §15.4) | 3 days |
| **Backend — shipping** | |
| Add IE, NZ to ShipEngine | 1 day |
| Add VN to NinjaVan | 1 day |
| AE / Aramex (deferred v2) | 1 week |
| **Frontend — admin** | |
| Pricing page (geo-localized, local-currency display, Pro $1,188/yr framing) | 1 week |
| Plan management UI + billing-period switch + monthly +20% Pro disclosure | 1 week |
| Cancellation flow + survey + save-offer (prospective disclosure) | 3 days |
| Email usage counter with ramp visibility | 2 days |
| Tax ID field + 14d reminder + clock-pause indicator + migration fast-path upload | 4 days |
| Failed-payment banner + add-card flow | 2 days |
| Contact-sales form for Pro + Notion + Slack | 2 days |
| Pro+App add-on purchase flow + co-termination disclosure | 2 days |
| Arbitrage self-service appeal | 2 days |
| Store-close-before-downgrade UX (close-vs-delete, slot-not-freed copy, in-flight orders) | 3 days |
| **Security + compliance** | |
| PCI-A code audit | 2 days |
| GCP Secret Manager + WIF + HMAC key rotation Cloud Scheduler | 3 days |
| Break-glass + TOTP + rotation | 3 days |
| Enterprise API keys (Pro) | 1 week |
| Webhook endpoint hardening | 2 days |
| **Observability** | |
| Subscription metrics + trial activation funnel + arbitrage + orphan SLA + downgrade-blocked | 3 days |
| Dashboards | 1 day |
| Alerts | 2 days |
| **SSO (Pro / Pro+App)** | 2-3 weeks |
| **White-label app (first deal)** | 4-6 weeks |

**v1 self-serve launch (Starter + Studio + Trial + Pro base contact-sales):** ~10-12 engineer-weeks back-end + ~3 weeks front-end.

**Pro + App first deal:** adds ~4-6 weeks including credential management.

---

## 26. Risks & open questions

### 26.1 Pre-launch blockers

1. Legal review (TOS, Privacy, DPA) — $15-30k
2. **NZ tax counsel + potential GST registration (critical path)** — 1-2w opinion + 4-8w registration
3. Stripe India test of `invoice.payment_action_required` flow
4. Tax ID validators operational (12 countries; SEA enrollment-gated possibly)
5. Cyber liability $1M before first Pro+App
6. India DPDP grievance officer designated

### 26.2 Strategic risks

1. SendGrid cost at 500+ merchants — SES migration eval
2. Stripe AU acquiring rate for US/UK cards — revisit at $500k ARR
3. **White-label app build automation** — critical before 2nd deal. **First 2-3 Pro+App deals are deliberate loss leaders** — margin positive only after automation.
4. Apple 4.2.6 rejection ~30% — CSM UI-variety gate mitigates
5. Trial abuse — card-BIN verification if gates insufficient
6. Geo-arbitrage — triangulation + false-positive appeal path handles most
7. Pro $99 contact-sales friction — revisit self-serve at 3-month post-launch review
8. 90-day trial long-tail — telemetry-driven iteration (<30% Day-30 activation → shorten to 30d)
9. **Split-currency USD settlement — v1 optional / activate at $200k ARR**

### 26.3 Open questions

1. SOC 2 Type I timing — at first $100k deal or 18 months?
2. Grandfathered launch rate — slots?
3. Pro+App "app-only" continuity tier ($49/mo post-Pro-churn) — v2 eval?

### 26.4 Out of scope

- Marketplace plan features (placeholder)
- Payment gateway aggregation (never)
- Customer-facing storefront discounts (separate product)
- Multi-language / multi-currency storefronts (future)
- WhatsApp Business API (future)
- AI merchant tooling (future)

---

## 27. Decision log

(Complete decision trail from all prior rounds. v2.3 additions marked.)

| Decision | Rationale |
|---|---|
| 3-month trial, no card at signup | Editorial positioning; conversion quality > rate |
| Trial campaign volume ramp | Protects anti-spam without penalizing serious merchants |
| B2B-only reverse-charge | Eliminates 12+ jurisdictions' VAT/GST registration |
| 4-tier pricing | Trial + Starter + Studio + Pro; Studio solves $19→$99 cliff |
| $99 Pro base + CSM/SLA moved to add-on | Accessible mid-market; CSM aligned with customer who values it |
| Pro visible $1,188/yr ($99/mo) floor | Self-qualifies prospects |
| **Pro monthly +20% premium ($119/mo)** | **Council finding — prevents 1-month churn exploit** |
| **App add-on co-terminates with Pro annual renewal** | **Council finding — avoids misaligned renewal dates** |
| PPP discount for 6 emerging markets | Industry standard; prevents accessibility gap |
| **Canada billed in CAD (not USD)** | **Council + Codex finding — resolves ambiguity with native-currency policy** |
| Native local-currency billing via `currency_options` | Truth-in-advertising; single Price per plan × period (8 total) |
| **USD-AU split settlement — design kept, v1 optional / $200k ARR activation** | **SA review — operational cost > savings at v1 scale** |
| Zero transaction fees | Differentiated from Shopify |
| Flat monthly | Rejected GMV-tier on brand grounds |
| 14-day cooling-off refund | Legally bulletproof 18 countries |
| Pro contact-sales with visible floor | Self-qualification + flexibility |
| First Pro+App deals are loss leaders | Unit economics positive only after automation |
| Mobile app via tenant-owned dev accounts | Apple 4.2.6 workaround |
| **Stripe-native challenge webhook (not hardcoded threshold)** | **Generalizes RBI/SCA/3DS** |
| **Trial card-add defers first charge to day 90** | **Council finding — Shopify model; preserves conversion** |
| Sync standby at 100 merchants | Outage during first product launch = churn |
| Excess-store downgrade block with `store_id` lock + cron re-check | Prevents silent over-quota |
| **Downgrade cron failure: block + stay on higher plan + email** | **Council finding — explicit failure behavior** |
| Image-limit grandfathering | Respects existing work |
| **White-label app 60-day sunset + separate lifecycle enum** | **Architect finding — orthogonal lifecycles** |
| **Sequential `expired → store_closed → pending_hard_delete` path** | **Council finding — eliminates branch confusion** |
| HTTP 200 + noindex for store-closed | Worker serves HTML; no redirect |
| Orphan webhook 5-min re-attempt + 6-retry cap + 24h dead-letter | Explicit SLA |
| **Portal session idempotency: 5-min bucket** | **Council finding — hour bucket returns expired URL** |
| Arbitrage: flag + review + self-service appeal | Handles diaspora/expat/corporate-card legitimately |
| **HMAC-SHA256 (not SHA-256 concat) + salt in Secret Manager** | **Security finding — correct keyed-hash construction** |
| **REVOKE DELETE on business_entity_attestations** | **Security finding — trigger alone insufficient** |
| **Apple/Google credentials in per-tenant Secret Manager paths + teardown purge** | **Security HIGH finding — publisher-account-level privilege** |
| 14-day tax-ID validation + clock-pause at queue entry | Closes attack surface + prevents ops-backlog lockout |
| **Tax revalidation: unpublish storefront (not pause billing)** | **Council finding — removes perverse incentive to let tax ID lapse** |
| US/CA legally-binding business-entity checkbox + immutable storage | Closes B2B enforcement gap |
| **`payment_action_required` NOT in read-only** | **Council finding — merchant needs admin access for 3DS/UPI auth** |
| SEA 30/week manual-review capacity trigger | Prevents silent SLA erosion |
| AU Stripe `tax_behavior: exclusive` | Prevents default-inclusive pitfall |
| NZ tax counsel critical-path sequencing | 8-week registration risk |
| Migration fast-path 48h + **90-day WHOIS evidence minimum** | Prevents social-engineering shortcut |
| IP hashed with HMAC+Secret Manager rotating key | GDPR pseudonymization correctly constructed |
| **`billing_currency` set at `checkout.session.completed`** | **BA finding — reliable binding event** |
| **Save-offer promo prospective only (next invoice)** | **BA finding — eliminates retroactive-credit ambiguity** |
| **Customer-subject data portal for closed-store GDPR rights** | **BA finding — reviews/orders are customer data** |
| **Promo absolute floor per plan/currency** | **Council finding — prevents margin-negative promo on PPP markets** |
| Single Stripe AU account | Operational simplicity; revisit at $500k ARR |

---

## 28. Success criteria

(36+ criteria from v2.2 + new for v2.3 additions.)

| # | Criterion | Test type |
|---|---|---|
| 1-36 | *(Previous v2.2 criteria preserved)* | — |
| 37 | Portal session idempotency: same `portal:*:5min` key returns same URL within 5 min, new URL after 5 min | Integration |
| 38 | `payment_action_required` state: merchant admin fully editable, storefront live | Integration |
| 39 | Downgrade cron failure: merchant has 5 stores at period end → downgrade blocks, stays on Studio, renewed at Studio rate, email fired | Integration |
| 40 | Promo code 50% off ₹999 Starter: rejected with "below absolute floor" error | Integration |
| 41 | Tax ID lapses on quarterly revalidation: storefront unpublishes day 14, billing continues | Integration (time-mocked) |
| 42 | Canada signup: billed in CAD (C$25), Stripe Price object for CAD used, not USD | Integration |
| 43 | Pro monthly subscription: charged $119/mo (not $99); switching to annual releases premium | Integration |
| 44 | White-label App add-on purchased month 6 of Pro annual: prorated to co-terminate; renews on original Pro anniversary | Integration |
| 45 | SEA tax ID enters manual-review queue: 14-day clock pauses at queue entry (not on completion) | Integration (time-mocked) |
| 46 | Trial merchant adds card day 45: subscription created, first charge NOT immediate, deferred to day 90 | Integration |
| 47 | Teardown day 0 dialog: no "app-only $49" option presented (v2 feature) | Manual UAT |
| 48 | Expired → pending_hard_delete: requires passage through `store_closed` state (no direct transition) | Integration |
| 49 | Sequential path: day 91 expired → day 105 store_closed → day 181 pending_hard_delete | Integration (time-mocked) |
| 50 | `business_entity_attestations` DELETE attempt by app DB user: rejected by role-level revoke (even if trigger dropped) | Security test |
| 51 | IP hash: HMAC-SHA256 with Secret Manager key; salt rotation 30d preserves 31-day overlap; cross-window correlation severed beyond 31d | Security test |
| 52 | Pro+App merchant cancellation at day 90: Apple + Google credentials deleted from Secret Manager | Integration |
| 53 | Migration fast-path: submitted domain with WHOIS <90d rejected; ≥90d + live product catalog accepted | Integration |
| 54 | Save-offer acceptance mid-cycle: current period NOT credited; next invoice applies discount | Integration |
| 55 | Orphan webhook event after 6 retries (30 min): auto-transitions to manual_review_required | Integration |

---

## 29. Next step

After user approval of v2.3, proceed to **implementation plan** via `writing-plans` skill. Plan decomposes §25 gap list into atomic sequenced tasks with tests-first structure and atomic commit boundaries. Target: ~10-12 engineer-weeks back-end + ~3 weeks front-end for v1 self-serve launch; +4-6 weeks for first Pro+App customer.
