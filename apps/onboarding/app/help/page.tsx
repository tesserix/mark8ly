import type { Metadata } from "next";
import Link from "next/link";
import { FaqAccordion, type FaqItem } from "@repo/ui/faq-accordion";

import { MarketingPage, PageHero } from "@/components/marketing/primitives";

/* ============================================================
   Help — grouped FAQ covering the questions a merchant evaluating
   or newly onboarded onto Mark8ly actually asks. Every answer is
   grounded in what the product does today (see the category
   comments below for the source); nothing here promises a
   feature that hasn't shipped.

   Rendered as the same asymmetric eyebrow-plus-content sections
   used on /about, with a FaqAccordion per category so the page
   reads as one continuous, scannable list rather than a wall of
   text. FAQPage JSON-LD is emitted from the flattened category
   list, following the pattern already used by SeoLanding for the
   comparison landing pages (see components/marketing/SeoLanding.tsx).
   ============================================================ */

export const metadata: Metadata = {
  title: "Help & FAQ",
  description:
    "Answers to the questions merchants actually ask about Mark8ly: the 90-day trial, pricing and fees, custom domains, payments, shipping, inventory, and taking your data with you.",
  alternates: { canonical: "/help" },
  openGraph: {
    title: "Help & FAQ · Mark8ly",
    description:
      "Getting started, the trial, pricing, payments, shipping, and data ownership — answered plainly, no ticket queue required.",
    url: "/help",
  },
};

interface FaqCategory {
  eyebrow: string;
  heading: string;
  items: ReadonlyArray<FaqItem>;
}

// Source for each category, so future edits stay grounded rather than
// drifting into marketing copy. A file existing under internal/ is NOT
// enough on its own — check what's actually reachable by a merchant
// today (supported-country arrays, plangate feature gates, whether a
// component is ever rendered) before repeating a claim from here:
//  - Trial/pricing: components/marketing/Pricing.tsx, app/page.tsx FAQ
//  - Payments: services/marketplace-api/internal/payment (stripe.go,
//    razorpay.go) and migrations/000121_retire_untested_paypal.up.sql,
//    which removed 'paypal' from every country's payment_providers array
//    and deactivated existing configs — PayPal is NOT offered today even
//    though paypal.go still exists.
//  - Shipping: services/marketplace-api/internal/shipping (delhivery.go,
//    ninjavan.go, shipengine.go), gated per-country by
//    supported_countries.shipping_carriers (migrations/000090).
//  - Inventory/warehouses: PR #538 (per-warehouse stock for
//    multi-variant products); migrations/000122 and 000123 confirm every
//    store has a real warehouse to hold that stock.
//  - Otto (services/otto) is NOT included here: its storefront wrapper,
//    apps/storefront/components/OttoSupportChat.tsx, is never imported
//    by any page, and its tenantId is hardcoded to "mark8ly" rather than
//    the merchant's own store — so it isn't something a merchant gets
//    today.
const CATEGORIES: ReadonlyArray<FaqCategory> = [
  {
    eyebrow: "Getting started",
    heading: "Opening a store",
    items: [
      {
        question: "I'm not technical. Can I actually do this myself?",
        answer:
          "Yes — that's the point of Mark8ly. If you can write an email, you can set up a store: add products, connect a payment processor, and pick a domain, all from the admin. Most merchants have a working store open by the end of the afternoon.",
      },
      {
        question: "Do I need a credit card to start?",
        answer:
          "No. The 90-day trial starts with just an email and doesn't ask for a card. You'll only be asked to choose a plan once the trial is ending, or earlier if you decide to upgrade.",
      },
    ],
  },
  {
    eyebrow: "Trial & billing",
    heading: "The 90 days, and after",
    items: [
      {
        question: "What's included in the free trial?",
        answer:
          "The full product — unlimited products and orders, your own domain, payments, shipping, and the admin, exactly as a paying merchant sees it. Nothing is watermarked or feature-limited during the 90 days.",
      },
      {
        question: "What happens when the trial ends?",
        answer:
          "You choose one of three plans — Starter, Studio, or Pro — starting at $15 a month billed yearly, or $19 a month billed monthly. There's no auto-upgrade and no surprise charge: if you haven't picked a plan, your store simply won't take new orders until you do.",
      },
      {
        question: "Can I cancel or change plans later?",
        answer:
          "Any time. Upgrades take effect immediately and prorate; downgrades take effect at the end of the current billing period. Cancelling doesn't lock your data — see data ownership below.",
      },
    ],
  },
  {
    eyebrow: "Pricing & fees",
    heading: "What Mark8ly actually charges",
    items: [
      {
        question: "Does Mark8ly take a cut of my sales?",
        answer:
          "No. We don't add a platform transaction fee on any plan. You pay only your payment gateway's standard rate — roughly 2% for UPI and 2–3% for cards — and that's the entire cost of taking a payment.",
      },
      {
        question: "What's the difference between Starter, Studio, and Pro?",
        answer:
          "All three include unlimited products and orders. Starter covers up to 2 stores. Studio adds up to 5 stores, custom CSS and fonts, a read-only API, and a 12-month audit log. Pro adds up to 10 stores, a full read/write API, SSO, and priority support.",
      },
    ],
  },
  {
    eyebrow: "Domains",
    heading: "Your own address",
    items: [
      {
        question: "Can I use my own domain?",
        answer:
          "Yes, on every plan — it's included, not an add-on. Connect a domain you already own from Settings → Domains, or buy one during onboarding. Customers land on your brand, never a Mark8ly subdomain.",
      },
    ],
  },
  {
    eyebrow: "Payments",
    heading: "Getting paid, including UPI",
    items: [
      {
        question: "Which payment processors do you support?",
        answer:
          "Stripe for cards internationally, and Razorpay for merchants selling in India — which covers UPI, wallets, and local cards, not just international cards. You connect one from Settings → Payments; customers never leave your storefront to pay.",
      },
      {
        question: "Can I accept UPI payments if I'm selling in India?",
        answer:
          "Yes. Razorpay is a first-class processor in Mark8ly, so UPI, popular wallets, and net banking are available alongside cards for Indian customers, at Razorpay's standard rates — Mark8ly adds nothing on top.",
      },
    ],
  },
  {
    eyebrow: "Products & inventory",
    heading: "Catalogue and stock",
    items: [
      {
        question: "Is there a limit on how many products I can list?",
        answer:
          "No. Every plan includes unlimited products, variants, and orders. What differs between plans is how many separate storefronts you can run, not how big any one of them can grow.",
      },
      {
        question: "Can I track stock across more than one warehouse?",
        answer:
          "Yes. Stock is tracked per warehouse, including for products with multiple variants — so a product with several sizes or colours can show accurate, location-specific availability rather than one blended number.",
      },
    ],
  },
  {
    eyebrow: "Shipping",
    heading: "Getting orders to customers",
    items: [
      {
        question: "What shipping carriers can I use?",
        answer:
          "Delhivery for domestic shipping within India, NinjaVan for Southeast Asia, and ShipEngine for broader coverage — which connects carriers like USPS, UPS, FedEx, and DHL through a single integration. Live rates are calculated at checkout based on the carriers you've connected.",
      },
    ],
  },
  {
    eyebrow: "Data ownership",
    heading: "Nothing is locked in",
    items: [
      {
        question: "What happens to my data if I decide to leave?",
        answer:
          "It's yours. Export your products, customers, and orders in one click, at any time, whether or not you're cancelling. We'd rather earn a renewal than make leaving painful.",
      },
    ],
  },
];

const faqJsonLd = {
  "@context": "https://schema.org",
  "@type": "FAQPage",
  mainEntity: CATEGORIES.flatMap((category) =>
    category.items.map((item) => ({
      "@type": "Question",
      name: item.question,
      acceptedAnswer: { "@type": "Answer", text: item.answer },
    })),
  ),
};

export default function HelpPage() {
  return (
    <MarketingPage>
      <script
        type="application/ld+json"
        dangerouslySetInnerHTML={{ __html: JSON.stringify(faqJsonLd) }}
      />

      <PageHero
        eyebrow="Help"
        title={<>Answers, plainly.</>}
        lede="The questions merchants actually ask, grouped by topic. If yours isn't here, a real person reads every message we get."
      />

      {CATEGORIES.map((category) => (
        <section
          key={category.heading}
          className="border-t border-border-subtle py-16 sm:py-20"
        >
          <div className="mx-auto grid max-w-6xl gap-8 px-6 lg:grid-cols-[1fr_2fr] lg:gap-16">
            <div className="lg:sticky lg:top-32 lg:self-start">
              <p className="eyebrow mb-5">{category.eyebrow}</p>
              <h2 className="font-serif text-2xl font-medium leading-[1.1] tracking-[-0.015em] text-foreground">
                {category.heading}
              </h2>
            </div>
            <FaqAccordion
              items={[...category.items]}
              className="border-t border-border-subtle"
            />
          </div>
        </section>
      ))}

      <section className="border-t border-border-subtle py-20 sm:py-24">
        <div className="mx-auto max-w-6xl px-6">
          <p className="max-w-xl text-lg leading-[1.55] text-foreground-secondary">
            Still stuck, or asking something we haven&rsquo;t covered? Email
            us — we&rsquo;re a small team and we read every message
            ourselves.
          </p>
          <div className="mt-10 flex flex-wrap items-center gap-x-8 gap-y-4">
            <Link
              href="/onboarding"
              className="inline-flex h-12 items-center rounded-md bg-primary px-6 text-base font-medium text-primary-foreground hover:bg-primary-hover"
            >
              Open your store
            </Link>
            <Link href="/contact" className="btn-ghost">
              Contact us
            </Link>
          </div>
        </div>
      </section>
    </MarketingPage>
  );
}
