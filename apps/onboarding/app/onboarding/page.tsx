import Link from "next/link";

import { locations } from "@/lib/api/platform-api";
import { OnboardingForm } from "@/components/onboarding/OnboardingForm";
import { SlimFooter } from "@/components/onboarding/SlimFooter";

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
    <div className="min-h-screen flex flex-col">
      {/* Slim brand bar — links back to marketing site */}
      <div className="border-b border-warm-200/80 bg-background/85 backdrop-blur-sm">
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
            className="inline-flex items-center gap-2 rounded-full border border-warm-200 bg-white/80 px-4 py-2 text-sm font-medium text-foreground-secondary shadow-sm transition-[background-color,border-color,color,box-shadow] hover:border-warm-300 hover:bg-white hover:text-foreground hover:shadow-md"
          >
            <span aria-hidden>←</span>
            <span>Back Home</span>
          </Link>
        </div>
      </div>

      <main className="flex-1 px-4 py-10 sm:px-6 sm:py-14">
        <div className="max-w-6xl mx-auto grid gap-8 xl:grid-cols-[0.92fr_1.08fr] items-start">
          <section className="relative overflow-hidden rounded-[2rem] border border-warm-200/90 bg-[linear-gradient(180deg,rgba(255,255,255,0.78),rgba(250,247,242,0.9))] px-7 py-8 shadow-[0_20px_70px_rgba(43,38,34,0.08)] sm:px-10 sm:py-10">
            <div
              aria-hidden
              className="pointer-events-none absolute -right-20 top-0 h-56 w-56 rounded-full bg-terracotta-200/40 blur-3xl"
            />
            <div
              aria-hidden
              className="pointer-events-none absolute -left-16 bottom-0 h-44 w-44 rounded-full bg-sage-200/40 blur-3xl"
            />

            <div className="relative">
              <div className="inline-flex items-center gap-2 rounded-full border border-sage-200 bg-sage-50/90 px-4 py-2 text-xs font-medium uppercase tracking-[0.16em] text-sage-700">
                <span className="h-2 w-2 rounded-full bg-sage-500" aria-hidden />
                Concierge-style setup
              </div>

              <h1 className="mt-6 font-serif text-4xl font-medium tracking-[-0.03em] text-foreground sm:text-5xl">
                Launch with the calm confidence of a tailored storefront.
              </h1>
              <p className="mt-5 max-w-xl text-base leading-7 text-foreground-secondary sm:text-lg">
                This setup keeps the essentials moving and leaves the fiddly
                bits for later. Choose your store name, claim your URL, and we
                will guide you through the final verification step.
              </p>

              <div className="mt-8 grid gap-3 sm:grid-cols-3">
                {[
                  { label: "Time to launch", value: "About 2 minutes" },
                  { label: "Commitment", value: "No credit card" },
                  { label: "Can edit later", value: "Brand, domain & more" },
                ].map((item) => (
                  <div
                    key={item.label}
                    className="rounded-2xl border border-white/70 bg-white/80 p-4 shadow-[0_12px_30px_rgba(43,38,34,0.05)]"
                  >
                    <p className="text-xs uppercase tracking-[0.14em] text-foreground-tertiary">
                      {item.label}
                    </p>
                    <p className="mt-2 text-sm font-medium text-foreground">
                      {item.value}
                    </p>
                  </div>
                ))}
              </div>

              <div className="mt-8 rounded-[1.75rem] border border-warm-200/90 bg-foreground px-6 py-6 text-warm-100 shadow-[0_18px_60px_rgba(31,30,28,0.22)]">
                <p className="text-xs uppercase tracking-[0.16em] text-warm-400">
                  What happens next
                </p>
                <ol className="mt-4 space-y-4">
                  {[
                    "We send a verification link to confirm you own the email.",
                    "Your storefront is provisioned with your region and currency.",
                    "You land in a ready-to-customize workspace with your draft store live.",
                  ].map((step, index) => (
                    <li key={step} className="flex gap-3">
                      <span className="mt-0.5 flex h-7 w-7 flex-none items-center justify-center rounded-full bg-white/10 text-sm font-medium text-white">
                        {index + 1}
                      </span>
                      <span className="text-sm leading-6 text-warm-100/90">
                        {step}
                      </span>
                    </li>
                  ))}
                </ol>
              </div>
            </div>
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
