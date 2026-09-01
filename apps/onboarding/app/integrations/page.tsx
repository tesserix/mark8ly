import type { Metadata } from "next";
import Link from "next/link";

import { MarketingPage, PageHero } from "@/components/marketing/primitives";

/* ============================================================
   Integrations — an honest list of what Mark8ly connects to
   today, grouped by function. Every entry below is confirmed
   reachable by a merchant right now — not just present as code —
   see the comment above each group for exactly how that was
   checked. Nothing here is aspirational — if something is
   genuinely on the roadmap it belongs in a labelled "in progress"
   group, not folded in as if it already works.

   PayPal is deliberately absent even though
   internal/payment/paypal.go exists: migrations/
   000121_retire_untested_paypal.up.sql removed 'paypal' from
   every country's supported_countries.payment_providers and
   deactivated any existing configs, specifically because the
   gateway had never been run against a real PayPal account. Its
   code being present is not evidence it's offered — don't infer
   availability from an internal/ file existing without checking
   what's actually enabled for merchants (supported-country
   arrays, plangate feature gates, whether a component is ever
   rendered).
   ============================================================ */

export const metadata: Metadata = {
  title: "Integrations",
  description:
    "What Mark8ly actually connects to: Stripe and Razorpay for payments, Delhivery, NinjaVan, and ShipEngine for shipping, TaxJar for tax, and SendGrid or Resend for email.",
  alternates: { canonical: "/integrations" },
  openGraph: {
    title: "Integrations · Mark8ly",
    description:
      "A small, considered set of integrations — payments, shipping, tax, and email — each one honestly described, nothing implied that isn't live.",
    url: "/integrations",
  },
};

interface Integration {
  name: string;
  description: string;
}

interface IntegrationGroup {
  eyebrow: string;
  heading: string;
  intro: string;
  items: ReadonlyArray<Integration>;
}

// services/marketplace-api/internal/payment (stripe.go, razorpay.go) —
// cross-checked against supported_countries.payment_providers as seeded
// by migrations/000090 and left standing after 000100 and 000121. PayPal
// (paypal.go) is intentionally excluded: see the file-level comment above.
const PAYMENTS: IntegrationGroup = {
  eyebrow: "Payments",
  heading: "Getting paid",
  intro:
    "Connect one processor per store from Settings → Payments. Mark8ly never touches the money or adds a fee on top — you pay only the processor's own rate.",
  items: [
    {
      name: "Stripe",
      description:
        "Cards and wallets for merchants selling internationally. The default choice outside India.",
    },
    {
      name: "Razorpay",
      description:
        "Cards, UPI, wallets, and net banking for merchants selling in India — a full local payment stack, not just a card gateway.",
    },
  ],
};

// services/marketplace-api/internal/shipping (delhivery.go, ninjavan.go,
// shipengine.go), cross-checked against
// supported_countries.shipping_carriers (migrations/000090; untouched by
// any later migration) — Delhivery for India, NinjaVan for the Southeast
// Asian countries in the seed, ShipEngine everywhere else in it.
const SHIPPING: IntegrationGroup = {
  eyebrow: "Shipping",
  heading: "Getting orders out the door",
  intro:
    "Connected carriers appear as live rates at checkout and generate tracking numbers automatically once you fulfil an order.",
  items: [
    {
      name: "Delhivery",
      description:
        "Domestic carrier for shipments within India, including rate calculation and label generation.",
    },
    {
      name: "NinjaVan",
      description: "Regional carrier for shipments within Southeast Asia.",
    },
    {
      name: "ShipEngine",
      description:
        "A carrier aggregator that brings USPS, UPS, FedEx, DHL, and others in through a single connection — useful for merchants shipping outside India and Southeast Asia.",
    },
  ],
};

// services/marketplace-api/internal/tax (taxjar.go), cross-checked
// against supported_countries.tax_strategy (migrations/000090) — 'taxjar'
// only for the US row; India runs its own GST calculator, everyone else a
// flat rate, so TaxJar is not claimed for those.
const TAX: IntegrationGroup = {
  eyebrow: "Tax",
  heading: "Sales tax, calculated for you",
  intro:
    "Tax is calculated automatically at checkout based on where you and your customer are.",
  items: [
    {
      name: "TaxJar",
      description:
        "Automatic US sales tax calculation by jurisdiction. India uses a built-in calculator instead, since GST doesn't need a third party.",
    },
  ],
};

// services/marketplace-api/internal/email (sendgrid.go, resend.go,
// fallback.go) — providers.go wires both into NewFromConfig's
// FallbackSender chain unconditionally, not behind a plan or country
// gate.
const EMAIL: IntegrationGroup = {
  eyebrow: "Email",
  heading: "Order and account email",
  intro:
    "Transactional email — order confirmations, shipping updates, password resets — and campaign email both run through the same connected provider.",
  items: [
    {
      name: "SendGrid",
      description:
        "The default provider for transactional and campaign email.",
    },
    {
      name: "Resend",
      description:
        "An alternative provider; Mark8ly automatically falls back to it if the primary provider has an outage, so a delivery failure doesn't mean a lost order confirmation.",
    },
  ],
};

// internal/plangate/matrix.go's FeatureReadAPI / FeatureFullAPI, wired to
// real handlers in internal/handlers/admin/apikeys_handler.go (Studio gets
// read-only key scopes, Pro gets full read/write). Pricing.tsx additionally
// advertises "webhooks" (Studio) and "custom code injection" (Pro) — both
// left off here because they don't check out: no merchant-facing outbound
// webhook subscription exists anywhere in the codebase (internal/outbox is
// an internal event-bus pattern, not something a merchant configures), and
// matrix.go's own comment on FeatureCustomCodeInjection calls it "reserved
// for the future... no handler accepts arbitrary <script> payloads today."
const DEVELOPER: IntegrationGroup = {
  eyebrow: "Build on Mark8ly",
  heading: "For everything else",
  intro:
    "If the integration you need isn't on this list yet, you can often build it yourself.",
  items: [
    {
      name: "REST API",
      description:
        "Read-only API access on Studio; full read/write access on Pro — enough to sync orders and inventory with tools we don't connect to directly.",
    },
  ],
};

const GROUPS: ReadonlyArray<IntegrationGroup> = [
  PAYMENTS,
  SHIPPING,
  TAX,
  EMAIL,
  DEVELOPER,
];

export default function IntegrationsPage() {
  return (
    <MarketingPage>
      <PageHero
        eyebrow="Integrations"
        title={<>A small, considered set.</>}
        lede="We pick integrations the way an editor picks recommended tools — only the ones that earn their place. Here's exactly what's live today."
      />

      {GROUPS.map((group) => (
        <section
          key={group.heading}
          className="border-t border-border-subtle py-16 sm:py-20"
        >
          <div className="mx-auto grid max-w-6xl gap-8 px-6 lg:grid-cols-[1fr_2fr] lg:gap-16">
            <div className="lg:sticky lg:top-32 lg:self-start">
              <p className="eyebrow mb-5">{group.eyebrow}</p>
              <h2 className="font-serif text-2xl font-medium leading-[1.1] tracking-[-0.015em] text-foreground">
                {group.heading}
              </h2>
              <p className="mt-4 max-w-sm text-foreground-secondary leading-relaxed">
                {group.intro}
              </p>
            </div>
            <ul className="space-y-8 border-t border-border-subtle pt-8">
              {group.items.map((item) => (
                <li
                  key={item.name}
                  className="border-t border-border-subtle pt-8 first:border-t-0 first:pt-0"
                >
                  <p className="font-serif text-xl text-foreground">
                    {item.name}
                  </p>
                  <p className="mt-2 max-w-xl text-foreground-secondary leading-relaxed">
                    {item.description}
                  </p>
                </li>
              ))}
            </ul>
          </div>
        </section>
      ))}

      <section className="border-t border-border-subtle py-20 sm:py-24">
        <div className="mx-auto max-w-6xl px-6">
          <p className="max-w-xl text-lg leading-[1.55] text-foreground-secondary">
            Using something that isn&rsquo;t here? Tell us — partnerships and
            provider requests both come to a real inbox.
          </p>
          <div className="mt-10 flex flex-wrap items-center gap-x-8 gap-y-4">
            <Link
              href="/onboarding"
              className="inline-flex h-12 items-center rounded-md bg-primary px-6 text-base font-medium text-primary-foreground hover:bg-primary-hover"
            >
              Open your store
            </Link>
            <Link href="/contact" className="btn-ghost">
              Suggest one
            </Link>
          </div>
        </div>
      </section>
    </MarketingPage>
  );
}
