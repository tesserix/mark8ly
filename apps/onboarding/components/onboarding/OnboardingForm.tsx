"use client";

import { useEffect, useMemo, useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import { useForm, Controller } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import {
  Input,
  Label,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@tesserix/web";

import type { Country, Currency, Timezone } from "@/lib/types";
import { useOnboardingStore } from "@/lib/store/onboarding-store";
import { checkSlug, submitOnboarding } from "@/app/onboarding/actions";

interface Props {
  countries: Country[];
  currencies: Currency[];
  timezones: Timezone[];
}

/* ============================================================
   Validation schema
   ------------------------------------------------------------
   Single source of truth for client-side validation. Runs on
   blur the first time a field is touched, then on every change
   until the field is valid (mode: "onTouched" + reValidateMode).
   ============================================================ */

const SLUG_PATTERN = /^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$/;

const schema = z.object({
  email: z
    .string()
    .min(1, "Email is required")
    .email("Enter a valid email address"),
  businessName: z
    .string()
    .min(1, "Business name is required")
    .min(2, "Business name is too short")
    .max(80, "Business name is too long"),
  slug: z
    .string()
    .min(3, "3-63 characters, lowercase letters, numbers, and hyphens")
    .max(63, "Must be 63 characters or fewer")
    .regex(SLUG_PATTERN, "Lowercase letters, numbers, and hyphens only"),
  countryCode: z.string().min(1, "Please select a country"),
  currencyCode: z.string().min(1, "Please select a currency"),
});

type FormValues = z.infer<typeof schema>;

/* ============================================================
   Async slug availability — runs alongside RHF validation.
   The schema enforces format; this hook enforces uniqueness.
   ============================================================ */

type SlugAvailability =
  | { state: "idle" }
  | { state: "checking" }
  | { state: "available" }
  | { state: "taken" }
  | { state: "invalid"; message: string };

/**
 * OnboardingForm — single-page signup using react-hook-form + zod
 * for client validation, with inline field-level errors.
 *
 * Phase M: collects business details + email, sends a magic link,
 * and hands off to /onboarding/check-inbox. The credential step
 * runs on /onboarding/set-password after the link is clicked.
 */
export function OnboardingForm({ countries, currencies, timezones }: Props) {
  const router = useRouter();
  const setSubmitted = useOnboardingStore((s) => s.setSubmitted);

  const [submitError, setSubmitError] = useState<string | null>(null);
  const [pending, startTransition] = useTransition();
  const [slugAvailability, setSlugAvailability] = useState<SlugAvailability>({
    state: "idle",
  });
  const [slugTouched, setSlugTouched] = useState(false);

  const {
    register,
    control,
    handleSubmit,
    watch,
    setValue,
    setError: setFieldError,
    formState: { errors, touchedFields },
  } = useForm<FormValues>({
    resolver: zodResolver(schema),
    mode: "onTouched", // validate on blur first
    reValidateMode: "onChange", // then on every change until valid
    defaultValues: {
      email: "",
      businessName: "",
      slug: "",
      countryCode: "",
      currencyCode: "",
    },
  });

  const watchedBusinessName = watch("businessName");
  const watchedSlug = watch("slug");
  const watchedCountry = watch("countryCode");

  // Browser timezone, auto-detected once on mount.
  const browserTimezone = useMemo(() => {
    try {
      return Intl.DateTimeFormat().resolvedOptions().timeZone;
    } catch {
      return "UTC";
    }
  }, []);
  const resolvedTimezone = useMemo(() => {
    const exact = timezones.find((tz) => tz.id === browserTimezone);
    return exact?.id ?? timezones[0]?.id ?? "UTC";
  }, [browserTimezone, timezones]);

  // Auto-suggest slug from business name until the user touches the slug field.
  useEffect(() => {
    if (slugTouched) return;
    setValue("slug", slugify(watchedBusinessName), {
      shouldValidate: touchedFields.slug === true,
    });
  }, [watchedBusinessName, slugTouched, setValue, touchedFields.slug]);

  // Auto-derive currency from selected country.
  useEffect(() => {
    if (!watchedCountry) return;
    const country = countries.find((c) => c.code === watchedCountry);
    if (country?.currency_code) {
      setValue("currencyCode", country.currency_code, { shouldValidate: true });
    }
  }, [watchedCountry, countries, setValue]);

  // Debounced slug availability check — only runs for well-formed slugs.
  useEffect(() => {
    if (!watchedSlug || watchedSlug.length < 3) {
      setSlugAvailability({ state: "idle" });
      return;
    }
    if (!SLUG_PATTERN.test(watchedSlug)) {
      setSlugAvailability({ state: "idle" });
      return;
    }
    setSlugAvailability({ state: "checking" });
    const handle = setTimeout(async () => {
      const r = await checkSlug(watchedSlug);
      if (!r.ok) {
        setSlugAvailability({ state: "invalid", message: r.message });
        return;
      }
      setSlugAvailability({
        state: r.data.available ? "available" : "taken",
      });
    }, 300);
    return () => clearTimeout(handle);
  }, [watchedSlug]);

  const canSubmit = slugAvailability.state === "available" && !pending;

  function onValid(values: FormValues) {
    setSubmitError(null);

    // Final guard: schema passed but slug isn't marked available yet.
    if (slugAvailability.state !== "available") {
      setSubmitError("Please pick an available store URL.");
      return;
    }

    const payload = {
      email: values.email.trim().toLowerCase(),
      businessName: values.businessName.trim(),
      slug: values.slug,
      countryCode: values.countryCode,
      currencyCode: values.currencyCode,
      timezone: resolvedTimezone,
    };

    startTransition(async () => {
      const r = await submitOnboarding(payload);
      if (!r.ok) {
        // Route field-specific server errors to the matching field
        // so they render inline beside the input that caused them.
        // Fall back to a top-level banner for everything else.
        const routed = routeServerError(r.message);
        if (routed) {
          setFieldError(routed.field, {
            type: "server",
            message: routed.message,
          });
        } else {
          setSubmitError(r.message);
        }
        return;
      }
      setSubmitted({
        email: payload.email,
        sessionId: r.data.sessionId,
        businessName: payload.businessName,
        slug: payload.slug,
        countryCode: payload.countryCode,
        currencyCode: payload.currencyCode,
        timezone: resolvedTimezone,
      });
      router.push("/onboarding/check-inbox");
    });
  }

  return (
    <div className="w-full max-w-lg mx-auto lg:mx-0">
      <form
        onSubmit={handleSubmit(onValid)}
        noValidate
        className="space-y-5"
      >
        {/* Email */}
        <Field
          id="email"
          label="Email address"
          error={errors.email?.message}
        >
          <Input
            id="email"
            type="email"
            placeholder="founder@yourbusiness.com"
            autoComplete="email"
            spellCheck={false}
            aria-invalid={errors.email ? true : undefined}
            aria-describedby={errors.email ? "email-error" : undefined}
            {...register("email")}
          />
        </Field>

        {/* Business name */}
        <Field
          id="businessName"
          label="Business name"
          error={errors.businessName?.message}
        >
          <Input
            id="businessName"
            type="text"
            placeholder="Acme Co"
            autoComplete="organization"
            aria-invalid={errors.businessName ? true : undefined}
            aria-describedby={
              errors.businessName ? "businessName-error" : undefined
            }
            {...register("businessName")}
          />
        </Field>

        {/* Slug — format via zod, availability via async check */}
        <Field
          id="slug"
          label="Store URL"
          error={
            errors.slug?.message ?? slugError(slugAvailability) ?? undefined
          }
          hint={slugHint(slugAvailability, errors.slug?.message)}
          hintState={slugHintState(slugAvailability)}
        >
          <div className="flex items-stretch overflow-hidden rounded-md border border-border bg-background-elevated focus-within:border-ring focus-within:ring-2 focus-within:ring-ring/20">
            <input
              id="slug"
              type="text"
              placeholder="acme"
              spellCheck={false}
              autoComplete="off"
              aria-invalid={
                errors.slug || slugAvailability.state === "taken"
                  ? true
                  : undefined
              }
              aria-describedby="slug-error slug-hint"
              {...register("slug", {
                onChange: (e) => {
                  setSlugTouched(true);
                  e.target.value = e.target.value.toLowerCase();
                },
              })}
              className="flex-1 bg-transparent px-3 py-2.5 text-sm text-foreground focus:outline-none"
            />
            <span className="flex items-center border-l border-border bg-paper-100 px-3 text-sm font-medium text-foreground-secondary">
              .mark8ly.com
            </span>
          </div>
        </Field>

        {/* Country + Currency */}
        <div className="grid grid-cols-2 gap-3">
          <Field
            id="country"
            label="Country"
            error={errors.countryCode?.message}
          >
            <Controller
              control={control}
              name="countryCode"
              render={({ field }) => (
                <Select value={field.value} onValueChange={field.onChange}>
                  <SelectTrigger
                    id="country"
                    aria-invalid={errors.countryCode ? true : undefined}
                  >
                    <SelectValue placeholder="Select…" />
                  </SelectTrigger>
                  <SelectContent>
                    {countries.map((c) => (
                      <SelectItem key={c.code} value={c.code}>
                        {c.flag_emoji} {c.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )}
            />
          </Field>

          <Field
            id="currency"
            label="Currency"
            error={errors.currencyCode?.message}
          >
            <Controller
              control={control}
              name="currencyCode"
              render={({ field }) => (
                <Select value={field.value} onValueChange={field.onChange}>
                  <SelectTrigger
                    id="currency"
                    aria-invalid={errors.currencyCode ? true : undefined}
                  >
                    <SelectValue placeholder="Select…" />
                  </SelectTrigger>
                  <SelectContent>
                    {currencies.map((c) => (
                      <SelectItem key={c.code} value={c.code}>
                        {c.code} — {c.symbol}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )}
            />
          </Field>
        </div>

        {/* Server-level submit error (network, slug collision race, etc.) */}
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
          disabled={!canSubmit}
          className="inline-flex h-12 w-full items-center justify-center rounded-md bg-primary px-6 text-base font-medium text-primary-foreground hover:bg-primary-hover disabled:cursor-not-allowed disabled:bg-ink-600"
        >
          {pending ? (
            <span aria-live="polite">Sending verification link…</span>
          ) : (
            "Send verification link"
          )}
        </button>

        <p className="text-xs text-foreground-tertiary">
          By creating an account you agree to our{" "}
          <a
            href="/terms"
            className="text-foreground underline decoration-moss-700 decoration-2 underline-offset-4 hover:text-moss-700"
          >
            Terms
          </a>
          ,{" "}
          <a
            href="/privacy"
            className="text-foreground underline decoration-moss-700 decoration-2 underline-offset-4 hover:text-moss-700"
          >
            Privacy Policy
          </a>
          , and{" "}
          <a
            href="/legal"
            className="text-foreground underline decoration-moss-700 decoration-2 underline-offset-4 hover:text-moss-700"
          >
            Security Policy
          </a>
          .
        </p>
      </form>
    </div>
  );
}

/* ============================================================
   Field — label + control + reserved-space error row
   ------------------------------------------------------------
   The error <p> always renders (with empty content when there
   is no error) so the form never jitters when an error appears
   or disappears. `hint` is an optional helper line used only
   by the slug field for the live "available/checking" status.
   ============================================================ */

interface FieldProps {
  id: string;
  label: string;
  error?: string;
  hint?: string;
  hintState?: "default" | "muted" | "success" | "error";
  children: React.ReactNode;
}

function Field({ id, label, error, hint, hintState, children }: FieldProps) {
  const hintColor =
    hintState === "success"
      ? "text-moss-700"
      : hintState === "error"
        ? "text-danger"
        : "text-foreground-tertiary";

  return (
    <div className="space-y-1.5">
      <Label htmlFor={id} className="text-foreground">
        {label}
      </Label>
      {children}
      <div className="min-h-[1.125rem]">
        {error ? (
          <p
            id={`${id}-error`}
            role="alert"
            aria-live="polite"
            className="text-xs text-danger"
          >
            {error}
          </p>
        ) : hint ? (
          <p
            id={`${id}-hint`}
            aria-live="polite"
            className={`text-xs ${hintColor}`}
          >
            {hint}
          </p>
        ) : null}
      </div>
    </div>
  );
}

/* ============================================================
   Slug helpers — translate SlugAvailability into the bits
   needed by the Field component.
   ============================================================ */

function slugError(status: SlugAvailability): string | null {
  if (status.state === "taken") return "That URL is already taken";
  if (status.state === "invalid") return status.message;
  return null;
}

function slugHint(
  status: SlugAvailability,
  schemaError: string | undefined,
): string | undefined {
  // When the schema has a format error, don't compete with it.
  if (schemaError) return undefined;
  if (status.state === "idle") {
    return "3-63 characters · lowercase letters, numbers, and hyphens";
  }
  if (status.state === "checking") return "Checking availability…";
  if (status.state === "available") return "✓ Available";
  return undefined;
}

function slugHintState(
  status: SlugAvailability,
): "default" | "muted" | "success" | "error" {
  if (status.state === "available") return "success";
  if (status.state === "taken" || status.state === "invalid") return "error";
  return "default";
}

/* ============================================================
   routeServerError — translate a server-side error message into
   a field-level error when we can recognise it, or return null
   to fall back to the top-level banner. Heuristic-only; safe to
   extend as the platform-api error codes stabilise.
   ============================================================ */

function routeServerError(
  message: string,
): { field: keyof FormValues; message: string } | null {
  const m = message.toLowerCase();

  if (/slug|store url|already taken|unavailable/.test(m)) {
    return { field: "slug", message };
  }
  if (/email|already exists|already in use/.test(m)) {
    return { field: "email", message };
  }
  if (/business name/.test(m)) {
    return { field: "businessName", message };
  }
  if (/country/.test(m)) {
    return { field: "countryCode", message };
  }
  if (/currency/.test(m)) {
    return { field: "currencyCode", message };
  }

  return null;
}

/* ============================================================
   slugify — mirrors the legacy auto-suggest behavior.
   ============================================================ */

function slugify(input: string): string {
  return input
    .toLowerCase()
    .trim()
    .replace(/[^a-z0-9\s-]/g, "")
    .replace(/\s+/g, "-")
    .replace(/-+/g, "-")
    .replace(/^-|-$/g, "")
    .slice(0, 63);
}
