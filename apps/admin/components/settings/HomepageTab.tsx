"use client";

import { useState } from "react";
import { RotateCcw, Sparkles } from "lucide-react";
import { AlertDialog } from "@tesserix/web";

import type {
  AdminCategory,
  AdminPage,
  HomepageContent,
  HomepageHero,
  HomepageSection,
  StoreBranding,
} from "@/lib/api/marketplace-api";

import { FieldLabel, TextInput, ToggleSwitch, ColorField } from "./BrandingSettingsClient";
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
  // Bubbles hero a11y validity to parent so the global Save can be gated.
  onHeroValidityChange?: (valid: boolean) => void;
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
  onHeroValidityChange,
}: HomepageTabProps) {
  const content: HomepageContent = form.homepage_content ?? {};
  const hero: HomepageHero = content.hero ?? defaultHero();
  const sections: HomepageSection[] = content.sections ?? [];
  const isEmpty = sections.length === 0;
  const [resetOpen, setResetOpen] = useState(false);

  const setContent = (next: HomepageContent) => {
    patch({ homepage_content: next });
  };

  const applyDefaults = () => {
    const recipe = starterRecipeFor(form.layout_variant, store);
    setContent(recipe);
  };

  const resetToDefaults = () => {
    setResetOpen(true);
  };

  const confirmReset = () => {
    setResetOpen(false);
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
        storeId={form.store_id}
        onValidityChange={onHeroValidityChange}
      />

      {/* ── Announcement bar ──────────────────────────────────────
          Moved here from the (removed) Layout tab. Displays the
          announcement strip above the storefront nav. */}
      <div className="space-y-4 border-t border-border-subtle pt-10">
        <div className="flex items-center justify-between">
          <div className="space-y-0.5">
            <p className="text-sm font-medium text-foreground">Announcement bar</p>
            <p className="text-xs text-foreground-secondary">
              Display a promotional message at the top of your storefront.
            </p>
          </div>
          <ToggleSwitch
            checked={form.announcement_active}
            onChange={(v) => editable && patch({ announcement_active: v })}
            disabled={!editable}
          />
        </div>
        {form.announcement_active && (
          <div className="space-y-4">
            <div className="space-y-1.5">
              <FieldLabel htmlFor="announcement_text">Message</FieldLabel>
              <TextInput
                id="announcement_text"
                value={form.announcement_text ?? ""}
                onChange={(v) => patch({ announcement_text: v || null })}
                placeholder="Free shipping on orders over ₹999"
                disabled={!editable}
                maxLength={300}
              />
            </div>
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="space-y-1.5">
                <FieldLabel htmlFor="announcement_link">Link (optional)</FieldLabel>
                <TextInput
                  id="announcement_link"
                  value={form.announcement_link ?? ""}
                  onChange={(v) => patch({ announcement_link: v || null })}
                  placeholder="/pages/shipping-policy"
                  disabled={!editable}
                />
              </div>
              <ColorField
                id="announcement_bg"
                label="Bar color"
                value={form.announcement_bg ?? form.color_accent}
                onChange={(v) => patch({ announcement_bg: v })}
                disabled={!editable}
              />
            </div>
          </div>
        )}
      </div>

      <div className="border-t border-border-subtle pt-10">
        {isEmpty ? (
          <div className="mb-6 flex flex-wrap items-center gap-3 rounded-md border border-dashed border-border px-4 py-3 text-sm">
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

      <AlertDialog
        isOpen={resetOpen}
        onClose={() => setResetOpen(false)}
        title="Reset to theme defaults?"
        message="Your current homepage content will be replaced. You can still undo by reloading before saving."
        type="confirm"
        confirmLabel="Reset"
        cancelLabel="Keep my content"
        onConfirm={confirmReset}
        onCancel={() => setResetOpen(false)}
      />
    </div>
  );
}
