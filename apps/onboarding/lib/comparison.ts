// Homepage comparison table data — every competitor fact in one place,
// each platform carrying the page it came from and the date someone last
// checked it there. Comparative claims are Australian Consumer Law
// territory: a cell nobody can source is a cell we should not publish.
//
// TODO(verify): every `verifiedOn` is null — none of these cells has a
// recorded check against the live pricing page. Re-verify each platform,
// then set the date. The "As of" note under the table renders only from
// verified dates, so it stays hidden until then rather than vouching for
// numbers nobody checked. Known suspects:
//   - Squarespace: "Basic Commerce" is a legacy plan name; the current
//     line-up is Basic / Core / Plus / Advanced, and Basic charges a
//     platform fee on physical product sales — the "None" fee cell and
//     the $27 price both need checking.
//   - Shopify: Basic is not the cheapest paid plan (Starter is), and
//     introductory trial pricing may apply.
//   - Wix and BigCommerce: prices and trial lengths.

import type { Currency } from "@repo/ui/subscription";

export type CompetitorId = "shopify" | "bigcommerce" | "wix" | "squarespace";

export interface Competitor {
  id: CompetitorId;
  name: string;
  /** The plan the starting price quotes. */
  plan: string;
  source: string;
  /** ISO date the cells were last checked against `source`; null = never. */
  verifiedOn: string | null;
  /**
   * Annual-billed monthly equivalent, pre-formatted in each platform's own
   * convention. Currencies not listed fall back to USD, tagged "(USD)".
   */
  startingPrice: Partial<Record<Currency, string>>;
}

export const COMPETITORS: readonly Competitor[] = [
  {
    id: "shopify",
    name: "Shopify",
    plan: "Basic",
    source: "https://www.shopify.com/pricing",
    verifiedOn: null,
    startingPrice: {
      USD: "$29",
      AUD: "A$42", // shopify.com/au/pricing
      INR: "₹1,499", // shopify.com/in/pricing
      GBP: "£19",
      EUR: "€27",
    },
  },
  {
    id: "bigcommerce",
    name: "BigCommerce",
    plan: "Standard",
    source: "https://www.bigcommerce.com/essentials/pricing/",
    verifiedOn: null,
    // $29 monthly with "save 25%" annually → $22/mo. USD only globally.
    startingPrice: { USD: "$22" },
  },
  {
    id: "wix",
    name: "Wix",
    plan: "Core",
    source: "https://www.wix.com/plans",
    verifiedOn: null,
    startingPrice: { USD: "$29" },
  },
  {
    id: "squarespace",
    name: "Squarespace",
    plan: "Basic Commerce",
    source: "https://www.squarespace.com/pricing",
    verifiedOn: null,
    startingPrice: { USD: "$27" },
  },
];

export interface CompareRow {
  label: string;
  mark8ly: string;
  competitors: Record<CompetitorId, string>;
}

// Facts only. The "Default storefront design" row ("Generic templates" vs
// "Editorial, designer-led") was opinion under a factual heading and is gone.
export const COMPARISON_ROWS: readonly CompareRow[] = [
  {
    label: "Free to try",
    mark8ly: "90 days",
    competitors: {
      shopify: "3 days",
      bigcommerce: "15 days",
      wix: "14 days",
      squarespace: "14 days",
    },
  },
  {
    label: "Platform fee per sale",
    mark8ly: "None",
    competitors: {
      shopify: "2%, unless you use Shopify Payments",
      bigcommerce: "None",
      wix: "None",
      squarespace: "None",
    },
  },
  {
    // Mark8ly connects a domain the merchant owns (internal/domain); it
    // does not register one, so "Included" overstated it.
    label: "Use your own domain",
    mark8ly: "Connect your own, SSL included",
    competitors: {
      shopify: "Bring your own",
      bigcommerce: "Bring your own",
      wix: "First year included",
      squarespace: "First year included",
    },
  },
  {
    label: "Local payments (UPI, wallets)",
    mark8ly: "Built in",
    competitors: {
      shopify: "Limited by region",
      bigcommerce: "Limited by region",
      wix: "Limited by region",
      squarespace: "Mostly Stripe / PayPal",
    },
  },
  {
    label: "Take your data when you leave",
    mark8ly: "One click, any time",
    competitors: {
      shopify: "CSV export",
      bigcommerce: "CSV export",
      wix: "Partial",
      squarespace: "Partial",
    },
  },
];

/** Oldest verification date across competitors, or null if any never was. */
export function comparisonAsOf(): string | null {
  const dates = COMPETITORS.map((c) => c.verifiedOn);
  if (dates.some((d) => d === null)) return null;
  return (dates as string[]).sort()[0]!;
}
