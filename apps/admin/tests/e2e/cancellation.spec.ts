/**
 * P16 — Cancellation wizard (authenticated, fully mocked).
 *
 * Covers two paths through the 4-step cancellation flow:
 *
 *   Path A (decline save-offer):
 *     Confirm → Survey (too_expensive) → SaveOffer (decline) → FinalConfirm
 *     → POST /subscription/cancel → status: cancel_scheduled
 *     → "Subscription cancelled" final screen + back-to-billing CTA.
 *
 *   Path B (accept save-offer):
 *     Confirm → Survey → SaveOffer (accept)
 *     → POST /subscription/cancel { accept_save_offer: true }
 *     → status: active → redirect + "restored" toast.
 *
 * Backend is fully mocked via page.route().
 */

import { expect, test } from "@playwright/test";

import { ADMIN_URL } from "./helpers";
import {
  STUB_STORE_ID,
  mockAuthSession,
  mockActiveSubscription,
  setupMockRoutes,
} from "./fixtures/subscription";

const CANCEL_URL = `${ADMIN_URL}/admin/stores/${STUB_STORE_ID}/settings/billing/cancel`;

test.describe("cancellation wizard", () => {
  test.beforeEach(async ({ page }) => {
    await mockAuthSession(page);
    await setupMockRoutes(page);
    await mockActiveSubscription(page, "studio", "annual");
  });

  test("decline save-offer path ends on cancelled confirmation screen", async ({
    page,
  }) => {
    // Mock the cancellation POST — decline path.
    await page.route("**/api/v1/admin/subscription/cancel", (route) => {
      const method = route.request().method();
      if (method !== "POST") {
        void route.continue();
        return;
      }
      void route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            status: "cancel_scheduled",
            cancels_at: "2026-05-18T00:00:00Z",
          },
        }),
      });
    });

    await page.goto(CANCEL_URL);

    // ── Step 1: Confirm ──────────────────────────────────────────────────
    await expect(
      page.getByRole("heading", { name: /cancel/i }),
    ).toBeVisible({ timeout: 8_000 });

    const confirmBtn = page
      .getByRole("button", { name: /continue/i })
      .or(page.getByRole("button", { name: /next/i }))
      .or(page.getByRole("button", { name: /yes, cancel/i }));
    await confirmBtn.click();

    // ── Step 2: Survey ───────────────────────────────────────────────────
    // Select "too expensive" reason.
    const tooExpensiveOption = page
      .getByLabel(/too expensive/i)
      .or(page.getByRole("radio", { name: /too expensive/i }))
      .or(page.getByText(/too expensive/i));
    await tooExpensiveOption.click();

    const surveyNextBtn = page
      .getByRole("button", { name: /next/i })
      .or(page.getByRole("button", { name: /continue/i }));
    await surveyNextBtn.click();

    // ── Step 3: Save-offer ───────────────────────────────────────────────
    // Should show a retention offer; decline it.
    await expect(
      page.getByText(/offer/i).or(page.getByText(/save/i)),
    ).toBeVisible({ timeout: 8_000 });

    const declineBtn = page
      .getByRole("button", { name: /no thanks/i })
      .or(page.getByRole("button", { name: /decline/i }))
      .or(page.getByRole("button", { name: /still cancel/i }));
    await declineBtn.click();

    // ── Step 4: Final confirm ────────────────────────────────────────────
    const finalConfirmBtn = page
      .getByRole("button", { name: /confirm cancellation/i })
      .or(page.getByRole("button", { name: /cancel subscription/i }));
    await finalConfirmBtn.click();

    // Final screen: cancelled state.
    await expect(
      page
        .getByText(/subscription cancelled/i)
        .or(page.getByText(/cancellation confirmed/i)),
    ).toBeVisible({ timeout: 10_000 });

    // Back-to-billing CTA should be present.
    await expect(
      page.getByRole("link", { name: /billing/i }),
    ).toBeVisible();
  });

  test("accept save-offer path restores the subscription", async ({ page }) => {
    // Mock POST — accept_save_offer path returns active.
    await page.route("**/api/v1/admin/subscription/cancel", (route) => {
      if (route.request().method() !== "POST") {
        void route.continue();
        return;
      }
      void route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          data: {
            status: "active",
            save_offer_msg: "Your 20% discount has been applied.",
          },
        }),
      });
    });

    await page.goto(CANCEL_URL);

    // Navigate through Confirm → Survey.
    const confirmBtn = page
      .getByRole("button", { name: /continue/i })
      .or(page.getByRole("button", { name: /next/i }))
      .or(page.getByRole("button", { name: /yes, cancel/i }));
    await confirmBtn.click();

    const tooExpensiveOption = page
      .getByLabel(/too expensive/i)
      .or(page.getByRole("radio", { name: /too expensive/i }))
      .or(page.getByText(/too expensive/i));
    await tooExpensiveOption.click();

    const surveyNextBtn = page
      .getByRole("button", { name: /next/i })
      .or(page.getByRole("button", { name: /continue/i }));
    await surveyNextBtn.click();

    // On save-offer step: accept the offer.
    await expect(
      page.getByText(/offer/i).or(page.getByText(/save/i)),
    ).toBeVisible({ timeout: 8_000 });

    const acceptBtn = page
      .getByRole("button", { name: /accept/i })
      .or(page.getByRole("button", { name: /claim offer/i }))
      .or(page.getByRole("button", { name: /apply discount/i }));
    await acceptBtn.click();

    // After accepting: should redirect and show a "restored" or
    // "subscription kept" message.
    await expect(
      page
        .getByText(/discount applied/i)
        .or(page.getByText(/subscription restored/i))
        .or(page.getByText(/offer applied/i))
        .or(page.getByRole("status")),
    ).toBeVisible({ timeout: 10_000 });

    // Should NOT be on the cancellation page anymore.
    await expect(page).not.toHaveURL(/\/cancel/, { timeout: 10_000 });
  });
});
