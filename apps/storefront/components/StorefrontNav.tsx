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

  // Self-contained layout: the nav bar now sets its own max-width
  // and gutters so pages like /cart, /account, and /gift-cards don't
  // need to wrap it in a matching container to avoid overflow. The
  // home page already wraps it in a `max-w-6xl` div — nesting a
  // second max-width here is a no-op when the outer container is
  // tighter, so this change is backwards compatible.
  return (
    <nav
      aria-label="Store"
      className="mb-10 w-full border-b border-[color:var(--storefront-text,var(--ink-900))] border-opacity-10"
    >
      <div className="mx-auto flex max-w-6xl items-center justify-between gap-4 px-6 py-4 sm:px-8">
        <Link
          href="/"
          className="max-w-[45%] truncate font-[family-name:var(--storefront-heading-font,var(--font-source-serif))] text-lg text-[color:var(--storefront-text,var(--ink-900))] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))] sm:max-w-[55%] md:max-w-none"
          title={storeName ?? undefined}
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
                  className="min-h-[44px] flex items-center text-[color:var(--storefront-text,var(--ink-900))] opacity-70 transition-opacity hover:opacity-100 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))]"
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
              className="min-h-[44px] inline-flex items-center gap-1.5 text-[color:var(--storefront-text,var(--ink-900))] opacity-70 transition-opacity hover:opacity-100 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))]"
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
      </div>
    </nav>
  );
}
