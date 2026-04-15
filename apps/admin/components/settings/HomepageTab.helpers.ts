import type { HomepageContent, HomepageSection } from "@/lib/api/marketplace-api";

/**
 * Admin-side starter recipes. Purpose: populate the editor with a
 * reasonable starting point when the merchant clicks "Start from theme
 * defaults" on an empty homepage. The authoritative render-time recipes
 * live in the storefront (`apps/storefront/components/layouts/*.defaults.ts`);
 * these are a parallel minimal set so the admin can seed the form
 * without pulling storefront code into the admin bundle. Keep roughly
 * in sync when storefront recipes change materially.
 */

export interface StoreInfo {
  name: string;
  slug: string;
  countryCode?: string | null;
}

export function starterRecipeFor(
  layoutVariant: string,
  store: StoreInfo,
): HomepageContent {
  switch (layoutVariant) {
    case "editorial":
      return editorialRecipe(store);
    case "bold-promo":
      return boldPromoRecipe(store);
    case "catalog-first":
      return catalogFirstRecipe(store);
    case "classic-shop":
      return classicShopRecipe(store);
    case "compact":
      return compactRecipe(store);
    case "minimal":
      return minimalRecipe(store);
    case "split-hero":
      return splitHeroRecipe(store);
    case "story-led":
      return storyLedRecipe(store);
    default:
      return genericRecipe(store);
  }
}

function editorialRecipe(store: StoreInfo): HomepageContent {
  const sections: HomepageSection[] = [
    {
      type: "marquee",
      items: [
        "Hand picked",
        "Small batch",
        "Ships worldwide",
        store.slug.toUpperCase(),
        "Est. 2026",
      ],
    },
    {
      type: "pull_quote",
      text: "We chose fewer things and we chose them well.",
      attribution: `Editor's note · ${store.name}`,
    },
    { type: "featured_products", heading: "Three pieces we love right now", limit: 3 },
    {
      type: "letter",
      eyebrow: "Letter from the studio",
      title: "Built for the long shelf life.",
      body: `Every piece in ${store.name} is chosen to outlast a season. Paper packaging, repairable construction, and a quiet attitude toward trend cycles.`,
      cta_label: "About the studio",
      cta_url: "/pages/about",
    },
  ];
  return {
    hero: {
      enabled: true,
      heading: store.name,
      subheading: `A considered storefront for ${store.name.toLowerCase()} — a place where the catalog feels written, not assembled.`,
      cta_label: "Shop the edit",
      cta_url: "/products",
      eyebrow: "Issue 01 · Now open",
      cta_secondary_label: "Read the story",
      cta_secondary_url: "/pages/about",
      aside_image_url: "/layout-placeholders/editorial-cover.svg",
      aside_image_alt: `${store.name} cover story`,
    },
    sections,
  };
}

function boldPromoRecipe(store: StoreInfo): HomepageContent {
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
      { type: "featured_products", heading: "The pieces", limit: 3 },
      {
        type: "pull_quote",
        text: "Twelve pieces. One drop. No encores.",
        attribution: `${store.name} · Drop 01`,
      },
    ],
  };
}

function catalogFirstRecipe(store: StoreInfo): HomepageContent {
  return {
    hero: {
      enabled: true,
      heading: store.name,
      subheading: "The catalog — shop everything.",
      cta_label: "Browse all",
      cta_url: "/products",
      eyebrow: "The catalog",
    },
    sections: [
      { type: "featured_products", heading: "Featured", limit: 5 },
      { type: "featured_products", heading: "Shop by mood", limit: 3 },
    ],
  };
}

function classicShopRecipe(store: StoreInfo): HomepageContent {
  return {
    hero: {
      enabled: true,
      heading: store.name,
      subheading:
        "A clean storefront for merchants who want familiar footing without sacrificing craft.",
      cta_label: "Shop everything",
      cta_url: "/products",
      eyebrow: "Spring collection · Live now",
    },
    sections: [
      { type: "featured_products", heading: "What's in this week", limit: 4 },
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

function compactRecipe(store: StoreInfo): HomepageContent {
  const cc = store.countryCode ?? "—";
  return {
    hero: {
      enabled: true,
      heading: store.name,
      subheading: `${store.slug}.mark8ly.com — ships from ${cc}.`,
      cta_label: "Browse catalog",
      cta_url: "/products",
    },
    sections: [
      { type: "featured_products", heading: `All ${store.name} products`, limit: 6 },
    ],
  };
}

function minimalRecipe(store: StoreInfo): HomepageContent {
  return {
    hero: {
      enabled: true,
      heading: store.name,
      subheading: "Fewer things, chosen carefully.",
      cta_label: "Enter the store",
      cta_url: "/products",
      eyebrow: "Now open",
    },
    sections: [{ type: "featured_products", limit: 1 }],
  };
}

function splitHeroRecipe(store: StoreInfo): HomepageContent {
  return {
    hero: {
      enabled: true,
      heading: store.name,
      subheading:
        "A cinematic opening frame — product on the right, narrative on the left. Equal weight, no competition.",
      cta_label: "Shop the drop",
      cta_url: "/products",
      eyebrow: "Featured",
      aside_image_url: "/layout-placeholders/split-aside.svg",
      aside_image_alt: `${store.name} featured`,
    },
    sections: [{ type: "featured_products", limit: 3 }],
  };
}

function storyLedRecipe(store: StoreInfo): HomepageContent {
  const cc = store.countryCode ?? "the studio";
  return {
    hero: {
      enabled: true,
      heading: store.name,
      subheading: `${store.name} began as a single question: what if a store felt like a letter from the people who made it?`,
      cta_label: "Read the story",
      cta_url: "/pages/about",
      eyebrow: "From the studio",
      aside_image_url: "/layout-placeholders/story-aside.svg",
      aside_image_alt: "Studio workshop",
    },
    sections: [
      {
        type: "text",
        markdown: [
          `Every piece starts in a small studio in ${cc}. The catalog is short by design — we only list things we've worn, used, and lived with.`,
          "",
          "Seasons move slower here. Restocks happen when the work is done. Customer notes get read the same week they arrive.",
        ].join("\n"),
      },
      {
        type: "pull_quote",
        text: "The best storefronts feel like walking into a studio, not a supermarket.",
        attribution: `${store.name} · Founder's note`,
      },
      { type: "featured_products", heading: "Chapters", limit: 2 },
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

function genericRecipe(store: StoreInfo): HomepageContent {
  return {
    hero: {
      enabled: true,
      heading: store.name,
      cta_label: "Shop",
      cta_url: "/products",
    },
    sections: [{ type: "featured_products", limit: 4 }],
  };
}
