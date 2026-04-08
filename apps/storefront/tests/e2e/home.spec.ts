import { expect, test } from "@playwright/test";

const API_URL = process.env.API_URL ?? "http://localhost:8086";
const ONBOARDING_URL =
  process.env.ONBOARDING_URL ?? "http://localhost:4201";
const STOREFRONT_URL =
  process.env.STOREFRONT_BASE_URL ?? "http://localhost:4203";

/**
 * Phase S — storefront home page.
 *
 * Two assertions:
 *   1. Unknown slug renders a "not found" page.
 *   2. A real, onboarded store renders its name + currency +
 *      country chips. We onboard a merchant inline to avoid
 *      depending on seed data.
 *
 * The test uses ?slug=… to target specific stores rather than
 * host-based subdomain routing — Playwright can't trivially set
 * a real subdomain for localhost without DNS tricks.
 */
test("unknown slug renders store-not-found", async ({ page }) => {
  await page.goto(`${STOREFRONT_URL}/?slug=no-such-store`);
  await expect(page.getByText(/nothing here/i)).toBeVisible();
});

test("onboarded store renders on the storefront", async ({
  page,
  request,
}) => {
  const stamp = `${Date.now().toString(36)}${Math.floor(Math.random() * 1e4).toString(36)}`;
  const details = {
    email: `storefront-e2e-${stamp}@example.com`,
    slug: `storefront-${stamp}`.replace(/[^a-z0-9-]/g, "").slice(0, 60),
    businessName: `Storefront ${stamp}`,
    password: "e2e-test-password-123",
  };

  // Drive the onboarding form quickly.
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

  // Fetch the magic link token and complete the set-password step.
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

  // Now the store exists. Hit the storefront with its slug.
  await page.goto(`${STOREFRONT_URL}/?slug=${details.slug}`);

  await expect(
    page.getByRole("heading", { name: new RegExp(details.businessName, "i") }),
  ).toBeVisible({ timeout: 10_000 });
  // Chip components render "<LABEL> <VALUE>" where label is the
  // small uppercase pill ("Currency", "Country"). Exact-match the
  // value text to avoid "US" matching "USD".
  await expect(
    page.getByText("USD", { exact: true }).first(),
  ).toBeVisible();
  await expect(
    page.getByText("US", { exact: true }).first(),
  ).toBeVisible();
});
