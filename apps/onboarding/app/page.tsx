import Link from "next/link";
import { cookies } from "next/headers";
import { FaqAccordion } from "@repo/ui/faq-accordion";
import {
  CURRENCY_COOKIE_NAME,
  Money,
  SHARED_PRICING_CATALOGUE,
  getAddOnPrice,
  getPlanPrice,
  normalizeCurrency,
  type Currency,
  type PlanId,
} from "@repo/ui/subscription";

import { Header } from "@/components/marketing/Header";
import { Footer } from "@/components/marketing/Footer";

/**
 * Marketing landing page.
 *
 * Server Component. The only client island on the page is the
 * FAQ accordion, imported from @repo/ui. Everything else is
 * static markup so the JS bundle on the highest-traffic page
 * stays close to zero.
 *
 * Layout philosophy: editorial, left-aligned, asymmetric. No
 * card grids. No icon tiles above headings. No hero metric
 * strips. The serif (Source Serif 4) carries the weight; one
 * moss accent does the work color used to do.
 */
export default async function HomePage() {
  // Geo-localized currency — middleware sets `mk8_currency` from
  // CF-IPCountry. Fallback is USD so the page always renders.
  const cookieStore = await cookies();
  const currency = normalizeCurrency(
    cookieStore.get(CURRENCY_COOKIE_NAME)?.value,
  );

  return (
    <div className="bg-background text-foreground">
      <Header />

      <main id="main">
        <Hero />
        <Manifesto />
        <Features />
        <Pricing currency={currency} />
        <HowItWorks />
        <Faq />
        <FinalCta />
      </main>

      <Footer />
    </div>
  );
}

/* ============================================================
   Hero
   ------------------------------------------------------------
   Asymmetric: copy left, single editorial moment right. No
   floating badges, no fake browser chrome, no metric strip.
   Headline carries the offer; everything else gets out of the way.
   ============================================================ */

function Hero() {
  return (
    <section className="relative overflow-hidden">
      <div className="mx-auto max-w-6xl px-6 pb-16 pt-16 lg:pb-24 lg:pt-20">
        <div className="max-w-3xl">
          <h1 className="font-serif text-5xl font-medium leading-[1.02] tracking-[-0.025em] text-foreground">
            A storefront
            <br />
            worth opening.
          </h1>
          <p className="mt-8 max-w-xl text-lg leading-[1.55] text-foreground-secondary">
            Mark8ly is a quiet, considered commerce platform for people who
            actually make things. Set up in an afternoon. Keep your margins.
            Sell on a storefront that doesn&rsquo;t look like everyone
            else&rsquo;s.
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
            Free for ninety days. No card required. Three clear plans after
            that, from $29 a month.
          </p>
        </div>
      </div>

      <div className="rule mx-auto max-w-6xl" />
    </section>
  );
}

/* ============================================================
   Manifesto — three short editorial beats. Replaces the old
   trust-badge row + hero metric strip. Left-aligned, asymmetric
   widths, generous whitespace.
   ============================================================ */

function Manifesto() {
  const beats = [
    {
      number: "i.",
      title: "Quiet by design.",
      body:
        "No popups, no abandoned-cart guilt, no aggressive nudges in the admin. Your store should feel like a shop you walk into, not an inbox you can't close.",
    },
    {
      number: "ii.",
      title: "Yours, fully.",
      body:
        "No transaction fees from us. No platform skim. Use your own domain, export your data anytime, leave whenever you want. The store is yours — we just keep it running.",
    },
    {
      number: "iii.",
      title: "Made well.",
      body:
        "Real merchants worked on the design. Real engineers built the infrastructure. The result is a tool that does fewer things, but does them properly.",
    },
  ];

  return (
    <section id="features" className="py-16 sm:py-24">
      <div className="mx-auto max-w-6xl px-6">
        <div className="grid gap-12 lg:grid-cols-[1fr_2fr] lg:gap-16">
          <div className="lg:sticky lg:top-32 lg:self-start">
            <p className="eyebrow mb-5">What we believe</p>
            <h2 className="font-serif text-4xl font-medium leading-[1.05] tracking-[-0.02em] text-foreground">
              Three things we&rsquo;ll never compromise on.
            </h2>
          </div>

          <ol className="space-y-12">
            {beats.map((beat) => (
              <li key={beat.number} className="grid grid-cols-[auto_1fr] gap-8">
                <span className="font-serif text-3xl font-medium text-moss-700">
                  {beat.number}
                </span>
                <div>
                  <h3 className="font-serif text-2xl text-foreground">
                    {beat.title}
                  </h3>
                  <p className="mt-3 max-w-xl text-base leading-relaxed text-foreground-secondary">
                    {beat.body}
                  </p>
                </div>
              </li>
            ))}
          </ol>
        </div>
      </div>
    </section>
  );
}

/* ============================================================
   Features — alternating editorial rows. NOT a 4-up icon-card
   grid. Each feature gets a real moment.
   ============================================================ */

function Features() {
  const features = [
    {
      kicker: "Storefront",
      title: "A theme that feels considered, out of the box.",
      body:
        "Quiet typography, generous whitespace, real attention to product detail pages. The default storefront looks like something you would have hired a designer to build — because we did.",
    },
    {
      kicker: "Checkout",
      title: "Payments that work everywhere customers do.",
      body:
        "Cards, UPI, wallets, and local methods, all behind a single checkout. No upcharges from us. Standard processor fees only.",
    },
    {
      kicker: "Admin",
      title: "An admin you don't have to learn.",
      body:
        "Products, orders, customers, inventory. Each screen does one thing, clearly. No dashboards full of metrics that don't matter to you.",
    },
  ];

  return (
    <section className="border-t border-border-subtle py-16 sm:py-24">
      <div className="mx-auto max-w-6xl px-6">
        <div className="mb-12 max-w-2xl">
          <p className="eyebrow mb-5">What you get</p>
          <h2 className="font-serif text-4xl font-medium leading-[1.05] tracking-[-0.02em] text-foreground">
            Less software,
            <br />
            more shop.
          </h2>
        </div>

        <div className="space-y-14">
          {features.map((f, i) => (
            <article
              key={f.title}
              className={`grid gap-8 lg:grid-cols-[1fr_1.4fr] lg:gap-12 ${
                i % 2 === 1 ? "lg:[&>*:first-child]:order-2" : ""
              }`}
            >
              <div className="lg:max-w-md lg:self-center">
                <p className="eyebrow text-moss-700">{f.kicker}</p>
                <h3
                  className="mt-4 font-serif text-foreground"
                  style={{
                    fontSize: "var(--text-3xl)",
                    lineHeight: 1.1,
                    letterSpacing: "-0.015em",
                  }}
                >
                  {f.title}
                </h3>
                <p className="mt-5 text-foreground-secondary leading-relaxed">
                  {f.body}
                </p>
              </div>
              <FeaturePlate kicker={f.kicker} index={i + 1} />
            </article>
          ))}
        </div>
      </div>
    </section>
  );
}

interface FeaturePlateProps {
  kicker: string;
  index: number;
}

/**
 * FeaturePlate — visual slot beside each feature row.
 *
 * Currently a quiet typographic still — kicker label, serif
 * issue numeral, hairline accent rule. Designed to look like
 * an intentional editorial composition rather than a missing
 * image. Swap each one for a real product screenshot once the
 * admin and storefront surfaces exist.
 */
function FeaturePlate({ kicker, index }: FeaturePlateProps) {
  return (
    <figure
      aria-hidden="true"
      className="relative aspect-[4/3] overflow-hidden rounded-lg bg-paper-100 ring-1 ring-border-subtle"
    >
      <div className="absolute inset-0 flex flex-col justify-between p-10">
        <p className="eyebrow text-foreground-tertiary">{kicker}</p>
        <div>
          <div className="h-px w-12 bg-moss-700" />
          <p
            className="mt-4 font-serif text-foreground"
            style={{
              fontSize: "var(--text-3xl)",
              lineHeight: 1,
              letterSpacing: "-0.02em",
            }}
          >
            №{String(index).padStart(2, "0")}
          </p>
        </div>
      </div>
    </figure>
  );
}

/* ============================================================
   Pricing — three plans, geo-localized currency via the
   `mk8_currency` cookie set by middleware. Reads the shared
   catalogue from @repo/ui/subscription, so admin and onboarding
   always show the same prices for the same visitor.
   ============================================================ */

interface PlanMeta {
  id: PlanId;
  name: string;
  tagline: string;
  cadence: string;
  features: string[];
}

const PLAN_META: PlanMeta[] = [
  {
    id: "starter",
    name: "Starter",
    tagline: "For your first store.",
    cadence: "/mo",
    features: [
      "1 storefront",
      "Up to 100 products",
      "500 orders / month",
      "Your own domain",
    ],
  },
  {
    id: "studio",
    name: "Studio",
    tagline: "For stores finding their rhythm.",
    cadence: "/mo",
    features: [
      "3 storefronts",
      "Unlimited products",
      "5,000 orders / month",
      "Discount codes, advanced analytics",
    ],
  },
  {
    id: "pro",
    name: "Pro",
    tagline: "For teams scaling past $10k a month.",
    cadence: "/mo annual",
    features: [
      "Unlimited storefronts",
      "Unlimited everything",
      "SLA-backed uptime, dedicated infrastructure",
      "Named account manager",
    ],
  },
];

function Pricing({ currency }: { currency: Currency }) {
  const plans = SHARED_PRICING_CATALOGUE.plans;
  const proAppPrice = getAddOnPrice(SHARED_PRICING_CATALOGUE.proApp, currency);

  return (
    <section id="pricing" className="border-t border-border-subtle py-16 sm:py-24">
      <div className="mx-auto max-w-6xl px-6">
        <div className="mb-12 grid gap-8 lg:grid-cols-[1fr_2fr] lg:gap-12">
          <div>
            <p className="eyebrow mb-5">Pricing</p>
            <h2 className="font-serif text-4xl font-medium leading-[1.05] tracking-[-0.02em] text-foreground">
              Three plans.
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

        <div className="grid gap-10 sm:grid-cols-2 lg:grid-cols-[1.05fr_1fr_1.05fr]">
          {PLAN_META.map((meta, i) => {
            const plan = plans.find((p) => p.id === meta.id)!;
            const price = getPlanPrice(plan, currency);
            // Pro uses the annual-monthly-equivalent as the headline
            // (matches admin /pricing's default "annual" toggle).
            // Starter/Studio show the monthly price.
            const headline =
              meta.id === "pro" ? price.annualMonthlyEquivalent : price.monthly;
            return (
              <div
                key={meta.id}
                className={
                  i < PLAN_META.length - 1
                    ? "lg:border-r lg:border-border lg:pr-10"
                    : ""
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
                    fontSize: "clamp(3rem, 1.5rem + 5vw, 5rem)",
                    lineHeight: 0.95,
                    letterSpacing: "-0.03em",
                  }}
                >
                  <Money amount={headline} currency={currency} showCents={false} />
                  <span className="ml-1 font-sans text-base font-normal text-foreground-tertiary">
                    {meta.cadence}
                  </span>
                </p>
                <ul className="mt-6 space-y-3 text-foreground-secondary">
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
            );
          })}
        </div>

        <p className="mt-10 text-sm text-foreground-tertiary">
          Prices shown in {currency}. Annual billing available on every plan.
          Pro includes an optional White-label App add-on from{" "}
          <Money
            amount={proAppPrice.annualMonthlyEquivalent}
            currency={currency}
            showCents={false}
          />{" "}
          /mo (annual).
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
  );
}

/* ============================================================
   How it works — three steps, numbered, left-aligned, narrow
   measure. No "How it works → Three steps" eyebrow redundancy.
   ============================================================ */

function HowItWorks() {
  const steps = [
    {
      number: "01",
      title: "Tell us about your shop.",
      body: "Name, country, what you sell. Two minutes.",
    },
    {
      number: "02",
      title: "Add what you make.",
      body: "Upload products, set prices, organize them however you like.",
    },
    {
      number: "03",
      title: "Open the doors.",
      body: "Share your link. Your storefront is live.",
    },
  ];

  return (
    <section className="border-t border-border-subtle py-16 sm:py-24">
      <div className="mx-auto max-w-6xl px-6">
        <div className="max-w-2xl">
          <p className="eyebrow mb-5">From signup to live</p>
          <h2 className="font-serif text-4xl font-medium leading-[1.05] tracking-[-0.02em] text-foreground">
            An afternoon, start to finish.
          </h2>
        </div>

        <ol className="mt-12 grid gap-10 sm:grid-cols-3 sm:gap-8">
          {steps.map((step) => (
            <li key={step.number}>
              <p className="font-serif text-3xl text-moss-700">
                {step.number}
              </p>
              <h3 className="mt-4 font-serif text-xl leading-[1.2] text-foreground">
                {step.title}
              </h3>
              <p className="mt-3 text-foreground-secondary leading-relaxed">
                {step.body}
              </p>
            </li>
          ))}
        </ol>
      </div>
    </section>
  );
}

/* ============================================================
   FAQ — accessible accordion via @repo/ui island. The only
   client component on this page.
   ============================================================ */

const faqItems = [
  {
    question: "I'm not technical. Can I still use this?",
    answer:
      "Yes — that's the entire point. If you can write an email, you can run a store on Mark8ly. And if you get stuck, real humans answer real messages.",
  },
  {
    question: "What happens after the ninety-day free trial?",
    answer:
      "You choose between three plans — Starter, Studio, or Pro — starting at $29 a month. No added transaction fees from Mark8ly, ever. You can cancel any time and take your data with you.",
  },
  {
    question: "What does Mark8ly take from each sale?",
    answer:
      "Nothing. Your payment processor charges their standard fee (around 2% for UPI, 2–3% for cards). We don't add anything on top of that.",
  },
  {
    question: "Is there a limit on products?",
    answer:
      "Starter caps at 100 products for merchants just opening their first store. Studio and Pro are unlimited — add as many products, photos, and variants as you like.",
  },
  {
    question: "Can I leave?",
    answer:
      "Whenever you like. Export your products, customers, and orders in one click and take everything with you. No hard feelings.",
  },
];

function Faq() {
  return (
    <section id="faq" className="border-t border-border-subtle py-16 sm:py-24">
      <div className="mx-auto max-w-6xl px-6">
        <div className="grid gap-12 lg:grid-cols-[1fr_2fr] lg:gap-16">
          <div className="lg:sticky lg:top-32 lg:self-start">
            <p className="eyebrow mb-5">Anticipated</p>
            <h2 className="font-serif text-4xl font-medium leading-[1.05] tracking-[-0.02em] text-foreground">
              Questions, answered.
            </h2>
          </div>
          <FaqAccordion items={faqItems} className="border-t border-border-subtle" />
        </div>
      </div>
    </section>
  );
}

/* ============================================================
   Final CTA — full-width ink panel. Single confident sentence
   in serif, single CTA, nothing else.
   ============================================================ */

function FinalCta() {
  return (
    <section className="border-t border-border-subtle bg-ink-900 py-20 text-paper-50 sm:py-28">
      <div className="mx-auto max-w-5xl px-6 text-left">
        <p className="eyebrow text-paper-400">Ready when you are</p>
        <h2 className="mt-6 font-serif text-5xl font-medium leading-[1.02] tracking-[-0.025em]">
          Open your shop
          <br />
          this afternoon.
        </h2>
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
  );
}
