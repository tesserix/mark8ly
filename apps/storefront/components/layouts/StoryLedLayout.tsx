import { HeroSection } from "@/components/homepage/HeroSection";
import { SectionsRenderer } from "@/components/homepage/SectionsRenderer";

import { defaultStoryLedContent } from "./StoryLedLayout.defaults";
import { resolveLayoutContent, type LayoutProps } from "./shared";

export function StoryLedLayout({ store, theme, content }: LayoutProps) {
  const resolved = resolveLayoutContent(content, defaultStoryLedContent(store));
  return (
    <article className="mx-auto max-w-4xl space-y-16">
      <HeroSection hero={resolved.hero} theme={theme} fallbackHeading={store.name} />
      <SectionsRenderer sections={resolved.sections} theme={theme} storeSlug={store.slug} />
    </article>
  );
}
