import type { HomepageHero, HomepageSection } from "@/lib/api/marketplace-api";
import type { PublicStore } from "@/lib/api/platform-api";

/**
 * Grid Dense — catalog-led landing. A trimmed hero (or none), then product
 * blocks back-to-back. Suits practical browsing — marketplaces, tools,
 * electronics, anywhere merchants want maximum density.
 */
export function defaultGridDenseContent(store: PublicStore): {
  hero?: HomepageHero;
  sections: HomepageSection[];
} {
  return {
    hero: {
      enabled: true,
      heading: store.name,
      subheading: "Browse the catalog.",
      cta_label: "Shop all",
      cta_url: "/products",
      eyebrow: "Now in stock",
    },
    sections: [
      {
        type: "featured_products",
        heading: "Top picks",
        limit: 8,
      },
      {
        type: "featured_products",
        heading: "New arrivals",
        limit: 8,
      },
      {
        type: "featured_products",
        heading: "Bestsellers",
        limit: 8,
      },
    ],
  };
}
