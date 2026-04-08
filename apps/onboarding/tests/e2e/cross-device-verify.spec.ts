import { expect, test } from "@playwright/test";

import { fetchMagicLinkToken, uniqueEmail } from "./helpers";

/**
 * Phase L — cross-tab/cross-device magic link.
 *
 * The Phase L refactor moved the GIP credentials from per-tab
 * sessionStorage into the persisted onboarding session draft. This
 * test proves that change works:
 *
 *   1. Submit the form in browser context A.
 *   2. Land on /onboarding/check-inbox.
 *   3. Open a SECOND browser context (no shared storage) and visit the
 *      magic-link URL there.
 *   4. The verify page reads everything from the server-persisted
 *      draft and lands on /welcome.
 *
 * Before this change, step 3 always failed with "We couldn't find your
 * in-progress onboarding session" because the second context had no
 * sessionStorage. If this test ever fails, the cross-tab/cross-device
 * regression is back.
 */
test("magic link works in a fresh browser context (cross-device)", async ({
  browser,
  request,
}) => {
  const details = uniqueEmail("xdev");

  // ── Context A: submit the form ────────────────────────────────────────
  const a = await browser.newContext();
  const aPage = await a.newPage();

  await aPage.goto("http://localhost:4201/onboarding");
  await aPage.getByLabel(/email address/i).fill(details.email);
  await aPage.getByLabel(/business name/i).fill(details.businessName);
  await aPage.locator("#slug").fill(details.slug);
  await expect(aPage.getByText(/✓ available/i)).toBeVisible({ timeout: 5000 });

  await aPage.getByLabel(/country/i).click();
  await aPage.getByRole("option", { name: /united states/i }).click();
  const currencyTrigger = aPage.getByLabel(/currency/i);
  if ((await currencyTrigger.textContent())?.includes("Select")) {
    await currencyTrigger.click();
    await aPage.getByRole("option", { name: /usd/i }).first().click();
  }
  await aPage.getByRole("button", { name: /get my store ready/i }).click();
  await expect(aPage).toHaveURL(/\/onboarding\/check-inbox/, {
    timeout: 10_000,
  });

  // Pull the magic-link token via the test helper.
  const token = await fetchMagicLinkToken(request, details.email);

  // Close context A entirely so its sessionStorage cannot influence
  // the verify run. The whole point is "different browser, different
  // device" — leaving A open might mask the bug.
  await a.close();

  // ── Context B: open the magic link in a fresh browser ────────────────
  const b = await browser.newContext();
  const bPage = await b.newPage();
  await bPage.goto(
    `http://localhost:4201/onboarding/verify?token=${encodeURIComponent(token)}`,
  );

  // Phase M: cross-device verify lands on /onboarding/set-password in
  // the second context, since the credential collection now happens
  // post-verify. The set-password page server-fetches the session draft
  // — that's the cross-device safety guarantee we're proving here.
  await expect(bPage).toHaveURL(/\/onboarding\/set-password/, {
    timeout: 15_000,
  });
  await bPage.locator("#password").fill(details.password);
  await bPage.getByRole("button", { name: /create account/i }).click();
  await expect(bPage).toHaveURL(/\/welcome/, { timeout: 15_000 });
  await expect(
    bPage.getByRole("link", { name: /open admin dashboard/i }),
  ).toBeVisible();

  await b.close();
});
