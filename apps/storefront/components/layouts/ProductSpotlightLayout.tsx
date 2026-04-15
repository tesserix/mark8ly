import { HeroSection } from "@/components/homepage/HeroSection";
import { SectionsRenderer } from "@/components/homepage/SectionsRenderer";

import { defaultProductSpotlightContent } from "./ProductSpotlightLayout.defaults";
import { resolveLayoutContent, type LayoutProps } from "./shared";

/**
 * Product Spotlight — narrow max-width, hero-forward, one featured product
 * gets real estate before a lean catalog. Suits merchants launching a
 * signature item or running a single-SKU-forward storefront.
 */
export function ProductSpotlightLayout({ store, theme, content }: LayoutProps) {
  const resolved = resolveLayoutContent(content, defaultProductSpotlightContent(store));
  return (
    <article className="mx-auto max-w-5xl space-y-20 px-4 sm:px-6">
      <HeroSection hero={resolved.hero} theme={theme} fallbackHeading={store.name} />
      <SectionsRenderer sections={resolved.sections} theme={theme} storeSlug={store.slug} />
    </article>
  );
}
