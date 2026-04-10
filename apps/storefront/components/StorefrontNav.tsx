// apps/storefront/components/StorefrontNav.tsx
//
// Minimal storefront navigation bar. Home / Shop / Cart. Uses
// Paper · Ink · Moss tokens so it sits cleanly on any theme background.

import { Suspense } from "react";
import Link from "next/link";
import { CartCountBadge } from "./CartCountBadge";

export interface StorefrontNavProps {
  /** Optional store name shown as the left-hand brand slot. */
  storeName?: string;
}

export function StorefrontNav({ storeName }: StorefrontNavProps) {
  return (
    <nav
      aria-label="Store"
      className="mb-10 flex items-center justify-between gap-4 border-b border-[color:var(--ink-900)] border-opacity-10 pb-4"
    >
      <Link
        href="/"
        className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-lg text-[color:var(--ink-900)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
      >
        {storeName ?? "Store"}
      </Link>
      <ul className="flex items-center gap-6 text-sm">
        <li>
          <Link
            href="/"
            className="text-[color:var(--ink-900)] opacity-70 transition-opacity hover:opacity-100 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
          >
            Home
          </Link>
        </li>
        <li>
          <Link
            href="/products"
            className="text-[color:var(--ink-900)] opacity-70 transition-opacity hover:opacity-100 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
          >
            Shop
          </Link>
        </li>
        <li>
          <Link
            href="/cart"
            className="inline-flex items-center gap-1.5 text-[color:var(--ink-900)] opacity-70 transition-opacity hover:opacity-100 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
          >
            Cart
            <Suspense fallback={null}>
              <CartCountBadge />
            </Suspense>
          </Link>
        </li>
      </ul>
    </nav>
  );
}
