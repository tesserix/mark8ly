import Link from "next/link";

export interface BreadcrumbItem {
  label: string;
  href?: string;
}

interface BreadcrumbsProps {
  items: BreadcrumbItem[];
  className?: string;
}

export function Breadcrumbs({ items, className }: BreadcrumbsProps) {
  if (items.length === 0) return null;

  return (
    <nav
      aria-label="Breadcrumb"
      className={
        "flex flex-wrap items-center gap-x-2 gap-y-1 text-sm text-foreground-secondary" +
        (className ? ` ${className}` : "")
      }
    >
      {items.map((item, index) => {
        const isLast = index === items.length - 1;
        return (
          <span key={`${item.label}-${index}`} className="flex items-center gap-2">
            {item.href && !isLast ? (
              <Link
                href={item.href}
                className="rounded-sm transition-colors hover:text-foreground focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
              >
                {item.label}
              </Link>
            ) : (
              <span
                className={isLast ? "truncate text-foreground" : undefined}
                aria-current={isLast ? "page" : undefined}
              >
                {item.label}
              </span>
            )}
            {!isLast && (
              <span aria-hidden="true" className="text-foreground-tertiary">
                /
              </span>
            )}
          </span>
        );
      })}
    </nav>
  );
}
