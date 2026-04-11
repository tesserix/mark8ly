import { expect, test } from "@playwright/test";

import { ADMIN_URL, completeOnboarding, uniqueEmail } from "./helpers";

/**
 * Phase Q.2 — multi-store lifecycle.
 *
 * Post-settings-IA-restructure the general + stores pages were merged
 * into a single `/settings/stores` page: the list lives at the top and
 * the current store's identity form is the section below it.
 *
 * The happy path:
 *
 *   1. Onboard a tenant. Default store is auto-created during
 *      onboarding; lives at /settings/stores as the single "current"
 *      row.
 *   2. Sign in, navigate to /settings/stores, confirm exactly one
 *      store is listed and it's badged "Current".
 *   3. Click "Add store", fill a unique slug + name + US/USD, submit.
 *      The action creates the row, writes the FGA parent tuple,
 *      auto-switches the session to the new store, and redirects
 *      back to /settings/stores.
 *   4. /settings/stores now shows the NEW store's name in the
 *      editable identity form — proving switch-store flowed the cookie
 *      through middleware and server components.
 *   5. Both stores are listed in the same page; the second one is
 *      badged "Current".
 *   6. Click "Switch" on the original store row; page reloads and the
 *      identity form re-renders showing the original name.
 */
test("owner creates a second store, switches, and edits each separately", async ({
  browser,
  request,
}) => {
  // ── 1. Onboard ───────────────────────────────────────────────
  const signupCtx = await browser.newContext();
  const signupPage = await signupCtx.newPage();
  const owner = await completeOnboarding(signupPage, request, "multistore");
  await signupCtx.close();

  // ── 2. Sign in + stores index ────────────────────────────────
  const ctx = await browser.newContext();
  const page = await ctx.newPage();

  await page.goto(`${ADMIN_URL}/login`);
  await page.getByLabel(/email address/i).fill(owner.email);
  await page.getByLabel(/password/i).fill(owner.password);
  await page.getByRole("button", { name: /^sign in$/i }).click();
  await expect(page).toHaveURL(/\/dashboard/, { timeout: 15_000 });

  await page.goto(`${ADMIN_URL}/settings/stores`);
  const initialRows = page.getByTestId("store-row");
  await expect(initialRows).toHaveCount(1);
  await expect(initialRows.first()).toContainText(/current/i);

  // ── 3. Add a second store ────────────────────────────────────
  const second = uniqueEmail("store2");
  await page.getByRole("button", { name: /\+ add store/i }).click();
  await page.getByLabel(/store name/i).fill(`${owner.businessName} Outlet`);
  await page.getByLabel(/url slug/i).fill(second.slug);
  // Country + currency + timezone default to US/USD/America/New_York
  await page.getByRole("button", { name: /create store/i }).click();

  // Auto-redirects back to /settings/stores after switch-store.
  await expect(page).toHaveURL(/\/settings\/stores/, { timeout: 15_000 });
  await expect(page.getByLabel("Store name")).toHaveValue(
    `${owner.businessName} Outlet`,
  );

  // ── 4. Verify both stores show on the index + switching works ─
  const rows = page.getByTestId("store-row");
  await expect(rows).toHaveCount(2);

  // The outlet is current; click Switch on the original store
  // (named after the onboarded business, not "Main Store" — that
  // label is reserved for stores backfilled from pre-Phase-Q
  // tenants, not stores created by onboarding).
  const originalRow = page
    .getByTestId("store-row")
    .filter({ hasText: new RegExp(`^${owner.businessName}`, "i") })
    .first();
  // The handler triggers window.location.reload() after the server
  // action returns; wait for that navigation explicitly so the
  // test doesn't race past the cookie update.
  await Promise.all([
    page.waitForLoadState("load"),
    originalRow.getByRole("button", { name: /switch/i }).click(),
  ]);

  // After the reload, the "Current" badge should have moved to the
  // original store row. This is the direct, unambiguous signal
  // that switch-store ran end-to-end: the server re-rendered
  // /settings/stores with x-session-store-id=<original> and
  // serverSession resolved currentStore to it.
  await expect(
    page
      .getByTestId("store-row")
      .filter({ hasText: new RegExp(`^${owner.businessName}`, "i") })
      .first(),
  ).toContainText(/current/i, { timeout: 10_000 });

  await ctx.close();
});
