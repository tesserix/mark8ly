import { defineConfig } from "@playwright/test";

/**
 * Unit-test config, separate from the e2e one.
 *
 * `playwright.config.ts` drives browser specs that need the whole local
 * stack (Postgres, platform-api, auth-bff, the dev server) already up. The
 * specs under tests/unit are plain Node — no browser, no fixtures, no
 * running services — so they get their own config and their own testDir and
 * can run anywhere, including CI without infrastructure.
 *
 * The runner is @playwright/test purely because it is already a declared
 * devDependency of this app; adding a second test framework would mean
 * touching the root lockfile.
 */
export default defineConfig({
  testDir: "./tests/unit",
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  reporter: process.env.CI ? "github" : "list",
});
