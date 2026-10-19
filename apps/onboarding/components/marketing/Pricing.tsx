'use client'

/**
 * Marketing /#pricing section.
 *
 * Client island for the monthly/annual toggle state and for resolving
 * the visitor's currency from the `mk8_currency` cookie. All pricing
 * data is static at build-time via SHARED_PRICING_CATALOGUE.
 *
 * Design rules per mark8ly/.impeccable.md:
 *  - Paper / Ink / Moss only; no other decorative colours.
 *  - Hairline rules between sections; NO bordered card boxes.
 *  - Left-aligned, asymmetric grid at lg.
 *  - Source Serif 4 for plan names + display prices.
 *  - One moss-700 accent per view (toggle pill active state).
 *  - No emoji. No urgency copy.
 *
 * Pricing source of truth: packages/ui/src/subscription/pricing-data.ts.
 * Plan features and taglines: lib/plans.ts (shared with /integrations).
 */

import Link from 'next/link'
import { useState } from 'react'
import {
  Money,
  getAddOnPrice,
  getPlanPrice,
  type SharedPricingCatalogue,
} from '@repo/ui/subscription'

import { useGeoCurrency } from '@/lib/geo/use-geo-currency'
import { PLAN_META } from '@/lib/plans'

interface PricingProps {
  catalogue: SharedPricingCatalogue
}

type BillingPeriod = 'monthly' | 'annual'


// No tax disclosure is rendered for any currency, AUD included: Tesserix is
// not GST-registered, so no GST is charged and none may be disclosed. The AU
// catalogue prices stay GST-exclusive; the copy returns if we register (#699).

function TogglePill({
  period,
  onChange,
}: {
  period: BillingPeriod
  onChange: (next: BillingPeriod) => void
}) {
  const base =
    'inline-flex h-9 items-center rounded-full px-4 text-sm font-medium transition-colors'
  const active =
    'bg-foreground text-background-elevated shadow-sm'
  const inactive =
    'text-foreground-secondary hover:text-foreground'

  return (
    <div
      role="radiogroup"
      aria-label="Billing period"
      className="inline-flex rounded-full border border-border p-1"
    >
      <button
        type="button"
        role="radio"
        aria-checked={period === 'monthly'}
        className={`${base} ${period === 'monthly' ? active : inactive}`}
        onClick={() => onChange('monthly')}
      >
        Monthly
      </button>
      <button
        type="button"
        role="radio"
        aria-checked={period === 'annual'}
        className={`${base} ${period === 'annual' ? active : inactive}`}
        onClick={() => onChange('annual')}
      >
        Annual
        <span
          className={`ml-2 rounded-full px-2 py-0.5 text-xs font-normal ${
            period === 'annual'
              ? 'bg-background-elevated/20 text-background-elevated'
              : 'bg-moss-700/10 text-moss-700'
          }`}
        >
          Save ~20%
        </span>
      </button>
    </div>
  )
}

export function Pricing({ catalogue }: PricingProps) {
  // Resolved on the client, not handed down from the server: reading the
  // cookie in the page's server component made the whole marketing route
  // dynamic and uncacheable at the edge. See lib/geo/currency.ts for the
  // measurements and the accepted first-paint flash.
  const currency = useGeoCurrency()
  const [period, setPeriod] = useState<BillingPeriod>('annual')
  const plans = catalogue.plans
  // The add-on always bills in USD globally (spec §4.1.2) regardless of the
  // visitor's currency, so the amount is always read from the USD row —
  // the Money below renders `currency="USD"` literally, never the resolved
  // currency.
  const { price: proApp } = getAddOnPrice(catalogue.proApp, 'USD')
  const showUsdBilledNote = currency !== 'USD'
  // Page-level "prices shown in X" copy MUST use the currency actually
  // resolved for the plans below, not the raw cookie value — otherwise a
  // visitor whose currency has no row would read "Prices shown in THB." over
  // amounts that are actually in USD. Every priceable currency has a row on
  // every plan (see PRICEABLE_CURRENCIES), so any plan resolves the same way;
  // the first plan is used as the anchor.
  const resolvedPageCurrency = plans[0] ? getPlanPrice(plans[0], currency).currency : currency

  return (
    <section
      id="pricing"
      className="border-t border-border-subtle py-16 sm:py-24"
    >
      <div className="mx-auto max-w-6xl px-6">
        <div className="mb-10 grid gap-8 lg:grid-cols-[1fr_2fr] lg:gap-12">
          <div>
            <p className="eyebrow mb-5">Pricing</p>
            <h2 className="font-serif text-4xl font-medium leading-[1.05] tracking-[-0.02em] text-foreground">
              Three plans.{' '}
              <br />
              Honestly priced.
            </h2>
          </div>
          <p className="max-w-xl self-end text-lg leading-relaxed text-foreground-secondary">
            Ninety days free to start. After that, three clear plans — no
            hidden fees, no transaction cuts from Mark8ly, only what your
            payment processor charges at their standard rate. Change plans
            any time; upgrades prorate, downgrades wait for the period to
            close.
          </p>
        </div>

        <div className="mb-10 flex flex-wrap items-center justify-between gap-4">
          <TogglePill period={period} onChange={setPeriod} />
          <p className="text-sm text-foreground-tertiary">
            Prices shown in {resolvedPageCurrency}.
          </p>
        </div>

        <div className="grid gap-10 sm:grid-cols-2 lg:grid-cols-[1.05fr_1fr_1.05fr]">
          {PLAN_META.map((meta, i) => {
            const plan = plans.find((p) => p.id === meta.id)!
            // `resolvedCurrency` is what MUST be rendered — when `currency`
            // has no row on this plan, getPlanPrice falls back to USD, and
            // rendering the visitor's raw currency would label that USD
            // amount with a currency it isn't priced in.
            const { price, currency: resolvedCurrency } = getPlanPrice(plan, currency)
            const headline =
              period === 'monthly' ? price.monthly : price.annualMonthlyEquivalent
            const cadence = period === 'monthly' ? '/mo' : '/mo, billed yearly'
            return (
              <div
                key={meta.id}
                className={
                  i < PLAN_META.length - 1
                    ? 'lg:border-r lg:border-border lg:pr-10'
                    : ''
                }
              >
                <p className="font-serif text-2xl font-medium text-foreground">
                  {meta.name}
                </p>
                <p className="mt-2 text-sm text-foreground-tertiary">
                  {meta.tagline}
                </p>
                <p
                  className="mt-6 font-serif text-foreground"
                  style={{
                    fontSize: 'clamp(3rem, 1.5rem + 5vw, 5rem)',
                    lineHeight: 0.95,
                    letterSpacing: '-0.03em',
                  }}
                >
                  {/* Cents only when there are some: rounding $182/yr down to
                      "$15/mo" understated the annual total. */}
                  <Money amount={headline} currency={resolvedCurrency} showCents={headline % 100 !== 0} />
                  <span
                    className="ml-1 font-sans text-base font-normal text-foreground-tertiary"
                    style={{ letterSpacing: 'normal', whiteSpace: 'nowrap' }}
                  >
                    {cadence}
                  </span>
                </p>
                {period === 'annual' && (
                  <p className="mt-1 text-sm text-foreground-tertiary">
                    <Money amount={price.annual} currency={resolvedCurrency} showCents={false} />{' '}
                    charged once a year
                  </p>
                )}
                {meta.includes && (
                  <p className="mt-6 text-sm font-medium text-foreground">
                    Everything in {meta.includes}, plus
                  </p>
                )}
                <ul
                  className={`${meta.includes ? 'mt-3' : 'mt-6'} space-y-3 text-foreground-secondary`}
                >
                  {meta.features.map((feature) => (
                    <li key={feature} className="flex items-baseline gap-3">
                      <span
                        aria-hidden="true"
                        className="font-serif text-moss-700"
                      >
                        ·
                      </span>
                      <span>{feature}</span>
                    </li>
                  ))}
                </ul>
              </div>
            )
          })}
        </div>

        <p className="mt-10 text-sm text-foreground-tertiary">
          Pro offers an optional White-label mobile app add-on at{' '}
          <Money amount={proApp.monthly} currency="USD" showCents={false} />
          /mo plus a <Money amount={200000} currency="USD" showCents={false} />{' '}
          one-time setup{showUsdBilledNote ? ' (billed in USD globally)' : ''}.
        </p>

        {/* Wording mirrors /security §9; keep the two in step. */}
        <p className="mt-3 text-sm text-foreground-tertiary">
          Card payments are handled by Stripe, Razorpay and Cashfree Payments;
          Mark8ly never stores full card numbers.{' '}
          <Link href="/security" className="underline underline-offset-4">
            How we keep payments secure
          </Link>
        </p>

        <div className="mt-10 flex flex-wrap items-center gap-x-8 gap-y-4 border-t border-border-subtle pt-10">
          <Link
            href="/onboarding"
            className="inline-flex h-12 items-center rounded-md bg-primary px-6 text-base font-medium text-primary-foreground hover:bg-primary-hover"
          >
            Open your store — free for 90 days
          </Link>
        </div>
      </div>
    </section>
  )
}
