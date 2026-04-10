# Storefront Branding, Subscription Tiers & Mobile App — Design Spec

**Date:** 2026-04-10
**Status:** Approved — reviewed by specialists

## 1. Overview

Three milestones plus pricing/feature framework: storefront branding customization (logo, full color palette, fonts, layout, footer, announcement bar), subscription tier feature gating (Free/Starter/Pro/Enterprise/Marketplace with zero transaction fees), and an React Native mobile app wrapper for Enterprise+ merchants.

### Build order

1. **B1 — Storefront Branding** (admin settings UI + storefront theme application)
2. **B2 — Subscription Tiers** (feature gating logic, regional pricing, trial management)
3. **B3 — Expo Native App** (wrapper app, merchant branding, build pipeline)

### Constraints

- Same repos (`mark8ly` for branding + gating, new `apps/mobile` for Expo app)
- Branding stored per-store in database, applied via CSS custom properties
- Subscription gating checks `store_subscriptions.plan` — requires S3 (Subscription) to be shipped first
- Expo app is a thin wrapper around the storefront URL — not a separate codebase
- Color validation: reject low-contrast combinations that break readability
- **Zero transaction fees** — revenue from subscriptions only
- Marketplace tier is v2 (multi-vendor) — plan definition included, implementation separate

## 2. Pricing

### 2.1 Positioning

**"0% transaction fees. Your revenue is yours."**

mark8ly undercuts every major competitor on the one thing merchants hate most. Revenue comes from subscriptions only. The upgrade motivation is feature access, not fee savings.

### 2.2 Regional pricing

**US / EU / AU / UK / CA / SG (USD)**

| | **Free** | **Starter** | **Pro** | **Enterprise** | **Marketplace** |
|---|---------|------------|---------|---------------|-----------------|
| Monthly | $0 | $9.99 | $29.99 | $99.99 | $249.99 |
| Annual (per month) | $0 | $7.99 (save 20%) | $24.99 (save 17%) | $83.99 (save 16%) | $208.99 (save 16%) |
| Trial | Permanent | 3 months free | 14-day | Contact sales | Contact sales |
| Transaction fee | **0%** | **0%** | **0%** | **0%** | **0%** |

**India (INR)**

| | **Free** | **Starter** | **Pro** | **Enterprise** | **Marketplace** |
|---|---------|------------|---------|---------------|-----------------|
| Monthly | ₹0 | ₹499 | ₹1,499 | ₹4,999 | ₹14,999 |
| Annual (per month) | ₹0 | ₹399 (save 20%) | ₹1,249 (save 17%) | ₹4,166 (save 17%) | ₹12,499 (save 17%) |
| Trial | Permanent | 3 months free | 14-day | Contact sales | Contact sales |

**SEA — MY, TH, PH, ID (USD)**

| | **Free** | **Starter** | **Pro** | **Enterprise** | **Marketplace** |
|---|---------|------------|---------|---------------|-----------------|
| Monthly | $0 | $6.99 | $19.99 | $49.99 | $124.99 |
| Annual (per month) | $0 | $5.99 | $16.99 | $41.99 | $104.99 |
| Trial | Permanent | 3 months free | 14-day | Contact sales | Contact sales |

**Mobile app setup fee (Enterprise + Marketplace):**

| Region | Setup fee | Per rebuild after first 2 |
|--------|----------|--------------------------|
| US/EU/AU | $499 one-time | $99 |
| India | ₹24,999 one-time | ₹4,999 |
| SEA | $249 one-time | $49 |

### 2.3 Trial strategy

| Segment | Starter trial | Pro trial |
|---------|--------------|-----------|
| India / SEA | 3 months free | 14-day |
| US / EU / AU | 14 days free, then $1 first month | 14-day |

After trial: **soft downgrade** to Free tier limits (not cancellation). Merchant keeps their store, data, and customers. Excess items (products above 25, staff above 1) become **read-only** — visible and editable but no new items can be created until they upgrade or reduce. 30-day grace period with email warnings at day 7, day 1, and day 0. No data deletion.

### 2.4 GMV-based upgrade nudge (Starter)

Dashboard shows "You processed $X this month. Pro merchants save on average Y% with unlimited orders" when Starter merchant crosses $5K GMV/month. Not a hard cap — just a visible nudge. Tracks which gate triggers the upgrade decision (product limit, staff limit, GMV nudge, feature attempt) for analytics.

### 2.4 Implementation

- Stripe Billing with multi-currency Price IDs per region per plan
- Region auto-detected from store's `country_code` in `supported_countries`
- Price IDs stored in env vars, not hardcoded
- `platform_fee_configs` and `platform_fee_ledger` tables retained but dormant (fee_percent = 0, fee_fixed = 0) for future flexibility

## 3. Feature Matrix

### 3.1 Complete feature gating

| Feature | **Free** | **Starter** | **Pro** | **Enterprise** | **Marketplace** |
|---|---|---|---|---|---|
| **Store** |
| Products | 25 | 500 | Unlimited | Unlimited | Unlimited |
| Categories | 5 | 25 | Unlimited | Unlimited | Unlimited |
| Staff members | 1 (owner) | 3 | 10 | Unlimited | Unlimited |
| Stores | 1 | 1 | 3 | 10 | 10 |
| **Branding** |
| Logo + favicon | Yes | Yes | Yes | Yes | Yes |
| Accent color only | Yes | — | — | — | — |
| Full color palette + fonts | — | Yes | Yes | Yes | Yes |
| Announcement bar | — | Yes | Yes | Yes | Yes |
| Social links + footer | Yes | Yes | Yes | Yes | Yes |
| Remove "Powered by mark8ly" | — | — | Yes | Yes | Yes |
| Custom CSS | — | — | — | Yes | Yes |
| Custom domain | — | — | Yes | Yes | Yes |
| **Orders & Checkout** |
| Orders/month | 50 | 500 | Unlimited | Unlimited | Unlimited |
| Storefront checkout | Yes | Yes | Yes | Yes | Yes |
| Abandoned cart tracking | Yes | Yes | Yes | Yes | Yes |
| Returns & refunds | — | Yes | Yes | Yes | Yes |
| **Payments** |
| Stripe/Razorpay/PayPal | Yes | Yes | Yes | Yes | Yes |
| Transaction fees | **0%** | **0%** | **0%** | **0%** | **0%** |
| **Shipping** |
| Shipping rate calculation | Yes | Yes | Yes | Yes | Yes |
| Label creation + tracking | — | Yes | Yes | Yes | Yes |
| **Tax** |
| Flat rate / GST / TaxJar | Yes | Yes | Yes | Yes | Yes |
| **Marketing** |
| Coupons | 5 active | 50 active | Unlimited | Unlimited | Unlimited |
| Gift cards | — | Yes | Yes | Yes | Yes |
| Loyalty program | — | — | Yes | Yes | Yes |
| Campaigns | — | 5/month | 50/month | Unlimited | Unlimited |
| **Customers** |
| Customer profiles | Yes | Yes | Yes | Yes | Yes |
| Customer accounts (auth) | Yes | Yes | Yes | Yes | Yes |
| Reviews | — | Yes | Yes | Yes | Yes |
| **Support** |
| Help center | Yes | Yes | Yes | Yes | Yes |
| Support tickets | — | Yes | Yes | Yes | Yes |
| Priority support | — | — | — | Yes | Yes |
| **Analytics** |
| Basic dashboard | Yes | Yes | Yes | Yes | Yes |
| Audit logs | — | — | Yes | Yes | Yes |
| **Apps** |
| Expo mobile app | — | — | — | Yes | Yes |
| **Import/Export** |
| CSV import/export | — | Yes | Yes | Yes | Yes |
| **Marketplace (v2)** |
| Vendor onboarding | — | — | — | — | Yes |
| Per-vendor dashboard | — | — | — | — | Yes |
| Commission management | — | — | — | — | Yes |
| Auto payout splits | — | — | — | — | Yes |
| Multi-vendor cart | — | — | — | — | Yes |

### 3.2 Feature gating architecture

New package: `internal/plangate/gate.go`

```go
type Plan string

const (
    PlanFree       Plan = "free"
    PlanStarter    Plan = "starter"
    PlanPro        Plan = "pro"
    PlanEnterprise Plan = "enterprise"
    PlanMarketplace Plan = "marketplace"
)

type Feature string

// 25+ feature constants defined here

type Limit struct {
    Feature Feature
    Plan    Plan
    Max     int // -1 = unlimited, 0 = disabled
}

// IsAllowed checks boolean feature access
func IsAllowed(plan Plan, feature Feature) bool

// GetLimit returns numeric limit for a feature on a plan
func GetLimit(plan Plan, feature Feature) int

// Gin middleware
func RequirePlan(minPlan Plan) gin.HandlerFunc

// Gin middleware for numeric limits
func EnforceLimit(feature Feature) gin.HandlerFunc
```

Frontend: `useSubscription()` hook returns plan + limits. `<PlanGate feature="loyalty" fallback={<UpgradePrompt />}>` component.

## 4. B1 — Storefront Branding

### 4.1 Data model

**Migration 000020 — Store branding**

```sql
CREATE TABLE store_branding (
    id                  UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           UUID          NOT NULL,
    store_id            UUID          NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    -- Identity
    logo_url            TEXT,
    favicon_url         TEXT,
    tagline             VARCHAR(200),
    -- Colors (hex values)
    color_background    VARCHAR(7)    NOT NULL DEFAULT '#F7F6F2',
    color_text          VARCHAR(7)    NOT NULL DEFAULT '#0E0E0C',
    color_accent        VARCHAR(7)    NOT NULL DEFAULT '#2D4A2B',
    color_button_bg     VARCHAR(7)    NOT NULL DEFAULT '#0E0E0C',
    color_button_text   VARCHAR(7)    NOT NULL DEFAULT '#F7F6F2',
    -- Typography
    heading_font        VARCHAR(50)   NOT NULL DEFAULT 'source-serif-4',
    body_font           VARCHAR(50)   NOT NULL DEFAULT 'source-sans-3',
    -- Homepage
    layout_variant      VARCHAR(30)   NOT NULL DEFAULT 'classic-shop',
    hero_image_url      TEXT,
    announcement_text   VARCHAR(300),
    announcement_link   TEXT,
    announcement_bg     VARCHAR(7),
    announcement_active BOOLEAN       NOT NULL DEFAULT false,
    -- Footer
    footer_tagline      VARCHAR(300),
    footer_copyright    VARCHAR(200),
    social_instagram    VARCHAR(300),
    social_twitter      VARCHAR(300),
    social_facebook     VARCHAR(300),
    social_tiktok       VARCHAR(300),
    social_youtube      VARCHAR(300),
    -- Advanced (Enterprise)
    custom_css          TEXT,
    show_powered_by     BOOLEAN       NOT NULL DEFAULT true,
    -- Timestamps
    created_at          TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ   NOT NULL DEFAULT now(),
    UNIQUE (store_id)
);
```

### 4.2 Font options

| Key | Name | Style |
|-----|------|-------|
| `source-serif-4` | Source Serif 4 | Editorial serif (default) |
| `playfair-display` | Playfair Display | Elegant serif |
| `lora` | Lora | Readable serif |
| `inter` | Inter | Clean sans |
| `source-sans-3` | Source Sans 3 | Neutral sans (default body) |
| `dm-sans` | DM Sans | Modern sans |

### 4.3 Color contrast validation

WCAG AA minimum:
- Text on background: >= 4.5:1
- Button text on button background: >= 4.5:1
- Accent on background: >= 3:1

Reject with helpful inline error. Calculate contrast ratio using relative luminance formula.

### 4.4 API endpoints

**Admin:**
- `GET /admin/stores/:storeId/branding` — get current branding
- `PUT /admin/stores/:storeId/branding` — update (plan-gated per field)
- `POST /admin/stores/:storeId/branding/logo` — upload logo (GCS)
- `POST /admin/stores/:storeId/branding/favicon` — upload favicon (GCS)
- `POST /admin/stores/:storeId/branding/hero` — upload hero image (GCS)

**Storefront:**
- `GET /api/v1/storefront/stores/:slug/branding` — public branding (cached, no auth)

### 4.5 Storefront theme application

Storefront layout reads branding and injects CSS custom properties:
```tsx
<style>{`:root {
  --paper-200: ${branding.color_background};
  --ink-900: ${branding.color_text};
  --moss-700: ${branding.color_accent};
  --button-bg: ${branding.color_button_bg};
  --button-text: ${branding.color_button_text};
  --font-heading: '${fontFamily(branding.heading_font)}', serif;
  --font-body: '${fontFamily(branding.body_font)}', sans-serif;
}`}</style>
```

Existing components already use these CSS variables — they pick up merchant colors automatically.

### 4.6 Admin UI

`/settings/storefront` enhanced with sections:

```
Identity: logo upload, favicon upload, tagline
Colors: 5 color pickers with live preview + contrast warnings (plan-gated)
Typography: heading + body font dropdowns with preview (plan-gated)
Homepage: layout variant visual selector, hero image, announcement bar
Footer: tagline, copyright, 5 social link inputs
Advanced: custom CSS textarea, "Powered by" toggle (plan-gated)
```

Live preview: iframe pointing to storefront with `?preview=true&branding=<base64>`.

### 4.7 Custom CSS sanitization (Enterprise)

Strip before storage:
- `@import` rules (prevent loading external resources)
- `url()` with external domains (allow only GCS bucket URLs)
- `javascript:` expressions
- `expression()` (IE legacy)
- `behavior:` (IE legacy)

## 5. B3 — React Native Mobile App

### 5.1 Architecture

```
apps/mobile/                    # Expo + React Native app
├── app.json                    # Dynamic per merchant
├── app/
│   ├── (tabs)/
│   │   ├── index.tsx           # Home / featured products
│   │   ├── shop.tsx            # Product catalog browse
│   │   ├── cart.tsx            # Cart
│   │   ├── orders.tsx          # Order history
│   │   └── account.tsx         # Account / profile
│   ├── product/[handle].tsx    # Product detail
│   ├── checkout.tsx            # Native checkout flow
│   └── category/[slug].tsx     # Category browse
├── components/
│   ├── ProductCard.tsx
│   ├── CartProvider.tsx
│   ├── VariantSelector.tsx
│   └── ReviewSection.tsx
├── lib/
│   ├── api.ts                  # marketplace-api client (same endpoints as storefront)
│   ├── push-notifications.ts   # Expo push notifications
│   ├── branding.ts             # Load merchant branding → theme
│   └── offline.ts              # Offline product cache
├── assets/
│   └── splash-template.png
└── scripts/
    └── build-for-merchant.ts   # Per-merchant build script
```

Proper React Native app with native UI components, not a WebView wrapper. Uses the same storefront API endpoints as the web storefront. Includes:
- Native tab navigation (Home, Shop, Cart, Orders, Account)
- Native product catalog with offline caching
- Native checkout flow
- Push notifications via Expo Push
- Merchant branding applied as a theme (colors, fonts, logo)
- Deep linking for product URLs

### 5.2 Build pipeline

1. Admin clicks "Build Mobile App" in Enterprise settings
2. Backend creates build job with merchant branding config
3. CI pipeline (EAS Build or GitHub Actions):
   - Clone `apps/mobile` template
   - Inject merchant's `app.json` (name, icon, splash, bundleId, scheme)
   - Apply merchant's branding theme (colors, fonts, logo)
   - Build iOS + Android via EAS Build
   - Upload to App Store Connect / Google Play Console (via Fastlane)
4. Admin shows build status + download links + store URLs
5. First 2 rebuilds included in setup fee. Additional rebuilds at per-rebuild rate.

### 5.3 Data model

**Migration 000021 — Mobile app**

```sql
CREATE TABLE mobile_app_configs (
    id                  UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           UUID          NOT NULL,
    store_id            UUID          NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    app_name            VARCHAR(100)  NOT NULL,
    bundle_id           VARCHAR(200)  NOT NULL,
    ios_team_id         VARCHAR(20),
    android_package     VARCHAR(200),
    icon_url            TEXT,
    splash_url          TEXT,
    primary_color       VARCHAR(7),
    status              VARCHAR(20)   NOT NULL DEFAULT 'draft',
    last_build_at       TIMESTAMPTZ,
    last_build_status   VARCHAR(20),
    ios_app_store_url   TEXT,
    android_play_url    TEXT,
    created_at          TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ   NOT NULL DEFAULT now(),
    UNIQUE (store_id)
);

CREATE TABLE mobile_app_builds (
    id                  UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    config_id           UUID          NOT NULL REFERENCES mobile_app_configs(id) ON DELETE CASCADE,
    platform            VARCHAR(10)   NOT NULL,
    version             VARCHAR(20)   NOT NULL,
    build_number        INT           NOT NULL,
    status              VARCHAR(20)   NOT NULL DEFAULT 'queued',
    eas_build_id        VARCHAR(100),
    artifact_url        TEXT,
    error_message       TEXT,
    started_at          TIMESTAMPTZ,
    completed_at        TIMESTAMPTZ,
    created_at          TIMESTAMPTZ   NOT NULL DEFAULT now()
);
CREATE INDEX mab_config_idx ON mobile_app_builds (config_id);
```

### 5.4 Admin UI

```
/settings/mobile-app            (Enterprise+ only)
├── App config: name, bundle ID, icon, splash, colors
├── iOS: Team ID, App Store Connect link
├── Android: package name, Play Console link
├── Build: "Build iOS" / "Build Android" buttons
├── Build history table
└── Status: draft → building → published
```

## 6. Security

- **Branding:** Color values hex-validated, contrast checked. Uploads via GCS with content-type validation. Custom CSS sanitized (no @import, no external URLs, no javascript:).
- **Subscription gating:** Plan checked server-side via `plangate.IsAllowed()`. Frontend `<PlanGate>` is UX only — never trust client.
- **Pricing:** Stripe Price IDs in env vars, never in source code. Regional pricing resolved server-side from store country.
- **Mobile app:** Merchant credentials stored encrypted. Build pipeline in isolated CI. API calls use same storefront auth + X-Storefront-Key.

## 7. Review Findings (from Architect, Business Analyst, UX Specialist)

### 7.1 Architecture fixes
- **Soft downgrade policy (P0):** Excess items become read-only on downgrade (visible, editable, no new creates). 30-day grace with email warnings at day 7, 1, 0. No data deletion.
- **Limit count caching (P1):** Cache product/order counts in-memory per store with reconciliation on create/delete. Don't run COUNT queries on every write.
- **Branding cache (P2):** 5-min TTL on public branding endpoint + explicit purge on admin save. Inject `branding_version` query param so open tabs detect staleness.

### 7.2 UX fixes
- **Tabbed branding settings (Critical):** 6-tab left-rail navigation (Identity, Colors, Typography, Layout, Footer, Advanced) instead of single scroll. Live preview persists in right panel on desktop, modal on mobile.
- **Color presets (Critical):** 6-8 curated palettes above the color pickers. One-click applies all 5 values with guaranteed contrast. Advanced users override individual pickers.
- **Live preview: PostMessage bridge (P1):** Storefront preview listens for messages and applies CSS variables directly — no iframe URL reload. On mobile (<768px), replace persistent iframe with "Preview" modal on tap.
- **Font preview (P2):** Render each dropdown option in its actual typeface.
- **Trial expiry banners (P2):** 7-day and 1-day pre-expiry banners in admin header. Post-downgrade: dismissible banner with diff-style list of what changed.

### 7.3 Business/pricing fixes
- **SEA Starter raised to $6.99** from $4.99 (margin concern)
- **Enterprise raised to $99.99** from $79.99 (multi-store + mobile app justifies premium)
- **Mobile app setup fee** ($499 USD / ₹24,999 / $249 SEA one-time)
- **Customer accounts moved to Free tier** (broken order tracking otherwise)
- **GMV nudge on Starter** at $5K/month (dashboard indicator, not hard cap)
- **Instrument upgrade trigger events** — track which feature gate drives each upgrade

### 7.4 Mobile app
- **Proper React Native app, not WebView** — native UI, navigation, offline product cache, deep linking. Addresses Apple App Store 4.2 rejection risk.

## 8. Testing

- **B1:** Branding CRUD, contrast validation, storefront renders merchant colors, live preview via PostMessage, logo/favicon upload, plan gating (free = accent only), custom CSS sanitization, color presets, branding cache purge on save
- **B2:** Plan gate per feature × plan, middleware 403 with upgrade message, numeric limits (25 products on free), soft downgrade (excess read-only), trial expiry + grace period, regional pricing, Stripe webhook updates plan, GMV nudge triggers
- **B3:** Mobile app config CRUD, build job creation/status, plan gating (Enterprise+), React Native renders merchant branding, push notification registration, offline product cache

## 9. Out of Scope

- Custom email templates
- White-label admin panel
- Multiple mobile apps per store
- Push notification content customization
- App Store review automation (manual submission initially)
- A/B testing storefront themes
- Custom font upload (curated list only)
- Marketplace tier implementation (v2 — separate spec)
