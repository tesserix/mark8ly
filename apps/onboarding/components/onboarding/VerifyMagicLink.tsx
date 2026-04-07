"use client";

import { useEffect, useRef, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";

import { useOnboardingStore } from "@/lib/store/onboarding-store";
import { refreshIdToken } from "@/lib/gip/signup";
import { verifyAndLogin } from "@/app/onboarding/actions";

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
  }, []);

  if (error) {
    return (
      <div className="w-full max-w-md mx-auto bg-white dark:bg-zinc-900 rounded-2xl shadow-sm border border-zinc-200 dark:border-zinc-800 p-8 text-center">
        <div className="text-5xl mb-4" aria-hidden>⚠️</div>
        <h1 className="text-xl font-semibold">We couldn't verify your link</h1>
        <p className="mt-3 text-sm text-zinc-600 dark:text-zinc-400">{error}</p>
        <button
          onClick={() => {
            reset();
            router.push("/onboarding");
          }}
          className="mt-6 text-sm font-medium underline text-zinc-900 dark:text-white"
        >
          Start over
        </button>
      </div>
    );
  }

  return (
    <div className="w-full max-w-md mx-auto bg-white dark:bg-zinc-900 rounded-2xl shadow-sm border border-zinc-200 dark:border-zinc-800 p-8 text-center">
      <div className="inline-block w-12 h-12 border-4 border-zinc-200 border-t-zinc-900 dark:border-zinc-800 dark:border-t-white rounded-full animate-spin" />
      <h2 className="mt-6 text-lg font-medium">Setting up your store…</h2>
      <p className="mt-2 text-sm text-zinc-500">
        This usually takes a couple of seconds.
      </p>
    </div>
  );
}
