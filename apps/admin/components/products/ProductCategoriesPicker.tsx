"use client";

// apps/admin/components/products/ProductCategoriesPicker.tsx
//
// Client component: chip row + searchable dropdown for category selection.
// Supports inline category creation: when the search query doesn't match
// any existing category, a "+ Create [name]" button is shown.

import { useMemo, useState, useTransition } from "react";
import { X, Plus } from "lucide-react";

import type { AdminCategory } from "@/lib/api/marketplace-api";
import { createCategoryInline } from "@/app/(admin)/products/category-actions";

export interface ProductCategoriesPickerProps {
  allCategories: AdminCategory[];
  selectedIds: string[];
  onChange: (ids: string[]) => void;
  disabled?: boolean;
  storeId?: string;
}

export function ProductCategoriesPicker({
  allCategories,
  selectedIds,
  onChange,
  disabled,
  storeId,
}: ProductCategoriesPickerProps) {
  const [query, setQuery] = useState("");
  const [isOpen, setIsOpen] = useState(false);
  const [creating, startCreating] = useTransition();
  const [localCategories, setLocalCategories] = useState<AdminCategory[]>([]);

  const allCats = useMemo(
    () => [...allCategories, ...localCategories],
    [allCategories, localCategories],
  );

  const selected = useMemo(
    () =>
      selectedIds
        .map((id) => allCats.find((c) => c.id === id))
        .filter((c): c is AdminCategory => !!c),
    [selectedIds, allCats],
  );

  const available = useMemo(() => {
    const q = query.trim().toLowerCase();
    return allCats
      .filter((c) => c.is_active || selectedIds.includes(c.id))
      .filter((c) => !selectedIds.includes(c.id))
      .filter((c) => (q ? c.name.toLowerCase().includes(q) : true))
      .slice(0, 50);
  }, [allCats, selectedIds, query]);

  const canCreate =
    !disabled &&
    storeId &&
    query.trim().length > 0 &&
    !allCats.some(
      (c) => c.name.toLowerCase() === query.trim().toLowerCase(),
    );

  function handleCreate() {
    if (!storeId || !query.trim()) return;
    const name = query.trim();
    startCreating(async () => {
      const result = await createCategoryInline(storeId, name);
      if (result.ok && result.category) {
        const newCat: AdminCategory = {
          id: result.category.id,
          store_id: storeId,
          parent_id: null,
          name: result.category.name,
          slug: result.category.slug,
          description: null,
          image_url: null,
          position: 0,
          is_active: true,
        };
        setLocalCategories((prev) => [...prev, newCat]);
        onChange([...selectedIds, newCat.id]);
        setQuery("");
        setIsOpen(false);
      }
    });
  }

  const add = (id: string) => {
    onChange([...selectedIds, id]);
    setQuery("");
    setIsOpen(false);
  };
  const remove = (id: string) => {
    onChange(selectedIds.filter((x) => x !== id));
  };

  return (
    <div className="flex flex-col gap-3">
      {selected.length > 0 && (
        <div className="flex flex-wrap gap-2">
          {selected.map((cat) => (
            <span
              key={cat.id}
              className="inline-flex items-center gap-2 rounded-full border border-[color:var(--ink-900)] border-opacity-20 bg-[color:var(--background-elevated,white)] px-3 py-1 text-sm text-[color:var(--ink-900)]"
            >
              {cat.name}
              {!disabled && (
                <button
                  type="button"
                  onClick={() => remove(cat.id)}
                  aria-label={`Remove ${cat.name}`}
                  className="inline-flex h-4 w-4 items-center justify-center rounded-full opacity-60 hover:opacity-100 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
                >
                  <X className="h-3 w-3" aria-hidden="true" />
                </button>
              )}
            </span>
          ))}
        </div>
      )}

      {!disabled && (
        <div className="relative">
          <label className="relative block">
            <Plus
              className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-[color:var(--ink-900)] opacity-50"
              aria-hidden="true"
            />
            <input
              type="text"
              value={query}
              placeholder="Add category…"
              aria-label="Add category"
              onChange={(e) => {
                setQuery(e.target.value);
                setIsOpen(true);
              }}
              onFocus={() => setIsOpen(true)}
              onBlur={() => setTimeout(() => setIsOpen(false), 120)}
              className="w-full rounded-md border border-[color:var(--ink-900)] border-opacity-20 bg-[color:var(--background-elevated,white)] py-2 pl-10 pr-3 text-sm text-[color:var(--ink-900)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
            />
          </label>
          {isOpen && available.length > 0 && (
            <ul
              role="listbox"
              className="absolute left-0 right-0 top-full z-10 mt-1 max-h-60 overflow-y-auto rounded-md border border-[color:var(--ink-900)] border-opacity-10 bg-[color:var(--background-elevated,white)] shadow-sm"
            >
              {available.map((cat) => (
                <li key={cat.id} role="option" aria-selected="false">
                  <button
                    type="button"
                    onMouseDown={(e) => {
                      e.preventDefault();
                      add(cat.id);
                    }}
                    className="flex w-full items-center justify-between px-3 py-2 text-left text-sm text-[color:var(--ink-900)] hover:bg-[color:var(--ink-900)]/5 focus-visible:bg-[color:var(--ink-900)] focus-visible:bg-opacity-5 focus-visible:outline-none"
                  >
                    <span>{cat.name}</span>
                    <span className="text-xs opacity-50">/{cat.slug}</span>
                  </button>
                </li>
              ))}
            </ul>
          )}
          {isOpen && available.length === 0 && query.trim() && (
            <div className="absolute left-0 right-0 top-full z-10 mt-1 rounded-md border border-[color:var(--ink-900)] border-opacity-10 bg-[color:var(--background-elevated,white)] shadow-sm">
              {canCreate ? (
                <button
                  type="button"
                  disabled={creating}
                  onMouseDown={(e) => {
                    e.preventDefault();
                    handleCreate();
                  }}
                  className="flex w-full items-center gap-2 px-3 py-2 text-left text-sm text-[color:var(--moss-700)] hover:bg-[color:var(--ink-900)]/5 focus-visible:bg-[color:var(--ink-900)] focus-visible:bg-opacity-5 focus-visible:outline-none"
                >
                  <Plus className="h-3 w-3" aria-hidden="true" />
                  {creating
                    ? "Creating..."
                    : `Create "${query.trim()}"`}
                </button>
              ) : (
                <p className="px-3 py-2 text-sm text-[color:var(--ink-900)] opacity-60">
                  No matching categories.
                </p>
              )}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
