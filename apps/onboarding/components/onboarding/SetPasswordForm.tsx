"use client";

// SetPasswordForm — Phase M.
//
// Shown after the merchant clicks the magic link and the session has
// been marked email-verified. Two paths to a credential:
//
//   1. Pick a name + password — client-side signUpWithName(email, password,
//      name) hits Identity Toolkit accounts:signUp, then accounts:update to
//      put the name on the GIP account.
//   2. Continue with Google — gsi/client popup → signInWithGoogle hits
//      accounts:signInWithIdp. The user's email must match the session's.
//      Google usually supplies the name itself, so we only write the typed
//      one as a fallback.
//
// The name lives on this step rather than the first-page signup form
// because this is where the GIP account is actually created — collecting it
// earlier would mean carrying it through the server action, the onboarding
// session record and the magic-link round trip for no benefit. Its only
// destination is Google; it is never sent to our own services.
//
// On success, completeOnboarding creates the tenant, mints the session
// cookie, and we redirect to /welcome.
//
// Validation layer: react-hook-form + zod for the password field with
// inline errors. Server errors are routed to the password field when
// recognisable and fall back to a top-level alert otherwise.

import { useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { Input, Label } from "@tesserix/web";

import {
  signUpWithName,
  signInWithGoogle,
  updateDisplayName,
  GIPSignupError,
} from "@/lib/gip/signup";
import { getGoogleCredential } from "@/lib/gip/google-gsi";
import {
  completeOnboarding,
  completeOnboardingWithZitadel,
} from "@/app/onboarding/actions";
import {
  PASSWORD_REQUIREMENTS_TEXT,
  validateNewPassword,
} from "@/lib/auth/password-policy";
import { useOnboardingStore } from "@/lib/store/onboarding-store";

interface Props {
  sessionId: string;
  email: string;
  businessName: string;
  /**
   * Which identity provider backs merchant accounts. Read defensively,
   * exactly as apps/admin's AcceptInviteForm reads it: only the literal
   * "zitadel" switches this form onto the Zitadel path — anything else,
   * including undefined, keeps the GIP flow byte-for-byte. Wired from
   * `publicConfig.authProvider` by app/onboarding/set-password/page.tsx.
   */
  provider?: string;
}

const schema = z.object({
  // Required. The merchant is already past email verification here, so the
  // abandonment cost of one more short field is low — whereas leaving it
  // optional would recreate exactly the gap we are closing: every
  // password-signup merchant to date has no name anywhere in the product.
  name: z
    .string()
    .trim()
    .min(1, "Your name is required")
    .max(80, "Name is too long"),
  // The shared floor, left exactly as it was: GIP's own minimum is 8 and
  // the GIP path is not being changed here.
  //
  // The real Zitadel policy (12 characters, upper, lower, number,
  // symbol — see lib/auth/password-policy.ts) is applied on top of this
  // in onValid, and ONLY on the Zitadel path. Applying it through a
  // second resolver schema would mean swapping resolvers underneath
  // react-hook-form on a prop change, a subtlety this form does not need.
  // A client rule LOOSER than the server's is worse than no rule at all —
  // it promises an acceptance the server will refuse — which is exactly
  // what min(8) was doing here (#685).
  password: z
    .string()
    .min(1, "Password is required")
    .min(8, "Password must be at least 8 characters"),
});

type FormValues = z.infer<typeof schema>;

export function SetPasswordForm({ sessionId, email, provider }: Props) {
  const router = useRouter();
  const isZitadel = provider === "zitadel";
  const setSubmitted = useOnboardingStore((s) => s.setSubmitted);
  const storedSlug = useOnboardingStore((s) => s.slug);
  const [submitError, setSubmitError] = useState<string | null>(null);
  const [pending, startTransition] = useTransition();
  const [googlePending, setGooglePending] = useState(false);

  const {
    register,
    handleSubmit,
    setError,
    getValues,
    formState: { errors },
  } = useForm<FormValues>({
    resolver: zodResolver(schema),
    mode: "onTouched",
    reValidateMode: "onChange",
    defaultValues: { name: "", password: "" },
  });

  /**
   * Finish the wizard on the Zitadel path.
   *
   * platform-api's complete endpoint provisions the Zitadel user from
   * this password, ensures the mark8ly-admin project grant, and writes
   * both FGA owner tuples — so there is nothing for the browser to create
   * first, and no GIP call of any kind on this path.
   *
   * No auto-login follows. See completeOnboardingWithZitadel's doc for
   * why: the admin is on a different origin and Zitadel's login-client
   * model has no session this page could mint. The merchant signs in
   * once, on the admin, with the password they just chose.
   */
  function completeWithZitadel(values: FormValues) {
    startTransition(async () => {
      const r = await completeOnboardingWithZitadel({
        sessionId,
        password: values.password,
        name: values.name,
      });
      if (!r.ok) {
        // The server is authoritative on the password policy: the client
        // copy above and it can drift. When platform-api names the rule
        // that was broken, put its message on the password field where
        // the fix is, not in the generic form-level alert.
        if (r.code === "password_policy") {
          setError("password", { type: "server", message: r.message });
          return;
        }
        setSubmitError(r.message);
        return;
      }
      if (r.data.slug) {
        setSubmitted({ email, sessionId, businessName: "", slug: r.data.slug, countryCode: "", currencyCode: "", timezone: "", taxId: "", migrationType: "new", whoisUrl: "", screenshotUrl: "" });
      }
      router.push("/welcome");
    });
  }

  function onValid(values: FormValues) {
    setSubmitError(null);

    if (isZitadel) {
      const policyError = validateNewPassword(values.password);
      if (policyError) {
        setError("password", { type: "validate", message: policyError });
        return;
      }
      completeWithZitadel(values);
      return;
    }

    startTransition(async () => {
      let uid = "";
      let idToken = "";
      try {
        // signUpWithName never rejects because of the name write — only a
        // genuine accounts:signUp failure lands in the catch below.
        const gip = await signUpWithName(email, values.password, values.name);
        uid = gip.uid;
        idToken = gip.idToken;
      } catch (err) {
        if (err instanceof GIPSignupError && err.code === "weak_password") {
          setError("password", {
            type: "server",
            message: "Password is too weak. Pick something stronger.",
          });
          return;
        }
        if (
          err instanceof GIPSignupError &&
          /EMAIL_EXISTS/.test(err.message)
        ) {
          setSubmitError(
            "An admin account already exists for this email. Use a different email for a separate admin identity, or sign in to that existing account and add a store from Settings.",
          );
          return;
        } else {
          setSubmitError(
            err instanceof Error
              ? `Account creation failed: ${err.message}`
              : "Account creation failed.",
          );
          return;
        }
      }

      const r = await completeOnboarding({
        sessionId,
        gipUid: uid,
        gipIdToken: idToken,
      });
      if (!r.ok) {
        setSubmitError(r.message);
        return;
      }
      // Persist slug from completion response so WelcomeCta builds
      // the correct per-tenant admin URL.
      if (r.data.slug) {
        setSubmitted({ email, sessionId, businessName: "", slug: r.data.slug, countryCode: "", currencyCode: "", timezone: "", taxId: "", migrationType: "new", whoisUrl: "", screenshotUrl: "" });
      }
      router.push("/welcome");
    });
  }

  async function handleGoogle() {
    setSubmitError(null);
    setGooglePending(true);
    try {
      const { credential } = await getGoogleCredential();
      const gip = await signInWithGoogle(credential);

      // Defense in depth: Google's verified email must match the session.
      const googleEmail = decodeJwtEmail(gip.idToken);
      if (googleEmail && googleEmail.toLowerCase() !== email.toLowerCase()) {
        setSubmitError(
          `This Google account (${googleEmail}) doesn${"\u2019"}t match the email you signed up with (${email}).`,
        );
        return;
      }

      // Google normally supplies the name itself, in which case the account
      // record already has one and there is nothing to write. Only fall back
      // to the typed name when it doesn't. Its own try/catch: a failed name
      // write must never take the surrounding signup down with it.
      const typedName = getValues("name").trim();
      if (!gip.displayName && typedName) {
        try {
          await updateDisplayName(gip.idToken, typedName);
        } catch {
          // Non-fatal by design — see signUpWithName.
        }
      }

      const r = await completeOnboarding({
        sessionId,
        gipUid: gip.uid,
        gipIdToken: gip.idToken,
      });
      if (!r.ok) {
        setSubmitError(r.message);
        return;
      }
      if (r.data.slug) {
        setSubmitted({ email, sessionId, businessName: "", slug: r.data.slug, countryCode: "", currencyCode: "", timezone: "", taxId: "", migrationType: "new", whoisUrl: "", screenshotUrl: "" });
      }
      router.push("/welcome");
    } catch (err) {
      setSubmitError(
        err instanceof Error
          ? `Google sign-in failed: ${err.message}`
          : "Google sign-in failed.",
      );
    } finally {
      setGooglePending(false);
    }
  }

  const disabled = pending || googlePending;

  return (
    <div className="w-full max-w-md border-t border-border-subtle pt-10">
      <form
        onSubmit={handleSubmit(onValid)}
        noValidate
        className="space-y-5"
      >
        <div className="space-y-1.5">
          <Label htmlFor="email" className="text-foreground">
            Email address
          </Label>
          <Input id="email" type="email" value={email} readOnly disabled />
          <div className="min-h-[1.125rem]">
            <p className="text-xs text-moss-700">Verified</p>
          </div>
        </div>

        <div className="space-y-1.5">
          <Label htmlFor="name" className="text-foreground">
            Your name
          </Label>
          <Input
            id="name"
            type="text"
            placeholder="Ada Lovelace"
            autoComplete="name"
            disabled={disabled}
            aria-invalid={errors.name ? true : undefined}
            aria-describedby={errors.name ? "name-error" : undefined}
            {...register("name")}
          />
          <div className="min-h-[1.125rem]">
            {errors.name ? (
              <p
                id="name-error"
                role="alert"
                aria-live="polite"
                className="text-xs text-danger"
              >
                {errors.name.message}
              </p>
            ) : null}
          </div>
        </div>

        <div className="space-y-1.5">
          <Label htmlFor="password" className="text-foreground">
            Password
          </Label>
          <Input
            id="password"
            type="password"
            placeholder={
              isZitadel ? "At least 12 characters" : "At least 8 characters"
            }
            autoComplete="new-password"
            disabled={disabled}
            aria-invalid={errors.password ? true : undefined}
            aria-describedby={
              errors.password
                ? "password-error"
                : isZitadel
                  ? "password-requirements"
                  : undefined
            }
            {...register("password")}
          />
          {/* The whole policy, shown BEFORE the first submit. A merchant
              choosing a password should not have to guess and be
              corrected one rule at a time — that drip-feed is what the
              incident behind lib/auth/password-policy.ts was made of.
              Zitadel path only: the GIP minimum really is 8. */}
          <div className="min-h-[1.125rem]">
            {errors.password ? (
              <p
                id="password-error"
                role="alert"
                aria-live="polite"
                className="text-xs text-danger"
              >
                {errors.password.message}
              </p>
            ) : isZitadel ? (
              <p
                id="password-requirements"
                className="text-xs text-foreground-tertiary"
              >
                {PASSWORD_REQUIREMENTS_TEXT}
              </p>
            ) : null}
          </div>
        </div>

        {submitError && (
          <p
            role="alert"
            aria-live="polite"
            className="text-sm text-danger"
          >
            {submitError}
          </p>
        )}

        <button
          type="submit"
          disabled={disabled}
          className="inline-flex h-12 w-full items-center justify-center rounded-md bg-primary px-6 text-base font-medium text-primary-foreground hover:bg-primary-hover disabled:cursor-not-allowed disabled:bg-ink-600"
        >
          {pending ? "Finishing up…" : "Create account"}
        </button>

        {/* Google is GIP-only here. On the Zitadel path the backend
            provisions from a PASSWORD rather than an IDP intent, and
            there is no auto-login to hand a Google credential to — a
            merchant who wants Google can use it on the admin sign-in page
            once their account exists. Mirrors what apps/admin's
            accept-invite form does, for the same two reasons (#680). */}
        {!isZitadel && (
          <>
            <div className="relative py-2">
          <div className="absolute inset-0 flex items-center" aria-hidden="true">
            <div className="w-full border-t border-border-subtle" />
          </div>
          <div className="relative flex justify-center">
            <span className="bg-background px-3 text-xs uppercase tracking-[0.16em] text-foreground-tertiary">
              or
            </span>
          </div>
        </div>

        <button
          type="button"
          onClick={handleGoogle}
          disabled={disabled}
          className="inline-flex h-12 w-full items-center justify-center gap-3 rounded-md border border-border bg-background-elevated px-6 text-sm font-medium text-foreground hover:bg-paper-100 disabled:cursor-not-allowed disabled:opacity-50"
        >
          <GoogleMark />
          {googlePending ? "Opening Google…" : "Continue with Google"}
            </button>
          </>
        )}
      </form>
    </div>
  );
}

// decodeJwtEmail pulls the email claim out of a JWT without verifying the
// signature. Defense-in-depth only — the server is the source of truth.
function decodeJwtEmail(token: string): string | null {
  try {
    const [, payload] = token.split(".");
    if (!payload) return null;
    const padded = payload.padEnd(
      payload.length + ((4 - (payload.length % 4)) % 4),
      "=",
    );
    const json = atob(padded.replace(/-/g, "+").replace(/_/g, "/"));
    const claims = JSON.parse(json) as { email?: string };
    return claims.email ?? null;
  } catch {
    return null;
  }
}

function GoogleMark() {
  return (
    <svg width="18" height="18" viewBox="0 0 18 18" aria-hidden="true">
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
