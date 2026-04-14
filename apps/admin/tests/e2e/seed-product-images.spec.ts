import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { test, expect, type Page } from "@playwright/test";

/**
 * Seeds real Unsplash t-shirt images onto every Active product in the
 * target store. Walks the admin UI (no direct API calls) so it exercises
 * the real signed-URL + GCS finalize path.
 *
 * Run:
 *   SEED_IMAGES=1 \
 *   ADMIN_BASE_URL=https://playwrite-test-admin.mark8ly.com \
 *   ADMIN_EMAIL=... ADMIN_PASSWORD=... TENANT_NAME='playwrite test' \
 *   npx playwright test seed-product-images.spec.ts
 */

const SHOULD_RUN = process.env.SEED_IMAGES === "1";
const ADMIN_URL = process.env.ADMIN_BASE_URL ?? "";
const ADMIN_EMAIL = process.env.ADMIN_EMAIL ?? "";
const ADMIN_PASSWORD = process.env.ADMIN_PASSWORD ?? "";
const TENANT_NAME = process.env.TENANT_NAME ?? "";
const IMAGES_PER_PRODUCT = Number(process.env.IMAGES_PER_PRODUCT ?? 3);

// Unsplash CDN URLs — free, no API key, varied t-shirt / apparel shots.
// 404s are tolerated (Unsplash occasionally rotates IDs); we just need
// at least IMAGES_PER_PRODUCT successful downloads.
const UNSPLASH_URLS = [
  "https://images.unsplash.com/photo-1521572163474-6864f9cf17ab?w=1200&q=80",
  "https://images.unsplash.com/photo-1581655353564-df123a1eb820?w=1200&q=80",
  "https://images.unsplash.com/photo-1503341504253-dff4815485f1?w=1200&q=80",
  "https://images.unsplash.com/photo-1527719327859-c6ce80353573?w=1200&q=80",
  "https://images.unsplash.com/photo-1554568218-0f1715e72254?w=1200&q=80",
  "https://images.unsplash.com/photo-1618354691373-d851c5c3a990?w=1200&q=80",
  "https://images.unsplash.com/photo-1576566588028-4147f3842f27?w=1200&q=80",
  "https://images.unsplash.com/photo-1586790170083-2f9ceadc732d?w=1200&q=80",
  "https://images.unsplash.com/photo-1620799140408-edc6dcb6d633?w=1200&q=80",
  "https://images.unsplash.com/photo-1542272604-787c3835535d?w=1200&q=80",
  "https://images.unsplash.com/photo-1503342217505-b0a15ec3261c?w=1200&q=80",
  "https://images.unsplash.com/photo-1523381210434-271e8be1f52b?w=1200&q=80",
];

const TMP_DIR = path.join(os.tmpdir(), "mark8ly-seed-imgs");

async function downloadOnce(url: string, out: string): Promise<boolean> {
  if (fs.existsSync(out) && fs.statSync(out).size > 0) return true;
  try {
    const res = await fetch(url);
    if (!res.ok) {
      console.log(`  [skip] ${url} -> ${res.status}`);
      return false;
    }
    const buf = Buffer.from(await res.arrayBuffer());
    fs.writeFileSync(out, buf);
    return true;
  } catch (e) {
    console.log(`  [skip] ${url} -> ${e instanceof Error ? e.message : e}`);
    return false;
  }
}

async function ensureImages(): Promise<string[]> {
  fs.mkdirSync(TMP_DIR, { recursive: true });
  const paths: string[] = [];
  for (let i = 0; i < UNSPLASH_URLS.length; i++) {
    const p = path.join(TMP_DIR, `unsplash-${i}.jpg`);
    if (await downloadOnce(UNSPLASH_URLS[i]!, p)) paths.push(p);
  }
  if (paths.length < IMAGES_PER_PRODUCT) {
    throw new Error(`only ${paths.length} usable Unsplash images; need >= ${IMAGES_PER_PRODUCT}`);
  }
  return paths;
}

async function signIn(page: Page): Promise<void> {
  await page.goto("/login");
  await page.getByLabel(/email address/i).fill(ADMIN_EMAIL);
  await page.getByLabel(/password/i).fill(ADMIN_PASSWORD);
  await Promise.all([
    page.waitForURL(/\/(dashboard|pick-tenant)/, { timeout: 30_000 }).catch(() => {}),
    page.getByRole("button", { name: /^sign in$/i }).click(),
  ]);
  if (page.url().includes("/pick-tenant")) {
    await page.getByText(new RegExp(TENANT_NAME, "i")).first().click();
    await page.waitForURL(/\/dashboard/, { timeout: 15_000 }).catch(() => {});
  }
  expect(page.url()).toContain("/dashboard");
}

async function listActiveProductEditUrls(page: Page): Promise<string[]> {
  await page.goto("/products?status=active&page_size=100");
  await page.waitForLoadState("networkidle").catch(() => {});
  const rows = await page.locator("table tbody tr").all();
  const urls: string[] = [];
  for (const row of rows) {
    const href = await row.locator("a").first().getAttribute("href").catch(() => null);
    if (href && href.startsWith("/products/") && !urls.includes(href)) urls.push(href);
  }
  console.log(`  [active filter] ${rows.length} rows -> ${urls.length} unique`);
  return urls;
}

async function uploadMediaFor(page: Page, editUrl: string, files: string[]): Promise<void> {
  await page.goto(editUrl);
  await page.waitForLoadState("networkidle").catch(() => {});

  // Switch to the Media tab.
  await page.getByRole("tab", { name: /^media/i }).click();
  await expect(page.locator("#media-uploader-input")).toBeAttached({ timeout: 10_000 });

  // If the product already has media (e.g. a re-run), skip.
  const existing = await page.getByRole("list").locator("img").count().catch(() => 0);
  if (existing > 0) {
    console.log(`  [skip] already has ${existing} image(s) at ${editUrl}`);
    return;
  }

  await page.locator("#media-uploader-input").setInputFiles(files);

  // Apply the crop dialog once per file.
  for (let i = 0; i < files.length; i++) {
    const dlg = page.getByRole("dialog", { name: /crop image/i });
    await expect(dlg).toBeVisible({ timeout: 15_000 });
    await dlg.getByRole("button", { name: /^apply/i }).click();
    await expect(dlg).toBeHidden({ timeout: 20_000 });
  }

  // Wait until exactly `files.length` cards show up.
  const cardImages = page.getByRole("list").locator("img");
  await expect(cardImages).toHaveCount(files.length, { timeout: 30_000 });

  // Save.
  await page.getByRole("button", { name: /save changes/i }).click();
  await page.waitForLoadState("networkidle").catch(() => {});
  console.log(`  [ok] uploaded ${files.length} images -> ${editUrl}`);
}

test("seed Unsplash images onto Active products", async ({ browser }) => {
  test.skip(!SHOULD_RUN, "set SEED_IMAGES=1 to run");
  test.setTimeout(15 * 60_000);

  const images = await ensureImages();
  console.log(`[seed] downloaded ${images.length} source images to ${TMP_DIR}`);

  const ctx = await browser.newContext({ baseURL: ADMIN_URL, viewport: { width: 1440, height: 900 } });
  const page = await ctx.newPage();

  await signIn(page);

  const active = await listActiveProductEditUrls(page);
  console.log(`[seed] active products: ${active.length}`);

  // Deterministically spread images across products.
  for (let i = 0; i < active.length; i++) {
    const offset = (i * IMAGES_PER_PRODUCT) % images.length;
    const picks: string[] = [];
    for (let j = 0; j < IMAGES_PER_PRODUCT; j++) {
      picks.push(images[(offset + j) % images.length]!);
    }
    console.log(`[seed ${i + 1}/${active.length}] ${active[i]}`);
    try {
      await uploadMediaFor(page, active[i]!, picks);
    } catch (e) {
      console.error(`  [fail] ${active[i]}:`, e instanceof Error ? e.message : e);
      await page.screenshot({ path: `tests/e2e/.audit/seed-fail-${i}.png`, fullPage: true });
    }
  }

  await ctx.close();
});
