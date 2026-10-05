import { expect, test } from "@playwright/test";

import { STOREFRONT_URL, onboardStore, stamp } from "./helpers";

/**
 * Issue #995 — on a 400px phone the demo store's brand rendered as
 * "The B…" and the nav controls were crowded into the ~304px left over
 * after two stacked page gutters. This runs only under the mobile
 * Playwright project (see playwright.config.ts) and checks the header at
 * the widths the issue names.
 *
 * The merchant is onboarded inline with a deliberately long name so the
 * wrap behaviour is exercised, not just the happy short-name case.
 */

const WIDTHS = [320, 375, 400] as const;
const LONG_NAME = "The Bondi Beachside Ceramics and Homewares Studio";

test("brand reads in full and nothing overflows on narrow screens", async ({
  page,
  request,
}) => {
  const suffix = stamp();
  const merchant = {
    email: `nav-mobile-${suffix}@example.com`,
    slug: `nav-mobile-${suffix}`.replace(/[^a-z0-9-]/g, "").slice(0, 60),
    businessName: LONG_NAME,
    password: "e2e-test-password-123",
  };
  await onboardStore(page, request, merchant);

  for (const width of WIDTHS) {
    await page.setViewportSize({ width, height: 606 });
    await page.goto(`${STOREFRONT_URL}/?slug=${merchant.slug}`);

    const nav = page.getByRole("navigation", { name: "Store" });
    await expect(nav).toBeVisible();

    // No horizontal scrolling at any of the tested widths.
    const overflow = await page.evaluate(() => ({
      scrollWidth: document.documentElement.scrollWidth,
      clientWidth: document.documentElement.clientWidth,
    }));
    expect(
      overflow.scrollWidth,
      `page overflows horizontally at ${width}px`,
    ).toBeLessThanOrEqual(overflow.clientWidth);

    // The brand is the full store name, wrapped rather than clipped.
    const brand = nav.getByRole("link", { name: LONG_NAME });
    await expect(brand).toBeVisible();
    const brandFits = await brand.evaluate((el) => {
      const style = getComputedStyle(el);
      return (
        el.scrollWidth <= el.clientWidth + 1 &&
        style.textOverflow !== "ellipsis" &&
        style.whiteSpace !== "nowrap"
      );
    });
    expect(brandFits, `brand is clipped at ${width}px`).toBe(true);

    // Every control is on screen and usable.
    for (const name of [/^home$/i, /^shop$/i, /^cart/i]) {
      const link = nav.getByRole("link", { name });
      await expect(link).toBeVisible();
      const box = await link.boundingBox();
      expect(box, `${name} has no box at ${width}px`).not.toBeNull();
      expect(box!.x).toBeGreaterThanOrEqual(0);
      expect(box!.x + box!.width).toBeLessThanOrEqual(width);
    }

    // Anonymous visitor: the bell and account slots must not hold open
    // empty gaps. Any <li> with no rendered content is collapsed.
    const phantomSlots = await nav.evaluate((el) =>
      [...el.querySelectorAll("li")].filter(
        (li) =>
          li.childElementCount === 0 &&
          li.textContent?.trim() === "" &&
          getComputedStyle(li).display !== "none",
      ).length,
    );
    expect(phantomSlots, `empty nav slots take space at ${width}px`).toBe(0);
  }
});
