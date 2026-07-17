"use client";

import { useEffect, useMemo, useRef, useState, useTransition } from "react";
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
import { signupCopy } from "@/lib/copy/signup";

interface Props {
  countries: Country[];
  currencies: Currency[];
  timezones: Timezone[];
}

/* ============================================================
   Validation schema
   ============================================================ */

const SLUG_PATTERN = /^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$/;

// Migration fast-path (§5.1.1) is hidden from the UI until the flow is
// tested end-to-end — everyone signs up as a new store. Flip to true to
// restore the question; all validation and submit wiring stays intact.
const MIGRATION_UI_ENABLED = false;

// §5.1.1: when migration_type === "migrating", at least one of
// whois_url or screenshot_file must be present.
const schema = z
  .object({
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
    taxId: z.string().optional(),
    migrationType: z.enum(["new", "migrating"]),
    whoisUrl: z.string().optional(),
    // screenshotFile is handled outside RHF via a ref; its upload
    // result (a URL string) is stored in screenshotUrl.
    screenshotUrl: z.string().optional(),
  })
  .superRefine((data, ctx) => {
    if (data.migrationType === "migrating") {
      const hasWhois = Boolean(data.whoisUrl?.trim());
      const hasScreenshot = Boolean(data.screenshotUrl?.trim());
      if (!hasWhois && !hasScreenshot) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          message: signupCopy.migrationRequiredError,
          path: ["whoisUrl"],
        });
      }
    }
  });

type FormValues = z.infer<typeof schema>;

/* ============================================================
   Async slug availability
   ============================================================ */

type SlugAvailability =
  | { state: "idle" }
  | { state: "checking" }
  | { state: "available" }
  | { state: "taken" }
  | { state: "invalid"; message: string };

/* ============================================================
   Screenshot upload stub
   ============================================================
   The onboarding app does not yet have its own GCS upload helper.
   TODO: once apps/onboarding gains a /api/upload route backed by
   @google-cloud/storage (mirror of apps/admin/lib/brandingUpload.ts),
   replace uploadScreenshot below with the real implementation.
   ============================================================ */

async function uploadScreenshot(_file: File): Promise<string> {
  // TODO: implement GCS upload for onboarding app.
  // Expected shape: POST /api/upload/screenshot → { url: string }
  // For now, return a placeholder so the form can still be submitted
  // and the backend receives a non-empty marker.
  return `pending:${_file.name}`;
}

/**
 * OnboardingForm — single-page signup form extended with:
 *   - optional tax ID field with country-aware help text (§5.2)
 *   - migration fast-path evidence: WHOIS URL or screenshot upload (§5.1.1)
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
  const [screenshotUploading, setScreenshotUploading] = useState(false);
  const [screenshotFileName, setScreenshotFileName] = useState<string | null>(
    null,
  );
  const screenshotInputRef = useRef<HTMLInputElement>(null);

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
    mode: "onTouched",
    reValidateMode: "onChange",
    defaultValues: {
      email: "",
      businessName: "",
      slug: "",
      countryCode: "",
      currencyCode: "",
      taxId: "",
      migrationType: "new",
      whoisUrl: "",
      screenshotUrl: "",
    },
  });

  const watchedBusinessName = watch("businessName");
  const watchedSlug = watch("slug");
  const watchedCountry = watch("countryCode");
  const watchedMigrationType = watch("migrationType");
  const isMigrating = watchedMigrationType === "migrating";

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

  // Country-aware tax ID help text.
  const taxIdHelp = useMemo(() => {
    if (!watchedCountry) return signupCopy.taxIdHelpFallback;
    return (
      signupCopy.taxIdHelpByCountry[watchedCountry] ??
      signupCopy.taxIdHelpFallback
    );
  }, [watchedCountry]);

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

  // Clear migration evidence when switching back to "new store".
  useEffect(() => {
    if (!isMigrating) {
      setValue("whoisUrl", "");
      setValue("screenshotUrl", "");
      setScreenshotFileName(null);
    }
  }, [isMigrating, setValue]);

  async function handleScreenshotChange(
    e: React.ChangeEvent<HTMLInputElement>,
  ) {
    const file = e.target.files?.[0];
    if (!file) return;
    setScreenshotUploading(true);
    try {
      const url = await uploadScreenshot(file);
      setValue("screenshotUrl", url, { shouldValidate: true });
      setScreenshotFileName(file.name);
    } finally {
      setScreenshotUploading(false);
    }
  }

  // Only disable while genuinely busy. Gating the button on slug
  // availability left a mystery-disabled submit on first load (NN/g:
  // disabled buttons must explain themselves) — instead, submitting
  // with an unavailable slug surfaces the error and focuses the field.
  const canSubmit = !pending && !screenshotUploading;

  function onValid(values: FormValues) {
    setSubmitError(null);

    if (slugAvailability.state !== "available") {
      setSubmitError("Please pick an available store URL.");
      document.getElementById("slug")?.focus();
      return;
    }

    const payload = {
      email: values.email.trim().toLowerCase(),
      businessName: values.businessName.trim(),
      slug: values.slug,
      countryCode: values.countryCode,
      currencyCode: values.currencyCode,
      timezone: resolvedTimezone,
      taxId: values.taxId?.trim() || undefined,
      migrationType: values.migrationType,
      whoisUrl:
        values.migrationType === "migrating"
          ? values.whoisUrl?.trim() || undefined
          : undefined,
      screenshotUrl:
        values.migrationType === "migrating"
          ? values.screenshotUrl?.trim() || undefined
          : undefined,
    };

    startTransition(async () => {
      const r = await submitOnboarding(payload);
      if (!r.ok) {
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
        taxId: payload.taxId ?? "",
        migrationType: payload.migrationType,
        whoisUrl: payload.whoisUrl ?? "",
        screenshotUrl: payload.screenshotUrl ?? "",
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

        {/* Slug */}
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

        {/* ── Tax ID (§5.2) ─────────────────────────────────── */}
        <div className="border-t border-border-subtle pt-5">
          <Field
            id="taxId"
            label={signupCopy.taxIdLabel}
            hint={taxIdHelp}
            hintState="default"
            error={errors.taxId?.message}
          >
            <Input
              id="taxId"
              type="text"
              placeholder="e.g. GB123456789"
              autoComplete="off"
              spellCheck={false}
              aria-invalid={errors.taxId ? true : undefined}
              aria-describedby={
                errors.taxId ? "taxId-error" : "taxId-hint"
              }
              {...register("taxId")}
            />
          </Field>
        </div>

        {/* ── Migration fast-path (§5.1.1) ──────────────────── */}
        {MIGRATION_UI_ENABLED && (
        <fieldset className="space-y-3">
          <legend className="text-sm font-medium text-foreground">
            {signupCopy.migrationHeading}
          </legend>

          <label className="flex cursor-pointer items-start gap-3">
            <input
              type="radio"
              value="new"
              className="mt-0.5 h-4 w-4 shrink-0 accent-moss-700"
              aria-label={signupCopy.migrationNewOption}
              {...register("migrationType")}
            />
            <span className="text-sm text-foreground">
              {signupCopy.migrationNewOption}
            </span>
          </label>

          <label className="flex cursor-pointer items-start gap-3">
            <input
              type="radio"
              value="migrating"
              className="mt-0.5 h-4 w-4 shrink-0 accent-moss-700"
              aria-label={signupCopy.migrationMigratingOption}
              {...register("migrationType")}
            />
            <span className="text-sm text-foreground">
              {signupCopy.migrationMigratingOption}
            </span>
          </label>

          {/* Always mounted; grid-rows 0fr→1fr glides the evidence panel
              open instead of jump-cutting it into the layout. */}
          <div
            className={`grid transition-[grid-template-rows] duration-300 ease-out ${
              isMigrating ? "grid-rows-[1fr]" : "grid-rows-[0fr]"
            }`}
            inert={!isMigrating || undefined}
            aria-hidden={!isMigrating}
          >
            <div className="overflow-hidden">
            <div
              className="mt-3 space-y-4 border-l-2 border-moss-200 pl-4 pb-1"
              data-testid="migration-evidence-panel"
            >
              {/* WHOIS URL */}
              <Field
                id="whoisUrl"
                label={signupCopy.migrationWhoisLabel}
                hint={signupCopy.migrationWhoisHelp}
                hintState="default"
                error={errors.whoisUrl?.message}
              >
                <Input
                  id="whoisUrl"
                  type="url"
                  placeholder="https://yourstore.com"
                  autoComplete="url"
                  spellCheck={false}
                  aria-invalid={errors.whoisUrl ? true : undefined}
                  aria-describedby={
                    errors.whoisUrl ? "whoisUrl-error" : "whoisUrl-hint"
                  }
                  {...register("whoisUrl")}
                />
              </Field>

              {/* Screenshot upload */}
              <div className="space-y-1.5">
                <Label htmlFor="screenshotFile" className="text-foreground">
                  {signupCopy.migrationScreenshotLabel}
                </Label>

                <div className="flex items-center gap-3">
                  <button
                    type="button"
                    onClick={() => screenshotInputRef.current?.click()}
                    disabled={screenshotUploading}
                    className="inline-flex h-9 items-center rounded-md border border-border bg-background-elevated px-4 text-sm font-medium text-foreground hover:bg-paper-300 disabled:cursor-not-allowed disabled:opacity-50"
                  >
                    {screenshotUploading ? "Uploading…" : "Choose file"}
                  </button>
                  {screenshotFileName && (
                    <span className="truncate text-sm text-foreground-secondary">
                      {screenshotFileName}
                    </span>
                  )}
                </div>

                {/* Hidden file input — controlled via ref */}
                <input
                  ref={screenshotInputRef}
                  id="screenshotFile"
                  type="file"
                  accept="image/*"
                  className="sr-only"
                  aria-label={signupCopy.migrationScreenshotLabel}
                  onChange={handleScreenshotChange}
                />

                <p
                  id="screenshotFile-hint"
                  className="text-xs text-foreground-tertiary"
                >
                  {signupCopy.migrationScreenshotHelp}
                </p>
              </div>

              {/* Cross-field validation: no evidence provided */}
              {errors.whoisUrl?.type === "custom" && (
                <p
                  role="alert"
                  aria-live="polite"
                  className="text-xs text-danger"
                  data-testid="migration-required-error"
                >
                  {errors.whoisUrl.message}
                </p>
              )}
            </div>
            </div>
          </div>
        </fieldset>
        )}

        {/* Server-level submit error */}
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
   Slug helpers
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
   routeServerError
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
  if (/tax.?id|vat|gstin|ein/.test(m)) {
    return { field: "taxId", message };
  }

  return null;
}

/* ============================================================
   slugify
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
