"use client";

// apps/admin/components/products/CategoriesFeaturedManager.tsx
//
// Minimal admin surface for renaming, deleting and featuring categories.
// Featured categories surface on the storefront's /products filter
// grid; the rest remain reachable via the full /categories browse page.
// Per-row optimistic toggle with rollback on failure.

import * as React from "react";
import { useTransition } from "react";
import { Pencil, Star, Trash2 } from "lucide-react";

import type { AdminCategory } from "@/lib/api/marketplace-api";
import { deleteCategory, renameCategory, setCategoryFeatured } from "@/app/(admin)/products/category-actions";
import { useToast } from "@/components/feedback/Toaster";

export interface CategoriesFeaturedManagerProps {
  storeId: string;
  categories: AdminCategory[];
}

export function CategoriesFeaturedManager({
  storeId,
  categories,
}: CategoriesFeaturedManagerProps): React.ReactElement {
  const { toast } = useToast();
  const [rows, setRows] = React.useState<AdminCategory[]>(categories);
  const [query, setQuery] = React.useState("");
  const [pending, startTransition] = useTransition();
  const [pendingId, setPendingId] = React.useState<string | null>(null);

  const [editingId, setEditingId] = React.useState<string | null>(null);
  const [draftName, setDraftName] = React.useState("");
  const [confirmingId, setConfirmingId] = React.useState<string | null>(null);

  const featuredCount = rows.filter((c) => c.featured).length;

  const startRename = (cat: AdminCategory): void => {
    setEditingId(cat.id);
    setDraftName(cat.name);
  };

  const saveRename = (cat: AdminCategory): void => {
    const next = draftName.trim();
    setEditingId(null);
    if (!next || next === cat.name) return;
    setRows((prev) => prev.map((c) => (c.id === cat.id ? { ...c, name: next } : c)));
    setPendingId(cat.id);
    startTransition(async () => {
      const result = await renameCategory(storeId, cat.id, next);
      setPendingId(null);
      if (!result.ok) {
        setRows((prev) => prev.map((c) => (c.id === cat.id ? { ...c, name: cat.name } : c)));
        toast.error("Couldn't rename", result.error ?? "Try again in a moment.");
        return;
      }
      toast.success("Category renamed", next);
    });
  };

  const remove = (cat: AdminCategory): void => {
    setConfirmingId(null);
    setPendingId(cat.id);
    startTransition(async () => {
      const result = await deleteCategory(storeId, cat.id);
      setPendingId(null);
      if (!result.ok) {
        toast.error("Couldn't delete", result.error ?? "Try again in a moment.");
        return;
      }
      setRows((prev) => prev.filter((c) => c.id !== cat.id));
      toast.success("Category deleted", cat.name);
    });
  };

  const filtered = React.useMemo(() => {
    const q = query.trim().toLowerCase();
    if (q === "") return rows;
    return rows.filter(
      (c) =>
        c.name.toLowerCase().includes(q) ||
        c.slug.toLowerCase().includes(q),
    );
  }, [rows, query]);

  const toggle = (cat: AdminCategory): void => {
    const next = !cat.featured;
    // Optimistic: flip locally first
    setRows((prev) =>
      prev.map((c) => (c.id === cat.id ? { ...c, featured: next } : c)),
    );
    setPendingId(cat.id);
    startTransition(async () => {
      const result = await setCategoryFeatured(storeId, cat.id, next);
      setPendingId(null);
      if (!result.ok) {
        // Rollback
        setRows((prev) =>
          prev.map((c) =>
            c.id === cat.id ? { ...c, featured: !next } : c,
          ),
        );
        toast.error(
          "Couldn't update",
          result.error ?? "Try again in a moment.",
        );
        return;
      }
      toast.success(
        next ? "Category featured" : "Removed from featured",
        cat.name,
      );
    });
  };

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
        <input
          type="search"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Search categories…"
          aria-label="Search categories"
          className="h-10 w-full rounded-md border border-[color:var(--ink-900)]/20 bg-[color:var(--background-elevated)] px-3 text-sm text-foreground placeholder:text-foreground-tertiary transition-colors focus:border-[color:var(--moss-700)] focus:outline-none focus:ring-1 focus:ring-[color:var(--moss-700)] sm:max-w-sm"
        />
        <p className="text-xs uppercase tracking-widest text-foreground-tertiary">
          {featuredCount} featured · {rows.length} total
        </p>
      </div>

      {filtered.length === 0 ? (
        <p className="py-6 text-sm text-foreground-secondary">
          {query.length > 0
            ? "No categories match your search."
            : "No categories yet. Create one from a product's category picker."}
        </p>
      ) : (
        <table className="w-full border-collapse text-left">
          <thead>
            <tr className="border-b border-border-subtle text-xs uppercase tracking-widest text-foreground-tertiary">
              <th className="pl-4 pr-3 py-2 font-normal">Name</th>
              <th className="px-3 py-2 font-normal">Slug</th>
              <th className="px-3 pr-4 py-2 text-right font-normal">
                Featured
              </th>
            </tr>
          </thead>
          <tbody>
            {filtered.map((cat) => {
              const isPending = pending && pendingId === cat.id;
              return (
                <tr
                  key={cat.id}
                  className="border-b border-border-subtle last:border-b-0 transition-colors hover:bg-[color:var(--ink-900)]/[0.02]"
                >
                  <td className="pl-4 pr-3 py-3 text-sm text-foreground">
                    {editingId === cat.id ? (
                      <input
                        autoFocus
                        value={draftName}
                        onChange={(e) => setDraftName(e.target.value)}
                        onBlur={() => saveRename(cat)}
                        onKeyDown={(e) => {
                          if (e.key === "Enter") saveRename(cat);
                          if (e.key === "Escape") setEditingId(null);
                        }}
                        aria-label={`New name for ${cat.name}`}
                        className="h-8 w-full max-w-xs rounded-md border border-[color:var(--ink-900)]/20 bg-[color:var(--background-elevated)] px-2 text-sm text-foreground focus:border-[color:var(--moss-700)] focus:outline-none focus:ring-1 focus:ring-[color:var(--moss-700)]"
                      />
                    ) : (
                      <span className="inline-flex items-center gap-3">
                        {cat.name}
                        {confirmingId === cat.id ? (
                          <span className="inline-flex items-center gap-2 text-xs">
                            <span className="text-foreground-secondary">Delete?</span>
                            <button
                              type="button"
                              onClick={() => remove(cat)}
                              aria-label={`Confirm delete ${cat.name}`}
                              className="rounded-md px-2 py-1 font-medium text-danger hover:bg-danger/10 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-danger"
                            >
                              Delete
                            </button>
                            <button
                              type="button"
                              onClick={() => setConfirmingId(null)}
                              aria-label={`Keep ${cat.name}`}
                              className="rounded-md px-2 py-1 text-foreground-secondary hover:bg-[color:var(--ink-900)]/5 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
                            >
                              Keep
                            </button>
                          </span>
                        ) : (
                          <span className="inline-flex items-center gap-1">
                            <button
                              type="button"
                              onClick={() => startRename(cat)}
                              disabled={isPending}
                              aria-label={`Rename ${cat.name}`}
                              title="Rename"
                              className={ICON_BUTTON}
                            >
                              <Pencil className="h-3.5 w-3.5" strokeWidth={1.5} />
                            </button>
                            <button
                              type="button"
                              onClick={() => setConfirmingId(cat.id)}
                              disabled={isPending}
                              aria-label={`Delete ${cat.name}`}
                              title="Delete"
                              className={`${ICON_BUTTON} hover:text-danger`}
                            >
                              <Trash2 className="h-3.5 w-3.5" strokeWidth={1.5} />
                            </button>
                          </span>
                        )}
                      </span>
                    )}
                  </td>
                  <td className="px-3 py-3 font-mono text-xs text-foreground-tertiary">
                    {cat.slug}
                  </td>
                  <td className="px-3 pr-4 py-3 text-right">
                    <button
                      type="button"
                      onClick={() => toggle(cat)}
                      disabled={isPending}
                      aria-pressed={cat.featured}
                      aria-label={
                        cat.featured
                          ? `Unfeature ${cat.name}`
                          : `Feature ${cat.name}`
                      }
                      className={[
                        "inline-flex items-center gap-1.5 rounded-md px-3 py-1.5 text-xs uppercase tracking-widest transition-colors",
                        "focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]",
                        cat.featured
                          ? "bg-[color:var(--moss-700)]/[0.12] text-[color:var(--moss-700)] hover:bg-[color:var(--moss-700)]/[0.18]"
                          : "text-foreground-tertiary hover:text-[color:var(--moss-700)]",
                        "disabled:cursor-not-allowed disabled:opacity-50",
                      ].join(" ")}
                    >
                      <Star
                        className="h-3.5 w-3.5"
                        fill={cat.featured ? "currentColor" : "none"}
                        strokeWidth={cat.featured ? 0 : 1.5}
                      />
                      {cat.featured ? "Featured" : "Feature"}
                    </button>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      )}
    </div>
  );
}

const ICON_BUTTON =
  "inline-flex h-8 w-8 items-center justify-center rounded-md text-foreground-tertiary transition-colors hover:bg-[color:var(--ink-900)]/5 hover:text-[color:var(--moss-700)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:opacity-50";
