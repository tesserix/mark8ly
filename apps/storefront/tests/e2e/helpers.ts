import { expect, type APIRequestContext, type Page } from "@playwright/test";

export const API_URL = process.env.API_URL ?? "http://localhost:8086";
export const ONBOARDING_URL =
  process.env.ONBOARDING_URL ?? "http://localhost:4201";
export const STOREFRONT_URL =
  process.env.STOREFRONT_BASE_URL ?? "http://localhost:4203";

export interface MerchantDetails {
  email: string;
  slug: string;
  businessName: string;
  password: string;
}

/** A unique-enough suffix so parallel runs never collide on slug or email. */
export function stamp(): string {
  return `${Date.now().toString(36)}${Math.floor(Math.random() * 1e4).toString(36)}`;
}

/**
 * Drive a merchant through onboarding so a real store exists for the
 * storefront to render. Mirrors the inline flows in home.spec.ts and
 * auth-isolation.spec.ts; new specs should use this rather than copy them.
 */
export async function onboardStore(
  page: Page,
  request: APIRequestContext,
  details: MerchantDetails,
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
  await page.getByRole("button", { name: /send verification link/i }).click();
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
  // The set-password form requires an owner name; without it the
  // submit is blocked and the flow never reaches /welcome.
  await page.locator("#name").fill(details.businessName);
  await page.locator("#password").fill(details.password);
  await page.getByRole("button", { name: /create account/i }).click();
  await expect(page).toHaveURL(/\/welcome/, { timeout: 15_000 });
}
