import { HeroSection } from "@/components/homepage/HeroSection";
import { SectionsRenderer } from "@/components/homepage/SectionsRenderer";

import { defaultLandingStoryContent } from "./LandingStoryLayout.defaults";
import { resolveLayoutContent, type LayoutProps } from "./shared";

/**
 * Landing Story — long-scroll narrative landing. Hero, then modular
 * alternating text/image/quote/letter sections with a lean product block
 * near the end. Suits brand launches, kickstarter-like campaigns, and
 * story-heavy merchants (craft beer, indie electronics, heritage brands).
 */
export function LandingStoryLayout({ store, theme, content }: LayoutProps) {
  const resolved = resolveLayoutContent(content, defaultLandingStoryContent(store));
  return (
    <article className="mx-auto max-w-5xl space-y-28 px-4 py-12 sm:px-6">
      <HeroSection hero={resolved.hero} theme={theme} fallbackHeading={store.name} />
      <SectionsRenderer sections={resolved.sections} theme={theme} storeSlug={store.slug} />
    </article>
  );
}
