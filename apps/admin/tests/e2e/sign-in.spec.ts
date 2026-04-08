import { expect, test } from "@playwright/test";

import {
  ADMIN_URL,
  ONBOARDING_URL,
  completeOnboarding,
  uniqueEmail,
} from "./helpers";

/**
 * Phase M — Returning-user sign-in (admin app).
 *
 * Walks the full lifecycle for an email/password merchant:
 *
 *   1. Sign up via the onboarding form (which now ends on /set-password)
 *      using a known password. Lands on /welcome.
 *   2. Open a fresh browser context (no cookies, no storage).
 *   3. Visit ${ADMIN_URL}/login, fill the same email + password, submit.
 *   4. Assert the new context lands on /dashboard with a session cookie.
 *
 * If this ever fails the returning-user funnel is broken — either the
 * tenant-by-owner lookup, the auto-login retry, or the cookie forwarding
 * has regressed.
 *
 * Google sign-in coverage is intentionally NOT in this spec. The Google
 * popup needs Google Identity Services to load and prompt with a real
 * browser session, which the headless test environment can't provide
 * without extensive mocking.
 */
test("returning user signs in via email + password on admin /login", async ({
  browser,
  request,
}) => {
  // ── 1. Sign up so the tenant exists ─────────────────────────────────
  const signupCtx = await browser.newContext();
  const signupPage = await signupCtx.newPage();
  const details = await completeOnboarding(signupPage, request, "signin");
  await signupCtx.close();

  // ── 2. Fresh context, no cookies — sign back in via admin /login ────
  const loginCtx = await browser.newContext();
  const loginPage = await loginCtx.newPage();

  await loginPage.goto(`${ADMIN_URL}/login`);
  await expect(
    loginPage.getByRole("heading", { name: /welcome back/i }),
  ).toBeVisible();

  await loginPage.getByLabel(/email address/i).fill(details.email);
  await loginPage.getByLabel(/password/i).fill(details.password);
  await loginPage.getByRole("button", { name: /^sign in$/i }).click();

  // ── 3. Lands on /dashboard with the session cookie active ───────────
  await expect(loginPage).toHaveURL(/\/dashboard/, { timeout: 15_000 });
  await expect(
    loginPage.getByRole("heading", { name: new RegExp(details.businessName, "i") }),
  ).toBeVisible({ timeout: 10_000 });

  await loginCtx.close();
});

test("admin /login rejects unknown credentials", async ({ page }) => {
  const { email, password } = uniqueEmail("nosignup");

  await page.goto(`${ADMIN_URL}/login`);
  await page.getByLabel(/email address/i).fill(email);
  await page.getByLabel(/password/i).fill(password);
  await page.getByRole("button", { name: /^sign in$/i }).click();

  // Identity Toolkit returns INVALID_LOGIN_CREDENTIALS for unknown
  // accounts; the form maps it to a friendly message.
  await expect(
    page.getByText(/email or password is incorrect/i),
  ).toBeVisible({ timeout: 10_000 });
  await expect(page).not.toHaveURL(/\/dashboard/);

  // Suppress unused-import lint
  void ONBOARDING_URL;
});
