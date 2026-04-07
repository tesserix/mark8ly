import Link from "next/link";

import { locations } from "@/lib/api/platform-api";
import { OnboardingForm } from "@/components/onboarding/OnboardingForm";

/**
 * Server component: fetches reference data once on the server and hands
 * it down to the single-page form. Wrapped in a warm cream surface with
 * a slim brand bar (no full marketing header — keeps the surface focused).
 */
export default async function OnboardingPage() {
  const [countries, currencies, timezones] = await Promise.all([
    locations.listCountries(),
    locations.listCurrencies(),
    locations.listTimezones(),
  ]);

  return (
    <div className="min-h-screen bg-background flex flex-col">
      {/* Slim brand bar — links back to marketing site */}
      <div className="border-b border-warm-200 bg-background">
        <div className="max-w-6xl mx-auto px-6 h-16 flex items-center justify-between">
          <Link
            href="/"
            className="flex items-center hover:opacity-80 transition-opacity"
          >
            {/* eslint-disable-next-line @next/next/no-img-element */}
            <img
              src="/icon-192.png"
              alt="mark8ly icon"
              className="h-8 w-auto object-contain"
            />
            <span className="text-xl font-serif font-medium tracking-[-0.015em] text-foreground-secondary">
              mark8ly
            </span>
          </Link>
          <Link
            href="/"
            className="text-sm text-foreground-secondary hover:text-foreground transition-colors"
          >
            ← Back home
          </Link>
        </div>
      </div>

      {/* Form */}
      <main className="flex-1 flex items-center justify-center px-4 py-16">
        <OnboardingForm
          countries={countries}
          currencies={currencies}
          timezones={timezones}
        />
      </main>
    </div>
  );
}
