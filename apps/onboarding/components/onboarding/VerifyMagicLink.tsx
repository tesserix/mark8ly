"use client";

import { useEffect, useRef, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";

import { useOnboardingStore } from "@/lib/store/onboarding-store";
import { verifyToken } from "@/app/onboarding/actions";

/**
 * VerifyMagicLink — rendered inside PostSubmitShell on /onboarding/verify.
 *
 * The shell already owns the page eyebrow + title + lede. This component
 * handles the token exchange and renders either a quiet loading state
 * (a single spinner + status line) or an error state (hairline + reason +
 * recovery CTA). No inner card chrome, no duplicate h1.
 *
 * Phase M: validates the magic-link token, marks the session verified
 * server-side, then redirects to /onboarding/set-password where the
 * merchant picks a credential.
 */
export function VerifyMagicLink() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const token = searchParams.get("token") ?? "";

  // Reset the in-tab store on success — best-effort cleanup. Harmless
  // if the store is already empty (cross-device case).
  const reset = useOnboardingStore((s) => s.reset);

  const [error, setError] = useState<string | null>(null);
  const ranRef = useRef(false);

  useEffect(() => {
    // StrictMode double-render guard. The token is single-use.
    if (ranRef.current) return;
    ranRef.current = true;

    if (!token) {
      setError("Missing verification token in the URL.");
      return;
    }

    (async () => {
      try {
        const r = await verifyToken(token);
        if (!r.ok) {
          setError(`${r.code}: ${r.message}`);
          return;
        }
        reset();
        router.push(
          `/onboarding/set-password?session=${encodeURIComponent(r.data.sessionId)}`,
        );
      } catch (err) {
        setError(err instanceof Error ? err.message : "Something went wrong.");
      }
    })();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  if (error) {
    return (
      <div className="border-t border-border-subtle pt-10">
        <p className="eyebrow mb-3">We couldn&rsquo;t verify the link</p>
        <p className="text-foreground-secondary leading-relaxed" aria-live="polite">
          {error}
        </p>
        <div className="mt-10 flex flex-wrap items-center gap-x-8 gap-y-4">
          <button
            type="button"
            onClick={() => {
              reset();
              router.push("/onboarding");
            }}
            className="inline-flex h-12 items-center rounded-md bg-primary px-6 text-base font-medium text-primary-foreground hover:bg-primary-hover"
          >
            Start over
          </button>
          <p className="text-sm text-foreground-tertiary">
            We&rsquo;ll send a fresh verification email.
          </p>
        </div>
      </div>
    );
  }

  return (
    <div className="border-t border-border-subtle pt-10">
      <div className="flex items-center gap-5">
        <div
          className="h-8 w-8 flex-shrink-0 rounded-full border-2 border-ink-200 border-t-foreground motion-safe:animate-spin"
          aria-hidden="true"
        />
        <div>
          <p className="eyebrow">Email confirmed</p>
          <p className="mt-1 font-serif text-lg text-foreground">
            Taking you to the final step…
          </p>
        </div>
      </div>
    </div>
  );
}
