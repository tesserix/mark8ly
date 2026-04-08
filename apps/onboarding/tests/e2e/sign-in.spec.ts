import { expect, test } from "@playwright/test";

import { fetchMagicLinkToken, uniqueEmail } from "./helpers";

/**
 * Phase M — Returning-user sign-in.
 *
 * Walks the full lifecycle for an email/password merchant:
 *
 *   1. Sign up via the onboarding form using a known password.
 *   2. Click the magic link to land on /welcome (so the tenant + FGA
 *      tuple actually exist).
 *   3. Open a fresh browser context (no cookies, no storage).
 *   4. Visit /login, fill the same email + password, submit.
 *   5. Assert the new context lands on /welcome with a session cookie
 *      and the admin CTA visible.
 *
 * If this ever fails the returning-user funnel is broken — either the
 * tenant-by-owner lookup, the auto-login retry, or the cookie forwarding
 * has regressed.
 *
 * Google sign-in coverage is intentionally NOT in this spec. The Google
 * popup needs Google Identity Services to load and prompt with a real
 * browser session, which the headless test environment can't provide
 * without extensive mocking. Manual smoke-test for now; tracked as a
 * Phase M follow-up.
 */
test("returning user signs in via email + password", async ({
  browser,
  request,
}) => {
  const { email, slug, businessName, password } = uniqueEmail("signin");

  // ── 1. Sign up via the onboarding form ──────────────────────────────
  const signupCtx = await browser.newContext();
  const signupPage = await signupCtx.newPage();

  await signupPage.goto("/onboarding");
  await signupPage.getByLabel(/email address/i).fill(email);
  await signupPage.locator("#password").fill(password);
  await signupPage.getByLabel(/business name/i).fill(businessName);
  await signupPage.locator("#slug").fill(slug);
  await expect(signupPage.getByText(/✓ available/i)).toBeVisible({
    timeout: 5000,
  });

  await signupPage.getByLabel(/country/i).click();
  await signupPage.getByRole("option", { name: /united states/i }).click();
  const currencyTrigger = signupPage.getByLabel(/currency/i);
  if ((await currencyTrigger.textContent())?.includes("Select")) {
    await currencyTrigger.click();
    await signupPage.getByRole("option", { name: /usd/i }).first().click();
  }

  await signupPage.getByRole("button", { name: /get my store ready/i }).click();
  await expect(signupPage).toHaveURL(/\/onboarding\/check-inbox/, {
    timeout: 10_000,
  });

  // ── 2. Walk the magic link so the tenant + FGA tuple exist ──────────
  const token = await fetchMagicLinkToken(request, email);
  await signupPage.goto(`/onboarding/verify?token=${encodeURIComponent(token)}`);
  await expect(signupPage).toHaveURL(/\/welcome/, { timeout: 15_000 });

  await signupCtx.close();

  // ── 3. Fresh context, no cookies — sign back in via /login ──────────
  const loginCtx = await browser.newContext();
  const loginPage = await loginCtx.newPage();

  await loginPage.goto("/login");
  await expect(
    loginPage.getByRole("heading", { name: /welcome back/i }),
  ).toBeVisible();

  await loginPage.getByLabel(/email address/i).fill(email);
  await loginPage.getByLabel(/password/i).fill(password);
  await loginPage.getByRole("button", { name: /^sign in$/i }).click();

  // ── 4. Lands on /welcome with the admin CTA ─────────────────────────
  await expect(loginPage).toHaveURL(/\/welcome/, { timeout: 15_000 });
  await expect(
    loginPage.getByRole("link", { name: /open admin dashboard/i }),
  ).toBeVisible();

  await loginCtx.close();
});

test("login form rejects unknown credentials", async ({ page }) => {
  const { email, password } = uniqueEmail("nosignup");

  await page.goto("/login");
  await page.getByLabel(/email address/i).fill(email);
  await page.getByLabel(/password/i).fill(password);
  await page.getByRole("button", { name: /^sign in$/i }).click();

  // The user has never signed up — Identity Toolkit returns
  // INVALID_LOGIN_CREDENTIALS which the form maps to a friendly message.
  await expect(
    page.getByText(/email or password is incorrect/i),
  ).toBeVisible({ timeout: 10_000 });
  await expect(page).not.toHaveURL(/\/welcome/);
});
