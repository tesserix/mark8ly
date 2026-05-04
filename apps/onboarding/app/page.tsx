import Image from "next/image";
import Link from "next/link";
import { cookies } from "next/headers";
import { FaqAccordion } from "@repo/ui/faq-accordion";
import {
  CURRENCY_COOKIE_NAME,
  Money,
  SHARED_PRICING_CATALOGUE,
  getPlanPrice,
  normalizeCurrency,
  type Currency,
} from "@repo/ui/subscription";

import { Header } from "@/components/marketing/Header";
import { Footer } from "@/components/marketing/Footer";
import { Pricing } from "@/components/marketing/Pricing";

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
        <Tour />
        <Manifesto />
        <Features />
        <Pricing currency={currency} catalogue={SHARED_PRICING_CATALOGUE} />
        <Comparison currency={currency} />
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
   Tour — editorial product film. Asymmetric heading + body,
   then a 16:9 video frame matching the FeaturePlate language
   (rounded-lg, ring on paper). Poster shows first frame so
   the moment reads as composed before play. Forty-five
   seconds of the actual product running, no marketing tricks.
   ============================================================ */

function Tour() {
  return (
    <section aria-labelledby="tour-heading" className="py-16 sm:py-24">
      <div className="mx-auto max-w-6xl px-6">
        <div className="mb-10 grid gap-8 lg:mb-14 lg:grid-cols-[1fr_2fr] lg:gap-16">
          <div>
            <p className="eyebrow mb-5">The product</p>
            <h2
              id="tour-heading"
              className="font-serif text-4xl font-medium leading-[1.05] tracking-[-0.02em] text-foreground"
            >
              See it move.
            </h2>
          </div>
          <p className="max-w-xl self-end text-base leading-relaxed text-foreground-secondary">
            Forty-five seconds inside a working Mark8ly storefront, checkout
            and admin. No screenshots, no marketing tricks &mdash; just the
            product running.
          </p>
        </div>

        <figure className="relative aspect-video overflow-hidden rounded-lg bg-paper-100 ring-1 ring-border-subtle">
          <video
            controls
            playsInline
            preload="metadata"
            poster="/video/tour-poster.jpg"
            className="absolute inset-0 h-full w-full object-cover"
            aria-label="Mark8ly product tour"
          >
            <source src="/video/tour.mp4" type="video/mp4" />
          </video>
        </figure>
      </div>

      <div className="rule mx-auto mt-16 max-w-6xl sm:mt-24" />
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
      screen: "/screens/storefront.png",
      screenAlt: "Mark8ly storefront — coastal hero with editorial typography",
    },
    {
      kicker: "Checkout",
      title: "Payments that work everywhere customers do.",
      body:
        "Cards, UPI, wallets, and local methods, all behind a single checkout. No upcharges from us. Standard processor fees only.",
      screen: "/screens/checkout.png",
      screenAlt: "Mark8ly checkout — single page with order summary and progressive contact, address, shipping, payment steps",
    },
    {
      kicker: "Admin",
      title: "An admin you don't have to learn.",
      body:
        "Products, orders, customers, inventory. Each screen does one thing, clearly. No dashboards full of metrics that don't matter to you.",
      screen: "/screens/admin.png",
      screenAlt: "Mark8ly admin — branding settings with editorial layout gallery",
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
              <FeaturePlate
                kicker={f.kicker}
                index={i + 1}
                src={f.screen}
                alt={f.screenAlt}
              />
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
  src: string;
  alt: string;
}

function FeaturePlate({ kicker, index, src, alt }: FeaturePlateProps) {
  return (
    <figure className="relative aspect-[4/3] overflow-hidden rounded-lg bg-paper-100 ring-1 ring-border-subtle">
      <Image
        src={src}
        alt={alt}
        fill
        sizes="(min-width: 1024px) 50vw, 100vw"
        className="object-cover object-top"
      />
      <figcaption className="absolute bottom-0 left-0 right-0 flex items-end justify-between bg-gradient-to-t from-ink-900/40 to-transparent px-6 pb-4 pt-12 text-paper-50">
        <span className="eyebrow text-paper-50/80">{kicker}</span>
        <span className="font-serif text-sm tracking-tight">
          №{String(index).padStart(2, "0")}
        </span>
      </figcaption>
    </figure>
  );
}

/* ============================================================
   Pricing — rendered by the client island at
   components/marketing/Pricing.tsx. Source of truth for prices
   is @repo/ui/subscription/pricing-data.ts (shared with admin
   /pricing). Feature copy matches spec v2.3 §9 exactly.
   ============================================================ */

/* ============================================================
   Comparison — quiet, factual table that puts Mark8ly next to
   the names readers have already weighed: Shopify, BigCommerce,
   Wix, Squarespace.

   The "Starting price" row is geo-localized: Mark8ly's Starter
   tracks the SHARED_PRICING_CATALOGUE (single source of truth
   with /admin/pricing). Competitor prices use each platform's
   own *published* per-country price (Shopify India, Shopify AU,
   Wix UK, etc.) — not FX-converted from USD, because none of
   those platforms sell at FX rates. Currencies the platform
   doesn't localize fall back to USD with a "(USD)" tag so a
   merchant sees what they'd actually be charged.

   Editorial, not a SaaS bake-off — hairline rules between rows,
   no shaded card, Mark8ly column carries a single moss accent
   so the eye finds it without shouting.
   ============================================================ */

interface CompareRow {
  label: string;
  mark8ly: string;
  shopify: string;
  bigcommerce: string;
  wix: string;
  squarespace: string;
}

// Feature rows (currency-agnostic). The Starting-price row is
// rendered separately and pulls from COMPETITOR_STARTING_PRICES
// + the shared catalogue.
const COMPARISON_ROWS: readonly CompareRow[] = [
  {
    label: "Free to try",
    mark8ly: "90 days",
    shopify: "3 days",
    bigcommerce: "15 days",
    wix: "14 days",
    squarespace: "14 days",
  },
  {
    label: "Platform fee per sale",
    mark8ly: "None",
    shopify: "2%, unless you use Shopify Payments",
    bigcommerce: "None",
    wix: "None",
    squarespace: "None",
  },
  {
    label: "Default storefront design",
    mark8ly: "Editorial, designer-led",
    shopify: "Generic templates",
    bigcommerce: "Functional templates",
    wix: "Drag-and-drop builder",
    squarespace: "Designer themes",
  },
  {
    label: "Use your own domain",
    mark8ly: "Included",
    shopify: "Bring your own",
    bigcommerce: "Bring your own",
    wix: "First year included",
    squarespace: "First year included",
  },
  {
    label: "Local payments (UPI, wallets)",
    mark8ly: "Built in",
    shopify: "Limited by region",
    bigcommerce: "Limited by region",
    wix: "Limited by region",
    squarespace: "Mostly Stripe / PayPal",
  },
  {
    label: "Take your data when you leave",
    mark8ly: "One click, any time",
    shopify: "CSV export",
    bigcommerce: "CSV export",
    wix: "Partial",
    squarespace: "Partial",
  },
];

type CompetitorId = "shopify" | "bigcommerce" | "wix" | "squarespace";

// Pre-formatted strings, kept as strings (not minor units) because
// each competitor uses their own grouping/symbol convention on their
// own pricing pages — preserving that is more honest than re-formatting
// through Intl. Currencies missing from a competitor's map fall back
// to its USD entry with a "(USD)" tag in the renderer below.
const COMPETITOR_STARTING_PRICES: Record<
  CompetitorId,
  Partial<Record<Currency, string>>
> = {
  shopify: {
    USD: "$39",
    CAD: "C$54",
    GBP: "£25",
    EUR: "€36",
    AUD: "A$54",
    NZD: "NZ$54",
    SGD: "S$53",
    INR: "₹1,994",
  },
  bigcommerce: {
    // BigCommerce bills the Standard plan in USD globally — no
    // localized regional pricing pages.
    USD: "$39",
  },
  wix: {
    USD: "$29",
    CAD: "C$38",
    GBP: "£19",
    EUR: "€23",
    AUD: "A$45",
    NZD: "NZ$45",
    SGD: "S$39",
    INR: "₹1,000",
  },
  squarespace: {
    USD: "$27",
    CAD: "C$36",
    GBP: "£20",
    EUR: "€26",
    AUD: "A$48",
    NZD: "NZ$48",
  },
};

function competitorStartingPrice(
  id: CompetitorId,
  currency: Currency,
): string {
  const map = COMPETITOR_STARTING_PRICES[id];
  const localized = map[currency];
  if (localized) return `${localized} / mo`;
  // Platform doesn't publish a price in this currency — they bill
  // in USD. Tag it so a non-US merchant knows what they'd actually pay.
  const usd = map.USD!;
  return currency === "USD" ? `${usd} / mo` : `${usd} / mo (USD)`;
}

interface ComparisonProps {
  currency: Currency;
}

function Comparison({ currency }: ComparisonProps) {
  const competitorHeaderClass =
    "px-4 py-4 text-left align-bottom font-sans text-[0.9375rem] font-medium text-foreground-secondary";
  const competitorCellClass =
    "px-4 py-5 align-top text-foreground-tertiary";

  // Mark8ly's Starter monthly tracks the shared pricing catalogue
  // — same source of truth as the Pricing section above, so a price
  // change there flows here automatically.
  const starter = SHARED_PRICING_CATALOGUE.plans.find((p) => p.id === "starter");
  const mark8lyStarterMonthly = starter
    ? getPlanPrice(starter, currency).monthly
    : null;

  return (
    <section
      id="compare"
      aria-labelledby="compare-heading"
      className="border-t border-border-subtle py-16 sm:py-24"
    >
      <div className="mx-auto max-w-6xl px-6">
        <div className="mb-12 grid gap-8 lg:grid-cols-[1fr_2fr] lg:gap-16">
          <div>
            <p className="eyebrow mb-5">How we compare</p>
            <h2
              id="compare-heading"
              className="font-serif text-4xl font-medium leading-[1.05] tracking-[-0.02em] text-foreground"
            >
              Side by side,
              <br />
              no spin.
            </h2>
          </div>
          <p className="max-w-xl self-end text-base leading-relaxed text-foreground-secondary">
            We&rsquo;re not the only commerce platform on the table. Here&rsquo;s
            how Mark8ly lines up against the names you&rsquo;ve probably
            already looked at &mdash; the things that show up on the bill,
            and the things that show up on the storefront. Prices follow each
            platform&rsquo;s published rate in {currency}.
          </p>
        </div>

        <div className="-mx-6 overflow-x-auto px-6 sm:mx-0 sm:px-0">
          <table className="w-full min-w-[760px] border-collapse text-sm">
            <caption className="sr-only">
              How Mark8ly compares with Shopify, BigCommerce, Wix and
              Squarespace on price, trial length, fees, design and data
              portability. Prices shown in {currency}.
            </caption>
            <thead>
              <tr className="border-b border-border">
                <th
                  scope="col"
                  className="py-4 pr-6 text-left align-bottom"
                >
                  <span className="eyebrow">Feature</span>
                </th>
                <th
                  scope="col"
                  className="px-4 py-4 text-left align-bottom"
                >
                  <span className="font-serif text-lg font-medium text-foreground">
                    Mark8ly
                  </span>
                </th>
                <th scope="col" className={competitorHeaderClass}>
                  Shopify
                </th>
                <th scope="col" className={competitorHeaderClass}>
                  BigCommerce
                </th>
                <th scope="col" className={competitorHeaderClass}>
                  Wix
                </th>
                <th scope="col" className={competitorHeaderClass}>
                  Squarespace
                </th>
              </tr>
            </thead>
            <tbody>
              {/* Starting price — geo-localized row, rendered specially. */}
              <tr className="border-b border-border-subtle">
                <th
                  scope="row"
                  className="py-5 pr-6 text-left align-top font-normal text-foreground-secondary"
                >
                  Starting price
                </th>
                <td className="px-4 py-5 align-top">
                  <span className="block border-l-2 border-moss-700 pl-3 font-medium text-foreground">
                    {mark8lyStarterMonthly !== null ? (
                      <>
                        <Money
                          amount={mark8lyStarterMonthly}
                          currency={currency}
                          showCents={false}
                        />
                        <span className="text-foreground-tertiary"> / mo</span>
                      </>
                    ) : (
                      "—"
                    )}
                  </span>
                </td>
                <td className={competitorCellClass}>
                  {competitorStartingPrice("shopify", currency)}
                </td>
                <td className={competitorCellClass}>
                  {competitorStartingPrice("bigcommerce", currency)}
                </td>
                <td className={competitorCellClass}>
                  {competitorStartingPrice("wix", currency)}
                </td>
                <td className={competitorCellClass}>
                  {competitorStartingPrice("squarespace", currency)}
                </td>
              </tr>

              {/* Feature rows — currency-agnostic. */}
              {COMPARISON_ROWS.map((row) => (
                <tr
                  key={row.label}
                  className="border-b border-border-subtle"
                >
                  <th
                    scope="row"
                    className="py-5 pr-6 text-left align-top font-normal text-foreground-secondary"
                  >
                    {row.label}
                  </th>
                  <td className="px-4 py-5 align-top">
                    <span className="block border-l-2 border-moss-700 pl-3 font-medium text-foreground">
                      {row.mark8ly}
                    </span>
                  </td>
                  <td className={competitorCellClass}>{row.shopify}</td>
                  <td className={competitorCellClass}>{row.bigcommerce}</td>
                  <td className={competitorCellClass}>{row.wix}</td>
                  <td className={competitorCellClass}>{row.squarespace}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>

        <p className="mt-8 max-w-3xl text-sm text-foreground-tertiary">
          Prices follow each platform&rsquo;s published rate for the cheapest
          paid plan in your country. Where a platform doesn&rsquo;t localize
          (BigCommerce globally, Squarespace outside developed markets), we
          show their USD rate and tag it so you know what you&rsquo;ll
          actually be charged. Payment processor fees &mdash; around 2&ndash;3%
          for cards, closer to 1% for UPI &mdash; apply on every platform.
          That&rsquo;s your bank, not us. The line above is what the platform
          itself adds on top.
        </p>
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
        <h2 className="mt-6 font-serif text-5xl font-medium leading-[1.02] tracking-[-0.025em] text-paper-50">
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
