import { expect, test } from "@playwright/test";

import { fetchMagicLinkToken, uniqueEmail } from "./helpers";

/**
 * Phase G — Browser-close-and-resume.
 *
 * Real users submit the form on their laptop, close the tab, and click
 * the magic link from their phone or after lunch. Between submit and
 * verify, the onboarding store (with the slug, country, currency, and
 * GIP refresh token) lives in sessionStorage.
 *
 * This test simulates the close-and-reopen sequence:
 *   1. Open a fresh browser context, fill the form, submit.
 *   2. Land on /onboarding/check-inbox.
 *   3. Snapshot the storage state and close the context completely.
 *   4. Open a new context, hydrate it with the snapshot.
 *   5. Visit the verify URL with the magic-link token.
 *   6. Assert the welcome page renders.
 *
 * If this ever fails, sessionStorage isn't being preserved by the
 * snapshot or the verify page isn't waiting for hydration before reading
 * the store. (The hydration race was the original Phase G fix on
 * VerifyMagicLink — this test prevents it from regressing.)
 */
test("survives full browser close between submit and verify", async ({
  browser,
  request,
}) => {
  const { email, slug, businessName, password } = uniqueEmail("resume");

  // ── 1. First context: complete the form ───────────────────────────────
  const first = await browser.newContext();
  const firstPage = await first.newPage();

  await firstPage.goto("http://localhost:4201/onboarding");
  await firstPage.getByLabel(/email address/i).fill(email);
  await firstPage.getByLabel(/business name/i).fill(businessName);
  await firstPage.locator("#slug").fill(slug);
  await expect(firstPage.getByText(/✓ available/i)).toBeVisible({
    timeout: 5000,
  });

  await firstPage.getByLabel(/country/i).click();
  await firstPage.getByRole("option", { name: /united states/i }).click();
  const currencyTrigger = firstPage.getByLabel(/currency/i);
  if ((await currencyTrigger.textContent())?.includes("Select")) {
    await currencyTrigger.click();
    await firstPage.getByRole("option", { name: /usd/i }).first().click();
  }

  await firstPage.getByRole("button", { name: /send verification link/i }).click();
  await expect(firstPage).toHaveURL(/\/onboarding\/check-inbox/, {
    timeout: 10_000,
  });

  // ── 2. Snapshot storage and close everything ─────────────────────────
  // sessionStorage isn't captured by the default storageState() — it's
  // explicitly excluded because it's not meant to survive context
  // boundaries. We capture it manually and re-inject in the second
  // context to simulate the same browser tab being reopened.
  const sessionEntries = await firstPage.evaluate(() => {
    const out: Array<[string, string]> = [];
    for (let i = 0; i < window.sessionStorage.length; i++) {
      const key = window.sessionStorage.key(i);
      if (key) out.push([key, window.sessionStorage.getItem(key) ?? ""]);
    }
    return out;
  });
  expect(
    sessionEntries.length,
    "expected onboarding store to write at least one sessionStorage entry",
  ).toBeGreaterThan(0);

  await first.close();

  // ── 3. Second context: rehydrate sessionStorage on the same origin ───
  const second = await browser.newContext();
  const secondPage = await second.newPage();

  // sessionStorage is per-tab AND per-origin, so we have to navigate to
  // the origin first before we can write to it. A blank page on the same
  // origin is the cheapest seed.
  await secondPage.goto("http://localhost:4201/");
  await secondPage.evaluate((entries) => {
    for (const [k, v] of entries) {
      window.sessionStorage.setItem(k, v);
    }
  }, sessionEntries);

  // ── 4. Fetch the magic link token and visit verify ───────────────────
  const token = await fetchMagicLinkToken(request, email);
  await secondPage.goto(
    `http://localhost:4201/onboarding/verify?token=${encodeURIComponent(token)}`,
  );

  // ── 5. Set-password step (Phase M) → welcome page renders cold ───────
  await expect(secondPage).toHaveURL(/\/onboarding\/set-password/, {
    timeout: 15_000,
  });
  await secondPage.locator("#password").fill(password);
  await secondPage.getByRole("button", { name: /create account/i }).click();
  await expect(secondPage).toHaveURL(/\/welcome/, { timeout: 15_000 });

  await second.close();
});
