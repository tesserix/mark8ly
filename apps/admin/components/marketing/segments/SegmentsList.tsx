"use client";

import { useState, useCallback } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
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

  const handleDelete = useCallback(
    async (segmentId: string) => {
      if (!confirm("Delete this segment? This cannot be undone.")) return;
      setDeletingId(segmentId);
      const ok = await deleteSegment(storeId, segmentId, session);
      if (ok) {
        router.refresh();
      }
      setDeletingId(null);
    },
    [storeId, session, router],
  );

  if (segments.length === 0) {
    return (
      <div className="flex flex-col items-start gap-4 rounded-md border border-ink-200 bg-white px-6 py-12">
        <h2 className="font-serif text-lg font-semibold text-ink-900">
          No segments yet
        </h2>
        <p className="text-sm text-ink-500">
          Create your first segment to define an audience for your campaigns.
        </p>
        <Link
          href="/marketing/segments/new"
          className="inline-flex items-center gap-2 rounded-md bg-ink-900 px-4 py-2 text-sm font-medium text-paper-200 transition hover:bg-ink-800"
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
          <tr className="border-b border-ink-200 text-xs font-medium uppercase tracking-wider text-ink-500">
            <th className="pb-3 pr-4">Name</th>
            <th className="pb-3 pr-4">Members</th>
            <th className="pb-3 pr-4">Rules</th>
            <th className="pb-3 pr-4">Created</th>
            <th className="pb-3 pr-4">
              <span className="sr-only">Actions</span>
            </th>
          </tr>
        </thead>
        <tbody className="divide-y divide-ink-100">
          {segments.map((seg) => (
            <tr key={seg.id} className="group">
              <td className="py-3 pr-4">
                <span className="text-sm font-medium text-ink-900">
                  {seg.name}
                </span>
                {seg.description && (
                  <p className="mt-0.5 text-xs text-ink-500 line-clamp-1">
                    {seg.description}
                  </p>
                )}
              </td>
              <td className="py-3 pr-4 font-mono text-ink-700">
                {seg.member_count.toLocaleString()}
              </td>
              <td className="py-3 pr-4 text-ink-500">
                {summarizeRules(seg.rules)}
              </td>
              <td className="py-3 pr-4 text-ink-500">
                {formatDate(seg.created_at)}
              </td>
              <td className="py-3 pr-4 text-right">
                <button
                  onClick={() => handleDelete(seg.id)}
                  disabled={deletingId === seg.id}
                  aria-label={`Delete segment ${seg.name}`}
                  className="text-xs text-signal-700 opacity-0 transition group-hover:opacity-100 focus-visible:opacity-100 disabled:opacity-50"
                >
                  {deletingId === seg.id ? "Deleting..." : "Delete"}
                </button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
