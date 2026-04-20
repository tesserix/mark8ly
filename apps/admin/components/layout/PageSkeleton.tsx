"use client";

// PageSkeleton — suspense placeholders used across the admin app.
//
// Marked `"use client"` because the underlying `@tesserix/web` skeleton
// primitives ship with `"use client"` themselves — Turbopack's resolver
// returns undefined for those imports when they bubble through a
// server-component file, which breaks prerender of every route that has
// a `loading.tsx` importing this module.
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

import { Skeleton, PanelSkeleton } from "@tesserix/web";

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
      className="mx-auto w-full max-w-6xl space-y-10"
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
  // Hairline list skeleton — mirrors the real list pages. Uses
  // border-border-subtle (ink @ ~10% but tokenized) so rows read as
  // quiet hairlines, not black rules. No bordered table container,
  // no wrapped filter-box. Matches the visual weight of OrdersList
  // and ProductsList once they render.
  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-col gap-4">
        <div className="flex items-center gap-3 border-b border-border-subtle pb-2">
          <Skeleton className="h-5 flex-1 max-w-sm" />
        </div>
        <div className="flex flex-wrap items-center gap-3">
          <Skeleton className="h-3 w-14" />
          <Skeleton className="h-3 w-10" />
          <Skeleton className="h-3 w-16" />
          <Skeleton className="h-3 w-14" />
          <Skeleton className="h-3 w-18" />
        </div>
      </div>
      <div className="flex flex-col">
        <div className="grid grid-cols-[minmax(0,2fr)_minmax(0,1fr)_minmax(0,1fr)_minmax(0,1fr)] gap-6 border-b border-border-subtle pb-3">
          <Skeleton className="h-3 w-20" />
          <Skeleton className="h-3 w-16" />
          <Skeleton className="h-3 w-14" />
          <Skeleton className="h-3 w-12 justify-self-end" />
        </div>
        {Array.from({ length: 6 }).map((_, i) => (
          <div
            key={i}
            className="grid grid-cols-[minmax(0,2fr)_minmax(0,1fr)_minmax(0,1fr)_minmax(0,1fr)] items-center gap-6 border-b border-border-subtle py-4"
          >
            <div className="flex flex-col gap-1.5">
              <Skeleton className="h-4 w-48 max-w-full" />
              <Skeleton className="h-3 w-32 max-w-full" />
            </div>
            <Skeleton className="h-3 w-16" />
            <Skeleton className="h-3 w-20" />
            <Skeleton className="h-4 w-20 justify-self-end" />
          </div>
        ))}
      </div>
    </div>
  );
}

function DetailSkeleton() {
  // Detail skeleton — mirrors an entity detail page (order, product,
  // customer). Masthead serif number + subtitle row, lifecycle stepper
  // rail, two-column body with rail cards. Reserves space for the real
  // layout so first paint doesn't shift.
  return (
    <div className="space-y-8">
      <div className="flex flex-col gap-3">
        <div className="flex flex-wrap items-baseline gap-x-4 gap-y-1">
          <Skeleton className="h-9 w-56" />
          <Skeleton className="h-6 w-3" />
          <Skeleton className="h-7 w-32" />
        </div>
        <div className="flex flex-wrap items-center gap-x-5 gap-y-1">
          <Skeleton className="h-3 w-28" />
          <Skeleton className="h-3 w-36" />
          <Skeleton className="h-3 w-20" />
        </div>
      </div>
      <div className="flex items-center justify-between border-y border-border-subtle py-5">
        {Array.from({ length: 4 }).map((_, i) => (
          <div key={i} className="flex items-center gap-2.5">
            <Skeleton className="h-6 w-6 rounded-full" />
            <Skeleton className="h-3 w-16" />
          </div>
        ))}
      </div>
      <div className="grid gap-8 lg:grid-cols-[minmax(0,1fr)_280px]">
        <div className="space-y-6">
          <PanelSkeleton />
          <PanelSkeleton />
        </div>
        <div className="space-y-6">
          <PanelSkeleton />
          <PanelSkeleton />
        </div>
      </div>
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
