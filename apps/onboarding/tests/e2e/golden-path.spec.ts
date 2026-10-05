import { expect, test } from "@playwright/test";

import {
  fetchLatestOnboardingSession,
  fetchMagicLinkToken,
  uniqueEmail,
} from "./helpers";

/**
 * Phase G — Golden path.
 *
 * Walks the full onboarding journey end-to-end against the running
 * stack: visit landing → click Start Free → fill the single-page form
 * → submit → check-inbox screen → fetch the magic-link token via the
 * non-prod test helper → visit /onboarding/verify?token=... → assert
 * the welcome page renders.
 *
 * If this ever fails, the entire onboarding funnel is broken.
 */
test("golden path: landing → form → magic link → welcome", async ({
  page,
  request,
}) => {
  const { email, slug, businessName, password, name } = uniqueEmail();

  // 1. Landing page renders and the primary CTA goes to /onboarding.
  //
  // Arrive tagged (#992): the campaign values must still be against the
  // completed merchant at the end, having crossed a server action, the
  // verification step and completion. The stray `token` parameter must
  // NOT: only the allowlisted keys may leave the browser.
  await page.goto(
    "/?utm_source=e2e&utm_medium=spec&utm_campaign=attribution&utm_content=hero&token=must-not-persist",
  );
  await expect(
    // Pins both halves of the H1. The offer half is the point of
    // tesserix/mark8ly#599: the <title> leads with the offer, so an H1
    // that leads only with the brand line tells a different story to
    // anyone who arrives without passing through a SERP.
    page.getByRole("heading", {
      // \s+ rather than a literal space: the markup uses &nbsp; to stop
      // "days." orphaning on mobile, and a literal space would not match it.
      name: /a storefront worth opening\.\s*free for ninety\s+days\./i,
    }),
  ).toBeVisible();

  // 2. Onboarding form, reached the way a visitor reaches it: through the
  //    hero CTA, whose href carries no query string. The attribution
  //    survives that hop because it lives in the browser, not the URL.
  await page.getByRole("link", { name: /open your store/i }).first().click();
  await expect(page).toHaveURL(/\/onboarding$/);
  await expect(
    page.getByRole("heading", { name: /start your store/i }),
  ).toBeVisible();

  await page.getByLabel(/email address/i).fill(email);
  await page.getByLabel(/business name/i).fill(businessName);

  // The slug field auto-suggests from the business name; overwrite to
  // guarantee uniqueness across test runs.
  const slugInput = page.locator("#slug");
  await slugInput.fill(slug);

  // Wait for the debounced availability check to land.
  await expect(page.getByText(/✓ available/i)).toBeVisible({ timeout: 5000 });

  // Country/currency are Radix Selects — open and pick the first US/USD-ish
  // pair so the test doesn't depend on browser locale.
  await page.getByLabel(/country/i).click();
  await page.getByRole("option", { name: /united states/i }).click();
  // Currency auto-derives from country, but defensively re-select if not
  // populated.
  const currencyTrigger = page.getByLabel(/currency/i);
  if ((await currencyTrigger.textContent())?.includes("Select")) {
    await currencyTrigger.click();
    await page.getByRole("option", { name: /usd/i }).first().click();
  }

  // 3. Submit.
  await page.getByRole("button", { name: /send verification link/i }).click();

  // 4. Land on the check-inbox screen.
  await expect(page).toHaveURL(/\/onboarding\/check-inbox/, { timeout: 10_000 });

  // 5. Fetch the magic-link token via the test helper and "click" the
  // verification URL.
  const token = await fetchMagicLinkToken(request, email);
  await page.goto(`/onboarding/verify?token=${encodeURIComponent(token)}`);

  // 6. Phase M: lands on /onboarding/set-password. Pick a password and
  //    click Create account → /welcome.
  await expect(page).toHaveURL(/\/onboarding\/set-password/, {
    timeout: 15_000,
  });
  await page.locator("#name").fill(name);
  await page.locator("#password").fill(password);
  await page.getByRole("button", { name: /create account/i }).click();
  await expect(page).toHaveURL(/\/welcome/, { timeout: 15_000 });

  // 7. The campaign is against the completed merchant, server-side (#992),
  //    sanitised: the allowlisted values survive, the stray token does not,
  //    and an @example.com signup is classified test, not external.
  const session = await fetchLatestOnboardingSession(request, email);
  expect(session.status).toBe("completed");
  expect(session.tenant_id).toBeTruthy();
  expect(session.classification).toBe("test");
  expect(session.acquisition?.first).toMatchObject({
    utm_source: "e2e",
    utm_medium: "spec",
    utm_campaign: "attribution",
    utm_content: "hero",
    landing_path: "/",
  });
  expect(JSON.stringify(session.acquisition)).not.toContain("must-not-persist");
  expect(JSON.stringify(session.acquisition)).not.toContain("token");
});
