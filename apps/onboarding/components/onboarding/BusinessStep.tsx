"use client";

import { useEffect, useMemo, useState, useTransition } from "react";

import type { Country, Currency, Timezone } from "@/lib/types";
import { useWizardStore } from "@/lib/store/wizard-store";
import { checkSlug } from "@/app/onboarding/actions";

interface Props {
  countries: Country[];
  currencies: Currency[];
  timezones: Timezone[];
}

type SlugStatus =
  | { state: "idle" }
  | { state: "checking" }
  | { state: "available" }
  | { state: "taken" }
  | { state: "invalid"; message: string };

/**
 * BusinessStep is the third and final form step. Collects:
 *   - Business name (free text)
 *   - Slug (auto-suggested from business name, live availability check)
 *   - Country (combobox with search)
 *   - Currency (auto-derived from country, editable)
 *
 * Timezone is auto-detected from the browser via Intl.DateTimeFormat
 * and not shown as a UI element. The user can change it later in admin.
 */
export function BusinessStep({ countries, currencies, timezones }: Props) {
  const setBusinessFields = useWizardStore((s) => s.setBusinessFields);
  const setStep = useWizardStore((s) => s.setStep);

  const [businessName, setBusinessName] = useState("");
  const [slug, setSlug] = useState("");
  const [slugTouched, setSlugTouched] = useState(false);
  const [countryCode, setCountryCode] = useState("");
  const [currencyCode, setCurrencyCode] = useState("");
  const [slugStatus, setSlugStatus] = useState<SlugStatus>({ state: "idle" });
  const [pending, startTransition] = useTransition();

  // Detect browser timezone once on mount.
  const browserTimezone = useMemo(() => {
    try {
      return Intl.DateTimeFormat().resolvedOptions().timeZone;
    } catch {
      return "UTC";
    }
  }, []);

  // Pick a timezone from the seeded list that matches the browser, or
  // fall back to the first available one.
  const resolvedTimezone = useMemo(() => {
    const exact = timezones.find((tz) => tz.id === browserTimezone);
    return exact?.id ?? timezones[0]?.id ?? "UTC";
  }, [browserTimezone, timezones]);

  // ─── Business name → slug auto-suggestion ────────────────────────────
  useEffect(() => {
    if (slugTouched) return; // user has edited slug manually; stop suggesting
    const suggested = slugify(businessName);
    setSlug(suggested);
  }, [businessName, slugTouched]);

  // ─── Country → currency derivation ───────────────────────────────────
  useEffect(() => {
    if (!countryCode) return;
    const country = countries.find((c) => c.code === countryCode);
    if (country?.currency_code) {
      setCurrencyCode(country.currency_code);
    }
  }, [countryCode, countries]);

  // ─── Live slug availability check (debounced) ────────────────────────
  useEffect(() => {
    if (!slug || slug.length < 3) {
      setSlugStatus({ state: "idle" });
      return;
    }
    if (!isValidSlug(slug)) {
      setSlugStatus({
        state: "invalid",
        message: "lowercase letters, numbers, and hyphens only",
      });
      return;
    }

    setSlugStatus({ state: "checking" });
    const handle = setTimeout(async () => {
      const r = await checkSlug(slug);
      if (!r.ok) {
        setSlugStatus({ state: "invalid", message: r.message });
        return;
      }
      setSlugStatus({ state: r.data.available ? "available" : "taken" });
    }, 300);

    return () => clearTimeout(handle);
  }, [slug]);

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();

    if (!businessName.trim() || !slug || !countryCode || !currencyCode) return;
    if (slugStatus.state !== "available") return;

    startTransition(() => {
      setBusinessFields({
        businessName: businessName.trim(),
        slug,
        countryCode,
        currencyCode,
        timezone: resolvedTimezone,
      });
      setStep("submitting");
    });
  }

  const canSubmit =
    businessName.trim().length > 0 &&
    slugStatus.state === "available" &&
    countryCode &&
    currencyCode &&
    !pending;

  return (
    <form onSubmit={handleSubmit}>
      <h1 className="text-2xl font-semibold tracking-tight mb-2">
        Tell us about your store
      </h1>
      <p className="text-sm text-zinc-500 dark:text-zinc-400 mb-6">
        You can change all of this later in your admin.
      </p>

      <div className="space-y-4">
        {/* Business name */}
        <div>
          <label htmlFor="businessName" className="block text-sm font-medium mb-1">
            Business name
          </label>
          <input
            id="businessName"
            type="text"
            value={businessName}
            onChange={(e) => setBusinessName(e.target.value)}
            placeholder="Acme Co"
            required
            autoFocus
            className="w-full px-3 py-2 rounded-lg border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-900 focus:outline-none focus:ring-2 focus:ring-zinc-900 dark:focus:ring-white"
          />
        </div>

        {/* Slug with live availability */}
        <div>
          <label htmlFor="slug" className="block text-sm font-medium mb-1">
            Store URL
          </label>
          <div className="flex items-center rounded-lg border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-900 focus-within:ring-2 focus-within:ring-zinc-900 dark:focus-within:ring-white">
            <input
              id="slug"
              type="text"
              value={slug}
              onChange={(e) => {
                setSlugTouched(true);
                setSlug(e.target.value.toLowerCase());
              }}
              placeholder="acme"
              required
              className="flex-1 px-3 py-2 bg-transparent focus:outline-none"
            />
            <span className="px-3 py-2 text-zinc-500 text-sm border-l border-zinc-300 dark:border-zinc-700">
              .mark8ly.com
            </span>
          </div>
          <SlugStatusLine status={slugStatus} />
        </div>

        {/* Country */}
        <div>
          <label htmlFor="country" className="block text-sm font-medium mb-1">
            Country
          </label>
          <select
            id="country"
            value={countryCode}
            onChange={(e) => setCountryCode(e.target.value)}
            required
            className="w-full px-3 py-2 rounded-lg border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-900 focus:outline-none focus:ring-2 focus:ring-zinc-900 dark:focus:ring-white"
          >
            <option value="">Select a country</option>
            {countries.map((c) => (
              <option key={c.code} value={c.code}>
                {c.flag_emoji} {c.name}
              </option>
            ))}
          </select>
        </div>

        {/* Currency (auto-derived but editable) */}
        <div>
          <label htmlFor="currency" className="block text-sm font-medium mb-1">
            Currency
          </label>
          <select
            id="currency"
            value={currencyCode}
            onChange={(e) => setCurrencyCode(e.target.value)}
            required
            className="w-full px-3 py-2 rounded-lg border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-900 focus:outline-none focus:ring-2 focus:ring-zinc-900 dark:focus:ring-white"
          >
            <option value="">Select a currency</option>
            {currencies.map((c) => (
              <option key={c.code} value={c.code}>
                {c.code} — {c.name} ({c.symbol})
              </option>
            ))}
          </select>
        </div>
      </div>

      <button
        type="submit"
        disabled={!canSubmit}
        className="mt-6 w-full bg-zinc-900 hover:bg-zinc-800 disabled:bg-zinc-400 dark:bg-white dark:hover:bg-zinc-100 text-white dark:text-zinc-900 font-medium py-2.5 rounded-lg transition-colors"
      >
        Create my store
      </button>
    </form>
  );
}

// ─── Helpers ──────────────────────────────────────────────────────────────

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

function isValidSlug(slug: string): boolean {
  return /^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$/.test(slug);
}

function SlugStatusLine({ status }: { status: SlugStatus }) {
  if (status.state === "idle") {
    return (
      <p className="mt-1 text-xs text-zinc-500">
        3-63 characters, lowercase letters, numbers, and hyphens
      </p>
    );
  }
  if (status.state === "checking") {
    return <p className="mt-1 text-xs text-zinc-500">Checking…</p>;
  }
  if (status.state === "available") {
    return <p className="mt-1 text-xs text-emerald-600">✓ Available</p>;
  }
  if (status.state === "taken") {
    return <p className="mt-1 text-xs text-red-600">Already taken</p>;
  }
  return <p className="mt-1 text-xs text-red-600">{status.message}</p>;
}
