import { HeroSection } from "@/components/homepage/HeroSection";
import { SectionsRenderer } from "@/components/homepage/SectionsRenderer";

import { defaultLookbookContent } from "./LookbookLayout.defaults";
import { resolveLayoutContent, type LayoutProps } from "./shared";

export function LookbookLayout({ store, theme, content }: LayoutProps) {
  const resolved = resolveLayoutContent(content, defaultLookbookContent(store));
  return (
    <article className="space-y-24 pb-8">
      <HeroSection hero={resolved.hero} theme={theme} fallbackHeading={store.name} />
      <SectionsRenderer sections={resolved.sections} theme={theme} storeSlug={store.slug} />
    </article>
  );
}
