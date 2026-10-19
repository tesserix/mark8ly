// Homepage comparison table data — every competitor fact in one place,
// each platform carrying the page it came from and the date someone last
// checked it there. Comparative claims are Australian Consumer Law
// territory: a cell nobody can source is a cell we should not publish.
//
// Checked 2026-10-20. The fetch reached each platform's AU edition, so
// USD/GBP/EUR figures for Shopify and Squarespace come from third-party
// 2026 pricing guides, not the platform's own page; those two stay
// unverified (verifiedOn null) until someone reads them first-hand.
// Wix's page showed "USD for reference only" from AU, so it stays open too.
// The "As of" note renders only when every platform is verified.

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
    // AUD and INR read on shopify.com 2026-10-20; USD/GBP/EUR from guides.
    verifiedOn: null,
    startingPrice: {
      USD: "$29",
      AUD: "A$42",
      INR: "₹1,499",
      GBP: "£19",
      EUR: "€24",
    },
  },
  {
    id: "bigcommerce",
    name: "BigCommerce",
    plan: "Core",
    source: "https://www.bigcommerce.com/essentials/pricing/",
    verifiedOn: "2026-10-20",
    // $39 monthly, $29/mo billed annually. USD only globally.
    startingPrice: { USD: "$29" },
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
    // Core, not Basic: Basic takes 2% of every store sale, Core takes none.
    plan: "Core",
    source: "https://www.squarespace.com/pricing",
    // AUD read on squarespace.com 2026-10-20; USD from guides.
    verifiedOn: null,
    startingPrice: { USD: "$29", AUD: "A$28" },
  },
];

export interface CompareRow {
  label: string;
  mark8ly: string;
  competitors: Record<CompetitorId, string>;
}

// Facts only. The local-payments and data-export rows had no source for
// any competitor cell ("Partial", "Limited by region") and are gone, as is
// the "Default storefront design" row ("Generic templates" vs
// "Editorial, designer-led") was opinion under a factual heading and is gone.
export const COMPARISON_ROWS: readonly CompareRow[] = [
  {
    label: "Free to try",
    mark8ly: "90 days",
    competitors: {
      shopify: "3 days",
      bigcommerce: "15 days",
      wix: "Free plan; 14-day refund",
      squarespace: "14 days",
    },
  },
  {
    label: "Platform fee per sale",
    mark8ly: "None",
    competitors: {
      shopify: "2%, unless you use Shopify Payments",
      bigcommerce: "None with built-in providers, else 2%",
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
];

/** Oldest verification date across competitors, or null if any never was. */
export function comparisonAsOf(): string | null {
  const dates = COMPETITORS.map((c) => c.verifiedOn);
  if (dates.some((d) => d === null)) return null;
  return (dates as string[]).sort()[0]!;
}
