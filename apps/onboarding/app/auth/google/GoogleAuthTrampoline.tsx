"use client";

import { useCallback, useEffect, useRef, useState, Suspense } from "react";
import { useSearchParams } from "next/navigation";
import { LinkProviderPrompt } from "@repo/ui/auth/link-provider-prompt";
import { mountGoogleButton } from "@/lib/gip/google-gsi";
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
 * mark8ly.com (a fixed registered origin), we render the official Google
 * sign-in button here, exchange the credential for a GIP id_token in the
 * MP-Customer pool, mint a short-lived HMAC exchange code, and bounce
 * back to the originating store's /auth/google/finish route which mints
 * the per-host mp_customer_session cookie via the existing customerSignIn
 * server action.
 *
 * We render the GSI button (rather than relying on One-Tap) because
 * FedCM-era browsers may silently dismiss `gsi.prompt()` with
 * "unknown_reason", which would dead-end this page with no recoverable UI.
 * One-Tap is still fired passively as a convenience for users with an
 * active Google session.
 *
 * Phase 3: when GIP returns needConfirmation (existing email already
 * has password but is now signing in with Google), render the
 * LinkProviderPrompt overlay. User enters their existing password,
 * we link the providers via signInWithPassword + signInWithIdp, then
 * continue with the linked id_token.
 */
function TrampolineInner() {
  const params = useSearchParams();
  const buttonRef = useRef<HTMLDivElement | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [linkPromptError, setLinkPromptError] = useState<string | null>(null);
  const [status, setStatus] = useState<
    "idle" | "ready" | "exchanging" | "redirecting"
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

  const completeAndRedirect = useCallback(
    async (idToken: string): Promise<void> => {
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
    },
    [storeSlug, returnTo, intent],
  );

  const handleCredential = useCallback(
    async (credential: string): Promise<void> => {
      setError(null);
      setStatus("exchanging");
      try {
        const result = await signInWithGoogleCustomer(credential);
        if (result.kind === "needConfirmation") {
          setNeedConfirmation({
            email: result.email,
            pendingIdpCredential: result.pendingIdpCredential,
          });
          setStatus("ready");
          return;
        }
        await completeAndRedirect(result.idToken);
      } catch (err) {
        if (err instanceof CustomerGIPError) {
          setError(
            err.code === "config_missing"
              ? "Google sign-in is not available right now."
              : "Google sign-in failed. Please try again.",
          );
        } else {
          setError(
            err instanceof Error ? err.message : "Google sign-in failed.",
          );
        }
        setStatus("ready");
      }
    },
    [completeAndRedirect],
  );

  useEffect(() => {
    if (!returnTo || !storeSlug) {
      setError("Missing required parameters.");
      return;
    }
    const container = buttonRef.current;
    if (!container) return;
    let cancelled = false;
    let teardown: (() => void) | null = null;
    (async () => {
      try {
        const cleanup = await mountGoogleButton({
          buttonContainer: container,
          onCredential: (cred) => {
            if (cancelled) return;
            void handleCredential(cred);
          },
          onError: (err) => {
            if (cancelled) return;
            setError(err.message || "Google sign-in failed.");
          },
          buttonText: intent === "signup" ? "signup_with" : "continue_with",
          tryOneTap: true,
        });
        if (cancelled) {
          cleanup();
          return;
        }
        teardown = cleanup;
        setStatus("ready");
      } catch (err) {
        if (cancelled) return;
        setError(
          err instanceof Error ? err.message : "Google sign-in failed.",
        );
      }
    })();
    return () => {
      cancelled = true;
      teardown?.();
    };
  }, [returnTo, storeSlug, intent, handleCredential]);

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
        {status === "idle" && !error && "Preparing…"}
        {status === "ready" &&
          !error &&
          !needConfirmation &&
          "Click the button below to continue with Google."}
        {status === "exchanging" && "Completing sign-in…"}
        {status === "redirecting" && "Returning you to the store…"}
      </p>
      <div ref={buttonRef} className="mt-6 flex justify-center" />
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

export function GoogleAuthTrampoline() {
  return (
    <Suspense fallback={null}>
      <TrampolineInner />
    </Suspense>
  );
}
