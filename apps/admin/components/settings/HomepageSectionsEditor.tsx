"use client";

import { ArrowDown, ArrowUp, Plus, Trash2 } from "lucide-react";

import type {
  AdminCategory,
  HomepageSection,
} from "@/lib/api/marketplace-api";
import { SectionHeader } from "./BrandingSettingsClient";
import { TextSectionForm } from "./sections/TextSectionForm";
import { ImageSectionForm } from "./sections/ImageSectionForm";
import { QuoteSectionForm } from "./sections/QuoteSectionForm";
import { FeaturedProductsSectionForm } from "./sections/FeaturedProductsSectionForm";

const MAX_SECTIONS = 12;

type SectionType = HomepageSection["type"];

interface HomepageSectionsEditorProps {
  sections: HomepageSection[];
  onChange: (next: HomepageSection[]) => void;
  categories: Pick<AdminCategory, "id" | "slug" | "name">[];
  editable: boolean;
}

function newSection(type: SectionType): HomepageSection {
  switch (type) {
    case "text":              return { type: "text", markdown: "" };
    case "image":             return { type: "image", url: "" };
    case "quote":             return { type: "quote", text: "" };
    case "featured_products": return { type: "featured_products", collection_slug: "", limit: 8 };
  }
}

export function HomepageSectionsEditor({
  sections,
  onChange,
  categories,
  editable,
}: HomepageSectionsEditorProps) {
  const canAdd = sections.length < MAX_SECTIONS;

  const updateAt = (i: number, next: HomepageSection) => {
    onChange(sections.map((s, idx) => (idx === i ? next : s)));
  };
  const removeAt = (i: number) => onChange(sections.filter((_, idx) => idx !== i));
  const moveAt = (i: number, dir: -1 | 1) => {
    const target = i + dir;
    if (target < 0 || target >= sections.length) return;
    const next = [...sections];
    const tmp = next[i] as HomepageSection;
    next[i] = next[target] as HomepageSection;
    next[target] = tmp;
    onChange(next);
  };
  const addSection = (type: SectionType) => {
    if (!canAdd) return;
    onChange([...sections, newSection(type)]);
  };

  return (
    <div className="space-y-6">
      <SectionHeader
        title="Homepage sections"
        description="Stack blocks of content below the hero. Each block renders on every theme — switch layouts without losing content."
      />

      <div className="space-y-4">
        {sections.map((s, i) => (
          <article
            key={i}
            className="space-y-4 rounded-[var(--radius)] border border-border bg-[color:var(--background-elevated)] p-5"
          >
            <header className="flex items-center justify-between">
              <p className="text-xs font-medium uppercase tracking-wide text-foreground-secondary">
                {labelFor(s.type)} · #{i + 1}
              </p>
              {editable ? (
                <div className="flex items-center gap-1">
                  <button
                    type="button"
                    className="inline-flex h-8 w-8 items-center justify-center rounded-md text-foreground-secondary hover:bg-[color:var(--background)] hover:text-foreground disabled:opacity-30"
                    onClick={() => moveAt(i, -1)}
                    disabled={i === 0}
                    aria-label="Move section up"
                  >
                    <ArrowUp className="h-4 w-4" />
                  </button>
                  <button
                    type="button"
                    className="inline-flex h-8 w-8 items-center justify-center rounded-md text-foreground-secondary hover:bg-[color:var(--background)] hover:text-foreground disabled:opacity-30"
                    onClick={() => moveAt(i, 1)}
                    disabled={i === sections.length - 1}
                    aria-label="Move section down"
                  >
                    <ArrowDown className="h-4 w-4" />
                  </button>
                  <button
                    type="button"
                    className="inline-flex h-8 w-8 items-center justify-center rounded-md text-foreground-secondary hover:bg-danger/10 hover:text-danger"
                    onClick={() => {
                      if (window.confirm("Delete this section?")) removeAt(i);
                    }}
                    aria-label="Delete section"
                  >
                    <Trash2 className="h-4 w-4" />
                  </button>
                </div>
              ) : null}
            </header>
            {renderForm(s, (next) => updateAt(i, next), categories, editable)}
          </article>
        ))}

        {sections.length === 0 ? (
          <p className="rounded-[var(--radius)] border border-dashed border-border px-6 py-10 text-center text-sm text-foreground-secondary">
            No sections yet. Pick a block type below to add your first one.
          </p>
        ) : null}
      </div>

      {editable ? (
        <div className="flex flex-wrap items-center gap-2 border-t border-border pt-4">
          <span className="text-xs uppercase tracking-wide text-foreground-secondary">Add block</span>
          {(["text", "image", "quote", "featured_products"] as SectionType[]).map((t) => (
            <button
              key={t}
              type="button"
              onClick={() => addSection(t)}
              disabled={!canAdd}
              className="inline-flex h-9 items-center gap-1 rounded-md border border-border px-3 text-sm text-foreground hover:border-[color:var(--moss-700)] disabled:opacity-40"
            >
              <Plus className="h-3.5 w-3.5" />
              {labelFor(t)}
            </button>
          ))}
        </div>
      ) : null}
    </div>
  );
}

function labelFor(t: SectionType): string {
  switch (t) {
    case "text":              return "Text";
    case "image":             return "Image";
    case "quote":             return "Quote";
    case "featured_products": return "Featured products";
  }
}

function renderForm(
  section: HomepageSection,
  onChange: (next: HomepageSection) => void,
  categories: Pick<AdminCategory, "id" | "slug" | "name">[],
  editable: boolean,
) {
  switch (section.type) {
    case "text":
      return <TextSectionForm section={section} onChange={onChange} editable={editable} />;
    case "image":
      return <ImageSectionForm section={section} onChange={onChange} editable={editable} />;
    case "quote":
      return <QuoteSectionForm section={section} onChange={onChange} editable={editable} />;
    case "featured_products":
      return (
        <FeaturedProductsSectionForm
          section={section}
          onChange={onChange}
          categories={categories}
          editable={editable}
        />
      );
  }
}
