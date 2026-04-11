import { test, expect, type ConsoleMessage, type Page, type Request } from "@playwright/test";
import { mkdirSync, writeFileSync } from "node:fs";
import { join } from "node:path";

/**
 * Remote audit — exploratory crawl of the admin app against a live
 * environment. Signs in once with real credentials, walks every
 * top-level admin route, and records any console errors, page errors,
 * and failed network requests per route.
 *
 * This spec is intentionally NOT part of the local e2e suite. It only
 * runs when ADMIN_AUDIT=1, so `npx playwright test` against localhost
 * won't accidentally fire real credentials at production.
 *
 * Findings are written to tests/e2e/.audit/admin-findings.json.
 */

const SHOULD_RUN = process.env.ADMIN_AUDIT === "1";
const BASE_URL =
  process.env.ADMIN_BASE_URL ?? "https://india-store-admin.mark8ly.com";
// No fallback values — credentials must come from the environment.
// Running the audit against a real tenant without these set is a
// configuration error, not a default-to-this-one situation.
const EMAIL = process.env.ADMIN_AUDIT_EMAIL ?? "";
const PASSWORD = process.env.ADMIN_AUDIT_PASSWORD ?? "";

const OUT_DIR = join(__dirname, ".audit");
const OUT_FILE = join(OUT_DIR, "admin-findings.json");
const SCREENSHOT_DIR = join(OUT_DIR, "admin-screens");

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
  unauthorizedRedirect: boolean;
  screenshotPath: string | null;
  notes: string[];
}

const ROUTES: string[] = [
  "/dashboard",
  "/products",
  "/products/new",
  "/products/import",
  "/orders",
  "/customers",
  "/marketing",
  "/support",
  "/settings",
  "/settings/account",
  "/settings/general",
  "/settings/stores",
  "/settings/team",
  "/settings/domains",
  "/settings/themes",
  "/settings/payments",
  "/settings/shipping",
  "/settings/tax",
  "/settings/notifications",
  "/settings/subscription",
  "/settings/audit-logs",
];

test.describe("admin remote audit", () => {
  test.skip(!SHOULD_RUN, "set ADMIN_AUDIT=1 to run");
  test.describe.configure({ mode: "serial" });

  const findings: RouteFinding[] = [];

  test("sign in + crawl all admin routes", async ({ browser }) => {
    test.setTimeout(10 * 60 * 1000); // 10 min for the full crawl

    mkdirSync(SCREENSHOT_DIR, { recursive: true });

    const context = await browser.newContext({
      baseURL: BASE_URL,
      viewport: { width: 1440, height: 900 },
    });
    const page = await context.newPage();

    // --- Step 1: sign in ---
    // NOTE: The login page has a known race — the RSC prefetch to
    // admin.mark8ly.com fails CORS and the form can re-mount, wiping
    // whatever was typed. We go straight to /login, wait for the
    // network to settle, then fill + click in quick succession and
    // refill if the values got wiped.
    const signinFinding = await auditRoute(
      page,
      "/login (sign-in)",
      async () => {
        await page.goto(`${BASE_URL}/login`, { waitUntil: "domcontentloaded" });
        await page.waitForLoadState("networkidle", { timeout: 10_000 }).catch(() => null);

        const emailInput = page.getByLabel(/email address/i);
        const passwordInput = page.getByLabel(/password/i);
        const signInBtn = page.getByRole("button", { name: /^sign in$/i });

        // Two-pass fill to survive any re-mount between keystrokes.
        for (let attempt = 0; attempt < 3; attempt++) {
          await emailInput.fill("");
          await emailInput.fill(EMAIL);
          await passwordInput.fill("");
          await passwordInput.fill(PASSWORD);
          const emailVal = await emailInput.inputValue();
          const pwVal = await passwordInput.inputValue();
          if (emailVal === EMAIL && pwVal === PASSWORD) break;
          await page.waitForTimeout(500);
        }

        await Promise.all([
          page
            .waitForURL(/\/(dashboard|pick-tenant)/, { timeout: 45_000 })
            .catch(() => null),
          signInBtn.click(),
        ]);
      },
      "signin",
    );
    findings.push(signinFinding);

    // If we landed on pick-tenant, pick the "india store" tenant.
    // On india-store-admin.mark8ly.com it's arguably a bug that we
    // have to pick at all — the subdomain implies the tenant — but
    // for now the audit just clicks through.
    if (page.url().includes("/pick-tenant")) {
      const clicked = await clickTenantRow(page, /india store/i);
      if (!clicked) {
        findings.push({
          route: "/pick-tenant",
          finalUrl: page.url(),
          httpStatus: null,
          loadMs: 0,
          consoleErrors: [],
          consoleWarnings: [],
          pageErrors: [],
          failedRequests: [],
          unauthorizedRedirect: false,
          screenshotPath: null,
          notes: ["Could not click india store row on /pick-tenant"],
        });
      } else {
        await page.waitForURL(/\/dashboard/, { timeout: 20_000 }).catch(() => null);
      }
    }

    const afterSigninUrl = page.url();
    if (!afterSigninUrl.includes("/dashboard")) {
      findings.push({
        route: "post-signin",
        finalUrl: afterSigninUrl,
        httpStatus: null,
        loadMs: 0,
        consoleErrors: [],
        consoleWarnings: [],
        pageErrors: [],
        failedRequests: [],
        unauthorizedRedirect: false,
        screenshotPath: null,
        notes: [
          `Sign-in did not land on /dashboard (ended at ${afterSigninUrl}). Skipping authenticated route crawl.`,
        ],
      });
      writeFileSync(OUT_FILE, JSON.stringify(findings, null, 2));
      await context.close();
      // Do not fail — this is an audit, not a pass/fail test.
      return;
    }

    // --- Step 2: crawl every route ---
    for (const route of ROUTES) {
      const f = await auditRoute(page, route, async () => {
        await page.goto(`${BASE_URL}${route}`, {
          waitUntil: "domcontentloaded",
          timeout: 30_000,
        });
        // Give client-side data fetches a moment to run and emit errors.
        await page.waitForLoadState("networkidle", { timeout: 15_000 }).catch(() => null);
      });
      findings.push(f);
    }

    writeFileSync(OUT_FILE, JSON.stringify(findings, null, 2));
    await context.close();

    // Always pass — findings are the artifact.
    expect(findings.length).toBeGreaterThan(0);
  });

  async function clickTenantRow(page: Page, name: RegExp): Promise<boolean> {
    // Pick-tenant rows don't have a clear role — try the heading text,
    // walk up to the nearest clickable ancestor, and click it.
    const label = page.getByText(name).first();
    try {
      await label.waitFor({ state: "visible", timeout: 5000 });
    } catch {
      return false;
    }
    // Try clicking the label itself first.
    try {
      await label.click({ timeout: 2000 });
      return true;
    } catch {
      // Walk up to a button/link/role ancestor.
      const handle = await label.elementHandle();
      if (!handle) return false;
      const clicked = await page.evaluate((el) => {
        let cur: HTMLElement | null = el as HTMLElement;
        while (cur) {
          if (
            cur.tagName === "BUTTON" ||
            cur.tagName === "A" ||
            cur.getAttribute("role") === "button" ||
            cur.getAttribute("role") === "link" ||
            cur.onclick !== null
          ) {
            cur.click();
            return true;
          }
          cur = cur.parentElement;
        }
        return false;
      }, handle);
      return clicked;
    }
  }

  async function auditRoute(
    page: Page,
    route: string,
    navigate: () => Promise<void>,
    slug?: string,
  ): Promise<RouteFinding> {
    const consoleErrors: string[] = [];
    const consoleWarnings: string[] = [];
    const pageErrors: string[] = [];
    const failedRequests: RouteFinding["failedRequests"] = [];
    let httpStatus: number | null = null;

    const onConsole = (msg: ConsoleMessage) => {
      const type = msg.type();
      const text = msg.text();
      // Filter out known-noisy Next.js dev messages and analytics beacons.
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
      if (res.url() === `${BASE_URL}${route}` || res.url() === `${BASE_URL}${route}/`) {
        httpStatus = res.status();
      }
      // Record 4xx/5xx API responses that aren't expected auth gates
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
      await navigate();
    } catch (err) {
      notes.push(`Navigation threw: ${(err as Error).message}`);
    }
    const loadMs = Date.now() - start;

    page.off("console", onConsole);
    page.off("pageerror", onPageError);
    page.off("requestfailed", onRequestFailed);
    page.off("response", onResponse);

    const finalUrl = page.url();
    const unauthorizedRedirect = /\/login/.test(finalUrl) && !route.includes("login");

    const shotName = `${(slug ?? route).replace(/[^a-z0-9]+/gi, "_").replace(/^_|_$/g, "") || "root"}.png`;
    const screenshotPath = join(SCREENSHOT_DIR, shotName);
    try {
      await page.screenshot({ path: screenshotPath, fullPage: true });
    } catch (err) {
      notes.push(`Screenshot failed: ${(err as Error).message}`);
    }

    return {
      route,
      finalUrl,
      httpStatus,
      loadMs,
      consoleErrors,
      consoleWarnings,
      pageErrors,
      failedRequests,
      unauthorizedRedirect,
      screenshotPath,
      notes,
    };
  }
});
