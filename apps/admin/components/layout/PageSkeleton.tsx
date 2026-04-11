// PageSkeleton — suspense placeholders used across the admin app.
//
// The variants cover 90% of the surfaces: `list` for table/card lists,
// `detail` for entity detail pages with a form, `form` for tall forms,
// `dashboard` for the dashboard stat grid. Each variant mirrors the
// approximate shape of its target so the first paint settles smoothly.
//
// All skeletons use the same pulsing utility (`animate-pulse`) and the
// same `--ink-900` tint so the loading state feels cohesive regardless
// of which section the user is in.

import type { ReactNode } from "react";

function SkeletonBar({
  width = "w-full",
  height = "h-4",
  className = "",
}: {
  width?: string;
  height?: string;
  className?: string;
}) {
  return (
    <div
      aria-hidden="true"
      className={`rounded-[4px] bg-[color:var(--ink-900)]/5 ${height} ${width} ${className}`}
    />
  );
}

function SkeletonBlock({
  className = "",
  children,
}: {
  className?: string;
  children?: ReactNode;
}) {
  return (
    <div
      aria-hidden="true"
      className={`rounded-[6px] bg-white p-6 ${className}`}
    >
      {children}
    </div>
  );
}

// SkeletonHeader — mimics the AdminPage header block. Used by every
// variant so the page-level chrome doesn't shift when content loads.
function SkeletonHeader() {
  return (
    <header className="space-y-4">
      <SkeletonBar width="w-28" height="h-3" />
      <SkeletonBar width="w-80 max-w-full" height="h-10" />
      <SkeletonBar width="w-full max-w-xl" height="h-4" />
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
      className="mx-auto w-full max-w-5xl space-y-10 animate-pulse"
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

function ListSkeleton() {
  return (
    <div className="space-y-3">
      {/* Filter bar row */}
      <div className="flex items-center gap-3">
        <SkeletonBar width="flex-1 min-w-0" height="h-10" />
        <SkeletonBar width="w-40" height="h-10" />
      </div>
      {/* Rows */}
      {Array.from({ length: 6 }).map((_, i) => (
        <SkeletonBlock key={i}>
          <div className="flex items-center justify-between gap-4">
            <div className="flex-1 space-y-2">
              <SkeletonBar width="w-64 max-w-full" height="h-4" />
              <SkeletonBar width="w-40" height="h-3" />
            </div>
            <SkeletonBar width="w-20" height="h-6" />
          </div>
        </SkeletonBlock>
      ))}
    </div>
  );
}

function DetailSkeleton() {
  return (
    <div className="space-y-6">
      <SkeletonBlock>
        <div className="space-y-3">
          <SkeletonBar width="w-48" height="h-5" />
          <SkeletonBar width="w-full" height="h-4" />
          <SkeletonBar width="w-5/6" height="h-4" />
          <SkeletonBar width="w-4/6" height="h-4" />
        </div>
      </SkeletonBlock>
      <SkeletonBlock>
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-2">
            <SkeletonBar width="w-20" height="h-3" />
            <SkeletonBar height="h-10" />
          </div>
          <div className="space-y-2">
            <SkeletonBar width="w-20" height="h-3" />
            <SkeletonBar height="h-10" />
          </div>
        </div>
      </SkeletonBlock>
    </div>
  );
}

function FormSkeleton() {
  return (
    <div className="space-y-6">
      {Array.from({ length: 3 }).map((_, i) => (
        <SkeletonBlock key={i}>
          <div className="space-y-4">
            <SkeletonBar width="w-32" height="h-5" />
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="space-y-1.5">
                <SkeletonBar width="w-20" height="h-3" />
                <SkeletonBar height="h-10" />
              </div>
              <div className="space-y-1.5">
                <SkeletonBar width="w-24" height="h-3" />
                <SkeletonBar height="h-10" />
              </div>
            </div>
          </div>
        </SkeletonBlock>
      ))}
    </div>
  );
}

function DashboardSkeleton() {
  return (
    <div className="space-y-8">
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        {Array.from({ length: 4 }).map((_, i) => (
          <SkeletonBlock key={i}>
            <div className="space-y-3">
              <SkeletonBar width="w-20" height="h-3" />
              <SkeletonBar width="w-28" height="h-8" />
              <SkeletonBar width="w-16" height="h-3" />
            </div>
          </SkeletonBlock>
        ))}
      </div>
      <div className="grid gap-6 lg:grid-cols-3">
        <SkeletonBlock className="lg:col-span-2">
          <div className="space-y-3">
            <SkeletonBar width="w-36" height="h-5" />
            <SkeletonBar height="h-48" />
          </div>
        </SkeletonBlock>
        <SkeletonBlock>
          <div className="space-y-3">
            <SkeletonBar width="w-32" height="h-5" />
            {Array.from({ length: 4 }).map((_, i) => (
              <SkeletonBar key={i} height="h-4" />
            ))}
          </div>
        </SkeletonBlock>
      </div>
    </div>
  );
}
