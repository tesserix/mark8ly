"use client";

import { RotateCcw, Sparkles } from "lucide-react";

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
import { starterRecipeFor, type StoreInfo } from "./HomepageTab.helpers";

interface HomepageTabProps {
  form: StoreBranding;
  patch: (updates: Partial<StoreBranding>) => void;
  editable: boolean;
  pages: Pick<AdminPage, "id" | "slug" | "title">[];
  categories: Pick<AdminCategory, "id" | "slug" | "name">[];
  store: StoreInfo;
}

function defaultHero(): HomepageHero {
  return { enabled: true };
}

export function HomepageTab({
  form,
  patch,
  editable,
  pages,
  categories,
  store,
}: HomepageTabProps) {
  const content: HomepageContent = form.homepage_content ?? {};
  const hero: HomepageHero = content.hero ?? defaultHero();
  const sections: HomepageSection[] = content.sections ?? [];
  const isEmpty = sections.length === 0;

  const setContent = (next: HomepageContent) => {
    patch({ homepage_content: next });
  };

  const applyDefaults = () => {
    const recipe = starterRecipeFor(form.layout_variant, store);
    setContent(recipe);
  };

  const resetToDefaults = () => {
    if (
      !window.confirm(
        "Replace your homepage content with the theme defaults? This can't be undone once you save.",
      )
    ) {
      return;
    }
    applyDefaults();
  };

  return (
    <div className="space-y-10">
      <HeroEditor
        value={hero}
        onChange={(nextHero) => setContent({ ...content, hero: nextHero })}
        pages={pages}
        editable={editable}
        layoutVariant={form.layout_variant}
      />

      <div className="border-t border-border-subtle pt-10">
        {isEmpty ? (
          <div className="mb-6 flex flex-wrap items-center gap-3 rounded-[var(--radius)] border border-dashed border-border px-4 py-3 text-sm">
            <span className="rounded-full bg-[color:var(--moss-700)]/10 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-[color:var(--moss-700)]">
              Using theme defaults
            </span>
            <span className="text-foreground-secondary">
              Your storefront renders the {form.layout_variant} defaults until you add your own content.
            </span>
            {editable ? (
              <button
                type="button"
                onClick={applyDefaults}
                className="ml-auto inline-flex h-8 items-center gap-1.5 rounded-md border border-border bg-[color:var(--background-elevated)] px-3 text-xs font-medium text-foreground hover:border-[color:var(--moss-700)]"
              >
                <Sparkles className="h-3.5 w-3.5" />
                Start from theme defaults
              </button>
            ) : null}
          </div>
        ) : editable ? (
          <div className="mb-4 flex justify-end">
            <button
              type="button"
              onClick={resetToDefaults}
              className="inline-flex h-8 items-center gap-1.5 rounded-md border border-transparent px-3 text-xs font-medium text-foreground-secondary hover:border-border hover:text-foreground"
            >
              <RotateCcw className="h-3.5 w-3.5" />
              Reset to theme defaults
            </button>
          </div>
        ) : null}

        <HomepageSectionsEditor
          sections={sections}
          onChange={(nextSections) => setContent({ ...content, sections: nextSections })}
          categories={categories}
          editable={editable}
          storeId={form.store_id}
        />
      </div>
    </div>
  );
}
