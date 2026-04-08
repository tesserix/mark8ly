"use client";

// SetPasswordForm — Phase M.
//
// Shown after the merchant clicks the magic link and the session has
// been marked email-verified. Two paths to a credential:
//
//   1. Pick a password — client-side signUp(email, password) hits
//      Identity Toolkit accounts:signUp.
//   2. Continue with Google — gsi/client popup → signInWithGoogle hits
//      accounts:signInWithIdp. The user's email must match the session's.
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

import { signUp, signInWithGoogle, GIPSignupError } from "@/lib/gip/signup";
import { getGoogleCredential } from "@/lib/gip/google-gsi";
import { completeOnboarding } from "@/app/onboarding/actions";

interface Props {
  sessionId: string;
  email: string;
  businessName: string;
}

const schema = z.object({
  password: z
    .string()
    .min(1, "Password is required")
    .min(8, "Password must be at least 8 characters"),
});

type FormValues = z.infer<typeof schema>;

export function SetPasswordForm({ sessionId, email }: Props) {
  const router = useRouter();
  const [submitError, setSubmitError] = useState<string | null>(null);
  const [pending, startTransition] = useTransition();
  const [googlePending, setGooglePending] = useState(false);

  const {
    register,
    handleSubmit,
    setError,
    formState: { errors },
  } = useForm<FormValues>({
    resolver: zodResolver(schema),
    mode: "onTouched",
    reValidateMode: "onChange",
    defaultValues: { password: "" },
  });

  function onValid(values: FormValues) {
    setSubmitError(null);

    startTransition(async () => {
      let uid = "";
      let idToken = "";
      try {
        const gip = await signUp(email, values.password);
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
            "An account already exists for this email. Try signing in instead.",
          );
          return;
        }
        setSubmitError(
          err instanceof Error
            ? `Account creation failed: ${err.message}`
            : "Account creation failed.",
        );
        return;
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

      const r = await completeOnboarding({
        sessionId,
        gipUid: gip.uid,
        gipIdToken: gip.idToken,
      });
      if (!r.ok) {
        setSubmitError(r.message);
        return;
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
          <Label htmlFor="password" className="text-foreground">
            Password
          </Label>
          <Input
            id="password"
            type="password"
            placeholder="At least 8 characters"
            autoComplete="new-password"
            disabled={disabled}
            aria-invalid={errors.password ? true : undefined}
            aria-describedby={errors.password ? "password-error" : undefined}
            {...register("password")}
          />
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
