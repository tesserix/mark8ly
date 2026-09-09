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

  // /settings/themes was restructured into sections — a "Branding sections"
  // nav with Identity / Theme / Homepage / Pages / Footer / SEO / Policies /
  // Advanced. The layout picker moved under "Theme"; this spec predated that
  // and expected it on the landing section, so it timed out clicking a
  // control that was never on screen.
  await page
    .getByRole("navigation", { name: /branding sections/i })
    .getByRole("button", { name: /^theme$/i })
    .click();

  const layoutButton = page.getByTestId("layout-bold-promo");
  await layoutButton.click();
  await expect(layoutButton).toHaveAttribute("aria-pressed", "true");

  await page.getByTestId("save-storefront-theme").click();
  // Two live regions on this page now (the section shell has one of its
  // own), so a bare getByRole("status") is a strict-mode violation. Filter
  // to the one carrying the save confirmation rather than taking .first(),
  // which would pass even if the toast never appeared.
  // .first() is safe only because the filter already proves the text is
  // there: if the toast never rendered, the filtered set would be empty and
  // this would fail rather than silently pass on a nested wrapper.
  await expect(
    page.getByRole("status").filter({ hasText: /saved/i }).first(),
  ).toBeVisible();

  await page.reload();
  // The active section is component state, not part of the URL, so a reload
  // lands back on Identity — re-open Theme before asserting persistence.
  // (This is what makes the assertion meaningful: it proves the LAYOUT
  // survived the round trip, not that the section did.)
  await page
    .getByRole("navigation", { name: /branding sections/i })
    .getByRole("button", { name: /^theme$/i })
    .click();
  await expect(page.getByTestId("layout-bold-promo")).toHaveAttribute(
    "aria-pressed",
    "true",
  );

  await ctx.close();
});
