import { test } from "@playwright/test";

test("homepage with stock badges", async ({ browser }) => {
  const ctx = await browser.newContext({
    baseURL: process.env.STOREFRONT_BASE_URL ?? "",
    viewport: { width: 1440, height: 900 },
  });
  const page = await ctx.newPage();
  await page.goto("/");
  await page.waitForLoadState("networkidle").catch(() => {});
  await page.waitForTimeout(1500);
  await page.screenshot({ path: "tests/e2e/.audit/final-home-featured.png", fullPage: true });
  await ctx.close();
});
