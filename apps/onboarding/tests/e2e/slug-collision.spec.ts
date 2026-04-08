import { expect, test } from "@playwright/test";

import { fetchMagicLinkToken, uniqueEmail } from "./helpers";

/**
 * Phase G — Slug collision regression.
 *
 * The platform-api enforces slug uniqueness at tenant creation time
 * (`internal/tenant/repository.go` returns `Conflict("slug_taken")`).
 * The form surfaces the collision via the live `/slug-available` check.
 *
 * This test:
 *   1. Runs a full golden-path onboarding so a tenant claims the slug.
 *   2. Opens a fresh /onboarding form, types the same slug, asserts the
 *      "Already taken" indicator appears and submit stays disabled.
 *
 * If this ever fails, slug uniqueness is broken at the form layer and we
 * risk creating two tenants with the same vanity URL.
 */
test("rejects a slug that another tenant has already claimed", async ({
  page,
  request,
  context,
}) => {
  const claim = uniqueEmail("claim");

  // ── 1. Claim the slug end-to-end ──────────────────────────────────────
  await page.goto("/onboarding");
  await page.getByLabel(/email address/i).fill(claim.email);
  await page.locator("#password").fill(claim.password);
  await page.getByLabel(/business name/i).fill(claim.businessName);
  await page.locator("#slug").fill(claim.slug);
  await expect(page.getByText(/✓ available/i)).toBeVisible({ timeout: 5000 });

  await page.getByLabel(/country/i).click();
  await page.getByRole("option", { name: /united states/i }).click();
  const currencyTrigger = page.getByLabel(/currency/i);
  if ((await currencyTrigger.textContent())?.includes("Select")) {
    await currencyTrigger.click();
    await page.getByRole("option", { name: /usd/i }).first().click();
  }
  await page.getByRole("button", { name: /get my store ready/i }).click();
  await expect(page).toHaveURL(/\/onboarding\/check-inbox/, { timeout: 10_000 });

  const token = await fetchMagicLinkToken(request, claim.email);
  await page.goto(`/onboarding/verify?token=${encodeURIComponent(token)}`);
  await expect(page).toHaveURL(/\/welcome/, { timeout: 15_000 });

  // ── 2. Fresh tab, same slug, expect "Already taken" ───────────────────
  // A new browser context so the previous run's onboarding store
  // doesn't bleed in and confuse the slug auto-suggester.
  const fresh = await context.browser()!.newContext();
  const freshPage = await fresh.newPage();
  await freshPage.goto("http://localhost:4201/onboarding");

  const collide = uniqueEmail("collide");
  await freshPage.getByLabel(/email address/i).fill(collide.email);
  await freshPage.getByLabel(/business name/i).fill(collide.businessName);

  // Override the auto-suggested slug with the one we just claimed.
  const slugInput = freshPage.locator("#slug");
  await slugInput.fill(claim.slug);

  await expect(freshPage.getByText(/already taken/i)).toBeVisible({
    timeout: 5000,
  });
  await expect(
    freshPage.getByRole("button", { name: /get my store ready/i }),
  ).toBeDisabled();

  await fresh.close();
});
