import type { HomepageSection } from "@/lib/api/marketplace-api";
import type { StorefrontTheme } from "@repo/ui/storefront-theme";

import { TextSection } from "./sections/TextSection";
import { ImageSection } from "./sections/ImageSection";
import { FeaturedProductsSection } from "./sections/FeaturedProductsSection";
import { QuoteSection } from "./sections/QuoteSection";
import { MarqueeSection } from "./sections/MarqueeSection";
import { PullQuoteSection } from "./sections/PullQuoteSection";
import { LetterSection } from "./sections/LetterSection";

type SectionsRendererProps = {
  sections: HomepageSection[];
  theme: StorefrontTheme;
  storeSlug: string;
};

export function SectionsRenderer({
  sections,
  theme,
  storeSlug,
}: SectionsRendererProps) {
  if (sections.length === 0) return null;
  return (
    <div className="space-y-16 py-16">
      {sections.map((s, i) => {
        const key = `${s.type}-${i}`;
        switch (s.type) {
          case "text":
            return <TextSection key={key} section={s} theme={theme} />;
          case "image":
            return <ImageSection key={key} section={s} theme={theme} />;
          case "quote":
            return <QuoteSection key={key} section={s} theme={theme} />;
          case "featured_products":
            return (
              <FeaturedProductsSection
                key={key}
                section={s}
                theme={theme}
                storeSlug={storeSlug}
              />
            );
          case "marquee":
            return <MarqueeSection key={key} section={s} theme={theme} />;
          case "pull_quote":
            return <PullQuoteSection key={key} section={s} theme={theme} />;
          case "letter":
            return <LetterSection key={key} section={s} theme={theme} />;
          default:
            // Forward-compat: an unknown section type (e.g. one added in a
            // later build and served by a stale storefront) renders as a
            // silent gap rather than crashing the homepage.
            return null;
        }
      })}
    </div>
  );
}
