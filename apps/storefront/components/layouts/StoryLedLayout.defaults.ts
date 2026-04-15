import type { HomepageHero, HomepageSection } from "@/lib/api/marketplace-api";
import type { PublicStore } from "@/lib/api/platform-api";

export function defaultStoryLedContent(store: PublicStore): {
  hero?: HomepageHero;
  sections: HomepageSection[];
} {
  return {
    hero: {
      enabled: true,
      heading: store.name,
      subheading: `${store.name} began as a single question: what if a store felt like a letter from the people who made it?`,
      cta_label: "Read the story",
      cta_url: "/pages/about",
    },
    sections: [
      {
        type: "text",
        markdown: [
          `Every piece starts in a small studio in ${store.country_code}. The catalog is short by design — we only list things we've worn, used, and lived with.`,
          "",
          "Seasons move slower here. Restocks happen when the work is done. Customer notes get read the same week they arrive.",
        ].join("\n"),
      },
      {
        type: "pull_quote",
        text: "The best storefronts feel like walking into a studio, not a supermarket.",
        attribution: `${store.name} · Founder's note`,
      },
      {
        type: "featured_products",
        heading: "Chapters",
        limit: 2,
      },
      {
        type: "letter",
        title: "When you're ready, we're ready.",
        body: "No urgency, no countdown timers. Just quiet things we made because we wanted them to exist.",
        cta_label: "Browse the catalog",
        cta_url: "/products",
      },
    ],
  };
}
