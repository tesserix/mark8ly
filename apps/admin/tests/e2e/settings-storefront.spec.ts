import { expect, test } from "@playwright/test";

import { ADMIN_URL, completeOnboarding } from "./helpers";

test("settings/storefront saves and persists layout choice", async ({
  browser,
  request,
}) => {
  const signupCtx = await browser.newContext();
  const signupPage = await signupCtx.newPage();
  const details = await completeOnboarding(signupPage, request, "storefront-theme");
  await signupCtx.close();

  const ctx = await browser.newContext();
  const page = await ctx.newPage();

  await page.goto(`${ADMIN_URL}/login`);
  await page.getByLabel(/email address/i).fill(details.email);
  await page.getByLabel(/password/i).fill(details.password);
  await page.getByRole("button", { name: /^sign in$/i }).click();
  await expect(page).toHaveURL(/\/dashboard/, { timeout: 15_000 });

  await page.goto(`${ADMIN_URL}/settings/storefront`);
  await expect(
    page.getByRole("heading", { name: /theme & layout/i }),
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
