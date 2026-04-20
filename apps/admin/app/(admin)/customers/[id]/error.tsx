"use client";

import { useEffect } from "react";
import { EditorialError } from "@repo/ui/editorial-error";

export default function Error({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  useEffect(() => {
    console.error("customer detail error", error);
  }, [error]);
  return (
    <main>
      <EditorialError
        title="Couldn't load this customer"
        description="Something went wrong on our side. Try again, or come back in a moment."
        onRetry={reset}
      />
    </main>
  );
}
