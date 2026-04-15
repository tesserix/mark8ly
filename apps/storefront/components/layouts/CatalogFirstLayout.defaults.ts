import type { HomepageHero, HomepageSection } from "@/lib/api/marketplace-api";
import type { PublicStore } from "@/lib/api/platform-api";

export function defaultCatalogFirstContent(store: PublicStore): {
  hero?: HomepageHero;
  sections: HomepageSection[];
} {
  return {
    hero: {
      enabled: true,
      heading: store.name,
      subheading: "The catalog — shop everything.",
      cta_label: "Browse all",
      cta_url: "/products",
      eyebrow: "The catalog",
    },
    sections: [
      {
        type: "featured_products",
        heading: "Featured",
        limit: 5,
      },
      {
        type: "featured_products",
        heading: "Shop by mood",
        limit: 3,
      },
    ],
  };
}
