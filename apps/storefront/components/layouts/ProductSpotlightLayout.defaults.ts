import type { HomepageHero, HomepageSection } from "@/lib/api/marketplace-api";
import type { PublicStore } from "@/lib/api/platform-api";

export function defaultProductSpotlightContent(store: PublicStore): {
  hero?: HomepageHero;
  sections: HomepageSection[];
} {
  return {
    hero: {
      enabled: true,
      heading: store.name,
      subheading: `One product, done properly.`,
      cta_label: "See the spotlight",
      cta_url: "/products",
      eyebrow: "Spotlight",
      aside_image_url: "/layout-placeholders/editorial-cover.svg",
      aside_image_alt: `${store.name} spotlight product`,
    },
    sections: [
      {
        type: "featured_products",
        heading: "In the spotlight",
        limit: 1,
      },
      {
        type: "pull_quote",
        text: "Built to be the best version of itself.",
        attribution: store.name,
      },
      {
        type: "featured_products",
        heading: "Also worth a look",
        limit: 3,
      },
      {
        type: "letter",
        eyebrow: "Why this one",
        title: "A short note on the spotlight.",
        body: `${store.name} picks one product to show off at a time. Here is why we picked this one.`,
        cta_label: "Read more",
        cta_url: "/pages/about",
      },
    ],
  };
}
