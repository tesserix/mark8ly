"use client";

// apps/storefront/components/AccountMobileNav.tsx
//
// Horizontal scroll nav strip for /account/* pages on mobile.
// Mirrors the same links as AccountSidebar but renders only
// below the md breakpoint. Uses Paper · Ink · Moss tokens.

import Link from "next/link";
import { usePathname } from "next/navigation";

const NAV_ITEMS = [
  { href: "/account", label: "My account" },
  { href: "/account/orders", label: "Orders" },
  { href: "/account/loyalty", label: "Loyalty" },
  { href: "/account/gift-cards", label: "Gift cards" },
  { href: "/account/addresses", label: "Addresses" },
] as const;

export function AccountMobileNav() {
  const pathname = usePathname();

  return (
    <nav
      className="flex gap-2 overflow-x-auto border-b border-[color:var(--storefront-text,var(--ink-900))]/10 pb-3 md:hidden"
      aria-label="Account"
    >
      {NAV_ITEMS.map((item) => {
        const isActive =
          item.href === "/account"
            ? pathname === "/account"
            : pathname.startsWith(item.href);

        return (
          <Link
            key={item.href}
            href={item.href}
            className={[
              "shrink-0 rounded-[6px] px-3 py-1.5 text-sm whitespace-nowrap transition-colors",
              "focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))]",
              isActive
                ? "bg-[color:var(--storefront-background,var(--paper-200))] font-medium text-[color:var(--storefront-text,var(--ink-900))]"
                : "text-[color:var(--storefront-text,var(--ink-900))] opacity-60 hover:opacity-100 hover:bg-[color:var(--storefront-background,var(--paper-200))]",
            ].join(" ")}
            aria-current={isActive ? "page" : undefined}
          >
            {item.label}
          </Link>
        );
      })}
    </nav>
  );
}
