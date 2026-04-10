"use client";

// apps/storefront/components/CartCountBadge.tsx
//
// Tiny client island showing the number of items in the cart.
// Mounted inside StorefrontNav wrapped in <Suspense> to avoid
// SSR hydration mismatch (cart is localStorage-only on client).

import { useCart } from "./CartProvider";

export function CartCountBadge() {
  const { count } = useCart();

  if (count === 0) return null;

  return (
    <span
      aria-label={`${count} item${count === 1 ? "" : "s"} in cart`}
      className="inline-flex h-5 min-w-5 items-center justify-center rounded-full bg-[color:var(--ink-900)] px-1.5 text-[11px] font-semibold leading-none text-[color:var(--paper-200)]"
    >
      {count > 99 ? "99+" : count}
    </span>
  );
}
