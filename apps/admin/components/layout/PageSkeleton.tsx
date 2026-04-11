// PageSkeleton — suspense placeholders used across the admin app.
//
// Composes the skeleton primitives shipped by `@tesserix/web` — we do NOT
// hand-roll our own pulse/rounded-rect divs here. The design system owns
// the atomic shapes (`Skeleton`, `PanelSkeleton`, `DataTableSkeleton`);
// this file owns the page-level compositions (list, detail, form,
// dashboard) that mirror the target AdminPage layout.
//
// Variants cover 90% of surfaces:
//   - list      → filter bar + stacked rows (products, orders, customers)
//   - detail    → two-column detail blocks (entity pages)
//   - form      → tall setting forms (payment, shipping, tax, account)
//   - dashboard → stat grid + chart row (/dashboard)
//
// Every variant renders inside the same outer shell (max-w-5xl, space-y-10,
// header skeleton) so the first paint doesn't shift when real content arrives.

import {
  Skeleton,
  PanelSkeleton,
  DataTableSkeleton,
} from "@tesserix/web";

// Page header skeleton — mimics the AdminPage header block. Reused by
// every variant so top-level chrome is consistent.
function SkeletonHeader() {
  return (
    <header className="space-y-4">
      <Skeleton className="h-3 w-28" />
      <Skeleton className="h-10 w-80 max-w-full" />
      <Skeleton className="h-4 w-full max-w-xl" />
    </header>
  );
}

interface PageSkeletonProps {
  variant?: "list" | "detail" | "form" | "dashboard";
  /** Hide the page header skeleton — useful for nested suspense boundaries. */
  omitHeader?: boolean;
}

export function PageSkeleton({
  variant = "list",
  omitHeader = false,
}: PageSkeletonProps) {
  return (
    <div
      role="status"
      aria-live="polite"
      aria-label="Loading"
      className="mx-auto w-full max-w-5xl space-y-10"
    >
      {!omitHeader && <SkeletonHeader />}
      {variant === "list" && <ListSkeleton />}
      {variant === "detail" && <DetailSkeleton />}
      {variant === "form" && <FormSkeleton />}
      {variant === "dashboard" && <DashboardSkeleton />}
      <span className="sr-only">Loading…</span>
    </div>
  );
}

// ─── Variants ────────────────────────────────────────────────────────

function ListSkeleton() {
  return (
    <div className="space-y-3">
      {/* Filter bar */}
      <div className="flex items-center gap-3">
        <Skeleton className="h-10 min-w-0 flex-1" />
        <Skeleton className="h-10 w-40" />
      </div>
      {/* Table body — DataTableSkeleton from the design system */}
      <DataTableSkeleton rows={6} />
    </div>
  );
}

function DetailSkeleton() {
  return (
    <div className="space-y-6">
      <PanelSkeleton />
      <PanelSkeleton />
    </div>
  );
}

function FormSkeleton() {
  return (
    <div className="space-y-6">
      <PanelSkeleton />
      <PanelSkeleton />
      <PanelSkeleton />
    </div>
  );
}

function DashboardSkeleton() {
  return (
    <div className="space-y-8">
      {/* Stat grid */}
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        {Array.from({ length: 4 }).map((_, i) => (
          <PanelSkeleton key={i} />
        ))}
      </div>
      {/* Chart + side panel */}
      <div className="grid gap-6 lg:grid-cols-3">
        <div className="lg:col-span-2">
          <PanelSkeleton />
        </div>
        <PanelSkeleton />
      </div>
    </div>
  );
}
