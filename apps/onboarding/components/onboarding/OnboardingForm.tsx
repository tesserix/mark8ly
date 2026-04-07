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
 * OnboardingForm — single client component holding the entire wizard.
 *
 * The legacy app shipped a 5-step wizard. This is the new compressed
 * single-page replacement: five fields (email, business name, slug,
 * country, currency) on one page, then a magic-link verification email.
 * Visuals use the same warm editorial palette as the marketing surface.
 *
 * Behavior is unchanged from the previous incarnation; only chrome was
 * upgraded.
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
    <div className="w-full max-w-lg mx-auto">
      {/* Trust strip above the card */}
      <div className="text-center mb-6">
        <div className="inline-flex items-center gap-2 px-4 py-1.5 rounded-full bg-sage-50 text-sage-700 text-xs font-medium border border-sage-200">
          <span className="h-1.5 w-1.5 rounded-full bg-sage-500 animate-pulse" />
          12 months free · No credit card
        </div>
      </div>

      <div className="rounded-2xl border border-warm-200 bg-white shadow-lg overflow-hidden">
        {/* Card header strip */}
        <div className="px-8 pt-8 pb-6 border-b border-warm-100 bg-gradient-to-b from-warm-50 to-white">
          <h1 className="font-serif text-3xl font-medium tracking-tight text-foreground">
            Let&apos;s get your store live
          </h1>
          <p className="mt-2 text-sm text-foreground-secondary">
            We&apos;ll email you a verification link to finish setting up.
            Takes about 30 seconds.
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
              autoFocus
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
            <div className="flex items-stretch rounded-lg border border-warm-200 overflow-hidden bg-white focus-within:ring-2 focus-within:ring-foreground/15 focus-within:border-foreground/30 transition">
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
                className="flex-1 px-3 py-2.5 bg-transparent text-sm focus:outline-none text-foreground"
              />
              <span className="flex items-center px-3 text-sm font-medium border-l border-warm-200 bg-warm-50 text-foreground-secondary">
                .mark8ly.com
              </span>
            </div>
            <SlugStatusLine status={slugStatus} />
            <p className="text-xs text-foreground-tertiary">
              You can connect your own domain after launch — it&apos;s free.
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
              <p className="text-sm text-terracotta-700" role="alert">
                {error}
              </p>
            </div>
          )}

          <button
            type="submit"
            disabled={!canSubmit}
            className="group w-full bg-primary text-primary-foreground px-6 py-3.5 rounded-xl text-base font-medium hover:bg-primary-hover hover:shadow-lg transition-all disabled:opacity-50 disabled:cursor-not-allowed disabled:hover:shadow-none inline-flex items-center justify-center gap-2"
          >
            {pending ? (
              "Sending verification email…"
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
      <p className="text-xs text-foreground-tertiary">
        3-63 characters · lowercase letters, numbers, and hyphens
      </p>
    );
  }
  if (status.state === "checking")
    return <p className="text-xs text-foreground-tertiary">Checking…</p>;
  if (status.state === "available")
    return <p className="text-xs text-sage-700">✓ Available</p>;
  if (status.state === "taken")
    return <p className="text-xs text-terracotta-700">Already taken</p>;
  return <p className="text-xs text-terracotta-700">{status.message}</p>;
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
