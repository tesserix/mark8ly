import type { Metadata } from "next";
import Link from "next/link";

import { MarketingPage, PageHero } from "@/components/marketing/primitives";
import { GUIDES } from "./guides";

export const metadata: Metadata = {
  title: "Guides for merchants",
  description:
    "Short, honest guides for people opening an online store — how to start, how to price handmade products, and how to accept UPI payments. No filler.",
  alternates: { canonical: "/guides" },
  openGraph: {
    title: "Guides for merchants — Mark8ly",
    description:
      "Short, honest how-to guides for new merchants. No filler, no upsells.",
    url: "https://mark8ly.com/guides",
  },
};

export default function GuidesPage() {
  return (
    <MarketingPage>
      <PageHero
        eyebrow="Guides"
        title={<>Short, honest walkthroughs.</>}
        lede="A small set of guides for people opening a shop — first product, fair pricing, getting paid. Written to be useful and free of filler."
      />

      <section className="border-t border-border-subtle pb-24 pt-4">
        <div className="mx-auto max-w-3xl px-6">
          <ul className="divide-y divide-border-subtle">
            {GUIDES.map((guide) => (
              <li key={guide.slug}>
                <Link
                  href={`/guides/${guide.slug}`}
                  className="group block py-8"
                >
                  <p className="eyebrow mb-3 text-moss-700">
                    {guide.readingMinutes} min read
                  </p>
                  <h2 className="font-serif text-2xl font-medium leading-[1.15] tracking-[-0.015em] text-foreground group-hover:text-moss-700">
                    {guide.heading}
                  </h2>
                  <p className="mt-3 max-w-xl leading-relaxed text-foreground-secondary">
                    {guide.description}
                  </p>
                </Link>
              </li>
            ))}
          </ul>
        </div>
      </section>
    </MarketingPage>
  );
}
