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

/**
 * Real last-content-change dates, one per route, checked in.
 *
 * These used to be `new Date()` — the moment the sitemap was generated. That
 * told every crawler that all eighteen pages had changed, every time, which
 * is a signal we were actively making meaningless (tesserix/mark8ly#603).
 * The guides never had the problem: they carry a real per-article `updated`
 * date and always have.
 *
 * The values below are the dates the routes' own sources were last changed
 * in git, not plausible-looking dates picked to fill the field. Keep it that
 * way: when you change a page's content, move its date here in the same
 * commit. `git log -1 --format=%cs -- app/<route>/page.tsx` is where these
 * came from and is how to check one.
 *
 * Purely visual or structural edits — a shared stylesheet, the footer, a
 * schema tweak — are deliberately NOT reflected here. `lastmod` is a claim
 * about the content a searcher would read, and inflating it for a CSS change
 * is the same lie in slower motion.
 *
 * /guides is absent on purpose: the hub's content IS the guide list, so its
 * date is derived from the guides themselves below and cannot drift.
 */
const LAST_MODIFIED: Readonly<Record<string, string>> = {
  "/": "2026-09-03",
  "/about": "2026-08-11",
  "/contact": "2026-09-02",
  "/help": "2026-09-02",
  "/integrations": "2026-09-02",

  "/shopify-alternative": "2026-08-11",
  "/ecommerce-for-makers": "2026-08-11",
  "/sell-online-india": "2026-09-03",
  "/etsy-alternative": "2026-08-11",

  "/legal": "2026-09-02",
  "/privacy": "2026-09-02",
  "/terms": "2026-09-02",
  "/acceptable-use": "2026-09-02",
  "/cookies": "2026-09-02",
  "/refunds": "2026-09-02",
  "/sub-processors": "2026-09-02",
  "/security": "2026-09-02",
};

/**
 * Throws rather than falling back to "the current date" for an unlisted
 * route. The whole point of #603 was that a generated timestamp is
 * indistinguishable from a real one, so a silent default would let the bug
 * back in the first time someone adds a page. sitemap.ts is prerendered, so
 * this fails the build rather than shipping a lie.
 */
function lastModified(path: string): Date {
  const date = LAST_MODIFIED[path];
  if (!date) {
    throw new Error(
      `sitemap: no lastModified date for "${path}". Add one to ` +
        "LAST_MODIFIED — see the note above it, and do not reintroduce a " +
        "generated-at-build fallback (#603).",
    );
  }
  return new Date(date);
}

export default function sitemap(): MetadataRoute.Sitemap {

  // Canonical marketing pages — fully indexed.
  const primary: MetadataRoute.Sitemap = [
    {
      // `${SITE_URL}` with no trailing slash, matching the canonical the
      // homepage actually renders (<link rel="canonical"
      // href="https://mark8ly.com">). The sitemap used to say
      // "https://mark8ly.com/" while the page said "https://mark8ly.com",
      // so the two disagreed about the site's own front door
      // (tesserix/mark8ly#603). Google normalises the pair, so this was
      // never costing us rankings — it is fixed because there is no reason
      // for a site to be inconsistent about its most important URL, and
      // because the next person to diff the two should not have to work
      // out which one is authoritative.
      //
      // No-slash is the form every other reference in the app already uses
      // (SITE_URL itself, the JSON-LD @ids, the OG urls), so the sitemap
      // was the single outlier.
      url: SITE_URL,
      lastModified: lastModified("/"),
      changeFrequency: "weekly",
      priority: 1.0,
    },
    {
      url: `${SITE_URL}/about`,
      lastModified: lastModified("/about"),
      changeFrequency: "monthly",
      priority: 0.8,
    },
    {
      url: `${SITE_URL}/contact`,
      lastModified: lastModified("/contact"),
      changeFrequency: "monthly",
      priority: 0.7,
    },
    {
      url: `${SITE_URL}/help`,
      lastModified: lastModified("/help"),
      changeFrequency: "monthly",
      priority: 0.6,
    },
    {
      url: `${SITE_URL}/integrations`,
      lastModified: lastModified("/integrations"),
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
    lastModified: lastModified(path),
    changeFrequency: "monthly",
    priority: 0.9,
  }));

  // Guides — informational content hub + articles (indexed).
  // The hub lists the guides and nothing else, so it last changed when the
  // most recently updated guide did. Derived rather than checked in — a
  // constant here could fall out of step with the articles it summarises.
  const guidesUpdated = GUIDES.map((g) => new Date(g.updated).getTime());

  const guides: MetadataRoute.Sitemap = [
    {
      url: `${SITE_URL}/guides`,
      lastModified: new Date(Math.max(...guidesUpdated)),
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
    lastModified: lastModified(path),
    changeFrequency: "yearly",
    priority: 0.3,
  }));

  return [...primary, ...landing, ...guides, ...legal];
}
