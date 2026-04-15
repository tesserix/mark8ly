import type { HomepageHero, HomepageSection } from "@/lib/api/marketplace-api";
import type { PublicStore } from "@/lib/api/platform-api";

export function defaultBoldPromoContent(store: PublicStore): {
  hero?: HomepageHero;
  sections: HomepageSection[];
} {
  return {
    hero: {
      enabled: true,
      heading: store.name,
      subheading: "A limited edition release. When it's gone, it's gone.",
      cta_label: "Shop the drop",
      cta_url: "/products",
      eyebrow: "The drop · Live now",
      cta_secondary_label: "Lookbook",
      cta_secondary_url: "/pages/lookbook",
    },
    sections: [
      {
        type: "featured_products",
        heading: "The pieces",
        limit: 3,
      },
      {
        type: "pull_quote",
        text: "Twelve pieces. One drop. No encores.",
        attribution: `${store.name} · Drop 01`,
      },
    ],
  };
}
