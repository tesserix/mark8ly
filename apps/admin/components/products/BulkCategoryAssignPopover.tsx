"use client";

// apps/admin/components/products/BulkCategoryAssignPopover.tsx
//
// Inline popover that reuses the M7b ProductCategoriesPicker for bulk
// category assignment. Mounted from BulkActionsBar — no extra dialog.

import { useState, useCallback } from "react";

import { ProductCategoriesPicker } from "./ProductCategoriesPicker";
import type { AdminCategory } from "@/lib/api/marketplace-api";

export interface BulkCategoryAssignPopoverProps {
  isOpen: boolean;
  categories: AdminCategory[];
  count: number;
  onApply: (categoryIds: string[]) => void;
  onClose: () => void;
}

export function BulkCategoryAssignPopover({
  isOpen,
  categories,
  count,
  onApply,
  onClose,
}: BulkCategoryAssignPopoverProps) {
  const [selectedIds, setSelectedIds] = useState<string[]>([]);

  const handleApply = useCallback(() => {
    onApply(selectedIds);
    setSelectedIds([]);
  }, [selectedIds, onApply]);

  const handleClose = useCallback(() => {
    setSelectedIds([]);
    onClose();
  }, [onClose]);

  if (!isOpen) return null;

  return (
    <div className="fixed inset-x-0 bottom-16 z-40 mx-auto w-full max-w-md rounded-lg border border-[color:var(--ink-900)] border-opacity-10 bg-[color:var(--background-elevated,white)] p-4 shadow-[var(--shadow-2,0_4px_12px_rgba(0,0,0,0.08))]">
      <div className="mb-3 flex items-center justify-between">
        <h3 className="text-sm font-medium text-[color:var(--ink-900)]">
          Assign categories
        </h3>
        <button
          type="button"
          onClick={handleClose}
          className="text-xs text-foreground-tertiary hover:opacity-100 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
        >
          Close
        </button>
      </div>

      <ProductCategoriesPicker
        allCategories={categories}
        selectedIds={selectedIds}
        onChange={setSelectedIds}
      />

      <div className="mt-3 flex justify-end border-t border-[color:var(--ink-900)] border-opacity-10 pt-3">
        <button
          type="button"
          onClick={handleApply}
          disabled={selectedIds.length === 0}
          className="rounded-md bg-[color:var(--ink-900)] px-4 py-2 text-sm text-[color:var(--paper-200)] transition-colors hover:bg-[color:var(--moss-700)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:opacity-40"
        >
          Apply to {count}
        </button>
      </div>
    </div>
  );
}
