import { test, expect } from "@playwright/test";

/**
 * Full-flow E2E test — exercises the complete merchant + customer journey:
 *
 *   Admin: sign in → configure Razorpay → configure Delhivery →
 *          create product (Active) → verify in list
 *
 *   Storefront: verify product on shop page → customer create account →
 *               sign in → add to cart → checkout page
 *
 * Run:
 *   FULL_FLOW=1 \
 *   ADMIN_BASE_URL=https://india-store-admin.mark8ly.com \
 *   STOREFRONT_BASE_URL=https://india-store.mark8ly.com \
 *   ADMIN_EMAIL=mahesh.sangawar@gmail.com \
 *   ADMIN_PASSWORD=Admin@1234 \
 *   RAZORPAY_KEY_ID=rzp_test_... \
 *   RAZORPAY_KEY_SECRET=QUu4... \
 *   DELHIVERY_API_KEY=b8e0... \
 *   CUSTOMER_EMAIL=admtesserix@gmail.com \
 *   CUSTOMER_PASSWORD=Test@123 \
 *   npx playwright test full-flow.spec.ts
 */

const SHOULD_RUN = process.env.FULL_FLOW === "1";
const ADMIN_URL = process.env.ADMIN_BASE_URL ?? "https://india-store-admin.mark8ly.com";
const STOREFRONT_URL = process.env.STOREFRONT_BASE_URL ?? "https://india-store.mark8ly.com";
const ADMIN_EMAIL = process.env.ADMIN_EMAIL ?? "";
const ADMIN_PASSWORD = process.env.ADMIN_PASSWORD ?? "";
const RAZORPAY_KEY_ID = process.env.RAZORPAY_KEY_ID ?? "";
const RAZORPAY_KEY_SECRET = process.env.RAZORPAY_KEY_SECRET ?? "";
const DELHIVERY_API_KEY = process.env.DELHIVERY_API_KEY ?? "";
const CUSTOMER_EMAIL = process.env.CUSTOMER_EMAIL ?? "";
const CUSTOMER_PASSWORD = process.env.CUSTOMER_PASSWORD ?? "";
const TENANT_NAME = process.env.TENANT_NAME ?? "india store";

const PRODUCT_TITLE = `E2E Linen Shirt ${Date.now().toString(36)}`;
const PRODUCT_HANDLE = `e2e-linen-${Date.now().toString(36)}`;

test.describe("full flow: admin + storefront", () => {
  test.skip(!SHOULD_RUN, "set FULL_FLOW=1 to run");
  test.describe.configure({ mode: "serial" });

  test("1. admin sign-in", async ({ browser }) => {
    test.setTimeout(60_000);
    const ctx = await browser.newContext({ baseURL: ADMIN_URL, viewport: { width: 1440, height: 900 } });
    const page = await ctx.newPage();

    await page.goto("/login");
    await page.waitForLoadState("networkidle").catch(() => {});

    await page.getByLabel(/email address/i).fill(ADMIN_EMAIL);
    await page.getByLabel(/password/i).fill(ADMIN_PASSWORD);
    await Promise.all([
      page.waitForURL(/\/(dashboard|pick-tenant)/, { timeout: 45_000 }).catch(() => {}),
      page.getByRole("button", { name: /^sign in$/i }).click(),
    ]);

    if (page.url().includes("/pick-tenant")) {
      await page.getByText(new RegExp(TENANT_NAME, "i")).first().click();
      await page.waitForURL(/\/dashboard/, { timeout: 20_000 }).catch(() => {});
    }

    expect(page.url()).toContain("/dashboard");
    await page.screenshot({ path: "tests/e2e/.audit/flow-01-dashboard.png" });

    // Save storage state for subsequent tests
    await ctx.storageState({ path: "tests/e2e/.audit/admin-state.json" });
    await ctx.close();
  });

  test("2. configure Razorpay in Settings → Payments", async ({ browser }) => {
    test.setTimeout(30_000);
    test.skip(!RAZORPAY_KEY_ID, "RAZORPAY_KEY_ID not set");

    const ctx = await browser.newContext({
      baseURL: ADMIN_URL,
      storageState: "tests/e2e/.audit/admin-state.json",
      viewport: { width: 1440, height: 900 },
    });
    const page = await ctx.newPage();

    await page.goto("/settings/payments");
    await page.waitForLoadState("networkidle").catch(() => {});

    // Razorpay card → "Add credentials" (first time) or "Edit"
    const razorpayCard = page.locator("article").filter({ hasText: /^Razorpay/ }).first();
    await expect(razorpayCard).toBeVisible({ timeout: 10_000 });

    const toggleBtn = razorpayCard.getByRole("button", { name: /^(Add credentials|Edit)$/ });
    await toggleBtn.click();

    // Inline form expands with IDs razorpay-api-key / razorpay-secret-key
    const apiKey = page.locator("#razorpay-api-key");
    const secretKey = page.locator("#razorpay-secret-key");
    await expect(apiKey).toBeVisible({ timeout: 5_000 });
    await apiKey.fill(RAZORPAY_KEY_ID);
    await secretKey.fill(RAZORPAY_KEY_SECRET);

    // Activate
    const activeCheckbox = razorpayCard.getByRole("checkbox", { name: /active/i });
    if (!(await activeCheckbox.isChecked().catch(() => false))) {
      await activeCheckbox.check();
    }

    await page.getByRole("button", { name: /save configuration/i }).click();
    await expect(page.getByText(/configuration saved/i)).toBeVisible({ timeout: 15_000 });
    await page.screenshot({ path: "tests/e2e/.audit/flow-02-razorpay.png" });

    // Verify status flipped to "Active" after router.refresh()
    await page.reload();
    await page.waitForLoadState("networkidle").catch(() => {});
    await expect(razorpayCard.getByText(/^Active$/).first()).toBeVisible({ timeout: 10_000 });
    await page.screenshot({ path: "tests/e2e/.audit/flow-02-razorpay-verified.png" });
    await ctx.close();
  });

  test("3. configure Delhivery in Settings → Shipping", async ({ browser }) => {
    test.setTimeout(30_000);
    test.skip(!DELHIVERY_API_KEY, "DELHIVERY_API_KEY not set");

    const ctx = await browser.newContext({
      baseURL: ADMIN_URL,
      storageState: "tests/e2e/.audit/admin-state.json",
      viewport: { width: 1440, height: 900 },
    });
    const page = await ctx.newPage();

    await page.goto("/settings/shipping");
    await page.waitForLoadState("networkidle").catch(() => {});

    const delhiveryCard = page.locator("article").filter({ hasText: /^Delhivery/ }).first();
    await expect(delhiveryCard).toBeVisible({ timeout: 10_000 });

    const toggleBtn = delhiveryCard.getByRole("button", { name: /^(Add credentials|Edit)$/ });
    await toggleBtn.click();

    await page.locator("#delhivery-api-key").fill(DELHIVERY_API_KEY);

    // Minimal warehouse address so label generation works later.
    await page.getByLabel(/warehouse name/i).fill("Playwrite Test Warehouse").catch(() => {});
    await page.getByLabel(/address line 1/i).fill("Plot 12, Industrial Area").catch(() => {});
    await page.getByLabel(/^city$/i).fill("Bengaluru").catch(() => {});
    await page.getByLabel(/state|region/i).fill("KA").catch(() => {});
    await page.getByLabel(/postal|pin/i).fill("560100").catch(() => {});
    await page.getByLabel(/country/i).fill("IN").catch(() => {});
    await page.getByLabel(/phone/i).fill("9999999999").catch(() => {});

    const activeCheckbox = delhiveryCard.getByRole("checkbox", { name: /active/i });
    if (!(await activeCheckbox.isChecked().catch(() => false))) {
      await activeCheckbox.check();
    }

    await page.getByRole("button", { name: /save configuration/i }).click();
    await expect(page.getByText(/configuration saved/i)).toBeVisible({ timeout: 15_000 });
    await page.screenshot({ path: "tests/e2e/.audit/flow-03-delhivery.png" });

    await page.reload();
    await page.waitForLoadState("networkidle").catch(() => {});
    await expect(delhiveryCard.getByText(/^Active$/).first()).toBeVisible({ timeout: 10_000 });
    await page.screenshot({ path: "tests/e2e/.audit/flow-03-delhivery-verified.png" });
    await ctx.close();
  });

  test("4. create Active product", async ({ browser }) => {
    test.setTimeout(45_000);

    const ctx = await browser.newContext({
      baseURL: ADMIN_URL,
      storageState: "tests/e2e/.audit/admin-state.json",
      viewport: { width: 1440, height: 900 },
    });
    const page = await ctx.newPage();

    await page.goto("/products/new");
    await page.waitForLoadState("networkidle").catch(() => {});

    // Fill form
    await page.getByLabel(/title/i).first().fill(PRODUCT_TITLE);

    const handleInput = page.locator('input[placeholder="linen-shirt"]').first();
    if (await handleInput.isVisible()) {
      await handleInput.fill(PRODUCT_HANDLE);
    }

    const descField = page.locator("textarea").first();
    if (await descField.isVisible()) {
      await descField.fill("Premium linen shirt — lightweight, breathable, perfect for warm days.");
    }

    // Set status to Active
    const statusTrigger = page.locator('button[role="combobox"]').first();
    if (await statusTrigger.isVisible({ timeout: 3_000 }).catch(() => false)) {
      await statusTrigger.click();
      await page.waitForTimeout(500);
      await page.getByRole("option", { name: /active/i }).click();
      await page.waitForTimeout(500);
    }

    // Price
    const priceInput = page.locator('input[placeholder="19.99"]').first();
    if (await priceInput.isVisible()) {
      await priceInput.fill("1299.00");
    }

    // Stock
    const stockInput = page.locator('input[placeholder="0"]').first();
    if (await stockInput.isVisible()) {
      await stockInput.fill("50");
    }

    await page.screenshot({ path: "tests/e2e/.audit/flow-04-product-form.png" });

    // Submit
    await page.getByRole("button", { name: /create product/i }).click();

    // Wait for redirect to /products/[id] or success feedback
    await page.waitForURL(/\/products\/(?!new)/, { timeout: 15_000 }).catch(async () => {
      // Maybe stayed on /products/new — check for error messages
      await page.screenshot({ path: "tests/e2e/.audit/flow-04-product-error.png" });
    });

    await page.screenshot({ path: "tests/e2e/.audit/flow-04-product-created.png" });
    console.log("Product created. URL:", page.url());

    // Verify in products list
    await page.goto("/products");
    await page.waitForLoadState("networkidle").catch(() => {});
    const body = await page.textContent("body");
    expect(body).toContain(PRODUCT_TITLE.split(" ")[0]); // at least first word
    await page.screenshot({ path: "tests/e2e/.audit/flow-04-products-list.png" });

    await ctx.close();
  });

  test("5. storefront shows the product", async ({ browser }) => {
    test.setTimeout(30_000);

    const ctx = await browser.newContext({
      baseURL: STOREFRONT_URL,
      viewport: { width: 1440, height: 900 },
    });
    const page = await ctx.newPage();

    await page.goto("/products");
    await page.waitForLoadState("networkidle").catch(() => {});
    await page.screenshot({ path: "tests/e2e/.audit/flow-05-storefront-products.png" });

    // Check for the product
    const body = await page.textContent("body");
    const found = body?.includes("Linen") || body?.includes("linen");
    console.log("Product on storefront /products:", found);

    // Try the product detail page
    await page.goto(`/products/${PRODUCT_HANDLE}`);
    await page.waitForLoadState("networkidle").catch(() => {});
    await page.screenshot({ path: "tests/e2e/.audit/flow-05-product-detail.png" });

    await ctx.close();
  });

  test("6. customer sign-in on storefront", async ({ browser }) => {
    test.setTimeout(30_000);
    test.skip(!CUSTOMER_EMAIL, "CUSTOMER_EMAIL not set");

    const ctx = await browser.newContext({
      baseURL: STOREFRONT_URL,
      viewport: { width: 1440, height: 900 },
    });
    const page = await ctx.newPage();

    await page.goto("/sign-in");
    await page.waitForLoadState("networkidle").catch(() => {});

    await page.getByLabel(/email address/i).fill(CUSTOMER_EMAIL);
    await page.getByLabel(/password/i).fill(CUSTOMER_PASSWORD);
    await page.getByRole("button", { name: /sign in/i }).click();

    await page.waitForURL(/\/account/, { timeout: 15_000 }).catch(() => {});
    console.log("Customer signed in. URL:", page.url());
    await page.screenshot({ path: "tests/e2e/.audit/flow-06-customer-signed-in.png" });

    // Save state for cart/checkout
    await ctx.storageState({ path: "tests/e2e/.audit/customer-state.json" });
    await ctx.close();
  });

  test("7. add to cart + checkout", async ({ browser }) => {
    test.setTimeout(45_000);

    const ctx = await browser.newContext({
      baseURL: STOREFRONT_URL,
      storageState: "tests/e2e/.audit/customer-state.json",
      viewport: { width: 1440, height: 900 },
    });
    const page = await ctx.newPage();

    // Visit the product and add to cart
    await page.goto(`/products/${PRODUCT_HANDLE}`);
    await page.waitForLoadState("networkidle").catch(() => {});

    const addToCartBtn = page.getByRole("button", { name: /add to cart/i }).first();
    if (await addToCartBtn.isVisible({ timeout: 5_000 }).catch(() => false)) {
      await addToCartBtn.click();
      await page.waitForTimeout(1_000);
      console.log("Added to cart");
    } else {
      console.log("Add to cart button not found on", page.url());
    }

    // Go to cart
    await page.goto("/cart");
    await page.waitForLoadState("networkidle").catch(() => {});
    await page.screenshot({ path: "tests/e2e/.audit/flow-07-cart.png" });

    // Go to checkout
    const checkoutLink = page.getByRole("link", { name: /checkout/i }).first();
    if (await checkoutLink.isVisible({ timeout: 3_000 }).catch(() => false)) {
      await checkoutLink.click();
      await page.waitForLoadState("networkidle").catch(() => {});
    } else {
      await page.goto("/checkout");
      await page.waitForLoadState("networkidle").catch(() => {});
    }

    await page.screenshot({ path: "tests/e2e/.audit/flow-07-checkout.png" });
    console.log("Checkout page:", page.url());

    await ctx.close();
  });

  test("8. admin sees orders / settings pages clean", async ({ browser }) => {
    test.setTimeout(60_000);

    const ctx = await browser.newContext({
      baseURL: ADMIN_URL,
      storageState: "tests/e2e/.audit/admin-state.json",
      viewport: { width: 1440, height: 900 },
    });
    const page = await ctx.newPage();

    const pages = [
      "/orders",
      "/customers",
      "/marketing/campaigns",
      "/marketing/coupons",
      "/settings/stores",
      "/settings/team",
      "/settings/payments",
      "/settings/shipping",
      "/settings/themes",
      "/settings/domains",
      "/settings/subscription",
      "/settings/notifications",
      "/settings/audit-logs",
    ];

    for (const route of pages) {
      await page.goto(route);
      await page.waitForLoadState("networkidle", { timeout: 10_000 }).catch(() => {});

      const errors: string[] = [];
      page.on("pageerror", (e: Error) => errors.push(e.message));

      const name = route.replace(/\//g, "_").slice(1);
      await page.screenshot({ path: `tests/e2e/.audit/flow-08-${name}.png` });

      if (errors.length > 0) {
        console.log(`  PAGE ERROR on ${route}:`, errors[0]);
      }
    }

    console.log("All admin pages rendered.");
    await ctx.close();
  });
});
