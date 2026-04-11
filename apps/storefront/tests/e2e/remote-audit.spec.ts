import { test, expect, type ConsoleMessage, type Page, type Request } from "@playwright/test";
import { mkdirSync, writeFileSync } from "node:fs";
import { join } from "node:path";

/**
 * Remote audit — exploratory anonymous crawl of the storefront against
 * a live environment. Walks public routes, records console errors,
 * page errors, failed network requests, and broken images per route.
 *
 * Storefront sign-in is known broken, so this audit is strictly
 * anonymous — no auth attempted.
 *
 * Runs only when STOREFRONT_AUDIT=1. Findings are written to
 * tests/e2e/.audit/storefront-findings.json.
 */

const SHOULD_RUN = process.env.STOREFRONT_AUDIT === "1";
const BASE_URL =
  process.env.STOREFRONT_BASE_URL ?? "https://india-store.mark8ly.com";

const OUT_DIR = join(__dirname, ".audit");
const OUT_FILE = join(OUT_DIR, "storefront-findings.json");
const SCREENSHOT_DIR = join(OUT_DIR, "storefront-screens");

interface RouteFinding {
  route: string;
  finalUrl: string;
  httpStatus: number | null;
  loadMs: number;
  consoleErrors: string[];
  consoleWarnings: string[];
  pageErrors: string[];
  failedRequests: Array<{
    url: string;
    method: string;
    failure: string | null;
    status: number | null;
  }>;
  brokenImages: string[];
  screenshotPath: string | null;
  notes: string[];
}

// Static routes to always crawl. The audit also discovers category and
// product links from the home page and crawls the first few of each.
const STATIC_ROUTES: string[] = [
  "/",
  "/categories",
  "/cart",
  "/checkout",
  "/gift-cards",
  "/orders",
  "/account",
];

test.describe("storefront remote audit", () => {
  test.skip(!SHOULD_RUN, "set STOREFRONT_AUDIT=1 to run");
  test.describe.configure({ mode: "serial" });

  test("anonymous crawl of storefront", async ({ browser }) => {
    test.setTimeout(10 * 60 * 1000);

    mkdirSync(SCREENSHOT_DIR, { recursive: true });

    const context = await browser.newContext({
      baseURL: BASE_URL,
      viewport: { width: 1440, height: 900 },
    });
    const page = await context.newPage();

    const findings: RouteFinding[] = [];

    for (const route of STATIC_ROUTES) {
      findings.push(await auditRoute(page, route));
    }

    // Discover a few category and product links from home.
    await page.goto(BASE_URL, { waitUntil: "domcontentloaded" });
    const discoveredCategories = await page
      .$$eval('a[href^="/categories/"]', (as) =>
        Array.from(new Set(as.map((a) => (a as HTMLAnchorElement).getAttribute("href") || ""))).filter(Boolean),
      )
      .catch(() => [] as string[]);
    const discoveredProducts = await page
      .$$eval('a[href^="/products/"]', (as) =>
        Array.from(new Set(as.map((a) => (a as HTMLAnchorElement).getAttribute("href") || ""))).filter(Boolean),
      )
      .catch(() => [] as string[]);

    for (const href of discoveredCategories.slice(0, 3)) {
      findings.push(await auditRoute(page, href));
    }
    for (const href of discoveredProducts.slice(0, 5)) {
      findings.push(await auditRoute(page, href));
    }

    // Try a guest "add to cart" on the first discovered product, then
    // revisit /cart.
    if (discoveredProducts[0]) {
      try {
        await page.goto(`${BASE_URL}${discoveredProducts[0]}`, {
          waitUntil: "domcontentloaded",
        });
        const addBtn = page.getByRole("button", { name: /add to cart/i }).first();
        if (await addBtn.isVisible({ timeout: 3000 }).catch(() => false)) {
          await addBtn.click();
          findings.push(await auditRoute(page, "/cart (after add-to-cart)"));
        }
      } catch (err) {
        findings.push({
          route: "(add-to-cart)",
          finalUrl: page.url(),
          httpStatus: null,
          loadMs: 0,
          consoleErrors: [],
          consoleWarnings: [],
          pageErrors: [],
          failedRequests: [],
          brokenImages: [],
          screenshotPath: null,
          notes: [`add-to-cart threw: ${(err as Error).message}`],
        });
      }
    }

    writeFileSync(OUT_FILE, JSON.stringify(findings, null, 2));
    await context.close();
    expect(findings.length).toBeGreaterThan(0);
  });

  async function auditRoute(page: Page, route: string): Promise<RouteFinding> {
    const consoleErrors: string[] = [];
    const consoleWarnings: string[] = [];
    const pageErrors: string[] = [];
    const failedRequests: RouteFinding["failedRequests"] = [];
    let httpStatus: number | null = null;

    const onConsole = (msg: ConsoleMessage) => {
      const type = msg.type();
      const text = msg.text();
      if (/Download the React DevTools/i.test(text)) return;
      if (type === "error") consoleErrors.push(text);
      else if (type === "warning") consoleWarnings.push(text);
    };
    const onPageError = (err: Error) => {
      pageErrors.push(`${err.name}: ${err.message}`);
    };
    const onRequestFailed = (req: Request) => {
      failedRequests.push({
        url: req.url(),
        method: req.method(),
        failure: req.failure()?.errorText ?? null,
        status: null,
      });
    };
    const onResponse = (res: import("@playwright/test").Response) => {
      const bareRoute = route.split(" ")[0];
      if (res.url() === `${BASE_URL}${bareRoute}` || res.url() === `${BASE_URL}${bareRoute}/`) {
        httpStatus = res.status();
      }
      const status = res.status();
      if (status >= 400 && !res.url().includes("/_next/") && res.url().startsWith(BASE_URL)) {
        failedRequests.push({
          url: res.url(),
          method: res.request().method(),
          failure: null,
          status,
        });
      }
    };

    page.on("console", onConsole);
    page.on("pageerror", onPageError);
    page.on("requestfailed", onRequestFailed);
    page.on("response", onResponse);

    const start = Date.now();
    const notes: string[] = [];
    try {
      const bareRoute = route.split(" ")[0];
      await page.goto(`${BASE_URL}${bareRoute}`, {
        waitUntil: "domcontentloaded",
        timeout: 30_000,
      });
      await page.waitForLoadState("networkidle", { timeout: 15_000 }).catch(() => null);
    } catch (err) {
      notes.push(`Navigation threw: ${(err as Error).message}`);
    }
    const loadMs = Date.now() - start;

    // Broken image detection
    const brokenImages = await page
      .$$eval("img", (imgs) =>
        imgs
          .filter((img) => {
            const i = img as HTMLImageElement;
            return i.complete && (i.naturalWidth === 0 || i.naturalHeight === 0);
          })
          .map((img) => (img as HTMLImageElement).src),
      )
      .catch(() => [] as string[]);

    page.off("console", onConsole);
    page.off("pageerror", onPageError);
    page.off("requestfailed", onRequestFailed);
    page.off("response", onResponse);

    const shotName = `${route.replace(/[^a-z0-9]+/gi, "_").replace(/^_|_$/g, "") || "root"}.png`;
    const screenshotPath = join(SCREENSHOT_DIR, shotName);
    try {
      await page.screenshot({ path: screenshotPath, fullPage: true });
    } catch (err) {
      notes.push(`Screenshot failed: ${(err as Error).message}`);
    }

    return {
      route,
      finalUrl: page.url(),
      httpStatus,
      loadMs,
      consoleErrors,
      consoleWarnings,
      pageErrors,
      failedRequests,
      brokenImages,
      screenshotPath,
      notes,
    };
  }
});
