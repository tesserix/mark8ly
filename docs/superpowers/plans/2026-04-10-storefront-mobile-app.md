# Storefront Mobile App — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a per-merchant branded Expo React Native shopping app (Enterprise-plan gated) with full parity to the web storefront — browse, search, cart, checkout with native payment SDKs, account management, wishlists, reviews, and loyalty.

**Architecture:** Expo managed (~52) app at `apps/storefront-mobile/` consuming shared mobile logic from `packages/mobile-shared/` (API client, auth, push, haptics, deep links). Backend adds a `/api/v1/mobile/storefront/` route group on marketplace-api with `GIPBearerAuth` for mobile clients. Per-merchant builds via EAS with branding injected from B1 config.

**Tech Stack:** Expo ~52, React Native 0.76.x, expo-router, @react-native-firebase/auth, @stripe/stripe-react-native, razorpay-react-native, @tanstack/react-query, zustand, react-native-mmkv, expo-haptics, @tesserix/native (design system), @tesserix/tokens, @tesserix/hooks, @tesserix/icons

**Spec:** `docs/superpowers/specs/2026-04-10-storefront-mobile-app-design.md`

**Dependency:** B1 (Storefront Branding) must ship first for build-time branding. The app can be developed and tested with fallback Paper/Ink/Moss theming before B1 is available.

---

## Plan Overview

This plan is split into 6 phases. Each phase produces working, testable software. Phases 1-2 are foundational (shared package + backend). Phases 3-6 build the app screens incrementally.

| Phase | Scope | Depends on |
|-------|-------|------------|
| **Phase 1** | `packages/mobile-shared/` — API client, auth, push, haptics, deep links, stores | Nothing |
| **Phase 2** | Backend — `/api/v1/mobile/storefront/` route group, push token + notify-me tables, migrations | Phase 1 (types) |
| **Phase 3** | App shell — Expo project, navigation, theming, EAS config, `generate-brand.ts` | Phases 1-2 |
| **Phase 4** | Browse & Product — Home, Browse, Search, Categories, PDP, Recently Viewed, Notify Me | Phase 3 |
| **Phase 5** | Cart & Checkout — Cart store, 4-step checkout flow, payment SDKs, discounts | Phase 4 |
| **Phase 6** | Account & Features — Auth screens, Orders, Addresses, Wishlist, Loyalty, Reviews, Push | Phase 5 |

---

## Phase 1: Mobile Shared Package

Creates `packages/mobile-shared/` — the shared foundation consumed by both admin and storefront mobile apps.

### Task 1.1: Package scaffold

**Files:**
- Create: `packages/mobile-shared/package.json`
- Create: `packages/mobile-shared/tsconfig.json`
- Create: `packages/mobile-shared/src/index.ts`

- [ ] **Step 1: Create package.json**

```json
{
  "name": "@mark8ly/mobile-shared",
  "version": "0.0.1",
  "private": true,
  "main": "src/index.ts",
  "types": "src/index.ts",
  "scripts": {
    "test": "vitest run",
    "test:watch": "vitest",
    "check-types": "tsc --noEmit",
    "lint": "eslint src/"
  },
  "dependencies": {
    "@tanstack/react-query": "^5.83.0",
    "zustand": "^5.0.0",
    "expo-secure-store": "~14.0.0",
    "expo-haptics": "~14.0.0",
    "expo-notifications": "~0.31.0",
    "zod": "^3.25.0"
  },
  "devDependencies": {
    "typescript": "^5.9.2",
    "vitest": "^3.3.0",
    "@types/react": "^19.0.0",
    "react": "^19.2.0",
    "react-native": "^0.76.9"
  },
  "peerDependencies": {
    "react": ">=18.0.0",
    "react-native": ">=0.76.0"
  }
}
```

- [ ] **Step 2: Create tsconfig.json**

```json
{
  "extends": "@repo/typescript-config/base.json",
  "compilerOptions": {
    "baseUrl": ".",
    "paths": { "@/*": ["src/*"] },
    "jsx": "react-jsx",
    "types": ["react-native"]
  },
  "include": ["src/**/*"],
  "exclude": ["node_modules", "dist"]
}
```

- [ ] **Step 3: Create src/index.ts** (barrel export — initially empty)

```typescript
// @mark8ly/mobile-shared — shared logic for mobile apps
export {};
```

- [ ] **Step 4: Install dependencies**

Run: `cd /Users/Mahesh.Sangawar/personal/tesserix-new/mark8ly && npm install`
Expected: Success, `packages/mobile-shared` linked in workspace

- [ ] **Step 5: Verify type-check**

Run: `cd packages/mobile-shared && npx tsc --noEmit`
Expected: No errors

- [ ] **Step 6: Commit**

```bash
git add packages/mobile-shared/
git commit -m "feat(mobile-shared): scaffold shared mobile package"
```

### Task 1.2: Base API client

**Files:**
- Create: `packages/mobile-shared/src/api/client.ts`
- Create: `packages/mobile-shared/src/api/types.ts`
- Create: `packages/mobile-shared/src/api/__tests__/client.test.ts`

- [ ] **Step 1: Write failing test for API client**

```typescript
// src/api/__tests__/client.test.ts
import { describe, it, expect, vi, beforeEach } from "vitest";
import { createApiClient, ApiError } from "../client";

describe("createApiClient", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it("sends GET request with base URL and store slug", async () => {
    const mockFetch = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ data: [] }),
    });
    vi.stubGlobal("fetch", mockFetch);

    const client = createApiClient({
      baseUrl: "https://api.example.com",
      storeSlug: "acme",
    });
    await client.get("/products");

    expect(mockFetch).toHaveBeenCalledWith(
      "https://api.example.com/api/v1/mobile/storefront/stores/acme/products",
      expect.objectContaining({ method: "GET" })
    );
  });

  it("attaches Authorization header when token is provided", async () => {
    const mockFetch = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ data: {} }),
    });
    vi.stubGlobal("fetch", mockFetch);

    const client = createApiClient({
      baseUrl: "https://api.example.com",
      storeSlug: "acme",
      getToken: async () => "test-token",
    });
    await client.get("/orders");

    const [, options] = mockFetch.mock.calls[0];
    expect(options.headers.Authorization).toBe("Bearer test-token");
  });

  it("throws ApiError on non-ok response", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue({
      ok: false,
      status: 404,
      json: () => Promise.resolve({ error: "not_found", message: "Not found" }),
    }));

    const client = createApiClient({
      baseUrl: "https://api.example.com",
      storeSlug: "acme",
    });

    await expect(client.get("/products/missing")).rejects.toThrow(ApiError);
  });
});
```

- [ ] **Step 2: Run test — should fail**

Run: `cd packages/mobile-shared && npx vitest run src/api/__tests__/client.test.ts`
Expected: FAIL — module not found

- [ ] **Step 3: Create API types**

```typescript
// src/api/types.ts
export interface ApiClientConfig {
  baseUrl: string;
  storeSlug: string;
  getToken?: () => Promise<string | null>;
}

export interface ApiResponse<T> {
  data: T;
  message?: string;
  total?: number;
  page?: number;
  limit?: number;
}

export interface ApiErrorBody {
  error: string;
  message: string;
}

export interface ProductListItem {
  id: string;
  handle: string;
  title: string;
  description: string;
  price_amount: string;
  compare_at_price: string;
  currency_code: string;
  status: string;
  stock_status: string;
  stock_quantity: number;
  images: ProductImage[];
  category_name: string;
  average_rating: number;
  review_count: number;
}

export interface ProductImage {
  id: string;
  url: string;
  alt: string;
  position: number;
}

export interface ProductDetail extends ProductListItem {
  variants: ProductVariant[];
  options: ProductOption[];
}

export interface ProductVariant {
  id: string;
  sku: string;
  price_amount: string;
  compare_at_price: string;
  stock_quantity: number;
  stock_status: string;
  option_values: Record<string, string>;
}

export interface ProductOption {
  name: string;
  values: string[];
}

export interface Category {
  id: string;
  name: string;
  slug: string;
  image_url: string;
  product_count: number;
}

export interface OrderSummary {
  id: string;
  order_number: string;
  status: string;
  total_amount: string;
  currency_code: string;
  item_count: number;
  created_at: string;
}

export interface OrderDetail extends OrderSummary {
  customer_email: string;
  customer_name: string;
  line_items: OrderLineItem[];
  shipping_address: Address;
  shipping_method: string;
  shipping_cost: string;
  tracking_number: string;
  payment_method: string;
  payment_amount: string;
  subtotal: string;
  discount_amount: string;
  tax_amount: string;
  timeline: OrderEvent[];
}

export interface OrderLineItem {
  product_id: string;
  variant_id: string;
  title: string;
  variant_title: string;
  sku: string;
  quantity: number;
  unit_price: string;
  line_total: string;
  image_url: string;
}

export interface OrderEvent {
  type: string;
  description: string;
  created_at: string;
}

export interface Address {
  id: string;
  name: string;
  line1: string;
  line2: string;
  city: string;
  region: string;
  postal_code: string;
  country: string;
  is_default: boolean;
}

export interface CheckoutLineItem {
  product_id: string;
  variant_id: string;
  quantity: number;
}

export interface ShippingRate {
  id: string;
  carrier: string;
  service: string;
  estimated_days: string;
  price_amount: string;
  currency_code: string;
}

export interface CheckoutSubmitBody {
  email: string;
  customer_name: string;
  line_items: CheckoutLineItem[];
  shipping_address: Omit<Address, "id" | "is_default">;
  shipping_rate_id: string;
  payment_provider: string;
  coupon_code?: string;
  gift_card_code?: string;
  loyalty_points?: number;
  idempotency_key: string;
  save_address?: boolean;
}

export interface CheckoutResult {
  order_id: string;
  order_number: string;
  payment_token: string;
  payment_provider: string;
  totals: {
    subtotal: string;
    shipping: string;
    discount: string;
    tax: string;
    total: string;
    currency_code: string;
  };
}

export interface PaymentMethod {
  provider: string;
  enabled: boolean;
  supports_apple_pay: boolean;
  supports_google_pay: boolean;
}

export interface CustomerProfile {
  id: string;
  email: string;
  first_name: string;
  last_name: string;
  phone: string;
  created_at: string;
}

export interface WishlistItem {
  product_id: string;
  handle: string;
  title: string;
  price_amount: string;
  currency_code: string;
  image_url: string;
  stock_status: string;
  added_at: string;
}

export interface LoyaltyProgram {
  name: string;
  enabled: boolean;
  points_per_currency: number;
  currency_per_point: number;
  tiers: LoyaltyTier[];
  signup_bonus: number;
  referral_bonus: number;
}

export interface LoyaltyTier {
  name: string;
  min_points: number;
  multiplier: number;
}

export interface LoyaltyMe {
  points_balance: number;
  lifetime_points: number;
  current_tier: string;
  next_tier: string;
  points_to_next_tier: number;
  referral_code: string;
}

export interface ReviewItem {
  id: string;
  product_id: string;
  product_handle: string;
  product_title: string;
  product_image_url: string;
  rating: number;
  title: string;
  body: string;
  created_at: string;
}

export interface StoreBranding {
  store_name: string;
  logo_url: string;
  primary_color: string;
  accent_color: string;
  background_color: string;
  font_family: string;
  banner_url: string;
  banner_title: string;
}
```

- [ ] **Step 4: Create API client**

```typescript
// src/api/client.ts
import type { ApiClientConfig, ApiErrorBody } from "./types";

export class ApiError extends Error {
  constructor(
    public readonly status: number,
    public readonly code: string,
    message: string
  ) {
    super(message);
    this.name = "ApiError";
  }
}

export function createApiClient(config: ApiClientConfig) {
  const { baseUrl, storeSlug, getToken } = config;
  const prefix = `${baseUrl}/api/v1/mobile/storefront/stores/${storeSlug}`;

  async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
    const headers: Record<string, string> = {
      "Content-Type": "application/json",
    };

    if (getToken) {
      const token = await getToken();
      if (token) {
        headers.Authorization = `Bearer ${token}`;
      }
    }

    const res = await fetch(`${prefix}${path}`, {
      method,
      headers,
      body: body ? JSON.stringify(body) : undefined,
    });

    if (!res.ok) {
      let errorBody: ApiErrorBody;
      try {
        errorBody = await res.json();
      } catch {
        errorBody = { error: "unknown", message: res.statusText };
      }
      throw new ApiError(res.status, errorBody.error, errorBody.message);
    }

    return res.json();
  }

  return {
    get: <T>(path: string) => request<T>("GET", path),
    post: <T>(path: string, body?: unknown) => request<T>("POST", path, body),
    put: <T>(path: string, body?: unknown) => request<T>("PUT", path, body),
    patch: <T>(path: string, body?: unknown) => request<T>("PATCH", path, body),
    delete: <T>(path: string) => request<T>("DELETE", path),
    head: async (path: string) => {
      const headers: Record<string, string> = {};
      if (getToken) {
        const token = await getToken();
        if (token) headers.Authorization = `Bearer ${token}`;
      }
      const res = await fetch(`${prefix}${path}`, { method: "HEAD", headers });
      return { ok: res.ok, status: res.status };
    },
  };
}

export type ApiClient = ReturnType<typeof createApiClient>;
```

- [ ] **Step 5: Run tests — should pass**

Run: `cd packages/mobile-shared && npx vitest run src/api/__tests__/client.test.ts`
Expected: PASS (3 tests)

- [ ] **Step 6: Commit**

```bash
git add packages/mobile-shared/src/api/
git commit -m "feat(mobile-shared): add API client and shared types"
```

### Task 1.3: Auth store and token storage

**Files:**
- Create: `packages/mobile-shared/src/auth/token-storage.ts`
- Create: `packages/mobile-shared/src/auth/provider.tsx`
- Create: `packages/mobile-shared/src/stores/auth-store.ts`
- Create: `packages/mobile-shared/src/stores/__tests__/auth-store.test.ts`

- [ ] **Step 1: Write failing test for auth store**

```typescript
// src/stores/__tests__/auth-store.test.ts
import { describe, it, expect, beforeEach } from "vitest";
import { useAuthStore } from "../auth-store";

describe("authStore", () => {
  beforeEach(() => {
    useAuthStore.getState().reset();
  });

  it("starts unauthenticated", () => {
    const state = useAuthStore.getState();
    expect(state.isAuthenticated).toBe(false);
    expect(state.user).toBeNull();
  });

  it("sets authenticated state with user", () => {
    useAuthStore.getState().setUser({
      uid: "u1",
      email: "test@example.com",
      displayName: "Test User",
    });
    const state = useAuthStore.getState();
    expect(state.isAuthenticated).toBe(true);
    expect(state.user?.email).toBe("test@example.com");
  });

  it("resets to unauthenticated", () => {
    useAuthStore.getState().setUser({
      uid: "u1",
      email: "test@example.com",
      displayName: "Test User",
    });
    useAuthStore.getState().reset();
    expect(useAuthStore.getState().isAuthenticated).toBe(false);
  });
});
```

- [ ] **Step 2: Run test — should fail**

Run: `cd packages/mobile-shared && npx vitest run src/stores/__tests__/auth-store.test.ts`
Expected: FAIL

- [ ] **Step 3: Create auth store**

```typescript
// src/stores/auth-store.ts
import { create } from "zustand";

export interface AuthUser {
  uid: string;
  email: string;
  displayName: string;
}

interface AuthState {
  isAuthenticated: boolean;
  user: AuthUser | null;
  isLoading: boolean;
  setUser: (user: AuthUser) => void;
  setLoading: (loading: boolean) => void;
  reset: () => void;
}

export const useAuthStore = create<AuthState>((set) => ({
  isAuthenticated: false,
  user: null,
  isLoading: true,
  setUser: (user) => set({ isAuthenticated: true, user, isLoading: false }),
  setLoading: (isLoading) => set({ isLoading }),
  reset: () => set({ isAuthenticated: false, user: null, isLoading: false }),
}));
```

- [ ] **Step 4: Create token storage wrapper**

```typescript
// src/auth/token-storage.ts
import * as SecureStore from "expo-secure-store";

const TOKEN_KEY = "mark8ly_refresh_token";
const USER_KEY = "mark8ly_user";

export const tokenStorage = {
  async saveToken(token: string): Promise<void> {
    await SecureStore.setItemAsync(TOKEN_KEY, token);
  },
  async getToken(): Promise<string | null> {
    return SecureStore.getItemAsync(TOKEN_KEY);
  },
  async deleteToken(): Promise<void> {
    await SecureStore.deleteItemAsync(TOKEN_KEY);
  },
  async saveUser(user: { uid: string; email: string; displayName: string }): Promise<void> {
    await SecureStore.setItemAsync(USER_KEY, JSON.stringify(user));
  },
  async getUser(): Promise<{ uid: string; email: string; displayName: string } | null> {
    const raw = await SecureStore.getItemAsync(USER_KEY);
    return raw ? JSON.parse(raw) : null;
  },
  async clear(): Promise<void> {
    await SecureStore.deleteItemAsync(TOKEN_KEY);
    await SecureStore.deleteItemAsync(USER_KEY);
  },
};
```

- [ ] **Step 5: Create AuthProvider**

```typescript
// src/auth/provider.tsx
import React, { createContext, useContext, useEffect, type ReactNode } from "react";
import { useAuthStore, type AuthUser } from "../stores/auth-store";
import { tokenStorage } from "./token-storage";

interface AuthContextValue {
  isAuthenticated: boolean;
  user: AuthUser | null;
  isLoading: boolean;
  signIn: (user: AuthUser) => Promise<void>;
  signOut: () => Promise<void>;
}

const AuthContext = createContext<AuthContextValue | null>(null);

interface AuthProviderProps {
  children: ReactNode;
  onSignOut?: () => Promise<void>;
}

export function AuthProvider({ children, onSignOut }: AuthProviderProps) {
  const { isAuthenticated, user, isLoading, setUser, setLoading, reset } = useAuthStore();

  useEffect(() => {
    async function hydrate() {
      try {
        const savedUser = await tokenStorage.getUser();
        if (savedUser) {
          setUser(savedUser);
        } else {
          setLoading(false);
        }
      } catch {
        setLoading(false);
      }
    }
    hydrate();
  }, [setUser, setLoading]);

  const signIn = async (authUser: AuthUser) => {
    await tokenStorage.saveUser(authUser);
    setUser(authUser);
  };

  const signOut = async () => {
    await tokenStorage.clear();
    if (onSignOut) await onSignOut();
    reset();
  };

  return (
    <AuthContext.Provider value={{ isAuthenticated, user, isLoading, signIn, signOut }}>
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth() {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used within AuthProvider");
  return ctx;
}
```

- [ ] **Step 6: Run tests — should pass**

Run: `cd packages/mobile-shared && npx vitest run`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add packages/mobile-shared/src/auth/ packages/mobile-shared/src/stores/
git commit -m "feat(mobile-shared): add auth store, token storage, and AuthProvider"
```

### Task 1.4: Deep link validator

**Files:**
- Create: `packages/mobile-shared/src/deep-links/validator.ts`
- Create: `packages/mobile-shared/src/deep-links/__tests__/validator.test.ts`

- [ ] **Step 1: Write failing test**

```typescript
// src/deep-links/__tests__/validator.test.ts
import { describe, it, expect } from "vitest";
import { validateDeepLink, extractDeepLinkRoute } from "../validator";

describe("validateDeepLink", () => {
  it("accepts valid order detail path", () => {
    expect(validateDeepLink("account/orders/550e8400-e29b-41d4-a716-446655440000")).toBe(true);
  });

  it("accepts valid product path", () => {
    expect(validateDeepLink("browse/product/blue-widget-v2")).toBe(true);
  });

  it("accepts valid loyalty path", () => {
    expect(validateDeepLink("account/loyalty")).toBe(true);
  });

  it("rejects path traversal attempts", () => {
    expect(validateDeepLink("../../../login")).toBe(false);
  });

  it("rejects unknown routes", () => {
    expect(validateDeepLink("admin/settings")).toBe(false);
  });

  it("rejects malformed UUIDs", () => {
    expect(validateDeepLink("account/orders/not-a-uuid")).toBe(false);
  });

  it("rejects empty path", () => {
    expect(validateDeepLink("")).toBe(false);
  });
});

describe("extractDeepLinkRoute", () => {
  it("returns validated route for valid path", () => {
    const route = extractDeepLinkRoute("account/orders/550e8400-e29b-41d4-a716-446655440000");
    expect(route).toEqual({
      screen: "account/orders/[id]",
      params: { id: "550e8400-e29b-41d4-a716-446655440000" },
    });
  });

  it("returns null for invalid path", () => {
    expect(extractDeepLinkRoute("../hack")).toBeNull();
  });
});
```

- [ ] **Step 2: Run test — should fail**

Run: `cd packages/mobile-shared && npx vitest run src/deep-links/__tests__/validator.test.ts`
Expected: FAIL

- [ ] **Step 3: Implement validator**

```typescript
// src/deep-links/validator.ts
const UUID_PATTERN = "[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}";
const HANDLE_PATTERN = "[a-z0-9][a-z0-9-]*[a-z0-9]";

const ALLOWED_ROUTES: Array<{
  pattern: RegExp;
  screen: string;
  paramExtractor?: (match: RegExpMatchArray) => Record<string, string>;
}> = [
  {
    pattern: new RegExp(`^account/orders/(${UUID_PATTERN})$`),
    screen: "account/orders/[id]",
    paramExtractor: (m) => ({ id: m[1] }),
  },
  {
    pattern: /^account\/loyalty$/,
    screen: "account/loyalty",
  },
  {
    pattern: /^account\/wishlist$/,
    screen: "account/wishlist",
  },
  {
    pattern: /^account\/reviews$/,
    screen: "account/reviews",
  },
  {
    pattern: new RegExp(`^browse/product/(${HANDLE_PATTERN})$`),
    screen: "browse/product/[handle]",
    paramExtractor: (m) => ({ handle: m[1] }),
  },
  {
    pattern: new RegExp(`^browse/category/(${HANDLE_PATTERN})$`),
    screen: "browse/category/[slug]",
    paramExtractor: (m) => ({ slug: m[1] }),
  },
];

export function validateDeepLink(path: string): boolean {
  if (!path || path.includes("..")) return false;
  return ALLOWED_ROUTES.some((route) => route.pattern.test(path));
}

interface DeepLinkRoute {
  screen: string;
  params: Record<string, string>;
}

export function extractDeepLinkRoute(path: string): DeepLinkRoute | null {
  if (!path || path.includes("..")) return null;

  for (const route of ALLOWED_ROUTES) {
    const match = path.match(route.pattern);
    if (match) {
      return {
        screen: route.screen,
        params: route.paramExtractor ? route.paramExtractor(match) : {},
      };
    }
  }
  return null;
}
```

- [ ] **Step 4: Run tests — should pass**

Run: `cd packages/mobile-shared && npx vitest run src/deep-links/__tests__/validator.test.ts`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add packages/mobile-shared/src/deep-links/
git commit -m "feat(mobile-shared): add deep link validator with route allowlist"
```

### Task 1.5: Haptic feedback wrappers

**Files:**
- Create: `packages/mobile-shared/src/haptics/feedback.ts`
- Create: `packages/mobile-shared/src/haptics/__tests__/feedback.test.ts`

- [ ] **Step 1: Write failing test**

```typescript
// src/haptics/__tests__/feedback.test.ts
import { describe, it, expect, vi } from "vitest";

// Mock expo-haptics before import
vi.mock("expo-haptics", () => ({
  impactAsync: vi.fn(),
  notificationAsync: vi.fn(),
  ImpactFeedbackStyle: { Light: "light", Medium: "medium", Heavy: "heavy" },
  NotificationFeedbackType: { Success: "success", Error: "error", Warning: "warning" },
}));

import { haptics } from "../feedback";
import * as Haptics from "expo-haptics";

describe("haptics", () => {
  it("fires light impact for addToCart", async () => {
    await haptics.addToCart();
    expect(Haptics.impactAsync).toHaveBeenCalledWith(Haptics.ImpactFeedbackStyle.Light);
  });

  it("fires medium impact for removeFromCart", async () => {
    await haptics.removeFromCart();
    expect(Haptics.impactAsync).toHaveBeenCalledWith(Haptics.ImpactFeedbackStyle.Medium);
  });

  it("fires success notification for orderPlaced", async () => {
    await haptics.orderPlaced();
    expect(Haptics.notificationAsync).toHaveBeenCalledWith(Haptics.NotificationFeedbackType.Success);
  });

  it("fires error notification for paymentFailed", async () => {
    await haptics.paymentFailed();
    expect(Haptics.notificationAsync).toHaveBeenCalledWith(Haptics.NotificationFeedbackType.Error);
  });
});
```

- [ ] **Step 2: Run test — should fail**

- [ ] **Step 3: Implement haptics**

```typescript
// src/haptics/feedback.ts
import * as Haptics from "expo-haptics";

export const haptics = {
  addToCart: () => Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Light),
  removeFromCart: () => Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Medium),
  quantityChange: () => Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Light),
  wishlistToggle: () => Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Light),
  swipeDelete: () => Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Medium),
  orderPlaced: () => Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success),
  paymentFailed: () => Haptics.notificationAsync(Haptics.NotificationFeedbackType.Error),
  pullToRefresh: () => Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Light),
  checkoutStep: () => Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Light),
} as const;
```

- [ ] **Step 4: Run tests — should pass**

- [ ] **Step 5: Commit**

```bash
git add packages/mobile-shared/src/haptics/
git commit -m "feat(mobile-shared): add haptic feedback wrappers"
```

### Task 1.6: Push token registration

**Files:**
- Create: `packages/mobile-shared/src/push/registration.ts`
- Create: `packages/mobile-shared/src/push/__tests__/registration.test.ts`

- [ ] **Step 1: Write failing test**

```typescript
// src/push/__tests__/registration.test.ts
import { describe, it, expect, vi, beforeEach } from "vitest";

vi.mock("expo-notifications", () => ({
  getPermissionsAsync: vi.fn(),
  requestPermissionsAsync: vi.fn(),
  getExpoPushTokenAsync: vi.fn(),
}));

vi.mock("expo-device", () => ({
  isDevice: true,
  modelId: "test-device-123",
}));

import { registerPushToken } from "../registration";
import * as Notifications from "expo-notifications";

describe("registerPushToken", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it("requests permissions and returns token on success", async () => {
    vi.mocked(Notifications.getPermissionsAsync).mockResolvedValue({
      status: "undetermined",
      expires: "never",
      granted: false,
      canAskAgain: true,
    } as any);
    vi.mocked(Notifications.requestPermissionsAsync).mockResolvedValue({
      status: "granted",
      expires: "never",
      granted: true,
      canAskAgain: true,
    } as any);
    vi.mocked(Notifications.getExpoPushTokenAsync).mockResolvedValue({
      data: "ExponentPushToken[test123]",
      type: "expo",
    });

    const result = await registerPushToken();
    expect(result).toBe("ExponentPushToken[test123]");
  });

  it("returns null when permission denied", async () => {
    vi.mocked(Notifications.getPermissionsAsync).mockResolvedValue({
      status: "denied",
      expires: "never",
      granted: false,
      canAskAgain: false,
    } as any);

    const result = await registerPushToken();
    expect(result).toBeNull();
  });
});
```

- [ ] **Step 2: Run test — should fail**

- [ ] **Step 3: Implement registration**

```typescript
// src/push/registration.ts
import * as Notifications from "expo-notifications";
import * as Device from "expo-device";
import { Platform } from "react-native";

export async function registerPushToken(): Promise<string | null> {
  if (!Device.isDevice) return null;

  const { status: existing } = await Notifications.getPermissionsAsync();
  let finalStatus = existing;

  if (existing !== "granted") {
    const { status } = await Notifications.requestPermissionsAsync();
    finalStatus = status;
  }

  if (finalStatus !== "granted") return null;

  const { data: token } = await Notifications.getExpoPushTokenAsync();
  return token;
}

export function getDeviceId(): string {
  return Device.modelId ?? `${Platform.OS}-unknown`;
}

export function getPlatform(): "ios" | "android" {
  return Platform.OS === "ios" ? "ios" : "android";
}
```

- [ ] **Step 4: Run tests — should pass**

- [ ] **Step 5: Commit**

```bash
git add packages/mobile-shared/src/push/
git commit -m "feat(mobile-shared): add push token registration"
```

### Task 1.7: Export barrel and final verification

**Files:**
- Modify: `packages/mobile-shared/src/index.ts`

- [ ] **Step 1: Update barrel export**

```typescript
// src/index.ts
export { createApiClient, ApiError, type ApiClient } from "./api/client";
export * from "./api/types";
export { useAuthStore, type AuthUser } from "./stores/auth-store";
export { AuthProvider, useAuth } from "./auth/provider";
export { tokenStorage } from "./auth/token-storage";
export { validateDeepLink, extractDeepLinkRoute } from "./deep-links/validator";
export { haptics } from "./haptics/feedback";
export { registerPushToken, getDeviceId, getPlatform } from "./push/registration";
```

- [ ] **Step 2: Run all tests**

Run: `cd packages/mobile-shared && npx vitest run`
Expected: All tests PASS

- [ ] **Step 3: Type-check**

Run: `cd packages/mobile-shared && npx tsc --noEmit`
Expected: No errors

- [ ] **Step 4: Commit**

```bash
git add packages/mobile-shared/src/index.ts
git commit -m "feat(mobile-shared): finalize barrel exports"
```

---

## Phase 2: Backend — Mobile Storefront Route Group

Adds `/api/v1/mobile/storefront/` routes on marketplace-api with GIPBearerAuth, push token table, and notify-me table.

### Task 2.1: GIP Bearer auth middleware for mobile

**Files:**
- Create: `services/marketplace-api/internal/handlers/storefront/mobile_auth.go`
- Create: `services/marketplace-api/internal/handlers/storefront/mobile_auth_test.go`

- [ ] **Step 1: Write failing test**

```go
// internal/handlers/storefront/mobile_auth_test.go
package storefront

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestGIPBearerAuth_MissingToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/test", GIPBearerAuth("mp-customer", true), func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, 401, w.Code)
}

func TestGIPBearerAuth_DevMode_SkipsValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	// devMode=true skips actual Firebase token verification
	r.GET("/test", GIPBearerAuth("mp-customer", true), func(c *gin.Context) {
		c.JSON(200, gin.H{"uid": c.GetString("customer_gip_uid")})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer dev-token")
	r.ServeHTTP(w, req)
	// In dev mode with a non-empty token, should pass through
	assert.Equal(t, 200, w.Code)
}
```

- [ ] **Step 2: Run test — should fail**

Run: `cd services/marketplace-api && go test ./internal/handlers/storefront/ -run TestGIPBearerAuth -v`
Expected: FAIL — function not defined

- [ ] **Step 3: Implement GIP Bearer auth middleware**

```go
// internal/handlers/storefront/mobile_auth.go
package storefront

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// GIPBearerAuth validates Firebase/GIP Bearer tokens for mobile clients.
// In devMode, it accepts any non-empty Bearer token and extracts a dev UID.
// In production, it verifies the token against Google's public keys and
// checks the tenant pool matches.
//
// Sets context keys: customer_gip_uid, customer_email
func GIPBearerAuth(tenantPool string, devMode bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error":   "unauthorized",
				"message": "Missing or invalid Authorization header",
			})
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error":   "unauthorized",
				"message": "Empty bearer token",
			})
			return
		}

		if devMode {
			// Dev mode: accept any token, set a dev UID.
			c.Set("customer_gip_uid", "dev-uid-"+token[:min(8, len(token))])
			c.Set("customer_email", "dev@example.com")
			c.Next()
			return
		}

		// Production: verify token with Firebase Admin SDK.
		// This requires firebase.google.com/go/v4 and a GIP project config.
		// Implementation wired in main.go with a FirebaseApp instance.
		// For now, delegate to a verifier function set on the middleware.
		verifier, exists := c.Get("gip_token_verifier")
		if !exists {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error":   "internal",
				"message": "Token verifier not configured",
			})
			return
		}

		verify, ok := verifier.(func(string, string) (string, string, error))
		if !ok {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error":   "internal",
				"message": "Invalid token verifier",
			})
			return
		}

		uid, email, err := verify(token, tenantPool)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error":   "unauthorized",
				"message": "Invalid or expired token",
			})
			return
		}

		c.Set("customer_gip_uid", uid)
		c.Set("customer_email", email)
		c.Next()
	}
}

// Note: Go 1.26 has built-in min() — no custom function needed.
```

- [ ] **Step 4: Run tests — should pass**

Run: `cd services/marketplace-api && go test ./internal/handlers/storefront/ -run TestGIPBearerAuth -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/handlers/storefront/mobile_auth.go
git add services/marketplace-api/internal/handlers/storefront/mobile_auth_test.go
git commit -m "feat(marketplace-api): add GIP Bearer auth middleware for mobile storefront"
```

### Task 2.2: Push token and notify-me migrations

**Files:**
- Create: `services/marketplace-api/migrations/000021_storefront_push_tokens.up.sql`
- Create: `services/marketplace-api/migrations/000021_storefront_push_tokens.down.sql`
- Create: `services/marketplace-api/migrations/000022_product_notify_subscriptions.up.sql`
- Create: `services/marketplace-api/migrations/000022_product_notify_subscriptions.down.sql`
- Modify: `services/marketplace-api/migrations.go:17` — bump ExpectedSchemaVersion to 22

> **Note:** Migrations 000020 (branding) already exists. These are 000021 and 000022. Verify the current state of migrations before creating — if 000020 is not yet applied, the ExpectedSchemaVersion bump needs to account for that.

- [ ] **Step 1: Create push token migration (up)**

```sql
-- 000021_storefront_push_tokens.up.sql
CREATE TABLE IF NOT EXISTS storefront_push_tokens (
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
CREATE INDEX idx_storefront_push_tokens_customer ON storefront_push_tokens (customer_id);
```

- [ ] **Step 2: Create push token migration (down)**

```sql
-- 000021_storefront_push_tokens.down.sql
DROP TABLE IF EXISTS storefront_push_tokens;
```

- [ ] **Step 3: Create notify-me migration (up)**

```sql
-- 000022_product_notify_subscriptions.up.sql
CREATE TABLE IF NOT EXISTS product_notify_subscriptions (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID        NOT NULL,
    store_slug  VARCHAR(63) NOT NULL,
    customer_id UUID        NOT NULL,
    product_id  UUID        NOT NULL,
    notify_type VARCHAR(20) NOT NULL CHECK (notify_type IN ('back_in_stock', 'price_drop')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (customer_id, product_id, notify_type)
);

CREATE INDEX idx_product_notify_subs_product ON product_notify_subscriptions (product_id);
CREATE INDEX idx_product_notify_subs_store ON product_notify_subscriptions (store_slug);
```

- [ ] **Step 4: Create notify-me migration (down)**

```sql
-- 000022_product_notify_subscriptions.down.sql
DROP TABLE IF EXISTS product_notify_subscriptions;
```

- [ ] **Step 5: Bump ExpectedSchemaVersion**

Edit `services/marketplace-api/migrations.go:17`:
```go
const ExpectedSchemaVersion uint = 22
```

- [ ] **Step 6: Verify migrations compile**

Run: `cd services/marketplace-api && go build ./...`
Expected: Success

- [ ] **Step 7: Commit**

```bash
git add services/marketplace-api/migrations/000021_* services/marketplace-api/migrations/000022_*
git add services/marketplace-api/migrations.go
git commit -m "feat(marketplace-api): add storefront push token and notify-me migrations"
```

### Task 2.3: Push token handler and repository

**Files:**
- Create: `services/marketplace-api/internal/pushtoken/models.go`
- Create: `services/marketplace-api/internal/pushtoken/repository.go`
- Create: `services/marketplace-api/internal/handlers/storefront/push_tokens.go`

- [ ] **Step 1: Create push token models**

```go
// internal/pushtoken/models.go
package pushtoken

import (
	"time"

	"github.com/google/uuid"
)

type StorefrontPushToken struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID   uuid.UUID `gorm:"type:uuid;not null"`
	StoreSlug  string    `gorm:"type:varchar(63);not null"`
	CustomerID uuid.UUID `gorm:"type:uuid;not null"`
	DeviceID   string    `gorm:"type:varchar(255);not null"`
	Token      string    `gorm:"type:text;not null"`
	Platform   string    `gorm:"type:varchar(10);not null"`
	CreatedAt  time.Time `gorm:"not null;default:now()"`
	UpdatedAt  time.Time `gorm:"not null;default:now()"`
}

func (StorefrontPushToken) TableName() string {
	return "storefront_push_tokens"
}

type RegisterTokenInput struct {
	Token    string `json:"token" binding:"required"`
	DeviceID string `json:"device_id" binding:"required"`
	Platform string `json:"platform" binding:"required,oneof=ios android"`
}
```

- [ ] **Step 2: Create push token repository**

```go
// internal/pushtoken/repository.go
package pushtoken

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Upsert(ctx context.Context, token StorefrontPushToken) error {
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "customer_id"}, {Name: "device_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"token", "platform", "updated_at"}),
		}).
		Create(&token).Error
}

func (r *Repository) DeleteByID(ctx context.Context, id uuid.UUID, customerID uuid.UUID) error {
	return r.db.WithContext(ctx).
		Where("id = ? AND customer_id = ?", id, customerID).
		Delete(&StorefrontPushToken{}).Error
}

func (r *Repository) FindByCustomer(ctx context.Context, customerID uuid.UUID) ([]StorefrontPushToken, error) {
	var tokens []StorefrontPushToken
	err := r.db.WithContext(ctx).
		Where("customer_id = ?", customerID).
		Find(&tokens).Error
	return tokens, err
}

func (r *Repository) FindByStore(ctx context.Context, storeSlug string) ([]StorefrontPushToken, error) {
	var tokens []StorefrontPushToken
	err := r.db.WithContext(ctx).
		Where("store_slug = ?", storeSlug).
		Find(&tokens).Error
	return tokens, err
}
```

- [ ] **Step 3: Create push token handler**

```go
// internal/handlers/storefront/push_tokens.go
package storefront

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/mark8ly/marketplace-api/internal/pushtoken"
	"github.com/mark8ly/marketplace-api/internal/stores"
)

type PushTokenHandler struct {
	repo   *pushtoken.Repository
	logger *slog.Logger
}

func NewPushTokenHandler(repo *pushtoken.Repository, logger *slog.Logger) *PushTokenHandler {
	return &PushTokenHandler{repo: repo, logger: logger}
}

func (h *PushTokenHandler) Register(c *gin.Context) {
	var input pushtoken.RegisterTokenInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "bad_request",
			"message": "Invalid request body: " + err.Error(),
		})
		return
	}

	store := c.MustGet("store").(*stores.Store)
	customerID := c.GetString("customer_profile_id")
	if customerID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":   "unauthorized",
			"message": "Authentication required",
		})
		return
	}

	custUUID, err := uuid.Parse(customerID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request", "message": "Invalid customer ID"})
		return
	}
	tenantUUID, err := uuid.Parse(store.TenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal", "message": "Invalid tenant ID"})
		return
	}

	token := pushtoken.StorefrontPushToken{
		TenantID:   tenantUUID,
		StoreSlug:  store.Slug,
		CustomerID: custUUID,
		DeviceID:   input.DeviceID,
		Token:      input.Token,
		Platform:   input.Platform,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	if err := h.repo.Upsert(c.Request.Context(), token); err != nil {
		h.logger.Error("failed to register push token", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "internal",
			"message": "Failed to register push token",
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "Push token registered"})
}

func (h *PushTokenHandler) Delete(c *gin.Context) {
	tokenID, err := uuid.Parse(c.Param("tokenId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request", "message": "Invalid token ID"})
		return
	}

	customerID := c.GetString("customer_profile_id")
	custUUID, err := uuid.Parse(customerID)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized", "message": "Authentication required"})
		return
	}

	if err := h.repo.DeleteByID(c.Request.Context(), tokenID, custUUID); err != nil {
		h.logger.Error("failed to delete push token", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal", "message": "Failed to delete push token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Push token deleted"})
}
```

- [ ] **Step 4: Verify build**

Run: `cd services/marketplace-api && go build ./...`
Expected: Success

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/pushtoken/
git add services/marketplace-api/internal/handlers/storefront/push_tokens.go
git commit -m "feat(marketplace-api): add storefront push token handler and repository"
```

### Task 2.4: Mobile storefront route registration

**Files:**
- Modify: `services/marketplace-api/internal/handlers/storefront/routes.go` — add MobileDeps and RegisterMobileStorefront
- Modify: `services/marketplace-api/cmd/marketplace-api/main.go` — wire mobile deps and register routes

- [ ] **Step 1: Add MobileDeps and RegisterMobileStorefront to routes.go**

Add after the existing `RegisterStorefront` function (line 168):

```go
// MobileDeps groups dependencies for the mobile storefront route group.
// Mobile routes use GIP Bearer auth instead of cookie-based auth.
type MobileDeps struct {
	// Reuse existing handlers (read-only endpoints).
	Handler          *StorefrontHandler
	ShippingRatesHandler *ShippingRatesHandler
	PaymentMethodsHandler *PaymentMethodsHandler
	CheckoutExtHandler  *CheckoutExtHandler
	OrderDetailHandler  *OrderDetailHandler
	CouponValidateHandler *CouponValidateHandler
	GiftCardHandler     *GiftCardStorefrontHandler
	LoyaltyHandler      *LoyaltyHandler
	ReviewsHandler      *ReviewsHandler
	WishlistHandler     *WishlistHandler
	CustomerAccountHandler *CustomerAccountHandler
	PushTokenHandler    *PushTokenHandler
	SlugCache           *stores.SlugCache
	CustomerService     *customer.Service
	DevMode             bool
	Logger              *slog.Logger
}

// RegisterMobileStorefront mounts the mobile storefront routes.
// Chain: StoreContext → [GIPBearerAuth for authenticated routes].
// No X-Storefront-Key required (mobile apps can't keep it secret).
func RegisterMobileStorefront(router *gin.RouterGroup, deps MobileDeps) {
	storeMW := StoreContext(deps.SlugCache)
	bearerAuth := GIPBearerAuth("mp-customer", deps.DevMode)

	group := router.Group("/mobile/storefront/stores/:storeSlug", storeMW)

	// === Public routes (no auth) ===
	{
		group.GET("/products", deps.Handler.List)
		group.GET("/products/:handle", deps.Handler.GetByHandle)
		group.GET("/categories", deps.Handler.ListCategories)
		group.GET("/categories/:slug/products", deps.Handler.ListByCategorySlug)

		if deps.PaymentMethodsHandler != nil {
			group.GET("/payment-methods", deps.PaymentMethodsHandler.ListPaymentMethods)
		}
		if deps.ReviewsHandler != nil {
			group.GET("/products/:handle/reviews", deps.ReviewsHandler.ListProductReviews)
		}
		if deps.LoyaltyHandler != nil {
			group.GET("/loyalty/program", deps.LoyaltyHandler.GetProgram)
		}
	}

	// === Authenticated routes ===
	authed := group.Group("", bearerAuth)
	{
		// Checkout
		if deps.CheckoutExtHandler != nil {
			authed.POST("/checkout/shipping-rates", deps.ShippingRatesHandler.GetRates)
			authed.POST("/checkout/submit", deps.CheckoutExtHandler.Checkout)
		}

		// Orders
		if deps.OrderDetailHandler != nil {
			authed.GET("/orders", deps.OrderDetailHandler.ListOrders)
			authed.GET("/orders/:id", deps.OrderDetailHandler.GetOrder)
		}

		// Customer profile
		if deps.CustomerAccountHandler != nil {
			authed.GET("/customers/profile", deps.CustomerAccountHandler.GetProfile)
			authed.POST("/customers/register", deps.CustomerAccountHandler.Register)
			authed.GET("/addresses", deps.CustomerAccountHandler.ListAddresses)
			authed.POST("/addresses", deps.CustomerAccountHandler.CreateAddress)
			authed.PUT("/addresses/:id", deps.CustomerAccountHandler.UpdateAddress)
			authed.DELETE("/addresses/:id", deps.CustomerAccountHandler.DeleteAddress)
		}

		// Wishlist
		if deps.WishlistHandler != nil {
			authed.GET("/wishlist", deps.WishlistHandler.List)
			authed.POST("/wishlist", deps.WishlistHandler.Add)
			authed.DELETE("/wishlist/:productId", deps.WishlistHandler.Remove)
		}

		// Reviews
		if deps.ReviewsHandler != nil {
			authed.POST("/products/:handle/reviews",
				ratelimit.PerIP(0.167, 10),
				deps.ReviewsHandler.SubmitReview)
			authed.GET("/reviews/mine", deps.ReviewsHandler.ListMyReviews)
		}

		// Loyalty
		if deps.LoyaltyHandler != nil {
			authed.GET("/loyalty/me", deps.LoyaltyHandler.GetMe)
			authed.POST("/loyalty/enroll",
				ratelimit.PerIP(0.167, 10),
				deps.LoyaltyHandler.Enroll)
			authed.POST("/loyalty/redeem",
				ratelimit.PerIP(0.167, 10),
				deps.LoyaltyHandler.Redeem)
		}

		// Coupon validation
		if deps.CouponValidateHandler != nil {
			authed.POST("/coupons/validate",
				ratelimit.PerIP(0.167, 10),
				deps.CouponValidateHandler.Validate)
		}

		// Gift card balance
		if deps.GiftCardHandler != nil {
			authed.POST("/gift-cards/check-balance",
				ratelimit.PerIP(0.167, 10),
				deps.GiftCardHandler.CheckBalance)
		}

		// Push tokens
		if deps.PushTokenHandler != nil {
			authed.POST("/push-tokens",
				ratelimit.PerIP(0.083, 5), // 5 per hour
				deps.PushTokenHandler.Register)
			authed.DELETE("/push-tokens/:tokenId", deps.PushTokenHandler.Delete)
		}
	}
}
```

- [ ] **Step 2: Wire mobile deps in main.go**

After the existing `storefrontDeps` block (around line 444), add:

```go
		// Mobile storefront route group — GIP Bearer auth for mobile clients.
		pushTokenRepo := pushtoken.NewRepository(conn)
		pushTokenHandler := storefront.NewPushTokenHandler(pushTokenRepo, log)

		mobileSFDeps := storefront.MobileDeps{
			Handler:               storefrontHandler,
			ShippingRatesHandler:  shippingRatesHandler,
			PaymentMethodsHandler: paymentMethodsHandler,
			CheckoutExtHandler:    checkoutExtHandler,
			OrderDetailHandler:    orderDetailHandler,
			CouponValidateHandler: couponValidateHandler,
			GiftCardHandler:       giftCardSFHandler,
			LoyaltyHandler:        sfLoyaltyHandler,
			ReviewsHandler:        sfReviewsHandler,
			WishlistHandler:       wishlistHandler,
			CustomerAccountHandler: customerAccountHandler,
			PushTokenHandler:      pushTokenHandler,
			SlugCache:             slugCache,
			CustomerService:       customerSvc,
			DevMode:               cfg.Env == "development",
			Logger:                log,
		}
```

- [ ] **Step 3: Register mobile routes in engine setup**

In the `mode.Both` case (line ~569), add after `storefront.RegisterStorefront(...)`:
```go
		storefront.RegisterMobileStorefront(r.Group("/api/v1"), mobileSFDeps)
```

In the `mode.Storefront` case (line ~590), add after `storefront.RegisterStorefront(...)`:
```go
			storefront.RegisterMobileStorefront(engine.Group("/api/v1"), mobileSFDeps)
```

- [ ] **Step 4: Verify build**

Run: `cd services/marketplace-api && go build ./...`
Expected: Success

- [ ] **Step 5: Run existing tests**

Run: `cd services/marketplace-api && go test ./... -short`
Expected: All existing tests pass

- [ ] **Step 6: Commit**

```bash
git add services/marketplace-api/internal/handlers/storefront/routes.go
git add services/marketplace-api/cmd/marketplace-api/main.go
git commit -m "feat(marketplace-api): register mobile storefront route group with GIP Bearer auth"
```

### Task 2.5: Missing backend handlers

Several handler methods referenced in `RegisterMobileStorefront` (Task 2.4) don't exist yet on the existing handler structs. This task adds them.

**Files:**
- Create: `services/marketplace-api/internal/handlers/storefront/notify_me.go` — notify-me handler + repository
- Modify: `services/marketplace-api/internal/handlers/storefront/customer_account.go` — add `Register` method
- Modify: `services/marketplace-api/internal/handlers/storefront/order_detail.go` — add `ListOrders` method
- Modify: `services/marketplace-api/internal/handlers/storefront/reviews.go` — add `ListMyReviews` method
- Modify: `services/marketplace-api/internal/handlers/storefront/routes.go` — add `NotifyMeHandler` and `BrandingHandler` to `MobileDeps`, wire in `RegisterMobileStorefront`

- [ ] **Step 1: Create notify-me handler**

Create `internal/handlers/storefront/notify_me.go` with:
- `NotifyMeHandler` struct with `Subscribe(c *gin.Context)` and `Unsubscribe(c *gin.Context)` methods
- Internal `notifySubscription` model and repository (simple GORM CRUD against `product_notify_subscriptions` table)
- Subscribe: upsert by `(customer_id, product_id, notify_type)`, require auth
- Unsubscribe: delete by `(customer_id, product_id)`, require auth

- [ ] **Step 2: Add `Register` method to `CustomerAccountHandler`**

Add to `customer_account.go`:
```go
// Register creates a new customer profile after GIP registration (mobile flow).
// The GIP UID and email come from the verified Bearer token context.
func (h *CustomerAccountHandler) Register(c *gin.Context) {
    // Extract GIP UID and email from context (set by GIPBearerAuth middleware)
    gipUID := c.GetString("customer_gip_uid")
    email := c.GetString("customer_email")
    // Parse optional name from request body
    // Call h.customerSvc.EnsureProfile() — same as web cookie flow
    // Return the created profile
}
```

- [ ] **Step 3: Add `ListOrders` method to `OrderDetailHandler`**

Add to `order_detail.go`:
```go
// ListOrders returns paginated orders for the authenticated customer.
func (h *OrderDetailHandler) ListOrders(c *gin.Context) {
    customerID := c.GetString("customer_profile_id")
    // Parse pagination params (page, limit)
    // Query orders by customer_id
    // Return paginated list
}
```

- [ ] **Step 4: Add `ListMyReviews` method to `ReviewsHandler`**

Add to `reviews.go`:
```go
// ListMyReviews returns reviews submitted by the authenticated customer.
func (h *ReviewsHandler) ListMyReviews(c *gin.Context) {
    customerID := c.GetString("customer_profile_id")
    // Query reviews by customer_id with product join for title/image
    // Return list
}
```

- [ ] **Step 5: Add `CheckEmail` handler**

Add to `customer_account.go`:
```go
// CheckEmail returns 200 if an account exists for the given email, 404 otherwise.
// Used for mid-checkout account recognition nudge.
func (h *CustomerAccountHandler) CheckEmail(c *gin.Context) {
    email := c.Query("email")
    // Query customer_profiles by email + store_id
    // Return 200 if found, 404 if not
}
```

- [ ] **Step 6: Update `MobileDeps` and `RegisterMobileStorefront`**

Add to `MobileDeps`:
```go
    NotifyMeHandler *NotifyMeHandler
    BrandingHandler *BrandingHandler // reuse from admin package
```

Add to `RegisterMobileStorefront` public routes:
```go
    // Branding (B1)
    if deps.BrandingHandler != nil {
        group.GET("/branding", deps.BrandingHandler.GetStorefrontBranding)
    }
    // Customer email check (public — no auth leak, just 200/404)
    if deps.CustomerAccountHandler != nil {
        group.HEAD("/customers/check", deps.CustomerAccountHandler.CheckEmail)
    }
```

Add to `RegisterMobileStorefront` authenticated routes:
```go
    // Notify me (back in stock / price drop)
    if deps.NotifyMeHandler != nil {
        authed.POST("/products/:handle/notify", deps.NotifyMeHandler.Subscribe)
        authed.DELETE("/products/:handle/notify", deps.NotifyMeHandler.Unsubscribe)
    }
```

- [ ] **Step 7: Wire in main.go**

Add notify-me handler creation and update `mobileSFDeps` to include `NotifyMeHandler` and `BrandingHandler`.

- [ ] **Step 8: Verify build and tests**

Run: `cd services/marketplace-api && go build ./... && go test ./... -short`
Expected: Success

- [ ] **Step 9: Commit**

```bash
git add services/marketplace-api/internal/handlers/storefront/
git add services/marketplace-api/cmd/marketplace-api/main.go
git commit -m "feat(marketplace-api): add missing mobile storefront handlers — notify-me, register, list orders, my reviews, email check, branding"
```

### Task 2.6: Guest checkout support

The spec allows guest checkout (no account). The mobile route group puts checkout endpoints behind `bearerAuth`, which blocks guests. This task adds a guest checkout path.

**Files:**
- Modify: `services/marketplace-api/internal/handlers/storefront/routes.go` — move checkout submit to a group with optional auth

- [ ] **Step 1: Create OptionalGIPBearerAuth middleware**

Add to `mobile_auth.go`:
```go
// OptionalGIPBearerAuth extracts customer context if a Bearer token is present,
// but does not abort if missing. Used for guest checkout.
func OptionalGIPBearerAuth(tenantPool string, devMode bool) gin.HandlerFunc {
    // Same logic as GIPBearerAuth, but instead of aborting on missing/invalid token,
    // call c.Next() — handler checks c.GetString("customer_gip_uid") to distinguish
    // guest vs authenticated.
}
```

- [ ] **Step 2: Move checkout routes to optional-auth group**

In `RegisterMobileStorefront`, move:
```go
    // Guest-accessible (optional auth — guest checkout allowed)
    optionalAuth := OptionalGIPBearerAuth("mp-customer", deps.DevMode)
    guestOk := group.Group("", optionalAuth)
    {
        if deps.CheckoutExtHandler != nil {
            guestOk.POST("/checkout/shipping-rates", deps.ShippingRatesHandler.GetRates)
            guestOk.POST("/checkout/submit", deps.CheckoutExtHandler.Checkout)
        }
    }
```

- [ ] **Step 3: Verify build**

Run: `cd services/marketplace-api && go build ./...`
Expected: Success

- [ ] **Step 4: Commit**

```bash
git add services/marketplace-api/internal/handlers/storefront/
git commit -m "feat(marketplace-api): add optional Bearer auth for guest checkout"
```

---

## Phase 3: App Shell — Expo Project Setup

Creates `apps/storefront-mobile/` with Expo, navigation, theming, and EAS config.

### Task 3.1: Expo project scaffold

**Files:**
- Create: `apps/storefront-mobile/package.json`
- Create: `apps/storefront-mobile/app.json`
- Create: `apps/storefront-mobile/tsconfig.json`
- Create: `apps/storefront-mobile/eas.json`
- Create: `apps/storefront-mobile/babel.config.js`

- [ ] **Step 1: Create package.json**

```json
{
  "name": "@mark8ly/storefront-mobile",
  "version": "0.0.1",
  "private": true,
  "main": "expo-router/entry",
  "scripts": {
    "start": "expo start",
    "android": "expo start --android",
    "ios": "expo start --ios",
    "test": "jest",
    "check-types": "tsc --noEmit",
    "lint": "eslint app/ lib/ stores/ components/"
  },
  "dependencies": {
    "expo": "~52.0.0",
    "expo-router": "~4.0.0",
    "expo-secure-store": "~14.0.0",
    "expo-haptics": "~14.0.0",
    "expo-notifications": "~0.31.0",
    "expo-image": "~2.0.0",
    "expo-status-bar": "~2.0.0",
    "expo-splash-screen": "~0.29.0",
    "expo-font": "~13.0.0",
    "expo-linking": "~7.0.0",
    "@react-native-firebase/app": "^21.0.0",
    "@react-native-firebase/auth": "^21.0.0",
    "@stripe/stripe-react-native": "^0.40.0",
    "razorpay-react-native": "^2.3.0",
    "@tanstack/react-query": "^5.83.0",
    "zustand": "^5.0.0",
    "react-native-mmkv": "^3.2.0",
    "react": "^19.2.0",
    "react-native": "^0.76.9",
    "react-native-safe-area-context": "^5.0.0",
    "react-native-screens": "~4.0.0",
    "react-native-gesture-handler": "~2.20.0",
    "react-native-reanimated": "~3.16.0",
    "@mark8ly/mobile-shared": "*",
    "@tesserix/native": "*",
    "@tesserix/tokens": "*",
    "@tesserix/hooks": "*",
    "@tesserix/icons": "*",
    "zod": "^3.25.0"
  },
  "devDependencies": {
    "@types/react": "^19.0.0",
    "typescript": "^5.9.2",
    "jest": "^29.0.0",
    "@testing-library/react-native": "^13.3.0",
    "jest-expo": "~52.0.0"
  }
}
```

- [ ] **Step 2: Create app.json**

```json
{
  "expo": {
    "name": "Mark8ly Store",
    "slug": "mark8ly-store",
    "version": "1.0.0",
    "scheme": "mark8ly-store",
    "orientation": "portrait",
    "icon": "./assets/icon.png",
    "userInterfaceStyle": "light",
    "splash": {
      "image": "./assets/splash.png",
      "resizeMode": "contain",
      "backgroundColor": "#F7F6F2"
    },
    "assetBundlePatterns": ["**/*"],
    "ios": {
      "supportsTablet": false,
      "bundleIdentifier": "com.mark8ly.store"
    },
    "android": {
      "adaptiveIcon": {
        "foregroundImage": "./assets/adaptive-icon.png",
        "backgroundColor": "#F7F6F2"
      },
      "package": "com.mark8ly.store"
    },
    "plugins": [
      "expo-router",
      "expo-secure-store",
      "@react-native-firebase/app",
      "@stripe/stripe-react-native"
    ],
    "extra": {
      "eas": {
        "projectId": "your-eas-project-id"
      },
      "storeSlug": "default",
      "apiBaseUrl": "http://localhost:8088"
    }
  }
}
```

- [ ] **Step 3: Create eas.json**

```json
{
  "cli": {
    "version": ">= 13.0.0"
  },
  "build": {
    "development": {
      "developmentClient": true,
      "distribution": "internal"
    },
    "preview": {
      "distribution": "internal"
    },
    "production": {
      "autoIncrement": true
    }
  },
  "submit": {
    "production": {}
  }
}
```

> **Note:** Per-merchant build profiles are added dynamically. Secrets (STRIPE_PUBLISHABLE_KEY, etc.) go into EAS Secrets, never this file.

- [ ] **Step 4: Create tsconfig.json**

```json
{
  "extends": "expo/tsconfig.base",
  "compilerOptions": {
    "strict": true,
    "baseUrl": ".",
    "paths": {
      "@/*": ["./*"]
    }
  },
  "include": ["**/*.ts", "**/*.tsx", ".expo/types/**/*.ts", "expo-env.d.ts"]
}
```

- [ ] **Step 5: Create babel.config.js**

```javascript
module.exports = function (api) {
  api.cache(true);
  return {
    presets: ["babel-preset-expo"],
    plugins: ["react-native-reanimated/plugin"],
  };
};
```

- [ ] **Step 6: Create placeholder assets**

Run: `mkdir -p apps/storefront-mobile/assets && touch apps/storefront-mobile/assets/.gitkeep`

- [ ] **Step 7: Commit**

```bash
git add apps/storefront-mobile/
git commit -m "feat(storefront-mobile): scaffold Expo project with EAS config"
```

### Task 3.2: Root layout and navigation shell

**Files:**
- Create: `apps/storefront-mobile/app/_layout.tsx`
- Create: `apps/storefront-mobile/app/(tabs)/_layout.tsx`
- Create: `apps/storefront-mobile/app/(tabs)/index.tsx`
- Create: `apps/storefront-mobile/app/(tabs)/browse/_layout.tsx`
- Create: `apps/storefront-mobile/app/(tabs)/browse/index.tsx`
- Create: `apps/storefront-mobile/app/(tabs)/cart.tsx`
- Create: `apps/storefront-mobile/app/(tabs)/account/index.tsx`
- Create: `apps/storefront-mobile/app/checkout/_layout.tsx`
- Create: `apps/storefront-mobile/app/(auth)/login.tsx`
- Create: `apps/storefront-mobile/app/(auth)/register.tsx`

- [ ] **Step 1: Create root layout**

```tsx
// app/_layout.tsx
import { Stack } from "expo-router";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { AuthProvider } from "@mark8ly/mobile-shared";
import { StatusBar } from "expo-status-bar";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 60_000,
      retry: 2,
    },
  },
});

export default function RootLayout() {
  return (
    <QueryClientProvider client={queryClient}>
      <AuthProvider>
        <StatusBar style="dark" />
        <Stack screenOptions={{ headerShown: false }}>
          <Stack.Screen name="(tabs)" />
          <Stack.Screen name="(auth)" options={{ presentation: "modal" }} />
          <Stack.Screen name="checkout" />
        </Stack>
      </AuthProvider>
    </QueryClientProvider>
  );
}
```

- [ ] **Step 2: Create tab layout**

```tsx
// app/(tabs)/_layout.tsx
import { Tabs } from "expo-router";
import { Home, Search, ShoppingBag, User } from "lucide-react-native";

export default function TabLayout() {
  return (
    <Tabs
      screenOptions={{
        tabBarActiveTintColor: "#0E0E0C",
        tabBarInactiveTintColor: "#999",
        tabBarStyle: { backgroundColor: "#FFFFFF" },
        headerShown: false,
      }}
    >
      <Tabs.Screen
        name="index"
        options={{
          title: "Home",
          tabBarIcon: ({ color, size }) => <Home color={color} size={size} />,
        }}
      />
      <Tabs.Screen
        name="browse"
        options={{
          title: "Browse",
          tabBarIcon: ({ color, size }) => <Search color={color} size={size} />,
        }}
      />
      <Tabs.Screen
        name="cart"
        options={{
          title: "Cart",
          tabBarIcon: ({ color, size }) => <ShoppingBag color={color} size={size} />,
        }}
      />
      <Tabs.Screen
        name="account"
        options={{
          title: "Account",
          tabBarIcon: ({ color, size }) => <User color={color} size={size} />,
        }}
      />
    </Tabs>
  );
}
```

- [ ] **Step 3: Create placeholder screens**

Create minimal placeholder screens for each tab and route:

```tsx
// app/(tabs)/index.tsx
import { View, Text } from "react-native";
export default function HomeScreen() {
  return <View style={{ flex: 1, justifyContent: "center", alignItems: "center" }}>
    <Text>Home</Text>
  </View>;
}
```

```tsx
// app/(tabs)/browse/_layout.tsx
import { Stack } from "expo-router";
export default function BrowseLayout() {
  return <Stack screenOptions={{ headerShown: true }} />;
}
```

```tsx
// app/(tabs)/browse/index.tsx
import { View, Text } from "react-native";
export default function BrowseScreen() {
  return <View style={{ flex: 1, justifyContent: "center", alignItems: "center" }}>
    <Text>Browse</Text>
  </View>;
}
```

```tsx
// app/(tabs)/cart.tsx
import { View, Text } from "react-native";
export default function CartScreen() {
  return <View style={{ flex: 1, justifyContent: "center", alignItems: "center" }}>
    <Text>Cart</Text>
  </View>;
}
```

```tsx
// app/(tabs)/account/index.tsx
import { View, Text } from "react-native";
export default function AccountScreen() {
  return <View style={{ flex: 1, justifyContent: "center", alignItems: "center" }}>
    <Text>Account</Text>
  </View>;
}
```

```tsx
// app/checkout/_layout.tsx
import { Stack } from "expo-router";
export default function CheckoutLayout() {
  return <Stack screenOptions={{ headerShown: true, title: "Checkout" }} />;
}
```

```tsx
// app/(auth)/login.tsx
import { View, Text } from "react-native";
export default function LoginScreen() {
  return <View style={{ flex: 1, justifyContent: "center", alignItems: "center" }}>
    <Text>Login</Text>
  </View>;
}
```

```tsx
// app/(auth)/register.tsx
import { View, Text } from "react-native";
export default function RegisterScreen() {
  return <View style={{ flex: 1, justifyContent: "center", alignItems: "center" }}>
    <Text>Register</Text>
  </View>;
}
```

- [ ] **Step 4: Verify the project type-checks**

Run: `cd apps/storefront-mobile && npx tsc --noEmit`
Expected: No errors (or only warnings from expo types)

- [ ] **Step 5: Commit**

```bash
git add apps/storefront-mobile/app/
git commit -m "feat(storefront-mobile): add root layout, tab navigation, and placeholder screens"
```

### Task 3.3: Merchant theme provider

**Files:**
- Create: `apps/storefront-mobile/lib/theme/merchant-theme.ts`
- Create: `apps/storefront-mobile/lib/theme/theme-provider.tsx`

- [ ] **Step 1: Create theme override loader**

```typescript
// lib/theme/merchant-theme.ts
import type { StoreBranding } from "@mark8ly/mobile-shared";

// Default Mark8ly brand tokens (fallback when B1 not configured)
const DEFAULT_THEME = {
  primary: "#0E0E0C",    // Ink
  accent: "#2D4A2B",     // Moss
  background: "#F7F6F2", // Paper
  elevated: "#FFFFFF",
  text: "#0E0E0C",
  textSecondary: "#666666",
  border: "#E5E4DF",
  fontFamily: "SourceSans3",
} as const;

export interface MerchantTheme {
  primary: string;
  accent: string;
  background: string;
  elevated: string;
  text: string;
  textSecondary: string;
  border: string;
  fontFamily: string;
}

export function buildMerchantTheme(branding: StoreBranding | null): MerchantTheme {
  if (!branding) return { ...DEFAULT_THEME };

  return {
    primary: branding.primary_color || DEFAULT_THEME.primary,
    accent: branding.accent_color || DEFAULT_THEME.accent,
    background: branding.background_color || DEFAULT_THEME.background,
    elevated: DEFAULT_THEME.elevated,
    text: branding.primary_color || DEFAULT_THEME.text,
    textSecondary: DEFAULT_THEME.textSecondary,
    border: DEFAULT_THEME.border,
    fontFamily: branding.font_family || DEFAULT_THEME.fontFamily,
  };
}

// Try to load baked-in theme overrides (generated by generate-brand.ts)
let bakedTheme: MerchantTheme | null = null;
try {
  // eslint-disable-next-line @typescript-eslint/no-var-requires
  const overrides = require("../../theme-overrides.json");
  bakedTheme = buildMerchantTheme(overrides);
} catch {
  // No baked theme — use defaults
}

export function getMerchantTheme(): MerchantTheme {
  return bakedTheme ?? { ...DEFAULT_THEME };
}
```

- [ ] **Step 2: Create theme provider**

```tsx
// lib/theme/theme-provider.tsx
import React, { createContext, useContext, type ReactNode } from "react";
import { getMerchantTheme, type MerchantTheme } from "./merchant-theme";

const ThemeContext = createContext<MerchantTheme>(getMerchantTheme());

export function MerchantThemeProvider({ children }: { children: ReactNode }) {
  const theme = getMerchantTheme();
  return <ThemeContext.Provider value={theme}>{children}</ThemeContext.Provider>;
}

export function useTheme(): MerchantTheme {
  return useContext(ThemeContext);
}
```

- [ ] **Step 3: Commit**

```bash
git add apps/storefront-mobile/lib/theme/
git commit -m "feat(storefront-mobile): add merchant theme provider with B1 fallback"
```

### Task 3.4: Cart and checkout Zustand stores

**Files:**
- Create: `apps/storefront-mobile/stores/cart-store.ts`
- Create: `apps/storefront-mobile/stores/checkout-store.ts`
- Create: `apps/storefront-mobile/stores/recently-viewed-store.ts`
- Create: `apps/storefront-mobile/stores/__tests__/cart-store.test.ts`

- [ ] **Step 1: Write failing test for cart store**

```typescript
// stores/__tests__/cart-store.test.ts
import { describe, it, expect, beforeEach } from "vitest";
import { useCartStore } from "../cart-store";

describe("cartStore", () => {
  beforeEach(() => {
    useCartStore.getState().clear();
  });

  it("starts empty", () => {
    expect(useCartStore.getState().items).toEqual([]);
    expect(useCartStore.getState().count).toBe(0);
  });

  it("adds an item", () => {
    useCartStore.getState().addItem({
      productId: "p1",
      variantId: "v1",
      handle: "widget",
      title: "Widget",
      priceAmount: "19.99",
      currencyCode: "USD",
      qty: 1,
    });
    expect(useCartStore.getState().items).toHaveLength(1);
    expect(useCartStore.getState().count).toBe(1);
  });

  it("merges quantity for same variant", () => {
    const item = {
      productId: "p1",
      variantId: "v1",
      handle: "widget",
      title: "Widget",
      priceAmount: "19.99",
      currencyCode: "USD",
      qty: 1,
    };
    useCartStore.getState().addItem(item);
    useCartStore.getState().addItem(item);
    expect(useCartStore.getState().items).toHaveLength(1);
    expect(useCartStore.getState().items[0].qty).toBe(2);
  });

  it("removes an item", () => {
    useCartStore.getState().addItem({
      productId: "p1",
      variantId: "v1",
      handle: "widget",
      title: "Widget",
      priceAmount: "19.99",
      currencyCode: "USD",
      qty: 1,
    });
    useCartStore.getState().removeItem("p1", "v1");
    expect(useCartStore.getState().items).toEqual([]);
  });

  it("updates quantity", () => {
    useCartStore.getState().addItem({
      productId: "p1",
      variantId: "v1",
      handle: "widget",
      title: "Widget",
      priceAmount: "19.99",
      currencyCode: "USD",
      qty: 1,
    });
    useCartStore.getState().updateQty("p1", "v1", 5);
    expect(useCartStore.getState().items[0].qty).toBe(5);
  });

  it("computes subtotal", () => {
    useCartStore.getState().addItem({
      productId: "p1",
      variantId: "v1",
      handle: "widget",
      title: "Widget",
      priceAmount: "10.00",
      currencyCode: "USD",
      qty: 3,
    });
    expect(useCartStore.getState().subtotal).toBe("30.00");
  });
});
```

- [ ] **Step 2: Run test — should fail**

- [ ] **Step 3: Implement cart store**

```typescript
// stores/cart-store.ts
import { create } from "zustand";

export interface CartItem {
  productId: string;
  variantId: string;
  handle: string;
  title: string;
  priceAmount: string;
  currencyCode: string;
  qty: number;
  imageUrl?: string;
}

interface CartState {
  items: CartItem[];
  count: number;
  subtotal: string;
  addItem: (item: CartItem) => void;
  removeItem: (productId: string, variantId: string) => void;
  updateQty: (productId: string, variantId: string, qty: number) => void;
  clear: () => void;
}

function computeDerived(items: CartItem[]) {
  const count = items.reduce((sum, i) => sum + i.qty, 0);
  const subtotal = items
    .reduce((sum, i) => sum + parseFloat(i.priceAmount) * i.qty, 0)
    .toFixed(2);
  return { count, subtotal };
}

export const useCartStore = create<CartState>((set) => ({
  items: [],
  count: 0,
  subtotal: "0.00",

  addItem: (item) =>
    set((state) => {
      const existing = state.items.find(
        (i) => i.productId === item.productId && i.variantId === item.variantId
      );
      const newItems = existing
        ? state.items.map((i) =>
            i.productId === item.productId && i.variantId === item.variantId
              ? { ...i, qty: i.qty + item.qty }
              : i
          )
        : [...state.items, item];
      return { items: newItems, ...computeDerived(newItems) };
    }),

  removeItem: (productId, variantId) =>
    set((state) => {
      const newItems = state.items.filter(
        (i) => !(i.productId === productId && i.variantId === variantId)
      );
      return { items: newItems, ...computeDerived(newItems) };
    }),

  updateQty: (productId, variantId, qty) =>
    set((state) => {
      const newItems = state.items.map((i) =>
        i.productId === productId && i.variantId === variantId
          ? { ...i, qty }
          : i
      );
      return { items: newItems, ...computeDerived(newItems) };
    }),

  clear: () => set({ items: [], count: 0, subtotal: "0.00" }),
}));
```

- [ ] **Step 4: Create checkout store**

```typescript
// stores/checkout-store.ts
import { create } from "zustand";
import type { Address } from "@mark8ly/mobile-shared";

interface CheckoutState {
  step: 1 | 2 | 3 | 4;
  email: string;
  customerName: string;
  address: Omit<Address, "id" | "is_default"> | null;
  selectedAddressId: string | null;
  shippingRateId: string | null;
  paymentProvider: string | null;
  couponCode: string | null;
  couponDiscount: string | null;
  giftCardCode: string | null;
  giftCardAmount: string | null;
  loyaltyPoints: number;
  saveAddress: boolean;
  setStep: (step: 1 | 2 | 3 | 4) => void;
  setContact: (email: string, name: string) => void;
  setAddress: (address: Omit<Address, "id" | "is_default">) => void;
  setSelectedAddressId: (id: string) => void;
  setShippingRate: (id: string) => void;
  setPaymentProvider: (provider: string) => void;
  setCoupon: (code: string, discount: string) => void;
  clearCoupon: () => void;
  setGiftCard: (code: string, amount: string) => void;
  clearGiftCard: () => void;
  setLoyaltyPoints: (points: number) => void;
  setSaveAddress: (save: boolean) => void;
  reset: () => void;
}

export const useCheckoutStore = create<CheckoutState>((set) => ({
  step: 1,
  email: "",
  customerName: "",
  address: null,
  selectedAddressId: null,
  shippingRateId: null,
  paymentProvider: null,
  couponCode: null,
  couponDiscount: null,
  giftCardCode: null,
  giftCardAmount: null,
  loyaltyPoints: 0,
  saveAddress: false,
  setStep: (step) => set({ step }),
  setContact: (email, customerName) => set({ email, customerName }),
  setAddress: (address) => set({ address }),
  setSelectedAddressId: (id) => set({ selectedAddressId: id }),
  setShippingRate: (id) => set({ shippingRateId: id }),
  setPaymentProvider: (provider) => set({ paymentProvider: provider }),
  setCoupon: (code, discount) => set({ couponCode: code, couponDiscount: discount }),
  clearCoupon: () => set({ couponCode: null, couponDiscount: null }),
  setGiftCard: (code, amount) => set({ giftCardCode: code, giftCardAmount: amount }),
  clearGiftCard: () => set({ giftCardCode: null, giftCardAmount: null }),
  setLoyaltyPoints: (points) => set({ loyaltyPoints: points }),
  setSaveAddress: (save) => set({ saveAddress: save }),
  reset: () =>
    set({
      step: 1,
      email: "",
      customerName: "",
      address: null,
      selectedAddressId: null,
      shippingRateId: null,
      paymentProvider: null,
      couponCode: null,
      couponDiscount: null,
      giftCardCode: null,
      giftCardAmount: null,
      loyaltyPoints: 0,
      saveAddress: false,
    }),
}));
```

- [ ] **Step 5: Create recently viewed store**

```typescript
// stores/recently-viewed-store.ts
import { create } from "zustand";

const MAX_RECENT = 20;

interface RecentProduct {
  handle: string;
  title: string;
  priceAmount: string;
  currencyCode: string;
  imageUrl: string;
}

interface RecentlyViewedState {
  products: RecentProduct[];
  addProduct: (product: RecentProduct) => void;
  clear: () => void;
}

export const useRecentlyViewedStore = create<RecentlyViewedState>((set) => ({
  products: [],
  addProduct: (product) =>
    set((state) => {
      const filtered = state.products.filter((p) => p.handle !== product.handle);
      return { products: [product, ...filtered].slice(0, MAX_RECENT) };
    }),
  clear: () => set({ products: [] }),
}));
```

- [ ] **Step 6: Run tests — should pass**

Run: `cd apps/storefront-mobile && npx vitest run stores/__tests__/cart-store.test.ts`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add apps/storefront-mobile/stores/
git commit -m "feat(storefront-mobile): add cart, checkout, and recently-viewed stores"
```

### Task 3.5: Generate brand script

**Files:**
- Create: `apps/storefront-mobile/scripts/generate-brand.ts`

- [ ] **Step 1: Create the brand generation script**

```typescript
// scripts/generate-brand.ts
/**
 * Pre-build script that fetches B1 branding config for a merchant
 * and generates app icon, splash, and theme overrides.
 *
 * Usage: STORE_SLUG=acme API_BASE_URL=https://api.mark8ly.com npx ts-node scripts/generate-brand.ts
 */
import * as fs from "fs";
import * as path from "path";

const STORE_SLUG = process.env.STORE_SLUG;
const STORE_NAME = process.env.STORE_NAME;
const API_BASE_URL = process.env.API_BASE_URL || "http://localhost:8088";

async function main() {
  if (!STORE_SLUG) {
    console.log("No STORE_SLUG set — using default branding");
    return;
  }

  console.log(`Fetching branding for store: ${STORE_SLUG}`);

  try {
    const res = await fetch(
      `${API_BASE_URL}/api/v1/mobile/storefront/stores/${STORE_SLUG}/branding`
    );

    if (!res.ok) {
      console.warn(`Branding not found (${res.status}) — using defaults`);
      return;
    }

    const branding = await res.json();

    // Write theme overrides
    const overridesPath = path.join(__dirname, "..", "theme-overrides.json");
    fs.writeFileSync(overridesPath, JSON.stringify(branding.data, null, 2));
    console.log(`Theme overrides written to ${overridesPath}`);

    // Update app.json with store name
    const appJsonPath = path.join(__dirname, "..", "app.json");
    const appJson = JSON.parse(fs.readFileSync(appJsonPath, "utf-8"));

    if (STORE_NAME) {
      appJson.expo.name = STORE_NAME;
    }
    appJson.expo.scheme = `${STORE_SLUG}-store`;
    appJson.expo.extra.storeSlug = STORE_SLUG;

    if (branding.data?.background_color) {
      appJson.expo.splash.backgroundColor = branding.data.background_color;
    }

    fs.writeFileSync(appJsonPath, JSON.stringify(appJson, null, 2));
    console.log(`app.json updated for store: ${STORE_NAME || STORE_SLUG}`);
  } catch (err) {
    console.warn("Failed to fetch branding — using defaults:", err);
  }
}

main().catch(console.error);
```

- [ ] **Step 2: Commit**

```bash
git add apps/storefront-mobile/scripts/
git commit -m "feat(storefront-mobile): add generate-brand pre-build script"
```

---

## Phase 4: Browse & Product

> **Note:** This phase implements the Home, Browse, Search, Category, and Product Detail screens. Each screen is a self-contained task. Code patterns established here (query hooks, component composition) are reused in Phases 5-6.
>
> **Implementation approach:** Follow the existing patterns from Phase 3 — create the storefront API functions in `lib/storefront-api/`, create React Query hooks, then build screen components. Test API functions with Vitest, test screens with React Native Testing Library.
>
> This phase is large. Each task below contains the key files, hook definitions, and screen structure. Full component code is written during implementation following the spec's screen descriptions (Section 6.1-6.2 of the design spec).

### Task 4.1: Storefront API functions

**Files:**
- Create: `apps/storefront-mobile/lib/storefront-api/store.ts`
- Create: `apps/storefront-mobile/lib/storefront-api/products.ts`
- Create: `apps/storefront-mobile/lib/storefront-api/categories.ts`
- Create: `apps/storefront-mobile/lib/storefront-api/__tests__/products.test.ts`

Implement API functions that wrap the shared `ApiClient`:
- `fetchStoreBranding(client)` → `GET /branding`
- `listProducts(client, options)` → `GET /products?search=&category=&sort=&page=&limit=`
- `getProductByHandle(client, handle)` → `GET /products/:handle`
- `listCategories(client)` → `GET /categories`
- `listProductsByCategory(client, slug, options)` → `GET /categories/:slug/products`

Write tests for the URL construction and parameter passing. Commit.

### Task 4.2: React Query hooks for products

**Files:**
- Create: `apps/storefront-mobile/lib/hooks/use-products.ts`
- Create: `apps/storefront-mobile/lib/hooks/use-categories.ts`
- Create: `apps/storefront-mobile/lib/hooks/use-store-branding.ts`

Create hooks:
- `useProducts(options)` — `useInfiniteQuery` with cursor pagination
- `useProductByHandle(handle)` — `useQuery`
- `useCategories()` — `useQuery`
- `useProductsByCategory(slug, options)` — `useInfiniteQuery`
- `useStoreBranding()` — `useQuery`

Commit.

### Task 4.3: Home screen

**Files:**
- Modify: `apps/storefront-mobile/app/(tabs)/index.tsx`
- Create: `apps/storefront-mobile/components/HomeBanner.tsx`
- Create: `apps/storefront-mobile/components/CategoryPills.tsx`
- Create: `apps/storefront-mobile/components/ProductCard.tsx`
- Create: `apps/storefront-mobile/components/RecentlyViewed.tsx`

Implement per spec Section 6.1: hero banner, category pills, featured products carousel, new arrivals grid, recently viewed row, pull-to-refresh, empty state for unconfigured merchants. Commit.

### Task 4.4: Browse screens (categories, search)

**Files:**
- Modify: `apps/storefront-mobile/app/(tabs)/browse/_layout.tsx` — persistent SearchBar
- Modify: `apps/storefront-mobile/app/(tabs)/browse/index.tsx` — category grid
- Create: `apps/storefront-mobile/app/(tabs)/browse/search.tsx`
- Create: `apps/storefront-mobile/app/(tabs)/browse/category/[slug].tsx`

Implement per spec Section 6.2: persistent SearchBar in header, category grid, search with recent history (MMKV, Text-only rendering), category listing with sort BottomSheet, infinite scroll.

Also create the search history store:

- Create: `apps/storefront-mobile/stores/search-history-store.ts`

```typescript
// stores/search-history-store.ts
import { create } from "zustand";

const MAX_SEARCHES = 10;

interface SearchHistoryState {
  searches: string[];
  addSearch: (term: string) => void;
  removeSearch: (term: string) => void;
  clear: () => void;
}

export const useSearchHistoryStore = create<SearchHistoryState>((set) => ({
  searches: [],
  addSearch: (term) =>
    set((state) => {
      const filtered = state.searches.filter((s) => s !== term);
      return { searches: [term, ...filtered].slice(0, MAX_SEARCHES) };
    }),
  removeSearch: (term) =>
    set((state) => ({
      searches: state.searches.filter((s) => s !== term),
    })),
  clear: () => set({ searches: [] }),
}));
```

> **Important:** Search history is rendered as `Text` components only — never via WebView or innerHTML. See spec Section 6.2 and security Section 13.

Commit.

### Task 4.5: Product detail screen

**Files:**
- Create: `apps/storefront-mobile/app/(tabs)/browse/product/[handle].tsx`
- Create: `apps/storefront-mobile/components/ProductGallery.tsx`
- Create: `apps/storefront-mobile/components/VariantSelector.tsx`
- Create: `apps/storefront-mobile/components/StarRating.tsx`
- Create: `apps/storefront-mobile/components/ReviewCard.tsx`
- Create: `apps/storefront-mobile/components/NotifyMeButton.tsx`
- Create: `apps/storefront-mobile/components/WishlistButton.tsx`

Implement per spec Section 6.2 PDP: media gallery with pinch-to-zoom + magnifier hint, title + price + inline rating, variant selector, stock indicator, notify-me for out-of-stock, sticky add-to-cart with haptics, wishlist heart, reviews accordion (expanded), description accordion (collapsed), related products. Commit.

### Task 4.6: Notify-me API

**Files:**
- Create: `apps/storefront-mobile/lib/storefront-api/notify.ts`
- Create: `apps/storefront-mobile/lib/hooks/use-notify-me.ts`

API functions:
- `subscribeNotify(client, handle, type)` → `POST /products/:handle/notify`
- `unsubscribeNotify(client, handle)` → `DELETE /products/:handle/notify`

Hook: `useNotifyMe(handle)` — mutation with optimistic toggle. Commit.

---

## Phase 5: Cart & Checkout

### Task 5.1: Cart screen

**Files:**
- Modify: `apps/storefront-mobile/app/(tabs)/cart.tsx`
- Create: `apps/storefront-mobile/components/CartItem.tsx`

Implement per spec Section 6.3: cart item list with quantity stepper, swipe-to-delete, save-for-later (→ wishlist), out-of-stock inline Banner, subtotal, checkout CTA (guest prompt), empty state, haptics.

**Save-for-later detail:** Swipe action on a cart item shows "Save for later" option. On tap:
- If authenticated: calls wishlist add mutation, removes item from cart, shows toast "Moved to wishlist"
- If guest: prompts login (same auth modal as checkout). On successful login, adds to wishlist and removes from cart.
- Requires `useWishlist` hook from Phase 6 — implement as a no-op stub in Phase 5, wire fully in Phase 6.

**Out-of-stock cart validation detail:** On cart screen mount and pull-to-refresh, call `listProducts` for all product IDs in cart and compare stock status. If stock has changed:
- `stock_quantity === 0`: show red Banner on that CartItem — "Out of stock — remove to continue"
- `stock_quantity < qty in cart`: show amber Banner — "Only X left"
- Disable "Checkout" CTA if any item is out of stock

Create a hook:
- Create: `apps/storefront-mobile/lib/hooks/use-cart-stock-check.ts`

```typescript
// lib/hooks/use-cart-stock-check.ts
// useCartStockCheck(cartItems) — queries current stock for all cart product IDs,
// returns Map<productId, { available: number; status: string }>.
// Used by cart screen to show inline warnings.
```

Commit.

### Task 5.2: Checkout API functions

**Files:**
- Create: `apps/storefront-mobile/lib/storefront-api/checkout.ts`
- Create: `apps/storefront-mobile/lib/hooks/use-checkout.ts`

API functions:
- `fetchShippingRates(client, body)` → `POST /checkout/shipping-rates`
- `submitCheckout(client, body)` → `POST /checkout/submit`
- `checkCustomerEmail(client, email)` → `HEAD /customers/check?email=`

Hooks:
- `useShippingRates(address, cartItems)` — `useQuery`, enabled when address is set
- `useSubmitCheckout()` — `useMutation`
- `useCheckCustomerEmail()` — `useMutation` (debounced)

Commit.

### Task 5.3: Checkout Step 1 — Details (Contact + Address)

**Files:**
- Create: `apps/storefront-mobile/app/checkout/details.tsx`
- Create: `apps/storefront-mobile/components/CheckoutProgress.tsx`

Implement per spec Section 6.4 Step 1: contact fields (pre-filled if authed), account recognition on email blur, address picker (saved addresses) or form, country select, save-address checkbox, "Continue" CTA.

**Mid-checkout login modal detail:** Also create:
- Create: `apps/storefront-mobile/components/LoginModal.tsx`

A modal overlay (not full-screen navigation) that:
1. Renders over the checkout screen (preserves checkout state in Zustand)
2. Shows email/password login form using `@react-native-firebase/auth`
3. On success: calls `signIn()` from AuthProvider, closes modal, checkout store persists, saved addresses now available
4. On dismiss: guest continues with manual address entry
5. Triggered by: account recognition nudge on email blur (uses `useCheckCustomerEmail` hook)

```tsx
// components/LoginModal.tsx — key structure
// Props: { visible: boolean; onDismiss: () => void; onSuccess: () => void; email?: string }
// Uses Modal from @tesserix/native
// Pre-fills email if passed from checkout contact field
// Does NOT reset checkout store on login — state persists
```

Commit.

### Task 5.4: Checkout Step 2 — Shipping

**Files:**
- Create: `apps/storefront-mobile/app/checkout/shipping.tsx`

Implement per spec Section 6.4 Step 2: fetch rates, radio list of shipping options, loading skeleton, zero-results error state, "Continue" CTA. Commit.

### Task 5.5: Checkout Step 3 — Payment

**Files:**
- Create: `apps/storefront-mobile/app/checkout/payment.tsx`

Implement per spec Section 6.4 Step 3: payment method selection (Apple/Google Pay, Stripe CardField, Razorpay, PayPal WebView), load failure error state with retry, "Continue" CTA. Commit.

### Task 5.6: Checkout Step 4 — Review + Discounts

**Files:**
- Create: `apps/storefront-mobile/app/checkout/review.tsx`
- Create: `apps/storefront-mobile/components/CouponInput.tsx`
- Create: `apps/storefront-mobile/components/GiftCardInput.tsx`
- Create: `apps/storefront-mobile/components/LoyaltyRedemption.tsx`

Implement per spec Section 6.4 Step 4: order summary with edit links, inline discounts (coupon, gift card, loyalty), totals, "Place order" CTA with idempotency key, loading state, error handling, server price mismatch confirmation, success haptic. Commit.

### Task 5.7: Order confirmation

**Files:**
- Create: `apps/storefront-mobile/app/checkout/confirmation/[id].tsx`

Implement per spec: order number, thank you message, order summary, guest account creation prompt, "Continue shopping" CTA, cart clear on arrival.

**Post-checkout account creation detail:** Also create:
- Create: `apps/storefront-mobile/components/GuestAccountPrompt.tsx`
- Create: `apps/storefront-mobile/lib/hooks/use-create-account.ts`

`GuestAccountPrompt` is shown only when the customer completed checkout as a guest (check `useAuth().isAuthenticated === false`):
1. Pre-fills email from checkout store
2. Shows a single password field + "Create account" button
3. On tap: calls `@react-native-firebase/auth` `createUserWithEmailAndPassword()` against `mp-customer` pool
4. Then calls `POST /customers/register` to create the customer profile
5. Then calls a backend endpoint to associate the just-placed order with the new customer ID (or the backend does this automatically by email match)
6. On success: calls `signIn()` from AuthProvider, shows toast "Account created"
7. Dismissible — "No thanks" link hides the prompt

```typescript
// lib/hooks/use-create-account.ts
// useCreateAccount() — useMutation that:
// 1. Creates Firebase user
// 2. Calls POST /customers/register
// 3. Calls signIn() on AuthProvider
// Returns { mutate, isPending, error }
```

Commit.

---

## Phase 6: Account & Features

### Task 6.1: Auth screens (Login + Register)

**Files:**
- Modify: `apps/storefront-mobile/app/(auth)/login.tsx`
- Modify: `apps/storefront-mobile/app/(auth)/register.tsx`

Implement: email/password login via `@react-native-firebase/auth` against `mp-customer` pool, registration with `POST /customers/register`, error handling (wrong password, network, locked), "Forgot password" link to web. Commit.

### Task 6.2: Account dashboard and profile

**Files:**
- Modify: `apps/storefront-mobile/app/(tabs)/account/index.tsx`
- Create: `apps/storefront-mobile/lib/storefront-api/customer.ts`
- Create: `apps/storefront-mobile/lib/hooks/use-customer.ts`

Implement per spec Section 6.5: profile header, quick links grid (orders, addresses, wishlist, loyalty, reviews, logout), auth gate. Commit.

### Task 6.3: Order history and detail

**Files:**
- Create: `apps/storefront-mobile/app/(tabs)/account/orders.tsx`
- Create: `apps/storefront-mobile/app/(tabs)/account/orders/[id].tsx`
- Create: `apps/storefront-mobile/lib/hooks/use-orders.ts`

Implement per spec: order list with status badges, infinite scroll, order detail with line items, shipping, payment, timeline. Commit.

### Task 6.4: Addresses CRUD

**Files:**
- Create: `apps/storefront-mobile/app/(tabs)/account/addresses.tsx`
- Create: `apps/storefront-mobile/lib/storefront-api/addresses.ts`
- Create: `apps/storefront-mobile/lib/hooks/use-addresses.ts`

Implement per spec: address list, default indicator, add/edit/delete, set default. Commit.

### Task 6.5: Wishlist

**Files:**
- Create: `apps/storefront-mobile/app/(tabs)/account/wishlist.tsx`
- Create: `apps/storefront-mobile/lib/storefront-api/wishlist.ts`
- Create: `apps/storefront-mobile/lib/hooks/use-wishlist.ts`

Implement per spec: wishlist grid, swipe-to-remove, add-to-cart action, empty state. Commit.

### Task 6.6: Loyalty dashboard

**Files:**
- Create: `apps/storefront-mobile/app/(tabs)/account/loyalty.tsx`
- Create: `apps/storefront-mobile/lib/storefront-api/loyalty.ts`
- Create: `apps/storefront-mobile/lib/hooks/use-loyalty.ts`

Implement per spec: points balance, tier progress, points history, referral code with share (12+ char, crypto random), enrollment prompt. Commit.

### Task 6.7: Reviews

**Files:**
- Create: `apps/storefront-mobile/app/(tabs)/account/reviews.tsx`
- Create: `apps/storefront-mobile/lib/storefront-api/reviews.ts`
- Create: `apps/storefront-mobile/lib/hooks/use-reviews.ts`

Implement per spec: my reviews list, product thumbnail + rating + text, tap → product detail. Commit.

### Task 6.8: Push notification setup

**Files:**
- Modify: `apps/storefront-mobile/app/_layout.tsx` — add push registration on auth
- Create: `apps/storefront-mobile/lib/push/setup.ts`

Implement per spec Section 8: register push token on login (with device_id), delete on logout, handle notification tap with deep link validation, rate limit 5/hour/customer. Commit.

---

## Final Verification

### Task F.1: Integration test

- [ ] **Step 1: Start marketplace-api in dev mode**

Run: `cd services/marketplace-api && MODE=both go run ./cmd/marketplace-api/`

- [ ] **Step 2: Start Expo dev server**

Run: `cd apps/storefront-mobile && npx expo start`

- [ ] **Step 3: Verify critical path on simulator/device**

Manual test checklist:
- [ ] App launches with default branding (Paper/Ink/Moss)
- [ ] Tab navigation works (Home, Browse, Cart, Account)
- [ ] Products load on Browse screen
- [ ] Search returns results
- [ ] Product detail shows gallery, variants, reviews
- [ ] Add to cart works (with haptic)
- [ ] Cart shows items, quantity stepper works
- [ ] Checkout flow: details → shipping → payment → review → confirm
- [ ] Login/register works
- [ ] Account screens load (orders, wishlist, loyalty)
- [ ] Push notification permission prompt appears on login

- [ ] **Step 4: Run all unit tests**

Run: `cd packages/mobile-shared && npx vitest run && cd ../../apps/storefront-mobile && npx vitest run`
Expected: All PASS

- [ ] **Step 5: Type-check**

Run: `cd apps/storefront-mobile && npx tsc --noEmit`
Expected: No errors

- [ ] **Step 6: Final commit**

```bash
git commit -m "feat(storefront-mobile): complete v1 storefront mobile app"
```
