import { HeroSection } from "@/components/homepage/HeroSection";
import { SectionsRenderer } from "@/components/homepage/SectionsRenderer";

import { defaultEditorialContent } from "./EditorialLayout.defaults";
import { resolveLayoutContent, type LayoutProps } from "./shared";

export function EditorialLayout({ store, theme, content }: LayoutProps) {
  const resolved = resolveLayoutContent(content, defaultEditorialContent(store));
  return (
    <article className="space-y-20">
      <HeroSection hero={resolved.hero} theme={theme} fallbackHeading={store.name} />
      <SectionsRenderer sections={resolved.sections} theme={theme} storeSlug={store.slug} />
    </article>
  );
}
