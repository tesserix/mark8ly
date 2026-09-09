import { existsSync } from "node:fs";
import { test } from "@playwright/test";

/**
 * Screenshot walk of the customer /account/orders pages. Not self-contained:
 * it consumes a Playwright storageState another spec has to produce first.
 *
 * PRODUCER / CONSUMER COUPLING for tests/e2e/.audit/customer-state.json
 * (paths are relative to the CWD of the run — apps/admin — not to __dirname):
 *
 *   producers  purchase-journey.spec.ts:173
 *              delivery-timeline.spec.ts:110
 *   consumers  this spec
 *              purchase-journey.spec.ts:181, :313
 *              delivery-timeline.spec.ts:309
 *
 * mark8ly#834 SPLIT this coupling. full-flow.spec.ts was a third producer of
 * the same path; moving it to tests/operator/ repointed its write (and its
 * own read) at tests/operator/.audit/customer-state.json, so it no longer
 * feeds anything under tests/e2e/. That is intended — operator scripts drive
 * a live host and must not seed the local suite — but it means running the
 * operator full flow no longer un-skips this spec. Run purchase-journey or
 * delivery-timeline for that.
 *
 * Both producers need the full local stack plus real customer credentials,
 * so on a bare checkout this spec skips, and the skip is correct: a
 * screenshot walk with no session would capture the sign-in page and call it
 * an order list.
 */

const CUSTOMER_STATE_PATH = "tests/e2e/.audit/customer-state.json";

test("customer /account/orders list + detail", async ({ browser }) => {
  test.skip(
    !existsSync(CUSTOMER_STATE_PATH),
    `${CUSTOMER_STATE_PATH} not present — it is written by ` +
      `purchase-journey.spec.ts or delivery-timeline.spec.ts; run one of ` +
      `those first (full-flow.spec.ts no longer writes this path, see the ` +
      `header)`,
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
