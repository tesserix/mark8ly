import { afterEach, describe, expect, it, vi } from "vitest";

import { fetchReceipt, resendReceipt } from "./receipt-api";

afterEach(() => vi.unstubAllGlobals());

function respond(status: number, body: unknown) {
  const f = vi.fn().mockResolvedValue(new Response(status === 202 ? null : JSON.stringify(body), { status }));
  vi.stubGlobal("fetch", f);
  return f;
}

describe("fetchReceipt", () => {
  it("reads the receipt by token on the store", async () => {
    const f = respond(200, { order_number: "M-1" });
    expect(await fetchReceipt("bondi", "tok_1")).toEqual({ order_number: "M-1" });
    expect(String(f.mock.calls[0]![0])).toMatch(/\/storefront\/stores\/bondi\/receipts\/tok_1$/);
    expect(f.mock.calls[0]![1]).toMatchObject({ cache: "no-store" });
  });

  it("is null for an unknown token", async () => {
    respond(404, { error: "not_found" });
    expect(await fetchReceipt("bondi", "x")).toBeNull();
  });
});

describe("resendReceipt", () => {
  it("posts to resend and reports whether it was accepted", async () => {
    const f = respond(202, null);
    expect(await resendReceipt("bondi", "tok_1")).toBe(true);
    expect(f.mock.calls[0]![1]).toMatchObject({ method: "POST" });
    respond(404, {});
    expect(await resendReceipt("bondi", "tok_1")).toBe(false);
  });
});
