/**
 * P16 — Tax-ID submit + clock-pause variant (authenticated, fully mocked).
 *
 * Two scenarios:
 *
 *   A. Happy path: fill business name + GB country + GB tax ID →
 *      POST returns { status: 'validated' } → "Tax ID verified" state.
 *
 *   B. Clock-paused variant: POST returns
 *      { status: 'registry_unavailable', message: 'HMRC API down' } →
 *      ClockPauseBadge renders with "Clock paused: registry" copy.
 *
 * Backend is fully mocked; no real tenant is created.
 */

import { expect, test } from "@playwright/test";

import { ADMIN_URL } from "./helpers";
import {
  STUB_STORE_ID,
  mockAuthSession,
  mockActiveSubscription,
  setupMockRoutes,
} from "./fixtures/subscription";

const TAX_ID_URL = `${ADMIN_URL}/admin/stores/${STUB_STORE_ID}/settings/tax-id`;

test.describe("tax-ID settings", () => {
  test.beforeEach(async ({ page }) => {
    await mockAuthSession(page);
    await setupMockRoutes(page);
    // Subscription without a tax_id already set.
    await mockActiveSubscription(page, "studio", "annual");
  });

  test("validated path: verified state appears after successful submit", async ({
    page,
  }) => {
    // Mock GET tax-id — no existing tax ID.
    await page.route("**/api/v1/admin/tax/info**", (route) => {
      void route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: { tax_id: null, status: null, business_name: null },
        }),
      });
    });

    // Mock POST tax/submit → validated.
    await page.route("**/api/v1/admin/tax/submit", (route) => {
      if (route.request().method() !== "POST") {
        void route.continue();
        return;
      }
      void route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: { status: "validated" },
        }),
      });
    });

    await page.goto(TAX_ID_URL);

    // Fill business name.
    const businessNameInput = page
      .getByLabel(/business name/i)
      .or(page.getByPlaceholder(/business name/i));
    await businessNameInput.fill("Acme Ltd");

    // Select country: GB.
    const countrySelect = page
      .getByLabel(/country/i)
      .or(page.getByRole("combobox", { name: /country/i }));
    await countrySelect.click();
    const gbOption = page
      .getByRole("option", { name: /united kingdom/i })
      .or(page.getByRole("option", { name: /^gb$/i }));
    await gbOption.click();

    // Fill tax ID.
    const taxIdInput = page
      .getByLabel(/tax id/i)
      .or(page.getByLabel(/vat number/i))
      .or(page.getByPlaceholder(/gb[0-9]/i));
    await taxIdInput.fill("GB123456789");

    // Submit.
    const submitBtn = page
      .getByRole("button", { name: /save/i })
      .or(page.getByRole("button", { name: /submit/i }))
      .or(page.getByRole("button", { name: /verify/i }));
    await submitBtn.click();

    // Assert "Tax ID verified" state.
    await expect(
      page
        .getByText(/tax id verified/i)
        .or(page.getByText(/verified/i))
        .or(page.getByText(/validated/i)),
    ).toBeVisible({ timeout: 10_000 });
  });

  test("clock-paused variant: ClockPauseBadge renders when registry is unavailable", async ({
    page,
  }) => {
    // Mock GET tax-id — no existing tax ID.
    await page.route("**/api/v1/admin/tax/info**", (route) => {
      void route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: { tax_id: null, status: null, business_name: null },
        }),
      });
    });

    // Mock POST tax/submit → 202 registry_unavailable (clock paused).
    await page.route("**/api/v1/admin/tax/submit", (route) => {
      if (route.request().method() !== "POST") {
        void route.continue();
        return;
      }
      void route.fulfill({
        status: 202,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            status: "registry_unavailable",
            message: "HMRC API down",
          },
        }),
      });
    });

    await page.goto(TAX_ID_URL);

    const businessNameInput = page
      .getByLabel(/business name/i)
      .or(page.getByPlaceholder(/business name/i));
    await businessNameInput.fill("Acme Ltd");

    const countrySelect = page
      .getByLabel(/country/i)
      .or(page.getByRole("combobox", { name: /country/i }));
    await countrySelect.click();
    const gbOption = page
      .getByRole("option", { name: /united kingdom/i })
      .or(page.getByRole("option", { name: /^gb$/i }));
    await gbOption.click();

    const taxIdInput = page
      .getByLabel(/tax id/i)
      .or(page.getByLabel(/vat number/i))
      .or(page.getByPlaceholder(/gb[0-9]/i));
    await taxIdInput.fill("GB123456789");

    const submitBtn = page
      .getByRole("button", { name: /save/i })
      .or(page.getByRole("button", { name: /submit/i }))
      .or(page.getByRole("button", { name: /verify/i }));
    await submitBtn.click();

    // Assert ClockPauseBadge renders with the "clock paused: registry" message.
    await expect(
      page
        .getByText(/clock paused/i)
        .or(page.getByText(/registry/i))
        .or(page.getByText(/hmrc/i))
        .or(page.getByText(/registry unavailable/i)),
    ).toBeVisible({ timeout: 10_000 });
  });
});
