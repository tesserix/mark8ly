/**
 * Visual-regression for the 8 storefront layouts × 3 content states.
 *
 * Replaces the manual "click through every layout and eyeball it"
 * smoke-walk with Playwright's toHaveScreenshot snapshots. Baselines
 * live next to this file as `<snapshot>.png`; diffs are produced on
 * any pixel drift.
 *
 * Content states:
 *   - empty:   branding.homepage_content = {} → layout renders via
 *              its .defaults.ts recipe
 *   - default: merchant hasn't overridden the recipe (identical to
 *              empty today, kept as a separate case so we catch the
 *              day we change that invariant)
 *   - custom:  a representative merchant-authored payload exercising
 *              text + marquee + pull_quote + letter + featured
 *
 * Seeding: requires a marketplace-api test-only endpoint to PATCH a
 * store's branding + layout_variant by slug. When
 * STOREFRONT_VISUAL_TEST_SLUG is not set, the suite skips — baselines
 * must be generated against a real environment (CI or local stack up).
 */

import { expect, test } from "@playwright/test";

import type {
  HomepageContent,
  StorefrontLayoutKey,
} from "./layout-blocks.fixtures";
import {
  CUSTOM_CONTENT,
  EMPTY_CONTENT,
  LAYOUTS,
} from "./layout-blocks.fixtures";

const MARKETPLACE_API_URL =
  process.env.MARKETPLACE_API_URL ?? "http://localhost:8088";
const STOREFRONT_URL =
  process.env.STOREFRONT_BASE_URL ?? "http://localhost:4203";
const TEST_SLUG = process.env.STOREFRONT_VISUAL_TEST_SLUG ?? "";

test.skip(
  !TEST_SLUG,
  "Set STOREFRONT_VISUAL_TEST_SLUG to run visual-regression against a seeded store " +
    "(also requires marketplace-api running with MARKETPLACE_API_ENABLE_TEST_ROUTES=true)",
);

type ContentState = "empty" | "default" | "custom";

const CONTENT_STATES: Array<{ key: ContentState; content: HomepageContent }> = [
  { key: "empty", content: EMPTY_CONTENT },
  { key: "default", content: EMPTY_CONTENT }, // same as empty today; separate case reserves the slot
  { key: "custom", content: CUSTOM_CONTENT },
];

async function seedBranding(
  request: import("@playwright/test").APIRequestContext,
  slug: string,
  layout: StorefrontLayoutKey,
  content: HomepageContent,
): Promise<void> {
  // Hits the marketplace-api test-only route:
  //   POST /api/v1/test/branding
  //   body: { slug, layout_variant, homepage_content }
  // The route is mounted only when MARKETPLACE_API_ENABLE_TEST_ROUTES=true.
  // Implementation: services/marketplace-api/internal/handlers/testroutes.
  const res = await request.post(`${MARKETPLACE_API_URL}/api/v1/test/branding`, {
    data: {
      slug,
      layout_variant: layout,
      homepage_content: content,
    },
  });
  if (!res.ok()) {
    throw new Error(
      `seedBranding failed: ${res.status()} ${await res.text()} — ` +
        `is marketplace-api running with MARKETPLACE_API_ENABLE_TEST_ROUTES=true?`,
    );
  }
}

for (const layout of LAYOUTS) {
  for (const { key: stateKey, content } of CONTENT_STATES) {
    test(`layout=${layout} state=${stateKey} renders without regression`, async ({
      page,
      request,
    }) => {
      await seedBranding(request, TEST_SLUG, layout, content);
      await page.goto(`${STOREFRONT_URL}/?slug=${TEST_SLUG}`);
      // Let lazy images + fonts settle before snapping.
      await page.waitForLoadState("networkidle");
      await expect(page).toHaveScreenshot(`${layout}-${stateKey}.png`, {
        fullPage: true,
        animations: "disabled",
        maxDiffPixelRatio: 0.01,
      });
    });
  }
}
