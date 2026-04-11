"use client";

import { ErrorState } from "@/components/layout/ErrorState";

export default function DashboardError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  return (
    <ErrorState
      title="We couldn't load your dashboard"
      description="Something went wrong while building the dashboard overview. Try again — most transient failures clear on retry."
      error={error}
      onRetry={reset}
    />
  );
}
