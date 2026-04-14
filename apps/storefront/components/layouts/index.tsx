import type { HomepageContent } from "@/lib/api/marketplace-api";
import type { PublicStore } from "@/lib/api/platform-api";
import type { StorefrontTheme } from "@repo/ui/storefront-theme";

import { BoldPromoLayout } from "./BoldPromoLayout";
import { CatalogFirstLayout } from "./CatalogFirstLayout";
import { ClassicShopLayout } from "./ClassicShopLayout";
import { CompactLayout } from "./CompactLayout";
import { EditorialLayout } from "./EditorialLayout";
import { MinimalLayout } from "./MinimalLayout";
import { SplitHeroLayout } from "./SplitHeroLayout";
import { StoryLedLayout } from "./StoryLedLayout";

/**
 * Layout dispatcher — picks the layout component for `theme.layout`.
 *
 * Adding a new variant is a one-file change here plus the new layout
 * file. The dispatcher is the only place that needs to know about the
 * full enum, so app/page.tsx stays a thin entry point.
 */
export function StorefrontLayoutRenderer({
  store,
  theme,
  content,
}: {
  store: PublicStore;
  theme: StorefrontTheme;
  content?: HomepageContent | null;
}) {
  switch (theme.layout) {
    case "classic-shop":
      return <ClassicShopLayout store={store} theme={theme} content={content} />;
    case "split-hero":
      return <SplitHeroLayout store={store} theme={theme} content={content} />;
    case "catalog-first":
      return <CatalogFirstLayout store={store} theme={theme} content={content} />;
    case "story-led":
      return <StoryLedLayout store={store} theme={theme} content={content} />;
    case "minimal":
      return <MinimalLayout store={store} theme={theme} content={content} />;
    case "bold-promo":
      return <BoldPromoLayout store={store} theme={theme} content={content} />;
    case "compact":
      return <CompactLayout store={store} theme={theme} content={content} />;
    case "editorial":
    default:
      return <EditorialLayout store={store} theme={theme} content={content} />;
  }
}

export {
  BoldPromoLayout,
  CatalogFirstLayout,
  ClassicShopLayout,
  CompactLayout,
  EditorialLayout,
  MinimalLayout,
  SplitHeroLayout,
  StoryLedLayout,
};
