"use client";

import { useEffect, useRef, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";

import { useOnboardingStore } from "@/lib/store/onboarding-store";
import { refreshIdToken } from "@/lib/gip/signup";
import { verifyAndLogin } from "@/app/onboarding/actions";

/**
 * Returns true once zustand's persist middleware has finished rehydrating
 * the onboarding store from sessionStorage. The hook exists because the
 * verify page is the magic-link landing target — when the user clicks
 * the link from their email it opens a fresh tab with an empty
 * in-memory store, and the verify effect must NOT bail out before the
 * hydrated values are available.
 */
function useOnboardingStoreHasHydrated(): boolean {
  const [hydrated, setHydrated] = useState(
    () => useOnboardingStore.persist.hasHydrated(),
  );
  useEffect(() => {
    const unsub = useOnboardingStore.persist.onFinishHydration(() =>
      setHydrated(true),
    );
    setHydrated(useOnboardingStore.persist.hasHydrated());
    return unsub;
  }, []);
  return hydrated;
}

/**
 * VerifyMagicLink is the magic link target.
 *
 * On mount it:
 *   1. Reads the token from the URL
 *   2. Reads the saved form fields from the onboarding store
 *   3. Refreshes the GIP id_token client-side (refresh token survived in store)
 *   4. Calls verifyAndLogin server action with everything
 *   5. Redirects to /welcome on success
 *
 * The user sees a single "Setting up your store…" spinner for the duration.
 * Total time: ~1-2s (token verify + tenant create + outbox + autologin retry).
 */
export function VerifyMagicLink() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const token = searchParams.get("token") ?? "";

  const hasHydrated = useOnboardingStoreHasHydrated();
  const businessName = useOnboardingStore((s) => s.businessName);
  const slug = useOnboardingStore((s) => s.slug);
  const countryCode = useOnboardingStore((s) => s.countryCode);
  const currencyCode = useOnboardingStore((s) => s.currencyCode);
  const timezone = useOnboardingStore((s) => s.timezone);
  const gipUid = useOnboardingStore((s) => s.gipUid);
  const gipRefreshToken = useOnboardingStore((s) => s.gipRefreshToken);
  const reset = useOnboardingStore((s) => s.reset);

  const [error, setError] = useState<string | null>(null);
  const ranRef = useRef(false);

  useEffect(() => {
    // Wait for zustand persist to finish rehydrating sessionStorage. Without
    // this guard, opening the magic link in a fresh tab races: the effect
    // fires before the store is populated and bails with "no session."
    if (!hasHydrated) return;
    // StrictMode double-render guard. We must run exactly once because the
    // token is single-use — running twice would consume it and the second
    // call would fail with "token already consumed."
    if (ranRef.current) return;
    ranRef.current = true;

    if (!token) {
      setError("Missing verification token in the URL");
      return;
    }
    if (!gipRefreshToken || !slug) {
      setError(
        "We couldn't find your in-progress onboarding session. Please start again.",
      );
      return;
    }

    (async () => {
      try {
        // Step 1: refresh the GIP id_token using the stored refresh token.
        // The original id_token from form-submit time may have expired if
        // the user took >1h to click the link.
        const fresh = await refreshIdToken(gipRefreshToken);

        // Step 2: server action does verify-token + complete + autologin
        // and forwards the session cookie via next/headers.
        const r = await verifyAndLogin({
          token,
          businessName,
          slug,
          countryCode,
          currencyCode,
          timezone,
          gipUid,
          gipIdToken: fresh.idToken,
        });

        if (!r.ok) {
          setError(`${r.code}: ${r.message}`);
          return;
        }

        // Step 3: clean up the session-stored form data; redirect.
        reset();
        router.push("/welcome");
      } catch (err) {
        setError(
          err instanceof Error ? err.message : "Something went wrong",
        );
      }
    })();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [hasHydrated]);

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
