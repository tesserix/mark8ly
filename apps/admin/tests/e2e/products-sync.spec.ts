import { test, expect } from "@playwright/test";

const ADMIN_URL = process.env.ADMIN_BASE_URL ?? "";
const STOREFRONT_URL = process.env.STOREFRONT_BASE_URL ?? "";
const ADMIN_EMAIL = process.env.ADMIN_EMAIL ?? "";
const ADMIN_PASSWORD = process.env.ADMIN_PASSWORD ?? "";
const TENANT_NAME = process.env.TENANT_NAME ?? "";

test.describe.configure({ mode: "serial" });

test("admin products → storefront parity", async ({ browser }) => {
  test.setTimeout(120_000);

  // 1. Admin sign-in
  const adminCtx = await browser.newContext({ baseURL: ADMIN_URL, viewport: { width: 1440, height: 900 } });
  const adminPage = await adminCtx.newPage();
  await adminPage.goto("/login");
  await adminPage.getByLabel(/email address/i).fill(ADMIN_EMAIL);
  await adminPage.getByLabel(/password/i).fill(ADMIN_PASSWORD);
  await Promise.all([
    adminPage.waitForURL(/\/(dashboard|pick-tenant)/, { timeout: 30_000 }).catch(() => {}),
    adminPage.getByRole("button", { name: /^sign in$/i }).click(),
  ]);
  if (adminPage.url().includes("/pick-tenant")) {
    await adminPage.getByText(new RegExp(TENANT_NAME, "i")).first().click();
    await adminPage.waitForURL(/\/dashboard/, { timeout: 15_000 }).catch(() => {});
  }

  // 2. Read admin product list
  await adminPage.goto("/products");
  await adminPage.waitForLoadState("networkidle").catch(() => {});
  await adminPage.screenshot({ path: "tests/e2e/.audit/sync-01-admin-products.png", fullPage: true });

  // Pull product titles + status per row. The title is the link text in the first cell;
  // status lives as its own <td> with the word "Active" or "Draft".
  const adminRows = await adminPage.locator("table tbody tr").all();
  const adminProducts: { title: string; handle: string; status: string }[] = [];
  for (const row of adminRows) {
    const link = row.locator("a").first();
    const title = (await link.textContent().catch(() => ""))?.trim() ?? "";
    const href = (await link.getAttribute("href").catch(() => "")) ?? "";
    const rowText = (await row.textContent().catch(() => ""))?.trim() ?? "";
    // Exact-word match — "Active" wins over "Draft" only if the row actually has Active
    const status = /\bActive\b/.test(rowText)
      ? "active"
      : /\bDraft\b/.test(rowText)
        ? "draft"
        : "unknown";
    if (title) adminProducts.push({ title, handle: href.split("/").pop() ?? "", status });
  }
  console.log("[admin] rows found:", adminProducts.length);
  console.log("[admin] products:", JSON.stringify(adminProducts, null, 2));

  await adminCtx.close();

  // 3. Storefront product list (public)
  const sfCtx = await browser.newContext({ baseURL: STOREFRONT_URL, viewport: { width: 1440, height: 900 } });
  const sfPage = await sfCtx.newPage();
  await sfPage.goto("/products");
  await sfPage.waitForLoadState("networkidle").catch(() => {});
  await sfPage.screenshot({ path: "tests/e2e/.audit/sync-02-storefront-products.png", fullPage: true });

  const sfBody = (await sfPage.textContent("body")) ?? "";
  const sfCount = (sfBody.match(/(\d+)\s+products?/i) ?? [])[1];
  console.log("[storefront] count header:", sfCount);
  console.log("[storefront] empty?:", /nothing in the shop yet|no products yet/i.test(sfBody));

  // Titles visible on the storefront grid
  const cards = await sfPage.locator("ul li a h2").allTextContents();
  console.log("[storefront] visible product titles:", cards);

  await sfCtx.close();

  // 4. Assertion — every Active admin product appears on storefront; drafts do NOT.
  const activeTitles = adminProducts.filter((p) => p.status === "active").map((p) => p.title);
  const draftTitles = adminProducts.filter((p) => p.status === "draft").map((p) => p.title);
  const sfLower = cards.map((c) => c.toLowerCase());

  console.log("[compare] admin-active count:", activeTitles.length);
  console.log("[compare] storefront card count:", cards.length);

  const missing: string[] = [];
  for (const t of activeTitles) {
    if (!sfLower.includes(t.toLowerCase())) missing.push(t);
  }
  const leaked: string[] = [];
  for (const t of draftTitles) {
    if (sfLower.includes(t.toLowerCase())) leaked.push(t);
  }

  console.log("[compare] active NOT on storefront:", missing);
  console.log("[compare] draft LEAKED to storefront:", leaked);

  expect(missing, "Active admin products missing from storefront").toEqual([]);
  expect(leaked, "Draft admin products incorrectly visible on storefront").toEqual([]);
});
