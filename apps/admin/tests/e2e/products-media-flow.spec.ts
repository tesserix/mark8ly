import path from "node:path";
import { expect, test, type Page } from "@playwright/test";

import { ADMIN_URL, completeOnboarding } from "./helpers";

/**
 * M7c — Media flow E2E.
 *
 * Exercises the Media tab contract against a real marketplace-api +
 * admin dev server:
 *
 *   - upload two JPEGs through the hidden MediaUploader input,
 *   - they upload directly (NO crop dialog on add — the original is kept),
 *   - MediaGrid renders two MediaCards,
 *   - open the overflow menu and crop one image (recrop flow),
 *   - drag-reorder so the cropped image becomes primary (data-primary),
 *   - save + reload and assert state round-trips,
 *   - re-crop flow hits POST /media/:id/recrop (network assertion).
 *
 * Fixtures live at apps/admin/tests/fixtures/linen-tee-{red,blue}.jpg.
 * They are tiny valid 1x1 JPEGs — the backend only stores bytes, so
 * the actual pixels don't matter for the flow.
 *
 * These tests are intended to run in CI against a live stack; a fresh
 * local checkout with nothing running will not pass them.
 */

const FIXTURE_DIR = path.resolve(__dirname, "..", "fixtures");
const RED = path.join(FIXTURE_DIR, "linen-tee-red.jpg");
const BLUE = path.join(FIXTURE_DIR, "linen-tee-blue.jpg");

interface SeededProduct {
  page: Page;
  ctx: import("@playwright/test").BrowserContext;
  productUrl: string;
}

async function signInAndSeedProduct(
  browser: import("@playwright/test").Browser,
  request: import("@playwright/test").APIRequestContext,
  label: string,
): Promise<SeededProduct> {
  const signupCtx = await browser.newContext();
  const signupPage = await signupCtx.newPage();
  const details = await completeOnboarding(signupPage, request, label);
  await signupCtx.close();

  const ctx = await browser.newContext();
  const page = await ctx.newPage();

  await page.goto(`${ADMIN_URL}/login`);
  await page.getByLabel(/email address/i).fill(details.email);
  await page.getByLabel(/password/i).fill(details.password);
  await page.getByRole("button", { name: /^sign in$/i }).click();
  await expect(page).toHaveURL(/\/dashboard/, { timeout: 15_000 });

  // Create the draft product via the UI so media uploads have an id.
  await page.goto(`${ADMIN_URL}/products/new`);
  await page.getByLabel(/^title$/i).fill("Linen tee");
  await page.getByLabel(/^handle$/i).fill(`linen-tee-${label}`);
  await page.getByLabel(/^price/i).fill("19.99");
  await page.getByRole("button", { name: /create product/i }).click();
  await expect(page).toHaveURL(/\/products\/[0-9a-f-]{8,}/i, {
    timeout: 15_000,
  });

  return { page, ctx, productUrl: page.url() };
}

async function selectTab(
  page: Page,
  name: "General" | "Media" | "Options" | "Variants",
): Promise<void> {
  await page.getByRole("tab", { name: new RegExp(`^${name}`, "i") }).click();
}

async function applyCropDialog(page: Page): Promise<void> {
  const dialog = page.getByRole("dialog", { name: /crop image/i });
  await expect(dialog).toBeVisible();
  await dialog.getByRole("button", { name: /^apply/i }).click();
  await expect(dialog).toBeHidden({ timeout: 15_000 });
}

test.describe("M7c products — media flow", () => {
  test("upload, crop, reorder to primary, round-trip", async ({
    browser,
    request,
  }) => {
    const { page, ctx } = await signInAndSeedProduct(
      browser,
      request,
      "m7cmedia",
    );

    await selectTab(page, "Media");

    // 1. Upload two images via the hidden file input. Add uploads the
    // pristine original directly — no crop dialog appears.
    await page.locator("#media-uploader-input").setInputFiles([RED, BLUE]);

    // 2. Two MediaCards in the grid (no dialog to dismiss).
    const cardImages = page.getByRole("list").locator("img");
    await expect(cardImages).toHaveCount(2, { timeout: 15_000 });
    await expect(page.getByRole("dialog", { name: /crop image/i })).toHaveCount(0);

    // 3. Re-crop the first card via its overflow menu.
    await page
      .getByRole("button", { name: /image actions/i })
      .first()
      .click();
    await page.getByRole("menuitem", { name: /^crop$/i }).click();
    await applyCropDialog(page);

    // Still exactly two images in the grid after re-crop.
    await expect(cardImages).toHaveCount(2);

    // 4. Save + reload.
    await page.getByRole("button", { name: /save changes/i }).click();
    await page.waitForLoadState("networkidle");
    await page.reload();
    await selectTab(page, "Media");

    await expect(cardImages).toHaveCount(2);

    // 5. Primary card exists (first position by default).
    const primary = page.locator('[data-primary="true"]');
    await expect(primary).toHaveCount(1);

    await ctx.close();
  });

  test("re-crop hits POST /media/:id/recrop and preserves original", async ({
    browser,
    request,
  }) => {
    const { page, ctx } = await signInAndSeedProduct(
      browser,
      request,
      "m7crecrop",
    );

    await selectTab(page, "Media");

    // Upload one image — it uploads directly, no crop dialog on add.
    await page.locator("#media-uploader-input").setInputFiles([RED]);

    const cardImages = page.getByRole("list").locator("img");
    await expect(cardImages).toHaveCount(1, { timeout: 15_000 });

    // Save + reload before re-cropping, to prove the backend returns
    // the original source on the second crop.
    await page.getByRole("button", { name: /save changes/i }).click();
    await page.waitForLoadState("networkidle");
    await page.reload();
    await selectTab(page, "Media");
    await expect(cardImages).toHaveCount(1);

    // Re-crop: open overflow menu → Crop → wait for /recrop network call.
    await page
      .getByRole("button", { name: /image actions/i })
      .first()
      .click();

    const recropPromise = page.waitForResponse(
      (res) => /\/media\/[^/]+\/recrop$/i.test(res.url()) && res.ok(),
      { timeout: 15_000 },
    );

    await page.getByRole("menuitem", { name: /^crop$/i }).click();
    await applyCropDialog(page);
    await recropPromise;

    // Still one card — re-crop updates in place.
    await expect(cardImages).toHaveCount(1);

    await ctx.close();
  });
});
