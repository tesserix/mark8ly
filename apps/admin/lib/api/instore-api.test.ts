import { afterEach, describe, expect, it, vi } from "vitest";

import { getInStorePermissions, getMySalesToday, quoteInStoreSale, recordInStoreSale } from "./instore-api";

const session = { userId: "u-1", tenantId: "t-1" };
const body = {
  idempotency_key: "k-1",
  lines: [{ variant_id: "v1", quantity: 1 }],
  payment_method: "card_external",
  expected_total: "44.00",
};

function respond(status: number, json: unknown) {
  const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(json), { status }));
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

afterEach(() => vi.unstubAllGlobals());

describe("recordInStoreSale", () => {
  it("201 is a new sale", async () => {
    const fetchMock = respond(201, { id: "o-1", order_number: "BON-1001", grand_total: "44.00" });
    expect(await recordInStoreSale("s1", body, session)).toEqual({
      ok: true, orderId: "o-1", orderNumber: "BON-1001", grandTotal: "44.00", reused: false,
    });
    const [url, init] = fetchMock.mock.calls[0]!;
    expect(String(url)).toMatch(/\/api\/v1\/admin\/stores\/s1\/in-store\/sales$/);
    const headers = init.headers as Record<string, string>;
    expect(headers["X-User-Id"]).toBe("u-1");
    expect(headers["X-Tenant-Id"]).toBe("t-1");
    expect(JSON.parse(init.body).idempotency_key).toBe("k-1");
  });

  it("200 is the same sale replayed", async () => {
    respond(200, { id: "o-1", order_number: "BON-1001", grand_total: "44.00" });
    expect(await recordInStoreSale("s1", body, session)).toMatchObject({ ok: true, reused: true });
  });

  it("409 out_of_stock names the variant", async () => {
    respond(409, { error: "out_of_stock", variant_id: "v1" });
    expect(await recordInStoreSale("s1", body, session)).toEqual({ ok: false, kind: "out_of_stock", variantId: "v1" });
  });

  it("409 total_changed carries the new total", async () => {
    respond(409, { error: "total_changed", grand_total: "46.00" });
    expect(await recordInStoreSale("s1", body, session)).toEqual({ ok: false, kind: "total_changed", grandTotal: "46.00" });
  });

  it("400 is rejected with the server message", async () => {
    respond(400, { error: "validation_failed", message: "cash tendered must cover the total" });
    expect(await recordInStoreSale("s1", body, session)).toEqual({
      ok: false, kind: "rejected", message: "cash tendered must cover the total",
    });
  });

  it("a network failure is unreachable", async () => {
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new TypeError("fetch failed")));
    expect(await recordInStoreSale("s1", body, session)).toEqual({ ok: false, kind: "unreachable" });
  });

  it("503 is unreachable: the outcome is unknown", async () => {
    respond(503, { error: "unavailable" });
    expect(await recordInStoreSale("s1", body, session)).toEqual({ ok: false, kind: "unreachable" });
  });
});

describe("quoteInStoreSale", () => {
  it("returns the server's totals", async () => {
    respond(200, { subtotal: "40.00", tax_total: "4.00", discount_total: "0.00", grand_total: "44.00", currency_code: "AUD" });
    const res = await quoteInStoreSale("s1", { lines: body.lines }, session);
    expect(res).toEqual({
      ok: true,
      data: { subtotal: "40.00", tax_total: "4.00", discount_total: "0.00", grand_total: "44.00", currency_code: "AUD" },
    });
  });
});

describe("getInStorePermissions", () => {
  it("returns the server's can_discount", async () => {
    const fetchMock = respond(200, { can_discount: true });
    expect(await getInStorePermissions("s1", session)).toEqual({ canDiscount: true });
    expect(String(fetchMock.mock.calls[0]![0])).toMatch(/\/in-store\/permissions$/);
  });

  it("denies discounts when it can't ask", async () => {
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new TypeError("fetch failed")));
    expect(await getInStorePermissions("s1", session)).toEqual({ canDiscount: false });
  });
});

describe("getMySalesToday", () => {
  it("returns the caller's sales", async () => {
    const sale = { id: "o-1", order_number: "M-1", grand_total: "28.00", currency_code: "AUD", payment_method: "cash", placed_at: "2026-11-15T01:00:00Z" };
    const fetchMock = respond(200, { data: [sale] });
    expect(await getMySalesToday("s1", session)).toEqual([sale]);
    expect(String(fetchMock.mock.calls[0]![0])).toMatch(/\/in-store\/sales$/);
  });

  it("is null when the list can't be loaded, so the screen can say so", async () => {
    respond(500, {});
    expect(await getMySalesToday("s1", session)).toBeNull();
  });
});
