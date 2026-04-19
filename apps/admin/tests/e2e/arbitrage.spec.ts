/**
 * P16 — Arbitrage flag + appeal flow (authenticated, fully mocked).
 *
 * Exercises:
 *   1. ArbitrageBanner renders when subscription has arbitrage_flag: true.
 *   2. Clicking Resolve navigates to /admin/settings/arbitrage.
 *   3. Filling and submitting the appeal form triggers POST /arbitrage-appeal.
 *   4. Success toast confirms submission.
 *
 * Backend is fully mocked via page.route().
 */

import { expect, test } from "@playwright/test";

import { ADMIN_URL } from "./helpers";
import {
  mockAuthSession,
  mockArbitrageFlag,
  setupMockRoutes,
} from "./fixtures/subscription";

const ADMIN_DASHBOARD_URL = `${ADMIN_URL}/admin`;
const ARBITRAGE_SETTINGS_URL = `${ADMIN_URL}/admin/settings/arbitrage`;

test.describe("arbitrage appeal flow", () => {
  test.beforeEach(async ({ page }) => {
    await mockAuthSession(page);
    await setupMockRoutes(page);
  });

  test("ArbitrageBanner appears on /admin when arbitrage_flag is true", async ({
    page,
  }) => {
    await mockArbitrageFlag(page, "material");

    await page.goto(ADMIN_DASHBOARD_URL);

    // Banner should be visible at the top of the page with the
    // discrepancy message.
    await expect(
      page
        .getByText(/discrepancy/i)
        .or(page.getByText(/we've noted a discrepancy/i))
        .or(page.getByText(/arbitrage/i)),
    ).toBeVisible({ timeout: 8_000 });
  });

  test("Resolve CTA navigates to arbitrage settings page", async ({ page }) => {
    await mockArbitrageFlag(page, "material");

    await page.goto(ADMIN_DASHBOARD_URL);

    // Find and click the Resolve / Review CTA on the banner.
    const resolveCta = page
      .getByRole("link", { name: /resolve/i })
      .or(page.getByRole("button", { name: /resolve/i }))
      .or(page.getByRole("link", { name: /review/i }));
    await resolveCta.click();

    await expect(page).toHaveURL(/\/settings\/arbitrage/, { timeout: 8_000 });
  });

  test("appeal form submits and shows success toast", async ({ page }) => {
    await mockArbitrageFlag(page, "material");

    // Mock POST /arbitrage-appeal → submitted.
    await page.route("**/api/v1/admin/arbitrage-appeal", (route) => {
      if (route.request().method() !== "POST") {
        void route.continue();
        return;
      }
      void route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: { status: "submitted" },
        }),
      });
    });

    await page.goto(ARBITRAGE_SETTINGS_URL);

    // Select jurisdiction: GB.
    const jurisdictionSelect = page
      .getByLabel(/jurisdiction/i)
      .or(page.getByRole("combobox", { name: /jurisdiction/i }));
    await jurisdictionSelect.click();
    const gbOption = page
      .getByRole("option", { name: /united kingdom/i })
      .or(page.getByRole("option", { name: /^gb$/i }));
    await gbOption.click();

    // Fill justification text.
    const justificationField = page
      .getByLabel(/justification/i)
      .or(page.getByLabel(/explanation/i))
      .or(page.getByRole("textbox", { name: /justification/i }));
    await justificationField.fill(
      "Our pricing aligns with UK market rates. We operate exclusively in the UK and our costs reflect local VAT and supplier pricing.",
    );

    // Submit the appeal.
    const submitBtn = page
      .getByRole("button", { name: /submit appeal/i })
      .or(page.getByRole("button", { name: /submit/i }));
    await submitBtn.click();

    // Success toast or confirmation should appear.
    await expect(
      page
        .getByText(/appeal submitted/i)
        .or(page.getByText(/submitted/i))
        .or(page.getByRole("status")),
    ).toBeVisible({ timeout: 10_000 });
  });
});
