import { expect, type APIRequestContext } from "@playwright/test";

/**
 * Cross-app helpers for the admin e2e suite.
 *
 * The admin tests reuse the same test-helper endpoint on platform-api
 * that the onboarding e2e tests use — `/api/v1/test/verification/latest`
 * — so the admin suite can walk the onboarding golden path without a
 * real inbox.
 */

export const ONBOARDING_URL =
  process.env.ONBOARDING_URL ?? "http://localhost:4201";
export const ADMIN_URL = process.env.ADMIN_URL ?? "http://localhost:4202";
export const API_URL = process.env.API_URL ?? "http://localhost:8086";

/**
 * Build a unique email + slug + business name per test run so Postgres
 * state from a previous run can't collide.
 */
export function uniqueEmail(label = "admin-e2e"): {
  email: string;
  slug: string;
  businessName: string;
} {
  const stamp = `${Date.now().toString(36)}${Math.floor(
    Math.random() * 1e4,
  ).toString(36)}`;
  const local = `${label}-${stamp}`;
  return {
    email: `${local}@example.com`,
    slug: local.replace(/[^a-z0-9-]/g, "").slice(0, 60),
    businessName: `${label} ${stamp}`,
  };
}

/**
 * Poll the test helper endpoint until a magic-link token shows up for
 * the given email.
 */
export async function fetchMagicLinkToken(
  request: APIRequestContext,
  email: string,
  { timeoutMs = 5000 }: { timeoutMs?: number } = {},
): Promise<string> {
  const deadline = Date.now() + timeoutMs;
  let lastStatus = 0;
  while (Date.now() < deadline) {
    const res = await request.get(
      `${API_URL}/api/v1/test/verification/latest?email=${encodeURIComponent(email)}`,
    );
    lastStatus = res.status();
    if (res.ok()) {
      const body = (await res.json()) as { data: { token: string } };
      expect(body.data.token, "test helper returned empty token").toBeTruthy();
      return body.data.token;
    }
    if (res.status() !== 404) {
      throw new Error(`test helper failed: ${res.status()}`);
    }
    await new Promise((r) => setTimeout(r, 100));
  }
  throw new Error(
    `magic-link token never appeared for ${email} (last status ${lastStatus})`,
  );
}

/**
 * Drives the onboarding form from blank to welcome page in a single
 * function. Returns the generated details so the caller can use them
 * in post-onboarding assertions (e.g. "dashboard shows the business
 * name I just typed").
 *
 * Takes a Playwright `page` already scoped to the onboarding origin —
 * this helper navigates within it rather than creating its own
 * context, so the session cookie it leaves behind is visible to
 * anything else in the same context.
 */
export async function completeOnboarding(
  page: import("@playwright/test").Page,
  request: APIRequestContext,
  label = "admin-e2e",
): Promise<{ email: string; slug: string; businessName: string }> {
  const details = uniqueEmail(label);

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

  const token = await fetchMagicLinkToken(request, details.email);
  await page.goto(
    `${ONBOARDING_URL}/onboarding/verify?token=${encodeURIComponent(token)}`,
  );
  await expect(page).toHaveURL(/\/welcome/, { timeout: 15_000 });

  return details;
}
