import { expect, test } from "@playwright/test";

import { STOREFRONT_URL, onboardStore } from "./helpers";



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
    password: "E2e-test-password-123!",
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
    password: "E2e-test-password-123!",
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
