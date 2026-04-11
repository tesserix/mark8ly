"use client";

// Forgot password funnel.
//
// Collects the merchant's email and POSTs to platform-api's
// /internal/auth/password-reset/request endpoint. Platform-api asks GIP
// for an oobCode with returnOobLink=true (so GIP does NOT send its
// default email) and dispatches our own branded HTML email via SendGrid
// that links to /reset-password in this very admin app. This replaces
// the previous direct-to-GIP flow which exposed the Firebase auth
// action URL and sent a plain-text default email.

import { useState, useTransition } from "react";
import Link from "next/link";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { Input } from "@tesserix/web";
import { Field } from "@repo/ui/field";

import { requestPasswordResetAction } from "@/app/forgot-password/actions";

const schema = z.object({
  email: z
    .string()
    .min(1, "Please enter your email")
    .email("Please enter a valid email"),
});

type FormValues = z.infer<typeof schema>;

export function ForgotPasswordForm() {
  const [submitError, setSubmitError] = useState<string | null>(null);
  const [sent, setSent] = useState(false);
  const [pending, startTransition] = useTransition();

  const {
    register,
    handleSubmit,
    formState: { errors },
  } = useForm<FormValues>({
    resolver: zodResolver(schema),
    mode: "onTouched",
    defaultValues: { email: "" },
  });

  function onValid(values: FormValues) {
    setSubmitError(null);
    startTransition(async () => {
      const result = await requestPasswordResetAction(
        values.email.trim().toLowerCase(),
      );
      if (result.ok) {
        setSent(true);
        return;
      }
      setSubmitError(result.message);
    });
  }

  if (sent) {
    return (
      <div className="w-full space-y-6">
        <div className="space-y-3">
          <p className="eyebrow">Check your inbox</p>
          <h1 className="font-serif text-4xl font-medium leading-[1.1] tracking-tight text-foreground">
            Reset link sent.
          </h1>
          <p className="max-w-md text-[15px] leading-relaxed text-foreground-secondary">
            If an account exists for that email, we&rsquo;ve sent a password
            reset link. Check your inbox and follow the instructions to
            choose a new password.
          </p>
        </div>
        <Link
          href="/login"
          className="inline-flex h-10 items-center rounded-[var(--radius)] bg-[color:var(--ink-900)] px-5 text-sm font-medium text-white transition-colors hover:bg-[color:var(--ink-800)]"
        >
          Back to sign in
        </Link>
      </div>
    );
  }

  return (
    <div className="w-full space-y-8">
      <div className="space-y-3">
        <p className="eyebrow">Reset password</p>
        <h1 className="font-serif text-4xl font-medium leading-[1.1] tracking-tight text-foreground">
          Forgot your password?
        </h1>
        <p className="max-w-md text-[15px] leading-relaxed text-foreground-secondary">
          Enter the email you used to sign up and we&rsquo;ll send you a link
          to choose a new one.
        </p>
      </div>

      <form
        onSubmit={handleSubmit(onValid)}
        noValidate
        className="space-y-5"
      >
        <Field id="email" label="Email address" error={errors.email?.message}>
          <Input
            id="email"
            type="email"
            autoComplete="email"
            disabled={pending}
            aria-invalid={errors.email ? true : undefined}
            {...register("email")}
          />
        </Field>

        {submitError && (
          <p role="alert" aria-live="polite" className="text-sm text-danger">
            {submitError}
          </p>
        )}

        <button
          type="submit"
          disabled={pending}
          className="inline-flex h-12 w-full items-center justify-center rounded-md bg-primary px-6 text-base font-medium text-primary-foreground hover:bg-primary-hover disabled:cursor-not-allowed disabled:bg-ink-600"
        >
          {pending ? "Sending…" : "Send reset link"}
        </button>

        <div className="flex justify-center">
          <Link
            href="/login"
            className="text-xs text-foreground-secondary underline underline-offset-4 decoration-border-subtle transition-colors hover:text-foreground hover:decoration-foreground-tertiary"
          >
            Back to sign in
          </Link>
        </div>
      </form>
    </div>
  );
}
