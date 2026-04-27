// apps/onboarding/app/auth/google/page.tsx
"use client";

import { Suspense, useEffect, useState } from "react";
import { useSearchParams } from "next/navigation";
import { getGoogleCredential } from "@/lib/gip/google-gsi";
import {
  signInWithGoogleCustomer,
  CustomerGIPError,
} from "@/lib/gip/customer-signin";
import { mintCustomerExchangeCode } from "./actions";

/**
 * Inner component that uses useSearchParams — must be wrapped in <Suspense>.
 */
function TrampolineInner() {
  const params = useSearchParams();
  const [error, setError] = useState<string | null>(null);
  const [status, setStatus] = useState<"idle" | "popup" | "exchanging" | "redirecting">(
    "idle",
  );

  const returnTo = params.get("return_to") ?? "";
  const storeSlug = params.get("store_slug") ?? "";
  const intentParam = params.get("intent");
  const intent: "signin" | "signup" = intentParam === "signup" ? "signup" : "signin";

  useEffect(() => {
    if (!returnTo || !storeSlug) {
      setError("Missing required parameters.");
      return;
    }
    let cancelled = false;
    (async () => {
      try {
        setStatus("popup");
        const { credential } = await getGoogleCredential();
        if (cancelled) return;
        setStatus("exchanging");
        const result = await signInWithGoogleCustomer(credential);
        if (cancelled) return;
        if (result.kind === "needConfirmation") {
          setError(
            "This email already has an account. Phase 3 link prompt is not yet wired here — please sign in with email and password instead.",
          );
          return;
        }
        const exchange = await mintCustomerExchangeCode({
          idToken: result.idToken,
          storeSlug,
          returnTo,
          intent,
        });
        if (cancelled) return;
        if (!exchange.ok) {
          setError(exchange.error);
          return;
        }
        setStatus("redirecting");
        window.location.assign(exchange.redirectUrl);
      } catch (err) {
        if (cancelled) return;
        if (err instanceof CustomerGIPError) {
          setError(
            err.code === "config_missing"
              ? "Google sign-in is not available right now."
              : "Google sign-in failed. Please try again.",
          );
        } else {
          setError(err instanceof Error ? err.message : "Google sign-in failed.");
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [returnTo, storeSlug, intent]);

  return (
    <>
      <h1 className="font-serif text-2xl">
        Continuing to {storeSlug || "store"}&hellip;
      </h1>
      <p className="mt-3 text-sm opacity-70">
        {status === "popup" && "Opening Google sign-in…"}
        {status === "exchanging" && "Completing sign-in…"}
        {status === "redirecting" && "Returning you to the store…"}
        {status === "idle" && !error && "Preparing…"}
      </p>
      {error && (
        <p role="alert" className="mt-4 text-sm text-[color:var(--danger,#a3322a)]">
          {error}
        </p>
      )}
    </>
  );
}

/**
 * /auth/google — the cross-tenant trampoline for customer Google sign-in.
 *
 * Per-tenant storefront subdomains (e.g. india-store.mark8ly.com) cannot
 * register with Google's OAuth client config (no wildcard origins, no
 * stable Admin API). So storefronts redirect customers to this page on
 * mark8ly.com (a fixed registered origin), we run the Google popup
 * here, exchange the credential for a GIP id_token in the MP-Customer
 * pool, mint a short-lived HMAC exchange code, and bounce back to the
 * originating store's /auth/google/finish route which mints the
 * per-host mp_customer_session cookie via the existing customerSignIn
 * server action.
 */
export default function GoogleAuthTrampolinePage() {
  return (
    <main className="mx-auto flex min-h-screen max-w-md flex-col items-center justify-center px-6 py-16 text-center">
      <Suspense
        fallback={
          <p className="text-sm opacity-70">Preparing&hellip;</p>
        }
      >
        <TrampolineInner />
      </Suspense>
    </main>
  );
}
