"use client";

import { useState, useTransition } from "react";
import { useRouter } from "next/navigation";

import { useToast } from "@/components/feedback/Toaster";
import type { CustomerStatus } from "@/lib/api/marketplace-api";

interface CustomerActionsBarProps {
  customerId: string;
  status: CustomerStatus;
  blockAction: (customerId: string, reason: string) => Promise<{ ok: boolean; error?: string }>;
  unblockAction: (customerId: string) => Promise<{ ok: boolean; error?: string }>;
}

export function CustomerActionsBar({
  customerId,
  status,
  blockAction,
  unblockAction,
}: CustomerActionsBarProps) {
  const router = useRouter();
  const { toast } = useToast();
  const [isPending, startTransition] = useTransition();
  const [showBlockForm, setShowBlockForm] = useState(false);
  const [blockReason, setBlockReason] = useState("");
  const [validationError, setValidationError] = useState<string | null>(null);

  function handleBlock() {
    if (!blockReason.trim()) {
      setValidationError("Reason is required.");
      return;
    }
    setValidationError(null);
    startTransition(async () => {
      const result = await blockAction(customerId, blockReason.trim());
      if (result.ok) {
        toast.success("Customer blocked", "They can no longer place orders.");
        setShowBlockForm(false);
        setBlockReason("");
        router.refresh();
      } else {
        toast.error("Couldn't block customer", result.error);
      }
    });
  }

  function handleUnblock() {
    setValidationError(null);
    startTransition(async () => {
      const result = await unblockAction(customerId);
      if (result.ok) {
        toast.success("Customer unblocked");
        router.refresh();
      } else {
        toast.error("Couldn't unblock customer", result.error);
      }
    });
  }

  return (
    <section aria-labelledby="actions-heading" className="flex flex-col gap-4">
      <div className="flex flex-col gap-2">
        <h2
          id="actions-heading"
          className="text-xs font-semibold uppercase tracking-[0.16em] text-foreground-tertiary"
        >
          Actions
        </h2>
        <p className="text-sm text-foreground-secondary">
          {status === "active"
            ? "Blocking prevents this customer from placing future orders."
            : "This customer is currently blocked."}
        </p>
      </div>

      {status === "active" && !showBlockForm && (
        <button
          type="button"
          onClick={() => setShowBlockForm(true)}
          disabled={isPending}
          className="inline-flex w-fit items-center rounded-md border border-[color:var(--danger)] px-4 py-2 text-sm font-medium text-[color:var(--danger)] transition-colors hover:bg-[color:var(--danger)]/[0.06] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:opacity-40"
        >
          Block customer
        </button>
      )}

      {status === "active" && showBlockForm && (
        <div className="flex max-w-md flex-col gap-3">
          <input
            type="text"
            value={blockReason}
            onChange={(e) => setBlockReason(e.target.value)}
            placeholder="Reason for blocking..."
            aria-label="Reason for blocking"
            disabled={isPending}
            className="w-full rounded-md border border-[color:var(--ink-900)]/20 bg-transparent px-3 py-2 text-sm text-foreground placeholder:text-foreground-tertiary focus:border-[color:var(--moss-700)] focus:outline-none disabled:opacity-50"
          />
          <div className="flex gap-2">
            <button
              type="button"
              onClick={handleBlock}
              disabled={isPending || !blockReason.trim()}
              className="rounded-md bg-[color:var(--danger)] px-4 py-2 text-sm font-medium text-[color:var(--primary-foreground)] transition-colors hover:bg-[color:var(--danger)]/90 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:opacity-40"
            >
              {isPending ? "Blocking..." : "Confirm block"}
            </button>
            <button
              type="button"
              onClick={() => {
                setShowBlockForm(false);
                setBlockReason("");
              }}
              disabled={isPending}
              className="rounded-md border border-[color:var(--ink-900)]/20 px-4 py-2 text-sm text-foreground transition-colors hover:bg-[color:var(--ink-900)]/[0.04] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:opacity-40"
            >
              Cancel
            </button>
          </div>
        </div>
      )}

      {status === "blocked" && (
        <button
          type="button"
          onClick={handleUnblock}
          disabled={isPending}
          className="inline-flex w-fit items-center rounded-md bg-[color:var(--moss-700)] px-4 py-2 text-sm font-medium text-[color:var(--primary-foreground)] transition-colors hover:bg-[color:var(--moss-800)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:opacity-40"
        >
          {isPending ? "Unblocking..." : "Unblock customer"}
        </button>
      )}

      {validationError && (
        <p role="alert" className="text-sm text-[color:var(--danger)]">
          {validationError}
        </p>
      )}
    </section>
  );
}
