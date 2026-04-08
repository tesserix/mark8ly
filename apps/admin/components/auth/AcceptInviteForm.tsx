"use client";

import { useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { Input } from "@tesserix/web";
import { Field } from "@repo/ui/field";
import { GoogleMark } from "@repo/ui/google-mark";
import { RoleBadge } from "@repo/ui/role-badge";

import type { InvitationVerifyResult } from "@/lib/api/platform-api";
import { getGoogleCredential } from "@/lib/gip/google-gsi";
import {
  GIPError,
  signInWithGoogle,
  signInWithPassword,
  signUp,
} from "@/lib/gip/signup";
import { acceptInvite } from "@/app/accept-invite/actions";

type Mode = "signin" | "create";

interface AcceptInviteFormProps {
  token: string;
  invitation: InvitationVerifyResult;
}

const baseSchema = z.object({
  password: z.string().min(8, "Password must be at least 8 characters"),
  confirmPassword: z.string().optional(),
});

type FormValues = z.infer<typeof baseSchema>;

export function AcceptInviteForm({
  token,
  invitation,
}: AcceptInviteFormProps) {
  const router = useRouter();
  const [mode, setMode] = useState<Mode>("signin");
  const [submitError, setSubmitError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);
  const [pending, startTransition] = useTransition();
  const [googlePending, setGooglePending] = useState(false);

  const {
    register,
    handleSubmit,
    setError,
    reset,
    formState: { errors },
  } = useForm<FormValues>({
    resolver: zodResolver(baseSchema),
    mode: "onTouched",
    reValidateMode: "onChange",
    defaultValues: { password: "", confirmPassword: "" },
  });

  const disabled = pending || googlePending;

  function complete(idToken: string, uid: string, verifiedEmail: string) {
    startTransition(async () => {
      const result = await acceptInvite({
        token,
        idToken,
        uid,
        verifiedEmail,
      });
      if (!result.ok) {
        setSubmitError(result.message);
        return;
      }
      setSuccess("Invite accepted. Opening your store…");
      router.push("/dashboard");
      router.refresh();
    });
  }

  function onValid(values: FormValues) {
    setSubmitError(null);
    setSuccess(null);

    if (mode === "create" && values.password !== values.confirmPassword) {
      setError("confirmPassword", {
        type: "validate",
        message: "Passwords do not match",
      });
      return;
    }

    startTransition(async () => {
      try {
        const auth =
          mode === "create"
            ? await signUp(invitation.email, values.password)
            : await signInWithPassword(invitation.email, values.password);

        complete(auth.idToken, auth.uid, invitation.email);
      } catch (err) {
        const message = describeAuthError(err, mode);
        if (/password/i.test(message)) {
          setError("password", { type: "server", message });
        } else {
          setSubmitError(message);
        }
      }
    });
  }

  async function handleGoogle() {
    setSubmitError(null);
    setSuccess(null);
    setGooglePending(true);
    try {
      const { credential } = await getGoogleCredential();
      const auth = await signInWithGoogle(credential);
      const googleEmail = decodeJwtEmail(auth.idToken);
      if (
        googleEmail &&
        googleEmail.toLowerCase() !== invitation.email.toLowerCase()
      ) {
        setSubmitError(
          `This Google account (${googleEmail}) does not match the invite email (${invitation.email}).`,
        );
        return;
      }
      complete(auth.idToken, auth.uid, invitation.email);
    } catch (err) {
      setSubmitError(describeGoogleError(err));
    } finally {
      setGooglePending(false);
    }
  }

  function switchMode(next: Mode) {
    setMode(next);
    setSubmitError(null);
    setSuccess(null);
    reset({ password: "", confirmPassword: "" });
  }

  return (
    <div className="w-full max-w-md space-y-8">
      <div className="space-y-3">
        <div className="flex flex-wrap items-center gap-3">
          <RoleBadge role={invitation.role} />
          <span className="text-[11px] font-semibold uppercase tracking-[0.16em] text-foreground-tertiary">
            {invitation.tenant_slug}.mark8ly.com
          </span>
        </div>
        <p className="text-sm leading-7 text-foreground-secondary">
          Use{" "}
          <strong className="font-medium text-foreground">
            {invitation.email}
          </strong>{" "}
          to accept this invite. If you already have a Mark8ly account, sign in.
          Otherwise create one now and we&apos;ll attach it to the store.
        </p>
      </div>

      <div
        className="grid grid-cols-2 gap-0 border-y border-border-subtle"
        role="tablist"
        aria-label="Account mode"
      >
        {modeOptions.map((option) => {
          const selected = option.value === mode;
          return (
            <button
              key={option.value}
              type="button"
              role="tab"
              aria-selected={selected}
              onClick={() => switchMode(option.value)}
              className={`py-4 text-left text-sm font-medium transition-colors ${
                selected
                  ? "text-foreground border-b-2 border-moss-700 -mb-px"
                  : "text-foreground-tertiary hover:text-foreground"
              }`}
            >
              <p className="font-serif text-base">{option.title}</p>
              <p className="mt-1 text-xs font-normal leading-5 text-foreground-tertiary">
                {option.body}
              </p>
            </button>
          );
        })}
      </div>

      <form onSubmit={handleSubmit(onValid)} noValidate className="space-y-5">
        <Field id="invite-email" label="Email">
          <Input
            id="invite-email"
            type="email"
            value={invitation.email}
            readOnly
            disabled
          />
        </Field>

        <Field
          id="invite-password"
          label={mode === "create" ? "Create password" : "Password"}
          error={errors.password?.message}
        >
          <Input
            id="invite-password"
            type="password"
            placeholder="At least 8 characters"
            disabled={disabled}
            autoComplete={mode === "create" ? "new-password" : "current-password"}
            aria-invalid={errors.password ? true : undefined}
            aria-describedby={
              errors.password ? "invite-password-error" : undefined
            }
            {...register("password")}
          />
        </Field>

        {mode === "create" && (
          <Field
            id="invite-confirm-password"
            label="Confirm password"
            error={errors.confirmPassword?.message}
          >
            <Input
              id="invite-confirm-password"
              type="password"
              placeholder="Repeat password"
              disabled={disabled}
              autoComplete="new-password"
              aria-invalid={errors.confirmPassword ? true : undefined}
              aria-describedby={
                errors.confirmPassword
                  ? "invite-confirm-password-error"
                  : undefined
              }
              {...register("confirmPassword")}
            />
          </Field>
        )}

        {submitError && (
          <p role="alert" aria-live="polite" className="text-sm text-danger">
            {submitError}
          </p>
        )}

        {success && (
          <p role="status" aria-live="polite" className="text-sm text-moss-700">
            {success}
          </p>
        )}

        <div className="space-y-3 pt-1">
          <button
            type="submit"
            disabled={disabled}
            className="inline-flex h-12 w-full items-center justify-center rounded-md bg-primary px-6 text-base font-medium text-primary-foreground hover:bg-primary-hover disabled:cursor-not-allowed disabled:bg-ink-600"
          >
            {pending
              ? mode === "create"
                ? "Creating account…"
                : "Signing in…"
              : mode === "create"
                ? "Create account and join store"
                : "Sign in and accept invite"}
          </button>

          <div className="relative py-1">
            <div
              className="absolute inset-0 flex items-center"
              aria-hidden="true"
            >
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
        </div>
      </form>
    </div>
  );
}

const modeOptions: Array<{ value: Mode; title: string; body: string }> = [
  {
    value: "signin",
    title: "I have an account",
    body: "Sign in with the invite email.",
  },
  {
    value: "create",
    title: "Create an account",
    body: "First time joining Mark8ly.",
  },
];

function describeAuthError(err: unknown, mode: Mode): string {
  if (err instanceof GIPError) {
    if (err.code === "invalid_credentials") {
      return "Email or password is incorrect.";
    }
    if (err.code === "weak_password") {
      return "Password must be at least 8 characters.";
    }
    if (/EMAIL_EXISTS/.test(err.message) && mode === "create") {
      return "An account already exists for this email. Try signing in instead.";
    }
    return err.message;
  }
  return err instanceof Error ? err.message : "Something went wrong.";
}

function describeGoogleError(err: unknown): string {
  if (err instanceof Error) {
    return `Google sign-in failed: ${err.message}`;
  }
  return "Google sign-in failed.";
}

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
