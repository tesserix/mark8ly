"use client";

import { useState, useTransition } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { Input } from "@tesserix/web";
import { Field } from "@repo/ui/field";
import { RoleBadge } from "@repo/ui/role-badge";

import type { InvitationVerifyResult } from "@/lib/api/platform-api";
import { acceptInviteWithZitadel } from "@/app/accept-invite/actions";
import {
  PASSWORD_MIN_LENGTH,
  PASSWORD_REQUIREMENTS_TEXT,
  validateNewPassword,
} from "@/lib/auth/password-policy";

type Mode = "signin" | "create";

interface AcceptInviteFormProps {
  token: string;
  invitation: InvitationVerifyResult;
}

/**
 * The resolver floor. The real Zitadel policy (12 characters, upper,
 * lower, number, symbol — see lib/auth/password-policy.ts) is applied on
 * top of this in onValid rather than in the schema, so react-hook-form
 * keeps one stable resolver.
 */
const baseSchema = z.object({
  password: z.string().min(8, "Password must be at least 8 characters"),
  confirmPassword: z.string().optional(),
});

type FormValues = z.infer<typeof baseSchema>;

export function AcceptInviteForm({
  token,
  invitation,
}: AcceptInviteFormProps) {
  const [mode, setMode] = useState<Mode>("signin");
  const [submitError, setSubmitError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);
  const [pending, startTransition] = useTransition();

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

  const disabled = pending;

  /**
   * The Zitadel submit path. platform-api's accept endpoint provisions
   * the Zitadel user (from this password, when the address has no
   * account yet), the admin project grant, and both FGA tuples — so
   * there is nothing for the browser to create first.
   *
   * On success the browser is handed to /login/authorize with a
   * full-page navigation, not router.push: that route is a Route
   * Handler that writes the PKCE/state cookies and 302s to Zitadel, and
   * a client-side navigation would never reach it.
   */
  function acceptWithZitadel(password: string) {
    startTransition(async () => {
      const result = await acceptInviteWithZitadel({
        token,
        email: invitation.email,
        password,
      });
      if (!result.ok) {
        // The server is authoritative on the password policy: client
        // validation is a copy of it and the two can drift. When
        // platform-api names the rule that was broken, put its message
        // on the password field where the fix is, not in the generic
        // form-level alert.
        if (result.code === "password_policy") {
          setError("password", { type: "server", message: result.message });
          return;
        }
        setSubmitError(result.message);
        return;
      }
      setSuccess("Invite accepted. Taking you to sign in…");
      if (typeof window !== "undefined") {
        window.location.assign(result.signInUrl);
      }
    });
  }

  function onValid(values: FormValues) {
    setSubmitError(null);
    setSuccess(null);

    const policyError = validateNewPassword(values.password);
    if (policyError) {
      setError("password", { type: "validate", message: policyError });
      return;
    }

    if (mode === "create" && values.password !== values.confirmPassword) {
      setError("confirmPassword", {
        type: "validate",
        message: "Passwords do not match",
      });
      return;
    }

    acceptWithZitadel(values.password);
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

        {/* The requirements are shown as a hint BEFORE the first
            submit, not only as an error after one. A merchant choosing a
            password should not have to guess it and be corrected — that
            guessing loop is what the eleven-character password ran into.
            Field renders the error in the hint's place once there is
            one, so the two never stack. */}
        <Field
          id="invite-password"
          label={mode === "create" ? "Create password" : "Password"}
          error={errors.password?.message}
          hint={PASSWORD_REQUIREMENTS_TEXT}
        >
          <Input
            id="invite-password"
            type="password"
            placeholder={`At least ${PASSWORD_MIN_LENGTH} characters`}
            disabled={disabled}
            autoComplete="new-password"
            aria-invalid={errors.password ? true : undefined}
            aria-describedby={
              errors.password ? "invite-password-error" : "invite-password-hint"
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
            {submitLabel({ pending, mode })}
          </button>

          <p className="text-xs leading-5 text-foreground-tertiary">
            We&apos;ll take you to the sign-in screen once your account is
            ready.
          </p>
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

/**
 * Submit-button copy. This form never signs the invitee in — it
 * provisions the account and hands the browser to /login/authorize — so
 * "Sign in and accept invite" would promise something it does not do.
 */
function submitLabel({
  pending,
  mode,
}: {
  pending: boolean;
  mode: Mode;
}): string {
  if (pending) return "Setting up your account…";
  return mode === "create"
    ? "Create account and join store"
    : "Accept invite and continue";
}
