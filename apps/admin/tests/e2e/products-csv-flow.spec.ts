import { expect, test } from "@playwright/test";

import {
  completeOnboarding,
  seedProducts,
  signInAsOwner,
} from "./helpers";

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

// KNOWN RED (#858) — the SPEC is right and the APP is broken. CSV import
// does not work in production, and this suite is what found it:
//
//   1. apps/admin/lib/api/csvImports.ts:97 builds
//      /api/v1/admin/stores/{id}/products/csv-imports
//      marketplace-api serves
//      /api/v1/admin/stores/{id}/csv-imports          (no /products segment)
//      The GET therefore lands on /products/:id with id="csv-imports" and
//      500s on `invalid input syntax for type uuid`.
//
//   2. apps/admin/app/(admin)/products/import/page.tsx:67 passes the literal
//      string "__STORE_ID__" as the store id, so the POST 404s. The page is
//      a client component with no store id in scope — threading one in is a
//      product change, not a test fix.
//
// apps/admin/lib/api/csvImports.test.ts:71 asserts the WRONG url, so the
// unit suite has been locking the first bug in. That is the whole argument
// for this issue: green unit tests, a feature that has never worked.
//
// Do not "fix" this spec to match the broken behaviour.

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

    // A fresh tenant has no products; these assertions need rows.
    await seedProducts(request, details);

    const ctx = await signInAsOwner(browser, request, details);
    const page = await ctx.newPage();

    // Navigate to import page
    await page.goto(`/products/import`);
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

    // Preview should show headers.
    //
    // Scoped to the preview table's <th> cells (CsvPreviewTable.tsx:23): a
    // bare getByText("title") matched three elements — the column header,
    // and the column-mapping controls that name the same field — so strict
    // mode refused. Asserting the COLUMN HEADER is also the thing this step
    // actually cares about: that the uploaded file was parsed into columns.
    const previewHeaders = page.getByRole("columnheader");
    await expect(
      previewHeaders.filter({ hasText: /^title$/i }),
    ).toBeVisible({ timeout: 5_000 });
    await expect(previewHeaders.filter({ hasText: /^price$/i })).toBeVisible();

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

    // A fresh tenant has no products; these assertions need rows.
    await seedProducts(request, details);

    const ctx = await signInAsOwner(browser, request, details);
    const page = await ctx.newPage();

    // Navigate to products list
    await page.goto(`/products`);
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
