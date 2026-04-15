import { HeroSection } from "@/components/homepage/HeroSection";
import { SectionsRenderer } from "@/components/homepage/SectionsRenderer";

import { defaultMinimalContent } from "./MinimalLayout.defaults";
import { resolveLayoutContent, type LayoutProps } from "./shared";

export function MinimalLayout({ store, theme, content }: LayoutProps) {
  const resolved = resolveLayoutContent(content, defaultMinimalContent(store));
  return (
    <article className="mx-auto max-w-4xl space-y-20 py-12">
      <HeroSection hero={resolved.hero} theme={theme} fallbackHeading={store.name} />
      <SectionsRenderer sections={resolved.sections} theme={theme} storeSlug={store.slug} />
    </article>
  );
}
