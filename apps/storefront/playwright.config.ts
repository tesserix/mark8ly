import { defineConfig, devices } from "@playwright/test";

export default defineConfig({
  testDir: "./tests/e2e",
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: 1,
  reporter: process.env.CI ? "github" : "list",
  use: {
    baseURL:
      process.env.STOREFRONT_BASE_URL ?? "http://localhost:4203",
    trace: "on-first-retry",
    screenshot: "only-on-failure",
  },
  projects: [
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"] },
      testIgnore: /\.mobile\.spec\.ts$/,
    },
    {
      // Phone-sized checks (tesserix/mark8ly#995). Only *.mobile.spec.ts
      // files run here, so the desktop suite is not doubled; those specs
      // set their own exact viewport widths on top of the device profile.
      name: "mobile-chromium",
      use: { ...devices["Pixel 7"] },
      testMatch: /\.mobile\.spec\.ts$/,
    },
  ],
});
