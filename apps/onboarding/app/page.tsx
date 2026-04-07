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
      <section className="pt-32 pb-20 px-6">
        <div className="max-w-6xl mx-auto grid lg:grid-cols-[1.1fr_0.9fr] gap-16 items-center">
          <div>
            <div className="inline-flex items-center gap-2 px-4 py-2 rounded-full bg-sage-50 text-sage-700 text-sm font-medium border border-sage-200 mb-6">
              <span className="h-2 w-2 rounded-full bg-sage-500 animate-pulse" aria-hidden />
              12 months free, then one flat price
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
                Start Your Free Year
                <ArrowRight />
              </Link>
              <Link
                href="#features"
                className="px-6 py-4 rounded-xl text-base font-medium text-foreground-secondary hover:text-foreground hover:bg-warm-50 transition-all border border-warm-200"
              >
                See how it works
              </Link>
            </div>
            <p className="text-sm text-foreground-tertiary mb-8">
              No credit card required. Cancel anytime.
            </p>

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
      <section id="features" className="py-24 px-6 border-t border-warm-200">
        <div className="max-w-6xl mx-auto">
          <div className="max-w-2xl mb-14">
            <h2 className="font-serif text-3xl sm:text-4xl font-medium mb-4 text-foreground">
              Everything you need,
              <br />
              <span className="text-foreground-secondary">nothing you don&apos;t</span>
            </h2>
            <p className="text-lg text-foreground-secondary">
              We built the tools that matter and left out the complexity that
              doesn&apos;t.
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
      <section id="pricing" className="py-24 px-6 border-t border-warm-200">
        <div className="max-w-5xl mx-auto">
          <div className="text-center mb-12">
            <h2 className="font-serif text-3xl sm:text-4xl font-medium mb-4 text-foreground">
              Simple, honest pricing
            </h2>
            <p className="text-lg text-foreground-secondary">
              Start free. Stay free for 12 months. Then one flat price.
            </p>
          </div>

          <div className="grid gap-6 sm:grid-cols-2 max-w-3xl mx-auto">
            {pricingPlans.map((plan) => (
              <div
                key={plan.name}
                className={`rounded-2xl border bg-white p-6 sm:p-8 flex flex-col transition-all duration-200 hover:shadow-md ${
                  plan.featured
                    ? "border-foreground ring-1 ring-foreground/10 shadow-md"
                    : "border-warm-200 shadow-sm"
                }`}
              >
                {plan.featured && (
                  <div className="inline-flex self-start items-center px-3 py-1 rounded-full bg-foreground text-background text-xs font-medium mb-3">
                    Most popular
                  </div>
                )}
                <h3 className="font-serif text-lg font-medium text-foreground mb-1">
                  {plan.name}
                </h3>
                <p className="text-sm text-foreground-tertiary mb-3">
                  {plan.tagline}
                </p>
                <div className="mb-5">
                  <div className="flex items-baseline gap-1">
                    <span
                      className={`font-serif font-medium text-foreground ${
                        plan.featured ? "text-3xl" : "text-2xl"
                      }`}
                    >
                      {plan.price}
                    </span>
                    {plan.cycle && (
                      <span className="text-foreground-secondary text-sm">
                        {plan.cycle}
                      </span>
                    )}
                  </div>
                  {plan.subtext && (
                    <p className="text-sm text-foreground-secondary mt-1">
                      {plan.subtext}
                    </p>
                  )}
                </div>
                <ul className="space-y-2.5 mb-6 flex-1">
                  {plan.features.map((f) => (
                    <li key={f} className="flex items-start gap-2.5">
                      <svg
                        width="16"
                        height="16"
                        viewBox="0 0 24 24"
                        fill="none"
                        stroke="currentColor"
                        strokeWidth="3"
                        strokeLinecap="round"
                        strokeLinejoin="round"
                        className="text-sage-500 flex-shrink-0 mt-0.5"
                      >
                        <path d="M5 13l4 4L19 7" />
                      </svg>
                      <span className="text-sm text-foreground-secondary">
                        {f}
                      </span>
                    </li>
                  ))}
                </ul>
                <Link
                  href="/onboarding"
                  className="w-full py-3 rounded-lg text-base font-medium transition-all bg-primary text-primary-foreground hover:bg-primary-hover hover:shadow-sm text-center"
                >
                  {plan.cta}
                </Link>
              </div>
            ))}
          </div>
        </div>
      </section>

      {/* Testimonials */}
      <section id="testimonial" className="py-24 px-6 border-t border-warm-200">
        <div className="max-w-6xl mx-auto">
          <div className="text-center mb-14">
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
            {testimonials.map((t) => (
              <div
                key={t.name}
                className="p-6 rounded-xl bg-white border border-warm-200 shadow-sm"
              >
                <div className="flex gap-1 mb-4 text-amber-400">
                  {[1, 2, 3, 4, 5].map((i) => (
                    <span key={i}>★</span>
                  ))}
                </div>
                <blockquote className="text-foreground leading-relaxed mb-6">
                  &ldquo;{t.quote}&rdquo;
                </blockquote>
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
      <section className="py-24 px-6 border-t border-warm-200">
        <div className="max-w-4xl mx-auto">
          <div className="text-center">
            <h2 className="font-serif text-3xl sm:text-4xl font-medium mb-6 text-foreground">
              Three steps to your new store
            </h2>
            <p className="text-lg text-foreground-secondary mb-12">
              Tell us about yourself, add your products, and you&apos;re live.
              That&apos;s really all there is to it.
            </p>
          </div>

          <div className="grid sm:grid-cols-3 gap-6">
            {howItWorks.map((item) => (
              <div
                key={item.step}
                className="rounded-xl border border-warm-200 bg-white p-6"
              >
                <div className="w-9 h-9 rounded-full bg-terracotta-100 text-terracotta-700 flex items-center justify-center text-sm font-medium mb-4">
                  {item.step}
                </div>
                <h3 className="font-medium text-foreground mb-2">{item.title}</h3>
                <p className="text-foreground-secondary text-sm">{item.body}</p>
              </div>
            ))}
          </div>

          <div className="mt-12 text-center">
            <Link
              href="/onboarding"
              className="group bg-primary text-primary-foreground px-8 py-3.5 rounded-lg text-base font-medium hover:bg-primary-hover transition-colors inline-flex items-center gap-2"
            >
              Let&apos;s Get Started
              <ArrowRight />
            </Link>
          </div>
        </div>
      </section>

      {/* FAQ */}
      <section id="faq" className="py-24 px-6 border-t border-warm-200">
        <div className="max-w-2xl mx-auto">
          <div className="text-center mb-12">
            <h2 className="font-serif text-3xl sm:text-4xl font-medium text-foreground">
              Questions you might have
            </h2>
          </div>

          <div className="space-y-3">
            {faqs.map((faq, i) => (
              <div
                key={faq.question}
                className="rounded-xl bg-white border border-warm-200 overflow-hidden"
              >
                <button
                  type="button"
                  aria-expanded={openFaq === i}
                  onClick={() => setOpenFaq(openFaq === i ? null : i)}
                  className="w-full p-6 text-left flex items-center justify-between hover:bg-warm-50 transition-colors"
                >
                  <span className="font-medium text-foreground pr-4">{faq.question}</span>
                  <svg
                    width="20"
                    height="20"
                    viewBox="0 0 24 24"
                    fill="none"
                    stroke="currentColor"
                    strokeWidth="2"
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    className={`text-foreground-tertiary transition-transform duration-200 flex-shrink-0 ${openFaq === i ? "rotate-180" : ""}`}
                  >
                    <polyline points="6 9 12 15 18 9" />
                  </svg>
                </button>
                <div
                  className={`overflow-hidden transition-all duration-200 ${openFaq === i ? "max-h-96" : "max-h-0"}`}
                >
                  <div className="px-6 pb-6">
                    <p className="text-foreground-secondary leading-relaxed">
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
      <section className="py-24 px-6 border-t border-warm-200">
        <div className="max-w-2xl mx-auto text-center">
          <h2 className="font-serif text-3xl sm:text-4xl font-medium mb-6 text-foreground">
            Ready to open your doors?
          </h2>
          <p className="text-xl text-foreground-secondary mb-10">
            Your store is waiting. Start free today.
          </p>
          <Link
            href="/onboarding"
            className="group bg-primary text-primary-foreground px-10 py-4 rounded-lg text-lg font-medium hover:bg-primary-hover transition-colors inline-flex items-center gap-3"
          >
            Create Your Store — Free
            <ArrowRight />
          </Link>
          <p className="text-sm text-foreground-tertiary mt-4">
            12 months free, then one flat price. Cancel anytime.
          </p>
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
      <div className="rounded-2xl border border-warm-200 bg-white shadow-lg p-5 transform hover:scale-[1.02] transition-transform duration-300">
        {/* Browser bar */}
        <div className="flex items-center gap-2 mb-4 pb-4 border-b border-warm-100">
          <div className="flex gap-1.5">
            <div className="w-3 h-3 rounded-full bg-warm-200" />
            <div className="w-3 h-3 rounded-full bg-warm-200" />
            <div className="w-3 h-3 rounded-full bg-warm-200" />
          </div>
          <div className="flex-1 h-6 bg-warm-50 rounded-md mx-4" />
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
        <div className="flex items-center gap-0.5 text-amber-400 text-sm">
          {[1, 2, 3, 4, 5].map((i) => (
            <span key={i}>★</span>
          ))}
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
      "Beautiful themes you can customize to match your brand. No design skills needed.",
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

const pricingPlans = [
  {
    name: "Free Trial",
    tagline: "Get started risk-free",
    price: "$0",
    cycle: "",
    subtext: "for 12 months",
    featured: false,
    cta: "Start Free",
    features: [
      "Unlimited products",
      "Mobile-responsive storefront",
      "Payment processing",
      "Order management",
      "24/7 human support",
    ],
  },
  {
    name: "Professional",
    tagline: "Everything you need to grow",
    price: "$4.99",
    cycle: "/month",
    subtext: undefined,
    featured: true,
    cta: "Get Started",
    features: [
      "Sell as many products as you want",
      "Use your own domain name",
      "Looks great on phones",
      "Accept cards, UPI, and wallets",
      "Analytics dashboard",
      "No transaction fees from us",
    ],
  },
];

const testimonials = [
  {
    quote:
      "I spent months trying to figure out Shopify. With mark8ly, I had my store up in an afternoon. It just... works.",
    name: "Sarah Chen",
    role: "Founder",
    company: "BloomBox",
    initials: "SC",
  },
  {
    quote:
      "The onboarding was so smooth I thought I must be missing something. Nope—it really is that simple. My store was live the same day.",
    name: "Marcus Rivera",
    role: "Owner",
    company: "Craft & Co",
    initials: "MR",
  },
  {
    quote:
      "Finally, an e-commerce platform that doesn't make me feel stupid. Clean, fast, and the support team actually responds.",
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
