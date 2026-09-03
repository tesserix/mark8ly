"use client";

// Returning-user sign-in for the admin app. Two paths:
//
//   1. Email + password — Identity Toolkit signInWithPassword via the GIP
//      REST helper, then signIn server action.
//   2. Continue with Google — gsi/client popup → Google credential →
//      Identity Toolkit signInWithIdp → same signIn server action.
//
// Both paths land at /dashboard on success (or /pick-tenant when the user
// belongs to multiple stores).

import { useState, useTransition } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { Input } from "@tesserix/web";
import { Field } from "@repo/ui/field";
import { GoogleMark } from "@repo/ui/google-mark";
import { AppleMark } from "@repo/ui/apple-mark";

import { signInWithPassword, signInWithGoogle, signInWithApple, GIPError } from "@/lib/gip/signup";
import { getGoogleCredential } from "@/lib/gip/google-gsi";
import { getAppleCredential } from "@/lib/gip/apple-js";
import { appleSignInEnabled } from "@/lib/config";
import { linkGoogleToInternalPassword } from "@/lib/gip/link";
import {
  signIn,
  confirmMFALogin,
  confirmEmailOTPLogin,
  resendEmailOTPCode,
} from "@/app/login/actions";
import { prepareCrossDomainNavigation } from "@/lib/auth/cross-domain-handoff";
import { LinkProviderPrompt } from "@repo/ui/auth/link-provider-prompt";

const MARKETING_URL =
  process.env.NEXT_PUBLIC_MARKETING_URL ?? "http://localhost:4201";

const schema = z.object({
  email: z
    .string()
    .min(1, "Email is required")
    .email("Enter a valid email address"),
  password: z.string().min(1, "Please enter your password"),
});

type FormValues = z.infer<typeof schema>;

interface SignInFormProps {
  /**
   * Where to redirect after a successful sign-in. Set by middleware on
   * per-tenant subdomains that bounce users here for authentication.
   * Must be pre-sanitized by the server (the /login page) — SignInForm
   * trusts this value and does a full-page navigation to it.
   */
  returnUrl?: string;
  /**
   * Zitadel's `auth_request_id`, present once `/login` has bounced
   * through Zitadel's `/authorize` and back (see
   * app/login/authorize/route.ts and app/auth/callback/route.ts).
   * Unused under GIP. Task 5 wires this into `signInWithZitadel`.
   */
  authRequestId?: string;
}

export function SignInForm({ returnUrl }: SignInFormProps = {}) {
  const router = useRouter();

  // Full-page navigation when returnUrl crosses origins (the common
  // case: signing in at admin.mark8ly.com → bouncing to
  // demo-store-admin.mark8ly.com). router.push only handles same-origin
  // client-side navigation, so we use window.location.assign for the
  // cross-subdomain case.
  //
  // If the returnUrl points at a custom admin domain
  // (admin.<merchant-tld>), the session cookie can't ride across TLDs;
  // we mint a short-lived handoff code and bounce through the handoff
  // route on the target host instead.
  async function goToDestination(defaultPath: string) {
    if (returnUrl) {
      const handoff = await prepareCrossDomainNavigation({ returnUrl });
      const dest = handoff.kind === "handoff" ? handoff.url : returnUrl;
      if (typeof window !== "undefined") {
        window.location.assign(dest);
      }
      return;
    }
    router.push(defaultPath);
  }

  const [submitError, setSubmitError] = useState<string | null>(null);
  const [pending, startTransition] = useTransition();
  const [googlePending, setGooglePending] = useState(false);
  const [applePending, setApplePending] = useState(false);
  const [needConfirmation, setNeedConfirmation] = useState<{
    email: string;
    pendingIdpCredential: string;
  } | null>(null);
  const [linkPromptError, setLinkPromptError] = useState<string | null>(null);

  // MFA challenge state — when the signIn server action reports
  // mfaRequired, we switch the form to a 6-digit challenge instead
  // of redirecting. `mfaMultipleTenants` is remembered from the
  // first step so the post-challenge redirect still respects
  // pick-tenant vs dashboard.
  // Which challenge the server asked for. "mfa" is an authenticator
  // app; "email_otp" is the new-device code auth-bff emails. Both leave
  // only a PENDING cookie, so neither may skip to goToDestination.
  const [challenge, setChallenge] = useState<"mfa" | "email_otp" | null>(null);
  const [resendNotice, setResendNotice] = useState<string | null>(null);
  const [mfaStep, setMfaStep] = useState(false);
  const [mfaCode, setMfaCode] = useState("");
  const [mfaPending, setMfaPending] = useState(false);
  const [mfaMultipleTenants, setMfaMultipleTenants] = useState(false);

  const {
    register,
    handleSubmit,
    setError,
    formState: { errors },
  } = useForm<FormValues>({
    resolver: zodResolver(schema),
    mode: "onTouched",
    reValidateMode: "onChange",
    defaultValues: { email: "", password: "" },
  });

  const disabled = pending || googlePending || applePending;

  function onValid(values: FormValues) {
    setSubmitError(null);
    const trimmedEmail = values.email.trim().toLowerCase();

    startTransition(async () => {
      let idToken = "";
      let uid = "";
      try {
        const gip = await signInWithPassword(trimmedEmail, values.password);
        idToken = gip.idToken;
        uid = gip.uid;
      } catch (err) {
        if (err instanceof GIPError && err.code === "invalid_credentials") {
          setError("password", {
            type: "server",
            message: "Email or password is incorrect",
          });
          return;
        }
        setSubmitError(
          err instanceof Error ? `Sign-in failed: ${err.message}` : "Sign-in failed",
        );
        return;
      }

      const r = await signIn({ idToken, uid });
      if (!r.ok) {
        if (r.code === "tenant_not_found") {
          setSubmitError(
            "We couldn't find a store for this account. Did you finish onboarding?",
          );
        } else {
          setSubmitError(r.message);
        }
        return;
      }
      if (r.data.mfaRequired || r.data.emailOtpRequired) {
        setMfaMultipleTenants(r.data.multipleTenants);
        setChallenge(r.data.mfaRequired ? "mfa" : "email_otp");
        setMfaStep(true);
        return;
      }
      await goToDestination(r.data.multipleTenants ? "/pick-tenant" : "/dashboard");
    });
  }

  async function completeSignIn(idToken: string, uid: string) {
    const r = await signIn({ idToken, uid });
    if (!r.ok) {
      setSubmitError(
        r.code === "tenant_not_found"
          ? "No store found for this Google account. Start a new store from the home page."
          : r.message,
      );
      return;
    }
    if (r.data.mfaRequired || r.data.emailOtpRequired) {
      setMfaMultipleTenants(r.data.multipleTenants);
      setChallenge(r.data.mfaRequired ? "mfa" : "email_otp");
      setMfaStep(true);
      return;
    }
    await goToDestination(r.data.multipleTenants ? "/pick-tenant" : "/dashboard");
  }

  async function handleApple() {
    setSubmitError(null);
    setApplePending(true);
    try {
      const { idToken: appleToken, nonce } = await getAppleCredential();
      const result = await signInWithApple(appleToken, nonce);
      if (result.kind === "needConfirmation") {
        // Same linking prompt as Google: GIP already has this email under
        // another provider and wants proof before joining them.
        setNeedConfirmation({
          email: result.email,
          pendingIdpCredential: result.pendingIdpCredential,
        });
        return;
      }
      await completeSignIn(result.idToken, result.uid);
    } catch (err) {
      const msg = err instanceof Error ? err.message : "";
      // Closing Apple's popup is a normal user action, not an error worth
      // shouting about.
      if (msg.includes("popup_closed") || msg.includes("user_cancelled")) {
        return;
      }
      setSubmitError(msg ? `Apple sign-in failed: ${msg}` : "Apple sign-in failed");
    } finally {
      setApplePending(false);
    }
  }

  async function handleGoogle() {
    setSubmitError(null);
    setGooglePending(true);
    try {
      const { credential } = await getGoogleCredential();
      const result = await signInWithGoogle(credential);
      if (result.kind === "needConfirmation") {
        setNeedConfirmation({
          email: result.email,
          pendingIdpCredential: result.pendingIdpCredential,
        });
        return;
      }
      await completeSignIn(result.idToken, result.uid);
    } catch (err) {
      setSubmitError(
        err instanceof Error
          ? `Google sign-in failed: ${err.message}`
          : "Google sign-in failed",
      );
    } finally {
      setGooglePending(false);
    }
  }

  async function handleLinkConfirm(password: string) {
    if (!needConfirmation) return;
    setLinkPromptError(null);
    try {
      const linked = await linkGoogleToInternalPassword(
        needConfirmation.email,
        password,
        needConfirmation.pendingIdpCredential,
      );
      setNeedConfirmation(null);
      await completeSignIn(linked.idToken, linked.uid);
    } catch (err) {
      if (err instanceof GIPError && err.code === "invalid_credentials") {
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

  function handleLinkCancel() {
    setNeedConfirmation(null);
    setSubmitError(
      "Linking cancelled. Sign in with email and password instead.",
    );
  }

  async function handleMFA(e: React.FormEvent) {
    e.preventDefault();
    setSubmitError(null);
    setMfaPending(true);
    try {
      const r =
        challenge === "email_otp"
          ? await confirmEmailOTPLogin(mfaCode)
          : await confirmMFALogin(mfaCode);
      if (!r.ok) {
        setSubmitError(r.message);
        return;
      }
      await goToDestination(mfaMultipleTenants ? "/pick-tenant" : "/dashboard");
    } finally {
      setMfaPending(false);
    }
  }

  function cancelMFA() {
    setMfaStep(false);
    setChallenge(null);
    setMfaCode("");
    setSubmitError(null);
    setResendNotice(null);
  }

  async function handleResend() {
    setSubmitError(null);
    setResendNotice(null);
    setMfaPending(true);
    try {
      const r = await resendEmailOTPCode();
      setResendNotice(
        r.ok ? "We've sent a new code. It expires in 5 minutes." : r.message,
      );
    } finally {
      setMfaPending(false);
    }
  }

  if (mfaStep) {
    return (
      <div className="w-full max-w-md">
        <div className="space-y-2">
          <p className="eyebrow">mark8ly admin</p>
          <h1 className="font-serif text-4xl font-medium tracking-tight text-foreground">
            {challenge === "email_otp" ? "Check your email" : "Two-factor check"}
          </h1>
          <p className="text-base leading-7 text-foreground-secondary">
            {challenge === "email_otp"
              ? "We emailed you a 6-digit code because this is a new device. It expires in 5 minutes."
              : "Enter the 6-digit code from your authenticator app to finish signing in."}
          </p>
        </div>

        <form onSubmit={handleMFA} className="mt-8 space-y-5">
          <Field
            id="mfa-code"
            label={challenge === "email_otp" ? "Sign-in code" : "Verification code"}
          >
            <Input
              id="mfa-code"
              type="text"
              inputMode="numeric"
              autoComplete="one-time-code"
              maxLength={6}
              placeholder="000000"
              value={mfaCode}
              onChange={(e) => setMfaCode(e.target.value.replace(/\D/g, ""))}
              disabled={mfaPending}
              autoFocus
              className="font-mono text-lg tracking-[0.4em]"
            />
          </Field>

          {resendNotice && (
            <p aria-live="polite" className="text-sm text-foreground-secondary">
              {resendNotice}
            </p>
          )}

          {submitError && (
            <p role="alert" aria-live="polite" className="text-sm text-danger">
              {submitError}
            </p>
          )}

          <button
            type="submit"
            disabled={mfaPending || mfaCode.length !== 6}
            className="inline-flex h-12 w-full items-center justify-center rounded-md bg-primary px-6 text-base font-medium text-primary-foreground hover:bg-primary-hover disabled:cursor-not-allowed disabled:bg-ink-600"
          >
            {mfaPending ? "Verifying…" : "Verify and continue"}
          </button>

          {challenge === "email_otp" && (
            <button
              type="button"
              onClick={handleResend}
              disabled={mfaPending}
              className="inline-flex h-11 w-full items-center justify-center text-sm text-foreground-secondary underline underline-offset-4 decoration-border-subtle hover:text-foreground disabled:cursor-not-allowed disabled:opacity-60"
            >
              Send a new code
            </button>
          )}

          <button
            type="button"
            onClick={cancelMFA}
            className="inline-flex h-11 w-full items-center justify-center text-sm text-foreground-secondary underline underline-offset-4 decoration-border-subtle hover:text-foreground"
          >
            Use a different account
          </button>
        </form>
      </div>
    );
  }

  return (
    <div className="w-full max-w-md">
      <div className="space-y-2">
        <p className="eyebrow">mark8ly admin</p>
        <h1 className="font-serif text-4xl font-medium tracking-tight text-foreground">
          Welcome back
        </h1>
        <p className="text-base leading-7 text-foreground-secondary">
          Sign in to your store dashboard.
        </p>
      </div>

      <form onSubmit={handleSubmit(onValid)} noValidate className="mt-8 space-y-5">
        <Field id="email" label="Email address" error={errors.email?.message}>
          <Input
            id="email"
            type="email"
            placeholder="founder@yourbusiness.com"
            autoComplete="email"
            spellCheck={false}
            disabled={disabled}
            aria-invalid={errors.email ? true : undefined}
            aria-describedby={errors.email ? "email-error" : undefined}
            {...register("email")}
          />
        </Field>

        <Field id="password" label="Password" error={errors.password?.message}>
          <Input
            id="password"
            type="password"
            autoComplete="current-password"
            disabled={disabled}
            aria-invalid={errors.password ? true : undefined}
            aria-describedby={errors.password ? "password-error" : undefined}
            {...register("password")}
          />
        </Field>

        <div className="-mt-2 flex justify-end">
          <Link
            href="/forgot-password"
            className="text-xs text-foreground-secondary underline underline-offset-4 decoration-border-subtle transition-colors hover:text-foreground hover:decoration-foreground-tertiary"
          >
            Forgot password?
          </Link>
        </div>

        {submitError && (
          <p role="alert" aria-live="polite" className="text-sm text-danger">
            {submitError}
          </p>
        )}

        <button
          type="submit"
          disabled={disabled}
          className="inline-flex h-12 w-full items-center justify-center rounded-md bg-primary px-6 text-base font-medium text-primary-foreground hover:bg-primary-hover disabled:cursor-not-allowed disabled:bg-ink-600"
        >
          {pending ? "Signing in…" : "Sign in"}
        </button>

        <div className="relative py-1">
          <div className="absolute inset-0 flex items-center" aria-hidden="true">
            <div className="w-full border-t border-border-subtle" />
          </div>
          <div className="relative flex justify-center">
            <span className="bg-background px-3 text-xs uppercase tracking-wider text-foreground-tertiary">
              or
            </span>
          </div>
        </div>

        <button
          type="button"
          onClick={handleGoogle}
          disabled={disabled}
          className="inline-flex h-11 w-full items-center justify-center gap-3 rounded-md border border-border bg-background-elevated px-6 text-sm font-medium text-foreground hover:border-border-strong disabled:cursor-not-allowed disabled:opacity-50"
        >
          <GoogleMark />
          {googlePending ? "Opening Google…" : "Continue with Google"}
        </button>

        {/* Rendered only when a Services ID is configured. Apple treats web
            as a separate client from the iOS app, so until that exists the
            button would fail at Apple rather than in our code. */}
        {appleSignInEnabled && (
          <button
            type="button"
            onClick={handleApple}
            disabled={disabled}
            className="mt-3 inline-flex h-11 w-full items-center justify-center gap-3 rounded-md border border-border bg-background-elevated px-6 text-sm font-medium text-foreground hover:border-border-strong disabled:cursor-not-allowed disabled:opacity-50"
          >
            <AppleMark />
            {applePending ? "Opening Apple…" : "Continue with Apple"}
          </button>
        )}

        <p className="text-center text-xs text-foreground-tertiary">
          Don&apos;t have a store yet?{" "}
          <a
            href={MARKETING_URL}
            className="text-foreground underline decoration-moss-700 decoration-2 underline-offset-4 hover:text-moss-700"
          >
            Start a new one
          </a>
          .
        </p>
      </form>

      {needConfirmation && (
        <LinkProviderPrompt
          email={needConfirmation.email}
          variant="admin"
          error={linkPromptError}
          onConfirm={handleLinkConfirm}
          onCancel={handleLinkCancel}
        />
      )}
    </div>
  );
}
