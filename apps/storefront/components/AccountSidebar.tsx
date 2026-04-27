"use client";

// apps/storefront/components/AccountSidebar.tsx
//
// Sidebar navigation for /account/* pages. Highlights the active
// link based on the current pathname. Paper · Ink · Moss tokens.

import Link from "next/link";
import { usePathname } from "next/navigation";

const NAV_ITEMS = [
  { href: "/account", label: "My account" },
  { href: "/account/orders", label: "Orders" },
  { href: "/account/loyalty", label: "Loyalty" },
  { href: "/account/gift-cards", label: "Gift cards" },
  { href: "/account/addresses", label: "Addresses" },
  { href: "/account/security", label: "Security" }, // Phase 3
  { href: "/account/tickets", label: "Support" },
] as const;

export function AccountSidebar() {
  const pathname = usePathname();

  return (
    <nav aria-label="Account" className="w-full space-y-1">
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
              "block rounded-[6px] px-3 py-2 text-sm transition-colors",
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
