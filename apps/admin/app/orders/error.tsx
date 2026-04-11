"use client";

import { ErrorState } from "@/components/layout/ErrorState";

export default function OrdersError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  return (
    <ErrorState
      title="We couldn't load your orders"
      description="Something went wrong while fetching orders. Try again — most transient failures clear on retry."
      error={error}
      onRetry={reset}
      backHref="/dashboard"
      backLabel="Go to dashboard"
    />
  );
}
