import { HeroSection } from "@/components/homepage/HeroSection";
import { SectionsRenderer } from "@/components/homepage/SectionsRenderer";

import { defaultCollectionShowcaseContent } from "./CollectionShowcaseLayout.defaults";
import { resolveLayoutContent, type LayoutProps } from "./shared";

/**
 * Collection Showcase — hero followed by category-led tiles above the
 * product grid. Suits merchants with a strong taxonomy (home goods,
 * apparel with clear collections, beauty with product families).
 */
export function CollectionShowcaseLayout({ store, theme, content }: LayoutProps) {
  const resolved = resolveLayoutContent(content, defaultCollectionShowcaseContent(store));
  return (
    <article className="space-y-14">
      <HeroSection hero={resolved.hero} theme={theme} fallbackHeading={store.name} />
      <SectionsRenderer sections={resolved.sections} theme={theme} storeSlug={store.slug} />
    </article>
  );
}
