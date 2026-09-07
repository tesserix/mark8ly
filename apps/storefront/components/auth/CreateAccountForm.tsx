"use client";

import { useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { registerCustomer, verifyCustomerEmail } from "@/app/create-account/actions";
import { resolveGoogleSignInUrl } from "@/lib/auth/google-sign-in";
import {
  applyRegisterResult,
  applyVerifyResult,
  INITIAL_CREATE_ACCOUNT_STATE,
} from "@/lib/auth/create-account-flow";

interface CreateAccountFormProps {
  storeSlug: string;
  returnUrl: string;
}

export function CreateAccountForm({
  storeSlug,
  returnUrl,
}: CreateAccountFormProps) {
  const router = useRouter();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [code, setCode] = useState("");
  // Which step of register -> "check your email" -> verify the form is
  // on, plus the current error.
  const [flow, setFlow] = useState(INITIAL_CREATE_ACCOUNT_STATE);
  const [pending, startTransition] = useTransition();
  const error = flow.error;

  function setError(message: string | null) {
    setFlow((prev) => ({ ...prev, error: message }));
  }

  function handleGoogle() {
    if (typeof window === "undefined") return;
    startTransition(async () => {
      const result = await resolveGoogleSignInUrl({
        storeSlug,
        intent: "signup",
        dest: "/account",
        origin: window.location.origin,
      });
      if (!result.ok) {
        setError(result.message);
        return;
      }
      window.location.assign(result.url);
    });
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);

    if (!email.trim()) {
      setError("Email is required.");
      return;
    }
    if (!password || password.length < 6) {
      setError("Password must be at least 6 characters.");
      return;
    }

    // Step 1 of register -> "check your email" -> verify.
    startTransition(async () => {
      const result = await registerCustomer({ email: email.trim(), password });
      setFlow(applyRegisterResult(result));
    });
  }

  function handleVerifySubmit(e: React.FormEvent) {
    e.preventDefault();
    if (flow.step.kind !== "verify") return;
    const verifyStep = flow.step;

    if (!code.trim()) {
      setError("Enter the code from your verification email.");
      return;
    }

    startTransition(async () => {
      const result = await verifyCustomerEmail({
        uid: verifyStep.uid,
        email: verifyStep.email,
        token: verifyStep.token,
        code: code.trim(),
        storeSlug,
      });

      if (!result.ok) {
        // A wrong/expired code (or any other failure) keeps the shopper on
        // this SAME verify step — applyVerifyResult never sends them back
        // to the registration form.
        setFlow(applyVerifyResult(verifyStep, result));
        return;
      }

      router.push(returnUrl);
      router.refresh();
    });
  }

  if (flow.step.kind === "verify") {
    return (
      <form onSubmit={handleVerifySubmit} noValidate className="mt-8 space-y-5">
        <p className="text-sm text-[color:var(--storefront-text,var(--ink-900))]/80">
          We sent a verification code to <strong>{flow.step.email}</strong>.
          Enter it below to finish creating your account — this also
          confirms the address for Google sign-in later.
        </p>

        <div className="space-y-1.5">
          <label
            htmlFor="signup-verify-code"
            className="block text-sm font-medium text-[color:var(--storefront-text,var(--ink-900))]"
          >
            Verification code
          </label>
          <input
            id="signup-verify-code"
            type="text"
            inputMode="text"
            autoComplete="one-time-code"
            value={code}
            onChange={(e) => setCode(e.target.value)}
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
          {pending ? "Verifying..." : "Verify email"}
        </button>
      </form>
    );
  }

  return (
    <form onSubmit={handleSubmit} noValidate className="mt-8 space-y-5">
      <div className="space-y-1.5">
        <label
          htmlFor="signup-email"
          className="block text-sm font-medium text-[color:var(--storefront-text,var(--ink-900))]"
        >
          Email address
        </label>
        <input
          id="signup-email"
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
          htmlFor="signup-password"
          className="block text-sm font-medium text-[color:var(--storefront-text,var(--ink-900))]"
        >
          Password
        </label>
        <input
          id="signup-password"
          type="password"
          autoComplete="new-password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          className="w-full rounded-md border border-[color:var(--storefront-text,var(--ink-900))]/20 bg-[color:var(--storefront-surface)] px-3 py-2.5 text-base text-[color:var(--storefront-text,var(--ink-900))] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))]"
        />
        <p className="text-xs text-[color:var(--storefront-text,var(--ink-900))] opacity-50">
          At least 6 characters.
        </p>
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
        {pending ? "Creating account..." : "Create account"}
      </button>

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

      <p className="text-center text-xs text-[color:var(--storefront-text,var(--ink-900))] opacity-60">
        Already have an account?{" "}
        <Link
          href="/sign-in"
          className="text-[color:var(--storefront-accent,var(--moss-700))] underline underline-offset-4"
        >
          Sign in
        </Link>
      </p>
    </form>
  );
}
