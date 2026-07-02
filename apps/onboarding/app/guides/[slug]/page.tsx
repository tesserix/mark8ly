import type { Metadata } from "next";
import Link from "next/link";
import { notFound } from "next/navigation";

import { MarketingPage, PageHero, Prose } from "@/components/marketing/primitives";
import { GUIDES, getGuide, type GuideBlock } from "../guides";

const SITE_URL = "https://mark8ly.com";

interface PageProps {
  params: Promise<{ slug: string }>;
}

export function generateStaticParams(): Array<{ slug: string }> {
  return GUIDES.map((g) => ({ slug: g.slug }));
}

export async function generateMetadata({
  params,
}: PageProps): Promise<Metadata> {
  const { slug } = await params;
  const guide = getGuide(slug);
  if (!guide) return { title: "Guide not found" };
  const url = `${SITE_URL}/guides/${guide.slug}`;
  return {
    title: guide.title,
    description: guide.description,
    alternates: { canonical: `/guides/${guide.slug}` },
    openGraph: {
      type: "article",
      title: guide.title,
      description: guide.description,
      url,
    },
  };
}

function Block({ block }: { block: GuideBlock }) {
  switch (block.type) {
    case "h2":
      return <h2>{block.text}</h2>;
    case "ul":
      return (
        <ul>
          {block.items.map((item, i) => (
            <li key={i}>{item}</li>
          ))}
        </ul>
      );
    case "p":
    default:
      return <p>{block.text}</p>;
  }
}

export default async function GuidePage({ params }: PageProps) {
  const { slug } = await params;
  const guide = getGuide(slug);
  if (!guide) notFound();

  const url = `${SITE_URL}/guides/${guide.slug}`;

  // Article JSON-LD — content is first-party, but we still escape `<`
  // in the serialized output as a matter of habit.
  const jsonLd = {
    "@context": "https://schema.org",
    "@type": "Article",
    headline: guide.heading,
    description: guide.description,
    datePublished: guide.updated,
    dateModified: guide.updated,
    author: { "@type": "Organization", name: "Mark8ly" },
    publisher: {
      "@type": "Organization",
      name: "Mark8ly",
      url: SITE_URL,
    },
    mainEntityOfPage: { "@type": "WebPage", "@id": url },
  };

  const otherGuides = GUIDES.filter((g) => g.slug !== guide.slug);

  return (
    <MarketingPage>
      <script
        type="application/ld+json"
        dangerouslySetInnerHTML={{
          __html: JSON.stringify(jsonLd).replace(/</g, "\\u003c"),
        }}
      />

      <PageHero eyebrow={guide.eyebrow} title={guide.heading} lede={guide.lede} />

      <Prose>
        {guide.blocks.map((block, i) => (
          <Block key={i} block={block} />
        ))}

        <p className="mt-4 text-sm text-foreground-tertiary">
          Last updated{" "}
          {new Date(guide.updated).toLocaleDateString("en-GB", {
            day: "numeric",
            month: "long",
            year: "numeric",
          })}
          .
        </p>
      </Prose>

      {/* Related — internal links to the other guides + a CTA. */}
      <section className="border-t border-border-subtle pb-24 pt-16">
        <div className="mx-auto max-w-3xl px-6">
          <p className="eyebrow mb-6">Keep reading</p>
          <ul className="divide-y divide-border-subtle">
            {otherGuides.map((g) => (
              <li key={g.slug}>
                <Link
                  href={`/guides/${g.slug}`}
                  className="group flex items-baseline justify-between gap-6 py-5"
                >
                  <span className="font-serif text-lg text-foreground group-hover:text-moss-700">
                    {g.heading}
                  </span>
                  <span className="shrink-0 text-sm text-foreground-tertiary">
                    {g.readingMinutes} min
                  </span>
                </Link>
              </li>
            ))}
          </ul>

          <div className="mt-12">
            <Link
              href="/onboarding"
              className="inline-flex h-12 items-center rounded-md bg-primary px-6 text-base font-medium text-primary-foreground hover:bg-primary-hover"
            >
              Open your store
            </Link>
          </div>
        </div>
      </section>
    </MarketingPage>
  );
}
