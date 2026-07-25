import Link from "next/link";

import { locations } from "@/lib/api/platform-api";
import { OnboardingForm } from "@/components/onboarding/OnboardingForm";
import { SlimFooter } from "@/components/onboarding/SlimFooter";

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
        <div className="mx-auto grid max-w-6xl gap-12 px-6 pb-20 pt-16 sm:pt-20 lg:grid-cols-[1fr_1.2fr] lg:gap-16">
          <section>
            <p className="eyebrow mb-5">Open your store</p>
            <h1 className="font-serif text-4xl font-medium leading-[1.05] tracking-[-0.02em] text-foreground">
              Two minutes to a storefront.
            </h1>
            <p className="mt-6 max-w-md text-lg leading-[1.55] text-foreground-secondary">
              Name, region, URL. We send a verification link to confirm your
              email, then open your admin.
            </p>

            <ol className="mt-12 space-y-6 border-t border-border-subtle pt-10">
              {[
                {
                  n: "i.",
                  t: "Tell us about your shop.",
                  d: "Business name, country, currency.",
                },
                {
                  n: "ii.",
                  t: "Verify your email.",
                  d: "We\u2019ll send a link so you don\u2019t need a password yet.",
                },
                {
                  n: "iii.",
                  t: "Open the doors.",
                  d: "You\u2019ll land in your admin, ready to go.",
                },
              ].map((step) => (
                <li key={step.n} className="grid grid-cols-[auto_1fr] gap-6">
                  <span className="font-serif text-xl text-moss-700">
                    {step.n}
                  </span>
                  <div>
                    <p className="font-serif text-lg text-foreground">
                      {step.t}
                    </p>
                    <p className="mt-1 text-sm text-foreground-secondary">
                      {step.d}
                    </p>
                  </div>
                </li>
              ))}
            </ol>
          </section>

          <section>
            <OnboardingForm
              countries={countries}
              currencies={currencies}
              timezones={timezones}
            />
          </section>
        </div>
      </main>

      <SlimFooter />
    </div>
  );
}
