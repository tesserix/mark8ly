"use client";

/**
 * Marketing /#compare section.
 *
 * A client island purely so the currency-bearing cells can resolve the
 * visitor's `mk8_currency` cookie after mount. It was inline in
 * app/page.tsx and rendered on the server until doing so forced the
 * whole route dynamic (#597) — see lib/geo/currency.ts for the TTFB
 * numbers that motivated the split.
 *
 * The markup still server-renders at build time in PRERENDER_CURRENCY,
 * so crawlers and the first paint get the full table; only the six
 * price cells and the two "in {currency}" sentences change on mount.
 * Splitting those into six micro-islands would keep a few kB out of the
 * bundle at the cost of scattering one table across several files, and
 * this section is well below the fold where bundle size is not the
 * constraint.
 */

import type { ReactNode } from "react";
import Link from "next/link";
import {
  Money,
  getPlanPrice,
  SHARED_PRICING_CATALOGUE,
  type Currency,
} from "@repo/ui/subscription";

import { useGeoCurrency } from "@/lib/geo/use-geo-currency";

/* ============================================================
   Comparison — quiet, factual table that puts Mark8ly next to
   the names readers have already weighed: Shopify, BigCommerce,
   Wix, Squarespace.

   Cadence: every "Starting price" cell shows the *annual-billed
   monthly equivalent* — the same cadence the Pricing section
   above defaults to. Without this, an AUD merchant saw A$23 in
   Pricing and A$29 in Comparison for the same plan, which read
   as a typo. Now both speak the same number.

   Mark8ly's Starter tracks the SHARED_PRICING_CATALOGUE (single
   source of truth with /admin/pricing). Competitor prices come
   from each platform's published per-country pricing page,
   verified against the live page where possible:
     - Shopify Basic AUD A$42 — shopify.com/au/pricing
     - Shopify Basic INR ₹1,499 — shopify.com/in/pricing
     - BigCommerce Standard USD $22 — bigcommerce.com (USD only globally)
   Where a platform doesn't publish a price in the visitor's
   currency (BigCommerce globally, Wix/Squarespace outside the
   US in our data set), the cell shows their USD rate with a
   "(USD)" tag so the merchant sees what they'd actually be
   charged. We deliberately keep the data set small and honest:
   only currencies we've verified, USD fallback for the rest.

   Editorial, not a SaaS bake-off — hairline rules between rows,
   no shaded card, Mark8ly column carries a single moss accent
   so the eye finds it without shouting.
   ============================================================ */

interface CompareRow {
  label: string;
  mark8ly: string;
  shopify: string;
  bigcommerce: string;
  wix: string;
  squarespace: string;
}

// Feature rows (currency-agnostic). The Starting-price row is
// rendered separately and pulls from COMPETITOR_STARTING_PRICES
// + the shared catalogue.
const COMPARISON_ROWS: readonly CompareRow[] = [
  {
    label: "Free to try",
    mark8ly: "90 days",
    shopify: "3 days",
    bigcommerce: "15 days",
    wix: "14 days",
    squarespace: "14 days",
  },
  {
    label: "Platform fee per sale",
    mark8ly: "None",
    shopify: "2%, unless you use Shopify Payments",
    bigcommerce: "None",
    wix: "None",
    squarespace: "None",
  },
  {
    label: "Default storefront design",
    mark8ly: "Editorial, designer-led",
    shopify: "Generic templates",
    bigcommerce: "Functional templates",
    wix: "Drag-and-drop builder",
    squarespace: "Designer themes",
  },
  {
    label: "Use your own domain",
    mark8ly: "Included",
    shopify: "Bring your own",
    bigcommerce: "Bring your own",
    wix: "First year included",
    squarespace: "First year included",
  },
  {
    label: "Local payments (UPI, wallets)",
    mark8ly: "Built in",
    shopify: "Limited by region",
    bigcommerce: "Limited by region",
    wix: "Limited by region",
    squarespace: "Mostly Stripe / PayPal",
  },
  {
    label: "Take your data when you leave",
    mark8ly: "One click, any time",
    shopify: "CSV export",
    bigcommerce: "CSV export",
    wix: "Partial",
    squarespace: "Partial",
  },
];

type CompetitorId = "shopify" | "bigcommerce" | "wix" | "squarespace";

// Annual-billed monthly equivalent (the price each platform leads
// with on their own pricing page, "$X/mo billed yearly"). Pre-
// formatted strings preserve each platform's own grouping and
// symbol convention. Currencies not in a competitor's map fall back
// to its USD entry with a "(USD)" tag in the renderer below — that's
// honest about platforms that don't localize for that currency.
const COMPETITOR_STARTING_PRICES: Record<
  CompetitorId,
  Partial<Record<Currency, string>>
> = {
  shopify: {
    // Shopify Basic, "$X/mo billed yearly" rate from each region.
    USD: "$29",
    AUD: "A$42", // shopify.com/au/pricing
    INR: "₹1,499", // shopify.com/in/pricing
    GBP: "£19",
    EUR: "€27",
  },
  bigcommerce: {
    // Standard plan annual-billed: $29 monthly with "save 25%" → $22/mo.
    // BigCommerce publishes USD only globally — every other currency
    // hits the USD-tagged fallback.
    USD: "$22",
  },
  wix: {
    // Core plan with ecommerce, annual billing.
    USD: "$29",
  },
  squarespace: {
    // Basic Commerce, annual billing.
    USD: "$27",
  },
};

function competitorStartingPrice(
  id: CompetitorId,
  currency: Currency,
): string {
  const map = COMPETITOR_STARTING_PRICES[id];
  const localized = map[currency];
  if (localized) return `${localized} / mo`;
  // Platform doesn't publish a price in this currency — they bill
  // in USD. Tag it so a non-US merchant knows what they'd actually pay.
  const usd = map.USD!;
  return currency === "USD" ? `${usd} / mo` : `${usd} / mo (USD)`;
}

export function Comparison() {
  // Resolved on the client for the same reason as the Pricing section:
  // reading `mk8_currency` on the server made the entire marketing route
  // dynamic and uncacheable at the edge. See lib/geo/currency.ts.
  const currency = useGeoCurrency();

  const competitorHeaderClass =
    "px-4 py-4 text-left align-bottom font-sans text-[0.9375rem] font-medium text-foreground-secondary";
  const competitorCellClass =
    "px-4 py-5 align-top text-foreground-tertiary";

  // Mark8ly's Starter price tracks the shared pricing catalogue,
  // and we deliberately use the annual-billed monthly equivalent so
  // the number matches what the Pricing section shows on its default
  // (annual) toggle. Without this, the same plan reads as A$23 in
  // Pricing and A$29 in Comparison — same plan, different cadence,
  // looks like a typo.
  const starter = SHARED_PRICING_CATALOGUE.plans.find((p) => p.id === "starter");
  const resolvedStarter = starter ? getPlanPrice(starter, currency) : null;
  const mark8lyStarterAnnualMonthly = resolvedStarter?.price.annualMonthlyEquivalent ?? null;
  // MUST render this — never the raw cookie value — for the same
  // reason as the Pricing section: when `currency` has no row, the
  // fallback amount is denominated in USD, and labelling it with the
  // visitor's raw currency code would misquote the price.
  const mark8lyStarterCurrency = resolvedStarter?.currency ?? currency;

  return (
    <section
      id="compare"
      aria-labelledby="compare-heading"
      className="border-t border-border-subtle py-16 sm:py-24"
    >
      <div className="mx-auto max-w-6xl px-6">
        <div className="mb-12 grid gap-8 lg:grid-cols-[1fr_2fr] lg:gap-16">
          <div>
            <p className="eyebrow mb-5">How we compare</p>
            <h2
              id="compare-heading"
              className="font-serif text-4xl font-medium leading-[1.05] tracking-[-0.02em] text-foreground"
            >
              Side by side,
              <br />
              no spin.
            </h2>
          </div>
          <p className="max-w-xl self-end text-base leading-relaxed text-foreground-secondary">
            We&rsquo;re not the only commerce platform on the table. Here&rsquo;s
            how Mark8ly lines up against the names you&rsquo;ve probably
            already looked at &mdash; the things that show up on the bill,
            and the things that show up on the storefront. Starting prices
            show each platform&rsquo;s annual-billed rate in {currency} where
            they publish one, the same cadence as the Pricing section above.
          </p>
        </div>

        <div className="-mx-6 overflow-x-auto px-6 sm:mx-0 sm:px-0">
          <table className="w-full min-w-[760px] border-collapse text-sm">
            <caption className="sr-only">
              How Mark8ly compares with Shopify, BigCommerce, Wix and
              Squarespace on price, trial length, fees, design and data
              portability. Prices shown in {currency}.
            </caption>
            <thead>
              <tr className="border-b border-border">
                <th
                  scope="col"
                  className="py-4 pr-6 text-left align-bottom"
                >
                  <span className="eyebrow">Feature</span>
                </th>
                <th
                  scope="col"
                  className="px-4 py-4 text-left align-bottom"
                >
                  <span className="font-serif text-lg font-medium text-foreground">
                    Mark8ly
                  </span>
                </th>
                <th scope="col" className={competitorHeaderClass}>
                  Shopify
                </th>
                <th scope="col" className={competitorHeaderClass}>
                  BigCommerce
                </th>
                <th scope="col" className={competitorHeaderClass}>
                  Wix
                </th>
                <th scope="col" className={competitorHeaderClass}>
                  Squarespace
                </th>
              </tr>
            </thead>
            <tbody>
              {/* Starting price — geo-localized row, rendered specially. */}
              <tr className="border-b border-border-subtle">
                <th
                  scope="row"
                  className="py-5 pr-6 text-left align-top font-normal text-foreground-secondary"
                >
                  Starting price
                </th>
                <td className="px-4 py-5 align-top">
                  <span className="block border-l-2 border-moss-700 pl-3 font-medium text-foreground">
                    {mark8lyStarterAnnualMonthly !== null ? (
                      <>
                        <Money
                          amount={mark8lyStarterAnnualMonthly}
                          currency={mark8lyStarterCurrency}
                          showCents={false}
                        />
                        <span className="text-foreground-tertiary"> / mo</span>
                      </>
                    ) : (
                      "—"
                    )}
                  </span>
                </td>
                <td className={competitorCellClass}>
                  {competitorStartingPrice("shopify", currency)}
                </td>
                <td className={competitorCellClass}>
                  {competitorStartingPrice("bigcommerce", currency)}
                </td>
                <td className={competitorCellClass}>
                  {competitorStartingPrice("wix", currency)}
                </td>
                <td className={competitorCellClass}>
                  {competitorStartingPrice("squarespace", currency)}
                </td>
              </tr>

              {/* Feature rows — currency-agnostic. */}
              {COMPARISON_ROWS.map((row) => (
                <tr
                  key={row.label}
                  className="border-b border-border-subtle"
                >
                  <th
                    scope="row"
                    className="py-5 pr-6 text-left align-top font-normal text-foreground-secondary"
                  >
                    {row.label}
                  </th>
                  <td className="px-4 py-5 align-top">
                    <span className="block border-l-2 border-moss-700 pl-3 font-medium text-foreground">
                      {row.mark8ly}
                    </span>
                  </td>
                  <td className={competitorCellClass}>{row.shopify}</td>
                  <td className={competitorCellClass}>{row.bigcommerce}</td>
                  <td className={competitorCellClass}>{row.wix}</td>
                  <td className={competitorCellClass}>{row.squarespace}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>

        <p className="mt-8 max-w-3xl text-sm text-foreground-tertiary">
          Starting prices show each platform&rsquo;s cheapest paid plan
          billed yearly &mdash; Shopify Basic, BigCommerce Standard, Wix
          Core, Squarespace Basic Commerce. Where a platform doesn&rsquo;t
          publish a price in your currency (BigCommerce globally; Wix and
          Squarespace outside the US in our verified data), we show the USD
          rate and tag it so you know what you&rsquo;d actually be charged.
          Payment processor fees &mdash; around 2&ndash;3% for cards, closer
          to 1% for UPI &mdash; apply on every platform. That&rsquo;s your
          bank, not us; the line above is what the platform itself adds on
          top.
        </p>

        <p className="mt-8 max-w-3xl text-base leading-relaxed text-foreground-secondary">
          Weighing a specific platform? Read the full comparisons:{" "}
          <ComparisonLink href="/shopify-alternative">
            Mark8ly vs Shopify
          </ComparisonLink>
          ,{" "}
          <ComparisonLink href="/etsy-alternative">
            the Etsy alternative
          </ComparisonLink>
          , our{" "}
          <ComparisonLink href="/ecommerce-for-makers">
            guide for makers
          </ComparisonLink>
          , or{" "}
          <ComparisonLink href="/sell-online-india">
            selling online in India
          </ComparisonLink>
          .
        </p>
      </div>
    </section>
  );
}

/* Inline editorial link — moss underline, matches the Prose link style. */
function ComparisonLink({
  href,
  children,
}: {
  href: string;
  children: ReactNode;
}) {
  return (
    <Link
      href={href}
      className="text-foreground underline decoration-moss-700 decoration-2 underline-offset-4 hover:text-moss-700"
    >
      {children}
    </Link>
  );
}
