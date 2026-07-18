// Shared Razorpay checkout launcher. Both the checkout page (open the widget
// immediately after the order is reserved — single click) and the /orders/[id]
// PaymentPrompt (the "Pay now" recovery path if the buyer dismissed the sheet)
// go through here so the load / open / verify logic lives in exactly one place.

const RZP_SCRIPT = "https://checkout.razorpay.com/v1/checkout.js";
const DEFAULT_ACCENT_HEX = "#2D4A2B";

declare global {
  interface Window {
    Razorpay?: new (options: Record<string, unknown>) => { open: () => void };
  }
}

/**
 * Razorpay's `theme.color` API accepts a hex string. Read the live
 * `--storefront-accent` CSS var off <body> (where the theme style is injected
 * from layout.tsx) so the merchant's brand accent flows through to the widget
 * instead of shipping every customer the hardcoded Mark8ly moss green. Falls
 * back to the default hex if the var resolves to something Razorpay wouldn't
 * accept.
 */
function resolveAccentHex(): string {
  if (typeof window === "undefined") return DEFAULT_ACCENT_HEX;
  const source = document.body ?? document.documentElement;
  const raw = getComputedStyle(source)
    .getPropertyValue("--storefront-accent")
    .trim();
  // Razorpay only reliably renders #RGB or #RRGGBB. Other formats (oklch,
  // color-mix, named colors) would silently render black.
  if (/^#(?:[0-9a-f]{3}|[0-9a-f]{6})$/i.test(raw)) return raw;
  return DEFAULT_ACCENT_HEX;
}

function loadRazorpay(): Promise<boolean> {
  if (typeof window === "undefined") return Promise.resolve(false);
  if (window.Razorpay) return Promise.resolve(true);
  return new Promise((resolve) => {
    // A previously-injected tag that already failed (CSP block, offline,
    // adblock) has ALREADY fired its load/error event. Attaching fresh
    // listeners to it would wait for an event that can never come again,
    // leaving the caller stuck forever with no error — so drop the dead tag
    // and retry from scratch instead.
    document
      .querySelectorAll<HTMLScriptElement>(`script[src="${RZP_SCRIPT}"]`)
      .forEach((tag) => tag.remove());

    const s = document.createElement("script");
    s.src = RZP_SCRIPT;
    s.async = true;
    s.onload = () => resolve(!!window.Razorpay);
    s.onerror = () => {
      s.remove();
      resolve(false);
    };
    document.head.appendChild(s);
  });
}

export interface RazorpayCheckoutContext {
  orderId: string;
  /** Razorpay order id returned by the checkout/reserve call. */
  paymentToken: string;
  publicKey: string;
  amount: string;
  currencyCode: string;
  storeName: string;
  customerName?: string;
  customerEmail?: string;
}

export interface RazorpayCheckoutCallbacks {
  /** Payment succeeded AND was verified server-side. */
  onSuccess: () => void;
  /** Buyer closed the sheet without paying — order stays reserved. */
  onDismiss: () => void;
  /** Script failed to load, widget failed to open, or verification failed. */
  onError: (message: string) => void;
}

/**
 * Loads Razorpay's checkout.js on demand and opens the widget for a
 * pre-reserved order. On widget success it POSTs the signed response to
 * /api/orders/{id}/verify-payment (which re-derives the HMAC and marks the
 * order paid via the same handler the webhook flow uses, so this works even
 * when no Razorpay webhook reaches the cluster), then invokes onSuccess.
 */
export async function openRazorpayCheckout(
  ctx: RazorpayCheckoutContext,
  callbacks: RazorpayCheckoutCallbacks,
): Promise<void> {
  const ok = await loadRazorpay();
  if (!ok || !window.Razorpay) {
    callbacks.onError(
      "Could not load the Razorpay widget. Check your connection and try again.",
    );
    return;
  }

  const amountPaise = Math.round(Number.parseFloat(ctx.amount) * 100);
  const options = {
    key: ctx.publicKey,
    amount: amountPaise,
    currency: ctx.currencyCode,
    name: ctx.storeName || "Storefront",
    order_id: ctx.paymentToken,
    prefill: {
      name: ctx.customerName ?? "",
      email: ctx.customerEmail ?? "",
    },
    handler: async (resp: {
      razorpay_order_id: string;
      razorpay_payment_id: string;
      razorpay_signature: string;
    }) => {
      try {
        const verifyRes = await fetch(
          `/api/orders/${ctx.orderId}/verify-payment`,
          {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(resp),
          },
        );
        if (!verifyRes.ok) {
          const body = await verifyRes.text().catch(() => "");
          callbacks.onError(
            `Payment verification failed (${verifyRes.status}). ${body}`.trim(),
          );
          return;
        }
        callbacks.onSuccess();
      } catch (e) {
        callbacks.onError(
          e instanceof Error ? e.message : "Unknown error verifying payment.",
        );
      }
    },
    modal: {
      ondismiss: () => callbacks.onDismiss(),
    },
    theme: { color: resolveAccentHex() },
  };

  try {
    new window.Razorpay(options).open();
  } catch (e) {
    callbacks.onError(
      e instanceof Error
        ? `Could not open the payment window: ${e.message}`
        : "Could not open the payment window. Please try again.",
    );
  }
}
