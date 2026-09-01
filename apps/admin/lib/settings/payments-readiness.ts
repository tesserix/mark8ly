import type { PaymentConfig } from "@/lib/api/settings-api";

/**
 * Why a store cannot actually take money, or cannot record that it did.
 *
 * The payments page shows a provider as `Active` on the strength of an API
 * key alone. That is not the same as working, and the gap is expensive:
 *
 *   • **No webhook signing secret** is the quiet one. Stripe accepts the
 *     payment, the shopper is charged, and the order sits `Pending`
 *     forever because nothing tells us the payment succeeded. Worse, the
 *     webhook handler answers 200 even when it REJECTS an unverifiable
 *     event (so Stripe stops retrying garbage) — so Stripe's dashboard
 *     reports delivery as successful while nothing is processed. Silent at
 *     both ends. the-bondi-store took a real payment on 2026-09-01 and the
 *     order never left `Pending`; a July order was stuck the same way.
 *
 *   • **Inactive** means the gateway is never offered at checkout, so a
 *     shopper reaches payment and finds no method.
 *
 * Auto-provisioning now registers the webhook on save, so a new config
 * should never lack a secret. This is what tells a merchant configured
 * BEFORE that — or one whose provisioning failed — which state they are in.
 */
export interface PaymentBlocker {
  code: "no_gateway" | "inactive" | "no_webhook_secret";
  message: string;
  /**
   * Blockers differ in kind: a shopper can't pay at all, versus the store
   * takes money it never records. Both matter; only the second is silent.
   */
  severity: "blocks_checkout" | "silently_loses_orders";
}

export function paymentReadinessFor(
  cfg: PaymentConfig | undefined,
): PaymentBlocker[] {
  if (!cfg) {
    return [
      {
        code: "no_gateway",
        message: "No credentials yet — customers cannot pay until this is configured.",
        severity: "blocks_checkout",
      },
    ];
  }

  const blockers: PaymentBlocker[] = [];

  if (!cfg.is_active) {
    blockers.push({
      code: "inactive",
      message:
        "This gateway is inactive, so it is never offered at checkout. Tick “Active” to enable it.",
      severity: "blocks_checkout",
    });
  }

  if (!cfg.webhook_secret?.trim()) {
    blockers.push({
      code: "no_webhook_secret",
      message:
        "No webhook signing secret. Payments will succeed but orders will stay unpaid, because nothing tells your store the payment went through. Re-save your API key to register the webhook automatically.",
      severity: "silently_loses_orders",
    });
  }

  return blockers;
}
