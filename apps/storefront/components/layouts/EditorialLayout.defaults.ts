import type { HomepageHero, HomepageSection } from "@/lib/api/marketplace-api";
import type { PublicStore } from "@/lib/api/platform-api";

/**
 * Editorial — default content for a brand-new store using the editorial
 * layout. Kicks in when `branding.homepage_content.sections` is empty.
 * Once the merchant saves their own sections this recipe stops running.
 */
export function defaultEditorialContent(store: PublicStore): {
  hero?: HomepageHero;
  sections: HomepageSection[];
} {
  return {
    hero: {
      enabled: true,
      heading: store.name,
      subheading: `A considered storefront for ${store.name.toLowerCase()} — a place where the catalog feels written, not assembled.`,
      cta_label: "Shop the edit",
      cta_url: "/products",
      eyebrow: "Issue 01 · Now open",
      cta_secondary_label: "Read the story",
      cta_secondary_url: "/pages/about",
      aside_image_url: "/layout-placeholders/editorial-cover.svg",
      aside_image_alt: `${store.name} cover story`,
    },
    sections: [
      {
        type: "marquee",
        items: [
          "Hand picked",
          "Small batch",
          "Ships worldwide",
          store.slug.toUpperCase(),
          "Est. 2026",
        ],
      },
      {
        type: "pull_quote",
        text: "We chose fewer things and we chose them well.",
        attribution: `Editor's note · ${store.name}`,
      },
      {
        type: "featured_products",
        heading: "Three pieces we love right now",
        limit: 3,
      },
      {
        type: "letter",
        eyebrow: "Letter from the studio",
        title: "Built for the long shelf life.",
        body: `Every piece in ${store.name} is chosen to outlast a season. Paper packaging, repairable construction, and a quiet attitude toward trend cycles.`,
        cta_label: "About the studio",
        cta_url: "/pages/about",
      },
    ],
  };
}
