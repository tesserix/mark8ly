import { existsSync } from "node:fs";
import { test } from "@playwright/test";

const CUSTOMER_STATE_PATH = "tests/e2e/.audit/customer-state.json";

test("customer /account/orders list + detail", async ({ browser }) => {
  test.skip(
    !existsSync(CUSTOMER_STATE_PATH),
    `${CUSTOMER_STATE_PATH} not present — no spec writes it, run one first`,
  );
  test.setTimeout(60_000);
  const ctx = await browser.newContext({
    baseURL: process.env.STOREFRONT_BASE_URL ?? "",
    storageState: CUSTOMER_STATE_PATH,
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
