"use client";

// apps/admin/components/products/BulkActionsBar.tsx
//
// Sticky bottom bar for bulk product actions. Renders only when ≥1
// product is selected AND role is admin or owner (staff sees nothing
// per §13.1.1). Paper surface, hairline top rule, Moss action accents.

import {
  Archive,
  ArchiveRestore,
  Globe,
  GlobeLock,
  Copy,
  FolderTree,
  Trash2,
} from "lucide-react";

import type { AdminCategory } from "@/lib/api/marketplace-api";

export type StoreRole = "owner" | "admin" | "staff";

export interface BulkActionsBarProps {
  selectedIds: string[];
  role: StoreRole;
  storeId: string;
  categories: AdminCategory[];
  onActionComplete: () => void;
  onCopyClick: () => void;
  onDeleteClick: () => void;
  onCategoryAssignClick: () => void;
  onBulkAction: (action: string) => void;
}

export function BulkActionsBar({
  selectedIds,
  role,
  onCopyClick,
  onDeleteClick,
  onCategoryAssignClick,
  onBulkAction,
}: BulkActionsBarProps) {
  if (selectedIds.length === 0) return null;
  if (role === "staff") return null;

  const count = selectedIds.length;
  const isOwner = role === "owner";

  return (
    <div
      className="fixed inset-x-0 bottom-0 z-30 border-t border-[color:var(--ink-900)] border-opacity-10 bg-[color:var(--paper-200)] shadow-[var(--shadow-1,0_-1px_3px_rgba(0,0,0,0.06))]"
      role="toolbar"
      aria-label="Bulk actions"
    >
      <div className="mx-auto flex max-w-7xl items-center gap-3 px-4 py-3">
        <span className="text-sm font-medium text-[color:var(--ink-900)]">
          {count} selected
        </span>

        <div className="h-4 w-px bg-[color:var(--ink-900)] opacity-15" />

        <ActionButton
          icon={<Archive className="h-4 w-4" />}
          label="Archive"
          onClick={() => onBulkAction("archive")}
        />
        <ActionButton
          icon={<ArchiveRestore className="h-4 w-4" />}
          label="Unarchive"
          onClick={() => onBulkAction("unarchive")}
        />
        <ActionButton
          icon={<Globe className="h-4 w-4" />}
          label="Publish"
          onClick={() => onBulkAction("publish")}
        />
        <ActionButton
          icon={<GlobeLock className="h-4 w-4" />}
          label="Unpublish"
          onClick={() => onBulkAction("unpublish")}
        />
        <ActionButton
          icon={<FolderTree className="h-4 w-4" />}
          label="Assign category"
          onClick={onCategoryAssignClick}
        />
        <ActionButton
          icon={<Copy className="h-4 w-4" />}
          label="Copy to store"
          onClick={onCopyClick}
        />

        {isOwner && (
          <>
            <div className="h-4 w-px bg-[color:var(--ink-900)] opacity-15" />
            <ActionButton
              icon={<Trash2 className="h-4 w-4" />}
              label="Delete"
              onClick={onDeleteClick}
              variant="danger"
            />
          </>
        )}
      </div>
    </div>
  );
}

function ActionButton({
  icon,
  label,
  onClick,
  variant = "default",
}: {
  icon: React.ReactNode;
  label: string;
  onClick: () => void;
  variant?: "default" | "danger";
}) {
  const colorClass =
    variant === "danger"
      ? "text-[color:var(--danger,#8B2500)] hover:bg-[color:var(--danger,#8B2500)] hover:bg-opacity-[0.08]"
      : "text-[color:var(--ink-900)] hover:bg-[color:var(--ink-900)] hover:bg-opacity-5";

  return (
    <button
      type="button"
      onClick={onClick}
      className={`inline-flex items-center gap-1.5 rounded-md px-3 py-1.5 text-sm transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] ${colorClass}`}
    >
      {icon}
      {label}
    </button>
  );
}
