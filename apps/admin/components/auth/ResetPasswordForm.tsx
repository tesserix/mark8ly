"use client";

// Reset password form — client component wired to the server action
// that proxies to platform-api. Users land here from the branded email
// with the oobCode already in the URL, so the form only collects the
// new password + confirmation and posts both back.

import { useState, useTransition } from "react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { Input } from "@tesserix/web";
import { Field } from "@repo/ui/field";

import { confirmPasswordResetAction } from "@/app/reset-password/actions";
import { publicConfig } from "@/lib/config";
import {
  PASSWORD_REQUIREMENTS_TEXT,
  validateNewPassword,
} from "@/lib/auth/password-policy";
import { signInHref } from "@/lib/auth/sign-in-href";

// The 8-char floor is GIP's minimum and stays for that path. Zitadel's
// real policy (12 + upper/lower/number/symbol) is layered on below via
// superRefine, gated on the provider — mirroring how AcceptInviteForm
// avoids tightening the shared GIP schema. #695: this form previously
// claimed 8 on BOTH paths and then repeated that number when Zitadel
// rejected an 8-character password, leaving the user no way forward.
const isZitadel = publicConfig.authProvider === "zitadel";

const schema = z
  .object({
    password: z
      .string()
      .min(8, "At least 8 characters")
      .max(128, "Password is too long"),
    confirm: z.string().min(1, "Please confirm your password"),
  })
  .superRefine((v, ctx) => {
    if (!isZitadel) return;
    const err = validateNewPassword(v.password);
    if (err) {
      ctx.addIssue({ code: z.ZodIssueCode.custom, message: err, path: ["password"] });
    }
  })
  .refine((v) => v.password === v.confirm, {
    message: "Passwords don't match",
    path: ["confirm"],
  });

type FormValues = z.infer<typeof schema>;

interface ResetPasswordFormProps {
  oobCode: string;
}

export function ResetPasswordForm({ oobCode }: ResetPasswordFormProps) {
  const router = useRouter();
  // Canonical /login 404s without a valid returnUrl, so only link when
  // the page carries one we can hand straight back. See signInHref.
  const backHref = signInHref(useSearchParams().get("returnUrl"));
  const [submitError, setSubmitError] = useState<string | null>(null);
  const [done, setDone] = useState(false);
  const [pending, startTransition] = useTransition();

  const {
    register,
    handleSubmit,
    formState: { errors },
    setError,
  } = useForm<FormValues>({
    resolver: zodResolver(schema),
    mode: "onTouched",
    defaultValues: { password: "", confirm: "" },
  });

  // Missing code means the user landed here without a valid link.
  if (!oobCode) {
    return (
      <div className="w-full space-y-6">
        <div className="space-y-3">
          <p className="eyebrow">Reset password</p>
          <h1 className="font-serif text-4xl font-medium leading-[1.1] tracking-tight text-foreground">
            Link not recognised.
          </h1>
          <p className="max-w-md text-[15px] leading-relaxed text-foreground-secondary">
            This reset link is missing its code. Request a new one and we
            will email you a fresh link.
          </p>
        </div>
        <Link
          href="/forgot-password"
          className="inline-flex h-10 items-center rounded-md bg-[color:var(--ink-900)] px-5 text-sm font-medium text-white transition-colors hover:bg-[color:var(--ink-800)]"
        >
          Request new link
        </Link>
      </div>
    );
  }

  function onValid(values: FormValues) {
    setSubmitError(null);
    startTransition(async () => {
      const result = await confirmPasswordResetAction(oobCode, values.password);
      if (result.ok) {
        setDone(true);
        // Only bounce when we have a sign-in target middleware accepts.
        // Bare /login 404s on the canonical host, so without a valid
        // returnUrl we leave the success screen up (it explains where to
        // sign in) rather than redirecting the user into a dead page.
        if (backHref) {
          setTimeout(() => router.push(backHref), 1500);
        }
        return;
      }
      // A policy rejection belongs on the password field, where the fix
      // is, not in the generic form-level alert — same handling as
      // AcceptInviteForm. platform-api answers `password_policy` with the
      // specific broken rule (#682); `weak_password` is this action's own
      // pre-check. Client validation and the server can drift, and the
      // server is authoritative, so render whatever rule it names.
      if (result.code === "password_policy" || result.code === "weak_password") {
        setError("password", { type: "server", message: result.message });
        return;
      }
      setSubmitError(result.message);
    });
  }

  if (done) {
    return (
      <div className="w-full space-y-6">
        <div className="space-y-3">
          <p className="eyebrow">All set</p>
          <h1 className="font-serif text-4xl font-medium leading-[1.1] tracking-tight text-foreground">
            Password updated.
          </h1>
          <p className="max-w-md text-[15px] leading-relaxed text-foreground-secondary">
            You can now sign in with your new password.
          </p>
        </div>
        {backHref ? (
          <Link href={backHref} className="inline-flex h-10 items-center rounded-md bg-[color:var(--ink-900)] px-5 text-sm font-medium text-white transition-colors hover:bg-[color:var(--ink-800)]">
            Go to sign in
          </Link>
        ) : (
          <p className="max-w-md text-xs leading-relaxed text-foreground-secondary">Sign in from your store&rsquo;s own admin address &mdash; the one ending in <span className="whitespace-nowrap">-admin.mark8ly.com</span>.</p>
        )}
      </div>
    );
  }

  return (
    <div className="w-full space-y-8">
      <div className="space-y-3">
        <p className="eyebrow">Reset password</p>
        <h1 className="font-serif text-4xl font-medium leading-[1.1] tracking-tight text-foreground">
          Choose a new password.
        </h1>
        <p className="max-w-md text-[15px] leading-relaxed text-foreground-secondary">
          {isZitadel
            ? PASSWORD_REQUIREMENTS_TEXT
            : "Use at least 8 characters. We recommend a passphrase you haven\u2019t used elsewhere."}
        </p>
      </div>

      <form
        onSubmit={handleSubmit(onValid)}
        noValidate
        className="space-y-5"
      >
        <Field
          id="password"
          label="New password"
          error={errors.password?.message}
        >
          <Input
            id="password"
            type="password"
            autoComplete="new-password"
            disabled={pending}
            aria-invalid={errors.password ? true : undefined}
            {...register("password")}
          />
        </Field>

        <Field
          id="confirm"
          label="Confirm new password"
          error={errors.confirm?.message}
        >
          <Input
            id="confirm"
            type="password"
            autoComplete="new-password"
            disabled={pending}
            aria-invalid={errors.confirm ? true : undefined}
            {...register("confirm")}
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
          {pending ? "Saving…" : "Save new password"}
        </button>

        <div className="flex justify-center">
          {backHref ? (
            <Link href={backHref} className="text-xs text-foreground-secondary underline underline-offset-4 decoration-border-subtle transition-colors hover:text-foreground hover:decoration-foreground-tertiary">
              Back to sign in
            </Link>
          ) : (
            <p className="max-w-md text-xs leading-relaxed text-foreground-secondary">Sign in from your store&rsquo;s own admin address &mdash; the one ending in <span className="whitespace-nowrap">-admin.mark8ly.com</span>.</p>
          )}
        </div>
      </form>
    </div>
  );
}
