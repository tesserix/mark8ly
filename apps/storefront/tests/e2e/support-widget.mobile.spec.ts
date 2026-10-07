import { expect, test, type Locator } from "@playwright/test";

import { STOREFRONT_URL, onboardStore, stamp } from "./helpers";

/**
 * Issue #995, support-widget half — at 400 × 606 the floating
 * "Chat with support" pill sat on the hero copy. Runs only under the
 * mobile Playwright project. Checks the compact launcher stays in its
 * corner and off the hero and nav, that the open panel fits the
 * viewport, and that the keyboard affordances the host adds on top of
 * @tesserix/otto-widget (Escape to close, focus into the panel on open,
 * back to the launcher on close) actually work.
 */

const WIDTHS = [320, 400] as const;
const HEIGHT = 606;

type Box = { x: number; y: number; width: number; height: number };

function overlaps(a: Box, b: Box): boolean {
  return (
    a.x < b.x + b.width &&
    a.x + a.width > b.x &&
    a.y < b.y + b.height &&
    a.y + a.height > b.y
  );
}

async function boxOf(locator: Locator): Promise<Box> {
  const box = await locator.boundingBox();
  expect(box, "element has no box").not.toBeNull();
  return box as Box;
}

test("compact launcher keeps clear of the hero and nav; panel fits the screen", async ({
  page,
  request,
}) => {
  const suffix = stamp();
  const merchant = {
    email: `support-mobile-${suffix}@example.com`,
    slug: `support-mobile-${suffix}`.replace(/[^a-z0-9-]/g, "").slice(0, 60),
    businessName: `Support Mobile ${suffix}`,
    password: "E2e-test-password-123!",
  };
  await onboardStore(page, request, merchant);

  for (const width of WIDTHS) {
    await page.setViewportSize({ width, height: HEIGHT });
    await page.goto(`${STOREFRONT_URL}/?slug=${merchant.slug}`);

    const launcher = page.getByRole("button", { name: "Open support chat" });
    await expect(launcher).toBeVisible();

    // Compact circle, inside the viewport, with a real touch target.
    const launcherBox = await boxOf(launcher);
    expect(launcherBox.width, `launcher too wide at ${width}px`).toBeLessThanOrEqual(56);
    expect(launcherBox.height).toBeGreaterThanOrEqual(44);
    expect(launcherBox.x).toBeGreaterThanOrEqual(0);
    expect(launcherBox.x + launcherBox.width).toBeLessThanOrEqual(width);
    expect(launcherBox.y + launcherBox.height).toBeLessThanOrEqual(HEIGHT);

    // Never over the header, the hero heading or its primary call to action.
    const nav = page.getByRole("navigation", { name: "Store" });
    expect(overlaps(launcherBox, await boxOf(nav)), `launcher over nav at ${width}px`).toBe(false);

    const heading = page.locator("main h1").first();
    if ((await heading.count()) > 0 && (await heading.isVisible())) {
      expect(
        overlaps(launcherBox, await boxOf(heading)),
        `launcher over hero heading at ${width}px`,
      ).toBe(false);
      const cta = heading.locator("xpath=following::a[1]");
      if ((await cta.count()) > 0 && (await cta.isVisible())) {
        expect(
          overlaps(launcherBox, await boxOf(cta)),
          `launcher over hero CTA at ${width}px`,
        ).toBe(false);
      }
    }

    // Still in its corner after scrolling.
    await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight / 2));
    const scrolledBox = await boxOf(launcher);
    expect(scrolledBox.y + scrolledBox.height).toBeLessThanOrEqual(HEIGHT);
    await page.evaluate(() => window.scrollTo(0, 0));

    // Open from the keyboard: focus lands inside the panel.
    await launcher.focus();
    await page.keyboard.press("Enter");
    const dialog = page.getByRole("dialog", { name: "Support chat" });
    await expect(dialog).toBeVisible();
    const dialogBox = await boxOf(dialog);
    expect(dialogBox.x).toBeGreaterThanOrEqual(0);
    expect(dialogBox.x + dialogBox.width).toBeLessThanOrEqual(width);
    expect(dialogBox.y).toBeGreaterThanOrEqual(0);
    expect(dialogBox.y + dialogBox.height).toBeLessThanOrEqual(HEIGHT);
    const focusInsideDialog = await page.evaluate(() => {
      const active = document.activeElement;
      return !!active && !!active.closest('[role="dialog"]');
    });
    expect(focusInsideDialog, "focus did not move into the panel").toBe(true);

    const close = dialog.getByRole("button", { name: "Close chat" });
    await expect(close).toBeVisible();
    const closeBox = await boxOf(close);
    expect(closeBox.y).toBeGreaterThanOrEqual(0);
    expect(closeBox.x + closeBox.width).toBeLessThanOrEqual(width);

    // Escape closes and hands focus back to the launcher.
    await page.keyboard.press("Escape");
    await expect(dialog).toBeHidden();
    await expect(launcher).toBeFocused();

    // Reopen with a tap, close with the button, focus returns again.
    await launcher.click();
    await expect(dialog).toBeVisible();
    await dialog.getByRole("button", { name: "Close chat" }).click();
    await expect(dialog).toBeHidden();
    await expect(launcher).toBeFocused();
  }
});
