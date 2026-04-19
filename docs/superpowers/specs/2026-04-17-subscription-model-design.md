# Mark8ly Subscription Model — Design

**Status:** v2.2 — addresses round-3 reviewer HIGH/IMPORTANT/MEDIUM items, Council review (GPT-5.4 + Gemini 3.1 Pro + Claude Opus 4.6 + Grok 4.20), local-currency billing model
**Date:** 2026-04-17 (revised 2026-04-18)
**Scope:** Final pricing, plans, trial mechanics, billing infrastructure, feature matrix, tax model, enforcement rules. Includes state machine, concurrency controls, security requirements, observability, and DR. Implementation-ready.

---

## 1. Summary

Mark8ly ships a **B2B-only** subscription model across 18 countries with **native local-currency billing** and PPP-adjusted pricing for emerging markets:

- **Trial** — 3 months free, no card, email-verified + reCAPTCHA gated, default on signup
- **Starter** — from $19/mo (developed markets) / ₹999 (India PPP), self-serve, 2 stores
- **Studio** — from $49/mo / ₹2,499 (India), self-serve, 5 stores, custom CSS, 50k emails, 12-month audit retention, read-only API
- **Pro** — **from $99/mo** / ₹5,499 (India), contact-sales with visible "Starts at" floor, 10 stores, SSO, full R/W API, custom code injection, priority support, unlimited
- **White-label mobile app add-on** — **$199/mo + $2,000 non-refundable setup**, Pro-only, bundles named CSM + SLA + onboarding concierge + branded iOS/Android apps under tenant's developer accounts
- **Marketplace** — placeholder in code, hidden from UI (future)

Mark8ly charges zero transaction fees on merchant GMV — merchants bring their own payment gateway keys (Stripe, Razorpay, PayPal) for *their* storefront checkout. Mark8ly's own billing runs through a single Stripe Australia account with **multi-currency Price objects**; each merchant is charged in their own local currency from signup through the entire billing term.

Because every merchant is a business entity, Mark8ly operates under the **B2B reverse-charge tax model** (EU/UK/SEA/India) — merchants self-account for VAT/GST. Tax IDs are validated at onboarding with a 14-day hard deadline before storefront publishes; SEA manual-review fallback clock pauses during validation outages.

Substantial scaffolding exists already: `plangate` Go package, `store_subscriptions` table, `internal/payment/stripe.go` client, shipping carriers (ShipEngine, NinjaVan, Delhivery) covering all 18 launch countries. This spec fixes a latent multi-tenant security defect, adds webhook idempotency + state-machine specifications, and defines failed-payment, cancellation, arbitrage, and white-label-app teardown flows that were previously implicit.

---

## 2. Existing infrastructure audit

| Artifact | Location | Status | Gap |
|---|---|---|---|
| Feature matrix | `services/marketplace-api/internal/plangate/gate.go` | 22 features × 5 plans | Rewrite to 4 plans (Trial/Starter/Studio/Pro) + Marketplace hidden per §9 |
| Subscription model | `services/marketplace-api/internal/subscription/models.go` | `StoreSubscription` struct + `SubscriptionPlan` enum | Update plan enum; add `ReverseChargeTaxID`, `TaxIDCountry`, `TaxIDValidated`, `TaxIDValidatedAt`, `BillingCurrency`, `PriceTier`, `HasWhiteLabelAppAddOn`, `ArbitrageFlag` fields |
| Subscription repository | `services/marketplace-api/internal/subscription/repository.go` | `GetByStoreID(ctx, db, storeID)` | **SECURITY BLOCKER: `tenant_id` missing from WHERE clause.** Change signature to `GetByStoreID(ctx, db, tenantID, storeID)`. Audit all call sites. |
| Migration | `000015_subscriptions.up.sql` | Base table exists | Add new migration: tax ID fields, plan enum rename, billing_currency, price_tier, add-on tracking, arbitrage flag |
| Stripe client | `services/marketplace-api/internal/payment/stripe.go` | Partial, idempotency keys missing, error-body logging leaks PII | Add multi-currency Price objects, subscription flow, Checkout sessions, Customer Portal, webhook dispatcher; fix log sanitization; add idempotency keys; configure `tax_behavior: exclusive` for AU prices |
| Admin subscription handler | `services/marketplace-api/internal/handlers/admin/subscription.go` | Exists | Update DTOs; add plan-switch, upgrade, cancel, store-close-before-downgrade endpoints |
| Shipping carriers | `services/marketplace-api/internal/shipping/{shipengine,delhivery,ninjavan}.go` | 15 countries | Add IE, NZ to ShipEngine; VN to NinjaVan; defer AE (Aramex) to post-v1 |
| Auth | GIP + OpenFGA | Multi-tenant; SAML + OIDC federation available | Build per-tenant SSO config for Pro (§12) |

**Audit finding:** the existing `GetByStoreID` query does not include `tenant_id` in its WHERE clause. This must be fixed as part of this spec's implementation.

---

## 3. Plan lineup

### 3.1 Tiers

| Plan | Visibility | Pricing (developed markets) | Billing | Audience |
|---|---|---|---|---|
| **Trial** | Default on signup, email-verified + reCAPTCHA | $0 for 90 days | N/A | Everyone B2B |
| **Starter** | Pricing page, self-serve | $19/mo or $182/yr (20% off) | Monthly or annual | Single-brand merchants, up to 2 stores |
| **Studio** | Pricing page, self-serve | $49/mo or $470/yr (20% off) | Monthly or annual | Growing merchants, custom CSS, larger marketing volume, up to 5 stores |
| **Pro** | Pricing page with **visible "Starts at $99/mo"** floor, "Contact sales" CTA | From $99/mo | Annual only (monthly on request) | Mid-market brands, SSO, multi-store, full API, custom code injection — up to 10 stores |
| **Pro + White-label mobile app add-on** | Pro-only add-on, sales-quoted | +$199/mo + **$2,000 non-refundable setup** | Annual only, renews with Pro subscription | Brands wanting branded iOS/Android apps; includes named CSM + SLA + onboarding concierge |
| **Marketplace** | *Hidden in UI, placeholder in code* | TBD (future phase) | TBD | Future multi-operator marketplace |

### 3.2 Pricing ladder rationale

```
$19  →  $49  →  $99  →  $298 (Pro + App)
      2.6×      2.0×    3.0×
```

No price cliffs. Each transition has a clear upgrade driver and a proportional price step.

### 3.3 Pro "contact sales" with visible floor

Pro is contact-sales-led with a **published floor price**. Pricing page Pro card displays:

- Feature list (SSO, multi-store, full API, custom code injection, priority support)
- **"Starts at $99/mo"** — visible floor, localized per country
- Primary CTA: "Contact sales"
- Secondary CTA: "Download Pro brief" (PDF one-pager)

**Clarification on the floor:** $99/mo is the **standard Pro price for a baseline Pro merchant**, not just the lowest possible deal. Sales can negotiate upward (Pro + App is the natural upsell; custom enterprise packages with higher store counts or specialized SLAs are quoted case-by-case), but $99 is the anchor the merchant sees and will expect to pay unless specific custom requirements push the quote higher. This eliminates ambiguity during sales conversations.

**Pricing page FAQ must clarify Pro support:** "Pro customers receive priority email support (4-hour response). Dedicated Customer Success Manager (CSM), uptime SLA, and onboarding concierge are available as part of the White-label App add-on for brands that want dedicated account management."

Contact form captures: business name, tax ID, country, annual GMV range, intended store count, timeline, need for white-label app (yes/no). Submissions auto-create a Notion record in `Sales Pipeline` and Slack notification to `#sales-inbox`. Response SLA 24h business hours.

**Strategic watch-out (from Council review):** a founder doing $50k/mo GMV who just wants SSO/API may prefer self-serve Pro. Trial-to-Pro conversion rate should be monitored. If conversion lags vs Studio, consider making Pro base self-serve at 3-month post-launch review, keeping only Pro + App add-on behind the contact-sales wall.

### 3.4 White-label mobile app add-on

Sold only as add-on to Pro. Cannot be purchased on Starter or Studio. The **$199/mo + $2,000 non-refundable setup** covers:

- Apple Developer + Google Play Console setup guidance (tenant owns the accounts)
- Branded iOS + Android app build (tenant's icon, splash, colors, deep links into storefront)
- First submission to both stores (5–10 business days typical)
- Firebase push notification infrastructure per tenant
- **Named Customer Success Manager** (1h/month dedicated)
- **Uptime SLA (99.9%)** with service credits
- **Onboarding concierge**
- 60 days post-launch issue support
- Ongoing per-major-OS update maintenance (2×/year)

**Loss-leader acknowledgment:** first 2-3 Pro+App deals are deliberate loss leaders. Unit economics break even only after the §13.4 build automation pipeline lands. Priced for case studies, pipeline, and category learning — not margin.

---

## 4. Pricing — localized for 18 countries

### 4.1 Developed markets — USD parity

12 countries priced at USD-equivalent local numbers. Geo-detected on pricing page from billing address or IP.

| Country | Shipping carrier | Starter monthly | Starter annual | Studio monthly | Studio annual | Pro from |
|---|---|---|---|---|---|---|
| US, CA | ShipEngine | $19 | $182 | $49 | $470 | $99 |
| UK (GB) | ShipEngine | £15 | £144 | £39 | £375 | £79 |
| Ireland (IE) *new carrier config* | ShipEngine | €17 | €163 | €45 | €432 | €89 |
| EU (DE, FR, IT, ES, NL) | ShipEngine | €17 | €163 | €45 | €432 | €89 |
| Australia *GST-exclusive, see §19.4* | ShipEngine | A$29 + GST | A$278 + GST | A$75 + GST | A$719 + GST | A$149 + GST |
| New Zealand *new carrier config* | ShipEngine | NZ$29 | NZ$278 | NZ$75 | NZ$719 | NZ$149 |
| Singapore | NinjaVan | S$25 | S$239 | S$65 | S$623 | S$129 |

### 4.1.1 Emerging markets — PPP-adjusted (~33% off USD parity)

6 countries priced ~33% below USD parity to reflect purchasing-power differences. Industry standard (Figma, Notion, Canva, Spotify).

| Country | Carrier | Starter monthly | Starter annual | Studio monthly | Studio annual | Pro from |
|---|---|---|---|---|---|---|
| India | Delhivery | ₹999 | ₹9,599 | ₹2,499 | ₹23,999 | ₹5,499 |
| Malaysia | NinjaVan | RM 59 | RM 569 | RM 149 | RM 1,429 | RM 299 |
| Thailand | NinjaVan | ฿499 | ฿4,799 | ฿1,199 | ฿11,519 | ฿2,399 |
| Philippines | NinjaVan | ₱749 | ₱7,199 | ₱1,899 | ₱18,239 | ₱3,799 |
| Indonesia | NinjaVan | Rp 199,000 | Rp 1,919,000 | Rp 499,000 | Rp 4,799,000 | Rp 999,000 |
| Vietnam *new NinjaVan country* | NinjaVan | ₫329,000 | ₫3,169,000 | ₫799,000 | ₫7,699,000 | ₫1,649,000 |

### 4.1.2 Add-on — white-label mobile app

**Priced globally in USD: $199/mo + $2,000 non-refundable setup.** No PPP localization — underlying cost base (Apple Developer, Firebase, build labor) is USD-denominated.

### 4.1.3 PPP disclosure policy

Because the ~33% PPP discount creates price variance across countries, Mark8ly must explicitly state its policy to support staff and in public communication.

**Policy:**
- **Geo-served pricing pages do not cross-display regional rates.** A US visitor sees only US pricing. An Indian visitor sees only Indian pricing. No dropdown to compare.
- **Pricing is not negotiable on the basis of another region's rate.** A US merchant cannot claim Indian pricing regardless of payment method, billing address, or card-country (see §18.8 anti-arbitrage).
- **PPP adjustment is Mark8ly's deliberate policy**, consistent with industry practice, to make commerce software accessible across purchasing-power tiers. Pricing page FAQ calls this out plainly: "We adjust prices to match local purchasing power in emerging markets, the same way Spotify, Figma, and Notion do."
- **Support response framework**: if a developed-market merchant discovers regional pricing and asks, support responds with the FAQ text + decline request. No ad-hoc concessions.

**Deferred to v2:** UAE (AE) — requires Aramex carrier integration (~1 week build). Launch Mark8ly waitlist for AE now.

### 4.2 Multi-currency billing model

**Each merchant is charged in their own local currency from subscription creation through the entire billing term.** Displayed price = charged price = card statement amount. No mystery FX gap.

#### 4.2.1 Architecture

- **Stripe Price objects per currency.** Each plan × billing-period has Price IDs for all 13 billing currencies (USD, CAD, GBP, EUR, AUD, NZD, SGD, INR, MYR, THB, PHP, IDR, VND). Stripe's `currency_options` on Price allows multi-currency from a single Price object; alternative is separate Price per currency.
- **Billing currency binding**: at subscription creation, `billing_currency` is set from the **verified billing country** (`customer.billing_address.country` on Stripe Customer), not IP. Stored on `store_subscriptions.billing_currency`.
- **Currency locked for active billing period.** Mid-cycle country changes do NOT change the billing currency — only the renewal re-evaluates.
- **Country change applies at next renewal only**, per §4.6. Merchant email 14 days before renewal confirms new currency + amount.
- **Invoices + admin UI show exact currency + amount**, never "~$X USD equivalent" estimates.

#### 4.2.2 Settlement optimization (split-currency Stripe AU account)

To avoid burning ~1.5% FX on global revenue, Mark8ly configures Stripe AU with **split-currency settlement**:

- **USD charges** (US, CA merchants billed in USD, CAD): settle to a **USD-denominated Australian business bank account**. Bypasses the 1.5% USD→AUD spread that would otherwise apply.
- **AUD charges** (AU merchants billed in AUD): settle to Mark8ly's primary AUD account. No conversion.
- **All other currencies** (GBP, EUR, NZD, SGD, INR, MYR, THB, PHP, IDR, VND): settle to AUD at Stripe's spread. Volume doesn't justify separate per-currency accounts at v1.
- **Why USD specifically gets its own account**: GCP, SendGrid, Firebase, Google Play fees, Apple Developer fee are all USD-denominated. Holding USD is a natural hedge on Mark8ly's cost base, not added exposure.

**Accounting impact:** Mark8ly now has AR in multiple currencies. Monthly close runs per-currency reconciliation against Stripe settlement reports. Tracked in observability via `subscription.mrr_{currency}` custom metric per currency for analytics normalization.

#### 4.2.3 FX risk exposure

- Starter monthly × 1,000 merchants in each market = modest exposure per currency
- Mark8ly bears the Stripe spread (~1.5%) on each non-AUD/USD settlement
- Revenue recognition normalizes to USD via daily mid-market rate snapshot for ARR/MRR reporting (but invoices and charges remain in native currency)

### 4.3 Price-table review cadence

Every 6 months for developed markets. Hard-update any currency row if USD has moved >10% against that currency since last review. **PPP discounts re-evaluated annually** — emerging-market currencies are volatile; rapid repricing hurts merchant trust.

### 4.4 Billing period changes

- **Monthly → Annual:** takes effect **immediately** with pro-rata credit. `credit = (days_remaining_in_month / days_in_month) × monthly_price`. Charge now = `annual_price - credit`. New billing cycle starts today; renewal in 12 months. **Currency unchanged** (locked at subscription creation per §4.2.1).
- **Annual → Monthly:** takes effect **at end of current annual period**. No mid-period repricing; no currency change mid-term.

### 4.5 Plan upgrades and downgrades

- **Upgrade immediately, prorate.** Starter→Studio charges the difference on the spot.
- **Downgrade at end of period**, gated by downgrade-block checks below.

#### 4.5.1 Downgrade-block checks

Two hard blocks prevent silent over-quota states and data loss:

**Store-count block (Studio → Starter requires ≤2 stores):**

When merchant initiates Studio → Starter downgrade, the UI presents a blocking dialog:

- Lists all stores (active + soft-deleted-but-restorable within 60 days)
- For each store with >0 in-flight orders: red warning banner + open-order count + "Download orders CSV" link
- Merchant must explicitly either **close** (storefront dark, data retained, can be restored) or **delete** (hard-delete, 60-day soft-delete grace, then purged) each excess store
- "Close" retains the slot — a closed store still counts toward the plan's store limit. To free a slot, merchant must delete.
- "Delete" removes the store; the 60-day soft-delete retention applies per §5.3

**Concurrency guard:** the downgrade-block check takes a **PostgreSQL advisory lock on `tenant_id`** and counts `stores WHERE deleted_at IS NULL OR deleted_at > now() - interval '60 days'` (i.e., include soft-deleted-but-restorable). **The end-of-period cron that executes the scheduled downgrade re-checks at execution time** — not trusting the scheduling-time check, because soft-deleted stores can be restored during the grace period and cause silent over-quota.

**Image-limit grandfathering (Studio → Starter images/product):**

Studio allows 50 images/product; Starter allows 25. On downgrade, **existing products are grandfathered** (products with >25 images retain their existing image set). **New uploads enforce the 25-image limit**. Admin UI shows a visible "Grandfathered image count" badge on affected products. Merchants can bulk-delete excess images via admin if they want consistency.

### 4.6 Country change mid-subscription

- Takes effect at **next renewal**, never mid-period
- Merchant receives email 14 days before renewal with new country, new pricing, new currency
- **Currency changes**: new `billing_currency` computed from new `customer.billing_address.country`; applied at renewal only
- If new country = India + billing period = annual: re-evaluate RBI e-mandate applicability (§4.7)
- If new country moves merchant from emerging-market PPP zone to developed-market zone (or vice versa): price change applies at next renewal per §4.1/§4.1.1
- If new country not yet supported (e.g. AE pre-v2): block the change; offer migration path (wait or cancel)

### 4.7 Stripe-native challenge fallback (replaces hardcoded RBI threshold)

**Do not hardcode the ₹15,000 threshold in backend logic.** Government regulations and Stripe compliance logic change; hardcoded thresholds drift. Instead, **listen for Stripe's native webhook** `invoice.payment_action_required`.

**When triggered:** Stripe emits this event when a card challenge is required (RBI e-mandate, EU SCA/PSD2, 3DS). This is Stripe's own compliance-aware signal and generalizes across jurisdictions.

**Fallback flow (same as previous RBI handling):**
1. Webhook received → flip subscription to `payment_action_required` state (`past_due` variant)
2. Create Stripe Invoice (not Subscription auto-charge)
3. Email merchant with hosted invoice URL + enabled local payment methods (UPI/NetBanking/3DS-link)
4. Reminder emails at T-14, T-7, T-1
5. On `invoice.paid` webhook, extend subscription by 12 months → return to `active`
6. If unpaid at T+0: flip to `past_due` and enter dunning §16

**Benefits over hardcoded threshold:**
- Generalizes to EU SCA (PSD2), 3DS2 challenges globally
- RBI threshold changes don't require code updates
- Stripe handles the compliance detection; Mark8ly handles the UX

---

## 5. Trial mechanics

### 5.1 Signup gates (v1 launch)

- **Email verification** required before storefront publishes
- **reCAPTCHA Enterprise** on signup form at Cloudflare Worker edge
- **Rate-limit signup endpoint**: 3 signups per IP per 24h; 1 signup per email
- **Disposable email blocklist** refreshed weekly
- **Campaign send volume ramp** (replaces 7-day hard block):

| Trial day | Max campaign emails/day |
|---|---|
| Day 1–3 | 500 |
| Day 4–7 | 2,000 |
| Day 8+ | Full plan allowance (Starter 15k/mo, Studio 50k/mo, Trial 5k/mo) |

Ramp protects against email-relay abuse without penalizing serious merchants importing a catalog + sending launch announcements on day 1. Volume counted via §10.1 atomic-decrement budget.

- **Signup volume alert**: Cloud Monitoring alert when trial signups exceed 50/day

### 5.1.1 Migration fast-path

Merchants migrating from existing platforms (Shopify, BigCommerce, WooCommerce) can request a **48-hour expedited tax-ID validation** by uploading evidence of prior storefront existence (DNS record, social proof URL, existing platform URL). CSM/support confirms within 48h; validated merchants skip the 14-day window (§5.2).

### 5.2 Tax ID validation at signup — 14-day hard window

Mark8ly is B2B-only. Signup requires a valid tax ID:

- Form asks: business name, country, tax ID, billing address
- Server-side validation per country (§19.3)
- **If validation fails or is pending**: signup completes; storefront does NOT publish until validated
- **14-day hard deadline** for first-time validation:
  - Day 7: reminder email
  - Day 12: escalation email
  - Day 14: admin locked to read-only + billing actions only; storefront remains unpublished
  - Day 30: signup cancelled, data purged (60-day soft-delete per §5.3)

**Clock-pause during registry outages:** if the country validator API is unavailable for >72 hours cumulatively within the 14-day window (VIES has documented multi-day outages; SEA APIs may be enrollment-gated), the 14-day clock **pauses** until the API is reachable OR §19.3's 5-business-day manual review completes. Merchants are emailed when the clock pauses and when it resumes.

- Passed validation: `tax_id_validated_at` timestamped; quarterly revalidation cron re-checks (§19.5)

### 5.3 Timeline

| Day | Event | Admin | Storefront |
|---|---|---|---|
| 0 | Signup | Full access, no campaigns | Unpublished until tax ID validated |
| 0–14 | Tax ID validation window (pauses on registry outage) | Full access except campaign-ramp | Publishes once tax ID validated |
| 0–3 | Volume ramp phase A | Max 500 campaign emails/day | Full (if validated) |
| 4–7 | Volume ramp phase B | Max 2,000 emails/day | Full |
| 8–59 | Normal use, full allowance | Full access | Full |
| 60 | Banner appears | "Add a card before day 90" | Full |
| 75 | Banner escalates | Amber accent + email reminder | Full |
| 85 | Final nudge | "5 days remaining" + email | Full |
| 90 (no card) | Trial expires | Read-only (allowlist per §17.3) | Editorial "store closed" page |
| 90–149 | Grace / soft-delete window | Read-only | "Store closed" |
| 150 | Hard delete | Data purged; billing archive retained (§23) | 404 |
| Any day card added | Activates | Normal | Normal |

### 5.4 Store-closed storefront page

Serving rule: expired subscription → Cloudflare Worker serves a branded "store closed" page from CF assets.

Implementation:
- `tenant-router-service` exposes `GET /status/:storeSlug → {status, store_name, logo_url}` (cached 60s at Worker)
- Worker checks status; if `status = closed` or `unpublished`, serves `/assets/closed.html` **with HTTP 200 OK + `X-Robots-Tag: noindex` header** (not 307 — the worker serves the HTML directly, it is not redirecting to another URL)
- Cache invalidation via POST from marketplace-api on subscription state change

---

## 6. Transaction fee model

**Mark8ly takes zero transaction fees on merchant GMV.** Merchants BYO gateway keys. "Your store, your payments, no middleman fees" — deliberate differentiator vs Shopify. Explicit on pricing page.

---

## 7. Promo codes

### 7.1 Rules

| Rule | Value |
|---|---|
| When applicable | **After** 90-day trial only |
| Max discount depth | 50% off |
| Max duration (monthly) | 6 months |
| Annual promo | One-shot, year-1 only |
| Stacking | One active promo per subscription |

### 7.2 Allowed shapes

- **Post-trial monthly discount** — e.g. `MARK8LY50EARLY`: first 3 months at 50% off
- **Annual upfront discount** — e.g. `MARK8LY20ANNUAL`: 20% off year-1
- **Grandfathered launch rate** — `grandfathered_price` override on `store_subscriptions`, one-shot segment strategy

### 7.3 Abuse prevention

- Each email can redeem a given code once
- **Rate limit**: 5 attempts per IP per hour; 10 per email per 24h
- **Timing-safe responses**: both "code not found" and "code expired" return identical generic `promo_code_invalid` response with consistent latency (no enumeration signal)
- **Minimum code length: 12 characters**, mixed-case alphanumeric, avoiding visually ambiguous characters (`0/O`, `1/l/I`)
- All codes stored as Stripe Coupon IDs for second-layer enforcement

---

## 8. Refunds

### 8.1 Policy

- **14-day cooling-off from first charge** → full refund, no questions. EU CRD Article 9 / UK / AU ACL compliant
- **After 14 days** → cancel anytime, access until period end, no pro-rated refund
- **Pro + App setup fee ($2,000 USD)** → never refundable
- **Chargebacks** → Mark8ly contests with access logs + TOS

### 8.2 Refund fraud prevention

- **Card fingerprint** stored on `store_subscriptions`
- **One refund per card fingerprint, lifetime.** Second requires manual CSM approval.
- **Device fingerprint** (IP hashed, user-agent hash, ASN) logged at card-add time

### 8.3 Refund flow (v1)

Stripe Dashboard + `refund_audit` table via internal endpoint. No dedicated admin UI in v1.

### 8.4 14-day dunning vs 14-day refund window

Refund window always dominates. A merchant in their first 14 days can request a refund regardless of dunning state.

### 8.5 TOS

Must state 14-day window + precise start point + non-refundable Pro+App setup fee + how to request refund. Legal-review required.

---

## 9. Feature matrix

| Feature | Trial | Starter | Studio | Pro | Pro + App |
|---|---|---|---|---|---|
| **Limits** | | | | | |
| Stores | 1 | 2 | 5 | up to 10 | up to 10 |
| Products, categories, orders | ∞ | ∞ | ∞ | ∞ | ∞ |
| Staff seats | ∞ | ∞ | ∞ | ∞ | ∞ |
| Images per product | 25 | 25 | 50 | Unlimited | Unlimited |
| Image file size (all plans) | 10 MB | 10 MB | 10 MB | 10 MB | 10 MB |
| Audit log retention | 90 days | 90 days | 12 months | Forever | Forever |
| Campaign emails/month | 5,000 (ramp §5.1) | 15,000 | 50,000 | Negotiated | Negotiated |
| Transactional emails | ∞ (100k/mo fair-use) | ∞ (same) | ∞ (same) | Negotiated ceiling | Negotiated ceiling |
| Active coupons, loyalty, campaigns | ∞ | ∞ | ∞ | ∞ | ∞ |
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
| SSO (SAML / OIDC via GIP) | — | — | — | ✓ | ✓ |
| Uptime SLA (99.9%) | — | — | — | — | ✓ |
| **Support** | | | | | |
| Standard email support (24h) | ✓ | ✓ | ✓ | — | — |
| Priority email support (4h) | — | — | — | ✓ | ✓ |
| Named CSM + onboarding concierge | — | — | — | — | ✓ |

**Note on CSM/SLA enforcement:** these features are gated on `HasWhiteLabelAppAddOn = true`, not on `plan = pro`. The §17.3 `RequireActive` middleware checks add-on status, not just plan.

### 9.1 Features NOT in the matrix (disclosed in pricing-page FAQ)

- **Multi-language storefronts** — future phase
- **Multi-currency storefronts** (customer-side) — future phase
- **Inventory transfer between stores** — available on all plans with ≥2 stores (Starter: between the 2; Studio: across 5; Pro: across up to 10)
- **Analytics data retention** — Starter 12 months; Studio 24 months; Pro forever
- **Staff permission granularity** — role-based (admin, staff, read-only); no custom role editor in v1

---

## 10. Email enforcement

### 10.1 Atomic-decrement budget (TOCTOU-safe)

```sql
CREATE TABLE campaign_email_budget (
    store_id   UUID         NOT NULL,
    month      DATE         NOT NULL,
    remaining  INT          NOT NULL,
    limit_set  INT          NOT NULL,
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

0 rows returned = budget exhausted → reject with 403 + upgrade message. PostgreSQL row-lock serializes concurrent UPDATEs atomically.

**Trial ramp** (§5.1): first 3 days, `limit_set = 500/day`; days 4-7, `limit_set = 2,000/day`; day 8+, plan allowance.

### 10.2 Monthly reset

- First-of-month scheduler creates new row per store
- No rollover
- Plan-change webhook handler recomputes `limit_set` and `remaining` inside same transaction that writes subscription change

### 10.3 Per-store concurrent send limit

Max 3 concurrent sends per store via Redis INCR + TTL or advisory lock.

### 10.4 Transactional emails — separate pipeline

Not metered against plan cap. Platform-wide 100k/store/month fair-use for abuse detection.

### 10.5 SendGrid cost posture

Evaluate SES migration at **500 paid merchants** (§26). At that scale SendGrid per-email cost (~$0.0009) dominates margin on emerging-market plans.

---

## 11. Image and file caps

Per-product cap enforced at upload URL issuance + server-side recheck. 10 MB per-file cap platform-wide. See §4.5 for downgrade grandfathering.

---

## 12. SSO (Pro / Pro + App)

SAML 2.0 + OIDC via GIP. `tenant_sso_configs` table per tenant.

### 12.4 Break-glass admin

Each Pro tenant has one non-SSO Mark8ly-native admin:
- Password: CSPRNG, **20 characters minimum**
- Stored in GCP Secret Manager `/projects/tesserix-prod/secrets/break-glass-{tenant_id}`
- **TOTP mandatory** (not SMS)
- **Rotation: 90 days** OR immediately after use
- Use triggers immediate alert to `#security-alerts` + audit log
- IAM access limited to ≤2 Mark8ly staff

---

## 13. White-label mobile app add-on (Pro-only)

### 13.1 Apple app-factory policy

Apps submitted under **tenant's own Apple Developer + Google Play Console accounts** (tenant owns credentials; Mark8ly handles build + submit under tenant accounts).

### 13.2 Apple Guideline 4.2.6 risk — UI variety requirement

Even with tenant-owned accounts, Apple reviewers reject apps from different publishers if UIs look functionally identical. Industry experience (Tapcart, Vajro): ~30% first-review rejection rate without sufficient UI differentiation.

**Mitigation:** CSM concierge onboarding must ensure:
- **Distinct splash screen** per tenant (not generic Mark8ly template)
- **Custom app icon** (tenant's brand, not a mark8ly-default)
- **At least 3 distinct brand customizations** applied (colors, fonts, custom CSS)
- **Custom onboarding flow** (tenant-specific welcome, not generic)

**Contractual acknowledgment:** Pro+App onboarding contract explicitly states: "First Apple App Store review may be rejected under Apple Guideline 4.2.6. Resubmission after UI variety updates typically succeeds within 1-2 additional cycles. Timeline budget: 2-4 weeks from submission to approval."

### 13.3 Setup fee covers (one-time $2,000 USD)

- Dev-account setup guidance
- Branded iOS + Android build
- First submission + resubmission if rejected
- Firebase push project per tenant
- Named CSM onboarding + 60 days post-launch support

### 13.4 Ongoing (included in $199/mo)

- Named CSM (1h/month)
- SLA 99.9%
- Onboarding concierge
- OS-release-driven updates (2×/year)
- Push infra + crash monitoring
- Tenant retains $99/yr Apple + $25 one-time Google fees

### 13.5 Teardown sequence on churn

When a Pro+App merchant cancels or churns, the apps cannot just vanish — their existing users still have them installed. Define the sequence explicitly:

**Day 0 — cancellation confirmed:**
- CSM sends formal teardown notice to merchant (14 days before next billing)
- Merchant can choose: (a) keep apps live under their own account with basic maintenance ($49/mo "app-only" tier — new), (b) sunset gracefully, (c) immediate pull

**Default path: graceful 60-day sunset**

| Day | Action |
|---|---|
| 0 | Cancellation confirmed. Push notification disabled (Firebase project placed in read-only). |
| 7 | In-app banner deploys: "Service ending in 53 days. Contact [merchant email] for questions." |
| 30 | New downloads blocked (app marked unavailable in both stores). Existing users still open app; storefront shows "Store closed" page. |
| 60 | App pulled from both stores. Firebase project archived. Existing installs show static "service ended" screen on launch. |
| 90 | Firebase project deleted. App removed from tenant's Apple/Google accounts (optional — merchant decides). |

**Merchant-initiated immediate pull**: merchant can request accelerated teardown. Apps pulled within 7 days, Firebase archived immediately.

**Optional "app-only" continuity tier (future consideration, not v1):** a merchant who churns their Pro subscription but wants to keep the apps live under their own maintenance could pay a reduced $49/mo "app-only" fee. Defer to v2; v1 launches with only the default sunset path.

---

## 14. Pro onboarding process

### 14.1 Pro base (no white-label app)

| Stage | Owner | Duration | Deliverable |
|---|---|---|---|
| Contact form submitted | Marketing → Notion | Immediate | Sales record, #sales-inbox notified |
| Sales-qualification reply | Sales | 24h | Pricing confirmation |
| Self-serve annual checkout | Sales | 1 day | Stripe Checkout URL at quoted price |
| Tenant provisioned | Engineering | 2 days post-payment | Pro plan active |
| Time-to-value | Buyer | 30 days from contact | Merchant on Pro |

### 14.2 Pro + App add-on

| Stage | Owner | Duration | Deliverable |
|---|---|---|---|
| Contact form | Marketing → Notion | Immediate | Sales record |
| Discovery call | Sales | 3 days to schedule | 45-min call |
| Quote issued | Sales | 2 days | PDF with price, scope, setup fee, SLA, MSA |
| MSA + DPA signed | Buyer + Sales | 2 weeks typical | DocuSign |
| Setup fee invoice | Finance | 1 day | Stripe invoice NET-15 |
| Setup fee paid | Buyer | 15 days | Stripe confirmation |
| Tenant provisioned | Engineering | 2 days | Pro + App active |
| CSM onboarding call | CSM | 1 week | Goals documented |
| UI customization gate (§13.2) | CSM | Before submission | 3+ brand customizations verified |
| App build + first submission | Engineering | 4–6 weeks | Apps live |
| Time-to-value | Buyer + CSM | 60 days from contact | Merchant live with apps |

### 14.3 Procurement prerequisites (pre-launch)

- MSA template drafted, legally reviewed, versioned
- DPA template (GDPR Article 28 compliant)
- Cyber liability insurance $1M minimum
- SOC 2 posture: v1 launch without SOC 2 acceptable for sub-$100k deals; budget Type I ($25-50k) for first $100k+ deal

---

## 15. Cancellation flow

### 15.1 Merchant-facing

- Entry: admin → Subscription → "Cancel subscription"
- Confirmation dialog: period-end retention, 60-day data retention, store-closed page
- **Exit survey** (required): *Too expensive*, *Missing features*, *Taking a break*, *Switched to competitor*, *Closed my business*, *Other* + optional comment
- **Save offer**: if reason = "Too expensive" and plan ∈ {Starter, Studio}, offer 50% off for next 3 months. Accepted → cancellation reversed (state transition `cancel_scheduled → active` per §17.2), promo tracked in `promo_redemptions`.
- **Final state**: `subscription_status = cancel_scheduled` with `cancels_at = current_period_end`

### 15.2 Post-cancellation

- At `cancels_at`: transitions to `expired`
- 60-day retention window
- Add-card flow available anytime in window

### 15.3 Win-back

Day 30 post-cancellation: win-back email with 20% off for 6 months if they return.

### 15.4 Pro+App cancellation

Triggers white-label app teardown sequence per §13.5.

---

## 16. Failed payment / dunning flow

### 16.1 Trigger

`invoice.payment_failed` webhook → subscription → `past_due`.

### 16.2 Schedule

| Day | Event |
|---|---|
| 0 | `past_due`. Storefront live. Admin fully editable. Email: "Payment failed, retry soon." |
| 1, 3, 5 | Stripe Smart Retries |
| 5 | Second email: "3 failed attempts. Update card by day 7." |
| 7 | Final retry + final email |
| 8 | `expired`. Admin read-only (§17.3). Storefront lives until day 14. |
| 14 | Storefront → "store closed" page |
| 90 | Hard-delete path starts |

### 16.3 Recovery

Card-add at any point before day 90 → subscription `active`. All reminders cancelled.

### 16.4 Tone

Editorial/calm, not urgent/threatening.

### 16.5 Dunning vs refund window

Refund window dominates within first 14 days from first charge.

---

## 17. State transitions & concurrency

### 17.1 State enum

```go
type SubscriptionStatus string

const (
    StatusSignup                 SubscriptionStatus = "signup"
    StatusTrialing               SubscriptionStatus = "trialing"
    StatusActive                 SubscriptionStatus = "active"
    StatusPastDue                SubscriptionStatus = "past_due"
    StatusPaymentActionRequired  SubscriptionStatus = "payment_action_required"
    StatusCancelScheduled        SubscriptionStatus = "cancel_scheduled"
    StatusExpired                SubscriptionStatus = "expired"
    StatusStoreClosed            SubscriptionStatus = "store_closed"
    StatusPendingHardDelete      SubscriptionStatus = "pending_hard_delete"
    StatusHardDeleted            SubscriptionStatus = "hard_deleted"
)
```

### 17.2 Allowed transitions

```
signup → trialing (email verified)
trialing → active (card added before day 90)
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
expired → active (card re-added)
expired → store_closed (day 14 post-expiry)
expired → pending_hard_delete (ONLY if expired_at + 14d AND expired_at + 90d both elapsed; guards skipping grace window)
store_closed → active (card re-added)
store_closed → pending_hard_delete (day 90 post-expiry)
pending_hard_delete → hard_deleted (deletion job)
```

### 17.3 Read-only mode — admin allowlist

When `subscription_status ∈ {expired, store_closed, pending_hard_delete}`:

Middleware `subscription.RequireActive()` blocks mutations except:
- `GET /admin/**`
- `POST /admin/stores/:storeId/billing/*`
- `POST /admin/stores/:storeId/subscription/*`
- `GET /admin/stores/:storeId/orders/export/*`
- `POST /admin/auth/**`

Ordering: `IstioAuth → TenantMiddleware → RequireActive → RequireFeature → handler`.

Returns **HTTP 402 Payment Required**.

**CSM/SLA gate:** allowed only if `HasWhiteLabelAppAddOn = true` AND subscription is `active`.

### 17.4 Concurrency controls

Every subscription write:
1. `SELECT pg_advisory_xact_lock(hashtext(store_id::text))` at start of transaction
2. Re-read row after lock
3. UPDATE with CAS: `WHERE status = :expected AND updated_at = :expected_ts` → 0 rows = retry from step 2
4. Log transition to `subscription_state_log` with actor

### 17.5 Idempotent crons

Pure functions of time + row state. Re-running produces identical state.

### 17.6 Stripe webhook event catalog

Full event list (15 events): `checkout.session.completed`, `customer.subscription.created/updated/deleted`, `customer.updated`, `invoice.created/finalized/paid/payment_failed/payment_action_required`, `charge.refunded`, `payment_method.attached/detached`, `radar.early_fraud_warning`.

### 17.7 Webhook idempotency + orphan handling

```sql
CREATE TABLE stripe_webhook_events (
    event_id         VARCHAR(100) PRIMARY KEY,
    event_type       VARCHAR(100) NOT NULL,
    store_id         UUID,  -- nullable for events awaiting link
    payload          JSONB       NOT NULL,
    received_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    processed_at     TIMESTAMPTZ,
    processing_error TEXT,
    retry_count      INT         NOT NULL DEFAULT 0
);
```

**Flow:**
1. Verify signature (raw body before JSON binding)
2. `INSERT ON CONFLICT (event_id) DO NOTHING`; if existing with `processed_at IS NOT NULL`, return 200 (replay)
3. Process within advisory-locked transaction on `store_id` (or advisory lock on `event_id` for orphans)
4. Update `processed_at` in same transaction
5. On link failure mid-transaction: set `processing_error`, leave `processed_at = NULL`, schedule re-attempt
6. Return 200

**Orphan event SLA:**
- Re-attempt cron runs **every 5 minutes** to retry orphan events (where `store_id IS NULL AND processed_at IS NULL`)
- After **1 hour** of unresolved orphan state: escalate to on-call via PagerDuty
- After **24 hours**: mark as `manual_review_required`; flag in observability dashboard

### 17.8 Idempotency keys — outbound Stripe calls

All mutating calls pass server-generated `Idempotency-Key`:
- Create customer: `customer:{store_id}`
- Checkout: `checkout:{store_id}:{plan}:{period}:{day_bucket}`
- Subscription: `subscription:{store_id}:{plan}:{billing_period}`
- Portal: `portal:{store_id}:{hour_bucket}`
- Refund: `refund:{invoice_id}`

---

## 18. Security & compliance requirements

### 18.1 PCI-A scope

Using Stripe Checkout + Customer Portal keeps Mark8ly in SAQ-A. Forbidden:
- ✗ Store full card, CVV, expiry, PAN
- ✗ Log raw Stripe bodies or webhook payloads
- ✗ Pass card details through Mark8ly API routes
- ✓ Display `card.last4` fetched from Stripe API (don't store)

### 18.2 Multi-tenant isolation (BLOCKER fix)

`GetByStoreID` requires `tenant_id` in query:

```go
GetByStoreID(ctx context.Context, db *gorm.DB, tenantID, storeID uuid.UUID) (*StoreSubscription, error)
```

Call-site audit mandatory in implementation.

### 18.3 Stripe secret key management

| Secret | Location | Rotation |
|---|---|---|
| Stripe AU live secret | `/projects/tesserix-prod/secrets/stripe-au-secret-key-live` | 90d / staff-leave |
| Stripe AU test secret | `/projects/tesserix-dev/secrets/stripe-au-secret-key-test` | Quarterly |
| Webhook endpoint secret | `/projects/tesserix-prod/secrets/stripe-au-webhook-secret-live` | On URL change |
| Merchant BYO gateway keys | `/projects/tesserix-prod/secrets/merchant/{tenant_id}/{provider}-secret` | Merchant-controlled, deleted on hard-delete |

WIF-only access. Audit log monitored. IAM limited to ≤3 staff.

### 18.4 Enterprise API keys

- 32 bytes entropy, `mk8_live_` prefix
- Stored as bcrypt/argon2 hash
- Per-key scopes + rate limits + tenant binding
- Immediate revocation; 24h rotation overlap

### 18.5 Webhook endpoint hardening

- Istio rate limit 100 req/min/IP
- `http.MaxBytesReader` at 512 KB
- Event-type allowlist post-signature
- No body logging

### 18.6 Log sanitization

- Stripe errors: `error.code` + `error.type` only, never full body
- Webhook payloads: `event.id` + `event.type` only
- Redact email, billing address, GSTIN/VAT, raw IP

### 18.7 Account recovery

- Card-add during grace window: self-service
- Post-hard-delete: email verify + director-level approval + two-person review

### 18.8 Geo-pricing anti-arbitrage

Card-country + IP + billing triangulation at subscription creation. Flag-based (not block) to handle diaspora/expat/corporate-card legitimate cases.

**Schema (`subscription_arbitrage_audit`):**

```sql
CREATE TABLE subscription_arbitrage_audit (
    id                   UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    subscription_id      UUID        NOT NULL REFERENCES store_subscriptions(id),
    tenant_id            UUID        NOT NULL,
    store_id             UUID        NOT NULL,
    card_country         CHAR(2),
    billing_country      CHAR(2),
    ip_country           CHAR(2),      -- country derived from IP, NOT raw IP
    ip_hash              VARCHAR(64),  -- SHA-256(raw_ip + rotating_salt), retained for correlation
    resolved_price_tier  VARCHAR(20) NOT NULL, -- 'developed' or 'emerging'
    mismatch_reason      VARCHAR(100),
    flagged_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    reviewed_by          UUID,
    reviewed_at          TIMESTAMPTZ,
    resolution           VARCHAR(30),  -- 'false_positive_cleared' | 'reprice_developed' | 'ongoing'
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ON subscription_arbitrage_audit (flagged_at) WHERE resolution = 'ongoing';
```

**PII handling:**
- **Raw IP is NEVER stored**; only `ip_country` (derived) + `ip_hash` (SHA-256 with rotating salt, rotated every 30 days)
- Retention: 7 years where enforcement action taken; **2 years maximum** for uncleared flags with no outcome
- Access scoped to `billing-ops` IAM role; every read logged to audit-service

**Enforcement action on confirmed arbitrage:**

The quarterly anti-arbitrage audit flags accounts where triangulation mismatch + evidence of intentional spoofing (e.g., VPN IP + non-diaspora context + billing mismatch). Enforcement path:

1. **First flag confirmed as intentional**: re-invoice at **developed-market tier** at next renewal. Merchant email 14 days before with explanation and payment options. **Do not immediately suspend** — merchant keeps storefront live through renewal transition.
2. **If merchant disputes**: support escalation path (see §18.8.1 below). Merchant can present documentary evidence (passport, utility bill, corporate registration) of jurisdiction. CSM reviews within 5 business days.
3. **If merchant refuses to pay developed rate**: cancellation path initiated at next renewal.
4. **If second flag arrives after first resolution**: escalate to management; may result in account termination.

**Concurrency:** the quarterly-audit job that clears flags takes the `store_id` advisory lock to avoid racing concurrent country changes (§4.6).

#### 18.8.1 False-positive resolution path (self-service appeal)

Legitimate cases that produce triangulation flags:
- **Prepaid cards** (Wise, Revolut, privacy.com): card country = issuer country, not cardholder
- **Corporate cards**: card country = HQ, user country different
- **Diaspora / expats**: Indian citizen living in US using India-issued card billed to Indian office

**Self-service flow:**
- Admin UI shows a subtle banner if `arbitrage_flag = true`: "We've noted a discrepancy in your billing details — [Resolve]"
- Click opens a form asking: "Which jurisdiction best describes your business?" + optional document upload
- Submission routes to `billing-ops` queue for 5-business-day review
- Resolved in merchant's favor: `resolution = false_positive_cleared`, continue at current tier
- Not resolved: merchant notified, re-invoice at developed tier at next renewal

---

## 19. Tax compliance — B2B reverse charge

### 19.1 Model

Mark8ly sells B2B SaaS only to registered business entities in 18 countries. Reverse-charge applies in EU/UK/India/SEA where buyer is VAT/GST-registered. Mark8ly does NOT collect VAT/GST on sale (except Australia).

### 19.2 Enforcement

Signup requires valid business tax ID. Validated in real-time against country registry. Invoices include reverse-charge annotation.

### 19.3 Per-country specifics

| Country | Tax | Validator | Reverse charge | Fallback on validator outage |
|---|---|---|---|---|
| US | Federal: none; state nexus eventually | EIN format + **legally-binding business-entity checkbox (§19.3.1)** | N/A | Accept if checkbox signed |
| CA | GST/HST 5-15% | Business Number + **checkbox** | Yes for registered GST/HST | Accept if checkbox signed |
| UK | VAT 20% | HMRC VAT API | Yes for B2B | Provisional; 14-day clock pauses if API down >72h |
| IE + EU (DE, FR, IT, ES, NL) | VAT 19-25% | VIES | Yes for B2B | Provisional; clock pauses |
| AU | GST 10% (Mark8ly AU entity must charge) | ABN Lookup | N/A domestic | Strict, block |
| NZ | GST 15% | IRD | Yes for B2B *(pending counsel confirmation §20.3)* | Provisional; clock pauses |
| India | GST 18% OIDAR | GSTN API | Yes for B2B | Provisional; §4.7 RBI fallback |
| Singapore | GST 9% | ACRA | Yes for B2B | Provisional |
| Malaysia | SST 8% | MOF SST | Partial — monitor | **5-business-day manual review** |
| Thailand | VAT 7% | RD API | Yes for B2B | **5-business-day manual review** |
| Philippines | VAT 12% | BIR | Yes for B2B | **5-business-day manual review** |
| Indonesia | VAT 11% | DJP NPWP | Yes for B2B | **5-business-day manual review** |
| Vietnam | VAT 10% | GDT | Yes for B2B | **5-business-day manual review** |

**SEA capacity threshold:** when the manual-review queue exceeds **30 submissions/week sustained over 2 weeks**, trigger ops capacity review + commit to API enrollment completion within 30 days. Without this, SLA erosion is silent.

**Name cross-check during manual review:** required tax IDs must be verified against registry-returned business name. Fuzzy-match acceptable; mismatch flagged for explicit review. New column `tax_id_name_match` on subscription: `matched | unmatched | not_checked`.

### 19.3.1 US/CA legally-binding business-entity checkbox

Signup form text:

> "☐ I confirm that the purchase is being made by a registered business entity (not an individual consumer) and I have the authority to enter into this agreement on behalf of the business. I understand this affirmation is legally binding and used by Mark8ly for tax-reporting purposes."

**Immutable storage:**

```sql
CREATE TABLE business_entity_attestations (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    store_id        UUID        NOT NULL,
    tenant_id       UUID        NOT NULL,
    country         CHAR(2)     NOT NULL,
    checkbox_text   TEXT        NOT NULL, -- the exact text shown to the user at signup (versioned)
    checkbox_version VARCHAR(20) NOT NULL, -- e.g., "v1.0", "v1.1"
    user_agent      TEXT,
    ip_hash         VARCHAR(64), -- same hashing as §18.8
    signed_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- Append-only. No UPDATE allowed. Trigger prevents modification.
```

Referenced in invoices. Reviewed by Mark8ly legal annually; checkbox text versioned.

### 19.4 Australia-specific

Mark8ly Pty Ltd charges 10% GST + remits to ATO. AU pricing shown **GST-exclusive** with "Plus GST" below price card. Invoice breaks out GST separately.

**Stripe configuration:** AU Price objects must use `tax_behavior: exclusive`. Stripe Tax enabled for AU GST registration. Default `tax_behavior: inclusive` would cause price/invoice mismatch — explicit configuration required at implementation.

### 19.5 Quarterly revalidation

Scheduled job re-checks tax IDs. Invalid → email merchant, 14-day update window; else pause billing until resolved.

### 19.6 Pre-launch tax counsel

Required before launch:
- EU/UK/India reverse-charge applicability for B2B SaaS
- **NZ GST reverse-charge applicability** — NZ has explicit "remote services" GST regime since 2016 that may require registration regardless of B2B status. Counsel opinion critical path (see §20.3).
- AU GST registration confirmed

---

## 20. Legal & TOS requirements

### 20.1 TOS must include

- 14-day cooling-off refund terms
- Non-refundable Pro+App setup fee
- Jurisdictional notices (all 18 countries)
- Subprocessor list (Stripe, SendGrid, GCP, Firebase, Cloudflare)
- GDPR Articles 13/14 disclosure
- India DPDP Act — grievance officer designation (operational dependency: virtual GRO service ~$500-2k/yr if founders not India-resident)
- Right-to-erasure exemption for billing records (AU ITAA s 382-5 — 5yr; EU VAT 242 — 10yr)
- Uptime SLA (Pro+App): 99.9%, calendar-month window, service credits on breach
- Acceptable Use Policy
- US/CA business-entity attestation (§19.3.1)
- AU GST inclusivity disclosure
- PPP pricing disclosure (§4.1.3)

### 20.2 DPA template (Pro+App)

GDPR Article 28 compliant.

### 20.3 Pre-launch legal checklist

| Item | Lead time | Critical path? |
|---|---|---|
| TOS drafted + legally reviewed (AU, UK, EU, IN) | 4–8 weeks | No |
| DPA template | 1–2 weeks | No |
| Cookie policy + GDPR consent | 1 week | No |
| Privacy Policy (DPDP + GDPR) | 2–3 weeks | Operational: grievance officer |
| MSA template for Pro+App | 2–3 weeks | No |
| Cyber liability insurance $1M | 2–4 weeks | Yes if first Pro+App deal imminent |
| **NZ tax counsel confirmation on reverse-charge** | 1–2 weeks opinion + **4–8 weeks registration processing if required** | **YES — critical path; start concurrently with TOS drafting** |
| EU/UK/India tax counsel confirmation | 1–2 weeks | No |

### 20.4 Post-launch legal work

- SOC 2 Type I audit on first $100k+ deal
- EU/UK tax counsel re-confirmation at 100 merchants/country
- India GST OIDAR registration evaluation at ₹20 lakh/mo

---

## 21. Observability requirements

### 21.1 Metrics

- `subscription.state.count{status}` — gauge per status
- `subscription.mrr_{currency}` — gauge per currency
- `subscription.trial.expired_today` — counter
- `subscription.trial.activated_day_30` — **new**, tracks trial-activation funnel
- `subscription.trial.product_created_day_30` — **new**, key activation milestone
- `subscription.payment_failed` — counter
- `subscription.payment_action_required` — **new**, counter (RBI/SCA webhook)
- `subscription.arbitrage_flagged` — counter (§18.8)
- `subscription.arbitrage_false_positive_cleared` — **new**, counter
- `campaign.email.sent{store_id}` — counter
- `webhook.processed{event_type}` — counter
- `webhook.failed{event_type, reason}` — counter
- `webhook.orphan_resolved_after_seconds` — histogram (§17.7)

### 21.2 Logs

Every state transition logs structured JSON with actor, from_status, to_status, plan_before, plan_after, timestamp.

### 21.3 Alerts

- **Trial scheduler dead-man's-switch**: page if cron >25h stale
- **Failed payment spike**: >5% of active subscriptions in 24h
- **Webhook latency P95 >5s**
- **Webhook failure >1% in 1h**
- **Trial signup anomaly**: >50/day
- **Break-glass admin use**: immediate Slack to `#security-alerts`
- **Arbitrage flag spike**: >5× baseline increment
- **Orphan webhook SLA breach**: unresolved >1h → page on-call
- **Trial activation below threshold**: if <30% of trials hit Day-30 product-added milestone over rolling 30d, alert; consider shortening trial

### 21.4 Dashboards

"Subscription Health" dashboard: MRR by currency, active-subscription count by plan, failed-payment trend, upcoming expirations, webhook performance, trial activation funnel.

---

## 22. Disaster recovery

### 22.1 CNPG backup posture

- **PITR**: enabled on `mark8ly-postgres`. 7-day window.
- **Daily snapshots** to GCS. 30-day retention.
- **Subscription-critical tables daily export**: `store_subscriptions`, `stripe_webhook_events`, `promo_redemptions`, `billing_archive`, `subscription_arbitrage_audit`, `business_entity_attestations` via Cloud Scheduler. 90-day retention.

### 22.2 Recovery scenarios

| Scenario | RTO | RPO | Mechanism |
|---|---|---|---|
| Pod restart | 30s | 0 | Knative auto-restart |
| CNPG primary failure (≥100 merchants, sync standby) | **2 min** | 0 | CNPG failover |
| CNPG primary failure (<100 merchants, no standby) | 4h | 24h | GCS snapshot restore |
| Accidental table drop | 1h | 5 min | PITR restore |
| Full cluster loss | 4h | 24h | GCS snapshot restore |
| Data-center loss | 24h+ | 24h | GCS → new region |

**Sync standby moved to 100-merchant tier** (was 500 in v2.1). Churn risk of 4h outage on paying merchants >> incremental compute cost of second CNPG instance (~$10-20/mo).

### 22.3 Stripe + GCS as reconciliation sources

Stripe can reconstruct subscription state but NOT: `promo_redemptions`, `refund_audit`, `subscription_arbitrage_audit`, `business_entity_attestations`. GCS snapshots are the only recovery path for those.

---

## 23. Audit logging & billing archive

### 23.1 Subscription mutations audit

Every write → `audit-service`: created, plan changed, status changed, card attached, refund issued, promo applied, hard-delete scheduled, SSO config changed, add-on purchased/cancelled.

### 23.2 Billing archive (7-year retention)

```sql
CREATE TABLE billing_archive (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    original_store_id    UUID NOT NULL,
    original_tenant_id   UUID NOT NULL,
    business_name        VARCHAR(500) NOT NULL,
    tax_id               VARCHAR(50),
    tax_id_country       CHAR(2),
    billing_country      CHAR(2),
    billing_currency     CHAR(3),  -- new: which currency was billed
    stripe_customer_id   VARCHAR(100) NOT NULL,
    all_invoices         JSONB NOT NULL,
    total_revenue_usd    NUMERIC(12,2),
    hard_deleted_at      TIMESTAMPTZ NOT NULL,
    archive_expires_at   TIMESTAMPTZ NOT NULL
);
```

### 23.3 GDPR erasure

- Live-table PII purged
- `billing_archive` PII retained under legal-obligation basis
- Stripe customer deleted via API
- Merchant notified of retained data + legal basis

---

## 24. Database sizing — CNPG staircase

| Merchant count | CPU | Memory | Storage | Sync standby | Notes |
|---|---|---|---|---|---|
| 0–100 | 0.5 CPU | 2 GiB | 50 GiB | No | Launch sizing; 4h RTO on primary failure |
| **100–500** | **1 CPU** | **4 GiB** | **200 GiB** | **Yes (sync standby added)** | **RTO drops to 2 min. PgBouncer pool.** |
| 500–2,000 | 2 CPU | 8 GiB | 500 GiB | Yes | WAL archiving tuned |
| 2,000–10,000 | 4 CPU | 16 GiB | 1 TiB | Yes + async replica | Scale-out considerations |
| 10,000+ | Evaluate horizontal partitioning | 32+ | 2+ TiB | Yes + async | Multi-tenant sharding or Cloud Spanner |

**Upgrade triggers:** >80% CPU/memory sustained 1h; P95 query latency >500ms; 80% storage.

---

## 25. Implementation gaps

| Area | Effort |
|---|---|
| **Backend — plans & billing** | |
| Update `plangate.go` to 4-plan matrix | 1 day |
| Migration: tax ID fields, plan enum rename, `billing_currency`, `price_tier`, add-on tracking, `arbitrage_flag` | 1 day |
| Fix `GetByStoreID` + call-site audit | 2 days |
| Stripe multi-currency Price objects + Checkout + Customer Portal | 1.5 weeks |
| Stripe split-currency settlement config (USD-AU account + AUD account) | 1 day |
| Webhook dispatcher (§17.6, §17.7) + orphan 5-min re-attempt cron | 1 week |
| State machine + advisory locking + idempotency | 4 days |
| Plan upgrade/downgrade + store-block + image-limit grandfathering | 5 days |
| Campaign budget pattern + trial volume ramp (§5.1) | 4 days |
| Read-only middleware + allowlist (§17.3) | 2 days |
| Trial + dunning crons | 1 week |
| Stripe-native challenge fallback (`invoice.payment_action_required` → invoice flow) | 4 days |
| Promo codes + abuse prevention | 1 week |
| Refund flow v1 (Stripe Dashboard + `refund_audit`) | 2 days |
| Store-closed page at Cloudflare Worker (HTTP 200 + noindex) | 3 days |
| Tax ID validation (12 validators + clock-pause logic) | 2 weeks |
| US/CA business-entity checkbox + immutable storage (§19.3.1) | 2 days |
| Reverse-charge invoice template + logic | 1 week |
| AU Stripe `tax_behavior: exclusive` + Stripe Tax AU GST config | 1 day |
| Billing archive + hard-delete hook | 2 days |
| Geo-pricing anti-arbitrage + `subscription_arbitrage_audit` schema + IP hashing + self-service appeal | 4 days |
| SEA manual-review with name cross-check + queue capacity monitoring | 3 days |
| White-label app teardown sequence (§13.5) | 3 days |
| **Backend — shipping** | |
| Add IE, NZ to ShipEngine | 1 day |
| Add VN to NinjaVan | 1 day |
| AE / Aramex carrier (deferred v2) | 1 week |
| **Frontend — admin** | |
| Pricing page (geo-localized, 18 countries, local-currency display) | 1 week |
| Plan management UI | 1 week |
| Cancellation flow + survey + save offer | 3 days |
| Email usage counter with ramp visibility | 2 days |
| Tax ID field + 14-day reminder banners + clock-pause indicator | 3 days |
| Failed-payment banner + add-card flow | 2 days |
| Contact-sales form for Pro + Notion + Slack | 2 days |
| Pro+App add-on purchase flow | 2 days |
| Arbitrage self-service appeal form | 2 days |
| Migration fast-path upload (§5.1.1) | 1 day |
| Store-close-before-downgrade UX (§4.5.1) | 3 days |
| **Security + compliance** | |
| PCI-A code audit | 2 days |
| GCP Secret Manager + WIF | 2 days |
| Break-glass + TOTP + rotation | 3 days |
| Enterprise API keys (Pro) | 1 week |
| Webhook endpoint hardening | 2 days |
| **Observability** | |
| Subscription metrics + trial activation funnel | 3 days |
| Dashboards | 1 day |
| Alerts | 2 days |
| **SSO (Pro / Pro+App)** | 2-3 weeks |
| **White-label app (first deal)** | 4-6 weeks |

**v1 self-serve launch (Starter + Studio + Trial + Pro base contact-sales):** ~9–11 engineer-weeks back-end + ~3 weeks front-end.

**Pro + App first deal:** adds ~4–6 weeks.

---

## 26. Risks & open questions

### 26.1 Pre-launch blockers

1. **Legal review of TOS, Privacy, DPA** — $15–30k or staff lawyer time. Lead: TOS drafting.
2. **NZ tax counsel + potential GST registration (critical path)** — 1-2 weeks opinion + 4-8 weeks processing if required.
3. **India RBI mandate flow tested end-to-end** via `invoice.payment_action_required` webhook.
4. **Tax ID validators operational** — 12 countries, SEA may require enrollment.
5. **Cyber liability $1M insurance** before first Pro+App.
6. **India DPDP grievance officer designated** — virtual GRO service if founders not India-resident.

### 26.2 Strategic risks

1. **SendGrid cost at 500+ merchants** — SES migration evaluated.
2. **Stripe AU acquiring rate for US/UK cards** — 1-3% lower auth; revisit Delaware + Stripe US at $500k ARR if merchant mix skews US/UK.
3. **White-label app build automation** — critical before 2nd deal. **First 2-3 Pro+App deals are deliberate loss leaders** — margin positive only after §13.4 automation pipeline lands.
4. **Apple 4.2.6 template-app rejection** — ~30% first-review rejection rate. CSM UI-variety gate mitigates.
5. **Trial abuse** — evaluate card-BIN verification if gates insufficient.
6. **Geo-arbitrage** — triangulation catches most; diaspora/expat edge cases require manual review. False-positive appeal path in §18.8.1.
7. **Pro at $99 contact-sales friction** — if trial-to-Pro conversion lags, revisit making Pro base self-serve at 3-month post-launch review.
8. **90-day trial long-tail "tourists"** — telemetry-driven iteration: if <30% of trials hit Day-30 product-added milestone, shorten trial to 30d with one-click merchant extension.

### 26.3 Open questions

1. **SOC 2 timing** — trigger at first $100k deal or 18 months?
2. **Grandfathered launch rate** — at v1? Slots?
3. **Pro+App continuity "app-only" tier** ($49/mo maintenance-only post-Pro-churn) — v2 evaluation?

### 26.4 Out of scope

- Marketplace plan features (placeholder only)
- Payment gateway aggregation (never)
- Customer-facing discounts on storefront (separate product)
- Multi-language + multi-currency storefronts (future)
- WhatsApp Business API (future)
- AI-powered merchant tooling (future)

---

## 27. Decision log

| Decision | Rationale |
|---|---|
| 3-month trial | Merchants take time to launch. Generous trial = quality conversion. Telemetry-driven iteration if <30% Day-30 activation. |
| No card at signup | Editorial positioning, low friction, trust product to earn card by day 60. |
| Trial campaign volume ramp (not 7-day block) | Protects against spam without penalizing merchants importing catalogs on day 1. |
| B2B-only, reverse-charge | Eliminates Mark8ly VAT/GST registration burden in 12+ jurisdictions. |
| 4-tier pricing | Trial + Starter + Studio + Pro covers indie → mid-market. |
| $99 Pro base | CSM/SLA moved to add-on; Pro accessible to mid-market brands wanting features, not full concierge. |
| Pro visible $99 floor | Self-qualifies prospects; $99 is standard Pro price, not just lowest. |
| White-label App add-on bundles CSM/SLA | Aligns dedicated human cost with customer who values it. |
| PPP discount for 6 emerging markets | Industry standard; without it, Starter ~3% of revenue for indie India DTC = unaffordable. |
| USD parity for 12 developed markets | Willingness-to-pay supports parity. |
| Native local-currency billing | Display = charge = statement. Matches editorial truthfulness. |
| Split-currency Stripe settlement (USD to USD account) | Recovers 1.5% FX spread on USD revenue; natural hedge on USD cost base. |
| Zero transaction fee | Genuinely differentiated from Shopify. |
| Flat monthly | Rejected GMV-tier model on brand grounds. |
| 14-day cooling-off refund | Legally bulletproof 18 countries; practically rare post-trial. |
| Pro contact-sales with visible floor | Self-qualification + flexibility. Revisit if conversion lags. |
| First Pro+App deals are loss leaders | Unit economics positive only after §13.4 build automation. |
| Mobile app via tenant-owned dev accounts | Apple 4.2.6 workaround; industry standard. |
| Stripe-native challenge fallback (webhook-driven) | Generalizes across RBI/SCA/3DS; no hardcoded thresholds. |
| Sync standby at 100 merchants (not 500) | Outage during first product launch = immediate churn. Worth $10-20/mo. |
| Excess-store downgrade block + soft-delete-inclusive check + re-check at cron | Prevents silent over-quota on store restore. |
| Image-limit grandfathering on downgrade | Existing products keep images; new uploads enforce limit. |
| White-label app 60-day sunset on churn | Respects existing app users while freeing Mark8ly obligations. |
| HTTP 200 + noindex for store-closed (not 307) | Worker serves HTML directly; no redirect. |
| Orphan webhook 5-min re-attempt + 1h escalation | Unactivated paid subscription = revenue failure; explicit SLA required. |
| Arbitrage: flag + review, not block; false-positive appeal path | Handles legitimate diaspora/expat cases without hurting revenue. |
| 14-day tax-ID validation with clock-pause on registry outage | Closes attack surface without punishing merchants for API downtime. |
| US/CA business-entity checkbox + immutable storage | Closes B2B enforcement gap where no federal validator exists. |
| SEA 30/week manual-review capacity trigger | Prevents silent SLA erosion; forces API enrollment or ops capacity decision. |
| AU Stripe `tax_behavior: exclusive` | Prevents price/invoice mismatch from default-inclusive pitfall. |
| NZ tax counsel critical-path sequencing | Potential 8-week registration lead time cannot block launch. |
| Migration fast-path 48h validation | Serious merchants with existing storefronts shouldn't suffer 14-day SEO darkness. |
| IP hashed with rotating salt, not stored raw | GDPR compliance; preserves correlation capability. |
| Single Stripe AU account | Operational simplicity; revisit at $500k ARR if merchant mix skews US/UK. |

---

## 28. Success criteria

| # | Criterion | Test type |
|---|---|---|
| 1 | New merchant signs up → email verified → reCAPTCHA pass → tax ID validated → trial store live | Integration e2e |
| 2 | Merchant signs up → tax ID validation pending → storefront unpublished → validated on day 10 → publishes | Integration |
| 3 | Tax ID registry down >72h → 14-day clock pauses → resumes when reachable | Integration (time-mocked) |
| 4 | Trial day 91 no card → admin read-only, storefront "closed" page served from Worker (200 OK + noindex) | Integration |
| 5 | Trial day 91 adds card → immediate `active`, charged in merchant's local currency | Integration |
| 6 | Merchant upgrades Starter → Studio: immediate prorate, currency unchanged | Integration |
| 7 | Merchant with 5 Studio stores + in-flight orders tries Starter downgrade: blocked with per-store order counts | Integration |
| 8 | Studio → Starter downgrade: existing products grandfathered to 50 images, new uploads capped at 25 | Integration |
| 9 | Soft-deleted store restored during downgrade grace period: downgrade execution re-check catches over-quota | Integration |
| 10 | Merchant cancels annual month 7: access until month 12, no refund, survey captured | Integration |
| 11 | Merchant accepts save-offer: `cancel_scheduled → active`, promo applied | Integration |
| 12 | Pro contact form → Notion + Slack within 60s | Integration |
| 13 | Payment fails: `past_due`, 3 retries, 3 reminder emails, day 8 read-only | Integration |
| 14 | Campaign send on trial day 2 of 600 recipients: blocked (500 cap); recipient 2,100 on day 5 blocked (2k cap); day 8 recipient 4,999 allowed | Integration |
| 15 | Create 26th product media on Starter: blocked | Integration |
| 16 | Tenant A cannot read Tenant B's subscription via URL manipulation | **Security test** |
| 17 | Stripe webhook replay: idempotent, no double-processing | **Security test** |
| 18 | 14-day cooling-off refund: Stripe Dashboard + `refund_audit` row | Manual UAT |
| 19 | Indian annual subscriber hits `invoice.payment_action_required`: invoice flow triggered with UPI link | Manual UAT |
| 20 | Break-glass admin login: TOTP + alert to #security-alerts | Integration |
| 21 | Pricing page localized prices for all 18 countries; PPP-correct for emerging markets | Manual UAT |
| 22 | Stripe charges Indian merchant ₹999 directly (not USD cross-border conversion) | Integration |
| 23 | USD-billed merchant's revenue settles to USD-AU account (not converted to AUD) | Integration |
| 24 | Support team issues refund via Stripe Dashboard with eligibility check | Runbook |
| 25 | MRR metric per currency updates within 60s | Integration |
| 26 | Trial scheduler dead-man's-switch fires if cron skipped | Chaos/SRE |
| 27 | B2B enforcement: US signup without business-entity checkbox rejected | Integration |
| 28 | Hard-delete → `billing_archive` row persisted with 7-year retention | Integration |
| 29 | Merchant's billing country changes EU → India: price + currency change applied at next renewal only | Integration |
| 30 | India merchant card issued in US + billing in India: developed-tier pricing applied, arbitrage flag set, self-service appeal form accessible | Integration |
| 31 | Arbitrage false-positive cleared via appeal: resolution = `false_positive_cleared`, pricing unchanged | Integration |
| 32 | Orphan webhook event persists unresolved 65 minutes: on-call paged via PagerDuty | Chaos/SRE |
| 33 | SEA manual-review queue exceeds 30/week for 2 weeks: ops alert triggered | Operational |
| 34 | Pro+App merchant cancels: 60-day sunset sequence executes (Firebase read-only day 0, banner day 7, new downloads blocked day 30, app pulled day 60, Firebase deleted day 90) | Integration |
| 35 | Pro+App app submitted with insufficient UI variety: CSM gate blocks submission pending customization | Operational |
| 36 | AU merchant invoice breaks out 10% GST separately from GST-exclusive base price | Manual UAT |

---

## 29. Next step

After user approval, proceed to **implementation plan** via `writing-plans` skill. Plan will decompose §25 gap list into atomic sequenced tasks with tests-first structure and atomic commit boundaries, targeting ~9–11 engineer-weeks back-end + ~3 weeks front-end for v1 self-serve launch, plus ~4–6 weeks for first Pro+App customer.
