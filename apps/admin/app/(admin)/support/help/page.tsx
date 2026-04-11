import Link from "next/link";

import { getServerSessionContext } from "@/lib/auth/serverSession";
import { getArticles } from "@/lib/help";
import { HelpSearch } from "@/components/support/HelpSearch";
import { HelpCategoryGrid } from "@/components/support/HelpCategoryGrid";

export default async function HelpCenterPage() {
  const {
    tenantName,
    email,
    role,
    memberships,
    tenantId,
  } = await getServerSessionContext();

  const articles = getArticles();

  // Build category data
  const categoryMap = new Map<
    string,
    { count: number; firstSlug: string }
  >();
  for (const article of articles) {
    const existing = categoryMap.get(article.category);
    if (existing) {
      categoryMap.set(article.category, {
        count: existing.count + 1,
        firstSlug: existing.firstSlug,
      });
    } else {
      categoryMap.set(article.category, {
        count: 1,
        firstSlug: article.slug,
      });
    }
  }

  const categories = Array.from(categoryMap.entries()).map(
    ([name, data]) => ({
      name,
      count: data.count,
      firstSlug: data.firstSlug,
    }),
  );

  // Featured: first 5 articles by order
  const featured = articles.slice(0, 5);

  return (
          <div className="mx-auto w-full max-w-5xl space-y-12">
        <header className="space-y-5">
          <p className="eyebrow">Support</p>
          <h1 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-5xl font-medium tracking-tight text-foreground">
            Help Center
          </h1>
          <p className="max-w-2xl text-base leading-7 text-foreground-secondary">
            Find answers, guides, and best practices for running your
            store on Mark8ly.
          </p>
          <HelpSearch articles={articles} />
        </header>

        {/* Categories */}
        <section>
          <h2 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-2xl font-medium text-foreground">
            Browse by category
          </h2>
          <div className="mt-6">
            <HelpCategoryGrid categories={categories} />
          </div>
        </section>

        {/* Featured articles */}
        <section className="border-t border-border-subtle pt-10">
          <h2 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-2xl font-medium text-foreground">
            Popular articles
          </h2>
          <ul className="mt-6 divide-y divide-border-subtle" role="list">
            {featured.map((article) => (
              <li key={article.slug}>
                <Link
                  href={`/support/help/${article.slug}`}
                  className="block min-h-[44px] py-4 transition-colors hover:bg-paper-100"
                >
                  <div className="flex items-start justify-between gap-4">
                    <div>
                      <p className="text-sm font-medium text-foreground">
                        {article.title}
                      </p>
                      <p className="mt-1 text-xs text-foreground-tertiary">
                        {article.category}
                      </p>
                    </div>
                    <span className="shrink-0 text-sm text-[color:var(--moss-700)]">
                      Read &rarr;
                    </span>
                  </div>
                </Link>
              </li>
            ))}
          </ul>
        </section>
      </div>
  );
}
