import type { HomepageHero, HomepageSection } from "@/lib/api/marketplace-api";
import type { PublicStore } from "@/lib/api/platform-api";

export function defaultSplitHeroContent(store: PublicStore): {
  hero?: HomepageHero;
  sections: HomepageSection[];
} {
  return {
    hero: {
      enabled: true,
      heading: store.name,
      subheading: "A cinematic opening frame — product on the right, narrative on the left. Equal weight, no competition.",
      cta_label: "Shop the drop",
      cta_url: "/products",
    },
    sections: [
      {
        type: "featured_products",
        limit: 3,
      },
    ],
  };
}
