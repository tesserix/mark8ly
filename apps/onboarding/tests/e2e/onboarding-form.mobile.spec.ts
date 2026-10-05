import { expect, test, type Page } from "@playwright/test";

/**
 * Issue #993 — on a 400 × 606 phone the first field of the signup form sat
 * about 750px down, under a headline, two sentences and a three-step list;
 * the copy promised "two minutes" and an automatic landing in the admin;
 * and the homepage's trial reassurance was nowhere near the submit button.
 *
 * Runs only under the mobile Playwright project. Needs the local stack
 * like the rest of this suite; it never submits the form.
 */

const WIDTHS = [320, 375, 400] as const;
const HEIGHT = 606;

async function noHorizontalOverflow(page: Page, width: number) {
  const { scrollWidth, clientWidth, offenders } = await page.evaluate(() => {
    const vw = document.documentElement.clientWidth;
    const offenders = [...document.querySelectorAll("body *")]
      .filter((el) => el.getBoundingClientRect().right > vw + 1)
      .slice(0, 8)
      .map((el) => {
        const r = el.getBoundingClientRect();
        const id = el.id ? `#${el.id}` : "";
        return `${el.tagName.toLowerCase()}${id} right=${Math.round(r.right)}`;
      });
    return {
      scrollWidth: document.documentElement.scrollWidth,
      clientWidth: vw,
      offenders,
    };
  });
  expect(
    scrollWidth,
    `page overflows horizontally at ${width}px; past the edge: ${offenders.join(", ") || "none found"}`,
  ).toBeLessThanOrEqual(clientWidth);
}

test.describe("signup form on a phone", () => {
  for (const width of WIDTHS) {
    test(`at ${width}px the email field is on the first screen and nothing is clipped`, async ({
      page,
    }) => {
      await page.setViewportSize({ width, height: HEIGHT });
      await page.goto("/onboarding");

      await noHorizontalOverflow(page, width);
      expect(await page.evaluate(() => window.scrollY)).toBe(0);

      // Label and the whole input are inside the initial viewport.
      const label = page.locator('label[for="email"]');
      const input = page.getByLabel(/email address/i);
      for (const [name, locator] of [
        ["email label", label],
        ["email input", input],
      ] as const) {
        await expect(locator).toBeVisible();
        const box = await locator.boundingBox();
        expect(box, `${name} has no box`).not.toBeNull();
        expect(box!.y, `${name} starts above the viewport`).toBeGreaterThanOrEqual(0);
        expect(
          box!.y + box!.height,
          `${name} is below the fold at ${width}px`,
        ).toBeLessThanOrEqual(HEIGHT);
        expect(box!.x).toBeGreaterThanOrEqual(0);
        expect(box!.x + box!.width).toBeLessThanOrEqual(width);
      }

      // Every control fits the width; nothing is cut off at the edge.
      const controls = page.locator("main input, main button, main summary, main [role=combobox]");
      const count = await controls.count();
      for (let i = 0; i < count; i++) {
        const control = controls.nth(i);
        if (!(await control.isVisible())) continue;
        const box = await control.boundingBox();
        if (!box) continue;
        expect(box.x, `control ${i} starts off screen`).toBeGreaterThanOrEqual(0);
        expect(box.x + box.width, `control ${i} runs off screen`).toBeLessThanOrEqual(width + 1);
      }
    });
  }

  test("copy describes the real flow and repeats the trial terms at the submit", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 400, height: HEIGHT });
    await page.goto("/onboarding");

    await expect(page.getByRole("heading", { level: 1 })).toHaveText(/start your store/i);
    await expect(page.getByText(/two minutes/i)).toHaveCount(0);
    await expect(page.getByText(/land in your admin/i)).toHaveCount(0);
    await expect(page.getByText(/don.t need a password/i)).toHaveCount(0);

    // Verification, password, sign in — in that order, before the submit.
    const steps = page.getByRole("list").filter({ hasText: /verify your email/i });
    await expect(steps.getByRole("listitem")).toHaveCount(3);
    const stepText = (await steps.innerText()).toLowerCase();
    expect(stepText.indexOf("verify your email")).toBeGreaterThan(-1);
    expect(stepText.indexOf("choose a password and sign in")).toBeGreaterThan(
      stepText.indexOf("verify your email"),
    );

    const submit = page.getByRole("button", { name: /send verification link/i });
    await submit.scrollIntoViewIfNeeded();
    const reassurance = page.getByTestId("trial-reassurance");
    await expect(reassurance).toBeVisible();
    await expect(reassurance).toContainText(/free for ninety days\. no card required\./i);

    const pricing = reassurance.getByRole("link", { name: /see the pricing/i });
    await expect(pricing).toHaveAttribute("href", "/#pricing");

    // Reassurance sits beside the decision, not somewhere above the fold.
    const submitBox = await submit.boundingBox();
    const reassuranceBox = await reassurance.boundingBox();
    expect(submitBox).not.toBeNull();
    expect(reassuranceBox).not.toBeNull();
    expect(submitBox!.y - (reassuranceBox!.y + reassuranceBox!.height)).toBeLessThan(48);
  });

  test("optional tax ID and promo code are behind a keyboard-operable disclosure", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 400, height: HEIGHT });
    await page.goto("/onboarding");

    const details = page.getByTestId("optional-fields");
    const summary = details.locator("summary");
    await expect(summary).toBeVisible();
    await expect(summary).toContainText(/tax id or promo code/i);
    await expect(page.locator("#taxId")).toBeHidden();
    await expect(page.locator("#promoCode")).toBeHidden();

    // Enter on the focused summary opens it; Enter again closes it.
    await summary.focus();
    await page.keyboard.press("Enter");
    await expect(page.locator("#taxId")).toBeVisible();
    await expect(page.locator("#promoCode")).toBeVisible();
    await page.keyboard.press("Enter");
    await expect(page.locator("#taxId")).toBeHidden();

    // A refused promo code opens the disclosure by itself so the message
    // is read in context, even if the merchant closed it after typing.
    await summary.click();
    await page.locator("#promoCode").fill("NOT-A-REAL-CODE");
    await summary.click();
    await expect(page.locator("#promoCode")).toBeHidden();
    const hint = page.locator("#promoCode-hint");
    await expect(hint).toContainText(/code|offer|carry on/i, { timeout: 10_000 });
    const opened = await details.evaluate((el) => (el as HTMLDetailsElement).open);
    const hintTone = await hint.evaluate((el) => el.className);
    // Only a refusal must force it open; an "unknown" outcome stays neutral.
    if (hintTone.includes("text-danger")) {
      expect(opened, "refused promo code left the field hidden").toBe(true);
    }
  });
});
