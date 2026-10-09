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
import { CustomerNotificationBell } from "./CustomerNotificationBell";
import { useStore } from "./StoreProvider";
import { resolveBrandName } from "@/lib/brand-name";

export interface StorefrontNavProps {
  /**
   * Store name for the left-hand brand slot. Optional: when a page does
   * not pass one, the layout's resolved store supplies it (StoreProvider),
   * so a client page like the cart cannot fall back to the literal
   * "Store" just by forgetting the prop.
   */
  storeName?: string;
}

const NAV_LINKS = [
  { href: "/", label: "Home", exact: true },
  { href: "/products", label: "Shop", exact: false },
] as const;

export function StorefrontNav({ storeName }: StorefrontNavProps) {
  const pathname = usePathname();
  const brandName = resolveBrandName(storeName, useStore().name);

  // The nav owns its own max-width and gutters, so pages must NOT wrap
  // it in a second `px-6` container: on a 400px phone the two gutters
  // together left ~304px for the brand and five controls, and the demo
  // store's name rendered as "The B…" (tesserix/mark8ly#995). Place it
  // as a direct child of <main>, before any content wrapper.
  //
  // Below `sm` the brand takes a row of its own and is allowed to wrap,
  // so a long store name reads in full instead of shrinking to a few
  // letters; the links sit in a compact row beneath it. From `sm` up
  // the bar is the familiar single row with the brand on the left.
  //
  // The bell and account slots render nothing until the customer is
  // signed in (or when the login URL is misconfigured). `empty:hidden`
  // collapses those <li>s so they do not hold open a gap each.
  return (
    <nav
      aria-label="Store"
      className="mb-10 w-full border-b border-[color:var(--storefront-text,var(--ink-900))] border-opacity-10"
    >
      <div className="mx-auto flex max-w-6xl flex-wrap items-center justify-between gap-x-6 gap-y-1 px-6 py-3 sm:flex-nowrap sm:gap-4 sm:px-8 sm:py-4">
        <Link
          href="/"
          className="basis-full break-words font-[family-name:var(--storefront-heading-font,var(--font-source-serif))] text-lg text-[color:var(--storefront-text,var(--ink-900))] empty:hidden focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))] sm:min-w-0 sm:max-w-[55%] sm:basis-auto sm:truncate md:max-w-none"
          title={brandName}
        >
          {brandName}
        </Link>
        <ul className="flex items-center gap-4 text-sm sm:ml-auto sm:gap-6">
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
          <li className="min-h-[44px] flex items-center empty:hidden">
            <CustomerNotificationBell />
          </li>
          <li className="min-h-[44px] flex items-center empty:hidden">
            <CustomerAccountMenu />
          </li>
        </ul>
      </div>
    </nav>
  );
}
