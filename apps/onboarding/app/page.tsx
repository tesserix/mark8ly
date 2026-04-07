"use client";

import Link from "next/link";
import { Button, Card, CardContent } from "@tesserix/web";

import { Header } from "@/components/marketing/Header";
import { Footer } from "@/components/marketing/Footer";

/**
 * Marketing landing page. Hero + features + CTA + footer.
 * Visually inspired by the legacy mark8ly_backup landing page, but rebuilt
 * lean on top of @tesserix/web rather than the old hand-rolled markup.
 *
 * Real CMS-driven content (testimonials, pricing tiers fetched from
 * platform-api) is deferred — see Phase F Tier 2 in the planning docs.
 */
export default function HomePage() {
  return (
    <div className="min-h-screen flex flex-col bg-white dark:bg-zinc-950 text-zinc-900 dark:text-zinc-50">
      <Header />

      <main className="flex-1">
        {/* Hero */}
        <section className="px-6 pt-24 pb-20">
          <div className="mx-auto max-w-4xl text-center">
            <div className="inline-flex items-center gap-2 px-4 py-1.5 rounded-full bg-emerald-50 dark:bg-emerald-950/40 text-emerald-700 dark:text-emerald-300 text-xs font-medium border border-emerald-200 dark:border-emerald-900 mb-8">
              <span className="h-1.5 w-1.5 rounded-full bg-emerald-500 animate-pulse" />
              12 months free, then one flat price
            </div>
            <h1 className="text-5xl sm:text-6xl font-semibold tracking-tight leading-[1.05] mb-6">
              Your online store,
              <br />
              <span className="text-zinc-500 dark:text-zinc-400">ready this afternoon</span>
            </h1>
            <p className="mx-auto max-w-2xl text-lg text-zinc-600 dark:text-zinc-400 mb-10">
              Set up your store in under an hour. No developer needed. Just you,
              your products, and customers ready to buy.
            </p>
            <div className="flex flex-col sm:flex-row gap-3 items-center justify-center">
              <Button asChild size="lg">
                <Link href="/onboarding">Start your free year</Link>
              </Button>
              <Button asChild size="lg" variant="outline">
                <Link href="/#features">See how it works</Link>
              </Button>
            </div>
            <p className="mt-4 text-xs text-zinc-500">
              No credit card required. Cancel anytime.
            </p>
          </div>
        </section>

        {/* Features */}
        <section
          id="features"
          className="px-6 py-24 border-t border-zinc-200 dark:border-zinc-800"
        >
          <div className="mx-auto max-w-6xl">
            <div className="max-w-2xl mb-14">
              <h2 className="text-3xl sm:text-4xl font-semibold tracking-tight mb-4">
                Everything you need,
                <br />
                <span className="text-zinc-500 dark:text-zinc-400">
                  nothing you don&apos;t
                </span>
              </h2>
              <p className="text-lg text-zinc-600 dark:text-zinc-400">
                We built the tools that matter and left out the complexity that
                doesn&apos;t.
              </p>
            </div>

            <div className="grid sm:grid-cols-2 lg:grid-cols-4 gap-6">
              {features.map((f) => (
                <Card key={f.title} className="h-full">
                  <CardContent className="p-6">
                    <div className="w-10 h-10 rounded-lg bg-zinc-100 dark:bg-zinc-800 flex items-center justify-center mb-4 text-xl">
                      {f.emoji}
                    </div>
                    <h3 className="font-semibold mb-2">{f.title}</h3>
                    <p className="text-sm text-zinc-600 dark:text-zinc-400 leading-relaxed">
                      {f.description}
                    </p>
                  </CardContent>
                </Card>
              ))}
            </div>
          </div>
        </section>

        {/* CTA */}
        <section className="px-6 py-24 border-t border-zinc-200 dark:border-zinc-800">
          <div className="mx-auto max-w-3xl text-center">
            <h2 className="text-3xl sm:text-4xl font-semibold tracking-tight mb-4">
              Ready to launch?
            </h2>
            <p className="text-lg text-zinc-600 dark:text-zinc-400 mb-8">
              Get your store live today. Free for 12 months — no credit card.
            </p>
            <Button asChild size="lg">
              <Link href="/onboarding">Get my store ready</Link>
            </Button>
          </div>
        </section>
      </main>

      <Footer />
    </div>
  );
}

const features = [
  {
    emoji: "🎨",
    title: "Make it yours",
    description:
      "Beautiful themes you can customize to match your brand. No design skills needed.",
  },
  {
    emoji: "💳",
    title: "Sell everywhere",
    description:
      "Accept payments from customers around the world in their preferred currency.",
  },
  {
    emoji: "📊",
    title: "Know your numbers",
    description:
      "Simple analytics that help you understand what's working and what's not.",
  },
  {
    emoji: "💬",
    title: "We've got your back",
    description:
      "Real humans ready to help when you need it. No chatbots, just friendly support.",
  },
];
