import { expect, test } from "@playwright/test";

import { ADMIN_URL, completeOnboarding } from "./helpers";

/**
 * M7a — Products list page.
 *
 * Exercises the minimum contract of the real products list page:
 *
 *   1. Onboard a fresh merchant via the shared helper.
 *   2. Sign in, land on /dashboard.
 *   3. Navigate to /products — because the fresh tenant has no
 *      products yet, assert the editorial empty state renders with
 *      the + New product CTA visible (admin role).
 *   4. Click the CTA → asserts the /products/new stub page loads.
 *   5. Back on /products, verify the search input and status filter
 *      render (the filter row is the only interactive surface in the
 *      empty state).
 *
 * Full seeded-catalogue coverage (pagination, status filter behavior,
 * variant pricing, overflow menu) lives in follow-up milestones that
 * have a way to seed products through the new admin API.
 */
test("products list renders the empty state for a fresh tenant", async ({
  browser,
  request,
}) => {
  const signupCtx = await browser.newContext();
  const signupPage = await signupCtx.newPage();
  const details = await completeOnboarding(signupPage, request, "m7a");
  await signupCtx.close();

  const ctx = await browser.newContext();
  const page = await ctx.newPage();

  await page.goto(`${ADMIN_URL}/login`);
  await page.getByLabel(/email address/i).fill(details.email);
  await page.getByLabel(/password/i).fill(details.password);
  await page.getByRole("button", { name: /^sign in$/i }).click();
  await expect(page).toHaveURL(/\/dashboard/, { timeout: 15_000 });

  await page.goto(`${ADMIN_URL}/products`);

  // Editorial header
  await expect(
    page.getByRole("heading", { name: /^products$/i, level: 1 }),
  ).toBeVisible();

  // Empty state copy (no-products variant)
  await expect(
    page.getByRole("heading", { name: /no products yet/i }),
  ).toBeVisible();

  // + New product CTA — admin can see it
  const newProductCta = page.getByRole("link", { name: /new product/i }).first();
  await expect(newProductCta).toBeVisible();

  // Filter surface is present even in the empty state
  await expect(page.getByPlaceholder(/search products/i)).toBeVisible();
  await expect(page.getByLabel(/filter by status/i)).toBeVisible();

  // CTA navigates to the stub detail page
  await newProductCta.click();
  await expect(page).toHaveURL(/\/products\/new/);
  await expect(
    page.getByRole("heading", { name: /new product/i }),
  ).toBeVisible();

  await ctx.close();
});
