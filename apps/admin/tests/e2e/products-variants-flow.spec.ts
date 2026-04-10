import { expect, test, type Page } from "@playwright/test";

import { ADMIN_URL, completeOnboarding } from "./helpers";

/**
 * M7c — Variants flow E2E.
 *
 * Drives the real ProductForm through its four tabs (General, Media,
 * Options, Variants) to prove the end-to-end contract for:
 *
 *   - creating a draft product from the General tab,
 *   - adding two options with multiple values on the Options tab,
 *   - auto-deriving the cartesian variant grid,
 *   - bulk-editing prices via VariantBulkBar,
 *   - inline-editing a single variant SKU,
 *   - saving + reloading and asserting the state round-trips,
 *   - hitting the 500-variant guardrail with a tall option matrix.
 *
 * These tests are intended to run against a live marketplace-api and
 * admin dev server in CI. They are NOT expected to pass against a
 * fresh local checkout with nothing running — see the task brief.
 */

async function signInFreshAdmin(
  browser: import("@playwright/test").Browser,
  request: import("@playwright/test").APIRequestContext,
  label: string,
): Promise<{ page: Page; ctx: import("@playwright/test").BrowserContext }> {
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

  return { page, ctx };
}

async function selectTab(
  page: Page,
  name: "General" | "Media" | "Options" | "Variants",
): Promise<void> {
  await page.getByRole("tab", { name: new RegExp(`^${name}`, "i") }).click();
}

async function addOptionWithValues(
  page: Page,
  name: string,
  values: readonly string[],
): Promise<void> {
  await page.getByRole("button", { name: /\+ add option/i }).click();
  // The newly added OptionRow is always the last one — target it by its
  // "Option name" placeholder and fill the last match.
  const nameInputs = page.getByPlaceholder("Option name");
  await nameInputs.last().fill(name);

  const valueInputs = page.getByPlaceholder("Add a value");
  for (const v of values) {
    await valueInputs.last().fill(v);
    await valueInputs.last().press("Enter");
  }
}

test.describe("M7c products — variants flow", () => {
  test("create product, add options, bulk-edit variant prices, round-trip", async ({
    browser,
    request,
  }) => {
    const { page, ctx } = await signInFreshAdmin(browser, request, "m7c-var");

    // 1. Create a draft product from /products/new.
    await page.goto(`${ADMIN_URL}/products/new`);
    await expect(
      page.getByRole("heading", { name: /new product/i }),
    ).toBeVisible();

    await page.getByLabel(/^title$/i).fill("Linen tee");
    await page.getByLabel(/^handle$/i).fill("linen-tee");
    await page.getByLabel(/^status$/i).selectOption("draft");
    await page.getByLabel(/^price/i).fill("19.99");

    await page.getByRole("button", { name: /create product/i }).click();
    await expect(page).toHaveURL(/\/products\/[0-9a-f-]{8,}/i, {
      timeout: 15_000,
    });

    // 2. Options tab: add Color (Red, Blue) and Size (S, M, L).
    await selectTab(page, "Options");
    await addOptionWithValues(page, "Color", ["Red", "Blue"]);
    await addOptionWithValues(page, "Size", ["S", "M", "L"]);

    // 3. Variants tab: 2 x 3 = 6 rows.
    await selectTab(page, "Variants");
    const priceInputs = page.getByRole("textbox", { name: /^price$/i });
    await expect(priceInputs).toHaveCount(6);

    // 4. Bulk set all prices to 29.99 via VariantBulkBar.
    await page.getByRole("button", { name: /set all prices/i }).click();
    const bulkInput = page
      .getByRole("spinbutton")
      .or(page.locator('input[type="number"]'))
      .last();
    await bulkInput.fill("29.99");
    await page.getByRole("button", { name: /^apply$/i }).click();

    for (let i = 0; i < 6; i++) {
      await expect(priceInputs.nth(i)).toHaveValue("29.99");
    }

    // 5. Inline-edit the first variant's SKU.
    const skuInputs = page.getByRole("textbox", { name: /^sku$/i });
    await skuInputs.first().fill("R-S-001");
    await skuInputs.first().blur();

    // 6. Save.
    await page.getByRole("button", { name: /save changes/i }).click();

    // Give the save round-trip a beat before reloading.
    await page.waitForLoadState("networkidle");

    // 7. Reload and assert state round-trips.
    await page.reload();
    await selectTab(page, "Variants");

    const pricesAfter = page.getByRole("textbox", { name: /^price$/i });
    await expect(pricesAfter).toHaveCount(6);
    for (let i = 0; i < 6; i++) {
      await expect(pricesAfter.nth(i)).toHaveValue("29.99");
    }
    const skusAfter = page.getByRole("textbox", { name: /^sku$/i });
    await expect(skusAfter.first()).toHaveValue("R-S-001");

    // Options tab still lists Color + Size with their values.
    await selectTab(page, "Options");
    await expect(page.locator('input[value="Color"]')).toBeVisible();
    await expect(page.locator('input[value="Size"]')).toBeVisible();
    for (const v of ["Red", "Blue", "S", "M", "L"]) {
      await expect(page.getByText(v, { exact: true }).first()).toBeVisible();
    }

    await ctx.close();
  });

  test("500-variant guardrail surfaces a Too many variants error", async ({
    browser,
    request,
  }) => {
    const { page, ctx } = await signInFreshAdmin(browser, request, "m7c-cap");

    await page.goto(`${ADMIN_URL}/products/new`);
    await page.getByLabel(/^title$/i).fill("Big matrix");
    await page.getByLabel(/^handle$/i).fill("big-matrix");
    await page.getByLabel(/^price/i).fill("1.00");
    await page.getByRole("button", { name: /create product/i }).click();
    await expect(page).toHaveURL(/\/products\/[0-9a-f-]{8,}/i, {
      timeout: 15_000,
    });

    await selectTab(page, "Options");

    // 11 x 11 x 5 = 605 > 500 cap.
    const letters = ["a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k"];
    await addOptionWithValues(page, "Alpha", letters);
    await addOptionWithValues(page, "Beta", letters);
    await addOptionWithValues(page, "Gamma", ["s", "m", "l", "xl", "xxl"]);

    await selectTab(page, "Variants");
    await expect(page.getByText(/too many variants/i)).toBeVisible({
      timeout: 10_000,
    });

    await ctx.close();
  });
});
