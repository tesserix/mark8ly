import { HeroSection } from "@/components/homepage/HeroSection";
import { SectionsRenderer } from "@/components/homepage/SectionsRenderer";

import { defaultGridDenseContent } from "./GridDenseLayout.defaults";
import { resolveLayoutContent, type LayoutProps } from "./shared";

/**
 * Grid Dense — tight pacing, product-forward. Hero is compact or absent.
 * Minimal vertical breathing room so maximum SKUs land above the fold.
 */
export function GridDenseLayout({ store, theme, content }: LayoutProps) {
  const resolved = resolveLayoutContent(content, defaultGridDenseContent(store));
  return (
    <article className="space-y-6 pb-4">
      <HeroSection hero={resolved.hero} theme={theme} fallbackHeading={store.name} />
      <SectionsRenderer sections={resolved.sections} theme={theme} storeSlug={store.slug} />
    </article>
  );
}
