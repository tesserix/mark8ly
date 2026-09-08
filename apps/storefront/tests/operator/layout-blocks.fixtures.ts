/**
 * Shared fixtures for the layout-blocks visual-regression spec.
 * Kept separate so adding a new layout or tweaking the "custom"
 * payload is a one-file change both the spec and any future
 * admin-side preview can consume.
 */

export type StorefrontLayoutKey =
  | "editorial"
  | "classic-shop"
  | "split-hero"
  | "catalog-first"
  | "story-led"
  | "minimal"
  | "bold-promo"
  | "compact";

export const LAYOUTS: StorefrontLayoutKey[] = [
  "editorial",
  "classic-shop",
  "split-hero",
  "catalog-first",
  "story-led",
  "minimal",
  "bold-promo",
  "compact",
];

export interface HomepageHero {
  enabled: boolean;
  heading?: string | null;
  subheading?: string | null;
  cta_label?: string | null;
  cta_url?: string | null;
  image_url?: string | null;
  eyebrow?: string | null;
  cta_secondary_label?: string | null;
  cta_secondary_url?: string | null;
  aside_image_url?: string | null;
  aside_image_alt?: string | null;
}

export type HomepageSection =
  | { type: "text"; heading?: string | null; markdown: string }
  | {
      type: "image";
      url: string;
      alt?: string | null;
      caption?: string | null;
      heading?: string | null;
    }
  | {
      type: "featured_products";
      collection_slug?: string;
      product_slugs?: string[];
      limit?: number;
      heading?: string | null;
    }
  | {
      type: "quote";
      text: string;
      attribution?: string | null;
      heading?: string | null;
    }
  | { type: "marquee"; items: string[]; speed?: "slow" | "normal" | "fast" }
  | {
      type: "pull_quote";
      text: string;
      attribution?: string | null;
      heading?: string | null;
    }
  | {
      type: "letter";
      eyebrow?: string | null;
      title: string;
      body: string;
      cta_label?: string | null;
      cta_url?: string | null;
    };

export interface HomepageContent {
  hero?: HomepageHero;
  sections?: HomepageSection[];
}

export const EMPTY_CONTENT: HomepageContent = {};

/**
 * Custom content exercises every new block type once so the
 * snapshot catches regressions in any one renderer. Kept short so
 * a full-page screenshot is tractable.
 */
export const CUSTOM_CONTENT: HomepageContent = {
  hero: {
    enabled: true,
    heading: "Visual regression harness",
    subheading: "This store's homepage is driven by the test fixture.",
    cta_label: "Shop",
    cta_url: "/products",
    eyebrow: "Fixture eyebrow",
    cta_secondary_label: "Secondary",
    cta_secondary_url: "/pages/about",
    aside_image_url: "/layout-placeholders/editorial-cover.svg",
    aside_image_alt: "Fixture aside image",
  },
  sections: [
    {
      type: "marquee",
      items: ["Snapshot", "Visual diff", "Pixel perfect", "Layout blocks"],
    },
    {
      type: "pull_quote",
      text: "Automate the eyeball — ship with confidence.",
      attribution: "Playwright",
    },
    {
      type: "text",
      heading: "A heading",
      markdown: "A paragraph of merchant-authored **markdown** with a [link](/pages/about).",
    },
    {
      type: "letter",
      eyebrow: "Test fixture",
      title: "Letter block lives here.",
      body: "Multi-line letter body rendered via the markdown pipeline to catch body-copy regressions.",
      cta_label: "About",
      cta_url: "/pages/about",
    },
  ],
};
