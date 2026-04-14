import { test } from "@playwright/test";

test("customer /account/orders list + detail", async ({ browser }) => {
  test.setTimeout(60_000);
  const ctx = await browser.newContext({
    baseURL: process.env.STOREFRONT_BASE_URL ?? "",
    storageState: "tests/e2e/.audit/customer-state.json",
    viewport: { width: 1440, height: 900 },
  });
  const page = await ctx.newPage();

  await page.goto("/account/orders");
  await page.waitForLoadState("networkidle").catch(() => {});
  await page.screenshot({ path: "tests/e2e/.audit/final-account-orders.png", fullPage: true });

  const firstLink = await page.locator('a[href^="/account/orders/"]').first().getAttribute("href");
  console.log("[account] first order link:", firstLink);
  if (firstLink) {
    await page.goto(firstLink);
    await page.waitForLoadState("networkidle").catch(() => {});
    await page.screenshot({ path: "tests/e2e/.audit/final-account-order-detail.png", fullPage: true });
  }
  await ctx.close();
});
