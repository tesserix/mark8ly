import Link from "next/link";
import { ArrowUpRight } from "lucide-react";

interface CategoryData {
  name: string;
  count: number;
  firstSlug: string;
}

interface HelpCategoryGridProps {
  categories: CategoryData[];
}

export function HelpCategoryGrid({ categories }: HelpCategoryGridProps) {
  return (
    <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
      {categories.map((cat) => (
        <Link
          key={cat.name}
          href={`/support/help/${cat.firstSlug}`}
          className="group flex min-h-[120px] flex-col justify-between rounded-lg border border-border-subtle bg-background-elevated p-5 transition-all hover:-translate-y-0.5 hover:border-[color:var(--moss-700)] hover:shadow-md"
        >
          <div>
            <p className="text-[11px] font-semibold uppercase tracking-[0.16em] text-foreground-tertiary">
              {cat.count} {cat.count === 1 ? "article" : "articles"}
            </p>
            <h3 className="mt-2 font-serif text-xl font-medium text-foreground group-hover:text-[color:var(--moss-700)]">
              {cat.name}
            </h3>
          </div>
          <ArrowUpRight
            className="mt-3 h-4 w-4 text-foreground-tertiary transition-colors group-hover:text-[color:var(--moss-700)]"
            aria-hidden="true"
          />
        </Link>
      ))}
    </div>
  );
}
