"use client";

import { useState } from "react";
import { ArrowDown, ArrowUp, ChevronDown, Plus, Trash2 } from "lucide-react";
import { AlertDialog } from "@tesserix/web";

import type {
  AdminCategory,
  HomepageSection,
} from "@/lib/api/marketplace-api";
import { SectionHeader } from "./BrandingSettingsClient";
import { TextSectionForm } from "./sections/TextSectionForm";
import { ImageSectionForm } from "./sections/ImageSectionForm";
import { QuoteSectionForm } from "./sections/QuoteSectionForm";
import { FeaturedProductsSectionForm } from "./sections/FeaturedProductsSectionForm";
import { MarqueeSectionForm } from "./sections/MarqueeSectionForm";
import { PullQuoteSectionForm } from "./sections/PullQuoteSectionForm";
import { LetterSectionForm } from "./sections/LetterSectionForm";

const MAX_SECTIONS = 12;

type SectionType = HomepageSection["type"];

interface AddGroup {
  label: string;
  types: SectionType[];
}

const ADD_GROUPS: AddGroup[] = [
  { label: "Content", types: ["text", "image", "quote"] },
  { label: "Featured", types: ["featured_products"] },
  { label: "Editorial", types: ["marquee", "pull_quote", "letter"] },
];

interface HomepageSectionsEditorProps {
  sections: HomepageSection[];
  onChange: (next: HomepageSection[]) => void;
  categories: Pick<AdminCategory, "id" | "slug" | "name">[];
  editable: boolean;
  storeId: string;
}

function newSection(type: SectionType): HomepageSection {
  switch (type) {
    case "text":              return { type: "text", markdown: "" };
    case "image":             return { type: "image", url: "" };
    case "quote":             return { type: "quote", text: "" };
    case "featured_products": return { type: "featured_products", collection_slug: "", limit: 8 };
    case "marquee":           return { type: "marquee", items: [] };
    case "pull_quote":        return { type: "pull_quote", text: "" };
    case "letter":            return { type: "letter", title: "", body: "" };
  }
}

export function HomepageSectionsEditor({
  sections,
  onChange,
  categories,
  editable,
  storeId,
}: HomepageSectionsEditorProps) {
  const canAdd = sections.length < MAX_SECTIONS;
  const [addMenuOpen, setAddMenuOpen] = useState(false);
  const [pendingRemoveIndex, setPendingRemoveIndex] = useState<number | null>(null);

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
            key={sectionKey(s, i)}
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
                    onClick={() => setPendingRemoveIndex(i)}
                    aria-label="Delete section"
                  >
                    <Trash2 className="h-4 w-4" />
                  </button>
                </div>
              ) : null}
            </header>
            {renderForm(s, (next) => updateAt(i, next), categories, editable, storeId)}
          </article>
        ))}

        {sections.length === 0 ? (
          <p className="rounded-[var(--radius)] border border-dashed border-border px-6 py-10 text-center text-sm text-foreground-secondary">
            No sections yet. Pick a block type below to add your first one.
          </p>
        ) : null}
      </div>

      {editable ? (
        <div className="relative border-t border-border pt-4">
          <button
            type="button"
            onClick={() => setAddMenuOpen((o) => !o)}
            disabled={!canAdd}
            className="inline-flex h-9 items-center gap-1.5 rounded-md border border-border px-3 text-sm text-foreground hover:border-[color:var(--moss-700)] disabled:opacity-40"
          >
            <Plus className="h-3.5 w-3.5" />
            Add block
            <ChevronDown className="h-3.5 w-3.5" />
          </button>
          {addMenuOpen && canAdd ? (
            <div
              className="absolute z-10 mt-1 w-56 rounded-[var(--radius)] border border-border bg-[color:var(--background-elevated)] p-1 shadow-lg"
              role="menu"
            >
              {ADD_GROUPS.map((group) => (
                <div key={group.label} className="py-1">
                  <p className="px-2 pb-1 text-[10px] font-semibold uppercase tracking-wide text-foreground-secondary">
                    {group.label}
                  </p>
                  {group.types.map((t) => (
                    <button
                      key={t}
                      type="button"
                      onClick={() => {
                        addSection(t);
                        setAddMenuOpen(false);
                      }}
                      className="flex w-full items-center gap-2 rounded px-2 py-1.5 text-left text-sm text-foreground hover:bg-[color:var(--background)]"
                      role="menuitem"
                    >
                      <Plus className="h-3 w-3 text-foreground-secondary" />
                      {labelFor(t)}
                    </button>
                  ))}
                </div>
              ))}
            </div>
          ) : null}
        </div>
      ) : null}

      <AlertDialog
        isOpen={pendingRemoveIndex !== null}
        onClose={() => setPendingRemoveIndex(null)}
        title="Delete section?"
        message="This section will be removed from your homepage."
        type="confirm"
        confirmLabel="Delete"
        cancelLabel="Cancel"
        onConfirm={() => {
          if (pendingRemoveIndex !== null) removeAt(pendingRemoveIndex);
          setPendingRemoveIndex(null);
        }}
        onCancel={() => setPendingRemoveIndex(null)}
      />
    </div>
  );
}

// sectionKey returns a stable React key for a homepage section.
// Per-type content hash falls back to index for content-light types
// (text/quote have mutable bodies so we lean on index there).
// Good enough to prevent cursor jumps on non-reorder changes; reorders
// will still re-mount controlled inputs, but form state is lifted so
// values aren't lost. Duplicate content produces duplicate keys —
// acceptable trade-off vs. the complexity of a keyed-state layer.
function sectionKey(s: HomepageSection, i: number): string {
  switch (s.type) {
    case "image":             return `image-${s.url || i}`;
    case "featured_products": return `featured-${s.collection_slug || (s.product_slugs ?? []).join(",") || i}`;
    default:                  return `${s.type}-${i}`;
  }
}

function labelFor(t: SectionType): string {
  switch (t) {
    case "text":              return "Text";
    case "image":             return "Image";
    case "quote":             return "Quote";
    case "featured_products": return "Featured products";
    case "marquee":           return "Marquee";
    case "pull_quote":        return "Pull quote";
    case "letter":            return "Letter";
  }
}

function renderForm(
  section: HomepageSection,
  onChange: (next: HomepageSection) => void,
  categories: Pick<AdminCategory, "id" | "slug" | "name">[],
  editable: boolean,
  storeId: string,
) {
  switch (section.type) {
    case "text":
      return <TextSectionForm section={section} onChange={onChange} editable={editable} />;
    case "image":
      return <ImageSectionForm section={section} onChange={onChange} editable={editable} storeId={storeId} />;
    case "quote":
      return <QuoteSectionForm section={section} onChange={onChange} editable={editable} />;
    case "featured_products":
      return (
        <FeaturedProductsSectionForm
          section={section}
          onChange={onChange}
          categories={categories}
          editable={editable}
          storeId={storeId}
        />
      );
    case "marquee":
      return <MarqueeSectionForm section={section} onChange={onChange} editable={editable} />;
    case "pull_quote":
      return <PullQuoteSectionForm section={section} onChange={onChange} editable={editable} />;
    case "letter":
      return <LetterSectionForm section={section} onChange={onChange} editable={editable} />;
  }
}
