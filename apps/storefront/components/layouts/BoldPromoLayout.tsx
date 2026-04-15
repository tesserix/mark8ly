import { HeroSection } from "@/components/homepage/HeroSection";
import { SectionsRenderer } from "@/components/homepage/SectionsRenderer";

import { defaultBoldPromoContent } from "./BoldPromoLayout.defaults";
import { resolveLayoutContent, type LayoutProps } from "./shared";

export function BoldPromoLayout({ store, theme, content }: LayoutProps) {
  const resolved = resolveLayoutContent(content, defaultBoldPromoContent(store));
  return (
    <article className="space-y-16">
      <HeroSection hero={resolved.hero} theme={theme} fallbackHeading={store.name} />
      <SectionsRenderer sections={resolved.sections} theme={theme} storeSlug={store.slug} />
    </article>
  );
}
