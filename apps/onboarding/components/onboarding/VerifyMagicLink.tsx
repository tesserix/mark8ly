"use client";

import { useEffect, useRef, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";

import { useOnboardingStore } from "@/lib/store/onboarding-store";
import { verifyToken } from "@/app/onboarding/actions";

/**
 * VerifyMagicLink is the magic link target.
 *
 * Phase M: this page now ONLY validates the magic-link token and marks
 * the session verified server-side. On success it redirects to
 * /onboarding/set-password where the merchant picks a credential
 * (password or Google) — that page completes the tenant + mints the
 * session cookie.
 *
 * The split exists so the credential-picker UX can run AFTER email
 * verification, which means a Google-signup user never has to read
 * the magic-link email.
 *
 * The onboarding store still gets reset on success because we want
 * the same browser to forget the in-progress fields if it does happen
 * to be the original tab.
 */
export function VerifyMagicLink() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const token = searchParams.get("token") ?? "";

  // Reset the in-tab store on success — best-effort cleanup. It's
  // harmless if the store is already empty (cross-device case).
  const reset = useOnboardingStore((s) => s.reset);

  const [error, setError] = useState<string | null>(null);
  const ranRef = useRef(false);

  useEffect(() => {
    // StrictMode double-render guard. The token is single-use — running
    // twice would consume it and the second call would fail with
    // "token already consumed."
    if (ranRef.current) return;
    ranRef.current = true;

    if (!token) {
      setError("Missing verification token in the URL");
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
        setError(
          err instanceof Error ? err.message : "Something went wrong",
        );
      }
    })();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  if (error) {
    return (
      <div className="w-full max-w-xl mx-auto overflow-hidden rounded-[2rem] border border-warm-200/90 bg-white/90 shadow-[0_24px_80px_rgba(43,38,34,0.12)] backdrop-blur-sm">
        <div className="px-8 py-8 text-center">
          <div className="mx-auto flex h-16 w-16 items-center justify-center rounded-full border border-terracotta-200 bg-terracotta-50 text-3xl shadow-sm" aria-hidden>
            !
          </div>
          <h1 className="mt-5 font-serif text-3xl font-medium text-foreground">
            We couldn&apos;t verify your link
          </h1>
          <p className="mt-3 text-sm leading-6 text-foreground-secondary" aria-live="polite">
            {error}
          </p>
        </div>
        <div className="border-t border-warm-100 px-8 py-6 text-center">
          <p className="text-sm text-foreground-secondary">
            Start again and we&apos;ll send a fresh verification email.
          </p>
        <button
          onClick={() => {
            reset();
            router.push("/onboarding");
          }}
          className="mt-5 inline-flex items-center justify-center rounded-full bg-primary px-5 py-2.5 text-sm font-medium text-primary-foreground transition-[background-color,box-shadow] hover:bg-primary-hover hover:shadow-lg"
        >
          Start over
        </button>
        </div>
      </div>
    );
  }

  return (
    <div className="w-full max-w-xl mx-auto overflow-hidden rounded-[2rem] border border-warm-200/90 bg-white/90 shadow-[0_24px_80px_rgba(43,38,34,0.12)] backdrop-blur-sm">
      <div className="px-8 py-10 text-center">
        <div className="mx-auto flex h-16 w-16 items-center justify-center rounded-full border border-warm-200 bg-warm-50 shadow-sm">
          <div className="h-8 w-8 rounded-full border-2 border-warm-300 border-t-foreground motion-safe:animate-spin" aria-hidden />
        </div>
        <p className="mt-5 text-xs font-medium uppercase tracking-[0.16em] text-foreground-tertiary">
          Finalizing setup
        </p>
        <h2 className="mt-2 font-serif text-3xl font-medium text-foreground">
          Setting up your store…
        </h2>
        <p className="mt-3 text-sm leading-6 text-foreground-secondary">
          We&apos;re preparing your workspace and signing you in. This usually
          takes a couple of seconds.
        </p>
      </div>
    </div>
  );
}
