import { expect, test } from "@playwright/test";

import { ADMIN_URL, ONBOARDING_URL, completeOnboarding } from "./helpers";

/**
 * Phase J — cross-app auth handoff.
 *
 * Proves the full merchant journey works end-to-end across the
 * marketing site and the admin app:
 *
 *   1. Fresh context. Visit admin with no cookie → middleware bounces
 *      us to admin's own /login (Phase M restructure).
 *   2. Walk the onboarding golden path on :4201 — this mints a real
 *      session cookie via auth-bff.
 *   3. Navigate to the admin dashboard on :4202 in the SAME context.
 *      The session cookie carries over (Domain=localhost in dev) and
 *      middleware validates it against auth-bff /auth/session.
 *   4. Dashboard renders with the tenant's actual business name pulled
 *      from platform-api — the full session → tenant resolution chain
 *      is exercised.
 *   5. Hit /logout → cookie cleared, follow-up visit to /dashboard
 *      bounces to login again.
 *
 * If this test fails, one of:
 *   - auth-bff /auth/session is broken
 *   - the cookie domain isn't visible across ports
 *   - the admin middleware headers aren't reaching the dashboard
 *   - platform-api /internal/tenants/:id is broken
 *   - the logout route isn't clearing the cookie
 */
test("onboarding → admin → dashboard with real tenant → logout", async ({
  page,
  request,
}) => {
  // 1. Anonymous admin visit → bounce to admin's own /login (Phase M).
  const anon = await page.goto(`${ADMIN_URL}/dashboard`);
  expect(anon?.status()).toBe(200);
  await expect(page).toHaveURL(/localhost:4202\/login/);

  // 2. Walk onboarding to welcome — this mints the session cookie.
  const details = await completeOnboarding(page, request, "handoff");

  // 3. Navigate to the admin. Same context = same cookie jar.
  await page.goto(`${ADMIN_URL}/dashboard`);
  await expect(page).toHaveURL(new RegExp(`${ADMIN_URL.replace(/\//g, "\\/")}/dashboard`));

  // 4. Dashboard shows the tenant's real business name.
  await expect(
    page.getByRole("heading", { name: new RegExp(details.businessName, "i") }),
  ).toBeVisible({ timeout: 10_000 });

  // 5. Logout → cookie cleared → dashboard re-gates.
  await page.goto(`${ADMIN_URL}/logout`);
  await expect(page).toHaveURL(new RegExp(ONBOARDING_URL.replace(/\//g, "\\/")));

  await page.goto(`${ADMIN_URL}/dashboard`);
  await expect(page).toHaveURL(/localhost:4202\/login/);
});
