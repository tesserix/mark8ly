import type { HomepageSection } from "@/lib/api/marketplace-api";
import type { StorefrontTheme } from "@repo/ui/storefront-theme";

import { TextSection } from "./sections/TextSection";
import { ImageSection } from "./sections/ImageSection";
import { FeaturedProductsSection } from "./sections/FeaturedProductsSection";
import { QuoteSection } from "./sections/QuoteSection";

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
          default:
            return null;
        }
      })}
    </div>
  );
}
