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
import { useRouter } from "next/navigation";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { Input } from "@tesserix/web";
import { Field } from "@repo/ui/field";
import { GoogleMark } from "@repo/ui/google-mark";

import { signInWithPassword, signInWithGoogle, GIPError } from "@/lib/gip/signup";
import { getGoogleCredential } from "@/lib/gip/google-gsi";
import { signIn } from "@/app/login/actions";

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
}

export function SignInForm({ returnUrl }: SignInFormProps = {}) {
  const router = useRouter();

  // Full-page navigation when returnUrl crosses origins (the common
  // case: signing in at admin.mark8ly.com → bouncing to
  // demo-store-admin.mark8ly.com). router.push only handles same-origin
  // client-side navigation, so we use window.location.assign for the
  // cross-subdomain case.
  function goToDestination(defaultPath: string) {
    if (returnUrl) {
      if (typeof window !== "undefined") {
        window.location.assign(returnUrl);
      }
      return;
    }
    router.push(defaultPath);
  }

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
    defaultValues: { email: "", password: "" },
  });

  const disabled = pending || googlePending;

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
      goToDestination(r.data.multipleTenants ? "/pick-tenant" : "/dashboard");
    });
  }

  async function handleGoogle() {
    setSubmitError(null);
    setGooglePending(true);
    try {
      const { credential } = await getGoogleCredential();
      const gip = await signInWithGoogle(credential);
      const r = await signIn({ idToken: gip.idToken, uid: gip.uid });
      if (!r.ok) {
        setSubmitError(
          r.code === "tenant_not_found"
            ? "No store found for this Google account. Start a new store from the home page."
            : r.message,
        );
        return;
      }
      goToDestination(r.data.multipleTenants ? "/pick-tenant" : "/dashboard");
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
    </div>
  );
}
