"use client";

// apps/admin/components/products/BulkDeleteConfirmDialog.tsx
//
// Hard-delete confirmation dialog — the ONE extra dialog permitted
// per §13.5 besides CopyToStoreDialog. Uses @tesserix/web AlertDialog.

import { AlertDialog } from "@tesserix/web";

export interface BulkDeleteConfirmDialogProps {
  isOpen: boolean;
  count: number;
  onConfirm: () => void;
  onCancel: () => void;
}

export function BulkDeleteConfirmDialog({
  isOpen,
  count,
  onConfirm,
  onCancel,
}: BulkDeleteConfirmDialogProps) {
  return (
    <AlertDialog
      isOpen={isOpen}
      onClose={onCancel}
      title={`Delete ${count} ${count === 1 ? "product" : "products"}?`}
      message="This can't be undone."
      type="confirm"
      confirmLabel="Delete"
      cancelLabel="Cancel"
      onConfirm={onConfirm}
      onCancel={onCancel}
    />
  );
}
