import { test } from "@playwright/test";

const STOREFRONT_URL = process.env.STOREFRONT_BASE_URL ?? "";
// No default: the previous hardcoded handle was written by one past
// seeding run and nothing else in the repo produces or references it.
const PRODUCT_HANDLE = process.env.STOREFRONT_FINAL_PRODUCT_HANDLE ?? "";

test("storefront after seeding", async ({ browser }) => {
  test.skip(!PRODUCT_HANDLE, "STOREFRONT_FINAL_PRODUCT_HANDLE not set");
  test.setTimeout(60_000);
  const ctx = await browser.newContext({ baseURL: STOREFRONT_URL, viewport: { width: 1440, height: 900 } });
  const page = await ctx.newPage();

  await page.goto("/products");
  await page.waitForLoadState("networkidle").catch(() => {});
  await page.waitForTimeout(2000); // allow images to paint
  await page.screenshot({ path: "tests/e2e/.audit/final-storefront-grid.png", fullPage: true });

  await page.goto(`/products/${PRODUCT_HANDLE}`);
  await page.waitForLoadState("networkidle").catch(() => {});
  await page.waitForTimeout(2000);
  await page.screenshot({ path: "tests/e2e/.audit/final-storefront-detail.png", fullPage: true });

  await ctx.close();
});
