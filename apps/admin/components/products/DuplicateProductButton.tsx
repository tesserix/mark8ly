"use client";

// Duplicate this product within the same store, as a draft, with its
// personalisation fields. For merchants who list one product per design:
// set up one well, then duplicate and swap the title, photos and price.

import * as React from "react";
import { useTransition } from "react";

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
      className="text-[color:var(--moss-700)] transition-opacity hover:opacity-80 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:cursor-wait disabled:opacity-50"
    >
      {pending ? "Duplicating…" : "Duplicate"}
    </button>
  );
}
