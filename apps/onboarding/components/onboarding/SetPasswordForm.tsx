"use client";

// SetPasswordForm — Phase M.
//
// Shown after the merchant clicks the magic link and the session has
// been marked email-verified. Two paths to a credential, both produce a
// fresh GIP id_token + uid that we hand to `completeOnboarding`:
//
//   1. Pick a password — client-side signUp(email, password) hits
//      Identity Toolkit accounts:signUp.
//   2. Continue with Google — gsi/client popup → signInWithGoogle hits
//      accounts:signInWithIdp. The user's email is whatever Google
//      asserted, NOT what they entered on /onboarding. We require the
//      two to match so a Google user can't claim someone else's session.
//
// On success, the server action completeOnboarding creates the tenant
// (the session is already verified, so the platform-api guard passes),
// calls auth-bff /auth/auto-login, and forwards the cookie. We then
// redirect to /welcome.

import { useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import { Input, Label } from "@tesserix/web";

import { signUp, signInWithGoogle, GIPSignupError } from "@/lib/gip/signup";
import { getGoogleCredential } from "@/lib/gip/google-gsi";
import { completeOnboarding } from "@/app/onboarding/actions";

interface Props {
  sessionId: string;
  email: string;
  businessName: string;
}

export function SetPasswordForm({ sessionId, email, businessName }: Props) {
  const router = useRouter();
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [pending, startTransition] = useTransition();
  const [googlePending, setGooglePending] = useState(false);

  function handlePasswordSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);

    if (password.length < 8) {
      setError("Password must be at least 8 characters");
      return;
    }

    startTransition(async () => {
      let uid = "";
      let idToken = "";
      try {
        const gip = await signUp(email, password);
        uid = gip.uid;
        idToken = gip.idToken;
      } catch (err) {
        if (err instanceof GIPSignupError && err.code === "weak_password") {
          setError("Password is too weak. Pick something stronger.");
          return;
        }
        // EMAIL_EXISTS means an account with this email already exists in
        // GIP — either the user clicked the magic link twice and signed
        // up on a previous attempt, or they should be on /login instead.
        if (err instanceof GIPSignupError && /EMAIL_EXISTS/.test(err.message)) {
          setError(
            "An account already exists for this email. Try signing in instead.",
          );
          return;
        }
        setError(
          err instanceof Error ? `Account creation failed: ${err.message}` : "Account creation failed",
        );
        return;
      }

      const r = await completeOnboarding({ sessionId, gipUid: uid, gipIdToken: idToken });
      if (!r.ok) {
        setError(r.message);
        return;
      }
      router.push("/welcome");
    });
  }

  async function handleGoogle() {
    setError(null);
    setGooglePending(true);
    try {
      const { credential } = await getGoogleCredential();
      const gip = await signInWithGoogle(credential);

      // Defense-in-depth: confirm Google's verified email matches the
      // session's email so a stray Google account can't hijack a session.
      const googleEmail = decodeJwtEmail(gip.idToken);
      if (googleEmail && googleEmail.toLowerCase() !== email.toLowerCase()) {
        setError(
          `This Google account (${googleEmail}) doesn't match the email you signed up with (${email}).`,
        );
        return;
      }

      const r = await completeOnboarding({
        sessionId,
        gipUid: gip.uid,
        gipIdToken: gip.idToken,
      });
      if (!r.ok) {
        setError(r.message);
        return;
      }
      router.push("/welcome");
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
        <div className="px-8 pt-8 pb-6 border-b border-warm-100 bg-[linear-gradient(180deg,rgba(243,238,230,0.72),rgba(255,255,255,0.98))]">
          <div className="mb-4 flex items-center justify-between gap-3">
            <span className="text-xs font-medium uppercase tracking-[0.16em] text-foreground-tertiary">
              Almost there
            </span>
            <span className="rounded-full bg-warm-100 px-3 py-1 text-xs font-medium text-foreground-secondary">
              Step 2 of 2
            </span>
          </div>
          <h1 className="font-serif text-3xl font-medium tracking-tight text-foreground">
            Set a password{businessName ? ` for ${businessName}` : ""}
          </h1>
          <p className="mt-2 text-sm leading-6 text-foreground-secondary">
            Pick a password you&apos;ll remember, or sign in with Google to skip
            it. We&apos;ll use this whenever you come back to your store.
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
              readOnly
              disabled
            />
            <p className="text-xs text-sage-700">✓ Verified</p>
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
              placeholder="At least 8 characters"
              required
              minLength={8}
              autoComplete="new-password"
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
            disabled={disabled || password.length < 8}
            className="inline-flex w-full items-center justify-center gap-2 rounded-xl bg-primary px-6 py-3.5 text-base font-medium text-primary-foreground shadow-[0_14px_30px_rgba(31,30,28,0.18)] transition hover:bg-primary-hover disabled:cursor-not-allowed disabled:opacity-50"
          >
            {pending ? "Finishing up…" : "Create account"}
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
        </form>
      </div>
    </div>
  );
}

// decodeJwtEmail pulls the email claim out of a JWT without verifying
// the signature. Used as a defense-in-depth check that the Google account
// matches the session's email — the server is the source of truth for
// what tenant gets created.
function decodeJwtEmail(token: string): string | null {
  try {
    const [, payload] = token.split(".");
    if (!payload) return null;
    const padded = payload.padEnd(payload.length + ((4 - (payload.length % 4)) % 4), "=");
    const json = atob(padded.replace(/-/g, "+").replace(/_/g, "/"));
    const claims = JSON.parse(json) as { email?: string };
    return claims.email ?? null;
  } catch {
    return null;
  }
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
