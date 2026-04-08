import { expect, test } from "@playwright/test";

import { uniqueEmail } from "./helpers";

/**
 * Phase G — Invalid magic link token.
 *
 * Walks the form up to /onboarding/check-inbox so the onboarding store is
 * populated, then visits /onboarding/verify with a garbage token. The
 * server-side verification call should reject with "token_not_found" and
 * VerifyMagicLink should render its error UI.
 *
 * If this ever fails silently (e.g. still navigates to /welcome), the
 * verify-token endpoint is accepting tokens it shouldn't.
 */
test("invalid magic link token surfaces an error and never navigates to welcome", async ({
  page,
}) => {
  const { email, slug, businessName } = uniqueEmail("invalid");

  await page.goto("/onboarding");
  await page.getByLabel(/email address/i).fill(email);
  await page.getByLabel(/business name/i).fill(businessName);
  await page.locator("#slug").fill(slug);
  await expect(page.getByText(/✓ available/i)).toBeVisible({ timeout: 5000 });

  await page.getByLabel(/country/i).click();
  await page.getByRole("option", { name: /united states/i }).click();
  const currencyTrigger = page.getByLabel(/currency/i);
  if ((await currencyTrigger.textContent())?.includes("Select")) {
    await currencyTrigger.click();
    await page.getByRole("option", { name: /usd/i }).first().click();
  }
  await page.getByRole("button", { name: /send verification link/i }).click();
  await expect(page).toHaveURL(/\/onboarding\/check-inbox/, { timeout: 10_000 });

  // Visit verify with a syntactically valid but unknown token.
  await page.goto("/onboarding/verify?token=this-token-does-not-exist-xyz");

  await expect(
    page.getByRole("heading", { name: /couldn.t verify your link/i }),
  ).toBeVisible({ timeout: 10_000 });
  await expect(page).not.toHaveURL(/\/welcome/);
});
