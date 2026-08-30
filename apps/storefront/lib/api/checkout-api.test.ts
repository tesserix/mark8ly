import { afterEach, describe, expect, it, vi } from "vitest";

import { isOutOfStock, submitCheckout } from "./checkout-api";

// #232 — a sold-out line must be distinguishable from a site fault.
//
// submitCheckout used to return null for EVERY non-2xx, so a 409 naming the
// variant arrived at the page identical to a 500. The shopper was told
// "something went wrong" for a situation they could resolve by removing one
// line.

const originalFetch = globalThis.fetch;

afterEach(() => {
  globalThis.fetch = originalFetch;
  vi.restoreAllMocks();
});

function mockFetch(status: number, body: unknown) {
  globalThis.fetch = vi.fn().mockResolvedValue({
    ok: status >= 200 && status < 300,
    status,
    json: async () => body,
  }) as unknown as typeof fetch;
}

const requestBody = { idempotency_key: "k", items: [] } as never;

describe("submitCheckout out-of-stock handling", () => {
  it("reports a 409 out_of_stock with the variant so the page can name the item", async () => {
    mockFetch(409, { error: "out_of_stock", variant_id: "v-123" });

    const result = await submitCheckout("store", requestBody);

    expect(isOutOfStock(result)).toBe(true);
    expect(isOutOfStock(result) && result.variantId).toBe("v-123");
  });

  // The status is the fact that matters. Losing it because the body did not
  // parse would drop the shopper back into the generic error they cannot act
  // on — which is the bug this exists to fix.
  it("still reports out-of-stock when the 409 body cannot be parsed", async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: false,
      status: 409,
      json: async () => {
        throw new Error("not json");
      },
    }) as unknown as typeof fetch;

    const result = await submitCheckout("store", requestBody);

    expect(isOutOfStock(result)).toBe(true);
  });

  it("does not treat other failures as out-of-stock", async () => {
    mockFetch(500, { error: "internal_error" });

    const result = await submitCheckout("store", requestBody);

    expect(result).toBeNull();
    expect(isOutOfStock(result)).toBe(false);
  });

  it("returns the order on success", async () => {
    mockFetch(201, { order_id: "o-1", order_number: "M-1", payment_token: "t" });

    const result = await submitCheckout("store", requestBody);

    expect(isOutOfStock(result)).toBe(false);
    expect(result && "order_id" in result && result.order_id).toBe("o-1");
  });
});
