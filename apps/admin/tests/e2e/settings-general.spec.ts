import { expect, test } from "@playwright/test";

import { ADMIN_URL, completeOnboarding } from "./helpers";

/**
 * Phase N — Tenant settings (general) page.
 *
 * Exercises the full round-trip:
 *
 *   1. Onboard a fresh merchant (helpers.completeOnboarding). This
 *      creates a real tenant row with real name/slug/country/currency/
 *      timezone/owner_email — the same data we expect the settings page
 *      to surface.
 *   2. Sign in to admin via email + password, land on /dashboard.
 *   3. Navigate to /settings/general. Assert every onboarding field is
 *      visible on the page — this is the "onboarding details flow
 *      through to admin" contract.
 *   4. Edit the store name, save, assert the success banner.
 *   5. Reload the page and assert the new name persists, proving the
 *      PATCH hit the database and not just component state.
 *
 * If this regresses we've broken either the PATCH endpoint, the
 * server action's session→path-param wiring, or the tenant fetch in
 * getServerSessionContext.
 */
test("settings/general surfaces onboarding data and saves a name edit", async ({
  browser,
  request,
}) => {
  // ── 1. Onboard ──────────────────────────────────────────────────────
  const signupCtx = await browser.newContext();
  const signupPage = await signupCtx.newPage();
  const details = await completeOnboarding(signupPage, request, "settings");
  await signupCtx.close();

  // ── 2. Sign in to admin ─────────────────────────────────────────────
  const ctx = await browser.newContext();
  const page = await ctx.newPage();

  await page.goto(`${ADMIN_URL}/login`);
  await page.getByLabel(/email address/i).fill(details.email);
  await page.getByLabel(/password/i).fill(details.password);
  await page.getByRole("button", { name: /^sign in$/i }).click();
  await expect(page).toHaveURL(/\/dashboard/, { timeout: 15_000 });

  // ── 3. Settings page surfaces every onboarding field ───────────────
  await page.goto(`${ADMIN_URL}/settings/general`);
  await expect(
    page.getByRole("heading", { name: /general settings/i }),
  ).toBeVisible();

  const nameInput = page.getByLabel("Store name");
  await expect(nameInput).toHaveValue(details.businessName);

  // Slug surfaces as a read-only hostname preview
  // ({slug}.mark8ly.com). The storefront lives at the bare
  // subdomain; admin lives at {slug}-admin.mark8ly.com.
  await expect(page.getByLabel("Store URL slug")).toHaveValue(
    `${details.slug}.mark8ly.com`,
  );

  // Owner email is read-only but visible
  await expect(page.getByLabel("Owner email")).toHaveValue(details.email);

  // Country + currency from onboarding (US → USD)
  await expect(page.getByLabel("Country")).toHaveValue("US");
  await expect(page.getByLabel("Currency")).toHaveValue("USD");

  // ── 4. Edit the name, save ──────────────────────────────────────────
  const newName = `${details.businessName} Renamed`;
  await nameInput.fill(newName);
  await page.getByRole("button", { name: /save changes/i }).click();
  await expect(page.getByRole("status")).toHaveText(/saved/i, {
    timeout: 10_000,
  });

  // ── 5. Reload — the new name must come from the DB, not state ──────
  await page.reload();
  await expect(page.getByLabel("Store name")).toHaveValue(newName);

  await ctx.close();
});

test("settings/general rejects an empty store name", async ({
  browser,
  request,
}) => {
  const signupCtx = await browser.newContext();
  const signupPage = await signupCtx.newPage();
  const details = await completeOnboarding(signupPage, request, "empty-name");
  await signupCtx.close();

  const ctx = await browser.newContext();
  const page = await ctx.newPage();

  await page.goto(`${ADMIN_URL}/login`);
  await page.getByLabel(/email address/i).fill(details.email);
  await page.getByLabel(/password/i).fill(details.password);
  await page.getByRole("button", { name: /^sign in$/i }).click();
  await expect(page).toHaveURL(/\/dashboard/, { timeout: 15_000 });

  await page.goto(`${ADMIN_URL}/settings/general`);
  const nameInput = page.getByLabel("Store name");
  await nameInput.fill("   ");

  // Save button enables on any change, but the server action rejects
  // whitespace-only. HTML5 `required` may also block the submit —
  // either path is acceptable, we just assert the name is NOT persisted.
  const saveBtn = page.getByRole("button", { name: /save changes/i });
  if (await saveBtn.isEnabled()) {
    await saveBtn.click();
  }

  // Reload and confirm the original name is still there.
  await page.reload();
  await expect(page.getByLabel("Store name")).toHaveValue(details.businessName);

  await ctx.close();
});
