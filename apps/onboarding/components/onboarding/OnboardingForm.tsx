"use client";

import { useEffect, useMemo, useState, useTransition } from "react";
import { useRouter } from "next/navigation";

import type { Country, Currency, Timezone } from "@/lib/types";
import { useOnboardingStore } from "@/lib/store/onboarding-store";
import { signUp } from "@/lib/gip/signup";
import { checkSlug, submitOnboarding } from "@/app/onboarding/actions";

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
 * OnboardingForm is the single client component holding the entire wizard.
 *
 * Five fields: email, business name, slug (auto-suggested + live check),
 * country, currency. Timezone auto-detected from browser. One submit
 * sends the magic link email and routes to /onboarding/check-inbox.
 */
export function OnboardingForm({ countries, currencies, timezones }: Props) {
  const router = useRouter();
  const setSubmitted = useOnboardingStore((s) => s.setSubmitted);

  const [email, setEmail] = useState("");
  const [businessName, setBusinessName] = useState("");
  const [slug, setSlug] = useState("");
  const [slugTouched, setSlugTouched] = useState(false);
  const [countryCode, setCountryCode] = useState("");
  const [currencyCode, setCurrencyCode] = useState("");
  const [slugStatus, setSlugStatus] = useState<SlugStatus>({ state: "idle" });
  const [error, setError] = useState<string | null>(null);
  const [pending, startTransition] = useTransition();

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

  // Slug auto-suggestion from business name.
  useEffect(() => {
    if (slugTouched) return;
    setSlug(slugify(businessName));
  }, [businessName, slugTouched]);

  // Currency auto-derivation from country.
  useEffect(() => {
    if (!countryCode) return;
    const country = countries.find((c) => c.code === countryCode);
    if (country?.currency_code) setCurrencyCode(country.currency_code);
  }, [countryCode, countries]);

  // Live slug availability check (debounced).
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
    setError(null);

    const trimmedEmail = email.trim().toLowerCase();
    if (!trimmedEmail.includes("@")) {
      setError("Please enter a valid email address");
      return;
    }
    if (!businessName.trim() || !slug || !countryCode || !currencyCode) return;
    if (slugStatus.state !== "available") return;

    startTransition(async () => {
      // Step 1: client-side GIP signup. Captures uid + refresh_token + id_token.
      let gipUid = "";
      let gipRefreshToken = "";
      try {
        const gip = await signUp(trimmedEmail);
        gipUid = gip.uid;
        gipRefreshToken = gip.refreshToken;
      } catch (err) {
        setError(
          err instanceof Error
            ? `Account creation failed: ${err.message}`
            : "Account creation failed",
        );
        return;
      }

      // Step 2: create session + send magic link via server action.
      const r = await submitOnboarding({
        email: trimmedEmail,
        businessName: businessName.trim(),
        slug,
        countryCode,
        currencyCode,
        timezone: resolvedTimezone,
      });
      if (!r.ok) {
        setError(r.message);
        return;
      }

      // Step 3: persist everything for the verify page + redirect to inbox.
      setSubmitted({
        email: trimmedEmail,
        sessionId: r.data.sessionId,
        gipUid,
        gipRefreshToken,
        businessName: businessName.trim(),
        slug,
        countryCode,
        currencyCode,
        timezone: resolvedTimezone,
      });
      router.push("/onboarding/check-inbox");
    });
  }

  const canSubmit =
    email.trim() &&
    businessName.trim() &&
    slugStatus.state === "available" &&
    countryCode &&
    currencyCode &&
    !pending;

  return (
    <form onSubmit={handleSubmit} className="w-full max-w-md mx-auto bg-white dark:bg-zinc-900 rounded-2xl shadow-sm border border-zinc-200 dark:border-zinc-800 p-8">
      <h1 className="text-2xl font-semibold tracking-tight">Get your store live</h1>
      <p className="mt-2 text-sm text-zinc-500 dark:text-zinc-400">
        We'll email you a verification link to finish setting up.
      </p>

      <div className="mt-6 space-y-4">
        <Field label="Email address" htmlFor="email">
          <input
            id="email"
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            placeholder="founder@yourbusiness.com"
            required
            autoComplete="email"
            autoFocus
            className="form-input"
          />
        </Field>

        <Field label="Business name" htmlFor="businessName">
          <input
            id="businessName"
            type="text"
            value={businessName}
            onChange={(e) => setBusinessName(e.target.value)}
            placeholder="Acme Co"
            required
            className="form-input"
          />
        </Field>

        <Field label="Store URL" htmlFor="slug">
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
        </Field>

        <div className="grid grid-cols-2 gap-3">
          <Field label="Country" htmlFor="country">
            <select
              id="country"
              value={countryCode}
              onChange={(e) => setCountryCode(e.target.value)}
              required
              className="form-input"
            >
              <option value="">Select…</option>
              {countries.map((c) => (
                <option key={c.code} value={c.code}>
                  {c.flag_emoji} {c.name}
                </option>
              ))}
            </select>
          </Field>

          <Field label="Currency" htmlFor="currency">
            <select
              id="currency"
              value={currencyCode}
              onChange={(e) => setCurrencyCode(e.target.value)}
              required
              className="form-input"
            >
              <option value="">Select…</option>
              {currencies.map((c) => (
                <option key={c.code} value={c.code}>
                  {c.code} — {c.symbol}
                </option>
              ))}
            </select>
          </Field>
        </div>
      </div>

      {error && (
        <p className="mt-4 text-sm text-red-600" role="alert">
          {error}
        </p>
      )}

      <button
        type="submit"
        disabled={!canSubmit}
        className="mt-6 w-full bg-zinc-900 hover:bg-zinc-800 disabled:bg-zinc-400 dark:bg-white dark:hover:bg-zinc-100 text-white dark:text-zinc-900 font-medium py-2.5 rounded-lg transition-colors"
      >
        {pending ? "Sending verification email…" : "Get my store ready"}
      </button>

      <style jsx>{`
        .form-input {
          width: 100%;
          padding: 0.5rem 0.75rem;
          border-radius: 0.5rem;
          border: 1px solid rgb(212 212 216);
          background: white;
          color: rgb(24 24 27);
        }
        .form-input:focus {
          outline: none;
          box-shadow: 0 0 0 2px rgb(24 24 27);
        }
        @media (prefers-color-scheme: dark) {
          .form-input {
            background: rgb(24 24 27);
            border-color: rgb(63 63 70);
            color: rgb(244 244 245);
          }
          .form-input:focus {
            box-shadow: 0 0 0 2px white;
          }
        }
      `}</style>
    </form>
  );
}

interface FieldProps {
  label: string;
  htmlFor: string;
  children: React.ReactNode;
}
function Field({ label, htmlFor, children }: FieldProps) {
  return (
    <div>
      <label htmlFor={htmlFor} className="block text-sm font-medium text-zinc-900 dark:text-zinc-100 mb-1">
        {label}
      </label>
      {children}
    </div>
  );
}

function SlugStatusLine({ status }: { status: SlugStatus }) {
  if (status.state === "idle") {
    return <p className="mt-1 text-xs text-zinc-500">3-63 characters, lowercase letters, numbers, and hyphens</p>;
  }
  if (status.state === "checking") return <p className="mt-1 text-xs text-zinc-500">Checking…</p>;
  if (status.state === "available") return <p className="mt-1 text-xs text-emerald-600">✓ Available</p>;
  if (status.state === "taken") return <p className="mt-1 text-xs text-red-600">Already taken</p>;
  return <p className="mt-1 text-xs text-red-600">{status.message}</p>;
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

function isValidSlug(slug: string): boolean {
  return /^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$/.test(slug);
}
