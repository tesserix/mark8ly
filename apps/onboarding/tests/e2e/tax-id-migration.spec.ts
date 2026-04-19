import { expect, test } from "@playwright/test";

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
