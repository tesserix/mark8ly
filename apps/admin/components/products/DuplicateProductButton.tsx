"use client";

// Duplicate this product within the same store, as a draft, with its
// personalisation fields. For merchants who list one product per design:
// set up one well, then duplicate and swap the title, photos and price.

import * as React from "react";
import { useTransition } from "react";
import { Copy } from "lucide-react";

import { duplicateProductAction } from "@/app/(admin)/products/actions";
import { useToast } from "@/components/feedback/Toaster";

export function DuplicateProductButton({
  storeId,
  productId,
}: {
  storeId: string;
  productId: string;
}): React.ReactElement {
  const { toast } = useToast();
  const [pending, startTransition] = useTransition();

  return (
    <button
      type="button"
      disabled={pending}
      onClick={() =>
        startTransition(async () => {
          // On success the action redirects to the new draft.
          const result = await duplicateProductAction(storeId, productId);
          if (result && !result.ok) {
            toast.error("Couldn't duplicate", result.error?.message ?? "Try again in a moment.");
          }
        })
      }
      // The copy is made from the saved product, not unsaved edits on screen.
      title="Make a draft copy of the saved product"
      className="inline-flex items-center gap-1.5 rounded-md border border-[color:var(--ink-200)] px-4 py-2 text-sm font-medium text-[color:var(--ink-900)] transition-colors hover:border-[color:var(--moss-700)] hover:text-[color:var(--moss-700)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:cursor-wait disabled:opacity-50"
    >
      <Copy className="h-4 w-4" aria-hidden="true" />
      {pending ? "Duplicating…" : "Duplicate"}
    </button>
  );
}
