"use client";

import { useEffect, useMemo, useState, useTransition } from "react";
import { useRouter } from "next/navigation";
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

type SlugStatus =
  | { state: "idle" }
  | { state: "checking" }
  | { state: "available" }
  | { state: "taken" }
  | { state: "invalid"; message: string };

/**
 * OnboardingForm — single client component holding the entire signup
 * wizard.
 *
 * Phase M restructure: this form no longer collects a credential. The
 * merchant enters business details + email here, gets a magic link, and
 * the credential (password OR Google) is collected on the
 * /onboarding/set-password page that runs after the link is clicked.
 *
 * Visuals use the same warm editorial palette as the marketing surface.
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

      setSubmitted({
        email: trimmedEmail,
        sessionId: r.data.sessionId,
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
    <div className="w-full max-w-lg mx-auto">
      <div className="rounded-[2rem] border border-warm-200/90 bg-white/90 shadow-[0_24px_80px_rgba(43,38,34,0.12)] backdrop-blur-sm overflow-hidden">
        {/* Card header strip */}
        <div className="px-8 pt-8 pb-6 border-b border-warm-100 bg-[linear-gradient(180deg,rgba(243,238,230,0.72),rgba(255,255,255,0.98))]">
          <div className="mb-4 flex items-center justify-between gap-3">
            <span className="text-xs font-medium uppercase tracking-[0.16em] text-foreground-tertiary">
              Create Your Store
            </span>
            <span className="rounded-full bg-warm-100 px-3 py-1 text-xs font-medium text-foreground-secondary">
              Step 1 of 2
            </span>
          </div>
          <div className="mb-4 inline-flex items-center gap-2 rounded-full border border-sage-200 bg-white/88 px-4 py-1.5 text-xs font-medium text-sage-700 shadow-sm">
            <span className="h-1.5 w-1.5 rounded-full bg-sage-500" aria-hidden />
            6 months free · No credit card
          </div>
          <h1 className="font-serif text-3xl font-medium tracking-tight text-foreground">
            Let&apos;s get your store live
          </h1>
          <p className="mt-2 text-sm leading-6 text-foreground-secondary">
            We&apos;ll email you a verification link to finish setting up.
            You can pick a password or sign in with Google after that.
          </p>
        </div>

        <form onSubmit={handleSubmit} className="px-8 py-8 space-y-5">
          <div className="space-y-1.5">
            <Label htmlFor="email" className="text-foreground">
              Email address
            </Label>
            <Input
              id="email"
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              placeholder="founder@yourbusiness.com"
              required
              autoComplete="email"
              spellCheck={false}
            />
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="businessName" className="text-foreground">
              Business name
            </Label>
            <Input
              id="businessName"
              type="text"
              value={businessName}
              onChange={(e) => setBusinessName(e.target.value)}
              placeholder="Acme Co"
              required
            />
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="slug" className="text-foreground">
              Store URL
            </Label>
            <div className="flex items-stretch rounded-lg border border-warm-200 overflow-hidden bg-white focus-within:ring-2 focus-within:ring-foreground/15 focus-within:border-foreground/30 transition-[border-color,box-shadow]">
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
                spellCheck={false}
                autoComplete="off"
                className="flex-1 px-3 py-2.5 bg-transparent text-sm focus:outline-none text-foreground"
              />
              <span className="flex items-center px-3 text-sm font-medium border-l border-warm-200 bg-warm-50 text-foreground-secondary">
                .mark8ly.com
              </span>
            </div>
            <SlugStatusLine status={slugStatus} />
            <p className="text-xs text-foreground-tertiary">
              Start with a Mark8ly URL now. You can connect a custom domain
              after launch.
            </p>
          </div>

          <div className="grid grid-cols-2 gap-3">
            <div className="space-y-1.5">
              <Label htmlFor="country" className="text-foreground">
                Country
              </Label>
              <Select value={countryCode} onValueChange={setCountryCode}>
                <SelectTrigger id="country">
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
            </div>

            <div className="space-y-1.5">
              <Label htmlFor="currency" className="text-foreground">
                Currency
              </Label>
              <Select value={currencyCode} onValueChange={setCurrencyCode}>
                <SelectTrigger id="currency">
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
            </div>
          </div>

          {error && (
            <div className="p-3 rounded-lg bg-terracotta-50 border border-terracotta-200">
              <p
                className="text-sm text-terracotta-700"
                role="alert"
                aria-live="polite"
              >
                {error}
              </p>
            </div>
          )}

          <button
            type="submit"
            disabled={!canSubmit}
            className="group inline-flex w-full items-center justify-center gap-2 rounded-xl bg-primary px-6 py-3.5 text-base font-medium text-primary-foreground shadow-[0_14px_30px_rgba(31,30,28,0.18)] transition-[background-color,box-shadow,transform] hover:bg-primary-hover hover:shadow-[0_18px_36px_rgba(31,30,28,0.22)] disabled:cursor-not-allowed disabled:opacity-50 disabled:hover:shadow-[0_14px_30px_rgba(31,30,28,0.18)]"
          >
            {pending ? (
              <>
                <span
                  className="h-4 w-4 rounded-full border-2 border-white/30 border-t-white motion-safe:animate-spin"
                  aria-hidden
                />
                <span aria-live="polite">Sending your verification link…</span>
              </>
            ) : (
              <>
                Get my store ready
                <svg
                  width="18"
                  height="18"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="2"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  className="transition-transform group-hover:translate-x-0.5"
                >
                  <path d="M5 12h14M13 5l7 7-7 7" />
                </svg>
              </>
            )}
          </button>

          <p className="text-xs text-foreground-tertiary text-center">
            By creating an account you agree to our{" "}
            <a href="/terms" className="underline hover:text-foreground">
              Terms
            </a>
            ,{" "}
            <a href="/privacy" className="underline hover:text-foreground">
              Privacy Policy
            </a>
            , and{" "}
            <a href="/legal" className="underline hover:text-foreground">
              Security &amp; Compliance Policy
            </a>
            .
          </p>
        </form>
      </div>
    </div>
  );
}

function SlugStatusLine({ status }: { status: SlugStatus }) {
  if (status.state === "idle") {
    return (
      <p className="text-xs text-foreground-tertiary" aria-live="polite">
        3-63 characters · lowercase letters, numbers, and hyphens
      </p>
    );
  }
  if (status.state === "checking")
    return (
      <p className="text-xs text-foreground-tertiary" aria-live="polite">
        Checking availability…
      </p>
    );
  if (status.state === "available")
    return (
      <p className="text-xs text-sage-700" aria-live="polite">
        ✓ Available
      </p>
    );
  if (status.state === "taken")
    return (
      <p className="text-xs text-terracotta-700" aria-live="polite">
        Already taken
      </p>
    );
  return (
    <p className="text-xs text-terracotta-700" aria-live="polite">
      {status.message}
    </p>
  );
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
