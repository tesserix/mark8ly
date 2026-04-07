"use client";

import Link from "next/link";
import { Heart, Zap, Users, Shield, ArrowRight } from "lucide-react";

import { Header } from "@/components/marketing/Header";
import { Footer } from "@/components/marketing/Footer";

export default function AboutPage() {
  return (
    <div className="min-h-screen bg-background">
      <Header />

      {/* Hero Section */}
      <section className="pt-32 pb-16 px-6">
        <div className="max-w-5xl mx-auto text-center">
          <div className="inline-flex items-center gap-2 rounded-full border border-warm-200 bg-white/80 px-4 py-2 text-sm font-medium text-foreground-secondary shadow-sm">
            <Heart className="w-4 h-4 text-terracotta-600" />
            Built for small business ambition
          </div>
          <h1 className="font-serif text-4xl sm:text-5xl font-medium tracking-tight mb-6 text-foreground">
            We believe every small business deserves a beautiful online store
          </h1>
          <p className="mx-auto max-w-3xl text-xl text-foreground-secondary leading-relaxed">
            mark8ly makes it simple for creators, makers, and small business
            owners to sell online—without the technical headaches or hidden
            fees.
          </p>
          <div className="mt-10 grid gap-4 sm:grid-cols-3 text-left">
            {[
              ["Made for", "Creators, makers & founders"],
              ["Promise", "No transaction fees, no needless complexity"],
              ["Approach", "Editorial design, human support, fast setup"],
            ].map(([label, value]) => (
              <div
                key={label}
                className="rounded-2xl border border-warm-200/90 bg-white/80 p-5 shadow-sm"
              >
                <p className="text-xs uppercase tracking-[0.14em] text-foreground-tertiary">
                  {label}
                </p>
                <p className="mt-2 text-sm font-medium text-foreground">
                  {value}
                </p>
              </div>
            ))}
          </div>
        </div>
      </section>

      {/* Our Story */}
      <section className="py-16 px-6 border-t border-warm-200">
        <div className="max-w-3xl mx-auto">
          <h2 className="font-serif text-3xl font-medium text-foreground mb-6">
            How We Started
          </h2>
          <div className="prose prose-lg text-foreground-secondary">
            <p className="leading-relaxed mb-4">
              We built mark8ly because we saw too many talented people
              struggling with the same problem: they had amazing products but
              couldn&apos;t afford the expensive platforms, transaction fees,
              and developers needed to sell online.
            </p>
            <p className="leading-relaxed">
              We knew there had to be a better way. So we created a platform
              that&apos;s powerful enough for growth, simple enough to set up
              in an afternoon, and fair enough that you keep more of what you
              earn.
            </p>
          </div>
        </div>
      </section>

      {/* Mission */}
      <section className="py-16 px-6 bg-warm-50/75">
        <div className="max-w-3xl mx-auto">
          <h2 className="font-serif text-3xl font-medium text-foreground mb-6">
            Our Mission
          </h2>
          <p className="text-lg text-foreground-secondary leading-relaxed mb-8">
            We&apos;re here to level the playing field. Whether you&apos;re a
            baker selling custom cakes, an artist selling prints, or a maker
            selling handcrafted goods, you deserve the same quality tools that
            big brands use—without the enterprise price tag.
          </p>
          <div className="grid sm:grid-cols-2 gap-6">
            {[
              {
                icon: Heart,
                title: "Simplicity over complexity",
                desc: "You shouldn't need a developer to launch your store",
              },
              {
                icon: Zap,
                title: "Fairness over fees",
                desc: "No extra platform transaction fees from mark8ly means you keep more of what you earn",
              },
              {
                icon: Users,
                title: "Support over automation",
                desc: "Real humans, available 24/7, whenever you need help",
              },
              {
                icon: Shield,
                title: "Quality over shortcuts",
                desc: "Modern technology that's fast, secure, and reliable",
              },
            ].map((item) => (
              <div
                key={item.title}
                className="flex gap-4 rounded-2xl border border-warm-200/90 bg-white/88 p-5 shadow-sm"
              >
                <div className="w-10 h-10 rounded-lg bg-sage-100 flex items-center justify-center flex-shrink-0">
                  <item.icon className="w-5 h-5 text-sage-600" />
                </div>
                <div>
                  <h3 className="font-medium text-foreground mb-1">{item.title}</h3>
                  <p className="text-sm text-foreground-secondary">{item.desc}</p>
                </div>
              </div>
            ))}
          </div>
        </div>
      </section>

      {/* What We Offer */}
      <section className="py-16 px-6 border-t border-warm-200">
        <div className="max-w-3xl mx-auto">
          <h2 className="font-serif text-3xl font-medium text-foreground mb-8">
            Built for Your Success
          </h2>
          <div className="grid sm:grid-cols-2 gap-8">
            {[
              {
                title: "No Transaction Fees",
                desc: "We don&apos;t add a platform transaction fee on top of your orders. You only pay your payment gateway&apos;s standard processing charges.",
              },
              {
                title: "Start Free",
                desc: "Start selling today, free for 6 months. After that, one flat monthly fee with no confusing plan ladder.",
              },
              {
                title: "No Developer Needed",
                desc: "Our platform is designed for real people, not engineers. If you can use email, you can build your store.",
              },
              {
                title: "24/7 Human Support",
                desc: "Got a question at midnight? We're here. Real people, real help, whenever you need it.",
              },
              {
                title: "Global Reach",
                desc: "Based in India, built for the world. Sell to customers anywhere, accept payments globally.",
              },
              {
                title: "Enterprise-Grade Security",
                desc: "GDPR and PCI DSS compliant. Your data and your customers' data are protected.",
              },
            ].map((item) => (
              <div key={item.title}>
                <h3 className="font-medium text-foreground mb-2">{item.title}</h3>
                <p className="text-foreground-secondary">{item.desc}</p>
              </div>
            ))}
          </div>
        </div>
      </section>

      {/* Team Philosophy */}
      <section className="py-16 px-6 bg-warm-50">
        <div className="max-w-3xl mx-auto">
          <h2 className="font-serif text-3xl font-medium text-foreground mb-6">
            How We Work
          </h2>
          <div className="text-lg text-foreground-secondary space-y-4">
            <p className="leading-relaxed">
              We&apos;re a small team that believes technology should serve
              people, not the other way around. We don&apos;t chase trends—we
              focus on building tools that actually help you grow your
              business.
            </p>
            <p className="leading-relaxed">
              Every feature we build starts with a simple question: &ldquo;Will
              this help our customers sell more and stress less?&rdquo; If the
              answer isn&apos;t a clear yes, we don&apos;t build it.
            </p>
            <p className="leading-relaxed">
              We&apos;re not trying to be the biggest platform. We&apos;re
              trying to be the best partner for small businesses who want to
              thrive online.
            </p>
          </div>
        </div>
      </section>

      {/* CTA */}
      <section className="py-20 px-6 border-t border-warm-200">
        <div className="max-w-2xl mx-auto text-center">
          <h2 className="font-serif text-3xl font-medium text-foreground mb-4">
            Ready to Start Selling?
          </h2>
          <p className="text-foreground-secondary mb-8">
            No credit card required. No extra platform transaction fees. No catch.
          </p>
          <Link
            href="/onboarding"
            className="inline-flex items-center gap-2 rounded-full bg-primary px-8 py-4 font-medium text-primary-foreground shadow-[0_14px_28px_rgba(31,30,28,0.18)] transition-[background-color,box-shadow] hover:bg-primary-hover hover:shadow-[0_18px_34px_rgba(31,30,28,0.22)]"
          >
            Launch Your Store Free
            <ArrowRight className="w-5 h-5" />
          </Link>
        </div>
      </section>

      <Footer />
    </div>
  );
}
