# Marketing Features — Unified Design Spec

**Date:** 2026-04-10
**Status:** Approved

## 1. Overview

Four marketing features for the mark8ly platform: Coupons, Gift Cards, Loyalty Program, and Campaigns. All live inside the existing `marketplace-api` Go binary as new packages. No new microservices. The old app (`mark8ly_backup`) is reference-only — new code follows the consolidated patterns established in products/orders/payments work.

### Build order

1. **M1 — Coupons** (most universal, simple checkout integration)
2. **M2 — Gift Cards** (payment method integration)
3. **M3 — Loyalty** (customer accounts + points economy)
4. **M4 — Campaigns** (email integration + segments)

### Constraints

- Same binary (`marketplace-api`), same repo (`mark8ly`)
- One migration per feature (000009–000012)
- Multi-tenant: all tables have `tenant_id` + `store_id`
- Follow existing patterns: GORM models, repository interfaces, Gin handlers, admin authz middleware
- Old code is reference for feature scope only — rewrite clean with tests

## 2. Architecture

```
services/marketplace-api/internal/
├── coupon/              # models, repository, service
├── giftcard/            # models, repository, service
├── loyalty/             # models, repository, service, points engine
├── campaign/            # models, repository, service, segment engine
└── handlers/
    ├── admin/           # CRUD handlers (extend existing Deps + routes)
    └── storefront/      # apply coupon, redeem gift card, earn points
```

Admin UI pages under `apps/admin/app/marketing/`. Storefront integration in checkout + dedicated pages.

## 3. Data Model

### 3.1 Migration 000009 — Coupons

```sql
CREATE TABLE coupons (
    id              UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID          NOT NULL,
    store_id        UUID          NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    code            VARCHAR(50)   NOT NULL,
    title           VARCHAR(200)  NOT NULL,
    description     TEXT,
    type            VARCHAR(20)   NOT NULL,  -- 'percentage', 'fixed_amount', 'free_shipping'
    value           NUMERIC(12,2) NOT NULL,  -- percent value or fixed amount
    currency_code   CHAR(3),                 -- required for fixed_amount
    min_purchase    NUMERIC(12,2),           -- NULL = no minimum
    max_discount    NUMERIC(12,2),           -- cap for percentage coupons
    usage_limit     INT,                     -- NULL = unlimited total uses
    per_customer    INT           NOT NULL DEFAULT 1,
    target_type     VARCHAR(20)   NOT NULL DEFAULT 'all', -- 'all', 'products', 'categories'
    target_ids      UUID[],                  -- product or category IDs when targeted
    stackable       BOOLEAN       NOT NULL DEFAULT false,
    starts_at       TIMESTAMPTZ   NOT NULL DEFAULT now(),
    ends_at         TIMESTAMPTZ,             -- NULL = no expiry
    status          VARCHAR(20)   NOT NULL DEFAULT 'active', -- 'active', 'disabled', 'expired'
    usage_count     INT           NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ   NOT NULL DEFAULT now(),
    UNIQUE (store_id, code)
);
CREATE INDEX coupons_store_status_idx ON coupons (store_id, status);

CREATE TABLE coupon_usage (
    id              UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    coupon_id       UUID          NOT NULL REFERENCES coupons(id) ON DELETE CASCADE,
    order_id        UUID          NOT NULL REFERENCES orders(id),
    customer_email  VARCHAR(300)  NOT NULL,
    discount_amount NUMERIC(12,2) NOT NULL,
    currency_code   CHAR(3)       NOT NULL,
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT now()
);
CREATE INDEX coupon_usage_coupon_idx ON coupon_usage (coupon_id);
CREATE INDEX coupon_usage_email_idx ON coupon_usage (coupon_id, customer_email);
```

### 3.2 Migration 000010 — Gift Cards

```sql
CREATE TABLE gift_cards (
    id              UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID          NOT NULL,
    store_id        UUID          NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    code            VARCHAR(50)   NOT NULL,
    initial_balance NUMERIC(12,2) NOT NULL,
    current_balance NUMERIC(12,2) NOT NULL,
    currency_code   CHAR(3)       NOT NULL,
    status          VARCHAR(20)   NOT NULL DEFAULT 'active', -- 'active', 'disabled', 'depleted'
    sender_name     VARCHAR(200),
    sender_email    VARCHAR(300),
    recipient_name  VARCHAR(200),
    recipient_email VARCHAR(300),
    message         TEXT,
    purchased_at    TIMESTAMPTZ,
    expires_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ   NOT NULL DEFAULT now(),
    UNIQUE (store_id, code)
);
CREATE INDEX gift_cards_store_status_idx ON gift_cards (store_id, status);

CREATE TABLE gift_card_transactions (
    id              UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    gift_card_id    UUID          NOT NULL REFERENCES gift_cards(id) ON DELETE CASCADE,
    order_id        UUID,
    type            VARCHAR(20)   NOT NULL, -- 'purchase', 'redeem', 'refund', 'adjustment'
    amount          NUMERIC(12,2) NOT NULL, -- positive = credit, negative = debit
    balance_after   NUMERIC(12,2) NOT NULL,
    note            VARCHAR(200),
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT now()
);
CREATE INDEX gc_txn_card_idx ON gift_card_transactions (gift_card_id);
```

### 3.3 Migration 000011 — Loyalty

```sql
CREATE TABLE loyalty_programs (
    id                  UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           UUID          NOT NULL,
    store_id            UUID          NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    is_active           BOOLEAN       NOT NULL DEFAULT false,
    points_per_dollar   NUMERIC(5,2)  NOT NULL DEFAULT 1.00,
    points_currency     VARCHAR(20)   NOT NULL DEFAULT 'points', -- display name
    signup_bonus        INT           NOT NULL DEFAULT 0,
    referral_bonus      INT           NOT NULL DEFAULT 0,
    referee_bonus       INT           NOT NULL DEFAULT 0,
    point_expiry_days   INT,           -- NULL = never expire
    min_redeem_points   INT           NOT NULL DEFAULT 100,
    points_value        NUMERIC(8,4)  NOT NULL DEFAULT 0.01, -- 1 point = $0.01
    tiers               JSONB         NOT NULL DEFAULT '[]'::jsonb,
    created_at          TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ   NOT NULL DEFAULT now(),
    UNIQUE (store_id)
);

CREATE TABLE customer_loyalties (
    id              UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID          NOT NULL,
    store_id        UUID          NOT NULL,
    customer_email  VARCHAR(300)  NOT NULL,
    customer_name   VARCHAR(200),
    points_balance  INT           NOT NULL DEFAULT 0,
    lifetime_points INT           NOT NULL DEFAULT 0,
    tier            VARCHAR(50)   NOT NULL DEFAULT 'bronze',
    referral_code   VARCHAR(20)   NOT NULL,
    referred_by     UUID,
    enrolled_at     TIMESTAMPTZ   NOT NULL DEFAULT now(),
    UNIQUE (store_id, customer_email)
);
CREATE INDEX cl_store_tier_idx ON customer_loyalties (store_id, tier);

CREATE TABLE loyalty_transactions (
    id              UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    loyalty_id      UUID          NOT NULL REFERENCES customer_loyalties(id) ON DELETE CASCADE,
    order_id        UUID,
    type            VARCHAR(20)   NOT NULL, -- 'earn', 'redeem', 'expire', 'adjust', 'signup', 'referral'
    points          INT           NOT NULL, -- positive = earn, negative = spend
    balance_after   INT           NOT NULL,
    description     VARCHAR(200),
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT now()
);
CREATE INDEX lt_loyalty_idx ON loyalty_transactions (loyalty_id);

CREATE TABLE referrals (
    id              UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID          NOT NULL,
    store_id        UUID          NOT NULL,
    referrer_id     UUID          NOT NULL REFERENCES customer_loyalties(id),
    referee_id      UUID          NOT NULL REFERENCES customer_loyalties(id),
    status          VARCHAR(20)   NOT NULL DEFAULT 'pending', -- 'pending', 'completed', 'expired'
    referrer_bonus  INT           NOT NULL DEFAULT 0,
    referee_bonus   INT           NOT NULL DEFAULT 0,
    completed_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT now(),
    UNIQUE (store_id, referee_id)
);
```

### 3.4 Migration 000012 — Campaigns

```sql
CREATE TABLE customer_segments (
    id          UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID         NOT NULL,
    store_id    UUID         NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    name        VARCHAR(200) NOT NULL,
    description TEXT,
    rules       JSONB        NOT NULL DEFAULT '[]'::jsonb,
    member_count INT         NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE TABLE campaigns (
    id              UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID         NOT NULL,
    store_id        UUID         NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    name            VARCHAR(200) NOT NULL,
    type            VARCHAR(20)  NOT NULL DEFAULT 'email', -- 'email', 'sms', 'push'
    status          VARCHAR(20)  NOT NULL DEFAULT 'draft', -- 'draft', 'scheduled', 'sending', 'sent', 'paused', 'cancelled'
    subject         VARCHAR(300),
    content         TEXT,
    segment_id      UUID         REFERENCES customer_segments(id),
    coupon_id       UUID         REFERENCES coupons(id),      -- optional attached coupon
    scheduled_at    TIMESTAMPTZ,
    sent_at         TIMESTAMPTZ,
    -- analytics
    total_recipients INT         NOT NULL DEFAULT 0,
    delivered       INT          NOT NULL DEFAULT 0,
    opened          INT          NOT NULL DEFAULT 0,
    clicked         INT          NOT NULL DEFAULT 0,
    converted       INT          NOT NULL DEFAULT 0,
    unsubscribed    INT          NOT NULL DEFAULT 0,
    failed          INT          NOT NULL DEFAULT 0,
    revenue         NUMERIC(12,2) NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT now()
);
CREATE INDEX campaigns_store_status_idx ON campaigns (store_id, status);

CREATE TABLE campaign_recipients (
    id              UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    campaign_id     UUID         NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
    customer_email  VARCHAR(300) NOT NULL,
    status          VARCHAR(20)  NOT NULL DEFAULT 'pending', -- 'pending', 'sent', 'delivered', 'opened', 'clicked', 'bounced', 'unsubscribed'
    sent_at         TIMESTAMPTZ,
    opened_at       TIMESTAMPTZ,
    clicked_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now()
);
CREATE INDEX cr_campaign_idx ON campaign_recipients (campaign_id);
CREATE INDEX cr_email_idx ON campaign_recipients (customer_email);
```

## 4. API Endpoints

### 4.1 Coupons

**Admin:**
- `GET /admin/stores/:storeId/coupons` — list with filters (status, search)
- `POST /admin/stores/:storeId/coupons` — create
- `GET /admin/stores/:storeId/coupons/:id` — detail + usage stats
- `PATCH /admin/stores/:storeId/coupons/:id` — update
- `DELETE /admin/stores/:storeId/coupons/:id` — soft-disable

**Storefront:**
- `POST /storefront/stores/:slug/coupons/validate` — validate code, return discount preview (does NOT apply)
- Integration: `checkout_ext.go` calls coupon service to apply discount during order creation

### 4.2 Gift Cards

**Admin:**
- `GET /admin/stores/:storeId/gift-cards` — list with filters (status, balance)
- `POST /admin/stores/:storeId/gift-cards` — issue new card
- `GET /admin/stores/:storeId/gift-cards/:id` — detail + transaction history

**Storefront:**
- `POST /storefront/stores/:slug/gift-cards/check-balance` — lookup by code
- Integration: `checkout_ext.go` applies gift card as payment method, deducting from balance

### 4.3 Loyalty

**Admin:**
- `GET /admin/stores/:storeId/loyalty/program` — get program config
- `PUT /admin/stores/:storeId/loyalty/program` — update program config
- `GET /admin/stores/:storeId/loyalty/members` — list members with points/tier
- `GET /admin/stores/:storeId/loyalty/members/:id` — member detail + transactions
- `POST /admin/stores/:storeId/loyalty/members/:id/adjust` — manual point adjustment

**Storefront:**
- `GET /storefront/stores/:slug/loyalty/program` — program details (public)
- `POST /storefront/stores/:slug/loyalty/enroll` — enroll by email
- `GET /storefront/stores/:slug/loyalty/me?email=` — customer's points/tier
- `POST /storefront/stores/:slug/loyalty/redeem` — redeem points at checkout
- Integration: post-checkout webhook awards points based on order total

### 4.4 Campaigns

**Admin:**
- `GET /admin/stores/:storeId/campaigns` — list with status filters
- `POST /admin/stores/:storeId/campaigns` — create
- `GET /admin/stores/:storeId/campaigns/:id` — detail + delivery analytics
- `PATCH /admin/stores/:storeId/campaigns/:id` — update draft
- `DELETE /admin/stores/:storeId/campaigns/:id` — delete draft
- `POST /admin/stores/:storeId/campaigns/:id/send` — trigger send
- `POST /admin/stores/:storeId/campaigns/:id/schedule` — schedule
- `POST /admin/stores/:storeId/campaigns/:id/pause` — pause active
- `GET /admin/stores/:storeId/segments` — list segments
- `POST /admin/stores/:storeId/segments` — create segment

## 5. Checkout Integration

Extended checkout flow with coupon + gift card + loyalty:

```
1. Validate items + create order (existing)
2. If coupon_code provided:
   → coupon.Service.Validate(code, storeID, email, subtotal, items)
   → coupon.Service.Apply(couponID, orderID) → adjust order subtotal
3. Calculate tax on adjusted subtotal (existing)
4. Calculate shipping (existing)
5. If gift_card_code provided:
   → giftcard.Service.CheckBalance(code, storeID)
   → giftcard.Service.Debit(cardID, min(balance, amountOwed)) → reduce payment amount
6. Create payment intent for remaining balance (existing, may be $0 if gift card covers all)
7. Post-checkout: loyalty.Service.AwardPoints(email, orderTotal, storeID)
```

Request body additions to `CheckoutExtRequest`:
```json
{
  "coupon_code": "SAVE20",
  "gift_card_code": "GC-XXXX-YYYY",
  "redeem_points": 500
}
```

Response additions to `CheckoutExtResponse`:
```json
{
  "discount_total": "10.00",
  "gift_card_applied": "15.00",
  "points_redeemed": 500,
  "points_value": "5.00",
  "points_earned": 42
}
```

## 6. Admin UI

### 6.1 Sidebar

Marketing section (already exists in AdminShell, placeholder routes):
```
Marketing  ▸ Coupons, Gift Cards, Loyalty, Campaigns
```

### 6.2 Pages

```
/marketing/coupons              — list: filterable table (status, search by code)
/marketing/coupons/new          — create form: type picker, value, limits, targeting, dates
/marketing/coupons/[id]         — detail: config summary + usage stats + usage table

/marketing/gift-cards           — list: filterable table (status, balance range)
/marketing/gift-cards/new       — issue form: amount, recipient, message, expiry
/marketing/gift-cards/[id]      — detail: card info + transaction ledger

/marketing/loyalty              — tabbed: Program config | Members | Referrals
  Program tab                   — points/dollar, tiers JSONB editor, bonuses, expiry
  Members tab                   — table: email, points, tier, enrolled date, actions
  Referrals tab                 — table: referrer, referee, status, bonuses

/marketing/campaigns            — list: status-filtered table with analytics preview
/marketing/campaigns/new        — stepped wizard: audience → content → schedule
/marketing/campaigns/[id]       — detail: content preview + delivery analytics chart
```

### 6.3 Design

All pages follow the established Paper/Ink/Moss editorial pattern:
- `AdminShell` wrapper
- Source Serif 4 headings
- Hairline rules between sections
- Tables use existing patterns from products list
- Forms use existing input patterns from settings pages

## 7. Storefront UI

### 7.1 Checkout additions

- Coupon code input field below order summary (validate on blur/enter)
- Gift card code input field with balance display
- Loyalty points redemption toggle (show available points + dollar value)
- Updated totals section: subtotal, discount, gift card, points, shipping, tax, total

### 7.2 New pages

- `/gift-cards` — purchase page: select amount, recipient details, preview card
- `/account/loyalty` — points balance, tier badge, referral code + share link, transaction history

## 8. Testing

Each milestone includes:
- Go unit tests for service layer (validation, calculation)
- Go integration tests for handlers (HTTP round-trip with real Postgres via testdb)
- Coupon: validate + apply + usage limit enforcement + expiry + stacking rules
- Gift card: balance check + debit + insufficient funds + concurrent debit safety
- Loyalty: points earn + redeem + tier promotion + point expiry + referral
- Campaign: create + schedule + send + pause + analytics increment

## 9. Out of Scope

- Email template builder/WYSIWYG for campaigns (use plain text/HTML for M4)
- Automatic coupon generation (batch create 1000 codes) — follow-up
- Gift card visual templates/PDF generation — follow-up
- Advanced segment builder UI with drag-and-drop rules — follow-up
- A/B testing for campaigns — follow-up
- Points marketplace (redeem for specific products) — follow-up
