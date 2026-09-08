/**
 * P16 — Public /pricing page (unauthenticated).
 *
 * All assertions are against the rendered DOM only — no backend calls
 * are needed for a static/RSC pricing page. Tests use a fresh browser
 * context with no session cookies.
 *
 * Currency localisation: the page reads the `mk8_currency` cookie, but
 * middleware rewrites that cookie from the `CF-IPCountry` request header
 * on every request, so the header is the only input a test can drive.
 *
 * Requires: admin dev server on ADMIN_URL (default :4202). Note the name:
 * navigation goes through helpers.ts, which reads ADMIN_URL — NOT the
 * ADMIN_BASE_URL that playwright.config.ts's `baseURL` reads. Every goto
 * here is an absolute URL built from ADMIN_URL, so `baseURL` is unused by
 * this spec and setting ADMIN_BASE_URL alone moves nothing.
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

  test("plan CTAs render the hrefs currently shipped — all three 404, see mark8ly#834", async ({
    browser,
  }) => {
    const ctx = await browser.newContext();
    const page = await ctx.newPage();

    await page.goto(PRICING_URL);

    // Select each CTA by the PLAN IT BELONGS TO, then assert its href.
    //
    // Not by label: the copy in apps/admin/lib/copy/pricing.ts has drifted
    // from this spec's original guesses ("Get started" / "Get Studio"), and
    // Starter and Studio now share the identical label "Start free trial".
    //
    // Not by href either, which is what this test used to do. Selecting
    // `a[href="/signup?plan=starter"]` and then asserting that same href is
    // a tautology: it can only hold or throw in the locator, never fail an
    // expect, and it would stay green if Starter's and Studio's hrefs were
    // swapped — the plan each link belongs to was never checked.
    //
    // apps/admin/app/pricing/PricingClient.tsx:90-100 renders every plan as
    // `<article aria-label="{name} plan">` with exactly one link inside, so
    // the article IS the per-plan handle. (The White-label App add-on and
    // the Pro contact CTAs are `<section>`s, role=region — they cannot
    // match `role: "article"`.)
    //
    // These assert what the page renders TODAY, which is not the same as
    // asserting the links work. All three hrefs 404 as of this commit:
    //   - there is no /signup route anywhere under apps/admin/app;
    //   - "(admin)" is a Next route GROUP, so pro-contact really lives at
    //     /settings/billing/pro-contact — the /admin prefix is not a path.
    // That is a product bug in lib/copy/pricing.ts, reported against
    // mark8ly#834; fix the hrefs there and update these three strings
    // together. Until then this test's NAME says 404 so the CI log does
    // not read as an endorsement of the URLs.
    const planCta = (plan: RegExp) =>
      page.getByRole("article", { name: plan }).getByRole("link");

    await expect(planCta(/starter plan/i)).toHaveAttribute(
      "href",
      "/signup?plan=starter",
    );
    await expect(planCta(/studio plan/i)).toHaveAttribute(
      "href",
      "/signup?plan=studio",
    );
    await expect(planCta(/pro plan/i)).toHaveAttribute(
      "href",
      "/admin/settings/billing/pro-contact",
    );

    await ctx.close();
  });

  test("currency switches to GBP when CF-IPCountry resolves to GB", async ({
    browser,
  }) => {
    // apps/admin/lib/geo/geoMiddleware.ts sets the mk8_currency cookie
    // unconditionally from the CF-IPCountry request header on every
    // request to /pricing — that Set-Cookie also rewrites what this same
    // request's RSC render sees, so a cookie set ahead of navigation (the
    // original approach here) is clobbered before the page ever reads it.
    // The header is the actual input; drive the test through it.
    const ctx = await browser.newContext({
      extraHTTPHeaders: { "CF-IPCountry": "GB" },
    });
    const page = await ctx.newPage();

    await page.goto(PRICING_URL);

    // At least one price element should display the £ symbol.
    await expect(page.getByText(/£/).first()).toBeVisible();

    await ctx.close();
  });
});
