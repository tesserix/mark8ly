"use client";

import { useEffect, useState, useTransition } from "react";
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

import type { Country, Currency, Store } from "@/lib/api/platform-api";
import {
  checkStoreSlug,
  createNewStore,
  switchToStore,
} from "@/app/(admin)/settings/stores/actions";
import { useToast } from "@/components/feedback/Toaster";

interface StoresListProps {
  stores: Store[];
  currentStoreId: string;
  canManage: boolean;
  countries: Country[];
  currencies: Currency[];
}

const SLUG_PATTERN = /^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$/;

const schema = z.object({
  name: z
    .string()
    .min(1, "Store name is required")
    .min(2, "Store name is too short")
    .max(200, "Store name must be 200 characters or fewer"),
  slug: z
    .string()
    .min(3, "3-63 characters, lowercase letters, numbers, and hyphens")
    .max(63, "Must be 63 characters or fewer")
    .regex(SLUG_PATTERN, "Lowercase letters, numbers, and hyphens only"),
  countryCode: z.string().min(1, "Please select a country"),
  currencyCode: z.string().min(1, "Please select a currency"),
});

type FormValues = z.infer<typeof schema>;

type SlugAvailability =
  | { state: "idle" }
  | { state: "checking" }
  | { state: "available" }
  | { state: "taken" }
  | { state: "invalid"; message: string };

export function StoresList({
  stores,
  currentStoreId,
  canManage,
  countries,
  currencies,
}: StoresListProps) {
  const { toast } = useToast();
  const [pending, startTransition] = useTransition();
  const [showAdd, setShowAdd] = useState(false);
  const [switchError, setSwitchError] = useState<string | null>(null);

  function handleSwitch(storeId: string) {
    setSwitchError(null);
    startTransition(async () => {
      const res = await switchToStore(storeId);
      if (!res.ok) {
        setSwitchError(res.message);
        toast.error("Couldn't switch store", res.message);
        return;
      }
      // Admin is hosted at {store-slug}-admin.mark8ly.com, so switching
      // stores needs to redirect to the target store's subdomain — a
      // plain reload would keep you on the old store's host and the
      // middleware would switch you back.
      if (typeof window !== "undefined") {
        const host = window.location.host;
        const rootDomain = host
          .replace(/^[^.]+-admin\./, "")
          .replace(/^admin\./, "");
        const isProdLike = rootDomain.includes("mark8ly.com");
        const target = stores.find((s) => s.id === storeId);
        if (target?.slug && isProdLike) {
          window.location.href = `https://${target.slug}-admin.${rootDomain}/dashboard`;
          return;
        }
      }
      window.location.reload();
    });
  }

  return (
    <div className="space-y-6">
      <section className="admin-panel space-y-4 rounded-[1.6rem] p-6">
        <div className="flex items-center justify-between">
          <h2 className="text-sm font-semibold uppercase tracking-[0.16em] text-muted-foreground">
            Your stores
          </h2>
          {canManage && !showAdd && (
            <button
              type="button"
              onClick={() => setShowAdd(true)}
              className="inline-flex items-center justify-center rounded-md border border-border bg-background-elevated/70 px-3 py-1.5 text-xs font-medium text-foreground hover:bg-background-elevated"
            >
              + Add store
            </button>
          )}
        </div>

        {stores.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            You don&apos;t have any stores yet.
          </p>
        ) : (
          <ul className="divide-y divide-border/60">
            {stores.map((s) => {
              const isCurrent = s.id === currentStoreId;
              return (
                <li
                  key={s.id}
                  data-testid="store-row"
                  className="flex items-center justify-between gap-4 py-4"
                >
                  <div className="min-w-0">
                    <p className="truncate text-sm font-medium text-foreground">
                      {s.name}
                    </p>
                    <p className="truncate text-xs text-muted-foreground">
                      <a
                        href={`https://${s.slug}.mark8ly.com`}
                        target="_blank"
                        rel="noreferrer noopener"
                        className="underline-offset-2 hover:text-foreground hover:underline"
                      >
                        {s.slug}.mark8ly.com
                      </a>
                      {" · "}
                      {s.currency_code}
                    </p>
                  </div>
                  {isCurrent ? (
                    <span className="rounded-full border border-emerald-200 bg-emerald-50 px-2.5 py-1 text-xs font-medium text-emerald-800">
                      Current
                    </span>
                  ) : (
                    <button
                      type="button"
                      onClick={() => handleSwitch(s.id)}
                      disabled={pending}
                      className="rounded-md border border-border px-3 py-1.5 text-xs font-medium text-muted-foreground transition-colors hover:border-foreground hover:text-foreground disabled:cursor-not-allowed disabled:opacity-50"
                    >
                      Switch
                    </button>
                  )}
                </li>
              );
            })}
          </ul>
        )}
      </section>

      {canManage && showAdd && (
        <AddStoreForm
          countries={countries}
          currencies={currencies}
          onCancel={() => setShowAdd(false)}
        />
      )}

      {switchError && !showAdd && (
        <div
          role="alert"
          className="rounded-md border border-destructive/20 bg-destructive/5 px-4 py-3 text-sm text-destructive"
        >
          {switchError}
        </div>
      )}
    </div>
  );
}

interface AddStoreFormProps {
  countries: Country[];
  currencies: Currency[];
  onCancel: () => void;
}

function AddStoreForm({
  countries,
  currencies,
  onCancel,
}: AddStoreFormProps) {
  const { toast } = useToast();
  const [pending, startTransition] = useTransition();
  const [submitError, setSubmitError] = useState<string | null>(null);
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
    setError,
    formState: { errors, touchedFields },
  } = useForm<FormValues>({
    resolver: zodResolver(schema),
    mode: "onTouched",
    reValidateMode: "onChange",
    defaultValues: {
      name: "",
      slug: "",
      countryCode: "",
      currencyCode: "",
    },
  });

  const watchedName = watch("name");
  const watchedSlug = watch("slug");
  const watchedCountry = watch("countryCode");

  // Auto-suggest slug from store name until the merchant edits the slug.
  useEffect(() => {
    if (slugTouched) return;
    setValue("slug", slugify(watchedName), {
      shouldValidate: touchedFields.slug === true,
    });
  }, [watchedName, slugTouched, setValue, touchedFields.slug]);

  // Auto-derive currency from selected country.
  useEffect(() => {
    if (!watchedCountry) return;
    const country = countries.find((c) => c.code === watchedCountry);
    if (country?.currency_code) {
      setValue("currencyCode", country.currency_code, { shouldValidate: true });
    }
  }, [watchedCountry, countries, setValue]);

  // Debounced slug availability.
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
      const r = await checkStoreSlug(watchedSlug);
      if (!r.ok) {
        setSlugAvailability({ state: "invalid", message: r.message });
        return;
      }
      setSlugAvailability({
        state: r.available ? "available" : "taken",
      });
    }, 300);
    return () => clearTimeout(handle);
  }, [watchedSlug]);

  const canSubmit = slugAvailability.state === "available" && !pending;

  function onValid(values: FormValues) {
    setSubmitError(null);

    if (slugAvailability.state !== "available") {
      setSubmitError("Please pick an available store URL.");
      return;
    }

    // Auto-detect merchant's timezone from the browser. Matches onboarding's
    // silent behaviour — merchants never type IANA identifiers.
    const timezone = resolveBrowserTimezone();

    startTransition(async () => {
      const res = await createNewStore({
        slug: values.slug,
        name: values.name.trim(),
        country_code: values.countryCode,
        currency_code: values.currencyCode,
        timezone,
      });
      if (!res.ok) {
        const routed = routeServerError(res.message);
        if (routed) {
          setError(routed.field, { type: "server", message: routed.message });
        } else {
          setSubmitError(res.message);
        }
        toast.error("Couldn't create store", res.message);
        return;
      }
      toast.success("Store created", `Switching to ${res.store.name}`);
      const switchRes = await switchToStore(res.store.id);
      if (!switchRes.ok) {
        setSubmitError(switchRes.message);
        toast.error("Couldn't switch to new store", switchRes.message);
        return;
      }
      window.location.href = "/settings/stores";
    });
  }

  return (
    <section className="admin-panel space-y-4 rounded-[1.6rem] p-6">
      <div className="flex items-center justify-between">
        <h2 className="text-sm font-semibold uppercase tracking-[0.16em] text-muted-foreground">
          Add a store
        </h2>
        <button
          type="button"
          onClick={onCancel}
          disabled={pending}
          className="text-xs text-muted-foreground hover:text-foreground disabled:cursor-not-allowed disabled:opacity-50"
        >
          Cancel
        </button>
      </div>

      <form onSubmit={handleSubmit(onValid)} noValidate className="space-y-5">
        <div className="grid gap-5 sm:grid-cols-2">
          {/* Store name */}
          <FormField
            id="new-store-name"
            label="Store name"
            error={errors.name?.message}
          >
            <Input
              id="new-store-name"
              placeholder="Acme Downtown"
              autoComplete="organization"
              aria-invalid={errors.name ? true : undefined}
              disabled={pending}
              {...register("name")}
            />
          </FormField>

          {/* Slug */}
          <FormField
            id="new-store-slug"
            label="Store URL"
            error={
              errors.slug?.message ?? slugError(slugAvailability) ?? undefined
            }
            hint={slugHint(slugAvailability, errors.slug?.message)}
            hintState={slugHintState(slugAvailability)}
          >
            <div className="flex items-stretch overflow-hidden rounded-md border border-border bg-background-elevated focus-within:border-ring focus-within:ring-2 focus-within:ring-ring/20">
              <input
                id="new-store-slug"
                type="text"
                placeholder="acme-downtown"
                spellCheck={false}
                autoComplete="off"
                aria-invalid={
                  errors.slug || slugAvailability.state === "taken"
                    ? true
                    : undefined
                }
                disabled={pending}
                {...register("slug", {
                  onChange: (e) => {
                    setSlugTouched(true);
                    e.target.value = e.target.value.toLowerCase();
                  },
                })}
                className="flex-1 bg-transparent px-3 py-2.5 text-sm text-foreground focus:outline-none disabled:cursor-not-allowed disabled:opacity-60"
              />
              <span className="flex items-center border-l border-border bg-paper-100 px-3 text-xs font-medium text-foreground-secondary">
                .mark8ly.com
              </span>
            </div>
          </FormField>

          {/* Country */}
          <FormField
            id="new-store-country"
            label="Country"
            error={errors.countryCode?.message}
          >
            <Controller
              control={control}
              name="countryCode"
              render={({ field }) => (
                <Select
                  value={field.value}
                  onValueChange={field.onChange}
                  disabled={pending}
                >
                  <SelectTrigger
                    id="new-store-country"
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
          </FormField>

          {/* Currency */}
          <FormField
            id="new-store-currency"
            label="Currency"
            error={errors.currencyCode?.message}
          >
            <Controller
              control={control}
              name="currencyCode"
              render={({ field }) => (
                <Select
                  value={field.value}
                  onValueChange={field.onChange}
                  disabled={pending}
                >
                  <SelectTrigger
                    id="new-store-currency"
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
          </FormField>
        </div>

        {submitError && (
          <p role="alert" aria-live="polite" className="text-sm text-danger">
            {submitError}
          </p>
        )}

        <div className="flex items-center justify-end gap-3">
          <button
            type="submit"
            disabled={!canSubmit}
            className="inline-flex items-center justify-center rounded-md bg-[color:var(--ink-900)] px-5 py-2 text-sm font-medium text-[color:var(--primary-foreground)] transition-colors hover:bg-[color:var(--moss-700)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:cursor-not-allowed disabled:opacity-50"
          >
            {pending ? "Creating…" : "Create store"}
          </button>
        </div>
      </form>
    </section>
  );
}

interface FormFieldProps {
  id: string;
  label: string;
  error?: string;
  hint?: string;
  hintState?: "default" | "success" | "error";
  children: React.ReactNode;
}

function FormField({
  id,
  label,
  error,
  hint,
  hintState,
  children,
}: FormFieldProps) {
  const hintColor =
    hintState === "success"
      ? "text-moss-700"
      : hintState === "error"
        ? "text-danger"
        : "text-foreground-tertiary";

  return (
    <div className="space-y-1.5">
      <Label htmlFor={id}>{label}</Label>
      {children}
      <div className="min-h-[1.125rem]">
        {error ? (
          <p id={`${id}-error`} role="alert" className="text-xs text-danger">
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
): "default" | "success" | "error" {
  if (status.state === "available") return "success";
  if (status.state === "taken" || status.state === "invalid") return "error";
  return "default";
}

function routeServerError(
  message: string,
): { field: keyof FormValues; message: string } | null {
  const m = message.toLowerCase();
  if (/slug|store url|already taken|unavailable/.test(m)) {
    return { field: "slug", message };
  }
  if (/store name|^name/.test(m)) {
    return { field: "name", message };
  }
  if (/country/.test(m)) {
    return { field: "countryCode", message };
  }
  if (/currency/.test(m)) {
    return { field: "currencyCode", message };
  }
  return null;
}

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

function resolveBrowserTimezone(): string {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC";
  } catch {
    return "UTC";
  }
}
