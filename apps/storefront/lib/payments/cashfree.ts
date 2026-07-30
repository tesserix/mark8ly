// Shared Cashfree checkout launcher, the sibling of razorpay.ts. Both the
// checkout page (open the sheet immediately after the order is reserved —
// single click) and the /orders/[id] PaymentPrompt ("Pay now" recovery if the
// buyer dismissed it) go through here so the load / open / confirm logic lives
// in exactly one place.
//
// The one structural difference from the Razorpay launcher: Cashfree's SDK
// hands back NO signed receipt. Razorpay's `handler` callback returns
// {order_id, payment_id, signature} which the server re-derives an HMAC over;
// Cashfree's checkout promise resolves with only a coarse outcome hint. So
// instead of POSTing a client value to be verified, we POST nothing and let the
// server ask Cashfree what was actually captured — see
// /api/orders/{id}/confirm-payment and the backend's payment_confirm.go. The
// client is never trusted with the outcome at all, which is why the resolved
// `paymentDetails` below is treated as a nudge to go confirm rather than as
// proof of payment.

const CASHFREE_SCRIPT = "https://sdk.cashfree.com/js/v3/cashfree.js";

/**
 * The subset of Cashfree's v3 SDK surface we depend on. `Cashfree()` is a
 * factory (not a constructor) and `mode` selects sandbox vs production —
 * getting that wrong points a live key at the sandbox, where payments appear
 * to succeed while capturing nothing.
 */
interface CashfreeCheckoutResult {
  error?: { message?: string };
  paymentDetails?: { paymentMessage?: string };
  redirect?: boolean;
}

interface CashfreeInstance {
  checkout: (options: {
    paymentSessionId: string;
    redirectTarget?: "_self" | "_blank" | "_modal";
  }) => Promise<CashfreeCheckoutResult>;
}

declare global {
  interface Window {
    Cashfree?: (config: { mode: "sandbox" | "production" }) => CashfreeInstance;
  }
}

function loadCashfree(): Promise<boolean> {
  if (typeof window === "undefined") return Promise.resolve(false);
  if (window.Cashfree) return Promise.resolve(true);
  return new Promise((resolve) => {
    // A previously-injected tag that already failed (CSP block, offline,
    // adblock) has ALREADY fired its load/error event. Attaching fresh
    // listeners to it would wait for an event that can never come again,
    // leaving the caller stuck forever with no error — so drop the dead tag
    // and retry from scratch instead. Same reasoning as razorpay.ts.
    document
      .querySelectorAll<HTMLScriptElement>(`script[src="${CASHFREE_SCRIPT}"]`)
      .forEach((tag) => tag.remove());

    const s = document.createElement("script");
    s.src = CASHFREE_SCRIPT;
    s.async = true;
    s.onload = () => resolve(!!window.Cashfree);
    s.onerror = () => {
      s.remove();
      resolve(false);
    };
    document.head.appendChild(s);
  });
}

export interface CashfreeCheckoutContext {
  orderId: string;
  /** Cashfree payment_session_id returned by the checkout/reserve call. */
  paymentToken: string;
  /**
   * Gateway mode from the payment-methods endpoint ("test" | "live").
   * Cashfree selects its environment by SDK mode + host, not by key prefix,
   * so this has to be threaded through rather than inferred from the key.
   */
  mode?: string;
}

export interface CashfreeCheckoutCallbacks {
  /** Payment succeeded AND was confirmed server-side. */
  onSuccess: () => void;
  /** Buyer closed the sheet without paying — order stays reserved. */
  onDismiss: () => void;
  /** Script failed to load, sheet failed to open, or confirmation failed. */
  onError: (message: string) => void;
}

/**
 * Asks the server whether Cashfree actually captured this order's payment.
 * Returns "ok" only when the gateway confirmed a capture; "pending" means the
 * buyer has not paid (a dismissed sheet), which is not an error.
 */
async function confirmPayment(
  orderId: string,
): Promise<{ status: "ok" | "pending" } | { error: string }> {
  try {
    const res = await fetch(`/api/orders/${orderId}/confirm-payment`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: "{}",
    });
    if (!res.ok) {
      const body = await res.text().catch(() => "");
      return {
        error: `Payment confirmation failed (${res.status}). ${body}`.trim(),
      };
    }
    const data = (await res.json()) as { status?: string };
    return { status: data.status === "ok" ? "ok" : "pending" };
  } catch (e) {
    return {
      error: e instanceof Error ? e.message : "Unknown error confirming payment.",
    };
  }
}

/**
 * Loads Cashfree's v3 SDK on demand and opens the checkout modal for a
 * pre-reserved order. Whatever the SDK reports, the outcome is decided by the
 * server-side confirm call, so a buyer cannot talk their way into a paid order
 * and a genuine payment is not lost to a flaky SDK callback.
 */
export async function openCashfreeCheckout(
  ctx: CashfreeCheckoutContext,
  callbacks: CashfreeCheckoutCallbacks,
): Promise<void> {
  const ok = await loadCashfree();
  if (!ok || !window.Cashfree) {
    callbacks.onError(
      "Could not load the Cashfree payment window. Check your connection and try again.",
    );
    return;
  }
  if (!ctx.paymentToken) {
    callbacks.onError(
      "This order has no active Cashfree payment session. Please contact the store.",
    );
    return;
  }

  let result: CashfreeCheckoutResult;
  try {
    const cashfree = window.Cashfree({
      mode: ctx.mode === "test" ? "sandbox" : "production",
    });
    result = await cashfree.checkout({
      paymentSessionId: ctx.paymentToken,
      // "_modal" keeps the buyer on our page, matching the Razorpay widget's
      // feel. Bank/UPI redirects still leave and come back inside the modal.
      redirectTarget: "_modal",
    });
  } catch (e) {
    callbacks.onError(
      e instanceof Error
        ? `Could not open the payment window: ${e.message}`
        : "Could not open the payment window. Please try again.",
    );
    return;
  }

  // A redirect flow never resolves here in a meaningful way — the page has
  // navigated and /orders/[id] picks up from the webhook or a later confirm.
  if (result?.redirect) return;

  // An SDK-reported error still gets a confirm: the modal can report a
  // failure for an attempt that in fact settled (a UPI collect that lands
  // after the sheet gave up), and quietly leaving that order unpaid is worse
  // than one extra API call. Only if the gateway also says "not paid" do we
  // surface the SDK's message.
  const confirmed = await confirmPayment(ctx.orderId);
  if ("error" in confirmed) {
    callbacks.onError(confirmed.error);
    return;
  }
  if (confirmed.status === "ok") {
    callbacks.onSuccess();
    return;
  }
  if (result?.error?.message) {
    callbacks.onError(result.error.message);
    return;
  }
  callbacks.onDismiss();
}
