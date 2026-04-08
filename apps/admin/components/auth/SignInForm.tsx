"use client";

// Returning-user sign-in form for the admin app. Two paths:
//
//   1. Email + password — Identity Toolkit signInWithPassword via the GIP
//      REST helper, then signIn server action.
//   2. Continue with Google — gsi/client popup → Google credential →
//      Identity Toolkit signInWithIdp → same signIn server action.
//
// Both paths land at /dashboard on success.

import { useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import { Input, Label } from "@tesserix/web";

import { signInWithPassword, signInWithGoogle, GIPError } from "@/lib/gip/signup";
import { getGoogleCredential } from "@/lib/gip/google-gsi";
import { signIn } from "@/app/login/actions";

const MARKETING_URL =
  process.env.NEXT_PUBLIC_MARKETING_URL ?? "http://localhost:4201";

export function SignInForm() {
  const router = useRouter();

  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [pending, startTransition] = useTransition();
  const [googlePending, setGooglePending] = useState(false);

  function handlePasswordSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);

    const trimmedEmail = email.trim().toLowerCase();
    if (!trimmedEmail.includes("@")) {
      setError("Please enter a valid email address");
      return;
    }
    if (password.length < 1) {
      setError("Please enter your password");
      return;
    }

    startTransition(async () => {
      let idToken = "";
      let uid = "";
      try {
        const gip = await signInWithPassword(trimmedEmail, password);
        idToken = gip.idToken;
        uid = gip.uid;
      } catch (err) {
        if (err instanceof GIPError && err.code === "invalid_credentials") {
          setError("Email or password is incorrect");
          return;
        }
        setError(
          err instanceof Error ? `Sign-in failed: ${err.message}` : "Sign-in failed",
        );
        return;
      }

      const r = await signIn({ idToken, uid });
      if (!r.ok) {
        setError(
          r.code === "tenant_not_found"
            ? "We couldn't find a store for this account. Did you finish onboarding?"
            : r.message,
        );
        return;
      }
      router.push("/dashboard");
    });
  }

  async function handleGoogle() {
    setError(null);
    setGooglePending(true);
    try {
      const { credential } = await getGoogleCredential();
      const gip = await signInWithGoogle(credential);
      const r = await signIn({ idToken: gip.idToken, uid: gip.uid });
      if (!r.ok) {
        setError(
          r.code === "tenant_not_found"
            ? "No store found for this Google account. Start a new store from the home page."
            : r.message,
        );
        return;
      }
      router.push("/dashboard");
    } catch (err) {
      setError(
        err instanceof Error ? `Google sign-in failed: ${err.message}` : "Google sign-in failed",
      );
    } finally {
      setGooglePending(false);
    }
  }

  const disabled = pending || googlePending;

  return (
    <div className="w-full max-w-md mx-auto">
      <div className="rounded-[2rem] border border-warm-200/90 bg-white/90 shadow-[0_24px_80px_rgba(43,38,34,0.12)] backdrop-blur-sm overflow-hidden">
        <div className="px-8 pt-8 pb-6 border-b border-warm-100">
          <p className="text-xs font-medium uppercase tracking-[0.16em] text-foreground-tertiary">
            Mark8ly admin
          </p>
          <h1 className="mt-3 font-serif text-3xl font-medium tracking-tight text-foreground">
            Welcome back
          </h1>
          <p className="mt-2 text-sm leading-6 text-foreground-secondary">
            Sign in to your store dashboard.
          </p>
        </div>

        <form onSubmit={handlePasswordSubmit} className="px-8 py-8 space-y-5">
          <div className="space-y-1.5">
            <Label htmlFor="email" className="text-foreground">
              Email address
            </Label>
            <Input
              id="email"
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              placeholder="founder@yourbusiness.com"
              required
              autoComplete="email"
              spellCheck={false}
              disabled={disabled}
            />
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="password" className="text-foreground">
              Password
            </Label>
            <Input
              id="password"
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              required
              autoComplete="current-password"
              disabled={disabled}
            />
          </div>

          {error && (
            <div className="p-3 rounded-lg bg-terracotta-50 border border-terracotta-200">
              <p className="text-sm text-terracotta-700" role="alert" aria-live="polite">
                {error}
              </p>
            </div>
          )}

          <button
            type="submit"
            disabled={disabled}
            className="inline-flex w-full items-center justify-center gap-2 rounded-xl bg-primary px-6 py-3.5 text-base font-medium text-primary-foreground shadow-[0_14px_30px_rgba(31,30,28,0.18)] transition hover:bg-primary-hover disabled:cursor-not-allowed disabled:opacity-50"
          >
            {pending ? "Signing in…" : "Sign in"}
          </button>

          <div className="relative py-1">
            <div className="absolute inset-0 flex items-center" aria-hidden>
              <div className="w-full border-t border-warm-200" />
            </div>
            <div className="relative flex justify-center">
              <span className="bg-white px-3 text-xs uppercase tracking-wider text-foreground-tertiary">
                or
              </span>
            </div>
          </div>

          <button
            type="button"
            onClick={handleGoogle}
            disabled={disabled}
            className="inline-flex w-full items-center justify-center gap-3 rounded-xl border border-warm-200 bg-white px-6 py-3 text-sm font-medium text-foreground transition hover:bg-warm-50 disabled:cursor-not-allowed disabled:opacity-50"
          >
            <GoogleMark />
            {googlePending ? "Opening Google…" : "Continue with Google"}
          </button>

          <p className="text-xs text-foreground-tertiary text-center">
            Don&apos;t have a store yet?{" "}
            <a href={MARKETING_URL} className="underline hover:text-foreground">
              Start a new one
            </a>
            .
          </p>
        </form>
      </div>
    </div>
  );
}

function GoogleMark() {
  return (
    <svg width="18" height="18" viewBox="0 0 18 18" aria-hidden>
      <path
        fill="#4285F4"
        d="M17.64 9.2c0-.637-.057-1.251-.164-1.84H9v3.481h4.844a4.14 4.14 0 0 1-1.796 2.716v2.258h2.908c1.702-1.567 2.684-3.874 2.684-6.615z"
      />
      <path
        fill="#34A853"
        d="M9 18c2.43 0 4.467-.806 5.956-2.184l-2.908-2.258c-.806.54-1.837.86-3.048.86-2.344 0-4.328-1.584-5.036-3.711H.957v2.332A8.997 8.997 0 0 0 9 18z"
      />
      <path
        fill="#FBBC05"
        d="M3.964 10.707A5.41 5.41 0 0 1 3.682 9c0-.593.102-1.17.282-1.707V4.961H.957A8.997 8.997 0 0 0 0 9c0 1.452.348 2.827.957 4.039l3.007-2.332z"
      />
      <path
        fill="#EA4335"
        d="M9 3.58c1.321 0 2.508.454 3.44 1.345l2.582-2.58C13.463.891 11.426 0 9 0A8.997 8.997 0 0 0 .957 4.961L3.964 7.293C4.672 5.166 6.656 3.58 9 3.58z"
      />
    </svg>
  );
}
