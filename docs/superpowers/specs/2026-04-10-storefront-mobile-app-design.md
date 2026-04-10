# Mark8ly Storefront Mobile App — Design Spec

**Date:** 2026-04-10
**Status:** Draft

## 1. Overview

A per-merchant branded Expo React Native shopping app on iOS and Android, gated to **Enterprise+ plan** ($499 setup fee). Full parity with the web storefront — browse, search, cart, checkout (with native payment SDKs), account management, wishlists, reviews, and loyalty. Each merchant gets their own App Store / Play Store listing under their store name, with their brand colors, logo, and fonts injected at build time.

This is separate from the Admin Mobile App (all-plan, single listing, merchant-facing). The storefront app is customer-facing, one build per merchant.

### Relationship to Admin App

| | Admin App | Storefront App (this spec) |
|---|---|---|
| **User** | Merchant / staff | Customer / shopper |
| **Plan gating** | All plans | Enterprise+ ($499 setup) |
| **App Store listing** | One: "Mark8ly Admin" | Per-merchant: "{Store Name}" |
| **Auth** | GIP `mp-internal` tenant pool | GIP `mp-customer` tenant pool |
| **API surface** | `/api/v1/mobile/admin/` | `/api/v1/mobile/storefront/` |
| **Branding** | Mark8ly Paper/Ink/Moss | Merchant's B1 branding config |
| **Guest access** | No — login required | Yes — browse/cart without auth |

### Dependencies

- **B1 (Storefront Branding)** must ship first — defines the merchant's logo, palette, fonts stored in marketplace-api. The mobile app consumes B1 output at build time and runtime.
- **`packages/mobile-shared/`** defined in the admin app spec — shared auth, API client, push registration, Zustand stores.

### Constraints

- Lives at `apps/storefront-mobile/` in the mark8ly monorepo
- Reuses `packages/mobile-shared/` (auth, API client, push, stores)
- Uses `@tesserix/native`, `@tesserix/tokens`, `@tesserix/hooks`, `@tesserix/icons` from the design-system repo
- Merchant branding from B1 overrides default Paper/Ink/Moss tokens
- Light mode only (brand requirement)
- **New mobile route group required on marketplace-api** — existing storefront endpoints use `X-Storefront-Key` or auth-bff cookies, neither works from a mobile client. A `/api/v1/mobile/storefront/` route group with `GIPBearerAuth` middleware is needed (same pattern as admin app's `/api/v1/mobile/admin/`).
- One new endpoint needed: `POST /api/v1/mobile/storefront/stores/:slug/push-tokens` for customer push registration
- Expo managed workflow (~52, React Native 0.76.x)

## 2. Architecture

### 2.1 Repo Structure

```
mark8ly/
├── packages/
│   └── mobile-shared/               # Shared (defined in admin app spec)
│       ├── api/
│       │   ├── client.ts            # Base HTTP client (auth headers, errors)
│       │   ├── types.ts             # Shared API response/request types
│       │   ├── products.ts          # Product endpoints (list, detail)
│       │   ├── orders.ts            # Order endpoints (list, detail)
│       │   ├── customers.ts         # Customer endpoints
│       │   ├── notifications.ts     # Notification endpoints
│       │   └── checkout-types.ts    # CheckoutItemBody, ShippingRateBody, etc.
│       ├── auth/
│       │   ├── provider.tsx         # AuthProvider (configurable GIP tenant pool)
│       │   ├── gip.ts              # Firebase/GIP auth wrapper
│       │   └── token-storage.ts    # Secure token persistence (expo-secure-store)
│       ├── push/
│       │   └── registration.ts     # Expo push token registration + permissions
│       ├── deep-links/
│       │   └── validator.ts        # Route allowlist + param validation for deep links
│       ├── haptics/
│       │   └── feedback.ts         # Haptic feedback wrappers (light, medium, success, error)
│       └── stores/
│           └── auth-store.ts       # Zustand — auth state
│
├── apps/
│   └── storefront-mobile/           # NEW — per-merchant branded Expo app
│       ├── app.json
│       ├── eas.json                 # Per-merchant build profiles (no secrets — use EAS Secrets)
│       ├── scripts/
│       │   └── generate-brand.ts   # Fetches B1 config → generates icon/splash/theme
│       ├── app/
│       │   ├── _layout.tsx          # Root: providers, theme injection, splash
│       │   ├── (auth)/
│       │   │   ├── login.tsx        # Email/password login
│       │   │   └── register.tsx     # Customer registration
│       │   ├── (tabs)/
│       │   │   ├── _layout.tsx      # TabBar: Home, Browse, Cart, Account
│       │   │   ├── index.tsx        # Home (storefront homepage)
│       │   │   ├── browse/
│       │   │   │   ├── _layout.tsx         # Persistent SearchBar in header
│       │   │   │   ├── index.tsx           # Categories + featured products
│       │   │   │   ├── search.tsx          # Search results
│       │   │   │   ├── category/[slug].tsx # Category listing
│       │   │   │   └── product/[handle].tsx # Product detail
│       │   │   ├── cart.tsx         # Cart screen
│       │   │   └── account/
│       │   │       ├── index.tsx           # Account dashboard
│       │   │       ├── orders.tsx          # Order history
│       │   │       ├── orders/[id].tsx     # Order detail
│       │   │       ├── addresses.tsx       # Saved addresses
│       │   │       ├── wishlist.tsx        # Saved products
│       │   │       ├── loyalty.tsx         # Loyalty dashboard
│       │   │       └── reviews.tsx         # My reviews
│       │   └── checkout/
│       │       ├── _layout.tsx      # Stack nav with progress bar (4 steps)
│       │       ├── details.tsx      # Step 1: Contact + Address (merged)
│       │       ├── shipping.tsx     # Step 2: Shipping method selection
│       │       ├── payment.tsx      # Step 3: Payment method selection
│       │       ├── review.tsx       # Step 4: Order summary + discounts + confirm
│       │       └── confirmation/[id].tsx # Order placed + account creation prompt
│       ├── lib/
│       │   ├── storefront-api/      # Storefront-specific API calls
│       │   │   ├── checkout.ts      # Shipping rates, submit checkout
│       │   │   ├── loyalty.ts       # Program config, enroll, redeem
│       │   │   ├── wishlist.ts      # Add/remove/list
│       │   │   ├── reviews.ts       # Submit, list my reviews
│       │   │   └── store.ts         # Store metadata + B1 branding config
│       │   └── theme/
│       │       └── merchant-theme.ts # Build B1 config into @tesserix/tokens overrides
│       ├── stores/
│       │   ├── cart-store.ts        # Zustand — cart state (persisted via MMKV)
│       │   ├── checkout-store.ts    # Zustand — multi-step checkout state
│       │   └── recently-viewed-store.ts # Zustand — recently viewed products (MMKV)
│       └── components/              # Storefront-specific compositions
│           ├── ProductCard.tsx
│           ├── ProductGallery.tsx
│           ├── VariantSelector.tsx
│           ├── CartItem.tsx
│           ├── CheckoutProgress.tsx
│           ├── CouponInput.tsx
│           ├── GiftCardInput.tsx
│           ├── LoyaltyRedemption.tsx
│           ├── CategoryPills.tsx
│           ├── HomeBanner.tsx
│           ├── WishlistButton.tsx
│           ├── NotifyMeButton.tsx
│           ├── ReviewCard.tsx
│           ├── StarRating.tsx
│           └── RecentlyViewed.tsx
```

### 2.2 Key Dependencies

| Package | Purpose |
|---------|---------|
| `expo` ~52 | Expo SDK |
| `expo-router` | File-based navigation |
| `expo-secure-store` | Token persistence |
| `expo-notifications` | Push notifications |
| `expo-haptics` | Haptic feedback |
| `expo-image` | Optimized image loading |
| `@react-native-firebase/auth` | GIP native auth (`mp-customer` pool) |
| `@stripe/stripe-react-native` | Stripe payments (Apple Pay, Google Pay, cards) |
| `razorpay-react-native` | Razorpay payments |
| `@tanstack/react-query` | Server state (consistent with web storefront) |
| `zustand` | Client state — cart, checkout, auth |
| `react-native-mmkv` | Fast persistent storage for cart + recently viewed |
| `@tesserix/native` | 123 components from design system |
| `@tesserix/tokens` | Design tokens (overridden by B1 branding) |
| `@tesserix/hooks` | Shared hooks (debounce, async, etc.) |
| `@tesserix/icons` | Lucide icons for React Native |

> **Note on PayPal:** `@paypal/react-native-checkout` has unstable availability for Expo managed workflow. PayPal support will use a WebView fallback (`PayPalWebView` component wrapping the PayPal JS SDK) if no stable native package exists at implementation time. Stripe and Razorpay are the priority native integrations.

### 2.3 Navigation

**Bottom tabs (4):**

```
[ Home ]  [ Browse ]  [ Cart (badge) ]  [ Account ]
```

- Home: storefront homepage (featured products, banners, categories, recently viewed)
- Browse: **persistent SearchBar in header** (always visible, not buried), category grid, product listings
- Cart: cart contents with item count badge on tab icon
- Account: profile, orders, addresses, wishlist, loyalty, reviews (auth-gated)

**Checkout** is a separate stack navigator (not in tabs) — entered from cart via "Checkout" CTA. Back button returns to cart. Progress bar shows 4-step position.

**Product detail** pushes onto the Browse stack. Gallery, variants, add-to-cart, wishlist, reviews all on one screen.

**Deep linking:** Each merchant app gets its own URL scheme (`{slug}-store://`) and associated domain for universal links (`{slug}.mark8ly.com/.well-known/apple-app-site-association`). All deep link paths are validated against an allowlist of known route patterns before navigation. UUIDs and handles are validated for format before being passed to API calls. See `packages/mobile-shared/deep-links/validator.ts`.

### 2.4 Shared vs App-Local API Boundary

Clear criterion: **`mobile-shared/api/`** contains endpoints used by both admin and storefront apps. **`storefront-mobile/lib/storefront-api/`** contains endpoints exclusive to the shopping experience.

| Module | Location | Rationale |
|--------|----------|-----------|
| `products.ts` (list, detail) | `mobile-shared/api/` | Admin views products too |
| `orders.ts` (list, detail) | `mobile-shared/api/` | Admin manages orders too |
| `customers.ts` (profile) | `mobile-shared/api/` | Admin views customer profiles too |
| `checkout-types.ts` | `mobile-shared/api/` | Admin may create orders in v2 |
| `checkout.ts` (rates, submit) | `storefront-mobile/lib/` | Shopping-only flow |
| `loyalty.ts` | `storefront-mobile/lib/` | Customer-facing only |
| `wishlist.ts` | `storefront-mobile/lib/` | Customer-facing only |
| `reviews.ts` | `storefront-mobile/lib/` | Customer submits; admin moderates (different API) |
| `store.ts` (branding) | `storefront-mobile/lib/` | Only storefront consumes B1 config |

## 3. Authentication

### 3.1 Flow

1. App launches → check `expo-secure-store` for saved GIP refresh token
2. Token exists → Firebase SDK silent refresh → hydrate auth store → navigate to tabs
3. Token missing → navigate to tabs anyway (guest browsing allowed)
4. Auth screens shown only when user taps Account tab or hits an auth-gated action
5. Login: email/password via `@react-native-firebase/auth` against GIP `mp-customer` pool
6. Register: same SDK, then `POST /api/v1/mobile/storefront/stores/{slug}/customers/register` to create customer profile
7. All authenticated API calls: `Authorization: Bearer <idToken>` + store slug from baked-in config
8. Logout: clear secure store, clear push token, reset cart (optional), navigate to home

### 3.2 Guest vs Authenticated

| Action | Guest | Authenticated |
|--------|-------|---------------|
| Browse products | Yes | Yes |
| Search | Yes | Yes |
| View product detail | Yes | Yes |
| Add to cart | Yes | Yes |
| Checkout | Guest checkout (email required) | Full checkout |
| View order history | No | Yes |
| Wishlist | No — prompt login | Yes |
| Submit review | No — prompt login | Yes |
| Loyalty enrollment | No — prompt login | Yes |
| Redeem loyalty points | No | Yes |
| Manage addresses | No | Yes |
| Notify me (back in stock) | No — prompt login | Yes |

### 3.3 Mid-Checkout Account Recognition

When a guest enters their email in checkout Step 1 (Details), the app checks if an existing customer account is associated with that email via a lightweight `HEAD /api/v1/mobile/storefront/stores/{slug}/customers/check?email=` endpoint. If found:
- Inline nudge: "We found an account for this email — sign in to use your saved addresses and earn loyalty points"
- "Sign in" link opens login modal (not full-screen navigation — preserves checkout state)
- On successful login, checkout state persists and saved addresses pre-populate

### 3.4 Post-Checkout Account Creation

On the confirmation screen, if the customer completed as a guest:
- Prompt: "Save your details for next time — create an account in one tap"
- Pre-filled email from checkout, only needs a password
- On success: associate the just-placed order with the new account
- Dismissible — not blocking

### 3.5 Multi-Tenant

No store switcher. The store slug is baked into `app.json` / `eas.json` at build time via `generate-brand.ts`. Every API call uses this slug. The app IS the merchant's store.

### 3.6 Reuse from `packages/mobile-shared/`

- `auth/provider.tsx` — same AuthProvider, configured with `mp-customer` pool instead of `mp-internal`
- `auth/gip.ts` — same Firebase wrapper (pool is a config parameter)
- `auth/token-storage.ts` — same secure store logic
- `stores/auth-store.ts` — same Zustand auth store
- `api/client.ts` — same base HTTP client (slug from baked-in config instead of store switcher)

## 4. Backend Changes

### 4.1 New Mobile Storefront Route Group

The web storefront uses `X-Storefront-Key` headers (public endpoints) and auth-bff session cookies (authenticated endpoints). Neither mechanism works from a mobile client sending Bearer tokens directly.

**Required:** A new `/api/v1/mobile/storefront/` route group on marketplace-api with `gosharedmw.GIPBearerAuth("mp-customer")` middleware, mirroring the admin app's `/api/v1/mobile/admin/` pattern.

**Public endpoints** (no auth, `X-Storefront-Key` not required for mobile):

```
GET  /api/v1/mobile/storefront/stores/:slug/products
GET  /api/v1/mobile/storefront/stores/:slug/products/:handle
GET  /api/v1/mobile/storefront/stores/:slug/categories
GET  /api/v1/mobile/storefront/stores/:slug/payment-methods
GET  /api/v1/mobile/storefront/stores/:slug/branding
HEAD /api/v1/mobile/storefront/stores/:slug/customers/check?email=
```

**Authenticated endpoints** (Bearer token required, `customer_id` derived from token):

```
# Checkout
POST /api/v1/mobile/storefront/stores/:slug/checkout/shipping-rates
POST /api/v1/mobile/storefront/stores/:slug/checkout/submit

# Orders
GET  /api/v1/mobile/storefront/stores/:slug/orders
GET  /api/v1/mobile/storefront/stores/:slug/orders/:id

# Wishlist
GET    /api/v1/mobile/storefront/stores/:slug/wishlist
POST   /api/v1/mobile/storefront/stores/:slug/wishlist
DELETE /api/v1/mobile/storefront/stores/:slug/wishlist/:productId

# Reviews
GET  /api/v1/mobile/storefront/stores/:slug/reviews/mine
POST /api/v1/mobile/storefront/stores/:slug/products/:handle/reviews

# Addresses
GET    /api/v1/mobile/storefront/stores/:slug/addresses
POST   /api/v1/mobile/storefront/stores/:slug/addresses
PUT    /api/v1/mobile/storefront/stores/:slug/addresses/:id
DELETE /api/v1/mobile/storefront/stores/:slug/addresses/:id

# Loyalty
GET  /api/v1/mobile/storefront/stores/:slug/loyalty/program
GET  /api/v1/mobile/storefront/stores/:slug/loyalty/me
POST /api/v1/mobile/storefront/stores/:slug/loyalty/enroll
POST /api/v1/mobile/storefront/stores/:slug/loyalty/redeem

# Customers
POST /api/v1/mobile/storefront/stores/:slug/customers/register
GET  /api/v1/mobile/storefront/stores/:slug/customers/profile

# Push
POST   /api/v1/mobile/storefront/stores/:slug/push-tokens
DELETE /api/v1/mobile/storefront/stores/:slug/push-tokens/:tokenId

# Notify me (back in stock / price drop)
POST   /api/v1/mobile/storefront/stores/:slug/products/:handle/notify
DELETE /api/v1/mobile/storefront/stores/:slug/products/:handle/notify
```

**Guest checkout** also goes through the authenticated route group but with a guest token flow: the app sends a temporary anonymous Firebase token or the endpoint accepts unauthenticated checkout with email-only identification. Server re-derives cart pricing from product handles + quantities on every `submitCheckout()` — client-provided prices are never trusted.

### 4.2 Push Token Table

```sql
CREATE TABLE storefront_push_tokens (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID        NOT NULL,
    store_slug  VARCHAR(63) NOT NULL,
    customer_id UUID        NOT NULL,
    device_id   VARCHAR(255) NOT NULL,
    token       TEXT        NOT NULL,
    platform    VARCHAR(10) NOT NULL CHECK (platform IN ('ios', 'android')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (customer_id, device_id)
);

CREATE INDEX idx_storefront_push_tokens_store ON storefront_push_tokens (store_slug);
```

- `customer_id` derived server-side from the Bearer token — never from request body
- `device_id` included for upsert on reinstall (prevents orphaned rows)
- `UNIQUE (customer_id, device_id)` allows token rotation without creating duplicates
- Rate limit: 5 registrations per customer per hour (matches admin app)

### 4.3 Notify-Me Table

```sql
CREATE TABLE product_notify_subscriptions (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID        NOT NULL,
    store_slug  VARCHAR(63) NOT NULL,
    customer_id UUID        NOT NULL,
    product_id  UUID        NOT NULL,
    notify_type VARCHAR(20) NOT NULL CHECK (notify_type IN ('back_in_stock', 'price_drop')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (customer_id, product_id, notify_type)
);
```

When inventory.back_in_stock or product.price_changed events fire, the push handler queries this table to find subscribed customers and sends push notifications.

## 5. Data Strategy

- **TanStack React Query** for all server state (consistent with web)
- **Stale-while-revalidate** — cached data shown immediately, background refresh
- **Cart persistence**: Zustand + `react-native-mmkv` for instant hydration on app launch
- **Recently viewed**: last 20 products stored in MMKV, displayed on Home screen
- **Search history**: last 10 searches stored in MMKV, rendered as Text components only (never via WebView/innerHTML)
- **Pull-to-refresh** on all list screens
- **Infinite scroll** on product listings and order history
- **Optimistic updates** for wishlist toggle, cart quantity changes
- **Offline behavior**: read-only graceful degradation
  - Cached product pages, cart remain viewable offline
  - Checkout and mutations show "No connection" toast
  - No full offline-first sync (overkill for v1)
- **Image caching**: `expo-image` with disk cache for product images

### Cart Type Mapping

The client-side `CartItem` must map cleanly to the server's `CheckoutItemBody`:

```typescript
// Client-side (cart-store.ts)
interface CartItem {
  productId: string;
  variantId: string;
  handle: string;       // product handle for routing
  title: string;        // display only — not sent to server
  priceAmount: string;  // display only — server recalculates
  currencyCode: string;
  qty: number;
  imageUrl?: string;    // display only
}

// Sent to server on submitCheckout()
interface CheckoutLineItem {
  product_id: string;   // from CartItem.productId
  variant_id: string;   // from CartItem.variantId
  quantity: number;      // from CartItem.qty
  // NO price fields — server derives from product_id + variant_id
}
```

The server recalculates `title_snapshot`, `sku_snapshot`, `line_total`, and all pricing from product/variant IDs. Client-provided display prices are never trusted.

### Query Key Structure

```
["store", slug, "products"]
["store", slug, "products", handle]
["store", slug, "categories"]
["store", slug, "orders"]               // authenticated
["store", slug, "wishlist"]             // authenticated
["store", slug, "loyalty", "me"]        // authenticated
["store", slug, "loyalty", "program"]   // public
["store", slug, "reviews", "mine"]      // authenticated
["store", slug, "addresses"]            // authenticated
["store", slug, "recently-viewed"]      // local only (MMKV)
```

## 6. Screens

### 6.1 Home

- **Hero banner**: merchant-configured banner image or featured collection (from B1 branding config)
- **Category row**: horizontal scroll of category pills (CategoryPills)
- **Featured products**: horizontal carousel of featured/promoted products (ProductCard)
- **New arrivals**: grid of recent products
- **Recently viewed**: horizontal carousel of last viewed products (populated from MMKV store)
- **Pull-to-refresh**
- Merchant's branding colors applied via theme
- **Empty state for unconfigured merchants**: if no featured products or banners are configured, show a "Store coming soon" placeholder with the merchant's logo and a "Browse all products" CTA

### 6.2 Browse

**Layout** (_layout.tsx)
- **Persistent SearchBar in the header** — always visible without scrolling or tapping into a search mode. Tapping focuses and navigates to search.tsx with keyboard open.

**Category grid** (index.tsx)
- Grid of category cards with image and name
- "View all products" option

**Search** (search.tsx)
- SearchBar auto-focused on entry
- Recent searches shown below (from MMKV, rendered as Text components — no WebView)
- Debounced input (`useDebounce` from `@tesserix/hooks`)
- Real-time product results as grid
- EmptyState when no results

**Category listing** (category/[slug].tsx)
- Product grid filtered by category
- Sort options (BottomSheet): newest, price low-high, price high-low
- Infinite scroll pagination
- Pull-to-refresh

**Product detail** (product/[handle].tsx)
- **Media gallery**: swipeable full-width images (ImageGallery from `@tesserix/native`), pinch-to-zoom with magnifier icon affordance on first image ("tap to zoom")
- **Title + price + rating**: product name, price, compare-at price (strikethrough if discounted), **average star rating + review count inline** (e.g., "4.3 (127 reviews)" — tappable, scrolls to reviews section)
- **Variant selector**: option chips (Color, Size, etc.) with availability checking — greyed out for unavailable combinations
- **Stock indicator**: "In stock", "Low stock — X left", "Out of stock"
- **Notify me button**: shown when out of stock (auth-gated). "Notify me when available" — registers for back-in-stock push notification via `POST .../products/:handle/notify`
- **Add to cart button**: sticky at bottom, disabled when out of stock. **Haptic feedback**: light impact on tap, success notification on add.
- **Wishlist heart icon**: top-right corner, auth-gated. **Haptic feedback**: light impact on toggle.
- **Reviews section** (collapsible Accordion, expanded by default): average rating breakdown, recent reviews list, "Write a review" CTA (auth-gated)
- **Description** (collapsible Accordion, collapsed by default): expandable text section
- **Related products**: horizontal carousel at bottom

### 6.3 Cart

- List of CartItem components: thumbnail, name, variant, unit price, quantity stepper (+/-), line total
- **Haptic feedback**: light impact on quantity change, medium impact on remove
- Swipe-to-delete on each item (with confirmation haptic)
- **"Save for later" action** on swipe (moves item to wishlist, auth-gated — prompts login if guest)
- **Out-of-stock warning**: if a cart item's stock has changed since it was added (detected on pull-to-refresh or app foreground), show an inline warning Banner on that item: "Only X left" or "Out of stock — remove to continue checkout"
- **Subtotal** displayed at bottom
- **"Checkout" CTA button** — sticky at bottom, disabled if any cart item is out of stock
  - If guest: prompt login/register or continue as guest
  - If authenticated: enter checkout flow
- **EmptyState** when cart is empty: illustration + "Start shopping" CTA
- Cart count badge on tab icon reflects total items

### 6.4 Checkout (Stack Navigator — 4 Steps)

**Progress bar**: `CheckoutProgress` component showing 4 steps with current highlight. Displayed in the stack header.

**Step 1: Details** (details.tsx) — Contact + Address merged
- **Contact section**:
  - If authenticated: pre-filled email + name, editable
  - If guest: email + name input fields
  - **Account recognition**: on email blur, check if account exists. If found, show inline nudge: "We found an account — sign in for saved addresses and loyalty points." Sign-in link opens login modal overlay (preserves checkout state).
  - Login/register prompt for guests: "Have an account? Sign in for faster checkout"
- **Address section** (below contact, same screen):
  - If authenticated with saved addresses: address picker (Radio list) + "Add new address"
  - If guest or no saved addresses: address form
  - Address form: name, line1, line2, city, region, postal code, country (Select)
  - Country list: AU, CA, DE, ES, FR, GB, ID, IN, IT, MY, NL, PH, SG, TH, US
  - "Save address" checkbox (authenticated only)
- "Continue" CTA

**Step 2: Shipping** (shipping.tsx)
- Fetches shipping rates from `fetchShippingRates()` based on address + cart
- List of shipping options: carrier name, estimated delivery, price (Radio list)
- Loading skeleton while rates fetch
- **Error state**: if zero rates returned, show "We couldn't find shipping options for this address. Please check your address or try a different one." with a "Go back" link.
- "Continue" CTA

**Step 3: Payment** (payment.tsx)
- **Payment method selection**:
  - Apple Pay / Google Pay (via Stripe SDK — shown first if available)
  - Credit/debit card (Stripe `CardField` component)
  - Razorpay
  - PayPal (WebView fallback if native SDK unavailable)
- **Error state**: if payment methods fail to load, show "Unable to load payment options — please try again" with retry button. Timeout after 10s.
- "Continue" CTA

**Step 4: Review** (review.tsx)
- Full order summary:
  - Line items (thumbnail, name, variant, qty, price)
  - Contact info (editable via "Edit" link → navigates back to Step 1)
  - Shipping address (editable via "Edit" link)
  - Shipping method + cost
  - Payment method
- **Discounts section** (inline on review screen where user sees running total):
  - CouponInput: code field + "Apply" button, shows discount amount on success
  - GiftCardInput: code field + "Check balance" button, shows applied amount
  - LoyaltyRedemption: toggle to redeem points, shows points available + currency value (authenticated only)
- **Totals**: subtotal, shipping, discount, tax (estimated), total
- **"Place order" CTA**
  - **Haptic feedback**: success notification on order placed
  - Idempotency key generated via `crypto.randomUUID()`
  - Loading state during submission (button disabled, spinner)
  - Error handling: payment failure → stay on review with error message, network failure → retry prompt

**Server-side cart validation on submit**: the `submitCheckout()` endpoint receives only `CheckoutLineItem[]` (product_id, variant_id, quantity) — no prices. Server re-derives all pricing, verifies stock, calculates tax, and returns the final total. If the server-calculated total differs from the client display total by more than rounding tolerance, the review screen shows an updated total and asks the customer to confirm before proceeding.

**Confirmation** (confirmation/[id].tsx)
- Order number
- "Thank you" message with success haptic
- Order summary
- **Guest account creation prompt**: "Save your details for faster checkout next time" — pre-filled email, just needs password. Dismissible.
- "Continue shopping" CTA → navigates to home
- Cart cleared on arrival

### 6.5 Account

**Dashboard** (index.tsx) — auth-gated, shows login screen if not authenticated
- Profile header: name, email
- Quick links grid:
  - Orders (with count)
  - Addresses
  - Wishlist (with count)
  - Loyalty (points balance)
  - Reviews
  - Logout

**Order History** (orders.tsx)
- List of past orders: order number, date, status badge, total, item count
- Tap → order detail
- Pull-to-refresh + infinite scroll
- EmptyState when no orders

**Order Detail** (orders/[id].tsx)
- Order header: number, date, status badge
- Line items: thumbnail, name, variant, qty, price
- Shipping: address, method, tracking number (if available)
- Payment: method, amount
- Totals: subtotal, shipping, discounts, tax, total
- Timeline of order events

**Addresses** (addresses.tsx)
- List of saved addresses
- Default address indicator
- Add / edit / delete actions
- "Set as default" option

**Wishlist** (wishlist.tsx)
- Grid of wishlisted products (ProductCard with heart icon filled)
- Tap → product detail
- Swipe-to-remove
- "Add to cart" quick action on each card
- EmptyState when empty

**Loyalty** (loyalty.tsx)
- Points balance prominently displayed
- Current tier + progress to next tier (ProgressBar)
- Points history (earned/redeemed)
- Referral code with share/copy action (ShareButton)
- **Referral code security**: codes are cryptographically random (min 12 chars, alphanumeric), rate-limited server-side (max 10 referral redemptions per code per day), and tied to customer_id
- Enrollment prompt if not yet enrolled

**Reviews** (reviews.tsx)
- List of reviews the customer has written
- Each: product thumbnail, star rating, review text, date
- Tap → product detail

## 7. Haptic Feedback

Haptic feedback is a first-class UX concern, not an afterthought. Shared via `packages/mobile-shared/haptics/feedback.ts` using `expo-haptics`.

| Action | Haptic Type | Intensity |
|--------|------------|-----------|
| Add to cart | `ImpactFeedbackStyle.Light` | Light tap |
| Remove from cart | `ImpactFeedbackStyle.Medium` | Medium tap |
| Quantity change (+/-) | `ImpactFeedbackStyle.Light` | Light tap |
| Wishlist toggle | `ImpactFeedbackStyle.Light` | Light tap |
| Swipe-to-delete threshold | `ImpactFeedbackStyle.Medium` | Medium tap |
| Order placed successfully | `NotificationFeedbackType.Success` | Success buzz |
| Payment failure | `NotificationFeedbackType.Error` | Error buzz |
| Pull-to-refresh trigger | `ImpactFeedbackStyle.Light` | Light tap |
| Checkout step transition | `ImpactFeedbackStyle.Light` | Light tap |

All haptics respect the device's haptic settings (disabled if system haptics are off).

## 8. Push Notifications

### 8.1 Registration

- On login success (or register), request notification permissions via `expo-notifications`
- Register Expo push token: `POST /api/v1/mobile/storefront/stores/:slug/push-tokens`
  - Request body: `{ "token": "ExponentPushToken[xxx]", "device_id": "<unique-device-id>", "platform": "ios|android" }`
  - **Auth required**: Bearer token in header. `customer_id` derived server-side — never from request body.
  - Rate limit: 5 registrations per customer per hour
- On logout: delete push token registration
- Guests: no push registration (no user ID to associate)
- Handle permission denied gracefully (in-app experience unaffected)

### 8.2 Events That Trigger Push

| Event | Title | Body | Deep link |
|-------|-------|------|-----------|
| Order confirmed | "Order confirmed" | "Order #1234 is confirmed" | `account/orders/[id]` |
| Order shipped | "Order shipped" | "Order #1234 is on its way" | `account/orders/[id]` |
| Order delivered | "Order delivered" | "Order #1234 has been delivered" | `account/orders/[id]` |
| Points earned | "Points earned" | "+50 points from your purchase" | `account/loyalty` |
| Tier upgrade | "Tier upgrade" | "You've reached Gold tier!" | `account/loyalty` |
| Wishlist price drop | "Price drop" | "Widget X is now $19.99" | `browse/product/[handle]` |
| Back in stock | "Back in stock" | "Widget X is available again" | `browse/product/[handle]` |

### 8.3 Deep Link Validation

All push notification deep links are validated before navigation via `packages/mobile-shared/deep-links/validator.ts`:

```typescript
const ALLOWED_ROUTES = [
  /^account\/orders\/[a-f0-9-]{36}$/,
  /^account\/loyalty$/,
  /^browse\/product\/[a-z0-9-]+$/,
];

function validateDeepLink(path: string): boolean {
  return ALLOWED_ROUTES.some(pattern => pattern.test(path));
}
```

Malformed or unrecognized paths are silently dropped (navigate to home instead). No raw path segments are passed to API calls — IDs are extracted and validated independently.

## 9. Payments

### 9.1 Stripe (Primary)

- `@stripe/stripe-react-native` SDK
- **Apple Pay**: enabled via Stripe SDK on iOS (no additional SDK needed)
- **Google Pay**: enabled via Stripe SDK on Android
- **Card payments**: Stripe `CardField` component for PCI-compliant card entry
- **Publishable key**: injected via EAS Secrets (not committed to source — see Section 11.1)
- Payment flow:
  1. Review screen → "Place order"
  2. `submitCheckout()` → returns `payment_token` (Stripe PaymentIntent client secret)
  3. `confirmPayment(clientSecret)` via Stripe SDK
  4. On success → navigate to confirmation
  5. On failure → show error, stay on review screen

### 9.2 Razorpay

- `razorpay-react-native` SDK
- Payment flow:
  1. `submitCheckout()` → returns Razorpay order ID
  2. Open Razorpay checkout modal (native SDK handles UI)
  3. On success → callback with payment ID → confirm with backend → navigate to confirmation
  4. On failure → show error

### 9.3 PayPal

- WebView-based fallback wrapping PayPal JS SDK (native RN SDK availability uncertain for Expo managed)
- Payment flow:
  1. `submitCheckout()` → returns PayPal order ID
  2. Open PayPal WebView checkout
  3. On approval → capture payment → navigate to confirmation
  4. On cancel/failure → show error

### 9.4 Payment Method Availability

Which payment methods show depends on the merchant's payment settings (already configured via admin web):
- `fetchPaymentMethods(storeSlug)` returns enabled providers
- Apple Pay / Google Pay shown only when device supports them AND Stripe is enabled

## 10. Merchant Branding (B1 Integration)

### 10.1 Build-Time Branding

`scripts/generate-brand.ts` runs before each EAS build:

1. **Auth check**: verifies the build is triggered by an authorized pipeline (checks EAS project slug against merchant database — prevents unauthorized builds for arbitrary slugs)
2. Fetches merchant's B1 branding config from `GET /api/v1/mobile/storefront/stores/:slug/branding` (using a build-time service account token, not a public endpoint)
3. Generates:
   - **App icon** — merchant's logo on their brand background color
   - **Splash screen** — merchant's logo centered on brand background
   - **`app.json` overrides** — app name (store name), bundle ID (`com.mark8ly.store.{slug}`), scheme (`{slug}-store`)
4. Generates `theme-overrides.json` — merchant's color palette mapped to `@tesserix/tokens` semantic slots

### 10.2 Runtime Theming

`lib/theme/merchant-theme.ts`:

1. Reads `theme-overrides.json` (baked in at build time)
2. Creates a `@tesserix/tokens` theme override:
   - `primary` → merchant's primary color
   - `accent` → merchant's accent color
   - `background` → merchant's background color (or default Paper)
   - Typography → merchant's chosen font family
3. Wraps app in `ThemeProvider` with merged tokens
4. All `@tesserix/native` components automatically pick up the merchant theme

### 10.3 Fallback

If B1 branding is not configured (merchant hasn't set it up yet):
- Default to Paper/Ink/Moss palette
- Default to Source Sans 3 font
- App icon: Mark8ly logo with store name
- This ensures the app can be built even before the merchant completes branding setup

## 11. Build & Distribution

### 11.1 EAS Build Profiles

Each merchant gets an EAS build profile in `eas.json`:

```json
{
  "build": {
    "merchant-acme": {
      "extends": "production",
      "env": {
        "STORE_SLUG": "acme",
        "STORE_NAME": "Acme Store",
        "BUNDLE_ID_SUFFIX": "acme"
      }
    }
  }
}
```

**Secrets management**: `STRIPE_PUBLISHABLE_KEY`, `RAZORPAY_KEY_ID`, and any other per-merchant keys are stored in **EAS Secrets** (environment-level), never committed to `eas.json` or source control. The `generate-brand.ts` script reads them from EAS Secrets at build time.

- `generate-brand.ts` runs as a pre-build hook, fetching B1 config for the slug
- Bundle ID: `com.mark8ly.store.{slug}` (unique per merchant for App Store)
- App name: merchant's store name
- URL scheme: `{slug}-store://` for deep links

### 11.2 OTA Update Security

- **EAS Code Signing** is required for all production OTA updates — the native binary verifies the signature of every JS bundle before loading
- One OTA channel per merchant (`{slug}-production`)
- OTA updates are signed with the project's code signing key (managed by EAS)
- This prevents malicious JS injection even if EAS credentials are compromised

### 11.3 Scalability

At **<50 Enterprise merchants**, static `eas.json` profiles are manageable.

At **50+ merchants**, transition to a dynamic build orchestration system:
- CLI tool (`scripts/build-merchant.ts`) that generates ephemeral EAS build configs from a database of merchant branding records
- Triggered via admin panel or API call, not manual `eas.json` edits
- Build artifacts tracked in a `merchant_builds` database table (slug, version, platform, status, store listing URLs)

### 11.4 Distribution

- **Development**: Expo Go for rapid iteration (uses default branding)
- **Preview builds**: EAS Build → internal distribution for merchant review
- **Production**: EAS Build → merchant's App Store Connect / Google Play Console accounts
- **OTA updates**: EAS Update for JS-only changes (signed, per-merchant channel)
- **CI**: GitHub Actions workflow — lint, type-check, test on PR; EAS Build triggered per merchant via admin panel or CLI

### 11.5 Merchant Onboarding Flow

1. Enterprise merchant enables "Mobile App" in admin settings
2. Mark8ly team (or automated pipeline) creates EAS build profile + EAS Secrets for merchant keys
3. `generate-brand.ts` pulls B1 branding config (with auth check)
4. EAS builds iOS + Android binaries (code-signed)
5. Merchant reviews via TestFlight / internal track
6. Merchant approves → submit to App Store / Play Store
7. Ongoing: OTA updates pushed via merchant's EAS Update channel (code-signed)

## 12. Design System Usage

### 12.1 Components from @tesserix/native

| Category | Components |
|----------|------------|
| **Navigation** | TabBar, Header, BackButton, SegmentedControl |
| **Layout** | SafeAreaView, ScrollView, Stack, Box, Divider, Container, Center |
| **Data display** | Card, Badge, Avatar, List, ListItem, Skeleton, EmptyState, Image, Chip, Rating |
| **Forms** | Input, Select, SearchBar, FormControl, Radio, RadioGroup, Accordion |
| **Feedback** | BottomSheet, Modal, Toast, Alert, Banner |
| **Interaction** | Pressable, InfiniteScroll, Carousel |
| **Shopping** | ProductCard, CartItem, WishlistButton, ReviewCard, OrderSummary |
| **Charts** | ProgressBar (loyalty tier) |
| **Workflow** | ProgressSteps (checkout) |
| **Sharing** | ShareButton (referral code), CopyToClipboard |

### 12.2 Theming

- Base: `@tesserix/tokens` semantic tokens
- Override: merchant's B1 branding config (primary, accent, background, font)
- Fallback: Paper (`#F7F6F2`) background, Ink (`#0E0E0C`) text, Moss (`#2D4A2B`) accent
- Source Sans 3 for UI text (default), merchant's chosen font as override
- Light mode only
- 6px default radius

### 12.3 Icons

- `@tesserix/icons/native` — Lucide icon set
- Tab bar icons: Home, Search, ShoppingBag, User
- Common: Heart (wishlist), Star (rating), ChevronRight, Plus, Minus, Trash2, Share2, Bell (notify me)

## 13. Security

- **Token storage**: `expo-secure-store` (Keychain on iOS, EncryptedSharedPreferences on Android)
- **No secrets in app bundle**: all API calls go through marketplace-api. Stripe publishable key injected via EAS Secrets, not source.
- **Payment PCI compliance**: Stripe SDK handles card data — never touches our code. Razorpay SDK similarly handles sensitive payment data natively. PayPal via WebView stays in PayPal's domain.
- **Certificate pinning**: deferred to v2 (API behind Cloudflare + TLS)
- **Session timeout**: Firebase SDK handles token expiry. If refresh fails → clear auth state, allow guest browsing
- **Push token security**: tokens require Bearer auth, `customer_id` derived server-side, rate-limited (5/hour/customer), `device_id` for upsert
- **Input validation**: all mutations validated server-side (existing middleware). Client-side validation via Zod for UX only.
- **Cart integrity**: cart is client-side for UX; server recalculates all prices/totals on `submitCheckout()` from product_id + variant_id + quantity only. Client cart is never trusted for pricing.
- **Deep link validation**: all deep link paths validated against route allowlist + UUID format check before navigation. Unrecognized paths redirect to home.
- **Referral codes**: cryptographically random (min 12 chars), rate-limited (10 redemptions/code/day), tied to customer_id server-side
- **OTA security**: EAS Code Signing required — native binary verifies JS bundle signature before loading
- **Build-time auth**: `generate-brand.ts` verifies build authorization against merchant database — prevents unauthorized builds for arbitrary store slugs
- **Search history**: stored locally in MMKV, rendered only as `Text` components — never via WebView or `innerHTML`

## 14. Testing

- **Unit tests**: Vitest for `packages/mobile-shared/` and storefront-specific lib code
- **Component tests**: React Native Testing Library for screen components
- **E2E**: Maestro for critical flows
- **Manual**: Expo Go for development, EAS Build for TestFlight / internal testing track

### Key Test Scenarios

**Auth:**
- Guest browse → add to cart → checkout prompts login
- Login success, login failure (wrong password, network error)
- Register → auto-login → profile created
- Token refresh, logout, auth state hydration on app launch
- Mid-checkout account recognition nudge
- Post-checkout guest account creation

**Shopping:**
- Browse categories, search products (with recent search history), view product detail
- Variant selection with availability checking
- Add to cart, update quantity, remove item, save for later
- Cart persistence across app restarts
- Out-of-stock warning in cart
- Recently viewed products
- Notify-me enrollment for out-of-stock products

**Checkout (critical path):**
- Full 4-step checkout: details → shipping → payment → review+confirm
- Guest checkout (no account)
- Saved address selection
- Coupon apply/remove, gift card apply, loyalty points redeem (on review screen)
- Payment failure → retry
- Network failure mid-checkout → recovery
- Idempotency (double-tap "Place order")
- Server price recalculation mismatch → user confirmation prompt
- Zero shipping rates → error state
- Payment methods load failure → retry

**Account:**
- Order history, order detail
- Wishlist add/remove, wishlist → add to cart
- Loyalty enrollment, points display, referral share
- Address CRUD
- Review submission

**Push:**
- Token registration on login (with device_id)
- Notification tap → correct deep link (validated)
- Malformed deep link → fallback to home
- Token cleanup on logout

**Offline:**
- Cached product pages visible offline
- Cart accessible offline
- Checkout blocked with toast when offline

**Haptics:**
- Correct haptic type fires for each action
- Haptics disabled when system setting is off

**Deep links:**
- Valid route patterns navigate correctly
- Invalid/malformed paths redirect to home
- SQL-injection-style path segments are rejected

## 15. Out of Scope (v1)

- Biometric auth (Face ID / fingerprint) — easy v2 add
- Dark mode (brand is light-only)
- Tablet-optimized layouts
- Social login (Google, Apple Sign-In) — v2 candidate
- Product reviews with photos (text-only for v1)
- In-app chat / support tickets
- Barcode/QR scanner
- AR product preview
- Certificate pinning
- Offline-first sync engine
- Real-time inventory WebSocket updates
- Multi-language / i18n (merchant's primary language only for v1)

## 16. v2 Candidates

In priority order based on customer value:
1. Social login (Apple Sign-In, Google) — reduces checkout friction
2. Biometric auth for quick re-entry
3. Review photos — customers expect this
4. Dark mode support
5. In-app chat with merchant (if D2 support tickets ships)
6. Barcode scanner for in-store product lookup
7. Tablet-optimized layouts
8. Multi-language support
9. AR product preview (category-dependent)
10. iOS widget for order tracking
