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
// lib/config reads these once, at module-load time. The config file is
// evaluated before any test module in every worker process, so this is the
// only place the values are guaranteed to be in place early enough. They are
// throwaway stand-ins — no fetch in these specs ever leaves the process.
process.env.NEXT_PUBLIC_GIP_API_KEY ||= "test-api-key";
process.env.NEXT_PUBLIC_GIP_TENANT_ID ||= "test-tenant";

export default defineConfig({
  testDir: "./tests/unit",
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  reporter: process.env.CI ? "github" : "list",
});
