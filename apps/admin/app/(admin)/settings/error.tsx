"use client";

import { ErrorState } from "@/components/layout/ErrorState";

export default function SettingsError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  return (
    <ErrorState
      title="We couldn't load these settings"
      description="Something went wrong while fetching your store configuration. Try again — most transient failures clear on retry."
      error={error}
      onRetry={reset}
      backHref="/dashboard"
      backLabel="Go to dashboard"
    />
  );
}
