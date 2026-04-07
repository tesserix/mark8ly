import { defineConfig, devices } from "@playwright/test";

/**
 * Playwright config for the onboarding e2e suite.
 *
 * Assumes the local stack (`make dev`) is already up — Postgres,
 * platform-api, auth-bff, firebase auth emulator, openfga, and the
 * onboarding Next.js dev server on :4201. The suite never spawns these
 * itself; spawning a full Go/Postgres stack from inside Playwright would
 * couple two unrelated lifecycles and make CI failures harder to debug.
 *
 * Override `BASE_URL` and `API_URL` from the environment if running
 * against a different host (e.g. inside the docker-compose network in
 * CI).
 */
export default defineConfig({
  testDir: "./tests/e2e",
  fullyParallel: false, // golden-path tests share Postgres state
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: 1,
  reporter: process.env.CI ? "github" : "list",
  use: {
    baseURL: process.env.BASE_URL ?? "http://localhost:4201",
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
