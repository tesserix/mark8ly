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

const jsonLd = {
  "@context": "https://schema.org",
  "@graph": [
    {
      "@type": "Organization",
      "@id": `${SITE_URL}#organization`,
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
      publisher: { "@id": `${SITE_URL}#organization` },
      inLanguage: "en",
    },
  ],
};

export const SITE_JSON_LD = JSON.stringify(jsonLd);
