import type { HomepageHero, HomepageSection } from "@/lib/api/marketplace-api";
import type { PublicStore } from "@/lib/api/platform-api";

export function defaultCompactContent(store: PublicStore): {
  hero?: HomepageHero;
  sections: HomepageSection[];
} {
  return {
    hero: {
      enabled: true,
      heading: store.name,
      subheading: `${store.slug}.mark8ly.com — ships from ${store.country_code}.`,
      cta_label: "Browse catalog",
      cta_url: "/products",
    },
    sections: [
      {
        type: "featured_products",
        heading: `All ${store.name} products`,
        limit: 6,
      },
    ],
  };
}
