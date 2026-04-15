"use client";

// apps/storefront/components/AddToCartButton.tsx
//
// Client island that replaces the disabled "coming soon" stub on the
// product detail page. Receives product + variant data as props from
// the server component. On click, calls useCart().add().

import { useState } from "react";
import { useCart } from "./CartProvider";

export interface AddToCartButtonProps {
  productId: string;
  variantId: string;
  handle: string;
  title: string;
  priceAmount: string;
  currencyCode: string;
  imageUrl?: string;
  inStock: boolean;
  disabled?: boolean;
  disabledReason?: string;
}

export function AddToCartButton({
  productId,
  variantId,
  handle,
  title,
  priceAmount,
  currencyCode,
  imageUrl,
  inStock,
  disabled = false,
  disabledReason,
}: AddToCartButtonProps) {
  const { add } = useCart();
  const [justAdded, setJustAdded] = useState(false);

  function handleClick() {
    add({
      productId,
      variantId,
      handle,
      title,
      priceAmount,
      currencyCode,
      qty: 1,
      imageUrl,
    });
    setJustAdded(true);
    setTimeout(() => setJustAdded(false), 1500);
    // Lazy-import to keep the button dep-light on SSR.
    import("@/lib/toast").then(({ toast }) =>
      toast({ title: `${title} added to cart`, tone: "success" }),
    );
  }

  if (!inStock) {
    return (
      <button
        type="button"
        disabled
        className="mt-2 inline-flex w-fit items-center gap-2 rounded-md bg-[color:var(--storefront-accent,var(--ink-900))] px-6 py-3 text-sm text-[color:var(--storefront-on-accent,var(--paper-200))] opacity-40 cursor-not-allowed"
      >
        Out of stock
      </button>
    );
  }

  if (disabled) {
    return (
      <button
        type="button"
        disabled
        className="mt-2 inline-flex w-fit items-center gap-2 rounded-md bg-[color:var(--storefront-accent,var(--ink-900))] px-6 py-3 text-sm text-[color:var(--storefront-on-accent,var(--paper-200))] opacity-40 cursor-not-allowed"
      >
        {disabledReason ?? "Add to cart"}
      </button>
    );
  }

  return (
    <button
      type="button"
      onClick={handleClick}
      className="mt-2 inline-flex w-fit items-center gap-2 rounded-md bg-[color:var(--storefront-accent,var(--ink-900))] px-6 py-3 text-sm text-[color:var(--storefront-on-accent,var(--paper-200))] transition-opacity hover:opacity-90 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))]"
    >
      {justAdded ? "Added ✓" : "Add to cart"}
    </button>
  );
}
