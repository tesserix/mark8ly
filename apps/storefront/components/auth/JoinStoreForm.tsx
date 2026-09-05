"use client";

import { useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { joinThisStore } from "@/app/join/actions";

interface JoinStoreFormProps {
  storeName: string;
  returnUrl: string;
}

/**
 * The consent step for joining a store. Deliberately has no inputs — the
 * identity comes from the server-signed join grant, never from anything
 * this component could send.
 */
export function JoinStoreForm({ storeName, returnUrl }: JoinStoreFormProps) {
  const router = useRouter();
  const [error, setError] = useState<string | null>(null);
  const [pending, startTransition] = useTransition();

  function handleJoin() {
    setError(null);
    startTransition(async () => {
      const result = await joinThisStore();
      if (!result.ok) {
        setError(result.message);
        return;
      }
      router.push(returnUrl);
      router.refresh();
    });
  }

  return (
    <div className="mt-8 space-y-5">
      {error && (
        <p
          role="alert"
          aria-live="polite"
          className="text-sm text-[color:var(--storefront-danger)]"
        >
          {error}
        </p>
      )}

      <button
        type="button"
        onClick={handleJoin}
        disabled={pending}
        className="w-full rounded-md bg-[color:var(--storefront-accent,var(--ink-900))] px-6 py-3 text-sm font-medium text-[color:var(--storefront-on-accent,var(--paper-200))] transition-opacity hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))]"
      >
        {pending ? "Joining..." : `Join ${storeName}`}
      </button>

      <p className="text-center text-xs text-[color:var(--storefront-text,var(--ink-900))] opacity-60">
        Changed your mind?{" "}
        <Link
          href="/"
          className="text-[color:var(--storefront-accent,var(--moss-700))] underline underline-offset-4"
        >
          Keep browsing
        </Link>
      </p>
    </div>
  );
}
