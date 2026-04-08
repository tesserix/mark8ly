import type { MetadataRoute } from "next";

/**
 * robots.txt — Next 16 file convention. Generated at build
 * from this route, served at /robots.txt.
 *
 * Indexing policy:
 *   - Allow everything by default.
 *   - Disallow the onboarding funnel (/onboarding/*, /welcome):
 *     these are funnel pages, not marketing surfaces, and we
 *     don&apos;t want them appearing in search or LLM answers.
 *   - Disallow /api (no public API surface on this app anyway).
 *   - Point at the sitemap.
 */
export default function robots(): MetadataRoute.Robots {
  return {
    rules: [
      {
        userAgent: "*",
        allow: "/",
        disallow: ["/onboarding/", "/welcome", "/api/"],
      },
    ],
    sitemap: "https://mark8ly.com/sitemap.xml",
    host: "https://mark8ly.com",
  };
}
