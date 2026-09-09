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
// Admin's middleware bounces an authenticated merchant to their slug
// subdomain, and canonical /login 404s without an https slug returnUrl —
// so the specs have to BE on `{slug}-admin.mark8ly.com`. Chromium maps it
// to the local server; no DNS entry, no TLS, no hosts-file edit, and
// nothing in the app is relaxed to accommodate the test (#858).
const HOST_MAP = [
  "--host-resolver-rules=MAP *-admin.mark8ly.com 127.0.0.1:4202",
];

export default defineConfig({
  testDir: "./tests/e2e",
  // 5s (the default) was calibrated when these specs mocked their backend.
  // Against a real stack the first render for a brand-new tenant has to
  // reach marketplace-api and platform-api, and the assertion regularly
  // fires before the RSC stream lands -- producing "element(s) not found"
  // on headings that demonstrably render (#858).
  expect: { timeout: 15_000 },
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
    launchOptions: { args: HOST_MAP },
  },
  projects: [
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"] },
    },
  ],
});
