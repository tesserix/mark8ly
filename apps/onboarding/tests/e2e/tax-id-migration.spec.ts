import { readFileSync } from "node:fs";
import { join } from "node:path";
import { expect, test } from "@playwright/test";

// The migration fast-path UI is gated behind a feature flag that is
// deliberately off (OnboardingForm.tsx:43): "Migration fast-path (§5.1.1)
// is hidden from the UI until the flow is tested end-to-end — everyone
// signs up as a new store. Flip to true to restore the question; all
// validation and submit wiring stays intact." While the flag is false,
// the radio group and evidence panel in the "migration fast-path"
// describe block below simply do not render, so those tests are skipped
// rather than treated as failures. Read the flag's current value out of
// the source file at test time (same shape as
// apps/onboarding/tests/unit/dev-stack-config.spec.ts) so that flipping
// the flag back to true automatically re-enables these tests — no one
// has to remember to touch this spec.
const formSource = readFileSync(
  join(
    __dirname,
    "../../components/onboarding/OnboardingForm.tsx",
  ),
  "utf8",
);
const migrationFlagMatch = formSource.match(
  /const MIGRATION_UI_ENABLED = (true|false);/,
);
if (!migrationFlagMatch) {
  throw new Error(
    "Could not find `const MIGRATION_UI_ENABLED = true|false;` in OnboardingForm.tsx — " +
      "update this regex if the flag declaration changed shape.",
  );
}
const MIGRATION_UI_ENABLED = migrationFlagMatch[1] === "true";

/**
 * P16 Task 12 — Tax-ID field + §5.1.1 migration fast-path evidence.
 *
 * Tests the additions to the single-page onboarding form:
 *   - Tax ID field renders and shows country-aware help text
 *   - Migration radio defaults to "new store"
 *   - Switching to "migrating" reveals the evidence panel
 *   - Submitting "migrating" without either evidence field → inline error
 *   - Submitting "migrating" with a WHOIS URL → no migration error
 *   - Submitting "new store" → evidence panel hidden, no migration fields in payload
 *
 * These are UI-level checks. Backend contract tests (tax-ID validation,
 * draft storage) live in the Go integration suite.
 */

test.describe("tax ID field", () => {
  test("renders with fallback help text when no country is selected", async ({
    page,
  }) => {
    await page.goto("/onboarding");

    const taxField = page.locator("#taxId");
    await expect(taxField).toBeVisible();

    // Fallback hint: no country selected yet
    await expect(
      page.getByText(/business tax or vat registration number/i),
    ).toBeVisible();
  });

  test("shows GB-specific help text when country is United Kingdom", async ({
    page,
  }) => {
    await page.goto("/onboarding");

    await page.getByLabel(/country/i).click();
    await page.getByRole("option", { name: /united kingdom/i }).click();

    await expect(page.getByText(/UK VAT Registration Number/i)).toBeVisible();
  });

  test("shows IN-specific help text when country is India", async ({
    page,
  }) => {
    await page.goto("/onboarding");

    await page.getByLabel(/country/i).click();
    await page.getByRole("option", { name: /india/i }).click();

    await expect(page.getByText(/GSTIN/i)).toBeVisible();
  });

  test("shows US-specific help text when country is United States", async ({
    page,
  }) => {
    await page.goto("/onboarding");

    await page.getByLabel(/country/i).click();
    await page.getByRole("option", { name: /united states/i }).click();

    await expect(page.getByText(/Employer Identification Number/i)).toBeVisible();
  });
});

test.describe("migration fast-path (§5.1.1)", () => {
  test.beforeEach(async () => {
    // See the top-of-file comment: OnboardingForm.tsx:43 hides this UI
    // behind MIGRATION_UI_ENABLED = false on purpose. This is not a bug —
    // do not remove this skip to "fix" a failure; flip the flag instead
    // once the flow has been tested end-to-end.
    test.skip(
      !MIGRATION_UI_ENABLED,
      "migration fast-path UI is behind MIGRATION_UI_ENABLED (OnboardingForm.tsx:43), currently off",
    );
  });

  test("migration radio defaults to 'new store'", async ({ page }) => {
    await page.goto("/onboarding");

    const newRadio = page.getByRole("radio", {
      name: /this is a new store/i,
    });
    await expect(newRadio).toBeChecked();

    // Evidence panel must not be visible by default
    await expect(
      page.locator("[data-testid='migration-evidence-panel']"),
    ).not.toBeVisible();
  });

  test("switching to 'migrating' reveals the evidence panel", async ({
    page,
  }) => {
    await page.goto("/onboarding");

    await page.getByRole("radio", {
      name: /i'm migrating from an existing store/i,
    }).click();

    await expect(
      page.locator("[data-testid='migration-evidence-panel']"),
    ).toBeVisible();

    // Both evidence inputs must be present
    await expect(page.locator("#whoisUrl")).toBeVisible();
    await expect(page.locator("#screenshotFile")).toBeAttached();
  });

  test("switching back to 'new store' hides the evidence panel", async ({
    page,
  }) => {
    await page.goto("/onboarding");

    await page.getByRole("radio", {
      name: /i'm migrating from an existing store/i,
    }).click();
    await expect(
      page.locator("[data-testid='migration-evidence-panel']"),
    ).toBeVisible();

    await page.getByRole("radio", { name: /this is a new store/i }).click();
    await expect(
      page.locator("[data-testid='migration-evidence-panel']"),
    ).not.toBeVisible();
  });

  test("submitting 'migrating' without evidence shows the required error", async ({
    page,
  }) => {
    await page.goto("/onboarding");

    // Fill required fields so validation reaches the migration check
    await page.getByLabel(/email address/i).fill("founder@example.com");
    await page.getByLabel(/business name/i).fill("Acme Co");

    const slugInput = page.locator("#slug");
    await slugInput.fill("acme-migrating-test");
    // Wait for availability — in isolation this will show "checking" or
    // "taken"; the error we care about is the migration one, not slug.
    // We trigger submit via keyboard to fire validation without waiting
    // for slug to settle.

    await page.getByLabel(/country/i).click();
    await page.getByRole("option", { name: /united states/i }).click();

    await page.getByRole("radio", {
      name: /i'm migrating from an existing store/i,
    }).click();

    // Submit the form — slug will likely block actual submission, but
    // the migration validation fires independently and shows the error.
    await page.getByRole("button", { name: /send verification link/i }).click();

    // The cross-field error from superRefine fires on submit attempt
    await expect(
      page.locator("[data-testid='migration-required-error']"),
    ).toBeVisible({ timeout: 3000 });
    await expect(
      page.getByText(/provide either a store url or a screenshot/i),
    ).toBeVisible();
  });

  test("filling WHOIS URL clears the migration required error", async ({
    page,
  }) => {
    await page.goto("/onboarding");

    await page.getByLabel(/email address/i).fill("founder@example.com");
    await page.getByLabel(/business name/i).fill("Acme Co");
    await page.locator("#slug").fill("acme-whois-test");

    await page.getByLabel(/country/i).click();
    await page.getByRole("option", { name: /united states/i }).click();

    await page.getByRole("radio", {
      name: /i'm migrating from an existing store/i,
    }).click();

    // Trigger validation to produce the error first
    await page.getByRole("button", { name: /send verification link/i }).click();
    await expect(
      page.locator("[data-testid='migration-required-error']"),
    ).toBeVisible({ timeout: 3000 });

    // Now fill the WHOIS URL — the cross-field error should clear on revalidation
    await page.locator("#whoisUrl").fill("https://myshopifystore.myshopify.com");
    // Trigger revalidation
    await page.locator("#whoisUrl").blur();

    await expect(
      page.locator("[data-testid='migration-required-error']"),
    ).not.toBeVisible({ timeout: 3000 });
  });
});
