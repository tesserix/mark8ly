"use client";

// apps/storefront/components/StorefrontNav.tsx
//
// Storefront navigation bar. Home / Shop / Cart / Auth. Uses
// Paper · Ink · Moss tokens so it sits cleanly on any theme background.

import { Suspense } from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { CartCountBadge } from "./CartCountBadge";
import { CustomerAccountMenu } from "./CustomerAccountMenu";

export interface StorefrontNavProps {
  /** Optional store name shown as the left-hand brand slot. */
  storeName?: string;
}

const NAV_LINKS = [
  { href: "/", label: "Home", exact: true },
  { href: "/products", label: "Shop", exact: false },
] as const;

export function StorefrontNav({ storeName }: StorefrontNavProps) {
  const pathname = usePathname();

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
        {NAV_LINKS.map((link) => {
          const isActive = link.exact
            ? pathname === link.href
            : pathname.startsWith(link.href);

          return (
            <li key={link.href}>
              <Link
                href={link.href}
                aria-current={isActive ? "page" : undefined}
                className="min-h-[44px] flex items-center text-[color:var(--ink-900)] opacity-70 transition-opacity hover:opacity-100 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
              >
                {link.label}
              </Link>
            </li>
          );
        })}
        <li>
          <Link
            href="/cart"
            aria-current={pathname === "/cart" ? "page" : undefined}
            className="min-h-[44px] inline-flex items-center gap-1.5 text-[color:var(--ink-900)] opacity-70 transition-opacity hover:opacity-100 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
          >
            Cart
            <Suspense fallback={null}>
              <CartCountBadge />
            </Suspense>
          </Link>
        </li>
        <li className="min-h-[44px] flex items-center">
          <CustomerAccountMenu />
        </li>
      </ul>
    </nav>
  );
}
