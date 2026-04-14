import { test } from "@playwright/test";

const ADMIN_URL = process.env.ADMIN_BASE_URL ?? "";
const STOREFRONT_URL = process.env.STOREFRONT_BASE_URL ?? "";
const ADMIN_EMAIL = process.env.ADMIN_EMAIL ?? "";
const ADMIN_PASSWORD = process.env.ADMIN_PASSWORD ?? "";
const TENANT_NAME = process.env.TENANT_NAME ?? "";

test("audit images: admin vs storefront", async ({ browser }) => {
  test.setTimeout(120_000);

  // Storefront: open each visible product, capture whether it has <img>.
  const sfCtx = await browser.newContext({ baseURL: STOREFRONT_URL, viewport: { width: 1440, height: 900 } });
  const sfPage = await sfCtx.newPage();
  await sfPage.goto("/products");
  await sfPage.waitForLoadState("networkidle").catch(() => {});

  const handles = await sfPage.$$eval("ul li a", (as) =>
    as
      .map((a) => (a as HTMLAnchorElement).getAttribute("href") || "")
      .filter((h) => h.startsWith("/products/"))
      .map((h) => h.replace("/products/", "")),
  );
  console.log("[sf] handles:", handles);

  const report: { handle: string; imgCount: number; noImage: boolean; sample?: string }[] = [];
  for (const h of handles.slice(0, 5)) {
    await sfPage.goto(`/products/${h}`);
    await sfPage.waitForLoadState("networkidle").catch(() => {});
    const imgs = await sfPage.$$eval("main img", (els) => els.map((el) => (el as HTMLImageElement).src));
    const noImg = (await sfPage.textContent("main"))?.toLowerCase().includes("no image") ?? false;
    report.push({ handle: h, imgCount: imgs.length, noImage: noImg, sample: imgs[0] });
    await sfPage.screenshot({ path: `tests/e2e/.audit/img-sf-${h}.png`, fullPage: true });
  }
  console.log("[sf report]", JSON.stringify(report, null, 2));
  await sfCtx.close();

  // Admin: open the same products to see if media is stored.
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

  await adminPage.goto("/products");
  await adminPage.waitForLoadState("networkidle").catch(() => {});

  // For each row, follow the edit link and check if it has media
  const rows = await adminPage.locator("table tbody tr").all();
  const adminReport: { title: string; status: string; editHref: string }[] = [];
  for (const r of rows.slice(0, 15)) {
    const a = r.locator("a").first();
    const href = (await a.getAttribute("href")) ?? "";
    const title = ((await a.textContent()) ?? "").trim().split("/")[0] ?? "";
    const rowText = (await r.textContent()) ?? "";
    const status = /Active/.test(rowText) ? "active" : /Draft/.test(rowText) ? "draft" : "?";
    adminReport.push({ title, status, editHref: href });
  }
  console.log("[admin rows]", JSON.stringify(adminReport, null, 2));

  // Sample 3 Active products' edit pages
  const actives = adminReport.filter((r) => r.status === "active").slice(0, 3);
  for (const a of actives) {
    await adminPage.goto(a.editHref);
    await adminPage.waitForLoadState("networkidle").catch(() => {});
    const mediaCount = await adminPage.locator("img, [data-media-item]").count();
    const hasNoMediaText = ((await adminPage.textContent("main")) ?? "").toLowerCase().includes("no media");
    console.log(`[admin ${a.title}] mediaCount=${mediaCount} noMediaText=${hasNoMediaText}`);
    await adminPage.screenshot({ path: `tests/e2e/.audit/img-admin-${a.title.replace(/[^a-z0-9-]/gi, "_")}.png`, fullPage: true });
  }
  await adminCtx.close();
});
