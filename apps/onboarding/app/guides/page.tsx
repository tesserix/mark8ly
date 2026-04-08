"use client";

import {
  BookOpen,
  Rocket,
  CheckSquare,
  ShoppingBag,
  Megaphone,
  ArrowRight,
  Clock,
  Star,
} from "lucide-react";

import { Header } from "@/components/marketing/Header";
import { Footer } from "@/components/marketing/Footer";

const guides = [
  {
    icon: Rocket,
    title: "Getting Started",
    description: "From zero to your first store in 10 minutes",
    duration: "10 min read",
    featured: true,
    steps: [
      "Create your account (2 min)",
      "Add your first products (3 min)",
      "Set up payments (2 min)",
      "Configure shipping (2 min)",
      "Go live (1 min)",
    ],
  },
  {
    icon: CheckSquare,
    title: "Pre-Launch Checklist",
    description: "Everything to check before you open for business",
    duration: "5 min read",
    featured: false,
  },
  {
    icon: ShoppingBag,
    title: "Your First Sale",
    description: "A 4-week plan to get your first customer",
    duration: "8 min read",
    featured: false,
  },
  {
    icon: Megaphone,
    title: "Marketing Basics",
    description: "Simple ways to get the word out",
    duration: "12 min read",
    featured: false,
  },
];

export default function GuidesPage() {
  const featuredGuide = guides[0];
  const remainingGuides = guides.slice(1);

  if (!featuredGuide) return null;

  return (
    <div className="min-h-screen bg-background">
      <Header />

      <section className="pt-32 pb-12 px-6">
        <div className="max-w-5xl mx-auto text-center">
          <div className="inline-flex items-center gap-2 rounded-full border border-warm-200 bg-white/80 px-4 py-2 text-sm font-medium text-foreground-secondary shadow-sm">
            <BookOpen className="w-4 h-4 text-terracotta-600" />
            Short, practical playbooks
          </div>
          <h1 className="font-serif text-4xl sm:text-5xl font-medium tracking-tight mb-6 text-foreground">
            Guides
          </h1>
          <p className="text-xl text-foreground-secondary max-w-2xl mx-auto">
            Step-by-step instructions for building and growing your online
            store. No experience required.
          </p>
        </div>
      </section>

      {/* Featured */}
      <section className="pb-12 px-6">
        <div className="max-w-4xl mx-auto">
          <div className="rounded-[2rem] border border-primary/20 bg-[linear-gradient(180deg,rgba(197,116,92,0.08),rgba(255,255,255,0.9))] p-8 shadow-[0_20px_60px_rgba(43,38,34,0.08)]">
            <div className="flex items-center gap-2 mb-4">
              <Star className="w-4 h-4 text-primary" />
              <span className="text-sm font-medium text-primary">Start here</span>
            </div>
            <div className="flex flex-col lg:flex-row lg:items-center gap-8">
              <div className="flex-1">
                <h2 className="font-serif text-2xl font-medium text-foreground mb-3">
                  {featuredGuide.title}
                </h2>
                <p className="text-foreground-secondary mb-4">
                  {featuredGuide.description}
                </p>
                <div className="flex items-center gap-4 text-sm text-foreground-tertiary mb-6">
                  <span className="flex items-center gap-1">
                    <Clock className="w-4 h-4" />
                    {featuredGuide.duration}
                  </span>
                </div>
                <a
                  href="/onboarding"
                  className="inline-flex items-center gap-2 rounded-full bg-primary px-5 py-2.5 font-medium text-primary-foreground transition-[background-color,box-shadow] hover:bg-primary-hover hover:shadow-lg"
                >
                  Start reading
                  <ArrowRight className="w-4 h-4" />
                </a>
              </div>
              <div className="lg:w-72">
                <div className="rounded-2xl border border-warm-200/90 bg-white/88 p-5 shadow-sm">
                  <p className="text-sm font-medium text-foreground mb-3">
                    In this guide:
                  </p>
                  <ul className="space-y-2">
                    {featuredGuide.steps?.map((step, idx) => (
                      <li
                        key={step}
                        className="flex items-center gap-2 text-sm text-foreground-secondary"
                      >
                        <span className="w-5 h-5 rounded-full bg-warm-100 flex items-center justify-center text-xs font-medium text-foreground-secondary flex-shrink-0">
                          {idx + 1}
                        </span>
                        {step}
                      </li>
                    ))}
                  </ul>
                </div>
              </div>
            </div>
          </div>
        </div>
      </section>

      {/* Other guides */}
      <section className="pb-20 px-6">
        <div className="max-w-4xl mx-auto">
          <h2 className="font-serif text-xl font-medium text-foreground mb-8">
            More guides
          </h2>
          <div className="grid gap-6">
            {remainingGuides.map((guide) => (
              <a
                key={guide.title}
                href="/onboarding"
                className="group flex flex-col gap-6 rounded-2xl border border-border bg-white/88 p-6 shadow-sm transition-[border-color,box-shadow,transform] hover:border-warm-300 hover:shadow-md sm:flex-row"
              >
                <div className="w-14 h-14 rounded-xl bg-warm-100 flex items-center justify-center flex-shrink-0">
                  <guide.icon className="w-7 h-7 text-foreground-secondary" />
                </div>
                <div className="flex-1">
                  <h3 className="text-lg font-semibold text-foreground mb-2 group-hover:text-primary transition-colors">
                    {guide.title}
                  </h3>
                  <p className="text-foreground-secondary mb-3">
                    {guide.description}
                  </p>
                  <div className="flex items-center gap-4 text-sm text-foreground-tertiary">
                    <span className="flex items-center gap-1">
                      <Clock className="w-4 h-4" />
                      {guide.duration}
                    </span>
                  </div>
                </div>
                <div className="flex items-center">
                  <ArrowRight className="w-5 h-5 text-foreground-tertiary group-hover:text-primary group-hover:translate-x-1 transition-all" />
                </div>
              </a>
            ))}
          </div>
        </div>
      </section>

      {/* CTA */}
      <section className="pb-20 px-6">
        <div className="max-w-4xl mx-auto">
          <div className="text-center rounded-[2rem] border border-warm-200/90 bg-[linear-gradient(180deg,rgba(243,238,230,0.9),rgba(255,255,255,0.98))] p-10 shadow-sm">
            <BookOpen className="w-12 h-12 text-foreground-secondary mx-auto mb-4" />
            <h2 className="font-serif text-2xl font-medium text-foreground mb-3">
              Ready to put this into practice?
            </h2>
            <p className="text-foreground-secondary mb-6">
              Your first 6 months are free. Start building your store today.
            </p>
            <a
              href="/onboarding"
              className="inline-flex items-center gap-2 rounded-full bg-primary px-6 py-3 font-medium text-primary-foreground transition-[background-color,box-shadow] hover:bg-primary-hover hover:shadow-lg"
            >
              Create your store
              <ArrowRight className="w-4 h-4" />
            </a>
          </div>
        </div>
      </section>

      <Footer />
    </div>
  );
}
