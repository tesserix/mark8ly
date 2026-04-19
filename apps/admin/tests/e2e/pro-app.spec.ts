/**
 * P16 — Pro+App purchase flow + credential upload (authenticated, fully mocked).
 *
 * Two scenarios:
 *
 *   A. White-label app purchase:
 *      - Pro plan WITHOUT white_label_app_addon.
 *      - Navigate to pro-app-purchase page.
 *      - Assert "Add the White-label App" heading.
 *      - Check terms checkbox → submit enabled.
 *      - POST add-on → 201 with hosted_invoice_url.
 *      - Assert new tab/popup opens (Stripe-hosted invoice).
 *
 *   B. Credential upload (Apple):
 *      - Navigate to pro-app-purchase/credentials.
 *      - Fill Apple team ID + key ID + attach fake .p8 blob.
 *      - POST /app-credentials/apple → 204.
 *      - Assert "Uploaded" confirmation state.
 *
 * Backend is fully mocked via page.route().
 */

import { expect, test } from "@playwright/test";

import { ADMIN_URL } from "./helpers";
import {
  STUB_STORE_ID,
  mockAuthSession,
  mockProPlanWithAppAddOn,
  setupMockRoutes,
} from "./fixtures/subscription";

const PRO_APP_PURCHASE_URL = `${ADMIN_URL}/admin/stores/${STUB_STORE_ID}/settings/billing/pro-app-purchase`;
const PRO_APP_CREDENTIALS_URL = `${PRO_APP_PURCHASE_URL}/credentials`;

test.describe("Pro+App purchase flow", () => {
  test.beforeEach(async ({ page }) => {
    await mockAuthSession(page);
    await setupMockRoutes(page);
  });

  test("purchase page heading is visible for Pro plan without add-on", async ({
    page,
  }) => {
    await mockProPlanWithAppAddOn(page, { withAddOn: false });

    await page.goto(PRO_APP_PURCHASE_URL);

    await expect(
      page
        .getByRole("heading", { name: /add the white.?label app/i })
        .or(page.getByRole("heading", { name: /white.?label app/i })),
    ).toBeVisible({ timeout: 8_000 });
  });

  test("submit button is disabled until terms checkbox is checked", async ({
    page,
  }) => {
    await mockProPlanWithAppAddOn(page, { withAddOn: false });

    await page.goto(PRO_APP_PURCHASE_URL);

    // Submit should be disabled before terms are accepted.
    const submitBtn = page
      .getByRole("button", { name: /purchase/i })
      .or(page.getByRole("button", { name: /add app/i }))
      .or(page.getByRole("button", { name: /confirm/i }));

    await expect(submitBtn).toBeDisabled({ timeout: 8_000 });

    // Check the terms checkbox.
    const termsCheckbox = page
      .getByRole("checkbox", { name: /terms/i })
      .or(page.getByLabel(/i agree/i))
      .or(page.getByLabel(/terms/i));
    await termsCheckbox.check();

    // Submit should now be enabled.
    await expect(submitBtn).toBeEnabled({ timeout: 5_000 });
  });

  test("purchasing the add-on opens the Stripe invoice in a new tab", async ({
    page,
  }) => {
    await mockProPlanWithAppAddOn(page, { withAddOn: false });

    // Mock the add-on purchase endpoint.
    await page.route(
      "**/api/v1/admin/subscription/add-on/white-label-app",
      (route) => {
        if (route.request().method() !== "POST") {
          void route.continue();
          return;
        }
        void route.fulfill({
          status: 201,
          contentType: "application/json",
          body: JSON.stringify({
            data: {
              hosted_invoice_url: "https://invoice.stripe.com/test-addon-invoice",
              add_on: "white_label_app",
              status: "pending_payment",
            },
          }),
        });
      },
    );

    await page.goto(PRO_APP_PURCHASE_URL);

    // Accept terms.
    const termsCheckbox = page
      .getByRole("checkbox", { name: /terms/i })
      .or(page.getByLabel(/i agree/i))
      .or(page.getByLabel(/terms/i));
    await termsCheckbox.check();

    const submitBtn = page
      .getByRole("button", { name: /purchase/i })
      .or(page.getByRole("button", { name: /add app/i }))
      .or(page.getByRole("button", { name: /confirm/i }));
    await expect(submitBtn).toBeEnabled({ timeout: 5_000 });

    // Click submit and wait for a popup (new tab).
    const [popup] = await Promise.all([
      page.waitForEvent("popup", { timeout: 10_000 }),
      submitBtn.click(),
    ]);

    // The popup should open the Stripe invoice URL.
    await expect(popup).toHaveURL(/invoice\.stripe\.com/);
  });
});

test.describe("Pro+App credential upload", () => {
  test.beforeEach(async ({ page }) => {
    await mockAuthSession(page);
    await setupMockRoutes(page);
    // Already has the add-on (purchased); now configuring credentials.
    await mockProPlanWithAppAddOn(page, { withAddOn: true });
  });

  test("Apple credentials upload shows Uploaded confirmation state", async ({
    page,
  }) => {
    // Mock POST /app-credentials/apple → 204 No Content.
    await page.route("**/api/v1/admin/app-credentials/apple", (route) => {
      if (route.request().method() !== "POST") {
        void route.continue();
        return;
      }
      void route.fulfill({
        status: 204,
        body: "",
      });
    });

    await page.goto(PRO_APP_CREDENTIALS_URL);

    // Fill Apple-specific fields.
    const teamIdInput = page
      .getByLabel(/team id/i)
      .or(page.getByPlaceholder(/[a-z0-9]{10}/i));
    await teamIdInput.fill("ABCDE12345");

    const keyIdInput = page
      .getByLabel(/key id/i)
      .or(page.getByPlaceholder(/key id/i));
    await keyIdInput.fill("XYZKEY123A");

    // Attach a fake .p8 key file using Playwright's setInputFiles.
    const fileInput = page.locator('input[type="file"]').first();
    await fileInput.setInputFiles({
      name: "AuthKey_XYZKEY123A.p8",
      mimeType: "application/x-pem-file",
      buffer: Buffer.from(
        "-----BEGIN PRIVATE KEY-----\nfake_p8_key_content_for_testing\n-----END PRIVATE KEY-----",
      ),
    });

    // Submit the credentials form.
    const uploadBtn = page
      .getByRole("button", { name: /upload/i })
      .or(page.getByRole("button", { name: /save/i }))
      .or(page.getByRole("button", { name: /submit/i }));
    await uploadBtn.click();

    // Assert "Uploaded" or success confirmation state.
    await expect(
      page
        .getByText(/uploaded/i)
        .or(page.getByText(/credentials saved/i))
        .or(page.getByText(/apple credentials/i)),
    ).toBeVisible({ timeout: 10_000 });
  });
});
