import type { MetadataRoute } from "next";

import { GUIDES } from "./guides/guides";

/**
 * sitemap.xml — Next 16 file convention. Emits every public
 * marketing route so search engines and LLM crawlers can index
 * the surface without following links.
 *
 * The onboarding funnel and the welcome page are intentionally
 * excluded — they&apos;re funnel destinations, not content, and
 * don&apos;t need to show up in search.
 */

const SITE_URL = "https://mark8ly.com";

export default function sitemap(): MetadataRoute.Sitemap {
  const now = new Date();

  // Canonical marketing pages — fully indexed.
  const primary: MetadataRoute.Sitemap = [
    {
      url: `${SITE_URL}/`,
      lastModified: now,
      changeFrequency: "weekly",
      priority: 1.0,
    },
    {
      url: `${SITE_URL}/about`,
      lastModified: now,
      changeFrequency: "monthly",
      priority: 0.8,
    },
    {
      url: `${SITE_URL}/contact`,
      lastModified: now,
      changeFrequency: "monthly",
      priority: 0.7,
    },
    {
      url: `${SITE_URL}/help`,
      lastModified: now,
      changeFrequency: "monthly",
      priority: 0.6,
    },
    {
      url: `${SITE_URL}/integrations`,
      lastModified: now,
      changeFrequency: "monthly",
      priority: 0.6,
    },
  ];

  // SEO landing pages — high-intent comparison/alternative
  // surfaces (#151). Indexed with strong priority; they're the
  // pages built to capture buying-intent search traffic.
  const landingRoutes: ReadonlyArray<string> = [
    "/shopify-alternative",
    "/ecommerce-for-makers",
    "/sell-online-india",
    "/etsy-alternative",
  ];
  const landing: MetadataRoute.Sitemap = landingRoutes.map((path) => ({
    url: `${SITE_URL}${path}`,
    lastModified: now,
    changeFrequency: "monthly",
    priority: 0.9,
  }));

  // Guides — informational content hub + articles (indexed).
  const guides: MetadataRoute.Sitemap = [
    {
      url: `${SITE_URL}/guides`,
      lastModified: now,
      changeFrequency: "weekly",
      priority: 0.7,
    },
    ...GUIDES.map((g) => ({
      url: `${SITE_URL}/guides/${g.slug}`,
      lastModified: new Date(g.updated),
      changeFrequency: "monthly" as const,
      priority: 0.7,
    })),
  ];

  // Legal — slowly changing, lower priority but indexed.
  // /dpa is intentionally excluded: noindex because it's the
  // auto-accepted controller-processor contract and doesn't
  // belong in search results.
  const legalRoutes: ReadonlyArray<string> = [
    "/legal",
    "/privacy",
    "/terms",
    "/acceptable-use",
    "/cookies",
    "/refunds",
    "/sub-processors",
    "/security",
  ];
  const legal: MetadataRoute.Sitemap = legalRoutes.map((path) => ({
    url: `${SITE_URL}${path}`,
    lastModified: now,
    changeFrequency: "yearly",
    priority: 0.3,
  }));

  return [...primary, ...landing, ...guides, ...legal];
}
