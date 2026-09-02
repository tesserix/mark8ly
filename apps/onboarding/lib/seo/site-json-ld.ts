/**
 * Site constants and the site-wide JSON-LD, serialized once so the root
 * layout and the CSP hash in lib/security/csp.ts are provably looking at
 * the same bytes — a mismatch would have the browser block the tag on
 * the nonce routes.
 *
 * Two graphs in a single script tag so Google + LLM crawlers pick up the
 * organization + website in one pass.
 */
export const SITE_URL = "https://mark8ly.com";
export const SITE_NAME = "Mark8ly";
export const SITE_TAGLINE = "Quiet commerce for people who make things";
export const SITE_DESCRIPTION =
  "A modern editorial commerce platform for independent merchants. Launch your store in an afternoon, keep every sale, and look considered from day one.";

/**
 * The `@id` of the Organization node below. Every page in the app carries
 * this graph via the root layout, and Google merges same-page JSON-LD — so
 * any other block that needs a publisher/author should reference this node
 * rather than declaring a second Organization of its own. A duplicate
 * declaration is invariably the poorer of the two (the guides' was missing
 * the logo entirely, tesserix/mark8ly#600), and it competes with the real
 * one for entity resolution.
 *
 * Exported so there is exactly one place the identifier is spelled.
 */
export const ORGANIZATION_ID = `${SITE_URL}#organization`;

const jsonLd = {
  "@context": "https://schema.org",
  "@graph": [
    {
      "@type": "Organization",
      "@id": ORGANIZATION_ID,
      name: SITE_NAME,
      legalName: "Tesserix",
      url: SITE_URL,
      // A square mark, not the 1200×630 social card. Google's
      // Organization logo guidance wants a logo image it can crop to
      // a square; handing it the OG banner gets the logo dropped from
      // knowledge-panel and AI-answer attribution.
      logo: `${SITE_URL}/icon-192.png`,
      description: SITE_DESCRIPTION,
      email: "hello@mark8ly.com",
      sameAs: [
        "https://twitter.com/mark8ly",
        "https://instagram.com/mark8ly",
        "https://linkedin.com/company/mark8ly",
      ],
      // Tesserix is an Australian entity. No specific locality
      // is emitted here until the legal registration address is
      // confirmed — fabricating a city to fill the schema shape is
      // worse than a narrower-but-accurate address.
      address: {
        "@type": "PostalAddress",
        addressCountry: "AU",
      },
    },
    {
      "@type": "WebSite",
      "@id": `${SITE_URL}#website`,
      url: SITE_URL,
      name: SITE_NAME,
      description: SITE_TAGLINE,
      publisher: { "@id": ORGANIZATION_ID },
      inLanguage: "en",
    },
  ],
};

export const SITE_JSON_LD = JSON.stringify(jsonLd);
