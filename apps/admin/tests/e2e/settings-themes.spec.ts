import { expect, test } from "@playwright/test";

import {
  completeOnboarding,
  signInAsOwner,
} from "./helpers";

/**
 * Post-settings-IA-restructure `/settings/storefront` was renamed to
 * `/settings/themes`; the page headline is "Branding"
 * (apps/admin/app/(admin)/settings/themes/page.tsx).
 * Old route redirects but the test navigates to the new URL directly.
 */
test("settings/themes saves and persists layout choice", async ({
  browser,
  request,
}) => {
  const signupCtx = await browser.newContext();
  const signupPage = await signupCtx.newPage();
  const details = await completeOnboarding(signupPage, request, "themes");
  await signupCtx.close();

  const ctx = await signInAsOwner(browser, request, details);
  const page = await ctx.newPage();

  await page.goto(`/settings/themes`);
  await expect(
    page.getByRole("heading", { name: /branding/i, level: 1 }),
  ).toBeVisible();

  const layoutButton = page.getByTestId("layout-bold-promo");
  await layoutButton.click();
  await expect(layoutButton).toHaveAttribute("aria-pressed", "true");

  await page.getByTestId("save-storefront-theme").click();
  await expect(page.getByRole("status")).toContainText(/saved/i);

  await page.reload();
  await expect(page.getByTestId("layout-bold-promo")).toHaveAttribute(
    "aria-pressed",
    "true",
  );

  await ctx.close();
});
