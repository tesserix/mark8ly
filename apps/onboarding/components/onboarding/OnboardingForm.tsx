"use client";

import { useEffect, useMemo, useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import {
  Button,
  Card,
  CardContent,
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
 * Five fields: email, business name, slug (auto-suggested + live check),
 * country, currency. Timezone auto-detected from browser. One submit
 * sends the magic link email and routes to /onboarding/check-inbox.
 *
 * Visuals are now driven by @tesserix/web (Input, Select, Button, Card,
 * Label) instead of hand-rolled markup. Behavior is unchanged.
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
    <Card className="w-full max-w-md mx-auto">
      <CardContent className="p-8">
        <h1 className="text-2xl font-semibold tracking-tight">
          Get your store live
        </h1>
        <p className="mt-2 text-sm text-zinc-500 dark:text-zinc-400">
          We&apos;ll email you a verification link to finish setting up.
        </p>

        <form onSubmit={handleSubmit} className="mt-6 space-y-4">
          <div className="space-y-1.5">
            <Label htmlFor="email">Email address</Label>
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
            <Label htmlFor="businessName">Business name</Label>
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
            <Label htmlFor="slug">Store URL</Label>
            <div className="flex items-stretch rounded-md border border-input overflow-hidden focus-within:ring-2 focus-within:ring-ring">
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
                className="flex-1 px-3 py-2 bg-transparent text-sm focus:outline-none"
              />
              <span className="flex items-center px-3 text-sm text-zinc-500 border-l border-input bg-zinc-50 dark:bg-zinc-900">
                .mark8ly.com
              </span>
            </div>
            <SlugStatusLine status={slugStatus} />
          </div>

          <div className="grid grid-cols-2 gap-3">
            <div className="space-y-1.5">
              <Label htmlFor="country">Country</Label>
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
              <Label htmlFor="currency">Currency</Label>
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
            <p className="text-sm text-red-600" role="alert">
              {error}
            </p>
          )}

          <Button
            type="submit"
            disabled={!canSubmit}
            isLoading={pending}
            loadingText="Sending verification email…"
            className="w-full"
            size="lg"
          >
            Get my store ready
          </Button>
        </form>
      </CardContent>
    </Card>
  );
}

function SlugStatusLine({ status }: { status: SlugStatus }) {
  if (status.state === "idle") {
    return (
      <p className="text-xs text-zinc-500">
        3-63 characters, lowercase letters, numbers, and hyphens
      </p>
    );
  }
  if (status.state === "checking")
    return <p className="text-xs text-zinc-500">Checking…</p>;
  if (status.state === "available")
    return <p className="text-xs text-emerald-600">✓ Available</p>;
  if (status.state === "taken")
    return <p className="text-xs text-red-600">Already taken</p>;
  return <p className="text-xs text-red-600">{status.message}</p>;
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
