import { expect, test } from "@playwright/test";

const API_URL = process.env.API_URL ?? "http://localhost:8086";
const ONBOARDING_URL =
  process.env.ONBOARDING_URL ?? "http://localhost:4201";
const STOREFRONT_URL =
  process.env.STOREFRONT_BASE_URL ?? "http://localhost:4203";

/**
 * Phase 1 — per-host customer cookie scoping.
 *
 * Browser-level cross-host isolation (cookie not sent to other store /
 * not sent to admin) requires real DNS subdomains and is verified in
 * prod smoke testing. In localhost dev with `?slug=...` routing every
 * request shares one host, so we can only assert what the cookie
 * Domain attribute is at set time.
 *
 * The regression we're guarding against: cookie Domain reverts to
 * `.mark8ly.com` (parent scope) which would let a cookie set at
 * store-a be sent to every mark8ly subdomain.
 *
 * Note on `?slug=` routing: the homepage (/) accepts ?slug= to target a
 * specific store. The /create-account and /sign-in pages resolve the store
 * from the Host header (then DEFAULT_STORE_SLUG env var for localhost).
 * The onboardStore helper below ensures a real merchant store exists;
 * the dev stack must have DEFAULT_STORE_SLUG pointing to a live store for
 * the customer sign-up/sign-in assertions to succeed. What we CAN assert
 * here in all environments — including localhost — is the cookie Domain
 * attribute and sign-out clearing.
 *
 * What goes to manual prod smoke testing (requires real subdomains / DNS):
 *   - Cookie not sent to a different store's subdomain.
 *   - Cookie not sent to the admin subdomain.
 *   - Custom domain cookie isolation.
 */

async function onboardStore(
  page: import("@playwright/test").Page,
  request: import("@playwright/test").APIRequestContext,
  details: {
    email: string;
    slug: string;
    businessName: string;
    password: string;
  },
): Promise<void> {
  await page.goto(`${ONBOARDING_URL}/onboarding`);
  await page.getByLabel(/email address/i).fill(details.email);
  await page.getByLabel(/business name/i).fill(details.businessName);
  await page.locator("#slug").fill(details.slug);
  await expect(page.getByText(/✓ available/i)).toBeVisible({ timeout: 5000 });
  await page.getByLabel(/country/i).click();
  await page.getByRole("option", { name: /united states/i }).click();
  const currencyTrigger = page.getByLabel(/currency/i);
  if ((await currencyTrigger.textContent())?.includes("Select")) {
    await currencyTrigger.click();
    await page.getByRole("option", { name: /usd/i }).first().click();
  }
  await page.getByRole("button", { name: /get my store ready/i }).click();
  await expect(page).toHaveURL(/\/onboarding\/check-inbox/, {
    timeout: 10_000,
  });

  const tokenRes = await request.get(
    `${API_URL}/api/v1/test/verification/latest?email=${encodeURIComponent(details.email)}`,
  );
  expect(tokenRes.ok()).toBeTruthy();
  const tokenBody = (await tokenRes.json()) as { data: { token: string } };
  await page.goto(
    `${ONBOARDING_URL}/onboarding/verify?token=${encodeURIComponent(tokenBody.data.token)}`,
  );
  await expect(page).toHaveURL(/\/onboarding\/set-password/, {
    timeout: 15_000,
  });
  await page.locator("#password").fill(details.password);
  await page.getByRole("button", { name: /create account/i }).click();
  await expect(page).toHaveURL(/\/welcome/, { timeout: 15_000 });
}

test("customer cookie Domain is the exact request host (not .mark8ly.com)", async ({
  page,
  request,
}) => {
  // Onboard a fresh merchant so we have a real store to sign a customer up against.
  // NOTE: /create-account resolves the store slug from the Host header (falling back
  // to DEFAULT_STORE_SLUG env var in localhost dev). The ?slug= below is appended for
  // readability / future compatibility only — it is not read by the page itself.
  const stamp = `${Date.now().toString(36)}${Math.floor(Math.random() * 1e4).toString(36)}`;
  const merchant = {
    email: `iso-merchant-${stamp}@example.com`,
    slug: `iso-${stamp}`.replace(/[^a-z0-9-]/g, "").slice(0, 60),
    businessName: `Iso ${stamp}`,
    password: "e2e-test-password-123",
  };
  await onboardStore(page, request, merchant);

  // Create a customer account on this store.
  // The form uses id="signup-email" and id="signup-password".
  // The store context is resolved server-side from the Host header.
  const customer = {
    email: `iso-customer-${stamp}@example.com`,
    password: "customer-password-123",
  };

  await page.goto(`${STOREFRONT_URL}/create-account?slug=${merchant.slug}`);
  await page.getByLabel(/email address/i).fill(customer.email);
  await page.getByLabel(/^password$/i).fill(customer.password);
  await page.getByRole("button", { name: /create account/i }).click();
  // Customer lands on /account after successful sign-up.
  await expect(page).toHaveURL(/\/account/, { timeout: 15_000 });

  // Inspect the cookie set by the storefront server action.
  const cookies = await page.context().cookies();
  const customerCookie = cookies.find((c) => c.name === "mp_customer_session");
  expect(
    customerCookie,
    "mp_customer_session must be set after sign-up",
  ).toBeDefined();

  // The server action sets Domain=<sanitizeHost(request-host)>.
  // In localhost dev, sanitizeHost("localhost:4203") strips the port → "localhost".
  // In production, it will be the exact subdomain (e.g. "acme.mark8ly.com").
  const expectedHost = new URL(STOREFRONT_URL).hostname;
  expect(
    customerCookie!.domain,
    "cookie Domain must match the request host (no leading dot)",
  ).toBe(expectedHost);
  // Belt + suspenders: explicitly assert no leading dot (parent-scope regression).
  expect(customerCookie!.domain.startsWith(".")).toBe(false);
});

test("sign-out clears the customer cookie", async ({ page, request }) => {
  // Re-onboard + sign customer in (test isolation — don't share state).
  const stamp = `${Date.now().toString(36)}${Math.floor(Math.random() * 1e4).toString(36)}`;
  const merchant = {
    email: `iso-merchant-out-${stamp}@example.com`,
    slug: `iso-out-${stamp}`.replace(/[^a-z0-9-]/g, "").slice(0, 60),
    businessName: `IsoOut ${stamp}`,
    password: "e2e-test-password-123",
  };
  await onboardStore(page, request, merchant);

  const customer = {
    email: `iso-customer-out-${stamp}@example.com`,
    password: "customer-password-123",
  };

  await page.goto(`${STOREFRONT_URL}/create-account?slug=${merchant.slug}`);
  await page.getByLabel(/email address/i).fill(customer.email);
  await page.getByLabel(/^password$/i).fill(customer.password);
  await page.getByRole("button", { name: /create account/i }).click();
  await expect(page).toHaveURL(/\/account/, { timeout: 15_000 });

  // Sanity: cookie is set before sign-out.
  const cookiesBefore = await page.context().cookies();
  expect(
    cookiesBefore.find((c) => c.name === "mp_customer_session"),
    "mp_customer_session must be set before sign-out",
  ).toBeDefined();

  // Hit sign-out. The route handler clears the cookie and redirects to /.
  await page.goto(`${STOREFRONT_URL}/sign-out`);
  await expect(page).toHaveURL(/\/$/, { timeout: 10_000 });

  const cookiesAfter = await page.context().cookies();
  expect(
    cookiesAfter.find((c) => c.name === "mp_customer_session"),
    "mp_customer_session must be cleared by sign-out",
  ).toBeUndefined();
});
