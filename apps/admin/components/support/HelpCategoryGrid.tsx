import Link from "next/link";

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
    <div className="grid grid-cols-1 gap-6 sm:grid-cols-2 lg:grid-cols-3">
      {categories.map((cat, idx) => (
        <Link
          key={cat.name}
          href={`/support/help/${cat.firstSlug}`}
          className={`group min-h-[44px] border-b border-border-subtle pb-5 transition-colors hover:border-[color:var(--moss-700)] ${
            idx === 0 ? "sm:col-span-2 lg:col-span-1" : ""
          }`}
        >
          <p className="text-[11px] font-semibold uppercase tracking-[0.16em] text-foreground-tertiary">
            {cat.count} {cat.count === 1 ? "article" : "articles"}
          </p>
          <h3 className="mt-2 font-serif text-xl font-medium text-foreground group-hover:text-[color:var(--moss-700)]">
            {cat.name}
          </h3>
        </Link>
      ))}
    </div>
  );
}
