"use client";

import { useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import {
  customerSignIn,
  confirmCustomerTotp,
  isTotpRequiredResult,
} from "@/app/sign-in/actions";

const TRAMPOLINE_BASE =
  process.env.NEXT_PUBLIC_MARK8LY_AUTH_URL ?? "https://mark8ly.com";

// Matches apps/storefront/app/sign-in/actions.ts's AUTH_PROVIDER rule
// exactly (see that file for the full rationale): only the literal string
// "zitadel" switches this form off the GIP/Identity Toolkit path. Both
// reads must agree, since the server action rejects a Zitadel-shaped
// payload sent while the flag says GIP and vice versa.
const AUTH_PROVIDER: "gip" | "zitadel" =
  process.env.NEXT_PUBLIC_AUTH_PROVIDER === "zitadel" ? "zitadel" : "gip";

interface GipConfig {
  apiKey: string;
  tenantId: string;
  projectId: string;
}

interface CustomerSignInFormProps {
  gipConfig: GipConfig;
  storeSlug: string;
  returnUrl: string;
}

class GIPError extends Error {
  constructor(
    public code: string,
    message: string,
  ) {
    super(message);
  }
}

async function signInWithPassword(
  email: string,
  password: string,
  config: GipConfig,
): Promise<{ uid: string; idToken: string }> {
  if (!config.apiKey || !config.tenantId) {
    throw new GIPError(
      "config_missing",
      "Sign-in is not configured for this store yet.",
    );
  }

  const url = `https://identitytoolkit.googleapis.com/v1/accounts:signInWithPassword?key=${config.apiKey}`;
  const res = await fetch(url, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      email,
      password,
      returnSecureToken: true,
      tenantId: config.tenantId,
    }),
  });

  if (!res.ok) {
    const body = (await res.json().catch(() => null)) as {
      error?: { message?: string };
    } | null;
    const code = body?.error?.message ?? "UNKNOWN";
    const friendly: Record<string, string> = {
      EMAIL_NOT_FOUND: "No account found with this email.",
      INVALID_PASSWORD: "Incorrect password.",
      INVALID_LOGIN_CREDENTIALS: "Email or password is incorrect.",
      USER_DISABLED: "This account has been disabled.",
      TOO_MANY_ATTEMPTS_TRY_LATER:
        "Too many attempts. Please wait a moment and try again.",
    };
    throw new GIPError(code, friendly[code] ?? "Sign-in failed. Please try again.");
  }

  const data = (await res.json()) as {
    localId: string;
    idToken: string;
  };
  return { uid: data.localId, idToken: data.idToken };
}

export function CustomerSignInForm({
  gipConfig,
  storeSlug,
  returnUrl,
}: CustomerSignInFormProps) {
  const router = useRouter();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [pending, startTransition] = useTransition();

  // Zitadel's own TOTP step-up: verifyCustomerCredential returned
  // "totp_required" instead of completing sign-in. sessionId/sessionToken
  // must be carried unchanged into confirmCustomerTotp — see the comment
  // on that action for why (only auth-bff holds the PAT that could mint
  // the session server-side instead). Unreachable under GIP: that path
  // never yields a totp_required outcome.
  const [totpChallenge, setTotpChallenge] = useState<{
    sessionId: string;
    sessionToken: string;
  } | null>(null);
  const [totpCode, setTotpCode] = useState("");
  const [totpError, setTotpError] = useState<string | null>(null);
  const [totpPending, startTotpTransition] = useTransition();

  function handleGoogle() {
    if (typeof window === "undefined") return;
    const url = new URL("/auth/google", TRAMPOLINE_BASE);
    url.searchParams.set("return_to", `${window.location.origin}/account`);
    url.searchParams.set("store_slug", storeSlug);
    url.searchParams.set("intent", "signin");
    window.location.assign(url.toString());
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);

    if (!email.trim()) {
      setError("Email is required.");
      return;
    }
    if (!password) {
      setError("Password is required.");
      return;
    }

    startTransition(async () => {
      try {
        let result: Awaited<ReturnType<typeof customerSignIn>>;

        if (AUTH_PROVIDER === "zitadel") {
          // Under Zitadel the browser never talks to Identity Toolkit —
          // the password goes straight to the server action, which calls
          // auth-bff's storefront-customer credential endpoint itself.
          result = await customerSignIn({
            loginName: email.trim(),
            password,
            storeSlug,
          });
        } else {
          const gipResult = await signInWithPassword(
            email.trim(),
            password,
            gipConfig,
          );

          result = await customerSignIn({
            idToken: gipResult.idToken,
            uid: gipResult.uid,
            storeSlug,
          });
        }

        if (!result.ok) {
          if (isTotpRequiredResult(result)) {
            setTotpChallenge({
              sessionId: result.sessionId,
              sessionToken: result.sessionToken,
            });
            return;
          }
          setError(result.message);
          return;
        }

        router.push(returnUrl);
        router.refresh();
      } catch (err) {
        if (err instanceof GIPError) {
          setError(err.message);
        } else {
          setError("Something went wrong. Please try again.");
        }
      }
    });
  }

  function handleTotpSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!totpChallenge) return;
    setTotpError(null);

    startTotpTransition(async () => {
      const result = await confirmCustomerTotp({
        storeSlug,
        sessionId: totpChallenge.sessionId,
        sessionToken: totpChallenge.sessionToken,
        code: totpCode,
      });

      if (!result.ok) {
        if (isTotpRequiredResult(result)) {
          // A FRESH challenge, not a wrong code — Zitadel handed back a
          // new sessionId/sessionToken pair. Update the held challenge
          // so the next submission carries the new credentials instead
          // of the stale ones the customer just used; retrying with the
          // old pair could never succeed.
          setTotpChallenge({
            sessionId: result.sessionId,
            sessionToken: result.sessionToken,
          });
          setTotpCode("");
          setTotpError(result.message);
          return;
        }
        setTotpError(result.message);
        return;
      }

      router.push(returnUrl);
      router.refresh();
    });
  }

  function cancelTotp() {
    setTotpChallenge(null);
    setTotpCode("");
    setTotpError(null);
  }

  if (totpChallenge) {
    return (
      <form onSubmit={handleTotpSubmit} noValidate className="mt-8 space-y-5">
        <p className="text-sm text-[color:var(--storefront-text,var(--ink-900))] opacity-70">
          Enter the 6-digit code from your authenticator app to finish
          signing in.
        </p>

        <div className="space-y-1.5">
          <label
            htmlFor="customer-totp-code"
            className="block text-sm font-medium text-[color:var(--storefront-text,var(--ink-900))]"
          >
            Verification code
          </label>
          <input
            id="customer-totp-code"
            type="text"
            inputMode="numeric"
            autoComplete="one-time-code"
            maxLength={6}
            placeholder="000000"
            value={totpCode}
            onChange={(e) => setTotpCode(e.target.value.replace(/\D/g, ""))}
            disabled={totpPending}
            autoFocus
            aria-invalid={totpError ? true : undefined}
            aria-describedby={totpError ? "customer-totp-error" : undefined}
            className="w-full rounded-md border border-[color:var(--storefront-text,var(--ink-900))]/20 bg-[color:var(--storefront-surface)] px-3 py-2.5 text-base font-mono tracking-[0.4em] text-[color:var(--storefront-text,var(--ink-900))] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))]"
          />
        </div>

        {totpError && (
          <p
            id="customer-totp-error"
            role="alert"
            aria-live="polite"
            className="text-sm text-[color:var(--storefront-danger)]"
          >
            {totpError}
          </p>
        )}

        <button
          type="submit"
          disabled={totpPending || totpCode.length !== 6}
          className="w-full rounded-md bg-[color:var(--storefront-accent,var(--ink-900))] px-6 py-3 text-sm font-medium text-[color:var(--storefront-on-accent,var(--paper-200))] transition-opacity hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))]"
        >
          {totpPending ? "Verifying..." : "Verify and continue"}
        </button>

        <button
          type="button"
          onClick={cancelTotp}
          className="inline-flex h-11 w-full items-center justify-center text-sm text-[color:var(--storefront-text,var(--ink-900))] opacity-70 underline underline-offset-4 hover:opacity-100"
        >
          Back to sign in
        </button>
      </form>
    );
  }

  return (
    <form onSubmit={handleSubmit} noValidate className="mt-8 space-y-5">
      <div className="space-y-1.5">
        <label
          htmlFor="customer-email"
          className="block text-sm font-medium text-[color:var(--storefront-text,var(--ink-900))]"
        >
          Email address
        </label>
        <input
          id="customer-email"
          type="email"
          autoComplete="email"
          spellCheck={false}
          value={email}
          onChange={(e) => setEmail(e.target.value)}
          className="w-full rounded-md border border-[color:var(--storefront-text,var(--ink-900))]/20 bg-[color:var(--storefront-surface)] px-3 py-2.5 text-base text-[color:var(--storefront-text,var(--ink-900))] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))]"
        />
      </div>

      <div className="space-y-1.5">
        <label
          htmlFor="customer-password"
          className="block text-sm font-medium text-[color:var(--storefront-text,var(--ink-900))]"
        >
          Password
        </label>
        <input
          id="customer-password"
          type="password"
          autoComplete="current-password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          className="w-full rounded-md border border-[color:var(--storefront-text,var(--ink-900))]/20 bg-[color:var(--storefront-surface)] px-3 py-2.5 text-base text-[color:var(--storefront-text,var(--ink-900))] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))]"
        />
      </div>

      {error && (
        <p role="alert" className="text-sm text-[color:var(--storefront-danger)]">
          {error}
        </p>
      )}

      <button
        type="submit"
        disabled={pending}
        className="w-full rounded-md bg-[color:var(--storefront-accent,var(--ink-900))] px-6 py-3 text-sm font-medium text-[color:var(--storefront-on-accent,var(--paper-200))] transition-opacity hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))]"
      >
        {pending ? "Signing in..." : "Sign in"}
      </button>

      {AUTH_PROVIDER === "gip" && (
        <>
          <div className="relative py-1">
            <div className="absolute inset-0 flex items-center" aria-hidden="true">
              <div className="w-full border-t border-[color:var(--storefront-text,var(--ink-900))]/15" />
            </div>
            <div className="relative flex justify-center">
              <span className="bg-[color:var(--storefront-background,var(--paper-200))] px-3 text-xs uppercase tracking-wider text-[color:var(--storefront-text,var(--ink-900))]/55">
                or
              </span>
            </div>
          </div>

          <button
            type="button"
            onClick={handleGoogle}
            disabled={pending}
            className="inline-flex h-11 w-full items-center justify-center gap-3 rounded-md border border-[color:var(--storefront-text,var(--ink-900))]/20 bg-[color:var(--storefront-surface)] px-6 text-sm font-medium text-[color:var(--storefront-text,var(--ink-900))] transition-colors hover:border-[color:var(--storefront-text,var(--ink-900))]/40 disabled:cursor-not-allowed disabled:opacity-50"
          >
            <svg width="18" height="18" viewBox="0 0 18 18" aria-hidden="true">
              <path d="M17.64 9.205c0-.638-.057-1.252-.164-1.841H9v3.481h4.844a4.14 4.14 0 0 1-1.796 2.716v2.259h2.908c1.702-1.567 2.684-3.875 2.684-6.615z" fill="#4285F4"/>
              <path d="M9 18c2.43 0 4.467-.806 5.956-2.18l-2.908-2.259c-.806.54-1.837.86-3.048.86-2.344 0-4.328-1.584-5.036-3.711H.957v2.332A8.997 8.997 0 0 0 9 18z" fill="#34A853"/>
              <path d="M3.964 10.71A5.41 5.41 0 0 1 3.682 9c0-.593.102-1.17.282-1.71V4.958H.957A8.996 8.996 0 0 0 0 9c0 1.452.348 2.827.957 4.042l3.007-2.332z" fill="#FBBC05"/>
              <path d="M9 3.58c1.321 0 2.508.454 3.44 1.345l2.582-2.58C13.463.891 11.426 0 9 0A8.997 8.997 0 0 0 .957 4.958L3.964 7.29C4.672 5.163 6.656 3.58 9 3.58z" fill="#EA4335"/>
            </svg>
            Continue with Google
          </button>
        </>
      )}

      <p className="text-center text-xs text-[color:var(--storefront-text,var(--ink-900))] opacity-60">
        Don&apos;t have an account?{" "}
        <Link
          href="/create-account"
          className="text-[color:var(--storefront-accent,var(--moss-700))] underline underline-offset-4"
        >
          Create one
        </Link>
      </p>
    </form>
  );
}
