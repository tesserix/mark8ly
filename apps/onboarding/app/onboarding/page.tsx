import Link from "next/link";

import { locations } from "@/lib/api/platform-api";
import { OnboardingForm } from "@/components/onboarding/OnboardingForm";
import { SlimFooter } from "@/components/onboarding/SlimFooter";
import { signupCopy } from "@/lib/copy/signup";

// Rendered per request so middleware's CSP nonce reaches the script
// tags; a prerendered page under that policy would have them blocked.
export const dynamic = "force-dynamic";

// Funnel entry point — never index (we want search traffic to
// land on the marketing home, not mid-funnel).
export const metadata = {
  title: "Open your store",
  robots: { index: false, follow: false },
};

// We only let merchants onboard in countries a tested shipping carrier can
// fulfil. This allowlist mirrors the carriers' own SupportedCountries() in
// services/marketplace-api/internal/shipping:
//   - ShipEngine: AU, CA, DE, ES, FR, GB, IE, IT, NL, NZ, US
//   - Delhivery:  IN
// NinjaVan (ID, MY, PH, SG, TH, VN) is intentionally EXCLUDED until its
// integration is tested end-to-end — add those codes here once it is verified.
// Countries with no carrier at all (e.g. BR, JP, SA, AE) are excluded by
// virtue of not being listed.
const SUPPORTED_SHIPPING_COUNTRY_CODES = new Set([
  // ShipEngine
  "AU",
  "CA",
  "DE",
  "ES",
  "FR",
  "GB",
  "IE",
  "IT",
  "NL",
  "NZ",
  "US",
  // Delhivery
  "IN",
]);

const ROMAN = ["i.", "ii.", "iii.", "iv.", "v."] as const;

/**
 * /onboarding — the single-page signup form.
 *
 * Server component: fetches reference data once on the server
 * and hands it down to the client form. Layout is the same slim
 * brand bar + editorial hero + content pattern used by the rest
 * of the onboarding flow, so the whole funnel feels consistent.
 */
export default async function OnboardingPage() {
  const [allCountries, currencies, timezones] = await Promise.all([
    locations.listCountries(),
    locations.listCurrencies(),
    locations.listTimezones(),
  ]);

  // Only show markets a tested shipping carrier can fulfil.
  const countries = allCountries.filter((c) =>
    SUPPORTED_SHIPPING_COUNTRY_CODES.has(c.code.toUpperCase()),
  );

  return (
    <div className="flex min-h-screen flex-col bg-background text-foreground">
      {/* Slim brand bar — wordmark is the home link */}
      <header className="border-b border-border-subtle">
        <div className="mx-auto flex h-[64px] max-w-6xl items-center px-6">
          <Link
            href="/"
            aria-label="mark8ly — home"
            className="-mx-2 inline-flex items-center px-2 py-2"
          >
            <span className="font-serif text-[1.5rem] font-medium tracking-[-0.025em] text-foreground">
              mark8ly
            </span>
          </Link>
        </div>
      </header>

      <main
        id="main"
        className="flex-1 motion-safe:animate-[fadeInUp_0.35s_ease-out_both]"
      >
        {/*
          Three grid items, placed so a phone reads heading \u2192 one sentence
          \u2192 form \u2192 what happens next, and a desktop keeps the two-column
          editorial layout with the form on the right spanning both rows
          (tesserix/mark8ly#993). Before this the whole explanation
          stacked above the form on a phone and the first field sat ~750px
          down. `lg:grid-rows-[auto_1fr]` keeps the steps directly under
          the intro instead of letting a tall form push them down.
        */}
        <div className="mx-auto grid max-w-6xl gap-x-16 gap-y-8 px-6 pb-20 pt-8 sm:pt-12 lg:grid-cols-[1fr_1.2fr] lg:grid-rows-[auto_1fr] lg:gap-y-12 lg:pt-20">
          <section className="min-w-0 lg:col-start-1 lg:row-start-1">
            <p className="eyebrow mb-4 lg:mb-5">{signupCopy.eyebrow}</p>
            <h1 className="font-serif text-3xl font-medium leading-[1.05] tracking-[-0.02em] text-foreground sm:text-4xl">
              {signupCopy.heading}
            </h1>
            <p className="mt-4 max-w-md text-base leading-[1.55] text-foreground-secondary sm:text-lg lg:mt-6">
              {signupCopy.intro}
            </p>
          </section>

          <section className="min-w-0 lg:col-start-2 lg:row-start-1 lg:row-span-2">
            <OnboardingForm
              countries={countries}
              currencies={currencies}
              timezones={timezones}
            />
          </section>

          <section
            aria-labelledby="onboarding-steps-heading"
            className="min-w-0 lg:col-start-1 lg:row-start-2"
          >
            <h2 id="onboarding-steps-heading" className="sr-only">
              What happens next
            </h2>
            <ol className="space-y-6 border-t border-border-subtle pt-8 lg:pt-10">
              {signupCopy.steps.map((step, index) => (
                <li
                  key={step.title}
                  className="grid grid-cols-[auto_1fr] gap-6"
                >
                  <span className="font-serif text-xl text-moss-700">
                    {ROMAN[index]}
                  </span>
                  <div>
                    <p className="font-serif text-lg text-foreground">
                      {step.title}
                    </p>
                    <p className="mt-1 text-sm text-foreground-secondary">
                      {step.detail}
                    </p>
                  </div>
                </li>
              ))}
            </ol>
          </section>
        </div>
      </main>

      <SlimFooter />
    </div>
  );
}
