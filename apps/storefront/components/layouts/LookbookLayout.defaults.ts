import type { HomepageHero, HomepageSection } from "@/lib/api/marketplace-api";
import type { PublicStore } from "@/lib/api/platform-api";

/**
 * Lookbook — image-forward layout for fashion/lifestyle brands. Big-bleed
 * hero, generous pacing, large image sections between product blocks.
 */
export function defaultLookbookContent(store: PublicStore): {
  hero?: HomepageHero;
  sections: HomepageSection[];
} {
  return {
    hero: {
      enabled: true,
      heading: store.name,
      subheading: `The ${store.name.toLowerCase()} lookbook — seasonal stories, product on location.`,
      cta_label: "Shop the lookbook",
      cta_url: "/products",
      eyebrow: "Spring / Summer",
      cta_secondary_label: "Read the notes",
      cta_secondary_url: "/pages/about",
      aside_image_url: "/layout-placeholders/editorial-cover.svg",
      aside_image_alt: `${store.name} lookbook cover`,
    },
    sections: [
      {
        type: "image",
        url: "/layout-placeholders/editorial-cover.svg",
        alt: `${store.name} editorial image`,
      },
      {
        type: "featured_products",
        heading: "Key pieces",
        limit: 4,
      },
      {
        type: "pull_quote",
        text: "Photographed on location. Worn in daylight.",
        attribution: `${store.name} studio`,
      },
      {
        type: "featured_products",
        heading: "Closing the story",
        limit: 3,
      },
    ],
  };
}
