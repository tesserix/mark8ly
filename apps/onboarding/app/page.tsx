"use client";

import { useState } from "react";
import Link from "next/link";
import {
  Package,
  CreditCard,
  BarChart3,
  Headphones,
  Users,
  Shield,
  Zap,
  Clock,
  type LucideIcon,
} from "lucide-react";

import { Header } from "@/components/marketing/Header";
import { Footer } from "@/components/marketing/Footer";

/**
 * Marketing landing page. Ported from mark8ly_backup using the same warm
 * editorial design tokens, serif headlines, and dark pill primary buttons.
 *
 * Sections: hero (with dashboard mockup), trust badges, features, how it
 * works, FAQ accordion, final CTA. Pricing + testimonials are deferred
 * until the platform-api content endpoints land in Phase F Tier 2.
 */
export default function HomePage() {
  const [openFaq, setOpenFaq] = useState<number | null>(null);

  return (
    <div className="min-h-screen bg-background text-foreground overflow-x-hidden">
      <Header currentPage="home" />

      {/* Hero */}
      <section className="px-6 pb-16 pt-28 sm:pb-18 sm:pt-30">
        <div className="max-w-6xl mx-auto grid lg:grid-cols-[1.1fr_0.9fr] gap-16 items-center">
          <div>
            <div className="mb-6 inline-flex items-center gap-2 rounded-full border border-sage-200 bg-sage-50 px-4 py-2 text-sm font-medium text-sage-700 shadow-sm">
              <span className="h-2 w-2 rounded-full bg-sage-500" aria-hidden />
              6 months free, then one flat monthly fee
            </div>
            <h1 className="font-serif text-4xl sm:text-5xl md:text-6xl font-medium tracking-tight mb-6 leading-[1.05] text-foreground">
              Your online store,
              <br />
              <span className="text-foreground-secondary">ready this afternoon</span>
            </h1>
            <p className="text-xl text-foreground-secondary max-w-xl mb-8 leading-relaxed">
              Set up your store in under an hour—no developer needed. Just you,
              your products, and customers ready to buy.
            </p>
            <div className="flex flex-col sm:flex-row gap-4 items-start sm:items-center mb-6">
              <Link
                href="/onboarding"
                className="group bg-primary text-primary-foreground px-8 py-4 rounded-xl text-base font-medium hover:bg-primary-hover transition-all hover:shadow-lg hover:-translate-y-0.5 inline-flex items-center gap-2"
              >
                Start Free for 6 Months
                <ArrowRight />
              </Link>
              <Link
                href="/#features"
                className="px-6 py-4 rounded-xl text-base font-medium text-foreground-secondary hover:text-foreground hover:bg-warm-50 transition-all border border-warm-200"
              >
                See how it works
              </Link>
            </div>
            <p className="text-sm text-foreground-tertiary mb-8">
              No credit card required. Cancel anytime.
            </p>

            <div className="mb-8 grid gap-4 sm:grid-cols-3">
              {heroStats.map((item) => (
                <div
                  key={item.label}
                  className="rounded-2xl border border-warm-200/90 bg-white/82 px-4 py-4 shadow-sm"
                >
                  <p className="text-xs uppercase tracking-[0.14em] text-foreground-tertiary">
                    {item.label}
                  </p>
                  <p className="mt-2 text-base font-medium text-foreground">
                    {item.value}
                  </p>
                </div>
              ))}
            </div>

            {/* Trust badges */}
            <div className="flex flex-wrap items-center gap-6 text-sm text-foreground-tertiary">
              {trustBadges.map((b) => {
                const Icon = b.icon;
                return (
                  <div key={b.label} className="flex items-center gap-2">
                    <Icon className="w-4 h-4" />
                    <span>{b.label}</span>
                  </div>
                );
              })}
            </div>
          </div>

          <DashboardMockup />
        </div>
      </section>

      {/* Features */}
      <section id="features" className="border-t border-warm-200 px-6 py-18 sm:py-20">
        <div className="max-w-6xl mx-auto">
          <div className="max-w-2xl mb-14">
            <h2 className="font-serif text-3xl sm:text-4xl font-medium mb-4 text-foreground">
              Everything you need,
              <br />
              <span className="text-foreground-secondary">nothing you don&apos;t</span>
            </h2>
            <p className="text-lg text-foreground-secondary">
              The essentials for launching, selling, and growing without the
              usual complexity.
            </p>
          </div>

          <div className="grid sm:grid-cols-2 lg:grid-cols-4 gap-6">
            {features.map((f) => {
              const Icon = f.icon;
              return (
                <div
                  key={f.title}
                  className="group p-6 rounded-xl bg-white border border-warm-200 shadow-sm hover:shadow-md hover:border-warm-300 transition-all duration-200 hover:-translate-y-1"
                >
                  <div className="w-12 h-12 rounded-xl bg-warm-100 flex items-center justify-center mb-4 group-hover:bg-primary group-hover:scale-110 transition-all duration-200">
                    <Icon className="w-6 h-6 text-warm-600 group-hover:text-white transition-colors" />
                  </div>
                  <h3 className="text-lg font-medium text-foreground mb-2">
                    {f.title}
                  </h3>
                  <p className="text-foreground-secondary leading-relaxed text-sm">
                    {f.description}
                  </p>
                </div>
              );
            })}
          </div>
        </div>
      </section>

      {/* Pricing */}
      <section id="pricing" className="border-t border-warm-200 px-6 py-18 sm:py-20">
        <div className="max-w-5xl mx-auto">
          <div className="text-center mb-12">
            <h2 className="font-serif text-3xl sm:text-4xl font-medium mb-4 text-foreground">
              Simple, honest pricing
            </h2>
            <p className="text-lg text-foreground-secondary">
              One clear offer: launch free for 6 months, then keep growing on a
              flat monthly fee with no transaction fees from Mark8ly.
            </p>
          </div>

          <div className="grid gap-6 lg:grid-cols-[1.2fr_0.8fr] items-stretch">
            <div className="rounded-[2rem] border border-foreground/10 bg-white p-6 shadow-[0_20px_60px_rgba(43,38,34,0.08)] sm:p-8">
              <div className="flex flex-wrap items-center gap-3 mb-6">
                <div className="inline-flex items-center rounded-full bg-foreground px-3 py-1 text-xs font-medium text-background">
                  The Offer
                </div>
                <p className="text-sm text-foreground-secondary">
                  Built for new and growing independent merchants
                </p>
              </div>

              <div className="grid gap-4 sm:grid-cols-2">
                {pricingPhases.map((phase) => (
                  <div
                    key={phase.name}
                    className={`rounded-[1.5rem] border p-6 ${
                      phase.emphasis
                        ? "border-foreground/10 bg-[linear-gradient(180deg,rgba(31,30,28,0.03),rgba(255,255,255,1))]"
                        : "border-warm-200 bg-warm-50/55"
                    }`}
                  >
                    <p className="text-xs uppercase tracking-[0.16em] text-foreground-tertiary">
                      {phase.name}
                    </p>
                    <div className="mt-4 flex items-end gap-2">
                      <span className="font-serif text-4xl font-medium text-foreground">
                        {phase.price}
                      </span>
                      {phase.cycle && (
                        <span className="pb-1 text-sm text-foreground-secondary">
                          {phase.cycle}
                        </span>
                      )}
                    </div>
                    <p className="mt-3 text-sm leading-6 text-foreground-secondary">
                      {phase.description}
                    </p>
                    <ul className="mt-5 space-y-2.5">
                      {phase.features.map((feature) => (
                        <li key={feature} className="flex items-start gap-2.5">
                          <svg
                            width="16"
                            height="16"
                            viewBox="0 0 24 24"
                            fill="none"
                            stroke="currentColor"
                            strokeWidth="3"
                            strokeLinecap="round"
                            strokeLinejoin="round"
                            className="text-sage-500 mt-0.5 flex-shrink-0"
                          >
                            <path d="M5 13l4 4L19 7" />
                          </svg>
                          <span className="text-sm text-foreground-secondary">
                            {feature}
                          </span>
                        </li>
                      ))}
                    </ul>
                  </div>
                ))}
              </div>

              <div className="mt-6 flex flex-col gap-4 rounded-[1.5rem] border border-warm-200 bg-warm-50/75 p-5 sm:flex-row sm:items-center sm:justify-between">
                <div>
                  <p className="text-sm font-medium text-foreground">
                    No setup fee. No transaction fee from Mark8ly. No migration later.
                  </p>
                  <p className="mt-1 text-sm text-foreground-secondary">
                    Start once, keep the same store, and move into paid growth
                    without rebuilding anything.
                  </p>
                </div>
                <Link
                  href="/onboarding"
                  className="inline-flex min-w-[168px] items-center justify-center gap-2 self-start rounded-2xl bg-primary px-6 py-3.5 text-base font-medium text-primary-foreground shadow-[0_12px_24px_rgba(31,30,28,0.16)] transition-[background-color,box-shadow,transform] hover:bg-primary-hover hover:shadow-[0_16px_30px_rgba(31,30,28,0.2)] sm:self-center"
                >
                  <span>Start Free</span>
                  <ArrowRight />
                </Link>
              </div>
            </div>

            <div className="rounded-[2rem] border border-warm-200 bg-[linear-gradient(180deg,rgba(243,238,230,0.88),rgba(255,255,255,0.98))] p-6 shadow-sm sm:p-8">
              <p className="text-xs uppercase tracking-[0.16em] text-foreground-tertiary">
                Why this pricing works
              </p>
              <h3 className="mt-4 font-serif text-2xl font-medium text-foreground">
                Enough runway to launch well, without delaying paid validation.
              </h3>
              <p className="mt-4 text-sm leading-6 text-foreground-secondary">
                Six free months gives merchants room to set up properly,
                collect early orders, and build momentum. After that, one flat
                fee keeps pricing easy to understand while protecting your
                margins from extra platform cuts.
              </p>

              <div className="mt-6 space-y-4">
                {pricingPrinciples.map((item) => (
                  <div
                    key={item.title}
                    className="rounded-2xl border border-white/70 bg-white/82 p-4"
                  >
                    <p className="text-sm font-medium text-foreground">
                      {item.title}
                    </p>
                    <p className="mt-1 text-sm leading-6 text-foreground-secondary">
                      {item.body}
                    </p>
                  </div>
                ))}
              </div>
            </div>
          </div>
        </div>
      </section>

      {/* Testimonials */}
      <section id="testimonial" className="border-t border-warm-200 px-6 py-18 sm:py-20">
        <div className="max-w-6xl mx-auto">
          <div className="text-center mb-14">
            <p className="mb-3 text-xs uppercase tracking-[0.16em] text-foreground-tertiary">
              Merchant stories
            </p>
            <div className="flex items-center justify-center gap-1 mb-4 text-amber-400 text-2xl">
              {[1, 2, 3, 4, 5].map((i) => (
                <span key={i}>★</span>
              ))}
            </div>
            <p className="text-foreground-secondary">
              Rated 4.9/5 from 150+ reviews
            </p>
          </div>

          <div className="grid md:grid-cols-3 gap-6">
            {testimonials.map((t, index) => (
              <div
                key={t.name}
                className={`rounded-[1.75rem] border p-6 shadow-sm ${
                  index === 1
                    ? "border-foreground/10 bg-[linear-gradient(180deg,rgba(243,238,230,0.95),rgba(255,255,255,1))] shadow-[0_20px_60px_rgba(43,38,34,0.08)]"
                    : "border-warm-200 bg-white"
                }`}
              >
                <div className="mb-4 flex items-center justify-between">
                  <div className="flex gap-1 text-amber-400">
                    {[1, 2, 3, 4, 5].map((i) => (
                      <span key={i}>★</span>
                    ))}
                  </div>
                  <span className="rounded-full bg-warm-100 px-3 py-1 text-xs font-medium text-foreground-secondary">
                    {t.highlight}
                  </span>
                </div>
                <blockquote className="mb-4 font-serif text-lg leading-7 text-foreground sm:text-xl sm:leading-8">
                  &ldquo;{t.quote}&rdquo;
                </blockquote>
                <p className="mb-5 text-sm leading-6 text-foreground-secondary">
                  {t.context}
                </p>
                <div className="flex items-center gap-3">
                  <div className="w-10 h-10 rounded-full bg-warm-100 flex items-center justify-center text-sm font-medium text-warm-700">
                    {t.initials}
                  </div>
                  <div>
                    <div className="font-medium text-foreground">{t.name}</div>
                    <div className="text-sm text-foreground-tertiary">
                      {t.role}, {t.company}
                    </div>
                  </div>
                </div>
              </div>
            ))}
          </div>
        </div>
      </section>

      {/* How It Works */}
      <section className="border-t border-warm-200 px-6 py-16 sm:py-18">
        <div className="max-w-5xl mx-auto">
          <div className="text-center mb-10">
            <p className="mb-2 text-xs uppercase tracking-[0.16em] text-foreground-tertiary">
              How it works
            </p>
            <h2 className="font-serif text-3xl sm:text-4xl font-medium mb-4 text-foreground">
              Three steps to your new store
            </h2>
            <p className="mx-auto max-w-2xl text-lg text-foreground-secondary">
              Tell us about yourself, add your products, and you&apos;re live.
              That&apos;s really all there is to it.
            </p>
          </div>

          <div className="grid gap-6 md:grid-cols-3">
            {howItWorks.map((item) => (
              <div
                key={item.step}
                className="relative overflow-hidden rounded-[1.75rem] border border-warm-200 bg-white p-6 shadow-sm"
              >
                <div
                  aria-hidden
                  className="absolute right-0 top-0 h-24 w-24 rounded-full bg-terracotta-100/55 blur-2xl"
                />
                <div className="relative">
                  <div className="mb-4 flex h-10 w-10 items-center justify-center rounded-full bg-terracotta-100 text-sm font-medium text-terracotta-700">
                    {item.step}
                  </div>
                  <h3 className="mb-2 text-lg font-medium text-foreground">
                    {item.title}
                  </h3>
                  <p className="text-sm leading-6 text-foreground-secondary">
                    {item.body}
                  </p>
                </div>
              </div>
            ))}
          </div>

          <div className="mt-8 text-center">
            <Link
              href="/onboarding"
              className="group inline-flex items-center gap-2 rounded-full bg-primary px-8 py-3.5 text-base font-medium text-primary-foreground transition-[background-color,box-shadow] hover:bg-primary-hover hover:shadow-lg"
            >
              Let&apos;s Get Started
              <ArrowRight />
            </Link>
          </div>
        </div>
      </section>

      {/* FAQ */}
      <section id="faq" className="border-t border-warm-200 px-6 py-18 sm:py-20">
        <div className="max-w-3xl mx-auto">
          <div className="text-center mb-12">
            <p className="mb-3 text-xs uppercase tracking-[0.16em] text-foreground-tertiary">
              Questions answered
            </p>
            <h2 className="font-serif text-3xl sm:text-4xl font-medium text-foreground">
              Questions you might have
            </h2>
          </div>

          <div className="space-y-4">
            {faqs.map((faq, i) => (
              <div
                key={faq.question}
                className={`overflow-hidden rounded-[1.5rem] border transition-[border-color,box-shadow] ${
                  openFaq === i
                    ? "border-foreground/10 bg-white shadow-[0_16px_40px_rgba(43,38,34,0.08)]"
                    : "border-warm-200 bg-white/92"
                }`}
              >
                <button
                  type="button"
                  aria-expanded={openFaq === i}
                  onClick={() => setOpenFaq(openFaq === i ? null : i)}
                  className="flex w-full items-center justify-between px-6 py-5 text-left transition-colors hover:bg-warm-50/60"
                >
                  <span className="pr-4 text-base font-medium text-foreground">
                    {faq.question}
                  </span>
                  <span className="flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-full bg-warm-50 text-foreground-tertiary">
                    <svg
                      width="18"
                      height="18"
                      viewBox="0 0 24 24"
                      fill="none"
                      stroke="currentColor"
                      strokeWidth="2"
                      strokeLinecap="round"
                      strokeLinejoin="round"
                      className={`transition-transform duration-200 ${openFaq === i ? "rotate-180" : ""}`}
                    >
                      <polyline points="6 9 12 15 18 9" />
                    </svg>
                  </span>
                </button>
                <div
                  className={`overflow-hidden transition-all duration-200 ${openFaq === i ? "max-h-96" : "max-h-0"}`}
                >
                  <div className="px-6 pb-6">
                    <p className="leading-7 text-foreground-secondary">
                      {faq.answer}
                    </p>
                  </div>
                </div>
              </div>
            ))}
          </div>
        </div>
      </section>

      {/* Final CTA */}
      <section className="border-t border-warm-200 px-6 py-18 sm:py-20">
        <div className="max-w-5xl mx-auto">
          <div className="relative overflow-hidden rounded-[2.25rem] border border-warm-200/90 bg-[linear-gradient(180deg,rgba(243,238,230,0.85),rgba(255,255,255,0.98))] px-8 py-12 text-center shadow-[0_24px_80px_rgba(43,38,34,0.08)] sm:px-12">
            <div
              aria-hidden
              className="pointer-events-none absolute -left-10 top-0 h-40 w-40 rounded-full bg-sage-100/70 blur-3xl"
            />
            <div
              aria-hidden
              className="pointer-events-none absolute -right-10 bottom-0 h-40 w-40 rounded-full bg-terracotta-100/70 blur-3xl"
            />
            <div className="relative">
              <p className="mb-3 text-xs uppercase tracking-[0.16em] text-foreground-tertiary">
                Ready when you are
              </p>
              <h2 className="font-serif text-3xl sm:text-5xl font-medium mb-6 text-foreground">
                Ready to open your doors?
              </h2>
              <p className="mx-auto mb-10 max-w-2xl text-xl leading-8 text-foreground-secondary">
                Start free for 6 months, keep every order free from extra
                platform transaction fees, and launch with a storefront that
                feels polished from day one.
              </p>
              <div className="flex flex-col items-center justify-center gap-4 sm:flex-row">
                <Link
                  href="/onboarding"
                  className="group inline-flex items-center gap-3 rounded-full bg-primary px-10 py-4 text-lg font-medium text-primary-foreground transition-[background-color,box-shadow] hover:bg-primary-hover hover:shadow-[0_18px_36px_rgba(31,30,28,0.2)]"
                >
                  Create Your Store
                  <ArrowRight />
                </Link>
                <Link
                  href="/#pricing"
                  className="inline-flex items-center gap-2 rounded-full border border-warm-300 bg-white/80 px-6 py-4 text-base font-medium text-foreground-secondary transition-colors hover:border-warm-400 hover:text-foreground"
                >
                  Review Pricing
                </Link>
              </div>
              <p className="mt-5 text-sm text-foreground-tertiary">
                Free for 6 months, then $9.99/month. Cancel anytime.
              </p>
            </div>
          </div>
        </div>
      </section>

      <Footer />
    </div>
  );
}

function ArrowRight() {
  return (
    <svg
      width="20"
      height="20"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      className="transition-transform group-hover:translate-x-0.5"
      aria-hidden
    >
      <path d="M5 12h14M13 5l7 7-7 7" />
    </svg>
  );
}

/**
 * Faux admin dashboard preview shown in the hero. Mirrors the legacy
 * mark8ly_backup mockup so the hero has visual weight without needing
 * any real data or API calls.
 */
function DashboardMockup() {
  return (
    <div className="relative" aria-hidden>
      <div className="rounded-[2rem] border border-warm-200 bg-[linear-gradient(180deg,rgba(255,255,255,0.98),rgba(250,247,242,0.95))] p-5 shadow-[0_24px_80px_rgba(43,38,34,0.12)] transform hover:scale-[1.02] transition-transform duration-300">
        {/* Browser bar */}
        <div className="flex items-center gap-2 mb-4 pb-4 border-b border-warm-100">
          <div className="flex gap-1.5">
            <div className="w-3 h-3 rounded-full bg-warm-200" />
            <div className="w-3 h-3 rounded-full bg-warm-200" />
            <div className="w-3 h-3 rounded-full bg-warm-200" />
          </div>
          <div className="mx-4 flex h-6 flex-1 items-center rounded-md bg-warm-50 px-3 text-[11px] text-foreground-tertiary">
            your-store-admin.mark8ly.com
          </div>
        </div>

        {/* Stats */}
        <div className="grid grid-cols-3 gap-3">
          <div className="p-3 rounded-lg bg-sage-50 border border-sage-100">
            <div className="text-xs text-sage-700 mb-1">Revenue</div>
            <div className="text-lg font-semibold text-foreground">$1,350</div>
          </div>
          <div className="p-3 rounded-lg bg-warm-50 border border-warm-100">
            <div className="text-xs text-warm-600 mb-1">Orders</div>
            <div className="text-lg font-semibold text-foreground">284</div>
          </div>
          <div className="p-3 rounded-lg bg-warm-50 border border-warm-100">
            <div className="text-xs text-warm-600 mb-1">Visitors</div>
            <div className="text-lg font-semibold text-foreground">3.2K</div>
          </div>
        </div>

        {/* Chart */}
        <div className="mt-4 h-32 rounded-xl bg-gradient-to-t from-sage-100 to-transparent border border-warm-100 flex items-end p-4 gap-2">
          {[40, 65, 45, 80, 55, 90, 70].map((h, i) => (
            <div
              key={i}
              className="flex-1 bg-sage-400/60 rounded-t"
              style={{ height: `${h}%` }}
            />
          ))}
        </div>

        {/* Product rows */}
        <div className="space-y-2 mt-4">
          {[1, 2].map((i) => (
            <div
              key={i}
              className="flex items-center gap-3 p-2 rounded-lg bg-warm-50/60"
            >
              <div className="w-10 h-10 rounded-lg bg-warm-200" />
              <div className="flex-1">
                <div className="h-3 w-24 bg-warm-200 rounded mb-1" />
                <div className="h-2 w-16 bg-warm-100 rounded" />
              </div>
              <div className="h-3 w-12 bg-sage-200 rounded" />
            </div>
          ))}
        </div>
      </div>

      {/* Floating new-order badge */}
      <div className="absolute -bottom-4 -left-4 p-4 rounded-xl bg-white border border-warm-200 shadow-lg">
        <div className="flex items-center gap-2">
          <div className="w-8 h-8 rounded-full bg-sage-100 flex items-center justify-center">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="3" strokeLinecap="round" strokeLinejoin="round" className="text-sage-700">
              <path d="M5 13l4 4L19 7" />
            </svg>
          </div>
          <div>
            <div className="text-xs text-foreground-tertiary">New order</div>
            <div className="text-sm font-medium text-foreground">$96</div>
          </div>
        </div>
      </div>

      {/* Floating rating */}
      <div className="absolute -top-4 -right-4 p-3 rounded-xl bg-white border border-warm-200 shadow-lg">
        <div className="flex items-center gap-2">
          <div className="flex items-center gap-0.5 text-amber-400 text-sm">
            {[1, 2, 3, 4, 5].map((i) => (
              <span key={i}>★</span>
            ))}
          </div>
          <span className="rounded-full bg-warm-50 px-2 py-1 text-[11px] font-medium text-foreground-secondary">
            Best seller
          </span>
        </div>
        <div className="text-xs text-foreground-tertiary mt-1">4.9 rating</div>
      </div>
    </div>
  );
}

interface Feature {
  icon: LucideIcon;
  title: string;
  description: string;
}

const features: Feature[] = [
  {
    icon: Package,
    title: "Make It Yours",
    description:
      "Customizable themes that match your brand. No design skills needed.",
  },
  {
    icon: CreditCard,
    title: "Sell Everywhere",
    description:
      "Accept payments from customers around the world in their preferred currency.",
  },
  {
    icon: BarChart3,
    title: "Know Your Numbers",
    description:
      "Simple analytics that help you understand what's working and what's not.",
  },
  {
    icon: Headphones,
    title: "We've Got Your Back",
    description:
      "Real humans ready to help when you need it. No chatbots, just friendly support.",
  },
];

interface TrustBadge {
  icon: LucideIcon;
  label: string;
}

const trustBadges: TrustBadge[] = [
  { icon: Users, label: "No Developer Needed" },
  { icon: Shield, label: "SSL Secured" },
  { icon: Zap, label: "99.9% Uptime" },
  { icon: Clock, label: "24/7 Support" },
];

const heroStats = [
  { label: "Launch time", value: "Under an hour" },
  { label: "Platform fee", value: "$0 for 6 months" },
  { label: "Transaction fee", value: "None from Mark8ly" },
];

const pricingPhases = [
  {
    name: "First 6 Months",
    price: "$0",
    cycle: "for 6 months",
    emphasis: false,
    description:
      "Launch your store, learn the platform, and make your first sales before the platform fee starts.",
    features: [
      "Unlimited products",
      "Beautiful mobile-friendly storefront",
      "Payments, shipping, and checkout included",
      "Order management",
      "24/7 human support",
    ],
  },
  {
    name: "After That",
    price: "$9.99",
    cycle: "/month",
    emphasis: true,
    description:
      "One flat monthly fee once the free period ends. No extra platform transaction fees and no awkward plan maze.",
    features: [
      "Keep the same store and data",
      "Use your own domain name",
      "Analytics and growth tools",
      "No transaction fee from Mark8ly",
      "Cancel anytime",
    ],
  },
];

const pricingPrinciples = [
  {
    title: "Long runway",
    body: "Six free months is enough time to get live, learn fast, and build early momentum.",
  },
  {
    title: "Fair paid plan",
    body: "$9.99/month keeps the offer simple, accessible, and easy to understand for independent merchants.",
  },
  {
    title: "No surprises",
    body: "No setup fee, no confusing tiers, and no platform transaction fee layered on top.",
  },
];

const testimonials = [
  {
    quote:
      "I spent months trying to figure out Shopify. With mark8ly, I had my store up in an afternoon. It just... works.",
    context:
      "The difference wasn’t just setup speed. It was finally having a storefront that looked considered without hiring anyone to build it.",
    highlight: "Fast launch",
    name: "Sarah Chen",
    role: "Founder",
    company: "BloomBox",
    initials: "SC",
  },
  {
    quote:
      "The onboarding was so smooth I thought I must be missing something. Nope—it really is that simple. My store was live the same day.",
    context:
      "What stood out most was how little friction there was between signing up and actually feeling ready to sell.",
    highlight: "Smooth onboarding",
    name: "Marcus Rivera",
    role: "Owner",
    company: "Craft & Co",
    initials: "MR",
  },
  {
    quote:
      "Finally, an e-commerce platform that doesn't make me feel stupid. Clean, fast, and the support team actually responds.",
    context:
      "It feels designed for real business owners, not for people who want to spend their week learning backend settings.",
    highlight: "Human support",
    name: "Emily Tran",
    role: "Founder",
    company: "Luna Candles",
    initials: "ET",
  },
];

const howItWorks = [
  {
    step: "1",
    title: "Tell us who you are",
    body: "Your name, your business—the basics.",
  },
  {
    step: "2",
    title: "Add what you sell",
    body: "Upload products, set prices, organize your catalog.",
  },
  {
    step: "3",
    title: "You're open for business",
    body: "Start sharing your store and making sales.",
  },
];

const faqs = [
  {
    question: "I'm not very technical. Can I still use this?",
    answer:
      "Absolutely. We built this for people who want to focus on their business, not on learning software. If you can use email, you can use mark8ly. And if you get stuck, we're here to help—no judgment, just friendly guidance.",
  },
  {
    question: "What happens after the free period?",
    answer:
      "After your free period, simple flat pricing kicks in. No hidden fees, no transaction costs from us, no surprises. And you can cancel anytime.",
  },
  {
    question: "Are there transaction fees or payment processing fees?",
    answer:
      "You'll pay standard payment processing fees (around 2% for UPI, 2-3% for cards). But unlike other platforms, we don't take an extra cut. Your money is your money.",
  },
  {
    question: "What if I decide this isn't for me?",
    answer:
      "Cancel anytime, no questions asked. You can even export all your data—products, customers, orders—and take it with you. No hard feelings.",
  },
  {
    question: "How many products can I add?",
    answer:
      "As many as you want. Unlimited products, unlimited photos, unlimited everything. We're not in the business of nickel-and-diming you.",
  },
  {
    question: "Do I need to hire a developer to set this up?",
    answer:
      "Not at all. That's the whole point. You can set up your entire store yourself—customize the design, add products, configure payments—all without writing a single line of code.",
  },
];
