# Storefront Branding, Subscription Tiers & Mobile App — Design Spec

**Date:** 2026-04-10
**Status:** Approved

## 1. Overview

Three milestones: storefront branding customization (logo, full color palette, fonts, layout, footer, announcement bar), subscription tier feature gating (Free/Starter/Pro/Enterprise across all features), and an Expo native app wrapper for Enterprise merchants.

### Build order

1. **B1 — Storefront Branding** (admin settings UI + storefront theme application)
2. **B2 — Subscription Tiers** (feature gating logic across all existing + new features)
3. **B3 — Expo Native App** (wrapper app, merchant branding, build pipeline)

### Constraints

- Same repos (`mark8ly` for branding + gating, new `apps/mobile` for Expo app)
- Branding stored per-store in the database, applied via CSS custom properties
- Subscription gating checks `store_subscriptions.plan` — requires S3 (Subscription) to be shipped first
- Expo app is a thin wrapper around the storefront URL — not a separate codebase for UI
- Color validation: reject low-contrast combinations that break readability

## 2. Subscription Tiers

### 2.1 Plan definitions

| Feature | Free | Starter | Pro | Enterprise |
|---------|------|---------|-----|------------|
| **Branding** |
| Logo + favicon | Yes | Yes | Yes | Yes |
| Accent color only | Yes | — | — | — |
| Full color palette | — | Yes | Yes | Yes |
| Custom fonts | — | Yes | Yes | Yes |
| Announcement bar | — | Yes | Yes | Yes |
| Social links + footer | Yes | Yes | Yes | Yes |
| Custom CSS injection | — | — | — | Yes |
| Remove "Powered by mark8ly" | — | — | Yes | Yes |
| **Infrastructure** |
| Custom domain | — | — | Yes | Yes |
| Expo native app | — | — | — | Yes |
| **Operations** |
| Products limit | 25 | 500 | Unlimited | Unlimited |
| Staff members | 1 | 3 | 10 | Unlimited |
| CSV import/export | — | Yes | Yes | Yes |
| **Marketing** |
| Coupons | 5 active | 50 active | Unlimited | Unlimited |
| Gift cards | — | Yes | Yes | Yes |
| Loyalty program | — | — | Yes | Yes |
| Campaigns | — | 5/month | Unlimited | Unlimited |
| **Support** |
| Help center | Yes | Yes | Yes | Yes |
| Tickets | — | Yes | Yes | Yes |
| Priority support | — | — | — | Yes |
| **Analytics** |
| Basic dashboard | Yes | Yes | Yes | Yes |
| Audit logs | — | — | Yes | Yes |

### 2.2 Pricing (configurable in Stripe Billing)

| Plan | Monthly | Annual (per month) |
|------|---------|-------------------|
| Free | $0 | $0 |
| Starter | $29 | $24 |
| Pro | $79 | $66 |
| Enterprise | $199 | $166 |

Stored in Stripe as Price IDs. Mark8ly admin never hardcodes prices — they come from Stripe.

### 2.3 Feature gating architecture

New package: `internal/plangate/gate.go`

```go
type Feature string

const (
    FeatureFullPalette      Feature = "full_palette"
    FeatureCustomFonts      Feature = "custom_fonts"
    FeatureAnnouncementBar  Feature = "announcement_bar"
    FeatureCustomCSS        Feature = "custom_css"
    FeatureRemoveBranding   Feature = "remove_branding"
    FeatureCustomDomain     Feature = "custom_domain"
    FeatureExpoApp          Feature = "expo_app"
    FeatureCSVImport        Feature = "csv_import"
    FeatureGiftCards        Feature = "gift_cards"
    FeatureLoyalty          Feature = "loyalty"
    FeatureTickets          Feature = "tickets"
    FeatureAuditLogs        Feature = "audit_logs"
)

// IsAllowed checks if a feature is available on the given plan.
func IsAllowed(plan string, feature Feature) bool { ... }

// Gin middleware: RequirePlan(minPlan string) gin.HandlerFunc
// Reads store_subscriptions.plan from context, returns 403 with upgrade message if insufficient.
```

The gate is a simple lookup table — no complex rules engine. Each handler that needs gating calls `plangate.IsAllowed()` or uses the middleware.

Frontend: `useSubscription()` hook returns current plan. Components check `canUse(feature)` to show/hide or show upgrade prompts.

## 3. B1 — Storefront Branding

### 3.1 Data model

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
    color_background    VARCHAR(7)    NOT NULL DEFAULT '#F7F6F2',  -- paper-200
    color_text          VARCHAR(7)    NOT NULL DEFAULT '#0E0E0C',  -- ink-900
    color_accent        VARCHAR(7)    NOT NULL DEFAULT '#2D4A2B',  -- moss-700
    color_button_bg     VARCHAR(7)    NOT NULL DEFAULT '#0E0E0C',  -- ink-900
    color_button_text   VARCHAR(7)    NOT NULL DEFAULT '#F7F6F2',  -- paper-200
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

### 3.2 Font options

Curated list — all Google Fonts, all with good language support:

| Key | Name | Style |
|-----|------|-------|
| `source-serif-4` | Source Serif 4 | Editorial serif (default) |
| `playfair-display` | Playfair Display | Elegant serif |
| `lora` | Lora | Readable serif |
| `inter` | Inter | Clean sans |
| `source-sans-3` | Source Sans 3 | Neutral sans (default body) |
| `dm-sans` | DM Sans | Modern sans |

### 3.3 Color contrast validation

Before saving colors, validate WCAG AA contrast ratios:
- Text on background: >= 4.5:1
- Button text on button background: >= 4.5:1
- Accent on background: >= 3:1 (large text / UI components)

Reject with helpful error: "Your text color doesn't have enough contrast against the background. Try a darker shade."

### 3.4 API endpoints

**Admin:**
- `GET /admin/stores/:storeId/branding` — get current branding
- `PUT /admin/stores/:storeId/branding` — update (with contrast validation + plan gating)
- `POST /admin/stores/:storeId/branding/logo` — upload logo (GCS)
- `POST /admin/stores/:storeId/branding/favicon` — upload favicon (GCS)
- `POST /admin/stores/:storeId/branding/hero` — upload hero image (GCS)

**Storefront:**
- `GET /api/v1/storefront/stores/:slug/branding` — public branding data (cached aggressively)

### 3.5 Storefront theme application

The storefront reads branding at request time and injects CSS custom properties:

```tsx
// In storefront layout.tsx
const branding = await fetchBranding(slug);

<style>{`
  :root {
    --paper-200: ${branding.color_background};
    --ink-900: ${branding.color_text};
    --moss-700: ${branding.color_accent};
    --button-bg: ${branding.color_button_bg};
    --button-text: ${branding.color_button_text};
    --font-heading: '${fontFamilyMap[branding.heading_font]}', serif;
    --font-body: '${fontFamilyMap[branding.body_font]}', sans-serif;
  }
`}</style>
```

All existing storefront components already use `var(--paper-200)`, `var(--ink-900)`, `var(--moss-700)` — they'll automatically pick up the merchant's colors.

### 3.6 Admin UI

`/settings/storefront` — enhanced with sections:

```
Identity section
├── Logo upload (drag-drop + preview)
├── Favicon upload
├── Tagline input

Colors section (plan-gated: Free=accent only, Starter+=full)
├── Color pickers: background, text, accent, button bg, button text
├── Live preview panel (mini storefront mockup updates in real-time)
├── Contrast warnings (inline, per pair)
├── "Reset to defaults" button

Typography section (plan-gated: Starter+)
├── Heading font dropdown with preview
├── Body font dropdown with preview

Homepage section
├── Layout variant selector (visual grid of 8 options)
├── Hero image upload
├── Announcement bar toggle + text + link + color

Footer section
├── Tagline input
├── Copyright input
├── Social links (5 URL inputs)

Advanced section (plan-gated: Enterprise)
├── Custom CSS textarea with syntax highlighting
├── "Powered by mark8ly" toggle
```

### 3.7 Live preview

A mini storefront preview panel on the right side of the settings page that updates in real-time as the merchant changes colors/fonts. Implemented as an iframe pointing to the storefront with `?preview=true&branding=<base64-encoded-overrides>` that applies overrides without saving.

## 4. B3 — Expo Native App

### 4.1 Architecture

```
apps/mobile/                    # New Expo app
├── app.json                    # Expo config (dynamic per merchant)
├── app/
│   └── index.tsx               # WebView wrapping storefront URL
├── lib/
│   ├── push-notifications.ts   # Expo push notifications
│   └── branding.ts             # Load merchant branding for splash/icon
├── assets/
│   └── splash-template.png     # Template splash screen
└── scripts/
    └── build-for-merchant.ts   # Per-merchant build script
```

### 4.2 How it works

The Expo app is a **configured WebView** wrapper:
1. On launch: show branded splash screen (merchant logo + colors)
2. Load `https://{merchant-domain}.mark8ly.com` in a WebView
3. Native push notifications via Expo Push + notification-service
4. Native share sheet, camera access for review photos
5. App icon generated from merchant logo

### 4.3 Build pipeline

Per-merchant build process (triggered from admin):
1. Admin clicks "Build Mobile App" in Enterprise settings
2. Backend creates a build job with merchant's branding config
3. CI pipeline (EAS Build or GitHub Actions):
   - Clone `apps/mobile` template
   - Inject merchant's `app.json` (name, icon, splash, scheme, bundleId)
   - Build iOS + Android via EAS
   - Upload to App Store Connect / Google Play Console (via Fastlane)
4. Admin shows build status + download links

### 4.4 Data model

**Migration 000021 — Mobile app builds**

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

### 4.5 Admin UI

```
/settings/mobile-app            (Enterprise only)
├── App configuration form
│   ├── App name
│   ├── Bundle ID (auto-generated from store slug)
│   ├── Icon upload (1024x1024)
│   ├── Splash screen upload
│   ├── Primary color (from branding)
│   └── iOS Team ID + Android package name
├── Build section
│   ├── "Build iOS" / "Build Android" buttons
│   ├── Build history table (version, platform, status, date)
│   └── Download / App Store links when complete
└── Status: draft → building → published
```

## 5. Security

- **Branding:** Color values validated (hex format, contrast ratios). Logo/favicon/hero uploads through GCS with content-type validation. Custom CSS sanitized (strip `@import`, `url()` with external domains, `javascript:` expressions).
- **Subscription gating:** Plan checked server-side via `plangate.IsAllowed()` — never trust client-side feature flags alone.
- **Mobile app:** Merchant's App Store credentials (team ID, etc.) stored encrypted. Build pipeline runs in isolated CI environment. WebView restricted to merchant's own domain.

## 6. Testing

- **B1:** Branding CRUD, contrast validation rejects bad combos, storefront renders with custom colors/fonts, live preview iframe, logo upload, announcement bar toggle, plan gating (free can't set full palette)
- **B2:** Plan gate allows/denies correctly for each feature × plan combination, middleware returns 403 with upgrade message, frontend hides gated features, downgrade behavior (features disabled but data preserved)
- **B3:** Expo app loads merchant storefront in WebView, push notifications register, build job creation, build status polling

## 7. Out of Scope

- Custom email templates (use notification-service defaults)
- White-label admin panel (admin is always mark8ly branded)
- Multiple Expo apps per store
- Expo app push notification content customization (uses system defaults)
- App Store review process automation (manual submission initially)
- A/B testing different storefront themes
