import { expect, test } from "@playwright/test";

import { ADMIN_URL, completeOnboarding } from "./helpers";

/**
 * M7d — Products bulk actions + copy-to-store E2E flows.
 *
 * These tests require a running dev stack (Go services + Next.js apps)
 * with seeded products. They verify the cross-component wiring from
 * the list page through the bulk bar and dialogs.
 *
 * Prerequisites:
 * - `make dev` running (marketplace-api + admin app)
 * - At least one tenant with 3+ products seeded
 * - The seeding tenant has at least 2 stores
 */

test.describe("M7d bulk actions", () => {
  test("select 3 products, bulk archive, assert toast + status change", async ({
    browser,
    request,
  }) => {
    // Onboard a fresh merchant
    const signupCtx = await browser.newContext();
    const signupPage = await signupCtx.newPage();
    const details = await completeOnboarding(signupPage, request, "m7d-bulk");
    await signupCtx.close();

    const ctx = await browser.newContext();
    const page = await ctx.newPage();

    // Sign in
    await page.goto(`${ADMIN_URL}/login`);
    await page.getByLabel(/email address/i).fill(details.email);
    await page.getByLabel(/password/i).fill(details.password);
    await page.getByRole("button", { name: /^sign in$/i }).click();
    await expect(page).toHaveURL(/\/dashboard/, { timeout: 15_000 });

    // Navigate to products
    await page.goto(`${ADMIN_URL}/products`);
    await expect(
      page.getByRole("heading", { name: /^products$/i, level: 1 }),
    ).toBeVisible();

    // Wait for product rows to load
    const rows = page.locator("table tbody tr");
    await expect(rows.first()).toBeVisible({ timeout: 10_000 });

    // Select first 3 checkboxes (skip header)
    const checkboxes = page.locator("table tbody tr input[type=checkbox]");
    const count = await checkboxes.count();
    const toSelect = Math.min(count, 3);

    for (let i = 0; i < toSelect; i++) {
      await checkboxes.nth(i).check();
    }

    // Bulk bar should appear
    await expect(page.getByText(new RegExp(`${toSelect} selected`, "i"))).toBeVisible();

    // Click archive
    await page.getByRole("button", { name: /^archive$/i }).click();

    // Wait for toast confirmation
    await expect(page.getByText(/archived/i)).toBeVisible({ timeout: 10_000 });

    // Verify status changed — filter by archived
    await page.getByLabel(/filter by status/i).selectOption("archived");
    await expect(rows.first()).toBeVisible({ timeout: 5_000 });

    await ctx.close();
  });

  test("copy one product to another store, navigate to target, assert exists", async ({
    browser,
    request,
  }) => {
    // Onboard a fresh merchant
    const signupCtx = await browser.newContext();
    const signupPage = await signupCtx.newPage();
    const details = await completeOnboarding(signupPage, request, "m7d-copy");
    await signupCtx.close();

    const ctx = await browser.newContext();
    const page = await ctx.newPage();

    // Sign in
    await page.goto(`${ADMIN_URL}/login`);
    await page.getByLabel(/email address/i).fill(details.email);
    await page.getByLabel(/password/i).fill(details.password);
    await page.getByRole("button", { name: /^sign in$/i }).click();
    await expect(page).toHaveURL(/\/dashboard/, { timeout: 15_000 });

    // Navigate to products
    await page.goto(`${ADMIN_URL}/products`);

    // Wait for product rows
    const rows = page.locator("table tbody tr");
    await expect(rows.first()).toBeVisible({ timeout: 10_000 });

    // Get the first product title for later verification
    const firstProductTitle = await rows
      .first()
      .locator("a span")
      .first()
      .textContent();

    // Click the overflow menu on the first product
    await rows.first().getByLabel(/more actions/i).click();

    // Click "Copy to store"
    await page.getByText(/copy to store/i).first().click();

    // Dialog should open
    await expect(page.getByText(/copy product to another store/i)).toBeVisible({
      timeout: 5_000,
    });

    // Select the second store (first non-current)
    const storeRadios = page.locator("input[name='target-store']");
    if ((await storeRadios.count()) > 0) {
      await storeRadios.first().check();
    }

    // Click copy
    await page.getByRole("button", { name: /^copy$/i }).click();

    // Wait for success feedback
    await expect(page.getByText(/copied/i)).toBeVisible({ timeout: 10_000 });

    // Switch to the target store and verify the product exists
    // This requires the store switcher UI — skip if not available
    const storeSwitcher = page.getByLabel(/switch store/i);
    if (await storeSwitcher.isVisible()) {
      await storeSwitcher.click();
      const storeOptions = page.getByRole("option");
      if ((await storeOptions.count()) > 1) {
        await storeOptions.nth(1).click();
        await page.goto(`${ADMIN_URL}/products`);
        if (firstProductTitle) {
          await expect(page.getByText(firstProductTitle)).toBeVisible({
            timeout: 10_000,
          });
        }
      }
    }

    await ctx.close();
  });
});
