import Link from "next/link";
import { ChevronLeft } from "lucide-react";
import { notFound } from "next/navigation";

import { getServerSessionContext } from "@/lib/auth/serverSession";
import { getArticle, getArticles } from "@/lib/help";
import { HelpArticleRenderer } from "@/components/support/HelpArticleRenderer";
import { HelpArticleFeedback } from "@/components/support/HelpArticleFeedback";

interface HelpArticlePageProps {
  params: Promise<{ slug: string }>;
}

export default async function HelpArticlePage({
  params,
}: HelpArticlePageProps) {
  const { slug } = await params;
  const {
    tenantName,
    email,
    role,
    memberships,
    tenantId,
  } = await getServerSessionContext();

  const article = getArticle(slug);
  if (!article) {
    notFound();
  }

  // Related articles: same category, excluding current
  const allArticles = getArticles();
  const related = allArticles
    .filter((a) => a.category === article.category && a.slug !== article.slug)
    .slice(0, 3);

  return (
          <div className="mx-auto w-full max-w-3xl space-y-8">
        {/* Breadcrumb */}
        <nav aria-label="Breadcrumb" className="flex items-center gap-2 text-sm text-foreground-secondary">
          <Link
            href="/support/help"
            className="hover:text-foreground"
          >
            Help Center
          </Link>
          <span aria-hidden="true">/</span>
          <span className="text-foreground">{article.category}</span>
        </nav>

        <Link
          href="/support/help"
          className="inline-flex min-h-[44px] items-center gap-1 text-sm text-foreground-secondary hover:text-foreground"
        >
          <ChevronLeft className="h-4 w-4" aria-hidden="true" />
          Back to Help Center
        </Link>

        <header>
          <h1 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-4xl font-medium tracking-tight text-foreground">
            {article.title}
          </h1>
        </header>

        <HelpArticleRenderer content={article.content} />

        {/* Feedback */}
        <div className="border-t border-border-subtle pt-6">
          <HelpArticleFeedback />
        </div>

        {/* Related articles */}
        {related.length > 0 && (
          <section className="border-t border-border-subtle pt-8">
            <h2 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-xl font-medium text-foreground">
              Related articles
            </h2>
            <ul className="mt-4 space-y-3">
              {related.map((r) => (
                <li key={r.slug}>
                  <Link
                    href={`/support/help/${r.slug}`}
                    className="inline-flex min-h-[44px] items-center text-sm text-[color:var(--moss-700)] hover:underline"
                  >
                    {r.title} &rarr;
                  </Link>
                </li>
              ))}
            </ul>
          </section>
        )}
      </div>
  );
}
