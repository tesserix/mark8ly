"use client";

import {
  PenLine,
  Mail,
  ArrowRight,
  Sparkles,
  TrendingUp,
  Users,
  Lightbulb,
  Heart,
} from "lucide-react";

import { Header } from "@/components/marketing/Header";
import { Footer } from "@/components/marketing/Footer";

const upcomingTopics = [
  {
    icon: TrendingUp,
    title: "Growing your store",
    description: "Practical tips from merchants who started where you are",
  },
  {
    icon: Users,
    title: "Customer stories",
    description: "Real businesses, real journeys, real lessons",
  },
  {
    icon: Lightbulb,
    title: "Product updates",
    description: "New features and how to use them",
  },
  {
    icon: Heart,
    title: "Behind the scenes",
    description: "How we build mark8ly and why",
  },
];

export default function BlogPage() {
  return (
    <div className="min-h-screen bg-background">
      <Header />

      <section className="pt-32 pb-16 px-6">
        <div className="max-w-5xl mx-auto text-center">
          <div className="inline-flex items-center gap-2 rounded-full border border-warm-200 bg-white/80 px-4 py-2 text-sm font-medium text-foreground-secondary shadow-sm mb-6">
            <Sparkles className="w-4 h-4" />
            Coming soon
          </div>
          <h1 className="font-serif text-4xl sm:text-5xl font-medium tracking-tight mb-6 text-foreground">
            The mark8ly Blog
          </h1>
          <p className="text-xl text-foreground-secondary max-w-2xl mx-auto mb-8">
            We are working on something. Stories, tips, and lessons from the
            world of small business e-commerce. No fluff, no jargon.
          </p>
          <div className="mx-auto mt-10 grid max-w-4xl gap-4 sm:grid-cols-3 text-left">
            {[
              ["Tone", "Practical, candid, grounded in merchant reality"],
              ["Cadence", "Only useful updates, never content for content’s sake"],
              ["Focus", "Growth, operations, customer stories, product craft"],
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

      <section className="pb-16 px-6">
        <div className="max-w-3xl mx-auto">
          <h2 className="font-serif text-2xl font-medium text-foreground mb-8 text-center">
            What you can expect
          </h2>
          <div className="grid sm:grid-cols-2 gap-6">
            {upcomingTopics.map((topic) => (
              <div
                key={topic.title}
                className="rounded-2xl border border-warm-200/90 bg-white/88 p-6 shadow-sm"
              >
                <div className="w-10 h-10 rounded-lg bg-warm-100 flex items-center justify-center mb-4">
                  <topic.icon className="w-5 h-5 text-foreground-secondary" />
                </div>
                <h3 className="font-semibold text-foreground mb-2">{topic.title}</h3>
                <p className="text-sm text-foreground-secondary">{topic.description}</p>
              </div>
            ))}
          </div>
        </div>
      </section>

      <section className="pb-20 px-6">
        <div className="max-w-3xl mx-auto">
          <div className="rounded-[2rem] border border-warm-200/90 bg-white/90 p-10 text-center shadow-[0_20px_60px_rgba(43,38,34,0.08)]">
            <div className="w-16 h-16 rounded-2xl bg-warm-100 flex items-center justify-center mx-auto mb-6">
              <Mail className="w-8 h-8 text-foreground-secondary" />
            </div>
            <h2 className="font-serif text-2xl font-medium text-foreground mb-3">
              Get notified when we launch
            </h2>
            <p className="text-foreground-secondary mb-8 max-w-md mx-auto">
              Join our newsletter. We will send you our best posts, no more
              than twice a month. Unsubscribe anytime.
            </p>

            <form className="flex flex-col sm:flex-row gap-3 max-w-md mx-auto">
              <input
                type="email"
                placeholder="you@example.com"
                className="flex-1 px-4 py-3 rounded-xl border border-border bg-background text-foreground placeholder:text-foreground-tertiary focus:outline-none focus:ring-2 focus:ring-primary/20 focus:border-primary"
              />
              <button
                type="submit"
                className="inline-flex items-center justify-center gap-2 rounded-full bg-primary px-6 py-3 font-medium text-primary-foreground transition-[background-color,box-shadow] hover:bg-primary-hover hover:shadow-lg"
              >
                Notify me
                <ArrowRight className="w-4 h-4" />
              </button>
            </form>

            <p className="text-xs text-foreground-tertiary mt-4">
              No spam, ever. We hate it as much as you do.
            </p>
          </div>
        </div>
      </section>

      <section className="pb-20 px-6">
        <div className="max-w-3xl mx-auto text-center">
          <h2 className="font-serif text-xl font-medium text-foreground mb-6">
            In the meantime
          </h2>
          <div className="flex flex-wrap justify-center gap-4">
            <a
              href="/guides"
              className="inline-flex items-center gap-2 rounded-full border border-border bg-white/80 px-5 py-2.5 text-foreground-secondary hover:text-foreground hover:border-warm-300 transition-colors"
            >
              <PenLine className="w-4 h-4" />
              Read our guides
            </a>
            <a
              href="/help"
              className="inline-flex items-center gap-2 rounded-full border border-border bg-white/80 px-5 py-2.5 text-foreground-secondary hover:text-foreground hover:border-warm-300 transition-colors"
            >
              Browse help center
            </a>
          </div>
        </div>
      </section>

      <Footer />
    </div>
  );
}
