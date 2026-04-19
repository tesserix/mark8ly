'use client'

/**
 * PricingClient — interactive 4-plan grid + Pro+App add-on.
 *
 * Design rules (non-negotiable):
 *  - Paper / Ink / Moss only. No other decorative colours.
 *  - Hairline column separators; NO bordered card boxes.
 *  - Left-aligned, asymmetric grid at lg (1.1fr · 1fr · 1fr · 1.1fr).
 *    The slight width variance is intentional editorial asymmetry —
 *    Starter and Pro are the narrative anchors; Growth and Scale flank.
 *  - Source Serif 4 for plan names and display prices.
 *  - Source Sans 3 for body, taglines, feature lists, CTAs.
 *  - One moss-700 accent per view: Pro card border-left.
 *  - No emoji. No urgency copy.
 */

import { useState } from 'react'
import { Money, PlanBadge } from '@repo/ui/subscription'
import { pricingCopy } from '@/lib/copy/pricing'
import type { PricingCatalogue, Plan, PlanPrice } from './page'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type BillingPeriod = 'monthly' | 'annual'

interface PricingClientProps {
  currency: string
  pricing: PricingCatalogue
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function getPlanPrice(plan: Plan, currency: string): PlanPrice {
  return plan.prices[currency] ?? plan.prices['USD']!
}

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

interface FeatureDotProps {
  children: string
}

function FeatureDot({ children }: FeatureDotProps) {
  return (
    <li className="flex items-start gap-2 text-sm leading-relaxed" style={{ color: 'var(--ink-700)' }}>
      <span
        aria-hidden="true"
        className="mt-1 w-1.5 h-1.5 rounded-full shrink-0 block"
        style={{ background: 'var(--moss-700)' }}
      />
      {children}
    </li>
  )
}

interface PlanColumnProps {
  plan: Plan
  copy: typeof pricingCopy.plans[number]
  currency: string
  period: BillingPeriod
  isLast: boolean
}

function PlanColumn({ plan, copy, currency, period, isLast }: PlanColumnProps) {
  const price = getPlanPrice(plan, currency)
  const isPro = plan.id === 'pro'

  const displayAmount = period === 'monthly' ? price.monthly : price.annualMonthlyEquivalent
  const isConversation = copy.cta === 'Start a conversation'

  return (
    <article
      className={[
        'flex flex-col gap-6 py-8 px-6',
        // Hairline right-border between columns except the last
        !isLast ? 'border-r border-[var(--hairline,var(--ink-100))]' : '',
        // Pro-only: moss accent left border — the ONE accent rule
        isPro ? 'border-l-2 border-l-[var(--moss-700)]' : '',
      ]
        .filter(Boolean)
        .join(' ')}
      aria-label={`${copy.name} plan`}
    >
      {/* Badge */}
      <div>
        <PlanBadge plan={plan.id} />
      </div>

      {/* Plan name — Source Serif 4 */}
      <div>
        <h2
          className="text-2xl font-semibold leading-tight mb-2"
          style={{ fontFamily: 'var(--font-editorial-serif, serif)', color: 'var(--ink-900)' }}
        >
          {copy.name}
        </h2>
        <p className="text-sm leading-relaxed" style={{ color: 'var(--ink-700)' }}>
          {copy.tagline}
        </p>
      </div>

      {/* Price display — Source Serif 4 */}
      <div>
        {isPro ? (
          <ProPriceBlock
            price={price}
            currency={currency}
            period={period}
          />
        ) : (
          <StandardPriceBlock
            amount={displayAmount}
            currency={currency}
            period={period}
          />
        )}
      </div>

      {/* CTA */}
      <a
        href={copy.ctaHref}
        className={[
          'inline-block text-center text-sm font-medium py-2.5 px-5 rounded-md transition-colors',
          'focus:outline-none focus-visible:ring-2 focus-visible:ring-[var(--moss-700)] focus-visible:ring-offset-1',
          isConversation
            ? 'border border-[var(--ink-900)] hover:bg-[var(--ink-900)] hover:text-[var(--paper-200)]'
            : '',
          !isConversation
            ? 'bg-[var(--ink-900)] text-[var(--paper-200)] hover:bg-[var(--moss-700)]'
            : '',
        ]
          .filter(Boolean)
          .join(' ')}
        style={{ fontFamily: 'var(--font-editorial-sans, sans-serif)' }}
      >
        {copy.cta}
      </a>

      {/* Feature list */}
      <ul className="flex flex-col gap-3 mt-2" aria-label={`${copy.name} features`}>
        {copy.features.map((feature) => (
          <FeatureDot key={feature}>{feature}</FeatureDot>
        ))}
      </ul>
    </article>
  )
}

// ---------------------------------------------------------------------------
// Price block variants
// ---------------------------------------------------------------------------

interface StandardPriceBlockProps {
  amount: number
  currency: string
  period: BillingPeriod
}

function StandardPriceBlock({ amount, currency, period }: StandardPriceBlockProps) {
  return (
    <div>
      <div className="flex items-baseline gap-1">
        <Money
          amount={amount}
          currency={currency}
          showCents={false}
          className="text-4xl font-semibold"
          // Source Serif 4 applied via inline style — className alone won't
          // pick up the custom font variable in Tailwind v4 arbitrary mode.
        />
        <span className="text-sm" style={{ color: 'var(--ink-600)' }}>
          / mo
        </span>
      </div>
      {period === 'annual' && (
        <p className="text-xs mt-1" style={{ color: 'var(--ink-600)' }}>
          Billed annually
        </p>
      )}
    </div>
  )
}

interface ProPriceBlockProps {
  price: PlanPrice
  currency: string
  period: BillingPeriod
}

function ProPriceBlock({ price, currency, period }: ProPriceBlockProps) {
  const isAnnual = period === 'annual'

  // USD-specific copy from the spec verbatim; other currencies show equivalent.
  const isUSD = currency === 'USD'

  return (
    <div className="flex flex-col gap-1">
      {isAnnual ? (
        <>
          <p className="text-sm font-medium" style={{ color: 'var(--ink-900)' }}>
            {isUSD ? (
              <>
                From{' '}
                <Money amount={price.annual} currency={currency} showCents={false} />/yr (
                <Money amount={price.annualMonthlyEquivalent} currency={currency} showCents={false} />
                /mo equivalent), billed annually.
              </>
            ) : (
              <>
                From{' '}
                <Money amount={price.annual} currency={currency} showCents={false} />/yr
                {' '}(
                <Money amount={price.annualMonthlyEquivalent} currency={currency} showCents={false} />
                /mo), billed annually.
              </>
            )}
          </p>
          <p className="text-xs" style={{ color: 'var(--ink-600)' }}>
            {isUSD
              ? 'Monthly available at $119/mo.'
              : <>Monthly available at <Money amount={price.monthly} currency={currency} showCents={false} />/mo.</>}
          </p>
        </>
      ) : (
        <>
          <div className="flex items-baseline gap-1">
            <Money
              amount={price.monthly}
              currency={currency}
              showCents={false}
              className="text-4xl font-semibold"
            />
            <span className="text-sm" style={{ color: 'var(--ink-600)' }}>
              / mo
            </span>
          </div>
          <p className="text-xs" style={{ color: 'var(--ink-600)' }}>
            {pricingCopy.proMonthlyPremiumNote}
          </p>
        </>
      )}
    </div>
  )
}

// ---------------------------------------------------------------------------
// Pro+App add-on card
// ---------------------------------------------------------------------------

interface ProAppCardProps {
  currency: string
  period: BillingPeriod
  pricing: PricingCatalogue
}

function ProAppCard({ currency, period, pricing }: ProAppCardProps) {
  const addonCopy = pricingCopy.proApp
  const price = pricing.proApp.prices[currency] ?? pricing.proApp.prices['USD']!
  const amount = period === 'monthly' ? price.monthly : price.annualMonthlyEquivalent

  return (
    <section
      aria-label="White-label App add-on"
      className="pt-8 border-t border-[var(--hairline,var(--ink-100))]"
    >
      <div className="flex flex-col gap-4 md:flex-row md:items-start md:justify-between">
        {/* Left: name + tagline + requirement */}
        <div className="flex flex-col gap-1 max-w-lg">
          <div className="flex items-center gap-3">
            <h2
              className="text-xl font-semibold"
              style={{ fontFamily: 'var(--font-editorial-serif, serif)', color: 'var(--ink-900)' }}
            >
              {addonCopy.name}
            </h2>
            <PlanBadge plan="studio" addon="white_label_app" />
          </div>
          <p className="text-sm" style={{ color: 'var(--ink-700)' }}>
            {addonCopy.tagline}
          </p>
          <p className="text-xs mt-1" style={{ color: 'var(--ink-500)' }}>
            {addonCopy.requirement}
          </p>
        </div>

        {/* Right: price + CTA */}
        <div className="flex flex-col gap-3 shrink-0">
          <div className="flex items-baseline gap-1">
            <Money
              amount={amount}
              currency={currency}
              showCents={false}
              className="text-2xl font-semibold"
            />
            <span className="text-sm" style={{ color: 'var(--ink-600)' }}>
              / mo{period === 'annual' ? ' equivalent' : ''}
            </span>
          </div>
          <a
            href={addonCopy.ctaHref}
            className={[
              'inline-block text-center text-sm font-medium py-2 px-5 rounded-md transition-colors',
              'bg-[var(--ink-900)] text-[var(--paper-200)] hover:bg-[var(--moss-700)]',
              'focus:outline-none focus-visible:ring-2 focus-visible:ring-[var(--moss-700)] focus-visible:ring-offset-1',
            ].join(' ')}
            style={{ fontFamily: 'var(--font-editorial-sans, sans-serif)' }}
          >
            {addonCopy.cta}
          </a>
        </div>
      </div>
    </section>
  )
}

// ---------------------------------------------------------------------------
// Main component
// ---------------------------------------------------------------------------

export function PricingClient({ currency, pricing }: PricingClientProps) {
  const [period, setPeriod] = useState<BillingPeriod>('annual')
  const plans = pricingCopy.plans

  return (
    <div className="px-8 py-16 max-w-7xl mx-auto">
      {/* ── Page headline + intro ─────────────────────────────────────────── */}
      <header className="mb-12 max-w-2xl">
        <h1
          className="text-5xl font-semibold leading-tight mb-4"
          style={{
            fontFamily: 'var(--font-editorial-serif, serif)',
            color: 'var(--ink-900)',
          }}
        >
          {pricingCopy.h1}
        </h1>
        <p
          className="text-base leading-relaxed"
          style={{
            fontFamily: 'var(--font-editorial-sans, sans-serif)',
            color: 'var(--ink-700)',
          }}
        >
          {pricingCopy.intro}
        </p>
      </header>

      {/* ── Billing toggle ───────────────────────────────────────────────── */}
      <section aria-label="Billing period selector" className="mb-10">
        <fieldset className="inline-flex items-center gap-1 p-1 rounded-md border border-[var(--hairline,var(--ink-100))]">
          <legend className="sr-only">{pricingCopy.toggle.label}</legend>

          <button
            type="button"
            role="radio"
            aria-checked={period === 'monthly'}
            onClick={() => setPeriod('monthly')}
            className={[
              'px-4 py-1.5 text-sm rounded-sm transition-colors',
              'focus:outline-none focus-visible:ring-2 focus-visible:ring-[var(--moss-700)]',
              period === 'monthly'
                ? 'bg-[var(--ink-900)] text-[var(--paper-200)]'
                : 'text-[var(--ink-700)] hover:text-[var(--ink-900)]',
            ].join(' ')}
            style={{ fontFamily: 'var(--font-editorial-sans, sans-serif)' }}
          >
            {pricingCopy.toggle.monthly}
          </button>

          <button
            type="button"
            role="radio"
            aria-checked={period === 'annual'}
            onClick={() => setPeriod('annual')}
            className={[
              'px-4 py-1.5 text-sm rounded-sm transition-colors',
              'focus:outline-none focus-visible:ring-2 focus-visible:ring-[var(--moss-700)]',
              period === 'annual'
                ? 'bg-[var(--ink-900)] text-[var(--paper-200)]'
                : 'text-[var(--ink-700)] hover:text-[var(--ink-900)]',
            ].join(' ')}
            style={{ fontFamily: 'var(--font-editorial-sans, sans-serif)' }}
          >
            {pricingCopy.toggle.annual}
          </button>
        </fieldset>
      </section>

      {/* ── Plan grid ────────────────────────────────────────────────────── */}
      {/*
        Asymmetric 4-column grid:
        - 1.1fr · 1fr · 1fr · 1.1fr
        - Starter and Pro (edges) are slightly wider — they are the narrative
          anchors. Growth and Scale flank. This is intentional editorial asymmetry,
          not an accident. Do not normalise to equal columns.
        - Hairline right-border between columns (border-r on all but last child).
        - NO rounded cards, NO box shadows.
      */}
      <section
        aria-label="Pricing plans"
        className={[
          // Mobile: single column stack
          'grid grid-cols-1',
          // Tablet: 2 columns
          'md:grid-cols-2',
          // Desktop: intentional asymmetric 4-col split
          'lg:grid-cols-[1.1fr_1fr_1fr_1.1fr]',
          'border border-[var(--hairline,var(--ink-100))] rounded-md overflow-hidden',
        ].join(' ')}
      >
        {pricing.plans.map((plan, index) => {
          const copyCopy = plans.find((p) => p.id === plan.id)
          if (!copyCopy) return null
          return (
            <PlanColumn
              key={plan.id}
              plan={plan}
              copy={copyCopy}
              currency={currency}
              period={period}
              isLast={index === pricing.plans.length - 1}
            />
          )
        })}
      </section>

      {/* ── Pro+App add-on ───────────────────────────────────────────────── */}
      <div className="mt-10">
        <ProAppCard currency={currency} period={period} pricing={pricing} />
      </div>

      {/* ── Pro contact CTAs (annual view only — on monthly the price block handles it) */}
      {period === 'annual' && (
        <section aria-label="Pro plan actions" className="mt-10 pt-8 border-t border-[var(--hairline,var(--ink-100))] flex gap-4 flex-wrap">
          <a
            href={pricingCopy.proCtas.conversationHref}
            className={[
              'inline-block text-sm font-medium py-2.5 px-5 rounded-md transition-colors',
              'bg-[var(--ink-900)] text-[var(--paper-200)] hover:bg-[var(--moss-700)]',
              'focus:outline-none focus-visible:ring-2 focus-visible:ring-[var(--moss-700)] focus-visible:ring-offset-1',
            ].join(' ')}
            style={{ fontFamily: 'var(--font-editorial-sans, sans-serif)' }}
          >
            {pricingCopy.proCtas.conversation}
          </a>
          <a
            href={pricingCopy.proCtas.briefHref}
            className={[
              'inline-block text-sm font-medium py-2.5 px-5 rounded-md transition-colors',
              'border border-[var(--ink-900)] text-[var(--ink-900)] hover:border-[var(--moss-700)] hover:text-[var(--moss-700)]',
              'focus:outline-none focus-visible:ring-2 focus-visible:ring-[var(--moss-700)] focus-visible:ring-offset-1',
            ].join(' ')}
            style={{ fontFamily: 'var(--font-editorial-sans, sans-serif)' }}
          >
            {pricingCopy.proCtas.brief}
          </a>
        </section>
      )}

      {/* ── Disclosures ──────────────────────────────────────────────────── */}
      <footer className="mt-12 pt-6 border-t border-[var(--hairline,var(--ink-100))]">
        <p
          className="text-xs"
          style={{
            fontFamily: 'var(--font-editorial-sans, sans-serif)',
            color: 'var(--ink-500)',
          }}
        >
          {pricingCopy.disclosureTemplate(currency)}
        </p>
      </footer>
    </div>
  )
}

export default PricingClient
