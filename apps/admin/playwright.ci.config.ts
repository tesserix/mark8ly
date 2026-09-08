import { defineConfig, devices } from "@playwright/test";

/**
 * CI-only Playwright config for admin.
 *
 * Unlike playwright.config.ts (which assumes `make dev` is already up),
 * this config serves the app itself via `webServer`, so it can run
 * standalone in a workflow with no backend stack. That only works for
 * specs that assert client-side state with no fetch dependency — see
 * .github/workflows/e2e-runnable.yml for why exactly one spec is listed
 * here.
 *
 * `webServer` SERVES ONLY — it does not build. `npm run build` is a
 * separate step in that workflow, on purpose: a cold Next 16 admin build on
 * ubuntu-latest routinely takes longer than any sane `webServer.timeout`,
 * so building in here reds the job on a build clock rather than on
 * anything the spec asserts. The step split also puts a build failure in
 * its own log line instead of burying it in a webServer timeout. Run
 * `npm run build` yourself before using this config locally.
 *
 * Port 4202 matches this app's own `start` script (`next start --port
 * 4202`) — do not invent a different port.
 */
export default defineConfig({
  testDir: "./tests/e2e",
  testMatch: ["pricing.spec.ts"],
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  // Deliberately 0, including on CI. The workflow gate asserts an exact
  // pass count, and a retry turns "5 passed" into "4 passed, 1 flaky" —
  // which reds the gate anyway, having spent a second run to get there.
  // A retry here can only cost runner minutes; it can never turn the job
  // green.
  retries: 0,
  workers: 1,
  reporter: process.env.CI ? "github" : "list",
  use: {
    // helpers.ts (which pricing.spec.ts navigates through) reads ADMIN_URL,
    // not ADMIN_BASE_URL, and builds absolute URLs — so `baseURL` is not
    // actually consulted by this spec. Reading the same variable keeps the
    // two from disagreeing about where the run is pointed.
    baseURL: process.env.ADMIN_URL ?? "http://localhost:4202",
    // With retries: 0, "on-first-retry" would never fire — there is no
    // first retry. Capture on failure instead, and upload it: see the
    // artifact step in .github/workflows/e2e-runnable.yml. Produced and
    // discarded is the same as not produced, only slower.
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
    video: "retain-on-failure",
  },
  webServer: {
    command: "npm run start",
    url: "http://localhost:4202/pricing",
    reuseExistingServer: false,
    // Serving a prebuilt .next only has to boot `next start` and render one
    // page, so this is generous. It is NOT a build budget — see above.
    timeout: 60_000,
  },
  projects: [
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"] },
    },
  ],
});
