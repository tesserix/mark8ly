"use client";

import Link from "next/link";
import { useState, useTransition } from "react";
import { Star } from "lucide-react";

import type { ReviewStatus } from "@/lib/api/marketplace-api";

import {
  approveReviewAction,
  rejectReviewAction,
  toggleFeaturedAction,
} from "@/app/(admin)/customers/reviews/actions";

interface ReviewRowActionsProps {
  reviewId: string;
  status: ReviewStatus;
  featured: boolean;
  showViewLink?: boolean;
}

export function ReviewRowActions({
  reviewId,
  status,
  featured,
  showViewLink = true,
}: ReviewRowActionsProps) {
  const [isPending, startTransition] = useTransition();
  const [error, setError] = useState<string | null>(null);

  const runAction = (fn: () => Promise<{ ok: boolean; error?: string }>) => {
    setError(null);
    startTransition(async () => {
      const result = await fn();
      if (!result.ok) {
        setError(result.error ?? "Action failed.");
      }
    });
  };

  return (
    <div className="flex flex-col items-end gap-1">
      <div className="flex flex-wrap items-center justify-end gap-2">
        <button
          type="button"
          onClick={() =>
            runAction(() => toggleFeaturedAction(reviewId, !featured))
          }
          disabled={isPending}
          aria-pressed={featured}
          aria-label={featured ? "Unfeature review" : "Feature review"}
          className={`inline-flex min-h-[2.5rem] items-center justify-center rounded-sm border px-2.5 py-1 transition-colors disabled:cursor-not-allowed disabled:opacity-40 ${
            featured
              ? "border-[color:var(--moss-700)] bg-[color:rgba(45,74,43,0.06)] text-[color:var(--moss-700)]"
              : "border-border-subtle text-foreground-secondary hover:text-foreground"
          }`}
          title={featured ? "Unfeature" : "Feature"}
        >
          <Star
            className="h-3.5 w-3.5"
            fill={featured ? "currentColor" : "none"}
            aria-hidden="true"
          />
        </button>

        <button
          type="button"
          onClick={() => runAction(() => approveReviewAction(reviewId))}
          disabled={isPending || status === "approved"}
          className="inline-flex min-h-[2.5rem] items-center rounded-sm border border-[color:var(--moss-700)] px-3 py-1 text-xs font-semibold uppercase tracking-wider text-[color:var(--moss-700)] transition-colors hover:bg-[color:rgba(45,74,43,0.06)] disabled:cursor-not-allowed disabled:opacity-40"
        >
          Approve
        </button>
        <button
          type="button"
          onClick={() => runAction(() => rejectReviewAction(reviewId))}
          disabled={isPending || status === "rejected"}
          className="inline-flex min-h-[2.5rem] items-center rounded-sm border border-border-subtle px-3 py-1 text-xs font-semibold uppercase tracking-wider text-foreground-secondary transition-colors hover:text-foreground disabled:cursor-not-allowed disabled:opacity-40"
        >
          Reject
        </button>
        {showViewLink && (
          <Link
            href={`/customers/reviews/${reviewId}`}
            className="inline-flex min-h-[2.5rem] items-center px-2 text-xs font-semibold uppercase tracking-wider text-foreground-secondary underline-offset-4 hover:text-foreground hover:underline"
          >
            View
          </Link>
        )}
      </div>
      {error && (
        <p
          role="alert"
          className="text-[11px] text-[color:var(--danger,#8e1a1a)]"
        >
          {error}
        </p>
      )}
    </div>
  );
}
