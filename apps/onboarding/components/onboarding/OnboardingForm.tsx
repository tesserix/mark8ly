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
import { signUp, signInWithGoogle, GIPSignupError } from "@/lib/gip/signup";
import { getGoogleCredential } from "@/lib/gip/google-gsi";
import {
  checkSlug,
  submitOnboarding,
  submitOnboardingWithGoogle,
} from "@/app/onboarding/actions";

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
  const [password, setPassword] = useState("");
  // Google sign-up credentials, populated when the user clicks "Continue
  // with Google" before submitting. When set, the email field is locked
  // and the password field is hidden — submission goes through the
  // verify-google bypass server action instead of the magic-link path.
  const [googleCreds, setGoogleCreds] = useState<{
    uid: string;
    idToken: string;
    refreshToken: string;
    email: string;
  } | null>(null);
  const [googlePending, setGooglePending] = useState(false);
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

  async function handleGoogleClick() {
    setError(null);
    setGooglePending(true);
    try {
      const { credential } = await getGoogleCredential();
      const gip = await signInWithGoogle(credential);
      // signInWithGoogle's response only includes uid + tokens. We need
      // the email too — Identity Toolkit returns it on the first call,
      // but our SignupResult shape doesn't surface it. Decode the
      // id_token's payload (no signature check needed here, it's just
      // for autofilling the form; the server re-verifies on submit).
      const decoded = decodeJwtEmail(gip.idToken);
      if (!decoded) {
        setError("Couldn't read email from Google sign-in");
        return;
      }
      setEmail(decoded);
      setGoogleCreds({
        uid: gip.uid,
        idToken: gip.idToken,
        refreshToken: gip.refreshToken,
        email: decoded,
      });
    } catch (err) {
      if (err instanceof GIPSignupError) {
        setError(`Google sign-up failed: ${err.message}`);
      } else {
        setError(
          err instanceof Error ? `Google sign-up failed: ${err.message}` : "Google sign-up failed",
        );
      }
    } finally {
      setGooglePending(false);
    }
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);

    const trimmedEmail = email.trim().toLowerCase();
    if (!trimmedEmail.includes("@")) {
      setError("Please enter a valid email address");
      return;
    }
    if (!googleCreds && password.length < 8) {
      setError("Password must be at least 8 characters");
      return;
    }
    if (!businessName.trim() || !slug || !countryCode || !currencyCode) return;
    if (slugStatus.state !== "available") return;

    // Google path: skip the magic link entirely. The id_token has already
    // proven email ownership, so go straight to complete + autoLogin via
    // submitOnboardingWithGoogle.
    if (googleCreds) {
      startTransition(async () => {
        const r = await submitOnboardingWithGoogle({
          email: googleCreds.email,
          businessName: businessName.trim(),
          slug,
          countryCode,
          currencyCode,
          timezone: resolvedTimezone,
          gipUid: googleCreds.uid,
          gipIdToken: googleCreds.idToken,
          gipRefreshToken: googleCreds.refreshToken,
        });
        if (!r.ok) {
          setError(r.message);
          return;
        }
        setSubmitted({
          email: googleCreds.email,
          sessionId: "",
          gipUid: googleCreds.uid,
          gipRefreshToken: googleCreds.refreshToken,
          businessName: businessName.trim(),
          slug,
          countryCode,
          currencyCode,
          timezone: resolvedTimezone,
        });
        router.push("/welcome");
      });
      return;
    }

    startTransition(async () => {
      let gipUid = "";
      let gipRefreshToken = "";
      try {
        const gip = await signUp(trimmedEmail, password);
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
        gipUid,
        gipRefreshToken,
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
    (googleCreds !== null || password.length >= 8) &&
    businessName.trim() &&
    slugStatus.state === "available" &&
    countryCode &&
    currencyCode &&
    !pending &&
    !googlePending;

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
            You can still refine your branding, domain, and catalog after this.
          </p>
        </div>

        <form onSubmit={handleSubmit} className="px-8 py-8 space-y-5">
          {!googleCreds && (
            <>
              <button
                type="button"
                onClick={handleGoogleClick}
                disabled={googlePending || pending}
                className="inline-flex w-full items-center justify-center gap-3 rounded-xl border border-warm-200 bg-white px-6 py-3 text-sm font-medium text-foreground transition hover:bg-warm-50 disabled:cursor-not-allowed disabled:opacity-50"
              >
                <GoogleMark />
                {googlePending ? "Opening Google…" : "Continue with Google"}
              </button>
              <div className="relative py-1">
                <div className="absolute inset-0 flex items-center" aria-hidden>
                  <div className="w-full border-t border-warm-200" />
                </div>
                <div className="relative flex justify-center">
                  <span className="bg-white px-3 text-xs uppercase tracking-wider text-foreground-tertiary">
                    or
                  </span>
                </div>
              </div>
            </>
          )}

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
              disabled={googleCreds !== null}
            />
            {googleCreds && (
              <p className="text-xs text-sage-700">
                ✓ Verified with Google
              </p>
            )}
          </div>

          {!googleCreds && (
            <div className="space-y-1.5">
              <Label htmlFor="password" className="text-foreground">
                Password
              </Label>
              <Input
                id="password"
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="At least 8 characters"
                required
                minLength={8}
                autoComplete="new-password"
              />
              <p className="text-xs text-foreground-tertiary">
                You&apos;ll use this to sign back in next time.
              </p>
            </div>
          )}

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

// decodeJwtEmail pulls the email claim out of a JWT without verifying the
// signature. Used only to autofill the form's email field after the
// Google sign-up popup — the server re-verifies the same id_token via
// Identity Toolkit accounts:lookup before trusting it for verification.
function decodeJwtEmail(token: string): string | null {
  try {
    const [, payload] = token.split(".");
    if (!payload) return null;
    const padded = payload.padEnd(payload.length + ((4 - (payload.length % 4)) % 4), "=");
    const json = atob(padded.replace(/-/g, "+").replace(/_/g, "/"));
    const claims = JSON.parse(json) as { email?: string };
    return claims.email ?? null;
  } catch {
    return null;
  }
}

function GoogleMark() {
  return (
    <svg width="18" height="18" viewBox="0 0 18 18" aria-hidden>
      <path
        fill="#4285F4"
        d="M17.64 9.2c0-.637-.057-1.251-.164-1.84H9v3.481h4.844a4.14 4.14 0 0 1-1.796 2.716v2.258h2.908c1.702-1.567 2.684-3.874 2.684-6.615z"
      />
      <path
        fill="#34A853"
        d="M9 18c2.43 0 4.467-.806 5.956-2.184l-2.908-2.258c-.806.54-1.837.86-3.048.86-2.344 0-4.328-1.584-5.036-3.711H.957v2.332A8.997 8.997 0 0 0 9 18z"
      />
      <path
        fill="#FBBC05"
        d="M3.964 10.707A5.41 5.41 0 0 1 3.682 9c0-.593.102-1.17.282-1.707V4.961H.957A8.997 8.997 0 0 0 0 9c0 1.452.348 2.827.957 4.039l3.007-2.332z"
      />
      <path
        fill="#EA4335"
        d="M9 3.58c1.321 0 2.508.454 3.44 1.345l2.582-2.58C13.463.891 11.426 0 9 0A8.997 8.997 0 0 0 .957 4.961L3.964 7.293C4.672 5.166 6.656 3.58 9 3.58z"
      />
    </svg>
  );
}
