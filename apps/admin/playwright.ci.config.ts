import { defineConfig, devices } from "@playwright/test";

/**
 * CI-only Playwright config for admin.
 *
 * Unlike playwright.config.ts (which assumes `make dev` is already up),
 * this config builds and serves the app itself via `webServer`, so it can
 * run standalone in a workflow with no backend stack. That only works for
 * specs that assert client-side state with no fetch dependency — see
 * .github/workflows/e2e-runnable.yml for why exactly one spec is listed
 * here.
 *
 * Port 4202 matches this app's own `start` script (`next start --port
 * 4202`) — do not invent a different port.
 */
export default defineConfig({
  testDir: "./tests/e2e",
  testMatch: ["pricing.spec.ts"],
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
  webServer: {
    command: "npm run build && npm run start",
    url: "http://localhost:4202/pricing",
    reuseExistingServer: false,
    timeout: 180_000,
  },
  projects: [
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"] },
    },
  ],
});
