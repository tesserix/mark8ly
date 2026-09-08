import { defineConfig, devices } from "@playwright/test";

/**
 * Config for the OPERATOR scripts in tests/operator/.
 *
 * These are not tests — they are one-off tools that drive a live system.
 * They need their own config because playwright.config.ts pins
 * `testDir: "./tests/e2e"`, and a `*` in a testDir does not reach a
 * sibling directory: without this file `npx playwright test
 * tests/operator/<spec>.spec.ts` exits "No tests found" — the scripts had
 * no working entry point at all (mark8ly#834).
 *
 * No `webServer`: every script targets a host the caller supplies via env
 * (see tests/operator/README.md), never a server this config could start.
 * No `baseURL` either — each script reads its own host variable and calls
 * `page.goto()` with an absolute URL, so a baseURL here would be unused
 * and would misdescribe where the run is pointed.
 */
export default defineConfig({
  testDir: "./tests/operator",
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  // Never retry: layout-blocks.spec.ts seeds a real store's branding
  // through a test-only API, so a retried step re-writes live data.
  retries: 0,
  workers: 1,
  reporter: "list",
  use: {
    trace: "on-first-retry",
    screenshot: "only-on-failure",
  },
  projects: [
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"] },
    },
  ],
});
