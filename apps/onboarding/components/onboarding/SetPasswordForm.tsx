"use client";

// SetPasswordForm — Phase M.
//
// Shown after the merchant clicks the magic link and the session has
// been marked email-verified. The merchant picks a name and a password;
// completeOnboardingWithZitadel hands both to platform-api, which
// provisions the Zitadel user, the admin project grant and both FGA
// owner tuples, then we redirect to /welcome.
//
// The name lives on this step rather than the first-page signup form
// because this is where the account is actually created — collecting it
// earlier would mean carrying it through the server action, the
// onboarding session record and the magic-link round trip for no
// benefit.
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

import { completeOnboardingWithZitadel } from "@/app/onboarding/actions";
import {
  PASSWORD_REQUIREMENTS_TEXT,
  validateNewPassword,
} from "@/lib/auth/password-policy";
import { useOnboardingStore } from "@/lib/store/onboarding-store";

interface Props {
  sessionId: string;
  email: string;
  businessName: string;
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
  // The resolver floor. The real Zitadel policy (12 characters, upper,
  // lower, number, symbol — see lib/auth/password-policy.ts) is applied
  // on top of this in onValid, so react-hook-form keeps one stable
  // resolver. A client rule LOOSER than the server's is worse than no
  // rule at all — it promises an acceptance the server will refuse —
  // which is exactly what min(8) was doing here (#685).
  password: z
    .string()
    .min(1, "Password is required")
    .min(8, "Password must be at least 8 characters"),
});

type FormValues = z.infer<typeof schema>;

export function SetPasswordForm({ sessionId, email }: Props) {
  const router = useRouter();
  const setSubmitted = useOnboardingStore((s) => s.setSubmitted);
  const storedSlug = useOnboardingStore((s) => s.slug);
  const [submitError, setSubmitError] = useState<string | null>(null);
  const [pending, startTransition] = useTransition();

  const {
    register,
    handleSubmit,
    setError,
    formState: { errors },
  } = useForm<FormValues>({
    resolver: zodResolver(schema),
    mode: "onTouched",
    reValidateMode: "onChange",
    defaultValues: { name: "", password: "" },
  });

  /**
   * Finish the wizard.
   *
   * platform-api's complete endpoint provisions the Zitadel user from
   * this password, ensures the mark8ly-admin project grant, and writes
   * both FGA owner tuples — so there is nothing for the browser to create
   * first.
   *
   * No session is minted here. See completeOnboardingWithZitadel's doc
   * for why: the admin is on a different origin and Zitadel's
   * login-client model has no session this page could mint. The merchant
   * signs in once, on the admin, with the password they just chose.
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

    const policyError = validateNewPassword(values.password);
    if (policyError) {
      setError("password", { type: "validate", message: policyError });
      return;
    }
    completeWithZitadel(values);
  }

  const disabled = pending;

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
            placeholder="At least 12 characters"
            autoComplete="new-password"
            disabled={disabled}
            aria-invalid={errors.password ? true : undefined}
            aria-describedby={
              errors.password ? "password-error" : "password-requirements"
            }
            {...register("password")}
          />
          {/* The whole policy, shown BEFORE the first submit. A merchant
              choosing a password should not have to guess and be
              corrected one rule at a time — that drip-feed is what the
              incident behind lib/auth/password-policy.ts was made of. */}
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
            ) : (
              <p
                id="password-requirements"
                className="text-xs text-foreground-tertiary"
              >
                {PASSWORD_REQUIREMENTS_TEXT}
              </p>
            )}
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

      </form>
    </div>
  );
}
