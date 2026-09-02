import type { Metadata } from "next";
import Link from "next/link";
import { notFound } from "next/navigation";

import { MarketingPage, PageHero, Prose } from "@/components/marketing/primitives";
import { ORGANIZATION_ID, SITE_URL } from "@/lib/seo/site-json-ld";
import { GUIDES, getGuide, type GuideBlock } from "../guides";

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
    case "sources":
      // Rendered, not hidden in the JSON-LD. A citation a reader cannot
      // click is a schema decoration; the point is that someone can
      // check the claim. Links stay dofollow on purpose \u2014 these are
      // genuine references to a regulator and a processor, and
      // nofollowing them would be disowning our own sources.
      return (
        <>
          <h2>Sources</h2>
          <ul>
            {block.items.map((source) => (
              <li key={source.url}>
                <a href={source.url} target="_blank" rel="noopener noreferrer">
                  {source.label}
                </a>
              </li>
            ))}
          </ul>
        </>
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

  const citations = guide.blocks
    .filter((block) => block.type === "sources")
    .flatMap((block) => block.items)
    .map((source) => ({
      "@type": "CreativeWork",
      name: source.label,
      url: source.url,
    }));

  // Article JSON-LD — content is first-party, but we still escape `<`
  // in the serialized output as a matter of habit.
  const jsonLd = {
    "@context": "https://schema.org",
    "@type": "Article",
    headline: guide.heading,
    description: guide.description,
    datePublished: guide.updated,
    dateModified: guide.updated,
    // `publisher` always references the Organization the root layout
    // already puts on this page, rather than declaring a second one.
    // Google merges same-page JSON-LD, and the inline copy that used to
    // live here carried no logo — so the richer node was being shadowed
    // by a poorer duplicate (tesserix/mark8ly#600).
    //
    // `author` is the Person where the guide names one, and falls back
    // to the Organization otherwise. Both are emitted in full rather
    // than as bare @id references, because the Person node exists
    // nowhere else on the page — an @id pointing at nothing resolves to
    // nothing, which is worse than no author at all.
    author: guide.author
      ? {
          "@type": "Person",
          "@id": guide.author.id,
          name: guide.author.name,
          jobTitle: guide.author.jobTitle,
          description: guide.author.bio,
          sameAs: [...guide.author.sameAs],
        }
      : { "@id": ORGANIZATION_ID },
    publisher: { "@id": ORGANIZATION_ID },
    mainEntityOfPage: { "@type": "WebPage", "@id": url },
    // Mirror any rendered "Sources" list into `citation`, so the
    // references a reader can see are also the ones a crawler is told
    // about. Omitted entirely when a guide cites nothing \u2014 an empty
    // array would assert "we checked and there are none", which is a
    // different and untrue claim from staying silent.
    ...(citations.length > 0 ? { citation: citations } : {}),
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

        {guide.author ? (
          // Rendered, not schema-only. A byline a reader cannot see is
          // markup for crawlers; the point of naming someone is that a
          // person deciding whether to trust this can weigh who wrote it.
          <p className="mt-10 border-t border-border-subtle pt-6 text-sm text-foreground-secondary">
            <span className="text-foreground">{guide.author.name}</span>
            {" — "}
            {guide.author.bio}
          </p>
        ) : null}

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
