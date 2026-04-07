import { defineConfig, devices } from "@playwright/test";

/**
 * Playwright config for the admin e2e suite.
 *
 * The admin tests exercise the CROSS-APP journey: they drive the
 * onboarding form on :4201, wait for the session cookie to be minted,
 * and then navigate to the admin on :4202. The test therefore expects
 * BOTH Next.js dev servers to be running alongside the Go stack.
 *
 * Assumes `make dev` (or equivalent) is already up.
 */
export default defineConfig({
  testDir: "./tests/e2e",
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: 1,
  reporter: process.env.CI ? "github" : "list",
  use: {
    baseURL: process.env.ADMIN_BASE_URL ?? "http://localhost:4202",
    trace: "on-first-retry",
    screenshot: "only-on-failure",
    video: "retain-on-failure",
  },
  projects: [
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"] },
    },
  ],
});
