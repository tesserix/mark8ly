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
      legalName: "Tesserix Pty Ltd",
      // The registration numbers are the point of this block, not
      // decoration. "Mark8ly" and "the Mark8ly team" are assertions;
      // an ACN is a fact anyone can check against the ASIC register in
      // about ten seconds, and an ABN against ABN Lookup. That is the
      // one Authoritativeness signal available to a company with no
      // trading history yet (tesserix/mark8ly#594), and it costs us
      // nothing because both numbers are already published on /privacy,
      // /terms, /security and /sub-processors. Digits only, unspaced —
      // that is the form both registers index on.
      identifier: [
        { "@type": "PropertyValue", propertyID: "ACN", value: "694070865" },
        { "@type": "PropertyValue", propertyID: "ABN", value: "59694070865" },
      ],
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
      // Tesserix is an Australian entity registered in New South
      // Wales — that much is stated on /privacy, so emitting the region
      // here is reporting our own published legal text, not guesswork.
      // Still no locality: /privacy names Sydney as where operations
      // are *conducted*, which is not the same claim as a registered
      // office, and fabricating a city to fill the schema shape stays
      // worse than a narrower-but-accurate address.
      address: {
        "@type": "PostalAddress",
        addressCountry: "AU",
        addressRegion: "NSW",
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
