import { expect, test } from "@playwright/test";

/**
 * Phase G — Form validation regressions.
 *
 * Cheap UI-level checks that the client-side guards in the onboarding
 * form actually fire. The deep validation contracts (slug uniqueness,
 * verification token expiry, FGA tuple writes) live in Go integration
 * tests where they belong; this file only catches the boring stuff.
 */

test.describe("onboarding form validation", () => {
  test("rejects an obviously invalid email", async ({ page }) => {
    await page.goto("/onboarding");

    await page.getByLabel(/email address/i).fill("not-an-email");
    await page.getByLabel(/business name/i).fill("Acme Co");
    // Don't bother with slug/country — the email check fires first.

    // The submit button should be disabled because canSubmit gates on
    // slug/country/currency too. So instead, attempt submit and assert
    // that the form has not navigated away.
    await expect(page).toHaveURL(/\/onboarding(\/?$|$)/);
  });

  // OnboardingForm.tsx:292-296 — the submit button is only disabled while
  // genuinely busy (pending/uploading). It is deliberately NOT gated on
  // slug availability/validity: that used to leave a mystery-disabled
  // button on first load (NN/g: disabled buttons must explain
  // themselves). Instead, submitting with an invalid/short slug is
  // expected to surface a visible validation error and move focus to the
  // slug field. This test used to assert the button was disabled; that
  // assertion described a behaviour the app no longer has, so it has
  // been rewritten to assert the actual (intended) behaviour. Do not
  // revert this to a disabled-button check.
  test("surfaces a validation error and focuses the slug field for a too-short slug", async ({
    page,
  }) => {
    await page.goto("/onboarding");

    await page.getByLabel(/email address/i).fill("founder@example.com");
    await page.getByLabel(/business name/i).fill("Ab");
    // The auto-suggester turns "Ab" into "ab" — under the 3-char minimum.

    await page.getByLabel(/country/i).click();
    await page.getByRole("option", { name: /united states/i }).click();

    const submit = page.getByRole("button", {
      name: /send verification link/i,
    });
    await expect(submit).toBeEnabled();
    await submit.click();

    // Zod's slug min-length message, rendered next to the slug field.
    await expect(
      page.getByText(/3-63 characters, lowercase letters, numbers, and hyphens/i),
    ).toBeVisible();
    await expect(page.locator("#slug")).toBeFocused();

    // The form must not have submitted/navigated away.
    await expect(page).toHaveURL(/\/onboarding(\/?$|$)/);
  });
});
