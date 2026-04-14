"use client";

import { ArrowDown, ArrowUp, Plus, Trash2 } from "lucide-react";

import type {
  AdminPage,
  FooterSection,
  FooterLinkItem,
} from "@/lib/api/marketplace-api";

const MAX_SECTIONS = 6;
const MAX_ITEMS_PER_SECTION = 10;

interface FooterSectionsEditorProps {
  sections: FooterSection[];
  onChange: (next: FooterSection[]) => void;
  pages: Pick<AdminPage, "id" | "slug" | "title">[];
  editable: boolean;
}

export function FooterSectionsEditor({
  sections,
  onChange,
  pages,
  editable,
}: FooterSectionsEditorProps) {
  const canAddSection = sections.length < MAX_SECTIONS;

  const updateSection = (idx: number, patch: Partial<FooterSection>) => {
    onChange(sections.map((s, i) => (i === idx ? { ...s, ...patch } : s)));
  };

  const addSection = () => {
    if (!canAddSection || !editable) return;
    onChange([...sections, { label: "New section", items: [] }]);
  };

  const removeSection = (idx: number) => {
    if (!editable) return;
    onChange(sections.filter((_, i) => i !== idx));
  };

  const moveSection = (idx: number, direction: -1 | 1) => {
    if (!editable) return;
    const target = idx + direction;
    if (target < 0 || target >= sections.length) return;
    const next = [...sections] as FooterSection[];
    const a = next[idx] as FooterSection;
    const b = next[target] as FooterSection;
    next[idx] = b;
    next[target] = a;
    onChange(next);
  };

  const updateItem = (
    secIdx: number,
    itemIdx: number,
    patch: Partial<FooterLinkItem>,
  ) => {
    const sec = sections[secIdx];
    if (!sec) return;
    updateSection(secIdx, {
      items: sec.items.map((it, i) =>
        i === itemIdx ? { ...it, ...patch } : it,
      ),
    });
  };

  const addItem = (secIdx: number) => {
    if (!editable) return;
    const section = sections[secIdx];
    if (!section) return;
    if (section.items.length >= MAX_ITEMS_PER_SECTION) return;
    updateSection(secIdx, {
      items: [
        ...section.items,
        { label: "New link", kind: "page", page_slug: pages[0]?.slug ?? "" },
      ],
    });
  };

  const removeItem = (secIdx: number, itemIdx: number) => {
    if (!editable) return;
    const sec = sections[secIdx];
    if (!sec) return;
    updateSection(secIdx, {
      items: sec.items.filter((_, i) => i !== itemIdx),
    });
  };

  const moveItem = (secIdx: number, itemIdx: number, direction: -1 | 1) => {
    if (!editable) return;
    const sec = sections[secIdx];
    if (!sec) return;
    const target = itemIdx + direction;
    const items = sec.items;
    if (target < 0 || target >= items.length) return;
    const next = [...items] as FooterLinkItem[];
    const a = next[itemIdx] as FooterLinkItem;
    const b = next[target] as FooterLinkItem;
    next[itemIdx] = b;
    next[target] = a;
    updateSection(secIdx, { items: next });
  };

  return (
    <div className="space-y-0">
      {sections.map((section, secIdx) => (
        <div key={secIdx}>
          {secIdx > 0 && (
            <div className="border-t border-border-subtle my-4" />
          )}

          {/* Section header row */}
          <div className="flex items-center gap-2 mb-3">
            <input
              type="text"
              value={section.label}
              onChange={(e) =>
                updateSection(secIdx, { label: e.target.value })
              }
              disabled={!editable}
              placeholder="Section label"
              className="h-9 flex-1 rounded-[var(--radius)] border border-border bg-[color:var(--background-elevated)] px-3 text-sm font-medium text-foreground placeholder:text-foreground-tertiary focus:border-[color:var(--moss-700)] focus:outline-none focus:ring-1 focus:ring-[color:var(--moss-700)] disabled:opacity-50"
            />
            <div className="flex items-center gap-1">
              <button
                type="button"
                onClick={() => moveSection(secIdx, -1)}
                disabled={!editable || secIdx === 0}
                className="flex h-8 w-8 items-center justify-center rounded-[var(--radius)] border border-border bg-[color:var(--background-elevated)] text-foreground-secondary transition-colors hover:text-foreground disabled:opacity-30"
                aria-label="Move section up"
              >
                <ArrowUp className="h-3.5 w-3.5" />
              </button>
              <button
                type="button"
                onClick={() => moveSection(secIdx, 1)}
                disabled={!editable || secIdx === sections.length - 1}
                className="flex h-8 w-8 items-center justify-center rounded-[var(--radius)] border border-border bg-[color:var(--background-elevated)] text-foreground-secondary transition-colors hover:text-foreground disabled:opacity-30"
                aria-label="Move section down"
              >
                <ArrowDown className="h-3.5 w-3.5" />
              </button>
              <button
                type="button"
                onClick={() => removeSection(secIdx)}
                disabled={!editable}
                className="flex h-8 w-8 items-center justify-center rounded-[var(--radius)] border border-border bg-[color:var(--background-elevated)] text-foreground-secondary transition-colors hover:text-[color:var(--danger)] disabled:opacity-30"
                aria-label="Remove section"
              >
                <Trash2 className="h-3.5 w-3.5" />
              </button>
            </div>
          </div>

          {/* Items list */}
          <div className="space-y-2 pl-3 border-l-2 border-border-subtle ml-1">
            {section.items.map((item, itemIdx) => (
              <div key={itemIdx} className="flex items-center gap-2">
                {/* Label */}
                <input
                  type="text"
                  value={item.label}
                  onChange={(e) =>
                    updateItem(secIdx, itemIdx, { label: e.target.value })
                  }
                  disabled={!editable}
                  placeholder="Link label"
                  className="h-9 w-0 flex-[2] rounded-[var(--radius)] border border-border bg-[color:var(--background-elevated)] px-3 text-sm text-foreground placeholder:text-foreground-tertiary focus:border-[color:var(--moss-700)] focus:outline-none focus:ring-1 focus:ring-[color:var(--moss-700)] disabled:opacity-50"
                />

                {/* Kind select */}
                <select
                  value={item.kind}
                  onChange={(e) =>
                    updateItem(secIdx, itemIdx, {
                      kind: e.target.value as "page" | "url",
                      page_slug: e.target.value === "page" ? (pages[0]?.slug ?? "") : undefined,
                      url: e.target.value === "url" ? "" : undefined,
                    })
                  }
                  disabled={!editable}
                  className="h-9 flex-[1] rounded-[var(--radius)] border border-border bg-[color:var(--background-elevated)] px-2 text-sm text-foreground focus:border-[color:var(--moss-700)] focus:outline-none focus:ring-1 focus:ring-[color:var(--moss-700)] disabled:opacity-50"
                >
                  <option value="page">Page</option>
                  <option value="url">URL</option>
                </select>

                {/* Value: page select or URL input */}
                {item.kind === "page" ? (
                  pages.length > 0 ? (
                    <select
                      value={item.page_slug ?? ""}
                      onChange={(e) =>
                        updateItem(secIdx, itemIdx, { page_slug: e.target.value })
                      }
                      disabled={!editable}
                      className="h-9 w-0 flex-[2] rounded-[var(--radius)] border border-border bg-[color:var(--background-elevated)] px-2 text-sm text-foreground focus:border-[color:var(--moss-700)] focus:outline-none focus:ring-1 focus:ring-[color:var(--moss-700)] disabled:opacity-50"
                    >
                      {pages.map((p) => (
                        <option key={p.id} value={p.slug}>
                          {p.title}
                        </option>
                      ))}
                    </select>
                  ) : (
                    <p className="w-0 flex-[2] px-2 text-xs text-[color:var(--signal)]">
                      No pages yet — create one in Settings → Pages
                    </p>
                  )
                ) : (
                  <input
                    type="text"
                    value={item.url ?? ""}
                    onChange={(e) =>
                      updateItem(secIdx, itemIdx, { url: e.target.value })
                    }
                    disabled={!editable}
                    placeholder="https://…"
                    className="h-9 w-0 flex-[2] rounded-[var(--radius)] border border-border bg-[color:var(--background-elevated)] px-3 text-sm text-foreground placeholder:text-foreground-tertiary focus:border-[color:var(--moss-700)] focus:outline-none focus:ring-1 focus:ring-[color:var(--moss-700)] disabled:opacity-50"
                  />
                )}

                {/* Reorder + delete */}
                <div className="flex items-center gap-1">
                  <button
                    type="button"
                    onClick={() => moveItem(secIdx, itemIdx, -1)}
                    disabled={!editable || itemIdx === 0}
                    className="flex h-7 w-7 items-center justify-center rounded-[var(--radius)] text-foreground-secondary transition-colors hover:text-foreground disabled:opacity-30"
                    aria-label="Move link up"
                  >
                    <ArrowUp className="h-3 w-3" />
                  </button>
                  <button
                    type="button"
                    onClick={() => moveItem(secIdx, itemIdx, 1)}
                    disabled={!editable || itemIdx === section.items.length - 1}
                    className="flex h-7 w-7 items-center justify-center rounded-[var(--radius)] text-foreground-secondary transition-colors hover:text-foreground disabled:opacity-30"
                    aria-label="Move link down"
                  >
                    <ArrowDown className="h-3 w-3" />
                  </button>
                  <button
                    type="button"
                    onClick={() => removeItem(secIdx, itemIdx)}
                    disabled={!editable}
                    className="flex h-7 w-7 items-center justify-center rounded-[var(--radius)] text-foreground-secondary transition-colors hover:text-[color:var(--danger)] disabled:opacity-30"
                    aria-label="Remove link"
                  >
                    <Trash2 className="h-3 w-3" />
                  </button>
                </div>
              </div>
            ))}

            {/* Add link */}
            <button
              type="button"
              onClick={() => addItem(secIdx)}
              disabled={!editable || section.items.length >= MAX_ITEMS_PER_SECTION}
              className="mt-1 flex items-center gap-1.5 text-xs text-[color:var(--moss-700)] transition-colors hover:text-[color:var(--moss-700)]/80 disabled:opacity-40"
            >
              <Plus className="h-3 w-3" />
              Add link
            </button>
          </div>
        </div>
      ))}

      {/* Add section */}
      <div className={sections.length > 0 ? "border-t border-border-subtle pt-4 mt-4" : ""}>
        <button
          type="button"
          onClick={addSection}
          disabled={!editable || !canAddSection}
          className="flex items-center gap-2 rounded-[var(--radius)] border border-dashed border-border px-4 py-2 text-sm text-foreground-secondary transition-colors hover:border-[color:var(--moss-700)] hover:text-[color:var(--moss-700)] disabled:opacity-40"
        >
          <Plus className="h-4 w-4" />
          Add section
          {!canAddSection && (
            <span className="text-xs text-foreground-tertiary">
              (max {MAX_SECTIONS})
            </span>
          )}
        </button>
      </div>
    </div>
  );
}
