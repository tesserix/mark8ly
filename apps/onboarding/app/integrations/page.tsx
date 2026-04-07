"use client";

import {
  CreditCard,
  Truck,
  Smartphone,
  CheckCircle,
  ArrowRight,
  Zap,
  Clock,
  Shield,
} from "lucide-react";

import { Header } from "@/components/marketing/Header";
import { Footer } from "@/components/marketing/Footer";

const currentIntegrations = [
  {
    category: "Payments",
    description: "Accept payments from anywhere in India and the world.",
    items: [
      {
        name: "Razorpay",
        description:
          "UPI, cards, netbanking, wallets - all payment modes Indians love",
        features: [
          "UPI & QR payments",
          "All major cards",
          "EMI options",
          "Instant settlements",
        ],
      },
      {
        name: "Stripe",
        description: "For international customers paying with global cards",
        features: [
          "135+ currencies",
          "Global cards",
          "Apple Pay & Google Pay",
          "Fraud protection",
        ],
      },
    ],
  },
  {
    category: "Shipping",
    description: "Ship anywhere in India with real-time tracking.",
    items: [
      {
        name: "Shiprocket",
        description: "Access to 25+ courier partners through one integration",
        features: [
          "Auto courier selection",
          "Bulk shipping",
          "NDR management",
          "COD support",
        ],
      },
      {
        name: "Delhivery",
        description: "Reliable delivery across 18,000+ pin codes",
        features: [
          "Express delivery",
          "Surface shipping",
          "Warehousing",
          "Returns management",
        ],
      },
    ],
  },
];

const comingSoonIntegrations = [
  {
    name: "Instagram Shopping",
    description:
      "Tag products in your posts and stories. Let customers buy without leaving Instagram.",
    icon: Smartphone,
  },
  {
    name: "WhatsApp Commerce",
    description:
      "Share your catalog on WhatsApp. Accept orders and payments in chat.",
    icon: Smartphone,
  },
];

export default function IntegrationsPage() {
  return (
    <div className="min-h-screen bg-background">
      <Header />

      <section className="pt-32 pb-16 px-6">
        <div className="max-w-5xl mx-auto text-center">
          <div className="inline-flex items-center gap-2 rounded-full border border-warm-200 bg-white/80 px-4 py-2 text-sm font-medium text-foreground-secondary shadow-sm mb-6">
            <Zap className="w-4 h-4" />
            Built-in, not bolted on
          </div>
          <h1 className="font-serif text-4xl sm:text-5xl font-medium tracking-tight mb-6 text-foreground">
            Everything connects.
            <br />
            Nothing to configure.
          </h1>
          <p className="text-xl text-foreground-secondary max-w-2xl mx-auto">
            Payments and shipping work out of the box. No plugins to install,
            no API keys to hunt down, no developer needed.
          </p>
        </div>
      </section>

      <section className="pb-16 px-6">
        <div className="max-w-4xl mx-auto">
          <div className="grid gap-6 md:grid-cols-3">
            <div className="rounded-2xl border border-warm-200/90 bg-white/88 p-6 text-center shadow-sm">
              <Clock className="w-8 h-8 text-foreground-secondary mx-auto mb-3" />
              <p className="font-medium text-foreground">5-minute setup</p>
              <p className="text-sm text-foreground-secondary">Not 5 hours</p>
            </div>
            <div className="rounded-2xl border border-warm-200/90 bg-white/88 p-6 text-center shadow-sm">
              <Shield className="w-8 h-8 text-foreground-secondary mx-auto mb-3" />
              <p className="font-medium text-foreground">PCI compliant</p>
              <p className="text-sm text-foreground-secondary">Bank-grade security</p>
            </div>
            <div className="rounded-2xl border border-warm-200/90 bg-white/88 p-6 text-center shadow-sm">
              <CreditCard className="w-8 h-8 text-foreground-secondary mx-auto mb-3" />
              <p className="font-medium text-foreground">No extra fees</p>
              <p className="text-sm text-foreground-secondary">Just gateway charges</p>
            </div>
          </div>
        </div>
      </section>

      <section className="pb-20 px-6">
        <div className="max-w-4xl mx-auto">
          {currentIntegrations.map((category) => (
            <div key={category.category} className="mb-16">
              <div className="flex items-center gap-3 mb-3">
                {category.category === "Payments" ? (
                  <CreditCard className="w-6 h-6 text-primary" />
                ) : (
                  <Truck className="w-6 h-6 text-primary" />
                )}
                <h2 className="font-serif text-2xl font-medium text-foreground">
                  {category.category}
                </h2>
              </div>
              <p className="text-foreground-secondary mb-8">
                {category.description}
              </p>

              <div className="grid sm:grid-cols-2 gap-6">
                {category.items.map((item) => (
                  <div
                    key={item.name}
                    className="rounded-2xl border border-border bg-white/88 p-6 shadow-sm transition-[border-color,box-shadow,transform] hover:border-warm-300 hover:shadow-md"
                  >
                    <h3 className="text-xl font-semibold text-foreground mb-2">
                      {item.name}
                    </h3>
                    <p className="text-foreground-secondary mb-4">
                      {item.description}
                    </p>
                    <ul className="space-y-2">
                      {item.features.map((feature) => (
                        <li
                          key={feature}
                          className="flex items-center gap-2 text-sm text-foreground-secondary"
                        >
                          <CheckCircle className="w-4 h-4 text-sage-600 flex-shrink-0" />
                          {feature}
                        </li>
                      ))}
                    </ul>
                  </div>
                ))}
              </div>
            </div>
          ))}
        </div>
      </section>

      <section className="pb-20 px-6">
        <div className="max-w-4xl mx-auto">
          <div className="text-center mb-12">
            <h2 className="font-serif text-3xl font-medium text-foreground mb-4">
              Coming soon
            </h2>
            <p className="text-foreground-secondary">
              Sell where your customers already spend their time.
            </p>
          </div>

          <div className="grid sm:grid-cols-2 gap-6">
            {comingSoonIntegrations.map((item) => (
              <div
                key={item.name}
                className="rounded-2xl border border-warm-200 border-dashed bg-white/82 p-6 shadow-sm"
              >
                <div className="flex items-start gap-4">
                  <div className="w-12 h-12 rounded-xl bg-warm-100 flex items-center justify-center flex-shrink-0">
                    <item.icon className="w-6 h-6 text-foreground-secondary" />
                  </div>
                  <div>
                    <h3 className="text-lg font-semibold text-foreground mb-1">
                      {item.name}
                    </h3>
                    <p className="text-foreground-secondary text-sm">
                      {item.description}
                    </p>
                  </div>
                </div>
              </div>
            ))}
          </div>
        </div>
      </section>

      <section className="pb-20 px-6">
        <div className="max-w-4xl mx-auto">
          <div className="text-center rounded-[2rem] border border-border bg-white/90 p-10 shadow-[0_20px_60px_rgba(43,38,34,0.08)]">
            <h2 className="font-serif text-2xl font-medium text-foreground mb-4">
              Ready to start selling?
            </h2>
            <p className="text-foreground-secondary mb-6">
              Your first 6 months are free. All integrations included.
            </p>
            <a
              href="/onboarding"
              className="inline-flex items-center gap-2 rounded-full bg-primary px-6 py-3 font-medium text-primary-foreground transition-[background-color,box-shadow] hover:bg-primary-hover hover:shadow-lg"
            >
              Start your store
              <ArrowRight className="w-4 h-4" />
            </a>
          </div>
        </div>
      </section>

      <Footer />
    </div>
  );
}
