import type { HomepageHero, HomepageSection } from "@/lib/api/marketplace-api";
import type { PublicStore } from "@/lib/api/platform-api";

export function defaultMinimalContent(store: PublicStore): {
  hero?: HomepageHero;
  sections: HomepageSection[];
} {
  return {
    hero: {
      enabled: true,
      heading: store.name,
      subheading: "Fewer things, chosen carefully.",
      cta_label: "Enter the store",
      cta_url: "/products",
      eyebrow: "Now open",
    },
    sections: [
      {
        type: "featured_products",
        limit: 1,
      },
    ],
  };
}
