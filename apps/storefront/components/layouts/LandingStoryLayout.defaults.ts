import type { HomepageHero, HomepageSection } from "@/lib/api/marketplace-api";
import type { PublicStore } from "@/lib/api/platform-api";

export function defaultLandingStoryContent(store: PublicStore): {
  hero?: HomepageHero;
  sections: HomepageSection[];
} {
  return {
    hero: {
      enabled: true,
      heading: store.name,
      subheading: `A brand launch, a long read, and the products that tell the story.`,
      cta_label: "Read the story",
      cta_url: "/pages/about",
      eyebrow: "The story",
      cta_secondary_label: "Shop now",
      cta_secondary_url: "/products",
    },
    sections: [
      {
        type: "text",
        markdown: `${store.name} started from a simple idea — make things worth keeping. Every piece that follows is a direct result of that idea.`,
      },
      {
        type: "image",
        url: "/layout-placeholders/editorial-cover.svg",
        alt: `${store.name} origin image`,
      },
      {
        type: "pull_quote",
        text: "We made fewer things, and we made them for longer.",
        attribution: `${store.name} founder`,
      },
      {
        type: "letter",
        eyebrow: "How we build",
        title: "Built slowly, on purpose.",
        body: `Every product in ${store.name} is built to outlast a season — materials sourced locally where possible, repairable construction, and packaging that goes back into the earth.`,
        cta_label: "About the studio",
        cta_url: "/pages/about",
      },
      {
        type: "featured_products",
        heading: "Start here",
        limit: 4,
      },
      {
        type: "marquee",
        items: ["Made to last", "Low and slow", store.slug.toUpperCase(), "Est. 2026"],
      },
    ],
  };
}
