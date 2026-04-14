"use client";

import type {
  AdminCategory,
  AdminPage,
  HomepageContent,
  HomepageHero,
  HomepageSection,
  StoreBranding,
} from "@/lib/api/marketplace-api";

import { HeroEditor } from "./HeroEditor";
import { HomepageSectionsEditor } from "./HomepageSectionsEditor";

interface HomepageTabProps {
  form: StoreBranding;
  patch: (updates: Partial<StoreBranding>) => void;
  editable: boolean;
  pages: Pick<AdminPage, "id" | "slug" | "title">[];
  categories: Pick<AdminCategory, "id" | "slug" | "name">[];
}

const DEFAULT_HERO: HomepageHero = { enabled: true };

export function HomepageTab({ form, patch, editable, pages, categories }: HomepageTabProps) {
  const content: HomepageContent = form.homepage_content ?? {};
  const hero: HomepageHero = content.hero ?? DEFAULT_HERO;
  const sections: HomepageSection[] = content.sections ?? [];

  const setContent = (next: HomepageContent) => {
    patch({ homepage_content: next });
  };

  return (
    <div className="space-y-10">
      <HeroEditor
        value={hero}
        onChange={(nextHero) => setContent({ ...content, hero: nextHero })}
        pages={pages}
        editable={editable}
      />

      <div className="border-t border-border-subtle pt-10">
        <HomepageSectionsEditor
          sections={sections}
          onChange={(nextSections) => setContent({ ...content, sections: nextSections })}
          categories={categories}
          editable={editable}
        />
      </div>
    </div>
  );
}
