"use client";

// apps/admin/components/products/CopyToStoreDialog.tsx
//
// Copy-to-store dialog using @tesserix/web Dialog primitive.
// Supports single-product and bulk mode (pass multiple productIds).
// Copied products land as drafts in the target store (fixed behavior).

import { useState, useCallback } from "react";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
  DialogClose,
} from "@tesserix/web";

import type { AdminStore } from "@/lib/api/marketplace-api";

export interface CopyToStoreRequest {
  targetStoreId: string;
  copyMedia: boolean;
  productIds: string[];
}

export interface CopyToStoreDialogProps {
  isOpen: boolean;
  onClose: () => void;
  stores: AdminStore[];
  currentStoreId: string;
  productIds: string[];
  onCopy: (request: CopyToStoreRequest) => Promise<void>;
}

export function CopyToStoreDialog({
  isOpen,
  onClose,
  stores,
  currentStoreId,
  productIds,
  onCopy,
}: CopyToStoreDialogProps) {
  const [selectedStoreId, setSelectedStoreId] = useState<string | null>(null);
  const [copyMedia, setCopyMedia] = useState(true);
  const [isPending, setIsPending] = useState(false);

  const availableStores = stores.filter((s) => s.id !== currentStoreId);
  const isBulk = productIds.length > 1;

  const handleSubmit = useCallback(async () => {
    if (!selectedStoreId) return;
    setIsPending(true);
    try {
      await onCopy({
        targetStoreId: selectedStoreId,
        copyMedia,
        productIds,
      });
    } finally {
      setIsPending(false);
    }
  }, [selectedStoreId, copyMedia, productIds, onCopy]);

  const handleOpenChange = useCallback(
    (open: boolean) => {
      if (!open) {
        onClose();
        setSelectedStoreId(null);
        setCopyMedia(true);
      }
    },
    [onClose],
  );

  const title = isBulk
    ? `Copy ${productIds.length} products to another store`
    : "Copy product to another store";

  return (
    <Dialog open={isOpen} onOpenChange={handleOpenChange}>
      <DialogContent size="sm">
        <DialogHeader>
          <DialogTitle className="font-[family-name:var(--font-serif,'Source_Serif_4',serif)] text-lg text-[color:var(--ink-900)]">
            {title}
          </DialogTitle>
          <DialogDescription className="text-sm text-[color:var(--ink-900)] opacity-60">
            Copied products will land as drafts in the target store.
          </DialogDescription>
        </DialogHeader>

        <fieldset className="flex flex-col gap-1 py-2">
          <legend className="pb-2 text-xs font-medium uppercase tracking-wide text-[color:var(--ink-900)] opacity-60">
            Target store
          </legend>
          {availableStores.map((store) => (
            <label
              key={store.id}
              className={`flex cursor-pointer items-center gap-3 rounded-md px-3 py-2.5 text-sm transition-colors ${
                selectedStoreId === store.id
                  ? "bg-[color:var(--moss-700)] bg-opacity-[0.08] text-[color:var(--ink-900)]"
                  : "text-[color:var(--ink-900)] hover:bg-[color:var(--ink-900)]/[0.03]"
              }`}
            >
              <input
                type="radio"
                name="target-store"
                value={store.id}
                checked={selectedStoreId === store.id}
                onChange={() => setSelectedStoreId(store.id)}
                aria-label={store.name}
                className="h-4 w-4 border-[color:var(--ink-900)] border-opacity-30 text-[color:var(--moss-700)] focus:ring-[color:var(--moss-700)]"
              />
              <span className="flex-1">{store.name}</span>
              <span className="text-xs opacity-50">
                {store.currency_code}
              </span>
            </label>
          ))}
        </fieldset>

        <label className="flex items-center gap-3 border-t border-[color:var(--ink-900)] border-opacity-10 py-3 text-sm text-[color:var(--ink-900)]">
          <input
            type="checkbox"
            checked={copyMedia}
            onChange={(e) => setCopyMedia(e.target.checked)}
            aria-label="Copy media"
            className="h-4 w-4 rounded border-[color:var(--ink-900)] border-opacity-30 text-[color:var(--moss-700)] focus:ring-[color:var(--moss-700)]"
          />
          Also copy media files
        </label>

        <DialogFooter className="flex gap-3">
          <DialogClose asChild>
            <button
              type="button"
              className="rounded-md border border-[color:var(--ink-900)] border-opacity-20 px-4 py-2 text-sm text-[color:var(--ink-900)] hover:bg-[color:var(--ink-900)]/5 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
            >
              Cancel
            </button>
          </DialogClose>
          <button
            type="button"
            disabled={!selectedStoreId || isPending}
            onClick={handleSubmit}
            className="rounded-md bg-[color:var(--ink-900)] px-4 py-2 text-sm text-[color:var(--paper-200)] transition-colors hover:bg-[color:var(--moss-700)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:opacity-40"
          >
            {isPending ? "Copying…" : "Copy"}
          </button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
