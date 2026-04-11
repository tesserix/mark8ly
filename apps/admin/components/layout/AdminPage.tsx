import type { ReactNode } from "react";

// AdminPage — canonical page wrapper for every admin surface.
//
// One place to fix page-level spacing, heading hierarchy, and container
// width, so we never copy/paste `<div className="mx-auto w-full max-w-5xl
// space-y-10">` + a 5-line header again.
//
// Usage:
//   <AdminPage
//     eyebrow="Store setup"
//     title="Payments & tax"
//     description="Configure payment gateways for your store (IN)."
//     actions={<AddButton />}
//   >
//     {content}
//   </AdminPage>
//
// Intentionally dumb. No data fetching, no auth checks — callers still
// own that. This just enforces consistent chrome.

interface AdminPageProps {
  /**
   * Optional eyebrow label shown above the title in small caps.
   * Follows the `.eyebrow` class from mark8ly-tokens.css.
   */
  eyebrow?: string;
  /**
   * Page title. Always Source Serif 4, always the same size. Do not
   * override per page — if a surface needs a different hierarchy, it's
   * probably a section heading, not a page heading.
   */
  title: string;
  /**
   * Optional description paragraph shown under the title. Muted, capped
   * at a comfortable reading measure.
   */
  description?: ReactNode;
  /**
   * Optional action slot rendered to the right of the title row on wide
   * viewports (primary CTAs, filters, etc.). Wraps below the title on
   * narrow viewports.
   */
  actions?: ReactNode;
  /**
   * Optional role-based read-only banner. Shown under the description.
   * Pass the merchant's role to surface the warning automatically.
   */
  readOnlyNotice?: ReactNode;
  /**
   * Container max-width token. Defaults to "md" (max-w-5xl) which fits
   * every settings and list page. Use "lg" (max-w-6xl) for wide layouts
   * like themes/branding, "sm" (max-w-3xl) for narrow detail pages.
   */
  maxWidth?: "sm" | "md" | "lg";
  children: ReactNode;
}

const MAX_WIDTH_CLASS: Record<NonNullable<AdminPageProps["maxWidth"]>, string> = {
  sm: "max-w-3xl",
  md: "max-w-5xl",
  lg: "max-w-6xl",
};

export function AdminPage({
  eyebrow,
  title,
  description,
  actions,
  readOnlyNotice,
  maxWidth = "md",
  children,
}: AdminPageProps) {
  return (
    <div
      className={`mx-auto w-full ${MAX_WIDTH_CLASS[maxWidth]} space-y-10`}
    >
      <header className="space-y-4">
        <div className="flex flex-wrap items-end justify-between gap-x-6 gap-y-4">
          <div className="min-w-0 flex-1 space-y-3">
            {eyebrow && <p className="eyebrow">{eyebrow}</p>}
            <h1 className="font-serif text-4xl font-medium tracking-tight text-foreground text-balance sm:text-5xl">
              {title}
            </h1>
            {description && (
              <p className="max-w-2xl text-base leading-7 text-foreground-secondary">
                {description}
              </p>
            )}
          </div>
          {actions && (
            <div className="flex shrink-0 items-center gap-2">{actions}</div>
          )}
        </div>
        {readOnlyNotice}
      </header>

      {children}
    </div>
  );
}

// PageSection — the standard section wrapper used inside an AdminPage.
// Optional eyebrow + heading + description, followed by children. Keeps
// inner sections consistent with the page-level header hierarchy.
interface PageSectionProps {
  eyebrow?: string;
  title?: string;
  description?: ReactNode;
  actions?: ReactNode;
  children: ReactNode;
}

export function PageSection({
  eyebrow,
  title,
  description,
  actions,
  children,
}: PageSectionProps) {
  const hasHeader = !!(eyebrow || title || description || actions);
  return (
    <section className="space-y-6">
      {hasHeader && (
        <div className="flex flex-wrap items-end justify-between gap-x-6 gap-y-3">
          <div className="min-w-0 flex-1 space-y-2">
            {eyebrow && <p className="eyebrow">{eyebrow}</p>}
            {title && (
              <h2 className="font-serif text-2xl font-medium tracking-tight text-foreground text-balance">
                {title}
              </h2>
            )}
            {description && (
              <p className="max-w-2xl text-sm leading-6 text-foreground-secondary">
                {description}
              </p>
            )}
          </div>
          {actions && (
            <div className="flex shrink-0 items-center gap-2">{actions}</div>
          )}
        </div>
      )}
      {children}
    </section>
  );
}

// ReadOnlyNotice — tiny helper used by pages to render the "your role can
// view but not edit" banner inside the AdminPage header slot. Kept as a
// helper so copy stays consistent everywhere.
export function ReadOnlyNotice({ role }: { role: string }) {
  return (
    <p className="text-sm text-warning">
      Read-only: your role ({role}) can view settings but cannot edit them.
    </p>
  );
}
