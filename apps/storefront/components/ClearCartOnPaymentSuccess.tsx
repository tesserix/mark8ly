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

import { useEffect } from "react";
import { useCart } from "@/components/CartProvider";

interface Props {
  /** True when the page was rendered with ?payment=success on the URL. */
  shouldClear: boolean;
}

export function ClearCartOnPaymentSuccess({ shouldClear }: Props) {
  const { clear } = useCart();
  useEffect(() => {
    if (shouldClear) clear();
  }, [shouldClear, clear]);
  return null;
}
