import type { HomepageHero, HomepageSection } from "@/lib/api/marketplace-api";
import type { PublicStore } from "@/lib/api/platform-api";

export function defaultCollectionShowcaseContent(store: PublicStore): {
  hero?: HomepageHero;
  sections: HomepageSection[];
} {
  return {
    hero: {
      enabled: true,
      heading: store.name,
      subheading: "Shop by collection.",
      cta_label: "Browse all",
      cta_url: "/products",
      eyebrow: "Collections",
    },
    sections: [
      {
        type: "marquee",
        items: ["New this week", "Most loved", "Gift ideas", "Back in stock"],
      },
      {
        type: "featured_products",
        heading: "Most loved",
        limit: 4,
      },
      {
        type: "text",
        markdown: "Every piece curated, tested, and packed with care.",
      },
      {
        type: "featured_products",
        heading: "New this week",
        limit: 4,
      },
    ],
  };
}
