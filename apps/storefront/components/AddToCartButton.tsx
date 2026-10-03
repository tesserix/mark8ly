"use client";

// apps/storefront/components/AddToCartButton.tsx
//
// Client island that replaces the disabled "coming soon" stub on the
// product detail page. Receives product + variant data as props from
// the server component. On click, calls useCart().add().

import { useCallback } from "react";
import Link from "next/link";
import { useCart } from "./CartProvider";
import {
  lineKey,
  type CartItemPersonalisation,
  type CartItemTaxCategory,
} from "@/lib/cart";

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
  // Tax classification copied from the product so the cart item carries
  // it through to checkout without a re-fetch.
  taxCode?: string;
  taxRateOverride?: string;
  taxCategory?: CartItemTaxCategory;
  // Shipping snapshot — variant weight + package dims so /shipping-rates
  // can quote real numbers instead of using the server's default envelope.
  weightGrams?: number;
  lengthCm?: number;
  widthCm?: number;
  heightCm?: number;
  /**
   * The buyer's answers (#965), already validated by the parent. Present
   * only on products that ask for something.
   */
  personalisation?: CartItemPersonalisation[];
  /**
   * Blocks the add when required fields are unanswered. The parent owns
   * the rule; this control only reports it.
   */
  blockedReason?: string;
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
  taxCode,
  taxRateOverride,
  taxCategory,
  weightGrams,
  lengthCm,
  widthCm,
  heightCm,
  personalisation,
  blockedReason,
}: AddToCartButtonProps) {
  const { add, items, updateQty } = useCart();

  // Drive the control off real cart state rather than a transient "just
  // added" flag: the quantity then survives navigation and matches the
  // cart badge, instead of reverting to "Add to cart" after 1.5s and
  // leaving the shopper unsure whether the click registered.
  // The identity of the line THIS button would add — variant plus the
  // buyer's current answers (#964/#965).
  //
  // Passing the answers in is what makes the quantity stepper find the
  // personalised line rather than a plain one of the same variant. Change
  // an answer and this key changes with it, which is correct: that is a
  // different line, and the stepper should go back to "Add to cart".
  const thisLineKey = lineKey({ productId, variantId, personalisation });
  const inCart = items.find((i) => lineKey(i) === thisLineKey);

  const handleClick = useCallback(() => {
    add({
      productId,
      variantId,
      handle,
      title,
      priceAmount,
      currencyCode,
      qty: 1,
      imageUrl,
      taxCode,
      taxRateOverride,
      taxCategory,
      weightGrams,
      lengthCm,
      widthCm,
      heightCm,
      personalisation,
    });
    // Lazy-import to keep the button dep-light on SSR.
    import("@/lib/toast").then(({ toast }) =>
      toast({
        title: `${title} added to cart`,
        tone: "success",
        // Without this the shopper has to hunt for the cart in the nav —
        // the toast is the one moment we know they want to go there.
        action: { label: "View cart", href: "/cart" },
      }),
    );
  }, [
    add,
    productId,
    variantId,
    handle,
    title,
    priceAmount,
    currencyCode,
    imageUrl,
    taxCode,
    taxRateOverride,
    taxCategory,
    weightGrams,
    lengthCm,
    widthCm,
    heightCm,
  ]);

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

  // A product that asks for something the buyer has not supplied yet.
  // Shown as a disabled button with the reason ON it, rather than a
  // silent no-op, so the shopper knows what is missing before hunting
  // for it — the form marks the individual fields.
  if (blockedReason) {
    return (
      <button
        type="button"
        disabled
        aria-describedby="personalisation-blocked"
        className="mt-2 inline-flex w-fit items-center gap-2 rounded-md bg-[color:var(--storefront-accent,var(--ink-900))] px-6 py-3 text-sm text-[color:var(--storefront-on-accent,var(--paper-200))] opacity-40 cursor-not-allowed"
      >
        {blockedReason}
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

  // Already in the cart → hand over a quantity stepper so the shopper can
  // adjust in place instead of clicking "Add to cart" repeatedly and
  // guessing how many they now have.
  if (inCart) {
    const setQty = (next: number) => updateQty(thisLineKey, next);
    return (
      <div className="mt-2 flex w-fit items-center gap-3">
        <div className="inline-flex items-center rounded-md border border-[color:var(--storefront-text,var(--ink-900))]/15">
          <button
            type="button"
            onClick={() => setQty(inCart.qty - 1)}
            aria-label={
              inCart.qty === 1 ? `Remove ${title} from cart` : `Decrease ${title} quantity`
            }
            className="px-4 py-3 text-sm leading-none transition-opacity hover:opacity-60 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))]"
          >
            −
          </button>
          <span
            aria-live="polite"
            className="min-w-[2.5rem] text-center text-sm tabular-nums"
          >
            {inCart.qty}
          </span>
          <button
            type="button"
            onClick={() => setQty(inCart.qty + 1)}
            aria-label={`Increase ${title} quantity`}
            className="px-4 py-3 text-sm leading-none transition-opacity hover:opacity-60 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))]"
          >
            +
          </button>
        </div>
        <Link
          href="/cart"
          className="text-sm underline underline-offset-4 hover:opacity-70 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))]"
        >
          View cart
        </Link>
      </div>
    );
  }

  return (
    <button
      type="button"
      onClick={handleClick}
      className="mt-2 inline-flex w-fit items-center gap-2 rounded-md bg-[color:var(--storefront-accent,var(--ink-900))] px-6 py-3 text-sm text-[color:var(--storefront-on-accent,var(--paper-200))] transition-opacity hover:opacity-90 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))]"
    >
      Add to cart
    </button>
  );
}
