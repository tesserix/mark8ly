import type { MetadataRoute } from "next";

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

  return [...primary, ...legal];
}
