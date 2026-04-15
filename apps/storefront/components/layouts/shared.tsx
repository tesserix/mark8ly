import type {
  HomepageContent,
  HomepageHero,
  HomepageSection,
} from "@/lib/api/marketplace-api";
import type { PublicStore } from "@/lib/api/platform-api";
import type { StorefrontTheme } from "@repo/ui/storefront-theme";

export interface LayoutProps {
  store: PublicStore;
  theme: StorefrontTheme;
  content?: HomepageContent | null;
}

export interface ResolvedLayoutContent {
  hero: HomepageHero | null;
  sections: HomepageSection[];
}

/** Resolves the hero + sections a layout should render.
 *
 *  Rules:
 *  - If the merchant authored any sections, use their sections verbatim
 *    AND their hero (or null if they opted out). The recipe does not
 *    fill gaps — authored content is all-or-nothing to avoid a
 *    merchant-authored page with surprise recipe content appearing.
 *  - If the merchant authored no sections, the recipe provides both
 *    hero and sections. An authored hero still wins over the recipe
 *    hero in this case so hero-only edits persist.
 */
export function resolveLayoutContent(
  content: HomepageContent | null | undefined,
  recipe: { hero?: HomepageHero; sections: HomepageSection[] },
): ResolvedLayoutContent {
  const authoredSections = content?.sections ?? [];
  const hasAuthored = authoredSections.length > 0;
  return {
    hero: content?.hero ?? (hasAuthored ? null : recipe.hero ?? null),
    sections: hasAuthored ? authoredSections : recipe.sections,
  };
}
