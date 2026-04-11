"use client";

import { ErrorState } from "@/components/layout";

export default function SupportError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  return (
    <ErrorState
      title="We couldn't load the support page"
      description="Something went wrong while fetching tickets or help articles. Try again — most transient failures clear on retry."
      error={error}
      onRetry={reset}
      backHref="/dashboard"
      backLabel="Go to dashboard"
    />
  );
}
