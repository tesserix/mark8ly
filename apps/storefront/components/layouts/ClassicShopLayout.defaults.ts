import type { HomepageHero, HomepageSection } from "@/lib/api/marketplace-api";
import type { PublicStore } from "@/lib/api/platform-api";

export function defaultClassicShopContent(store: PublicStore): {
  hero?: HomepageHero;
  sections: HomepageSection[];
} {
  return {
    hero: {
      enabled: true,
      heading: store.name,
      subheading: "A clean storefront for merchants who want familiar footing without sacrificing craft.",
      cta_label: "Shop everything",
      cta_url: "/products",
    },
    sections: [
      {
        type: "featured_products",
        heading: "What's in this week",
        limit: 4,
      },
      {
        type: "text",
        markdown: [
          "**Free shipping** — Worldwide, carbon offset at checkout.",
          "",
          "**Thirty day returns** — Change your mind, no questions asked.",
          "",
          "**Made to last** — Repairable, recyclable, documented.",
        ].join("\n"),
      },
    ],
  };
}
