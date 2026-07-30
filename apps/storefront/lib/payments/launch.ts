// Provider dispatch for embedded (JS-SDK) checkout.
//
// The checkout page and the /orders/[id] PaymentPrompt both need to answer
// "which SDK do I open for this provider, and is it embedded at all". Keeping
// that mapping here rather than in each caller means adding a gateway is one
// edit: without it, a new provider is silently treated as unsupported in one
// place and crashes in the other.

import { openRazorpayCheckout } from "./razorpay";
import { openCashfreeCheckout } from "./cashfree";

/**
 * Providers we can collect payment from with an in-page SDK. Hosted providers
 * (Stripe) are NOT here — they ship a payment_redirect_url instead and the
 * buyer leaves the site to pay.
 *
 * The checkout page uses this as a safety net: an order whose provider is
 * neither hosted nor listed here has nowhere for the buyer to pay, and must
 * fail loudly before the cart is cleared rather than leaving the merchant an
 * unpaid "completed" order.
 */
export const EMBEDDED_PROVIDERS = new Set(["razorpay", "cashfree"]);

export function isEmbeddedProvider(provider: string): boolean {
  return EMBEDDED_PROVIDERS.has(provider);
}

/** Everything any embedded launcher might need. Extra fields are ignored by
 * providers that don't use them (Cashfree needs no public key: its
 * payment_session_id already identifies the merchant and the order). */
export interface EmbeddedCheckoutContext {
  orderId: string;
  paymentToken: string;
  publicKey: string;
  amount: string;
  currencyCode: string;
  storeName: string;
  /** Gateway mode ("test" | "live") — Cashfree picks sandbox vs production from it. */
  mode?: string;
  customerName?: string;
  customerEmail?: string;
}

export interface EmbeddedCheckoutCallbacks {
  /** Payment succeeded AND was verified/confirmed server-side. */
  onSuccess: () => void;
  /** Buyer closed the sheet without paying — order stays reserved. */
  onDismiss: () => void;
  /** Script failed to load, sheet failed to open, or verification failed. */
  onError: (message: string) => void;
}

/**
 * Opens the payment sheet for `provider`. Both launchers only call onSuccess
 * after the server has independently confirmed the payment — Razorpay by
 * re-deriving the checkout HMAC, Cashfree by polling the gateway — so a caller
 * can treat onSuccess as "this order is paid" without further checks.
 */
export async function openEmbeddedCheckout(
  provider: string,
  ctx: EmbeddedCheckoutContext,
  callbacks: EmbeddedCheckoutCallbacks,
): Promise<void> {
  switch (provider) {
    case "razorpay":
      return openRazorpayCheckout(
        {
          orderId: ctx.orderId,
          paymentToken: ctx.paymentToken,
          publicKey: ctx.publicKey,
          amount: ctx.amount,
          currencyCode: ctx.currencyCode,
          storeName: ctx.storeName,
          customerName: ctx.customerName,
          customerEmail: ctx.customerEmail,
        },
        callbacks,
      );
    case "cashfree":
      return openCashfreeCheckout(
        {
          orderId: ctx.orderId,
          paymentToken: ctx.paymentToken,
          mode: ctx.mode,
        },
        callbacks,
      );
    default:
      callbacks.onError(
        `We can't open a payment window for ${provider}. Please contact the store — you have not been charged.`,
      );
  }
}
