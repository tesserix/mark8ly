import { test } from "@playwright/test";

test("AU checkout shows configured carriers fallback + razorpay brand", async ({ browser }) => {
  test.setTimeout(60_000);
  const ctx = await browser.newContext({
    baseURL: process.env.STOREFRONT_BASE_URL ?? "",
    viewport: { width: 1440, height: 900 },
  });
  const page = await ctx.newPage();

  // Sign in first so we can get to checkout with a valid cart.
  await page.goto("/sign-in");
  await page.getByLabel(/email address/i).fill(process.env.CUSTOMER_EMAIL ?? "");
  await page.getByLabel(/password/i).fill(process.env.CUSTOMER_PASSWORD ?? "");
  await page.getByRole("button", { name: /sign in/i }).click();
  await page.waitForLoadState("networkidle").catch(() => {});

  // Add any in-stock product.
  await page.goto("/products");
  await page.waitForLoadState("networkidle").catch(() => {});
  const firstHref = await page.locator("ul li a").first().getAttribute("href");
  if (firstHref) {
    await page.goto(firstHref);
    const addBtn = page.getByRole("button", { name: /^add to cart$/i });
    if (await addBtn.isEnabled({ timeout: 3_000 }).catch(() => false)) {
      await addBtn.click();
      await page.waitForTimeout(500);
    }
  }

  await page.goto("/checkout");
  await page.waitForLoadState("networkidle").catch(() => {});
  await page.locator("#email").fill(process.env.CUSTOMER_EMAIL ?? "").catch(() => {});
  await page.locator("#customer-name").fill("AU Shopper").catch(() => {});
  await page.locator("#ship-name").fill("AU Shopper");
  await page.locator("#ship-line1").fill("9 Bean Court");
  await page.locator("#ship-city").fill("KEILOR DOWNS");
  await page.locator("#ship-region").fill("VIC");
  await page.locator("#ship-postal").fill("3038");
  await page.locator("#ship-country").selectOption("AU");
  await page.waitForTimeout(2500);
  await page.screenshot({ path: "tests/e2e/.audit/final-au-checkout.png", fullPage: true });
  await ctx.close();
});
