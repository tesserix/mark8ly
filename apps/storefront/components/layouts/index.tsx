import type { HomepageContent } from "@/lib/api/marketplace-api";
import type { PublicStore } from "@/lib/api/platform-api";
import type { StorefrontTheme } from "@repo/ui/storefront-theme";

import { BoldPromoLayout } from "./BoldPromoLayout";
import { CatalogFirstLayout } from "./CatalogFirstLayout";
import { ClassicShopLayout } from "./ClassicShopLayout";
import { CollectionShowcaseLayout } from "./CollectionShowcaseLayout";
import { CompactLayout } from "./CompactLayout";
import { EditorialLayout } from "./EditorialLayout";
import { GridDenseLayout } from "./GridDenseLayout";
import { LandingStoryLayout } from "./LandingStoryLayout";
import { LookbookLayout } from "./LookbookLayout";
import { MinimalLayout } from "./MinimalLayout";
import { ProductSpotlightLayout } from "./ProductSpotlightLayout";
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
  // Cast widens the switch to accept legacy enum values (e.g. "hero-focus")
  // that existed in older branding records. These alias to the closest
  // modern layout rather than silently falling back to editorial.
  switch (theme.layout as string) {
    case "classic-shop":
      return <ClassicShopLayout store={store} theme={theme} content={content} />;
    case "split-hero":
    case "hero-focus":
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
    case "lookbook":
      return <LookbookLayout store={store} theme={theme} content={content} />;
    case "grid-dense":
      return <GridDenseLayout store={store} theme={theme} content={content} />;
    case "collection-showcase":
      return <CollectionShowcaseLayout store={store} theme={theme} content={content} />;
    case "product-spotlight":
      return <ProductSpotlightLayout store={store} theme={theme} content={content} />;
    case "landing-story":
      return <LandingStoryLayout store={store} theme={theme} content={content} />;
    case "editorial":
    default:
      return <EditorialLayout store={store} theme={theme} content={content} />;
  }
}

export {
  BoldPromoLayout,
  CatalogFirstLayout,
  ClassicShopLayout,
  CollectionShowcaseLayout,
  CompactLayout,
  EditorialLayout,
  GridDenseLayout,
  LandingStoryLayout,
  LookbookLayout,
  MinimalLayout,
  ProductSpotlightLayout,
  SplitHeroLayout,
  StoryLedLayout,
};
