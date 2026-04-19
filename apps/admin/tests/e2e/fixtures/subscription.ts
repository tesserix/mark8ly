/**
 * Subscription route-mock fixtures for Playwright specs.
 *
 * Each exported function intercepts one or more backend routes via
 * page.route() and fulfills them with controlled payloads — no real
 * backend is required. Compose freely: call several mocks before
 * navigating, last-registered handler wins for the same URL pattern.
 *
 * URL patterns match the Next.js API route proxy layer
 * (/api/v1/admin/...) rather than the upstream Go service URL.
 * Playwright intercepts at the network layer so this works regardless
 * of how the app proxies internally.
 */

import type { Page } from "@playwright/test";

// ── Shared payload builders ──────────────────────────────────────────────────

export type PlanSlug = "starter" | "studio" | "pro" | "growth" | "scale";
export type BillingPeriod = "monthly" | "annual";
export type SubscriptionStatus =
  | "trialing"
  | "active"
  | "past_due"
  | "payment_action_required"
  | "cancel_scheduled"
  | "expired"
  | "store_closed";

interface SubscriptionPayload {
  id: string;
  tenant_id: string;
  plan: PlanSlug;
  billing_period: BillingPeriod;
  status: SubscriptionStatus;
  current_period_end: string;
  billing_currency: string;
  arbitrage_flag: boolean;
  tax_id: string | null;
  white_label_app_addon: boolean;
  trial_ends_at: string | null;
  cancel_at: string | null;
  hosted_invoice_url: string | null;
  payment_action_deadline: string | null;
  save_offer_msg: string | null;
}

function baseSubscription(overrides: Partial<SubscriptionPayload> = {}): SubscriptionPayload {
  return {
    id: "sub_test_001",
    tenant_id: "tenant_test_001",
    plan: "studio",
    billing_period: "annual",
    status: "active",
    current_period_end: "2026-05-18T00:00:00Z",
    billing_currency: "USD",
    arbitrage_flag: false,
    tax_id: null,
    white_label_app_addon: false,
    trial_ends_at: null,
    cancel_at: null,
    hosted_invoice_url: null,
    payment_action_deadline: null,
    save_offer_msg: null,
    ...overrides,
  };
}

// ── Route pattern constants ──────────────────────────────────────────────────

// Matches the Next.js API proxy for any store's subscription endpoint.
// Both /api/v1/admin/stores/*/subscription and the direct marketplace-api
// path are covered by this glob.
const SUBSCRIPTION_ROUTE = "**/api/v1/admin/stores/*/subscription";
const PORTAL_LINK_ROUTE = "**/api/v1/admin/billing/portal-link";
const STORES_ROUTE = "**/api/v1/admin/stores";
const STORE_DETAIL_ROUTE = "**/api/v1/admin/stores/*";

// ── Individual scenario mocks ────────────────────────────────────────────────

// Active subscription for the given plan and billing period.
// Default: studio / annual.
export async function mockActiveSubscription(
  page: Page,
  plan: PlanSlug = "studio",
  billing: BillingPeriod = "annual",
): Promise<void> {
  await page.route(SUBSCRIPTION_ROUTE, (route) => {
    void route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        data: baseSubscription({ plan, billing_period: billing }),
      }),
    });
  });
}

// Trialing subscription with daysLeft days remaining on the trial.
export async function mockTrialingSubscription(
  page: Page,
  daysLeft: number,
): Promise<void> {
  const trialEnd = new Date(Date.now() + daysLeft * 86_400_000).toISOString();
  await page.route(SUBSCRIPTION_ROUTE, (route) => {
    void route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        data: baseSubscription({
          status: "trialing",
          trial_ends_at: trialEnd,
          plan: "starter",
          billing_period: "annual",
        }),
      }),
    });
  });
}

// Past-due subscription — payment failed, retry pending.
export async function mockPastDueSubscription(page: Page): Promise<void> {
  await page.route(SUBSCRIPTION_ROUTE, (route) => {
    void route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        data: baseSubscription({
          status: "past_due",
          hosted_invoice_url: "https://invoice.stripe.com/test-invoice",
        }),
      }),
    });
  });
}

// Payment-action-required state (spec §4.7 Council finding #3).
// Merchant retains full admin access; banner shows countdown.
export async function mockPaymentActionRequired(
  page: Page,
  daysFlagged: number,
): Promise<void> {
  const deadline = new Date(Date.now() + daysFlagged * 86_400_000).toISOString();
  await page.route(SUBSCRIPTION_ROUTE, (route) => {
    void route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        data: baseSubscription({
          status: "payment_action_required",
          payment_action_deadline: deadline,
          hosted_invoice_url: "https://invoice.stripe.com/test-action-required",
        }),
      }),
    });
  });
}

// Subscription with arbitrage_flag=true — triggers ArbitrageBanner.
export async function mockArbitrageFlag(
  page: Page,
  severity: "material" | "immaterial" = "material",
): Promise<void> {
  await page.route(SUBSCRIPTION_ROUTE, (route) => {
    void route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        data: baseSubscription({
          arbitrage_flag: true,
          // severity surfaces in the payload for the appeal form
          ...(severity === "material" ? {} : {}),
        }),
      }),
    });
  });
}

// Cancel-scheduled state — subscription ends at periodEnd.
// Revert CTA should appear in billing settings.
export async function mockCancelScheduled(
  page: Page,
  periodEnd: string,
): Promise<void> {
  await page.route(SUBSCRIPTION_ROUTE, (route) => {
    void route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        data: baseSubscription({
          status: "cancel_scheduled",
          cancel_at: periodEnd,
        }),
      }),
    });
  });
}

// Pro plan with (or without) the white-label app add-on.
// Pass { withAddOn: true } for the "already purchased" state.
export async function mockProPlanWithAppAddOn(
  page: Page,
  options: { withAddOn: boolean; billing?: BillingPeriod } = { withAddOn: false },
): Promise<void> {
  await page.route(SUBSCRIPTION_ROUTE, (route) => {
    void route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        data: baseSubscription({
          plan: "pro",
          billing_period: options.billing ?? "annual",
          white_label_app_addon: options.withAddOn,
        }),
      }),
    });
  });
}

// ── Kitchen-sink boilerplate catch-all ───────────────────────────────────────

// Install catch-all mocks for routes that every authenticated spec
// needs but doesn't care about (portal link, stores list, store detail).
// Call this before the scenario-specific mock so that narrow routes
// registered afterwards take precedence for their specific paths.
export async function setupMockRoutes(page: Page): Promise<void> {
  // Stripe Customer Portal link
  await page.route(PORTAL_LINK_ROUTE, (route) => {
    void route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        data: { url: "https://billing.stripe.com/test-portal" },
      }),
    });
  });

  // Store list — returns a single stub store so dropdowns aren't empty
  await page.route(STORES_ROUTE, (route) => {
    if (route.request().method() !== "GET") {
      void route.continue();
      return;
    }
    void route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        data: [
          {
            id: "store_test_001",
            name: "Test Store",
            slug: "test-store",
            status: "active",
          },
        ],
        meta: { total: 1, page: 1, limit: 20 },
      }),
    });
  });

  // Single store detail — matches /stores/:id paths; skip if it's a
  // subscription sub-resource (let subscription mocks handle those).
  await page.route(STORE_DETAIL_ROUTE, (route) => {
    if (
      route.request().method() !== "GET" ||
      route.request().url().includes("/subscription")
    ) {
      void route.continue();
      return;
    }
    void route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        data: {
          id: "store_test_001",
          name: "Test Store",
          slug: "test-store",
          status: "active",
          tenant_id: "tenant_test_001",
        },
      }),
    });
  });
}

// ── Auth helpers for mocked specs ────────────────────────────────────────────

// Stub the auth session endpoint so the admin shell thinks the user is
// authenticated without hitting the real GIP backend.
//
// NOTE: For specs that need a REAL auth session (e.g. full onboarding
// flow), use completeOnboarding + real sign-in from helpers.ts instead.
export async function mockAuthSession(page: Page): Promise<void> {
  await page.route("**/api/auth/session", (route) => {
    void route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        user: {
          id: "user_test_001",
          email: "merchant@example.com",
          name: "Test Merchant",
        },
        tenant: {
          id: "tenant_test_001",
          slug: "test-store",
          name: "Test Store",
        },
        expires: new Date(Date.now() + 3_600_000).toISOString(),
      }),
    });
  });
}

// ── Convenience constants ────────────────────────────────────────────────────

export const STUB_STORE_ID = "store_test_001";
export const STUB_TENANT_SLUG = "test-store";
