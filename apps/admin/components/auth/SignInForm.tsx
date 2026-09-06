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
import { Input, AuthOtpStep } from "@tesserix/web";
import { Field } from "@repo/ui/field";
import { GoogleMark } from "@repo/ui/google-mark";
import { AppleMark } from "@repo/ui/apple-mark";

import { signInWithPassword, signInWithGoogle, signInWithApple, GIPError } from "@/lib/gip/signup";
import { getGoogleCredential } from "@/lib/gip/google-gsi";
import { getAppleCredential } from "@/lib/gip/apple-js";
import { appleSignInEnabled, publicConfig } from "@/lib/config";
import { isTrustedZitadelHostedUrl, isTrustedCallbackUrl } from "@/lib/auth/zitadel-oidc";
import { linkGoogleToInternalPassword } from "@/lib/gip/link";
import {
  signIn,
  signInWithZitadel,
  confirmZitadelTotp,
  confirmMFALogin,
  confirmEmailOTPLogin,
  resendEmailOTPCode,
} from "@/app/login/actions";
import { startAdminGoogleSignIn } from "@/app/auth/idp/actions";
import { messageForAdminGoogleError } from "@/lib/auth/google-sign-in-admin";
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
   * Unused under GIP. Wired into `signInWithZitadel` below.
   */
  authRequestId?: string;
  /**
   * Which identity provider backs this sign-in. Read defensively: only
   * the exact literal `"zitadel"` switches this form onto the Zitadel
   * path — anything else, including undefined, keeps today's GIP flow.
   * Wired from `publicConfig` by `/login/page.tsx`.
   */
  provider?: string;
  /**
   * An outcome code from a completed (and rejected, or interrupted)
   * Google-through-Zitadel sign-in attempt — set by app/login/page.tsx
   * from `?error=` when app/auth/idp/finish/route.ts redirects back here.
   * Unused under GIP, where Google never leaves this page at all.
   * Mapped to a truthful, distinct message via messageForAdminGoogleError
   * rather than rendered directly — this value is a fixed code, never an
   * internal error string.
   */
  googleErrorCode?: string;
  /**
   * Set only when app/auth/idp/finish/route.ts redirected here because a
   * Google-through-Zitadel sign-in hit auth-bff's email-OTP gate. The form
   * then OPENS on the code step instead of the password fields: the
   * pending cookie auth-bff minted rode in on that redirect, and
   * `confirmEmailOTPLogin` resumes from that cookie plus the typed code
   * alone — nothing else has to survive the navigation.
   *
   * Deliberately narrower than the internal `challenge` state: "mfa" is
   * not accepted, because auth-bff's usermfa gate and Zitadel's own TOTP
   * step-up are still refused on the redirect path (they need a session
   * id/token that must never travel in a URL).
   */
  initialChallenge?: "email_otp";
  /**
   * Whether the account resolved to more than one store, carried across
   * the same redirect so the post-code landing is /pick-tenant rather
   * than /dashboard — the job `mfaMultipleTenants` does on the password
   * path, where it is remembered in state instead.
   */
  initialMultipleTenants?: boolean;
}

export function SignInForm({
  returnUrl,
  authRequestId,
  provider,
  googleErrorCode,
  initialChallenge,
  initialMultipleTenants,
}: SignInFormProps = {}) {
  const router = useRouter();
  const isZitadel = provider === "zitadel";

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

  const [submitError, setSubmitError] = useState<string | null>(
    googleErrorCode ? messageForAdminGoogleError(googleErrorCode) : null,
  );
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
  //
  // `initialChallenge` seeds all three from a redirect arrival (the Google
  // email-OTP path) instead of from a server-action result, so the code
  // screen renders on first paint with no flash of the password form.
  const [challenge, setChallenge] = useState<"mfa" | "email_otp" | null>(
    initialChallenge ?? null,
  );
  const [resendNotice, setResendNotice] = useState<string | null>(null);
  const [mfaStep, setMfaStep] = useState(initialChallenge !== undefined);
  const [mfaCode, setMfaCode] = useState("");
  const [mfaPending, setMfaPending] = useState(false);
  const [mfaMultipleTenants, setMfaMultipleTenants] = useState(
    initialMultipleTenants ?? false,
  );

  // Zitadel's own TOTP step-up — a different mechanism from auth-bff's
  // `usermfa` gate above. Zitadel itself demands a verified
  // authenticator code before the auth request can complete, and hands
  // back a session id/token plus a server-signed tenant code that must
  // ride unchanged into confirmZitadelTotp. Unreachable under GIP.
  const [zitadelTotpStep, setZitadelTotpStep] = useState(false);
  const [zitadelTotpCode, setZitadelTotpCode] = useState("");
  const [zitadelTotpPending, setZitadelTotpPending] = useState(false);
  const [zitadelTotpChallenge, setZitadelTotpChallenge] = useState<{
    sessionId: string;
    sessionToken: string;
    tenantCode: string;
  } | null>(null);

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

  // Shared by the Zitadel sign-in call and the Zitadel TOTP confirmation
  // below — both return the same `Result<SignInSuccess>` shape, so the
  // mfa/email-otp/totp/handoff/redirect branching only needs to live
  // once.
  async function afterZitadelResult(
    r: Awaited<ReturnType<typeof signInWithZitadel>>,
  ) {
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
    const { data } = r;
    if (data.totpRequired) {
      setMfaMultipleTenants(data.multipleTenants);
      setZitadelTotpChallenge({
        sessionId: data.zitadelSessionId ?? "",
        sessionToken: data.zitadelSessionToken ?? "",
        tenantCode: data.zitadelTenantCode ?? "",
      });
      setZitadelTotpStep(true);
      return;
    }
    if (data.mfaRequired || data.emailOtpRequired) {
      setMfaMultipleTenants(data.multipleTenants);
      setChallenge(data.mfaRequired ? "mfa" : "email_otp");
      setMfaStep(true);
      return;
    }
    if (data.handoffUrl) {
      // Server-supplied URL — validate against the one legitimate target
      // (Zitadel's own hosted login UI) before navigating. A bare
      // window.location.assign on an unchecked server value is an open
      // redirect if auth-bff (or anything sitting in front of it) is
      // ever compromised or mis-configured.
      if (
        typeof window !== "undefined" &&
        isTrustedZitadelHostedUrl(data.handoffUrl, publicConfig.zitadelIssuer)
      ) {
        window.location.assign(data.handoffUrl);
      }
      return;
    }
    if (data.callbackUrl) {
      // A completed Zitadel login carries its own /auth/callback URL —
      // navigate there instead of straight to the dashboard so that
      // route can verify state, clear the flow cookies, and only then
      // land the user on their destination. Skipping this made Task 4
      // (the callback route, the PKCE pair, the state check) dead code
      // the flow never traversed, and left the flow cookies alive for
      // their full TTL.
      //
      // Validate first: the state check on /auth/callback only runs if
      // the browser lands there at all — callbackUrl is what decides
      // where the browser goes, so an unchecked server value here is an
      // open redirect on a freshly-authenticated session. The only
      // legitimate target is this app's own /auth/callback, on this
      // app's own origin.
      if (
        typeof window !== "undefined" &&
        isTrustedCallbackUrl(data.callbackUrl, window.location.origin)
      ) {
        window.location.assign(data.callbackUrl);
        return;
      }
      // Untrusted callbackUrl: the session is already valid at this
      // point, so don't strand the merchant — log the rejection and
      // fall through to the normal destination logic below, exactly as
      // if no callbackUrl had been supplied.
      console.error("rejected an untrusted Zitadel callbackUrl", data.callbackUrl);
    }
    await goToDestination(data.multipleTenants ? "/pick-tenant" : "/dashboard");
  }

  function onValid(values: FormValues) {
    setSubmitError(null);
    const trimmedEmail = values.email.trim().toLowerCase();

    if (isZitadel) {
      startTransition(async () => {
        const r = await signInWithZitadel({
          email: trimmedEmail,
          password: values.password,
          authRequestId: authRequestId ?? "",
        });
        await afterZitadelResult(r);
      });
      return;
    }

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

  async function handleZitadelTotp(code: string) {
    if (!zitadelTotpChallenge) return;
    setSubmitError(null);
    setZitadelTotpPending(true);
    try {
      const r = await confirmZitadelTotp({
        authRequestId: authRequestId ?? "",
        sessionId: zitadelTotpChallenge.sessionId,
        sessionToken: zitadelTotpChallenge.sessionToken,
        code,
        zitadelTenantCode: zitadelTotpChallenge.tenantCode,
      });
      await afterZitadelResult(r);
    } finally {
      setZitadelTotpPending(false);
    }
  }

  function cancelZitadelTotp() {
    setZitadelTotpStep(false);
    setZitadelTotpChallenge(null);
    setZitadelTotpCode("");
    setSubmitError(null);
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

  /**
   * Under Zitadel there is no popup/credential exchange the way GIP has:
   * this asks auth-bff (via startAdminGoogleSignIn) for a Google authUrl
   * and does a full-page navigation there. The browser leaves this page
   * entirely and comes back at app/auth/idp/finish/route.ts, which mints
   * the session and redirects onward — there is nothing further for this
   * function to do on success, unlike the GIP path below.
   */
  async function handleGoogleZitadel() {
    setSubmitError(null);
    setGooglePending(true);
    try {
      const result = await startAdminGoogleSignIn(authRequestId ?? "");
      if (!result.ok) {
        setSubmitError(result.message);
        setGooglePending(false);
        return;
      }
      if (typeof window !== "undefined") {
        window.location.assign(result.authUrl);
      }
      // Leave googlePending true — the page is navigating away.
    } catch {
      setSubmitError("Google sign-in failed. Please try again.");
      setGooglePending(false);
    }
  }

  async function handleGoogle() {
    if (isZitadel) {
      await handleGoogleZitadel();
      return;
    }
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
    // Arrived here by redirect from the Google flow: the auth_request_id
    // on this page was already spent by idp/complete, so simply revealing
    // the password fields would hand the merchant a form Zitadel rejects
    // ("No valid authentication request found"). Re-enter through
    // /login/authorize for a fresh one instead. The password path never
    // takes this branch — initialChallenge is undefined there — so its
    // behaviour is unchanged.
    if (initialChallenge && typeof window !== "undefined") {
      window.location.assign("/login/authorize");
      return;
    }
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

  if (zitadelTotpStep) {
    return (
      <div className="w-full max-w-md">
        <div className="space-y-2">
          <p className="eyebrow">mark8ly admin</p>
          <h1 className="font-serif text-4xl font-medium tracking-tight text-foreground">
            Two-factor check
          </h1>
          <p className="text-base leading-7 text-foreground-secondary">
            Enter the 6-digit code from your authenticator app to finish signing in.
          </p>
        </div>

        <div className="mt-8">
          <AuthOtpStep
            value={zitadelTotpCode}
            onValueChange={setZitadelTotpCode}
            onSubmit={handleZitadelTotp}
            factor="totp"
            length={6}
            loading={zitadelTotpPending}
            error={submitError}
            label="Verification code"
            submitLabel={zitadelTotpPending ? "Verifying…" : "Verify and continue"}
          />
        </div>

        <button
          type="button"
          onClick={cancelZitadelTotp}
          className="mt-5 inline-flex h-11 w-full items-center justify-center text-sm text-foreground-secondary underline underline-offset-4 decoration-border-subtle hover:text-foreground"
        >
          Use a different account
        </button>
      </div>
    );
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

        {/* Google now authenticates through Zitadel's IDP-intent flow
            under both providers, so this renders unconditionally. Apple
            is out of scope for the Zitadel path in this phase and stays
            GIP-only. */}
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

        {!isZitadel && (
          <>
            {/* Rendered only when a Services ID is configured. Apple treats
                web as a separate client from the iOS app, so until that
                exists the button would fail at Apple rather than in our
                code. */}
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
          </>
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
