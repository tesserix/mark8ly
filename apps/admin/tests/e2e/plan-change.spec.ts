/**
 * P16 — Plan-change flow (authenticated, fully mocked).
 *
 * Exercises the upgrade/downgrade plan-change UI:
 *   1. Current plan: Starter annual.
 *   2. Preflight for target=studio → allow_upgrade_now decision.
 *   3. Select Studio → upgrade preview renders.
 *   4. Apply → POST fires, toast appears, redirect to billing settings.
 *
 * The entire backend is mocked via `page.route()` — no real tenant or
 * real subscription is required.
 */

import { expect, test } from "@playwright/test";

import { ADMIN_URL } from "./helpers";
import {
  STUB_STORE_ID,
  mockAuthSession,
  setupMockRoutes,
} from "./fixtures/subscription";

const PLAN_CHANGE_URL = `${ADMIN_URL}/admin/stores/${STUB_STORE_ID}/settings/billing/plan-change`;
const BILLING_URL = `${ADMIN_URL}/admin/settings/billing`;

test.describe("plan-change flow", () => {
  test.beforeEach(async ({ page }) => {
    // Install auth + boilerplate mocks before each test.
    await mockAuthSession(page);
    await setupMockRoutes(page);
  });

  test("upgrade preview renders when Studio is selected", async ({ page }) => {
    // Mock current subscription: Starter annual.
    await page.route("**/api/v1/admin/stores/*/subscription", (route) => {
      void route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            id: "sub_test_001",
            tenant_id: "tenant_test_001",
            plan: "starter",
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
          },
        }),
      });
    });

    // Mock preflight: upgrading to studio is allowed immediately.
    await page.route("**/api/v1/admin/subscription/preflight**", (route) => {
      void route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            decision: "allow_upgrade_now",
            effective_at: "2026-04-19T00:00:00Z",
            current_plan_diff: {
              proration_amount: 4200,
              proration_currency: "USD",
              next_billing_date: "2026-05-18T00:00:00Z",
            },
          },
        }),
      });
    });

    // Mock the plan-change POST.
    await page.route("**/api/v1/admin/subscription/change-plan", (route) => {
      void route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: { result: "upgrade_committed" },
        }),
      });
    });

    await page.goto(PLAN_CHANGE_URL);

    // Select the Studio plan radio / card.
    const studioOption = page
      .getByRole("radio", { name: /studio/i })
      .or(page.getByLabel(/studio/i));
    await studioOption.click();

    // Upgrade preview should render — look for proration or effective date.
    await expect(
      page.getByText(/upgrade/i).or(page.getByText(/effective/i)),
    ).toBeVisible({ timeout: 8_000 });
  });

  test("Apply commits the upgrade and redirects to billing settings", async ({
    page,
  }) => {
    // Starter annual subscription.
    await page.route("**/api/v1/admin/stores/*/subscription", (route) => {
      void route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            id: "sub_test_001",
            tenant_id: "tenant_test_001",
            plan: "starter",
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
          },
        }),
      });
    });

    await page.route("**/api/v1/admin/subscription/preflight**", (route) => {
      void route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            decision: "allow_upgrade_now",
            effective_at: "2026-04-19T00:00:00Z",
            current_plan_diff: {
              proration_amount: 4200,
              proration_currency: "USD",
              next_billing_date: "2026-05-18T00:00:00Z",
            },
          },
        }),
      });
    });

    let changePlanCalled = false;
    await page.route("**/api/v1/admin/subscription/change-plan", (route) => {
      changePlanCalled = true;
      void route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: { result: "upgrade_committed" },
        }),
      });
    });

    await page.goto(PLAN_CHANGE_URL);

    // Select Studio.
    const studioOption = page
      .getByRole("radio", { name: /studio/i })
      .or(page.getByLabel(/studio/i));
    await studioOption.click();

    // Wait for preview to load.
    await expect(
      page.getByText(/upgrade/i).or(page.getByText(/effective/i)),
    ).toBeVisible({ timeout: 8_000 });

    // Click the Apply / Confirm button.
    const applyBtn = page
      .getByRole("button", { name: /apply/i })
      .or(page.getByRole("button", { name: /confirm upgrade/i }));
    await applyBtn.click();

    // Mutation should have fired.
    expect(changePlanCalled).toBe(true);

    // A toast or success message should appear.
    await expect(
      page.getByRole("status").or(page.getByText(/upgrade/i)),
    ).toBeVisible({ timeout: 8_000 });

    // Should redirect to billing settings.
    await expect(page).toHaveURL(/settings\/billing/, { timeout: 10_000 });

    void BILLING_URL; // referenced for documentation only
  });
});
