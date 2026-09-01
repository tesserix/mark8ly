import { describe, expect, it } from "vitest";

import { paymentReadinessFor } from "./payments-readiness";
import type { PaymentConfig } from "@/lib/api/settings-api";

const healthy = {
  provider: "stripe",
  is_active: true,
  mode: "test",
  webhook_secret: "whse****",
} as unknown as PaymentConfig;

const codes = (cfg: PaymentConfig | undefined) =>
  paymentReadinessFor(cfg).map((b) => b.code);

describe("paymentReadinessFor", () => {
  it("reports nothing when the gateway can take and record a payment", () => {
    expect(paymentReadinessFor(healthy)).toEqual([]);
  });

  it("reports a missing gateway", () => {
    expect(codes(undefined)).toEqual(["no_gateway"]);
  });

  it("reports an inactive gateway", () => {
    expect(codes({ ...healthy, is_active: false })).toEqual(["inactive"]);
  });

  // The exact production state: Stripe Active, payment taken, order stuck
  // Pending for over a month because no webhook was ever registered.
  it("reports a missing webhook secret", () => {
    expect(codes({ ...healthy, webhook_secret: "" })).toEqual([
      "no_webhook_secret",
    ]);
  });

  it("treats a whitespace-only webhook secret as missing", () => {
    expect(codes({ ...healthy, webhook_secret: "   " })).toEqual([
      "no_webhook_secret",
    ]);
  });

  it("reports every blocker at once rather than one at a time", () => {
    expect(codes({ ...healthy, is_active: false, webhook_secret: "" })).toEqual([
      "inactive",
      "no_webhook_secret",
    ]);
  });

  // The distinction worth drawing: one stops a sale, the other takes the
  // money and loses the record of it.
  it("marks the webhook gap as silently losing orders", () => {
    const [b] = paymentReadinessFor({ ...healthy, webhook_secret: "" });
    expect(b?.severity).toBe("silently_loses_orders");
  });

  it("marks an inactive gateway as blocking checkout", () => {
    const [b] = paymentReadinessFor({ ...healthy, is_active: false });
    expect(b?.severity).toBe("blocks_checkout");
  });
});
