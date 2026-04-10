"use client";

import { useState, useTransition } from "react";
import { useRouter } from "next/navigation";

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
  const [isPending, startTransition] = useTransition();
  const [showBlockForm, setShowBlockForm] = useState(false);
  const [blockReason, setBlockReason] = useState("");
  const [error, setError] = useState<string | null>(null);

  function handleBlock() {
    if (!blockReason.trim()) return;
    setError(null);
    startTransition(async () => {
      const result = await blockAction(customerId, blockReason.trim());
      if (result.ok) {
        setShowBlockForm(false);
        setBlockReason("");
        router.refresh();
      } else {
        setError(result.error ?? "Failed to block customer");
      }
    });
  }

  function handleUnblock() {
    setError(null);
    startTransition(async () => {
      const result = await unblockAction(customerId);
      if (result.ok) {
        router.refresh();
      } else {
        setError(result.error ?? "Failed to unblock customer");
      }
    });
  }

  return (
    <section aria-labelledby="actions-heading" className="flex flex-col gap-4">
      <h2
        id="actions-heading"
        className="font-[family-name:var(--font-serif,'Source_Serif_4',serif)] text-2xl text-[color:var(--ink-900)]"
      >
        Actions
      </h2>

      {status === "active" && !showBlockForm && (
        <button
          type="button"
          onClick={() => setShowBlockForm(true)}
          disabled={isPending}
          className="w-full rounded-md border border-[color:var(--signal,#C4391D)] px-4 py-2 text-sm text-[color:var(--signal,#C4391D)] transition-colors hover:bg-[color:var(--signal,#C4391D)] hover:bg-opacity-[0.06] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:opacity-40"
        >
          Block customer
        </button>
      )}

      {status === "active" && showBlockForm && (
        <div className="flex flex-col gap-3">
          <input
            type="text"
            value={blockReason}
            onChange={(e) => setBlockReason(e.target.value)}
            placeholder="Reason for blocking..."
            aria-label="Reason for blocking"
            disabled={isPending}
            className="w-full rounded-md border border-[color:var(--ink-900)] border-opacity-20 bg-transparent px-3 py-1.5 text-sm text-[color:var(--ink-900)] placeholder:opacity-40 focus:border-[color:var(--moss-700)] focus:outline-none disabled:opacity-50"
          />
          <div className="flex gap-2">
            <button
              type="button"
              onClick={handleBlock}
              disabled={isPending || !blockReason.trim()}
              className="rounded-md bg-[color:var(--signal,#C4391D)] px-4 py-1.5 text-sm text-[color:var(--paper-200,#F7F6F2)] transition-opacity hover:opacity-90 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:opacity-40"
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
              className="rounded-md border border-[color:var(--ink-900)] border-opacity-20 px-4 py-1.5 text-sm text-[color:var(--ink-900)] transition-colors hover:bg-[color:var(--ink-900)] hover:bg-opacity-[0.04] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:opacity-40"
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
          className="w-full rounded-md bg-[color:var(--moss-700)] px-4 py-2 text-sm text-[color:var(--paper-200,#F7F6F2)] transition-opacity hover:opacity-90 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:opacity-40"
        >
          {isPending ? "Unblocking..." : "Unblock customer"}
        </button>
      )}

      {error && (
        <p role="alert" className="text-sm text-[color:var(--signal,#C4391D)]">
          {error}
        </p>
      )}
    </section>
  );
}
