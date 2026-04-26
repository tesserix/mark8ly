"use client";

// Tiny client island for /orders/[id]. When the buyer lands here from
// Stripe Checkout's success_url (?payment=success), the order has been
// reserved and either is paid (webhook beat us) or about to be paid. In
// either case the cart should be empty — keeping the items in it leads
// to "I just paid for these but they're still in my cart" confusion and
// risks accidental double-purchase if the buyer hits Pay again.
//
// We deliberately don't clear in the /checkout submit handler when a
// hosted-redirect URL is returned: a buyer who cancels mid-flow at
// Stripe should land back on the store with their cart intact. The
// success bounce is the right place to commit the clear.
//
// Implementation note — there's a hydration race between this island
// and CartProvider's HYDRATE effect. Child effects run before parent
// effects, so a naive `clear()` here gets overridden when CartProvider
// then re-hydrates from localStorage. Fix: purge the localStorage key
// directly AND dispatch the in-memory clear. localStorage purge is
// synchronous and survives the HYDRATE round-trip.

import { useEffect } from "react";
import { useCart } from "@/components/CartProvider";

interface Props {
  /** True when the page was rendered with ?payment=success on the URL. */
  shouldClear: boolean;
  /** Store slug — used to purge the localStorage cart key directly. */
  storeSlug: string;
}

export function ClearCartOnPaymentSuccess({ shouldClear, storeSlug }: Props) {
  const { clear } = useCart();
  useEffect(() => {
    if (!shouldClear) return;
    // 1. Purge persistent storage so CartProvider's HYDRATE effect (which
    //    runs after this child effect on the same mount cycle) reads an
    //    empty cart and doesn't restore the items we're trying to drop.
    if (typeof window !== "undefined") {
      try {
        window.localStorage.removeItem(`mark8ly.cart.${storeSlug}`);
      } catch {
        /* localStorage blocked / full — fall through to in-memory clear. */
      }
    }
    // 2. Clear the in-memory state so any open tab reflects the change
    //    without a refresh.
    clear();
  }, [shouldClear, storeSlug, clear]);
  return null;
}
