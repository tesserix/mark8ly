import { test, expect } from "@playwright/test";

const STOREFRONT_URL = process.env.STOREFRONT_BASE_URL ?? "";

test("every storefront product has a valid detail page with loadable images", async ({ browser }) => {
  test.setTimeout(180_000);

  const ctx = await browser.newContext({ baseURL: STOREFRONT_URL, viewport: { width: 1440, height: 900 } });
  const page = await ctx.newPage();

  // List page — collect handles
  await page.goto("/products");
  await page.waitForLoadState("networkidle").catch(() => {});

  const handles = await page.$$eval("ul li a", (as) =>
    as
      .map((a) => (a as HTMLAnchorElement).getAttribute("href") || "")
      .filter((h) => h.startsWith("/products/"))
      .map((h) => h.replace("/products/", "")),
  );
  console.log(`[health] ${handles.length} handles`);
  expect(handles.length).toBeGreaterThan(0);

  const report: Array<{
    handle: string;
    title: string;
    price: string;
    imgCount: number;
    brokenImgs: string[];
    statusOk: boolean;
  }> = [];

  for (const h of handles) {
    const row: (typeof report)[number] = {
      handle: h,
      title: "",
      price: "",
      imgCount: 0,
      brokenImgs: [],
      statusOk: false,
    };

    const resp = await page.goto(`/products/${h}`);
    row.statusOk = resp?.ok() ?? false;
    await page.waitForLoadState("networkidle").catch(() => {});

    row.title = (await page.locator("h1").first().textContent().catch(() => ""))?.trim() ?? "";
    const priceNode = await page
      .locator("text=/₹|\\$|INR/")
      .first()
      .textContent()
      .catch(() => null);
    row.price = (priceNode ?? "").trim();

    // Collect image URLs inside <main> excluding the nav logo.
    const imgs = await page.$$eval("main img", (els) =>
      els.map((el) => (el as HTMLImageElement).currentSrc || (el as HTMLImageElement).src).filter(Boolean),
    );
    row.imgCount = imgs.length;

    // Probe each unique CDN URL with HEAD.
    const unique = Array.from(new Set(imgs));
    for (const u of unique) {
      const check = await page.request.get(u, { maxRedirects: 3 }).catch(() => null);
      if (!check || !check.ok()) row.brokenImgs.push(`${u} -> ${check?.status() ?? "no-resp"}`);
    }
    report.push(row);
  }

  console.log("[health report]", JSON.stringify(report, null, 2));

  await ctx.close();

  // Hard expectations
  for (const r of report) {
    expect(r.statusOk, `detail page ok for ${r.handle}`).toBe(true);
    expect(r.title.length, `title rendered for ${r.handle}`).toBeGreaterThan(0);
    expect(r.imgCount, `at least one image on ${r.handle}`).toBeGreaterThan(0);
    expect(r.brokenImgs, `all images load 2xx on ${r.handle}`).toEqual([]);
  }
});
