import { expect, test } from "@playwright/test";

import {
  completeOnboarding,
  seedProducts,
  seedSecondStore,
  signInAsOwner,
} from "./helpers";

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

    // A fresh tenant has no products; these assertions need rows.
    await seedProducts(request, details);

    const ctx = await signInAsOwner(browser, request, details);
    const page = await ctx.newPage();

    // Navigate to products
    await page.goto(`/products`);
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

    // Verify status changed — filter by archived.
    // The status filter is a row of links now, not a <select>
    // (ProductsListFilters.tsx:83), so selectOption has nothing to drive.
    await page.getByRole("link", { name: "Archived", exact: true }).click();
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

    // A fresh tenant has no products; these assertions need rows.
    await seedProducts(request, details);
    // ...and exactly one store, so "copy to store" has no target without this.
    //
    // KNOWN RED (#858): seeding the second store makes the FIRST store's
    // admin host unreachable — the navigation dies with a bare
    // net::ERR_ABORTED. Isolated by removing this line: the goto then
    // succeeds and the test gets as far as the (targetless) copy dialog.
    //
    // So creating a store appears to move which host the merchant's admin
    // lives on. That is the same open product question stores-multi.spec.ts
    // raises from the other side ("does creating a store switch to it?"),
    // and it wants an answer before either spec is rewritten to match
    // whatever today's behaviour happens to be.
    await seedSecondStore(request, details);

    const ctx = await signInAsOwner(browser, request, details);
    const page = await ctx.newPage();

    // Navigate to products
    await page.goto(`/products`);

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

    // Success feedback is the dialog CLOSING — handleCopySubmit
    // (ProductsList.tsx:98) awaits the copy, closes, and clears the
    // selection. There is no toast.
    //
    // The old assertion waited for text matching /copied/i, which the
    // dialog's own static helper line ("Copied products will land as
    // drafts in the target store") satisfies before anything is copied —
    // so it proved nothing while the dialog was open and found nothing
    // once it closed. The real verification is the target-store check
    // below.
    await expect(
      page.getByText(/copy product to another store/i),
    ).toBeHidden({ timeout: 10_000 });

    // Switch to the target store and verify the product exists
    // This requires the store switcher UI — skip if not available
    const storeSwitcher = page.getByLabel(/switch store/i);
    if (await storeSwitcher.isVisible()) {
      await storeSwitcher.click();
      const storeOptions = page.getByRole("option");
      if ((await storeOptions.count()) > 1) {
        await storeOptions.nth(1).click();
        await page.goto(`/products`);
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
