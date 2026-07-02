import type { ReactNode } from "react";
import Link from "next/link";
import { FaqAccordion, type FaqItem } from "@repo/ui/faq-accordion";

import { MarketingPage } from "./primitives";

/* ============================================================
   SeoLanding — shared scaffold for high-intent comparison /
   "alternative" landing pages (#151). One data-driven Server
   Component so every page reads as the same editorial surface
   as the home page: left-aligned, asymmetric, hairline rules,
   one moss accent, the serif carrying the weight.

   The only client island is the shared FaqAccordion, matching
   the home page. Everything else is static markup so these
   search-entry pages ship near-zero JS.
   ============================================================ */

export interface SeoComparisonRow {
  /** Feature dimension, e.g. "Platform fee per sale". */
  label: string;
  /** Mark8ly's answer. */
  mark8ly: string;
  /** The competitor's answer. */
  them: string;
}

export interface SeoSection {
  heading: string;
  /** One or more body paragraphs. */
  body: ReadonlyArray<string>;
}

export interface SeoLandingProps {
  eyebrow: string;
  title: ReactNode;
  lede: string;
  /** Opening prose beneath the hero — the keyword-bearing intro. */
  intro: ReadonlyArray<string>;
  /** Column header for the "them" side, e.g. "Shopify". */
  competitorName: string;
  /** Optional one-line framing above the comparison table. */
  comparisonNote?: string;
  comparison: ReadonlyArray<SeoComparisonRow>;
  /** Editorial body sections, rendered as an asymmetric list. */
  sections: ReadonlyArray<SeoSection>;
  faq: ReadonlyArray<FaqItem>;
  ctaHeading: ReactNode;
  ctaBody: string;
}

export function SeoLanding({
  eyebrow,
  title,
  lede,
  intro,
  competitorName,
  comparisonNote,
  comparison,
  sections,
  faq,
  ctaHeading,
  ctaBody,
}: SeoLandingProps) {
  return (
    <MarketingPage>
      {/* Hero — mirrors PageHero but carries its own CTA so the
          highest-intent visitors can act without scrolling. */}
      <section className="pb-14 pt-16 sm:pt-24">
        <div className="mx-auto max-w-6xl px-6">
          <div className="max-w-3xl">
            <p className="eyebrow mb-5">{eyebrow}</p>
            <h1 className="font-serif text-5xl font-medium leading-[1.02] tracking-[-0.025em] text-foreground">
              {title}
            </h1>
            <p className="mt-6 max-w-xl text-lg leading-[1.55] text-foreground-secondary">
              {lede}
            </p>
            <div className="mt-10 flex flex-wrap items-center gap-x-8 gap-y-4">
              <Link
                href="/onboarding"
                className="inline-flex h-12 items-center rounded-md bg-primary px-6 text-base font-medium text-primary-foreground hover:bg-primary-hover"
              >
                Open your store
              </Link>
              <Link href="/#pricing" className="btn-ghost">
                See the pricing
              </Link>
            </div>
            <p className="mt-8 text-sm text-foreground-tertiary">
              Free for ninety days. No card required. No platform fees, ever.
            </p>
          </div>
        </div>
        <div className="rule mx-auto mt-16 max-w-6xl" />
      </section>

      {/* Intro prose — the keyword-rich opening. */}
      <section className="py-14 sm:py-20">
        <div className="mx-auto max-w-3xl px-6">
          {intro.map((paragraph, i) => (
            <p
              key={i}
              className="mb-5 text-lg leading-[1.6] text-foreground-secondary last:mb-0"
            >
              {paragraph}
            </p>
          ))}
        </div>
      </section>

      {/* Comparison — quiet two-way table, moss accent on the
          Mark8ly column so the eye finds it without shouting. */}
      <section className="border-t border-border-subtle py-16 sm:py-24">
        <div className="mx-auto max-w-6xl px-6">
          <div className="mb-10 grid gap-8 lg:grid-cols-[1fr_2fr] lg:gap-16">
            <div>
              <p className="eyebrow mb-5">Side by side</p>
              <h2 className="font-serif text-4xl font-medium leading-[1.05] tracking-[-0.02em] text-foreground">
                Mark8ly vs {competitorName}.
              </h2>
            </div>
            {comparisonNote ? (
              <p className="max-w-xl self-end text-base leading-relaxed text-foreground-secondary">
                {comparisonNote}
              </p>
            ) : null}
          </div>

          <table className="w-full border-collapse text-left">
            <thead>
              <tr className="border-b border-border-subtle">
                <th className="w-1/3 px-4 py-4 align-bottom font-sans text-[0.9375rem] font-medium text-foreground-secondary">
                  <span className="sr-only">Feature</span>
                </th>
                <th className="px-4 py-4 align-bottom font-serif text-lg font-medium text-moss-700">
                  Mark8ly
                </th>
                <th className="px-4 py-4 align-bottom font-sans text-[0.9375rem] font-medium text-foreground-secondary">
                  {competitorName}
                </th>
              </tr>
            </thead>
            <tbody>
              {comparison.map((row) => (
                <tr key={row.label} className="border-b border-border-subtle">
                  <th
                    scope="row"
                    className="px-4 py-5 align-top font-sans text-[0.9375rem] font-medium text-foreground"
                  >
                    {row.label}
                  </th>
                  <td className="px-4 py-5 align-top font-medium text-foreground">
                    {row.mark8ly}
                  </td>
                  <td className="px-4 py-5 align-top text-foreground-tertiary">
                    {row.them}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>

      {/* Body sections — asymmetric, sticky heading column. */}
      <section className="border-t border-border-subtle py-16 sm:py-24">
        <div className="mx-auto max-w-6xl px-6">
          <div className="space-y-16">
            {sections.map((section) => (
              <article
                key={section.heading}
                className="grid gap-6 lg:grid-cols-[1fr_2fr] lg:gap-16"
              >
                <h2 className="font-serif text-3xl font-medium leading-[1.1] tracking-[-0.015em] text-foreground lg:sticky lg:top-32 lg:self-start">
                  {section.heading}
                </h2>
                <div>
                  {section.body.map((paragraph, i) => (
                    <p
                      key={i}
                      className="mb-4 max-w-2xl leading-relaxed text-foreground-secondary last:mb-0"
                    >
                      {paragraph}
                    </p>
                  ))}
                </div>
              </article>
            ))}
          </div>
        </div>
      </section>

      {/* FAQ — shared accessible accordion island. */}
      <section className="border-t border-border-subtle py-16 sm:py-24">
        <div className="mx-auto max-w-6xl px-6">
          <div className="grid gap-12 lg:grid-cols-[1fr_2fr] lg:gap-16">
            <div className="lg:sticky lg:top-32 lg:self-start">
              <p className="eyebrow mb-5">Anticipated</p>
              <h2 className="font-serif text-4xl font-medium leading-[1.05] tracking-[-0.02em] text-foreground">
                Questions, answered.
              </h2>
            </div>
            <FaqAccordion
              items={[...faq]}
              className="border-t border-border-subtle"
            />
          </div>
        </div>
      </section>

      {/* Final CTA — full-width ink panel, matches the home page. */}
      <section className="border-t border-border-subtle bg-ink-900 py-20 text-paper-50 sm:py-28">
        <div className="mx-auto max-w-5xl px-6 text-left">
          <p className="eyebrow text-paper-400">Ready when you are</p>
          <h2 className="mt-6 font-serif text-5xl font-medium leading-[1.02] tracking-[-0.025em] text-paper-50">
            {ctaHeading}
          </h2>
          <p className="mt-6 max-w-xl text-lg leading-[1.55] text-paper-300">
            {ctaBody}
          </p>
          <div className="mt-12 flex flex-wrap items-center gap-x-8 gap-y-4">
            <Link
              href="/onboarding"
              className="inline-flex h-12 items-center rounded-md bg-paper-50 px-6 text-base font-medium text-ink-900 hover:bg-paper-100"
            >
              Start free
            </Link>
            <Link
              href="/#pricing"
              className="text-paper-300 underline decoration-paper-600 underline-offset-4 hover:text-paper-50"
            >
              Review the pricing first
            </Link>
          </div>
        </div>
      </section>
    </MarketingPage>
  );
}
