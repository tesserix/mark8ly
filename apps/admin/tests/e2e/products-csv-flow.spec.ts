import { expect, test } from "@playwright/test";

import { ADMIN_URL, completeOnboarding } from "./helpers";

/**
 * M7e — CSV import/export E2E flows.
 *
 * These tests require a running dev stack (Go services + Next.js apps)
 * with seeded products. They verify the full CSV import lifecycle and
 * CSV export download trigger.
 *
 * Prerequisites:
 * - `make dev` running (marketplace-api + admin app)
 * - At least one tenant with products seeded
 */

test.describe("M7e CSV import flow", () => {
  test("upload 10-row CSV, redirect to job page, see progress, reach completion", async ({
    browser,
    request,
  }) => {
    // Onboard a fresh merchant
    const signupCtx = await browser.newContext();
    const signupPage = await signupCtx.newPage();
    const details = await completeOnboarding(signupPage, request, "m7e-import");
    await signupCtx.close();

    const ctx = await browser.newContext();
    const page = await ctx.newPage();

    // Sign in
    await page.goto(`${ADMIN_URL}/login`);
    await page.getByLabel(/email address/i).fill(details.email);
    await page.getByLabel(/password/i).fill(details.password);
    await page.getByRole("button", { name: /^sign in$/i }).click();
    await expect(page).toHaveURL(/\/dashboard/, { timeout: 15_000 });

    // Navigate to import page
    await page.goto(`${ADMIN_URL}/products/import`);
    await expect(
      page.getByRole("heading", { name: /import products/i }),
    ).toBeVisible();

    // Build a 10-row CSV in memory and upload
    const csvContent = [
      "title,price,sku,status,stock",
      ...Array.from({ length: 10 }, (_, i) =>
        `Product ${i + 1},${(9.99 + i).toFixed(2)},SKU-${i + 1},draft,${i * 10}`,
      ),
    ].join("\n");

    // Create a temporary file via the file chooser
    const fileChooserPromise = page.waitForEvent("filechooser");
    await page.getByRole("button", { name: /upload csv/i }).click();
    const fileChooser = await fileChooserPromise;
    await fileChooser.setFiles({
      name: "test-products.csv",
      mimeType: "text/csv",
      buffer: Buffer.from(csvContent),
    });

    // Preview should show headers
    await expect(page.getByText("title")).toBeVisible({ timeout: 5_000 });
    await expect(page.getByText("price")).toBeVisible();

    // Submit import
    await page.getByRole("button", { name: /start import/i }).click();

    // Should redirect to the job status page
    await expect(page).toHaveURL(/\/products\/import\/[a-f0-9-]+/, {
      timeout: 10_000,
    });

    // Progress UI should appear
    await expect(page.getByRole("progressbar")).toBeVisible({ timeout: 5_000 });

    // Wait for completion (up to 30s — the worker processes 10 rows quickly)
    await expect(page.getByText("completed")).toBeVisible({ timeout: 30_000 });

    await ctx.close();
  });
});

test.describe("M7e CSV export flow", () => {
  test("select 3 products, click Export, assert CSV download triggers", async ({
    browser,
    request,
  }) => {
    // Onboard a fresh merchant
    const signupCtx = await browser.newContext();
    const signupPage = await signupCtx.newPage();
    const details = await completeOnboarding(signupPage, request, "m7e-export");
    await signupCtx.close();

    const ctx = await browser.newContext();
    const page = await ctx.newPage();

    // Sign in
    await page.goto(`${ADMIN_URL}/login`);
    await page.getByLabel(/email address/i).fill(details.email);
    await page.getByLabel(/password/i).fill(details.password);
    await page.getByRole("button", { name: /^sign in$/i }).click();
    await expect(page).toHaveURL(/\/dashboard/, { timeout: 15_000 });

    // Navigate to products list
    await page.goto(`${ADMIN_URL}/products`);
    await expect(
      page.getByRole("heading", { name: /^products$/i, level: 1 }),
    ).toBeVisible();

    // Wait for product rows to load
    const rows = page.locator("table tbody tr");
    await expect(rows.first()).toBeVisible({ timeout: 10_000 });

    // Select first 3 checkboxes
    const checkboxes = page.locator("table tbody tr input[type=checkbox]");
    const count = await checkboxes.count();
    const toSelect = Math.min(3, count);
    for (let i = 0; i < toSelect; i++) {
      await checkboxes.nth(i).check();
    }

    // Click "Export" in the bulk actions bar
    const downloadPromise = page.waitForEvent("download");
    await page.getByRole("button", { name: /^export$/i }).click();
    const download = await downloadPromise;

    // Verify the download is a CSV
    expect(download.suggestedFilename()).toMatch(/\.csv$/);

    await ctx.close();
  });
});
