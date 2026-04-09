"use client";

// apps/admin/app/products/error.tsx
//
// Error boundary sibling to the products page. Next.js App Router picks
// this up automatically when a server component down-tree throws.

import { useEffect } from "react";

export default function Error({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  useEffect(() => {
    console.error("products page error", error);
  }, [error]);
  return (
    <main className="flex flex-col gap-4 px-8 py-16" aria-live="polite">
      <h1 className="font-[family-name:var(--font-serif,'Source_Serif_4',serif)] text-3xl text-[color:var(--ink-900)]">
        Couldn&apos;t load your products
      </h1>
      <p className="max-w-prose text-[color:var(--ink-900)] opacity-70">
        Something went wrong on our side. Try again, or come back in a moment.
      </p>
      <button
        type="button"
        onClick={() => reset()}
        className="inline-flex w-fit items-center gap-2 rounded-md bg-[color:var(--ink-900)] px-4 py-2 text-sm text-[color:var(--paper-200)] transition-opacity hover:opacity-90 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
      >
        Try again
      </button>
    </main>
  );
}
