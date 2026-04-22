"use client";

import { useState, useCallback } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { AlertDialog } from "@tesserix/web";
import type { AdminSegment, SessionHeaders } from "@/lib/api/campaigns-api";
import { deleteSegment } from "@/lib/api/campaigns-api";

interface SegmentsListProps {
  segments: AdminSegment[];
  storeId: string;
  session: SessionHeaders;
}

function formatDate(dateStr: string): string {
  return new Date(dateStr).toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
    year: "numeric",
  });
}

function summarizeRules(rulesJson: string): string {
  try {
    const rules = JSON.parse(rulesJson) as { type: string; value?: string }[];
    if (rules.length === 0) return "All customers";
    return rules
      .map((r) => {
        switch (r.type) {
          case "all":
            return "All enrolled";
          case "loyalty_tier":
            return `Tier: ${r.value ?? ""}`;
          case "has_ordered":
            return "Has ordered";
          case "inactive_days":
            return `Inactive ${r.value ?? ""}d`;
          default:
            return r.type;
        }
      })
      .join(", ");
  } catch {
    return "Custom rules";
  }
}

export function SegmentsList({
  segments,
  storeId,
  session,
}: SegmentsListProps) {
  const router = useRouter();
  const [deletingId, setDeletingId] = useState<string | null>(null);
  const [pendingDelete, setPendingDelete] = useState<AdminSegment | null>(null);

  const requestDelete = useCallback((segment: AdminSegment) => {
    setPendingDelete(segment);
  }, []);

  const confirmDelete = useCallback(async () => {
    if (!pendingDelete) return;
    const segmentId = pendingDelete.id;
    setPendingDelete(null);
    setDeletingId(segmentId);
    const ok = await deleteSegment(storeId, segmentId, session);
    if (ok) {
      router.refresh();
    }
    setDeletingId(null);
  }, [pendingDelete, storeId, session, router]);

  if (segments.length === 0) {
    return (
      <div className="flex flex-col items-start gap-3 border-t border-border-subtle py-12">
        <h2 className="font-serif text-xl font-medium text-foreground">
          No segments yet
        </h2>
        <p className="max-w-prose text-sm text-foreground-secondary">
          Create your first segment to define an audience for your campaigns.
        </p>
        <Link
          href="/marketing/segments/new"
          className="mt-1 inline-flex items-center gap-2 rounded-md bg-[color:var(--ink-900)] px-4 py-2 text-sm font-medium text-[color:var(--primary-foreground)] transition-colors hover:bg-[color:var(--moss-700)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
        >
          Create segment
        </Link>
      </div>
    );
  }

  return (
    <div className="overflow-x-auto">
      <table className="w-full text-left text-sm">
        <thead>
          <tr className="border-b border-[color:var(--ink-900)]/15 text-xs font-medium uppercase tracking-wider text-foreground-tertiary">
            <th className="pb-3 pr-4">Name</th>
            <th className="pb-3 pr-4 text-right">Members</th>
            <th className="pb-3 pr-4">Rules</th>
            <th className="pb-3 pr-4">Created</th>
            <th className="pb-3 pr-4">
              <span className="sr-only">Actions</span>
            </th>
          </tr>
        </thead>
        <tbody>
          {segments.map((seg) => (
            <tr
              key={seg.id}
              className="group border-b border-[color:var(--ink-900)]/10 transition-colors hover:bg-[color:var(--ink-900)]/[0.03]"
            >
              <td className="py-3 pr-4">
                <span className="font-serif text-base text-foreground">
                  {seg.name}
                </span>
                {seg.description && (
                  <p className="mt-0.5 line-clamp-1 text-xs text-foreground-tertiary">
                    {seg.description}
                  </p>
                )}
              </td>
              <td className="py-3 pr-4 text-right font-serif text-base tabular-nums text-foreground">
                {seg.member_count.toLocaleString()}
              </td>
              <td className="py-3 pr-4 text-foreground-secondary">
                {summarizeRules(seg.rules)}
              </td>
              <td className="py-3 pr-4 text-foreground-tertiary tabular-nums">
                {formatDate(seg.created_at)}
              </td>
              <td className="py-3 pr-4 text-right">
                <div className="flex items-center justify-end gap-3 opacity-0 transition-opacity group-hover:opacity-100 focus-within:opacity-100">
                  <Link
                    href={`/marketing/segments/${seg.id}/edit`}
                    aria-label={`Edit segment ${seg.name}`}
                    className="text-xs text-foreground-secondary underline-offset-4 hover:text-foreground hover:underline"
                  >
                    Edit
                  </Link>
                  <button
                    onClick={() => requestDelete(seg)}
                    disabled={deletingId === seg.id}
                    aria-label={`Delete segment ${seg.name}`}
                    className="text-xs text-[color:var(--danger)] underline-offset-4 hover:underline disabled:opacity-50"
                  >
                    {deletingId === seg.id ? "Deleting…" : "Delete"}
                  </button>
                </div>
              </td>
            </tr>
          ))}
        </tbody>
      </table>

      <AlertDialog
        isOpen={pendingDelete !== null}
        onClose={() => setPendingDelete(null)}
        title={pendingDelete ? `Delete "${pendingDelete.name}"?` : "Delete segment?"}
        message="This can't be undone."
        type="confirm"
        confirmLabel="Delete"
        cancelLabel="Cancel"
        onConfirm={confirmDelete}
        onCancel={() => setPendingDelete(null)}
      />
    </div>
  );
}
