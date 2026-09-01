"use client";

// ProductSection — one band of the product form.
//
// The form used to be five tabs. Tabs hid whether a section held anything
// until you clicked it, and the tab bar changed shape between create and
// edit (Media only works once a product exists), which is a sign the
// information architecture was wrong rather than that Media needed a
// placeholder.
//
// Sections on one scrolling page, separated by hairline rules — the
// system's own idiom — mean the whole product is visible by scrolling,
// and a section that does not apply is simply absent rather than a dead
// tab.

import type { ReactNode } from "react";

export interface ProductSectionProps {
  id: string;
  title: string;
  /** One line under the heading. Say what the section is for, not what it is called. */
  description?: ReactNode;
  /** Rendered at the right of the heading row — counts, states, small actions. */
  aside?: ReactNode;
  children: ReactNode;
}

export function ProductSection({
  id,
  title,
  description,
  aside,
  children,
}: ProductSectionProps) {
  return (
    <section
      id={id}
      aria-labelledby={`${id}-heading`}
      className="scroll-mt-24 border-t border-border-subtle pt-8"
    >
      <div className="mb-6 flex flex-wrap items-baseline justify-between gap-x-6 gap-y-1">
        <div className="min-w-0">
          <h2
            id={`${id}-heading`}
            className="font-serif text-xl text-[color:var(--ink-900)]"
          >
            {title}
          </h2>
          {description && (
            <p className="mt-1 max-w-2xl text-sm text-foreground-secondary">
              {description}
            </p>
          )}
        </div>
        {aside}
      </div>
      {children}
    </section>
  );
}
