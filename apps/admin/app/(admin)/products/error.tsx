"use client";

// apps/admin/app/products/error.tsx
//
// Error boundary sibling to the products page. Next.js App Router picks
// this up automatically when a server component down-tree throws.
// Content is rendered via the shared EditorialError from @repo/ui so
// every admin section shows the same shape when its page errors.

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
    // Logged to the browser console so Playwright traces capture the
    // stack. Production logging is handled by Next's own error reporter.
    console.error("products page error", error);
  }, [error]);
  return (
    <main>
      <EditorialError
        title="Couldn't load your products"
        description="Something went wrong on our side. Try again, or come back in a moment."
        onRetry={reset}
      />
    </main>
  );
}
