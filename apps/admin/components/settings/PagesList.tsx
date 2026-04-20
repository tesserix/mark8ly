"use client";

import { useRouter } from "next/navigation";
import { useState, useTransition } from "react";
import Link from "next/link";
import { ExternalLink, EyeOff, Plus, Send, Trash2 } from "lucide-react";
import { AlertDialog } from "@tesserix/web";

import type { AdminPageSummary } from "@/lib/api/marketplace-api";
import {
  createDraftPageAndRedirect,
  deletePageAction,
  updatePageAction,
} from "@/app/(admin)/settings/pages/actions";
import { useToast } from "@/components/feedback/Toaster";

interface PagesListProps {
  pages: AdminPageSummary[];
  canManage: boolean;
}

function formatDate(iso: string): string {
  try {
    return new Date(iso).toLocaleDateString(undefined, {
      year: "numeric",
      month: "short",
      day: "numeric",
    });
  } catch {
    return iso;
  }
}

export function PagesList({ pages, canManage }: PagesListProps) {
  const router = useRouter();
  const [pending, startTransition] = useTransition();
  const [error, setError] = useState<string | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<AdminPageSummary | null>(null);
  const { toast } = useToast();

  const handleAdd = () => {
    setError(null);
    startTransition(async () => {
      const result = await createDraftPageAndRedirect();
      // redirect() throws — reaching here means an error was returned
      if (result && !result.ok) {
        setError(result.message);
        toast.error("Could not create page", result.message);
      }
    });
  };

  const handleTogglePublished = (p: AdminPageSummary) => {
    const next = !p.published;
    startTransition(async () => {
      const result = await updatePageAction(p.id, { published: next });
      if (result.ok) {
        toast.success(next ? "Page published" : "Page unpublished");
        router.refresh();
      } else {
        toast.error("Update failed", result.message);
      }
    });
  };

  const handleDeleteConfirm = () => {
    const target = deleteTarget;
    setDeleteTarget(null);
    if (!target) return;
    startTransition(async () => {
      const result = await deletePageAction(target.id);
      if (result.ok) {
        toast.success("Page deleted");
        router.refresh();
      } else {
        toast.error("Delete failed", result.message);
      }
    });
  };

  return (
    <section className="space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="font-serif text-2xl font-medium text-foreground">
          Your pages
        </h2>
        {canManage ? (
          <button
            type="button"
            onClick={handleAdd}
            disabled={pending}
            className="inline-flex items-center justify-center rounded-xl border border-border bg-background-elevated/70 px-3 py-1.5 text-xs font-medium text-foreground hover:bg-background-elevated disabled:cursor-not-allowed disabled:opacity-50"
          >
            <Plus className="mr-1.5 h-3.5 w-3.5" />
            {pending ? "Creating…" : "New page"}
          </button>
        ) : null}
      </div>

      {error ? (
        <p role="alert" className="text-sm text-danger">
          {error}
        </p>
      ) : null}

      {pages.length === 0 ? (
        <div className="rounded-md border border-dashed border-border px-6 py-12 text-center">
          <p className="font-serif text-lg text-foreground">No pages yet</p>
          <p className="mt-2 text-sm text-muted-foreground">
            Your first page could be About, Terms, or Privacy. Add it from the
            button above.
          </p>
        </div>
      ) : (
        <div className="overflow-hidden rounded-md border border-border">
          <table className="w-full text-left text-sm">
            <thead className="bg-muted/40 text-xs uppercase tracking-wide text-muted-foreground">
              <tr>
                <th className="px-4 py-3 font-medium">Title</th>
                <th className="px-4 py-3 font-medium">Path</th>
                <th className="px-4 py-3 font-medium">Status</th>
                <th className="px-4 py-3 font-medium">Last updated</th>
                <th className="px-4 py-3" />
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {pages.map((p) => (
                <tr key={p.id}>
                  <td className="px-4 py-3">
                    <Link
                      href={`/settings/pages/${p.id}`}
                      className="font-medium text-foreground hover:text-moss-700"
                    >
                      {p.title}
                    </Link>
                  </td>
                  <td className="px-4 py-3 text-muted-foreground">
                    <code>/pages/{p.slug}</code>
                  </td>
                  <td className="px-4 py-3">
                    {p.published ? (
                      <span className="inline-flex items-center rounded-full bg-moss-100 px-2 py-0.5 text-xs font-medium text-moss-800">
                        Published
                      </span>
                    ) : (
                      <span className="inline-flex items-center rounded-full bg-muted px-2 py-0.5 text-xs font-medium text-muted-foreground">
                        Draft
                      </span>
                    )}
                  </td>
                  <td className="px-4 py-3 text-muted-foreground">
                    {formatDate(p.updated_at)}
                  </td>
                  <td className="px-4 py-3 text-right">
                    <div className="flex items-center justify-end gap-1">
                      <a
                        href={`/pages/${p.slug}`}
                        target="_blank"
                        rel="noreferrer"
                        className="inline-flex h-8 items-center rounded-md px-2 text-sm text-moss-700 hover:bg-moss-50"
                        aria-label={`Preview ${p.title}`}
                        title="Preview"
                      >
                        <ExternalLink className="h-4 w-4" />
                      </a>
                      {canManage ? (
                        <>
                          <button
                            type="button"
                            onClick={() => handleTogglePublished(p)}
                            disabled={pending}
                            className="inline-flex h-8 items-center rounded-md px-2 text-muted-foreground hover:bg-muted hover:text-foreground disabled:opacity-40"
                            aria-label={p.published ? `Unpublish ${p.title}` : `Publish ${p.title}`}
                            title={p.published ? "Unpublish" : "Publish"}
                          >
                            {p.published ? (
                              <EyeOff className="h-4 w-4" />
                            ) : (
                              <Send className="h-4 w-4" />
                            )}
                          </button>
                          <button
                            type="button"
                            onClick={() => setDeleteTarget(p)}
                            disabled={pending}
                            className="inline-flex h-8 items-center rounded-md px-2 text-muted-foreground hover:bg-danger/10 hover:text-danger disabled:opacity-40"
                            aria-label={`Delete ${p.title}`}
                            title="Delete"
                          >
                            <Trash2 className="h-4 w-4" />
                          </button>
                        </>
                      ) : null}
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <AlertDialog
        isOpen={deleteTarget !== null}
        onClose={() => setDeleteTarget(null)}
        title="Delete page"
        message={
          deleteTarget
            ? `Delete "${deleteTarget.title}"? This cannot be undone.`
            : ""
        }
        type="confirm"
        confirmLabel="Delete"
        cancelLabel="Cancel"
        onConfirm={handleDeleteConfirm}
        onCancel={() => setDeleteTarget(null)}
      />
    </section>
  );
}
