"use client";

import { useEffect, useState, Suspense } from "react";
import { useSearchParams } from "next/navigation";
import { LinkProviderPrompt } from "@repo/ui/auth/link-provider-prompt";
import { getGoogleCredential } from "@/lib/gip/google-gsi";
import {
  signInWithGoogleCustomer,
  CustomerGIPError,
} from "@/lib/gip/customer-signin";
import { linkGoogleToCustomerPassword } from "@/lib/gip/customer-link";
import { mintCustomerExchangeCode } from "./actions";

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
 *
 * Phase 3: when GIP returns needConfirmation (existing email already
 * has password but is now signing in with Google), render the
 * LinkProviderPrompt overlay. User enters their existing password,
 * we link the providers via signInWithPassword + signInWithIdp, then
 * continue with the linked id_token.
 */
function TrampolineInner() {
  const params = useSearchParams();
  const [error, setError] = useState<string | null>(null);
  const [linkPromptError, setLinkPromptError] = useState<string | null>(null);
  const [status, setStatus] = useState<
    "idle" | "popup" | "exchanging" | "redirecting"
  >("idle");
  const [needConfirmation, setNeedConfirmation] = useState<{
    email: string;
    pendingIdpCredential: string;
  } | null>(null);

  const returnTo = params.get("return_to") ?? "";
  const storeSlug = params.get("store_slug") ?? "";
  const intentParam = params.get("intent");
  const intent: "signin" | "signup" | "link" =
    intentParam === "signup"
      ? "signup"
      : intentParam === "link"
        ? "link"
        : "signin";

  async function completeAndRedirect(idToken: string): Promise<void> {
    const exchange = await mintCustomerExchangeCode({
      idToken,
      storeSlug,
      returnTo,
      // The exchange-code action only knows signin/signup; link reuses
      // signin since the cookie mint is identical.
      intent: intent === "link" ? "signin" : intent,
    });
    if (!exchange.ok) {
      setError(exchange.error);
      return;
    }
    setStatus("redirecting");
    window.location.assign(exchange.redirectUrl);
  }

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
          setNeedConfirmation({
            email: result.email,
            pendingIdpCredential: result.pendingIdpCredential,
          });
          setStatus("idle");
          return;
        }
        await completeAndRedirect(result.idToken);
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
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [returnTo, storeSlug, intent]);

  async function handleLinkConfirm(password: string): Promise<void> {
    if (!needConfirmation) return;
    setLinkPromptError(null);
    try {
      const linked = await linkGoogleToCustomerPassword(
        needConfirmation.email,
        password,
        needConfirmation.pendingIdpCredential,
      );
      setNeedConfirmation(null);
      setStatus("exchanging");
      await completeAndRedirect(linked.idToken);
    } catch (err) {
      if (err instanceof CustomerGIPError && err.code === "invalid_credentials") {
        setLinkPromptError("That password is incorrect. Please try again.");
        return;
      }
      setLinkPromptError(
        err instanceof Error
          ? err.message
          : "Could not link Google. Please try again.",
      );
    }
  }

  function handleLinkCancel(): void {
    setNeedConfirmation(null);
    setError(
      "Linking cancelled. Please sign in with email and password instead.",
    );
  }

  return (
    <main className="mx-auto flex min-h-screen max-w-md flex-col items-center justify-center px-6 py-16 text-center">
      <h1 className="font-serif text-2xl">
        Continuing to {storeSlug || "store"}&hellip;
      </h1>
      <p className="mt-3 text-sm opacity-70">
        {status === "popup" && "Opening Google sign-in…"}
        {status === "exchanging" && "Completing sign-in…"}
        {status === "redirecting" && "Returning you to the store…"}
        {status === "idle" && !error && !needConfirmation && "Preparing…"}
      </p>
      {error && (
        <p
          role="alert"
          className="mt-4 text-sm text-[color:var(--danger,#a3322a)]"
        >
          {error}
        </p>
      )}
      {needConfirmation && (
        <LinkProviderPrompt
          email={needConfirmation.email}
          variant="storefront"
          error={linkPromptError}
          onConfirm={handleLinkConfirm}
          onCancel={handleLinkCancel}
        />
      )}
    </main>
  );
}

export default function GoogleAuthTrampolinePage() {
  return (
    <Suspense fallback={null}>
      <TrampolineInner />
    </Suspense>
  );
}
