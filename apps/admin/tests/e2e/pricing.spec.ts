/**
 * P16 — Public /pricing page (unauthenticated).
 *
 * All assertions are against the rendered DOM only — no backend calls
 * are needed for a static/RSC pricing page. Tests use a fresh browser
 * context with no session cookies.
 *
 * Currency localisation: the page reads `mk8_currency` cookie OR the
 * `CF-IPCountry` request header (set by Cloudflare middleware) to pick
 * the billing currency. In tests we inject the cookie before navigation.
 *
 * Requires: admin dev server on ADMIN_BASE_URL (default :4202).
 */

import { expect, test } from "@playwright/test";

import { ADMIN_URL } from "./helpers";

const PRICING_URL = `${ADMIN_URL}/pricing`;

test.describe("public /pricing page", () => {
  test("renders all three plan headings", async ({ browser }) => {
    const ctx = await browser.newContext();
    const page = await ctx.newPage();

    await page.goto(PRICING_URL);

    await expect(page.getByRole("heading", { name: /starter/i })).toBeVisible();
    await expect(page.getByRole("heading", { name: /studio/i })).toBeVisible();
    await expect(page.getByRole("heading", { name: /pro/i })).toBeVisible();

    await ctx.close();
  });

  test("Pro+App add-on section is visible", async ({ browser }) => {
    const ctx = await browser.newContext();
    const page = await ctx.newPage();

    await page.goto(PRICING_URL);

    // The add-on section should have a heading or label referencing the
    // white-label app add-on.
    await expect(
      page.getByRole("heading", { name: /white.?label app/i }),
    ).toBeVisible();

    await ctx.close();
  });

  test("toggling Monthly/Annual shows the billed-annually note", async ({
    browser,
  }) => {
    const ctx = await browser.newContext();
    const page = await ctx.newPage();

    await page.goto(PRICING_URL);

    // Find the billing period toggle — it should default to Annual.
    const annualTab = page.getByRole("tab", { name: /annual/i }).or(
      page.getByRole("radio", { name: /annual/i }),
    );
    const monthlyTab = page.getByRole("tab", { name: /monthly/i }).or(
      page.getByRole("radio", { name: /monthly/i }),
    );

    // On annual the "Billed annually" note should be visible somewhere
    // in the plan cards (e.g. as small print or a savings badge).
    await expect(page.getByText(/billed annually/i).first()).toBeVisible();

    // Switch to monthly — the note should disappear
    await monthlyTab.click();
    await expect(page.getByText(/billed annually/i)).not.toBeVisible();

    // Switch back to annual — the note reappears
    await annualTab.click();
    await expect(page.getByText(/billed annually/i).first()).toBeVisible();

    await ctx.close();
  });

  test("plan CTAs link to the correct signup URLs", async ({ browser }) => {
    const ctx = await browser.newContext();
    const page = await ctx.newPage();

    await page.goto(PRICING_URL);

    // Starter → /signup?plan=starter
    const starterCta = page
      .getByRole("link", { name: /get started/i })
      .or(page.getByRole("link", { name: /start for free/i }))
      .first();
    const starterHref = await starterCta.getAttribute("href");
    expect(starterHref).toMatch(/\/signup\?plan=starter/);

    // Studio → /signup?plan=studio
    const studioCta = page
      .getByRole("link", { name: /get studio/i })
      .or(page.getByRole("link", { name: /start studio/i }))
      .first();
    const studioHref = await studioCta.getAttribute("href");
    expect(studioHref).toMatch(/\/signup\?plan=studio/);

    // Pro → contact-sales
    const proCta = page
      .getByRole("link", { name: /contact sales/i })
      .or(page.getByRole("link", { name: /talk to sales/i }))
      .first();
    const proHref = await proCta.getAttribute("href");
    expect(proHref).toMatch(/\/admin\/settings\/billing\/pro-contact/);

    await ctx.close();
  });

  test("currency switches to GBP when mk8_currency cookie is set to GBP", async ({
    browser,
  }) => {
    // Set the currency cookie before the first navigation so the RSC
    // component picks it up on the initial render.
    const ctx = await browser.newContext();
    const page = await ctx.newPage();

    await page.context().addCookies([
      {
        name: "mk8_currency",
        value: "GBP",
        domain: new URL(ADMIN_URL).hostname,
        path: "/",
      },
    ]);

    await page.goto(PRICING_URL);

    // At least one price element should display the £ symbol.
    await expect(page.getByText(/£/).first()).toBeVisible();

    await ctx.close();
  });
});
