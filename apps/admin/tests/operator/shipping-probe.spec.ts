import { test } from "@playwright/test";

test("probe shipping config + shipments", async ({ browser }) => {
  test.setTimeout(60_000);
  const ctx = await browser.newContext({ baseURL: process.env.ADMIN_BASE_URL ?? "", viewport: { width: 1440, height: 900 } });
  const page = await ctx.newPage();
  await page.goto("/login");
  await page.getByLabel(/email address/i).fill(process.env.ADMIN_EMAIL ?? "");
  await page.getByLabel(/password/i).fill(process.env.ADMIN_PASSWORD ?? "");
  await Promise.all([
    page.waitForURL(/\/(dashboard|pick-tenant)/, { timeout: 30_000 }).catch(() => {}),
    page.getByRole("button", { name: /^sign in$/i }).click(),
  ]);
  if (page.url().includes("/pick-tenant")) {
    await page.getByText(new RegExp(process.env.TENANT_NAME ?? "", "i")).first().click();
    await page.waitForURL(/\/dashboard/, { timeout: 15_000 }).catch(() => {});
  }

  // Directly hit the backend via the admin's BFF session; let's see
  // exactly what row the carrier config has. The admin's Next.js
  // middleware injects x-session-* headers for same-origin fetches.
  await page.goto("/settings/shipping");
  await page.waitForLoadState("networkidle").catch(() => {});
  const html = await page.content();
  const storeMatch = html.match(/"id":"([0-9a-f-]{36})"/);
  console.log("[probe] storeId candidate:", storeMatch?.[1]);

  await page.waitForTimeout(500);
  // Click Edit on Delhivery to open the form — its prefilled state
  // tells us what the backend thinks is_active + warehouse fields are.
  const card = page.locator("article").filter({ hasText: /^Delhivery/ }).first();
  const editBtn = card.getByRole("button", { name: /^(Add credentials|Edit)$/ });
  await editBtn.click();
  const activeChecked = await card.getByRole("checkbox", { name: /active/i }).isChecked();
  const whPostal = await page.locator("#delhivery-wh-postal").inputValue().catch(() => "");
  const whCity = await page.locator("#delhivery-wh-city").inputValue().catch(() => "");
  const whCountry = await page.locator("#delhivery-wh-country").inputValue().catch(() => "");
  console.log(`[probe] is_active=${activeChecked} wh.city="${whCity}" wh.postal="${whPostal}" wh.country="${whCountry}"`);

  await ctx.close();
});
