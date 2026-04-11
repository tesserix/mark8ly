"use client";

import { ErrorState } from "@/components/layout/ErrorState";

export default function MarketingError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  return (
    <ErrorState
      title="We couldn't load this marketing page"
      description="Something went wrong while fetching campaign or segment data. Try again — most transient failures clear on retry."
      error={error}
      onRetry={reset}
      backHref="/dashboard"
      backLabel="Go to dashboard"
    />
  );
}
