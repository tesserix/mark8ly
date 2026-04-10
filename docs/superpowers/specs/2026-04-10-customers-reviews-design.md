# Customers & Reviews — Unified Design Spec

**Date:** 2026-04-10
**Status:** Approved

## 1. Overview

Three milestones adding storefront customer authentication, admin customer profiles (aggregation view), and product reviews with photos, helpful reactions, featured flag, and merchant replies. All inside the existing `marketplace-api` binary. Customer auth leverages the existing `auth-bff` + GIP `mp-customer` tenant pool.

### Build order

1. **C1 — Storefront Auth** (customer login/register, session middleware, /account shell)
2. **C2 — Customer Profiles** (admin customer list + detail, lightweight profile table)
3. **C3 — Reviews** (product reviews with moderation, photos, helpful, merchant replies)

### Constraints

- Same binary (`marketplace-api`), same repo (`mark8ly`)
- Auth via existing `auth-bff` + GIP `mp-customer` pool — no new auth service
- Customer profiles are an aggregation view, NOT a denormalized copy of order data
- Reviews are product-only (no vendor/service/experience types)
- Old app (`mark8ly_backup`) is reference only — rewrite clean

## 2. Architecture

```
services/marketplace-api/internal/
├── customer/            # models, repository, service (profile + aggregation)
├── review/              # models, repository, service (reviews + media + reactions)
└── handlers/
    ├── admin/           # customer + review admin handlers
    └── storefront/      # customer account + review submit/display handlers

apps/storefront/
├── middleware.ts         # MODIFIED — add optional customer auth from auth-bff cookie
├── app/account/          # NEW — customer account pages (requires auth)
└── app/products/[handle] # MODIFIED — review section on product detail
```

## 3. Data Model

### 3.1 Migration 000013 — Customer Profiles

```sql
CREATE TABLE customer_profiles (
    id              UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID          NOT NULL,
    store_id        UUID          NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    gip_uid         VARCHAR(200),         -- GIP user ID from mp-customer pool, nullable for guest-converted
    email           VARCHAR(300)  NOT NULL,
    first_name      VARCHAR(200),
    last_name       VARCHAR(200),
    phone           VARCHAR(40),
    avatar_url      TEXT,
    tags            TEXT[]        NOT NULL DEFAULT '{}',
    status          VARCHAR(20)   NOT NULL DEFAULT 'active', -- 'active', 'blocked'
    block_reason    VARCHAR(300),
    notes           TEXT,                  -- admin-only internal notes
    marketing_opt_in BOOLEAN      NOT NULL DEFAULT false,
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ   NOT NULL DEFAULT now(),
    UNIQUE (store_id, email)
);
CREATE INDEX cp_store_status_idx ON customer_profiles (store_id, status);
CREATE INDEX cp_gip_uid_idx ON customer_profiles (gip_uid) WHERE gip_uid IS NOT NULL;
CREATE INDEX cp_tags_idx ON customer_profiles USING GIN (tags);

-- Customer saved addresses (for storefront account)
CREATE TABLE customer_addresses (
    id              UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID          NOT NULL,
    customer_id     UUID          NOT NULL REFERENCES customer_profiles(id) ON DELETE CASCADE,
    label           VARCHAR(100),          -- 'Home', 'Work', etc.
    is_default      BOOLEAN       NOT NULL DEFAULT false,
    name            VARCHAR(200)  NOT NULL,
    line1           VARCHAR(300)  NOT NULL,
    line2           VARCHAR(300),
    city            VARCHAR(200)  NOT NULL,
    region          VARCHAR(200),
    postal_code     VARCHAR(40),
    country_code    CHAR(2)       NOT NULL,
    phone           VARCHAR(40),
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ   NOT NULL DEFAULT now()
);
CREATE INDEX ca_customer_idx ON customer_addresses (customer_id);
```

No denormalized counters (total_orders, total_spent, LTV). These are computed via joins to the orders table at query time. Keeps the profile lightweight and avoids sync issues.

### 3.2 Migration 000014 — Reviews

```sql
CREATE TABLE reviews (
    id                  UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           UUID          NOT NULL,
    store_id            UUID          NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    product_id          UUID          NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    customer_profile_id UUID          REFERENCES customer_profiles(id) ON DELETE SET NULL,
    customer_name       VARCHAR(200)  NOT NULL,
    customer_email      VARCHAR(300)  NOT NULL,
    rating              INT           NOT NULL CHECK (rating >= 1 AND rating <= 5),
    title               VARCHAR(300),
    content             TEXT          NOT NULL,
    status              VARCHAR(20)   NOT NULL DEFAULT 'pending', -- 'pending', 'approved', 'rejected'
    verified_purchase   BOOLEAN       NOT NULL DEFAULT false,
    featured            BOOLEAN       NOT NULL DEFAULT false,
    helpful_count       INT           NOT NULL DEFAULT 0,
    not_helpful_count   INT           NOT NULL DEFAULT 0,
    published_at        TIMESTAMPTZ,
    created_at          TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ   NOT NULL DEFAULT now()
);
CREATE INDEX reviews_product_status_idx ON reviews (product_id, status);
CREATE INDEX reviews_store_status_idx ON reviews (store_id, status);
CREATE INDEX reviews_customer_idx ON reviews (customer_email, store_id);

CREATE TABLE review_media (
    id              UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    review_id       UUID          NOT NULL REFERENCES reviews(id) ON DELETE CASCADE,
    url             TEXT          NOT NULL,
    alt             VARCHAR(300),
    position        INT           NOT NULL DEFAULT 0,
    media_type      VARCHAR(20)   NOT NULL DEFAULT 'image',
    width           INT,
    height          INT,
    file_size       INT,
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT now()
);
CREATE INDEX rm_review_idx ON review_media (review_id);

CREATE TABLE review_replies (
    id              UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    review_id       UUID          NOT NULL REFERENCES reviews(id) ON DELETE CASCADE,
    author_type     VARCHAR(20)   NOT NULL, -- 'merchant', 'customer'
    author_name     VARCHAR(200)  NOT NULL,
    author_email    VARCHAR(300),
    content         TEXT          NOT NULL,
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT now()
);
CREATE INDEX rr_review_idx ON review_replies (review_id);

CREATE TABLE review_reactions (
    id              UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    review_id       UUID          NOT NULL REFERENCES reviews(id) ON DELETE CASCADE,
    customer_email  VARCHAR(300)  NOT NULL,
    reaction        VARCHAR(20)   NOT NULL, -- 'helpful', 'not_helpful'
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT now(),
    UNIQUE (review_id, customer_email)
);
```

## 4. Storefront Auth Flow

### 4.1 Login/Register

```
Customer clicks "Sign in" on storefront nav
  → Link to auth-bff: /login?product=mp-customer&redirect_uri=<storefront-url>/account
  → auth-bff handles GIP OIDC (email/password, Google OAuth, passkey via WebAuthn)
  → on success: auth-bff sets encrypted session cookie (existing cookie.go)
  → redirect back to storefront
  → storefront middleware reads cookie → extracts gip_uid + email
  → upserts customer_profiles row (auto-create profile on first login)
```

### 4.2 Storefront Middleware

Existing middleware chain: `RequireStorefrontKey → StoreContext`

Add `OptionalCustomerAuth` after StoreContext:
- Reads auth-bff session cookie
- If valid: sets `customer_profile_id`, `customer_email`, `customer_gip_uid` on gin context
- If missing/invalid: continues without customer context (guest browsing)
- `/account/*` routes use `RequireCustomerAuth` which returns 401 if no customer context

### 4.3 Profile auto-creation

On first authenticated request, if no `customer_profiles` row exists for (store_id, email):
- Insert with `gip_uid`, `email`, name from GIP claims
- Return the new profile
- Subsequent requests read from the existing row

## 5. API Endpoints

### 5.1 Customer Profiles (admin)

- `GET /admin/stores/:storeId/customers` — paginated list with search (email, name), status filter, sort. Returns profile + computed stats via subquery:
  ```sql
  SELECT cp.*, 
    (SELECT COUNT(*) FROM orders WHERE customer_email = cp.email AND store_id = cp.store_id) as order_count,
    (SELECT COALESCE(SUM(grand_total), 0) FROM orders WHERE customer_email = cp.email AND store_id = cp.store_id) as total_spent,
    (SELECT MAX(created_at) FROM orders WHERE customer_email = cp.email AND store_id = cp.store_id) as last_order_at
  FROM customer_profiles cp WHERE cp.store_id = ? AND cp.tenant_id = ?
  ```
- `GET /admin/stores/:storeId/customers/:id` — detail: profile + orders + addresses + loyalty (if enrolled)
- `PATCH /admin/stores/:storeId/customers/:id` — update tags, notes, marketing_opt_in
- `POST /admin/stores/:storeId/customers/:id/block` — set status=blocked + block_reason
- `POST /admin/stores/:storeId/customers/:id/unblock` — set status=active

### 5.2 Customer Account (storefront, auth required)

- `GET /storefront/stores/:slug/account` — profile (name, email, phone)
- `PATCH /storefront/stores/:slug/account` — update name, phone, avatar_url
- `GET /storefront/stores/:slug/account/orders` — paginated order history
- `GET /storefront/stores/:slug/account/orders/:id` — order detail
- `GET /storefront/stores/:slug/account/addresses` — saved addresses
- `POST /storefront/stores/:slug/account/addresses` — create address
- `PATCH /storefront/stores/:slug/account/addresses/:id` — update
- `DELETE /storefront/stores/:slug/account/addresses/:id` — delete

### 5.3 Reviews (admin)

- `GET /admin/stores/:storeId/reviews` — moderation list (status filter, product filter, search)
- `GET /admin/stores/:storeId/reviews/:id` — detail with media + replies
- `PATCH /admin/stores/:storeId/reviews/:id` — approve/reject/feature (status, featured)
- `POST /admin/stores/:storeId/reviews/:id/reply` — merchant reply
- `DELETE /admin/stores/:storeId/reviews/:id` — delete review

### 5.4 Reviews (storefront)

- `GET /storefront/stores/:slug/products/:handle/reviews` — approved reviews with media + replies, paginated. Includes summary: average rating, count, rating distribution (1-5 breakdown)
- `POST /storefront/stores/:slug/products/:handle/reviews` — submit review (auth required). Auto-checks `verified_purchase` by querying orders for matching (customer_email, product_id)
- `POST /storefront/stores/:slug/reviews/:id/reaction` — mark helpful/not_helpful (auth required). Uses `INSERT ON CONFLICT` for idempotent toggle
- `POST /storefront/stores/:slug/reviews/:id/media` — upload review photo (auth required, max 3 per review). Uses existing GCS uploader from media package

## 6. Admin UI

### 6.1 Sidebar

Customers section (already exists with placeholder routes):
```
Customers  ▸ All Customers, Reviews
```

### 6.2 Pages

```
/customers              — list: searchable table with email, name, order count,
                          total spent, last order, status badge, tags
                          Computed stats via API (no denormalized columns)

/customers/[id]         — tabbed detail:
  Overview tab          — profile info, key stats (orders, spend, LTV, AOV),
                          tags editor, notes editor, block/unblock button
  Orders tab            — order history table (reuse existing order list components)
  Addresses tab         — saved addresses list
  Loyalty tab           — points balance, tier, referral code (if M3 shipped)

/reviews                — moderation list: status tabs (Pending | Approved | Rejected)
                          Each row: product name, customer, rating stars, excerpt,
                          date, quick approve/reject buttons
                          Expand row for full review + photos + reply form

/reviews/[id]           — full detail: review content, photos grid, customer info,
                          verified purchase badge, featured toggle,
                          merchant reply form, reaction counts
```

### 6.3 Design

Paper/Ink/Moss editorial pattern. Star ratings use ink-900 filled stars + ink-900/20 empty stars (no yellow — stays on-brand). Featured reviews get a subtle moss-700 left border accent.

## 7. Storefront UI

### 7.1 Auth UI (nav)

StorefrontNav gets a "Sign in" link (right side) that links to auth-bff login URL. When authenticated, shows customer name + dropdown (Account, Orders, Sign out).

### 7.2 Account pages

```
/account                — dashboard: welcome message, name, email, recent 3 orders,
                          quick links to orders/addresses/loyalty
/account/orders         — paginated order history table
/account/orders/[id]    — order detail (extends existing /orders/[id] with auth context)
/account/addresses      — saved addresses list with add/edit/delete/set-default
/account/loyalty        — (already planned in M3)
```

### 7.3 Product reviews section

On `/products/[handle]` detail page, below the product details:

```
Reviews section:
├── Summary bar: average rating (stars + number), total count, rating distribution bars
├── "Write a review" button (links to /products/[handle]/review, auth gate)
├── Review list (approved only, newest first, paginated)
│   ├── Each review: stars, title, content, customer name, date, verified badge,
│   │   photos grid (click to expand), helpful button + count
│   │   └── Merchant reply (if exists, visually distinct — indented, "Store response" label)
│   └── "Load more" button
└── Empty state: "No reviews yet. Be the first to share your experience."
```

Review submission page (`/products/[handle]/review`):
- Requires auth (redirect to login if not)
- Star picker (1-5, click to select)
- Title input (optional)
- Content textarea (required, min 20 chars)
- Photo upload (drag/drop or click, max 3 images, 5MB each)
- Submit button → redirects back to product page with success toast

## 8. Concurrency & Safety (from specialist reviews)

### 8.1 Helpful count — use customer_profile_id, not email
Reactions keyed on `(review_id, customer_profile_id)` not email — email is public and can be spoofed. Atomic: `UPDATE reviews SET helpful_count = helpful_count + 1 WHERE id = $id`. Reaction insert uses `INSERT ON CONFLICT DO NOTHING`.

### 8.2 Profile auto-creation
`INSERT INTO customer_profiles (...) ON CONFLICT (store_id, email) DO UPDATE SET gip_uid = EXCLUDED.gip_uid, updated_at = now()` — safe under concurrent first-login.

### 8.3 Verified purchase check
Read-only query against orders + order_items. Requires composite index `CREATE INDEX ON orders (store_id, customer_email)` added in migration 000013.

### 8.4 Review photo upload — SELECT FOR UPDATE
Use `SELECT ... FOR UPDATE` on the review row before counting media. Reject if count >= 3. This eliminates the TOCTOU race where concurrent uploads exceed the limit.

### 8.5 One review per product per customer
Add `UNIQUE (store_id, product_id, customer_email)` constraint on `reviews` table. Prevents review flooding.

### 8.6 Customer list performance
The admin customer list computes stats via correlated subqueries. Add `CREATE INDEX orders_store_email_idx ON orders (store_id, customer_email)` in migration 000013 to prevent seq scans. Consider skeleton loading states in the UI for perceived performance.

### 8.7 Helpful count denormalization
Keep `helpful_count` / `not_helpful_count` on the reviews table for read performance. Toggle logic: when reaction changes from helpful→not_helpful, decrement helpful_count and increment not_helpful_count atomically in one UPDATE. Document this explicitly in the service layer.

## 9. Security (from specialist reviews)

### 9.1 Auth cookie requirements
Auth-bff session cookie MUST have: `HttpOnly`, `Secure`, `SameSite=Lax`. Middleware validates the cookie signature with the server-side key — not just decodes. Verify `auth-bff/internal/session/cookie.go` sets all three flags before C1 implementation.

### 9.2 Auth middleware
- `OptionalCustomerAuth`: reads auth-bff cookie, validates signature, extracts claims. Does NOT fail on missing cookie — guest browsing continues.
- `RequireCustomerAuth`: wraps OptionalCustomerAuth + returns 401 if no customer context. Used on `/account/*` routes.
- Customer can only access their own resources. Handler verifies `customer_profile_id` from context matches the resource.
- **Blocked customer check:** Review submission and order placement MUST check `customer_profiles.status != 'blocked'` before proceeding.

### 9.3 Avatar URL validation
`avatar_url` restricted to GCS-originated URLs only (same bucket prefix as product media). Never accept arbitrary URLs — prevents SSRF and stored XSS via `javascript:` URIs.

### 9.4 Review content sanitization
- Content sanitized via existing `product/sanitizer.go` (bluemonday) before storage
- Plain text only, rendered with whitespace preserved
- Photo uploads: GCS with content-type validation (image/* only), max 5MB
- `review_replies.author_email` NEVER returned in storefront API responses

### 9.5 Self-review prevention
Review submission checks that the authenticated customer's email does not match the store owner's email. Prevents merchants reviewing their own products.

### 9.6 Rate limiting
- Review submission: max 5 per customer per store per day + one per product per customer (UNIQUE constraint)
- Profile update: standard per-IP rate limit
- Auth redirect: validate `redirect_uri` against allowlist in auth-bff app registry

### 9.7 Storefront profile response
Explicitly EXCLUDE from storefront account response: `notes`, `block_reason`, `status`, `tags`. These are admin-only fields.

### 9.8 Cross-tenant validation
Every admin handler validates `store_id` belongs to authenticated `tenant_id`. Customer stats subqueries scoped by both `store_id` AND `tenant_id`.

## 10. UX Refinements (from specialist reviews)

### 10.1 Star ratings — use moss-700
Filled stars: `text-[color:var(--moss-700)]`. Empty stars: `text-[color:var(--ink-900)] opacity-15`. No yellow — stays on-brand. Moss carries positive/success meaning.

### 10.2 Review section — inline, not tabbed
Keep review section below product details (not behind a tab). Better for SEO and conversion. Add a sticky "Reviews" anchor in the product nav for long pages.

### 10.3 Review submission auth redirect
Pass `redirect_uri=/products/[handle]/review` (not `/account`) so customers return to the review form after login.

### 10.4 Moderation reply form state
Reply form stays mounted after approve/reject action. Expand stays open until explicitly dismissed.

### 10.5 Loyalty nav visibility
Only show `/account/loyalty` in account nav when M3 (Loyalty) has shipped and the store has an active loyalty program. Hidden entirely otherwise (not disabled).

### 10.6 Empty states
Define empty states for:
- Admin customer list: "No customers yet. They'll appear here after their first order."
- Admin reviews per status tab: "No pending reviews" / "No approved reviews yet"
- Account orders: "You haven't placed any orders yet. Start shopping →"
- Account addresses: "No saved addresses. Add one for faster checkout."

### 10.7 Customer list skeleton loading
Show row-level skeleton states on initial load. Defer computed stats (order_count, total_spent) to a separate async call so name/email/status render immediately.

## 11. Testing

Each milestone includes:
- **C1:** Auth middleware unit tests (valid/invalid/expired cookie, missing flags), profile auto-creation idempotency, auth-required route 401, blocked customer rejection, redirect_uri validation
- **C2:** Customer list with computed stats integration test (verify index usage), tag/note update, block/unblock flow, search by email/name, skeleton loading, admin-only fields excluded from storefront
- **C3:** Review CRUD + approval workflow, one-per-product constraint, verified purchase detection, helpful count atomic toggle (helpful→not_helpful), concurrent reaction idempotency, photo upload limit (concurrent race test with FOR UPDATE), merchant reply, featured toggle, self-review prevention, blocked customer review rejection, storefront review list (only approved), review summary (avg rating, distribution)

## 12. Out of Scope

- Customer segments/dynamic grouping — use marketing campaigns (M4) for targeting
- Customer communication log — campaigns feature handles outbound
- Wishlist/saved lists — follow-up feature
- Customer payment methods — payment providers (Stripe/Razorpay) handle stored cards
- Review spam detection/sentiment analysis — follow-up with ML pipeline
- Video reviews — images only for now
- Multi-aspect ratings (quality, value, etc.) — single 1-5 star rating
- Customer import/export CSV — follow-up
- Orders customer_id backfill — follow-up (lazy backfill in profile auto-creation)
