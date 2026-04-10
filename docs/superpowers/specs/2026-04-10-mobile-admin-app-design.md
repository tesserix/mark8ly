# Mark8ly Admin Mobile App — Design Spec

**Date:** 2026-04-10
**Status:** Approved

## 1. Overview

A single "Mark8ly Admin" Expo React Native app on iOS and Android, available to **all plan tiers** (Free through Marketplace). Covers the daily merchant operational workflow — dashboard, orders, products (full CRUD with camera), customers, notifications — while deferring complex admin tasks (settings, campaigns, CSV imports) to the web.

This is separate from B3 (storefront mobile app) which is a customer-facing, per-merchant branded app gated to Enterprise+. The admin app is one build, one App Store listing, all merchants log in.

### Relationship to B3

| | Admin App (this spec) | Storefront App (B3) |
|---|---|---|
| **User** | Merchant / staff | Customer |
| **Plan gating** | All plans | Enterprise+ |
| **App Store listing** | One: "Mark8ly Admin" | Per-merchant branded builds |
| **Build cost** | None to merchant | $499 setup fee |
| **Auth** | GIP `mp-internal` tenant pool | GIP `mp-customer` tenant pool |
| **API surface** | Admin endpoints | Storefront endpoints |

### Constraints

- Lives at `apps/mobile-admin/` in the mark8ly monorepo
- Shared mobile logic in `packages/mobile-shared/` (reused by B3 storefront app later)
- Uses `@tesserix/native`, `@tesserix/tokens`, `@tesserix/hooks`, `@tesserix/icons` from the design-system repo (these packages exist in the `design-system` monorepo one level up — installed as npm deps)
- Mark8ly brand palette (Paper/Ink/Moss) — not merchant branding (that's storefront)
- Light mode only (brand requirement)
- Backend changes required: new GIP Bearer token auth middleware on marketplace-api (existing `HeaderTrustAuth` only trusts Istio-injected headers), push token endpoints, push notification handler
- New endpoint: `POST /api/v1/admin/stores/:storeId/push-tokens` for push notification registration

## 2. Architecture

### 2.1 Repo structure

```
mark8ly/
├── packages/
│   └── mobile-shared/               # NEW — shared between admin & future storefront app
│       ├── package.json
│       ├── api/
│       │   ├── client.ts            # Base HTTP client (auth headers, tenant context, errors)
│       │   ├── types.ts             # Shared API response/request types
│       │   ├── orders.ts            # Order endpoints (list, detail)
│       │   ├── products.ts          # Product endpoints (list, detail)
│       │   ├── customers.ts         # Customer endpoints (list, detail)
│       │   └── notifications.ts     # Notification endpoints
│       ├── auth/
│       │   ├── provider.tsx         # AuthProvider context (configurable by GIP tenant pool)
│       │   ├── gip.ts              # Firebase/GIP auth wrapper (accepts tenantId param: 'mp-internal' for admin, 'mp-customer' for storefront)
│       │   └── token-storage.ts    # Secure token persistence (expo-secure-store)
│       ├── push/
│       │   └── registration.ts     # Expo push token registration + permissions
│       └── stores/
│           ├── auth-store.ts       # Zustand — auth state
│           └── tenant-store.ts     # Zustand — active tenant/store context
│
├── apps/
│   └── mobile-admin/                # NEW — Expo React Native admin app
│       ├── app.json
│       ├── app/
│       │   ├── _layout.tsx          # Root: providers, auth gate, splash
│       │   ├── login.tsx            # GIP native auth screen
│       │   └── (tabs)/
│       │       ├── _layout.tsx      # TabBar: Dashboard, Orders, Products, Customers, More
│       │       ├── index.tsx        # Dashboard
│       │       ├── orders/
│       │       │   ├── _layout.tsx  # SegmentedControl: All / Active / Completed / Cancelled
│       │       │   ├── index.tsx    # Order list
│       │       │   └── [id].tsx     # Order detail + actions
│       │       ├── products/
│       │       │   ├── _layout.tsx  # SegmentedControl: All / Low Stock / Inactive
│       │       │   ├── index.tsx    # Product list
│       │       │   ├── [id].tsx     # Product detail + edit
│       │       │   └── new.tsx      # Product creation wizard
│       │       ├── customers/
│       │       │   ├── index.tsx    # Customer list
│       │       │   └── [id].tsx     # Customer detail + order history
│       │       └── more/
│       │           ├── index.tsx    # More menu
│       │           ├── notifications.tsx
│       │           └── account.tsx  # Profile + store switcher + logout
│       ├── lib/
│       │   └── admin-api/           # Admin-specific API calls
│       │       ├── order-actions.ts # Confirm, fulfill, cancel, refund
│       │       ├── product-crud.ts  # Create, update, archive, media upload
│       │       └── customer-actions.ts # Block/unblock
│       └── components/              # Admin-specific UI compositions
│           ├── OrderRow.tsx
│           ├── ProductRow.tsx
│           ├── CustomerRow.tsx
│           ├── DashboardStats.tsx
│           ├── ProductMediaPicker.tsx
│           └── StoreSelector.tsx
```

### 2.2 Key dependencies

| Package | Purpose |
|---------|---------|
| `expo` ~52 | Expo SDK |
| `expo-router` | File-based navigation |
| `expo-camera` | Product photography |
| `expo-image-picker` | Gallery selection |
| `expo-image-manipulator` | Crop/rotate before upload |
| `expo-secure-store` | Token persistence |
| `expo-notifications` | Push notifications |
| `@react-native-firebase/auth` | GIP native auth |
| `@tanstack/react-query` | Server state (matches web admin) |
| `zustand` | Client state (matches web storefront) |
| `@tesserix/native` | Component library (123 components) |
| `@tesserix/tokens` | Design tokens |
| `@tesserix/hooks` | Shared hooks |
| `@tesserix/icons` | Lucide icons for RN |

### 2.3 Navigation

**Bottom tabs (5):**

```
[ Dashboard ]  [ Orders ]  [ Products ]  [ Customers ]  [ More ]
```

- `@tesserix/native` TabBar component
- Top SegmentedControl within Orders and Products for status filtering
- "More" screen serves as the overflow menu and future v2 feature entry point

## 3. Authentication

### 3.1 Auth architecture

The marketplace-api currently uses `HeaderTrustAuth` middleware which trusts `X-User-Id` and `X-Tenant-Id` headers injected by Istio's upstream JWT verification. This works for cluster-internal callers (the Next.js admin app calls from within the cluster). A mobile app calling from the public internet **cannot** use this path — Istio strips external `X-User-Id` headers.

**Solution: Add a `GIPBearerAuth` middleware to marketplace-api.**

New middleware in `internal/auth/gip_bearer.go`:
- Accepts `Authorization: Bearer <idToken>` header
- Verifies the GIP ID token signature against Google's public keys (same as `go-shared/middleware/gip_auth.go`)
- Extracts `user_id` from token `sub` claim, `tenant_id` from custom claims
- Sets `c.Set("user_id", ...)` and `c.Set("tenant_id", ...)` on the Gin context — same contract as `HeaderTrustAuth`
- Token verification uses cached Google public keys with auto-refresh

Route registration adds a second auth path:
```go
// Mobile clients: Bearer token auth
mobileAdmin := router.Group("/api/v1/mobile/admin")
mobileAdmin.Use(auth.GIPBearerAuth(gipConfig))
mobileAdmin.Use(storeMiddleware)
// ... same handlers as existing admin routes
```

This keeps the existing `HeaderTrustAuth` path untouched for cluster-internal calls. Mobile gets its own route group with Bearer auth that resolves to the same handler layer.

### 3.2 API base URL

The mobile app calls marketplace-api via the public Cloudflare → Istio ingress path:
- Base URL: `https://api.mark8ly.com` (or `https://api.tesserix.app` for dev)
- Routed by Istio VirtualService to the marketplace-api Knative service
- Cloudflare provides DDoS protection, TLS termination, and rate limiting
- The `/api/v1/mobile/admin/*` route prefix distinguishes mobile traffic for rate limiting and auth middleware selection

### 3.3 Login flow

1. App launches → check `expo-secure-store` for saved GIP refresh token
2. Token exists → Firebase SDK refreshes ID token silently → hydrate auth store → navigate to tabs
3. Token missing/expired → show login screen
4. Login: email/password via `@react-native-firebase/auth` against GIP `mp-internal` tenant pool
5. On success → store token via `expo-secure-store`, register Expo push token with marketplace-api
6. All API calls: `Authorization: Bearer <idToken>` header. Store ID embedded in URL path (`/stores/:storeId/...`), not as a header.
7. Token refresh: handled automatically by Firebase SDK
8. Logout: clear secure store, clear push token registration, navigate to login

### 3.4 Store switcher

Multi-store merchants (Pro: 3, Enterprise: 10) need to switch active store context:
- Account screen shows list of stores for current tenant
- Tapping a store updates `tenant-store.ts` (active storeId used in API URL paths)
- All queries invalidate and refetch for new store context
- Active store shown in header across all tabs

### 3.5 Backend changes

1. **New middleware**: `internal/auth/gip_bearer.go` — GIP Bearer token verification
2. **New route group**: `/api/v1/mobile/admin/stores/:storeId/*` — same handlers, Bearer auth
3. **Rate limiting**: per-user rate limit on the mobile route group (prevent token abuse)

## 4. Data Strategy

- **TanStack React Query** for all server state (consistent with web admin)
- **Stale-while-revalidate** — show cached data immediately, background refresh
- **Optimistic updates** for quick actions (confirm order, toggle product, block customer)
- **Pull-to-refresh** on all list screens (`@tesserix/native` PullToRefresh)
- **Offline behavior**: read-only graceful degradation
  - Cached screens remain viewable when offline
  - Mutation attempts show "No connection — try again when online" toast
  - No full offline-first sync engine (overkill for v1)
- **Pagination**: cursor-based infinite scroll on list screens (`@tesserix/native` InfiniteScroll)

## 5. Screens

### 5.1 Login

- Mark8ly logo + "Admin" wordmark
- Email + password fields
- "Sign in" button (Ink background)
- "Forgot password" link → opens web reset flow in browser
- Error handling: invalid credentials, network failure, account locked
- Paper background, centered form, editorial typography

### 5.2 Dashboard

Matches the existing `DashboardResponse` from the API — no new backend fields needed.

- **Stats row 1**: Revenue today, Revenue this week, Revenue this month (MetricCard x3) with `revenue_change_pct` as delta indicator
- **Revenue trend**: 7-day sparkline (LineChart using `revenue_trend` array)
- **Stats row 2**: Orders today, Pending, Fulfilled, Cancelled (compact MetricCard x4)
- **Customers**: Total customers, New this week
- **Pending reviews** count (badge — tap to open web)
- **Recent orders** list (last 5, tap → order detail)
- **Top products** list (top sellers)
- **Low stock** alerts (tap → product detail)
- **Setup checklist** for new merchants (collapsed if all complete)
- **Quick actions**: "View all orders", "Add product"
- Pull-to-refresh

### 5.3 Orders

**List** (SegmentedControl: All / Active / Completed / Cancelled)
- Each row: order number, customer name, item count, total, status Badge, relative timestamp
- SearchBar: by order number or customer name
- Swipeable rows: swipe right → quick confirm (for pending orders)
- Pull-to-refresh + infinite scroll
- EmptyState when no orders match filter

**Detail** ([id].tsx)
- Order header: number, date, status badge
- Line items: product thumbnail, name, variant, qty, price
- Customer section: name, email, tap → customer detail
- Shipping: address, method, tracking number (if fulfilled)
- Payment: method, amount, transaction ID
- **Actions** (BottomSheet):
  - Pending → Confirm
  - Confirmed → Fulfill (enter tracking number via Modal)
  - Any → Cancel (confirmation Alert)
  - Fulfilled → Refund (amount input via Modal)
- Timeline of order events at bottom

### 5.4 Products

**List** (SegmentedControl: All / Low Stock / Inactive)
- Each row: thumbnail, name, price, stock count, active/inactive status
- SearchBar: by name or SKU
- FAB → "Add Product" (new.tsx)
- Swipeable rows: swipe → archive/activate toggle
- Pull-to-refresh + infinite scroll

**Detail / Edit** ([id].tsx)
- Hero: product media gallery (ImageGallery, swipeable)
- Editable fields: name, description (plain text on mobile), price, compare-at price, SKU, barcode, stock quantity, status (Switch for active/inactive)
- Category: tap → BottomSheet with category tree (TreeView)
- Tags: TagsInput component
- Variants section: list of variants with inline price/stock editing, "Add variant" button for simple variants (option name + value + price + stock)
- Media management: add photos (camera/gallery), reorder (SortableList), delete
- Save button in header

**Creation Wizard** (new.tsx)
- 4-step FormWizard:
  1. **Photos**: camera capture + gallery picker. Lead with visuals — this is the mobile advantage. Multi-select, crop/rotate, reorder.
  2. **Details**: name, description, price, compare-at price, SKU, stock
  3. **Organization**: category picker, tags, status (draft/active)
  4. **Variants** (optional): add simple variants or skip for single-variant products
- Progress indicator via ProgressSteps
- Save as draft at any step
- Full variant matrix creation (e.g., 3 sizes x 4 colors) deferred to web

### 5.5 Customers

**List**
- Each row: Avatar, name, email, order count, total spent
- SearchBar: by name or email
- Pull-to-refresh + infinite scroll

**Detail** ([id].tsx)
- Profile header: avatar, name, email, phone, joined date
- Stats row: total orders, total spent, average order value
- Order history: recent orders list (tap → order detail)
- Review count
- **Actions**: Block/Unblock (confirmation Alert)

### 5.6 More

**Menu screen**
- Notifications (with unread count Badge)
- Account & Profile
- "Open in web browser" (deep link to web admin)
- App version

**Notifications**
- Chronological feed matching the web notification bell
- Each notification: icon, title, description, timestamp, read/unread indicator
- Tap → deep link to relevant screen (e.g., new order → order detail)
- "Mark all as read" action

**Account**
- Profile: name, email, avatar (read-only, edit via web)
- Active store indicator + store switcher (for multi-store merchants)
- Logout button

## 6. Push Notifications

### 6.1 Registration

- On login success, request notification permissions via `expo-notifications`
- Register Expo push token with marketplace-api: `POST /api/v1/mobile/admin/stores/:storeId/push-tokens`
- On logout: delete push token registration
- On app foreground resume: re-register token (Expo tokens can change after OS updates or reinstalls)
- Handle permission denied gracefully (in-app notifications still work)

### 6.2 Events that trigger push

| Event | Title | Body | Deep link |
|-------|-------|------|-----------|
| New order | "New order #1234" | "John Doe — $45.99" | `/orders/[id]` |
| Low stock | "Low stock alert" | "Widget X — 3 remaining" | `/products/[id]` |
| Order cancelled by customer | "Order cancelled" | "#1234 cancelled by customer" | `/orders/[id]` |
| Support ticket reply | "Ticket reply" | "Re: Shipping issue..." | Web link (tickets not in v1) |

### 6.3 Backend changes

New endpoints on marketplace-api (under the mobile route group):

```
POST /api/v1/mobile/admin/stores/:storeId/push-tokens
Body: { "token": "ExponentPushToken[xxx]", "platform": "ios|android", "device_id": "unique-device-id" }

DELETE /api/v1/mobile/admin/stores/:storeId/push-tokens/:tokenId
```

Registration is an upsert — if the same `(user_id, device_id)` already exists, update the token and `updated_at`. This handles token rotation on app reinstall or OS update without accumulating orphaned tokens.

New table:

```sql
CREATE TABLE admin_push_tokens (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID        NOT NULL,
    store_id    UUID        NOT NULL,
    user_id     UUID        NOT NULL,
    device_id   VARCHAR(100) NOT NULL,
    token       TEXT        NOT NULL,
    platform    VARCHAR(10) NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, device_id),
    UNIQUE (token)
);
CREATE INDEX apt_store_idx ON admin_push_tokens (store_id);
```

**Push sending**: a Pub/Sub **push subscription** (HTTP endpoint) on marketplace-api. Pub/Sub delivers messages to an HTTP handler at `/internal/push-webhook` when events fire (order.created, inventory.low_stock). This fits the Knative request-driven model — no long-running pull subscriber needed. The handler looks up push tokens for the relevant store and sends via the Expo Push API. Stale tokens (Expo returns `DeviceNotRegistered`) are deleted on delivery failure.

Rate limit: 5 push token registrations per user per hour.

## 7. Design System Usage

### 7.1 Components from @tesserix/native

| Category | Components |
|----------|------------|
| **Navigation** | TabBar, Header, BackButton, SegmentedControl |
| **Layout** | SafeAreaView, ScrollView, Stack, Box, Divider, Container |
| **Data display** | Card, MetricCard, Badge, Avatar, List, ListItem, Skeleton, EmptyState, Image |
| **Forms** | Input, Textarea, Switch, Select, SearchBar, FormControl, Form |
| **Complex** | BottomSheet, Modal, Toast, Alert, ActionSheet, FAB |
| **Interaction** | Swipeable, PullToRefresh, SortableList, InfiniteScroll, Pressable |
| **Media** | ImageGallery |
| **Charts** | LineChart |
| **Workflow** | FormWizard, ProgressSteps, Timeline |
| **Data** | TreeView (categories), TagsInput |

### 7.2 Theming

- Mark8ly brand tokens from `@tesserix/tokens` as the base theme
- Paper (`#F7F6F2`) background, Ink (`#0E0E0C`) text/primary actions, Moss (`#2D4A2B`) for links/focus/success
- Source Sans 3 for all UI text (serif reserved for web editorial moments — mobile is utilitarian)
- Light mode only
- 6px default radius

### 7.3 Icons

- `@tesserix/icons/native` — Lucide icon set
- Tab bar icons: LayoutDashboard, ShoppingBag, Package, Users, MoreHorizontal

## 8. Security

- **Token storage**: `expo-secure-store` (Keychain on iOS, EncryptedSharedPreferences on Android)
- **No secrets in app bundle**: all API calls go through marketplace-api, no direct GCP/Stripe/etc access
- **Certificate pinning**: deferred to v2 (low risk since API is behind Cloudflare + TLS)
- **Session timeout**: Firebase SDK handles token expiry. If refresh fails → force logout
- **Push token cleanup**: tokens deleted on logout, stale tokens cleaned up server-side when push delivery fails
- **Input validation**: all mutations validated server-side (existing middleware). Client-side validation via Zod for UX only.
- **Image upload**: same content-type validation as web (existing media upload endpoint)

## 9. Testing

- **Unit tests**: Vitest for shared packages (`packages/mobile-shared/`)
- **Component tests**: React Native Testing Library for screen components
- **E2E**: Maestro for critical flows (login → dashboard → view order → confirm order)
- **Manual**: Expo Go for development, EAS Build for TestFlight/internal testing track

### Key test scenarios

- Auth: login success, login failure (wrong password, network error), token refresh, logout, store switch
- Dashboard: stats load, sparklines render, recent orders tap-through
- Orders: list/filter/search, detail view, confirm/fulfill/cancel/refund actions, optimistic update rollback on failure
- Products: list/filter/search, create via wizard (with camera mock), edit fields, media reorder, variant add
- Customers: list/search, detail view, block/unblock
- Push: token registration, notification tap deep link
- Offline: cached data visible, mutations blocked with toast

## 10. Deep Linking

- **Scheme**: `mark8ly-admin://` for development, universal links for production
- **Universal links (iOS)**: `https://admin.mark8ly.com/.well-known/apple-app-site-association`
- **App links (Android)**: `https://admin.mark8ly.com/.well-known/assetlinks.json`
- **Expo Router** handles incoming deep links automatically — push notification payloads include a path (e.g., `/orders/abc123`) that Expo Router resolves to the correct screen
- **Minimum OS**: iOS 16+, Android API 24+ (Expo SDK 52 requirements)

## 11. Build & Distribution

- **Development**: Expo Go for rapid iteration
- **Preview builds**: EAS Build → internal distribution (TestFlight for iOS, internal track for Android)
- **Production**: EAS Build → App Store Connect + Google Play Console
- **CI**: GitHub Actions workflow — lint, type-check, test on PR; EAS Build on merge to main
- **OTA updates**: EAS Update for JS-only changes (no native module changes)

App Store listing: "Mark8ly Admin" — single listing, all merchants download the same app.

## 12. Out of Scope (v1)

- Marketing (coupons, gift cards, loyalty, campaigns)
- All settings screens (payments, shipping, tax, domains, team, subscription, branding)
- CSV import/export
- Support tickets
- Review moderation
- Audit logs
- Full variant matrix creation (3 sizes x 4 colors)
- Biometric auth (easy v2 add with expo-local-authentication)
- Dark mode (brand is light-only)
- Tablet-optimized layouts
- Rich text editing for product descriptions
- Certificate pinning
- Analytics/tracking SDK integration

## 13. v2 Candidates

In priority order based on merchant value:
1. Biometric auth (Face ID / fingerprint for quick re-entry)
2. Review moderation (approve/reject from notification)
3. Marketing quick actions (activate/deactivate coupons)
4. Support ticket viewing and replies
5. Barcode scanner for inventory management (expo-barcode-scanner)
6. Basic settings (store name, logo — things you might do from your phone)
7. Tablet-optimized layouts
8. Widget (iOS) for at-a-glance order count / revenue
