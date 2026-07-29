// The variant quick-edit PATCH — the wire body and the cache keys.
//
// `PATCH /products/:id/variants/:variantId` takes `UpdateVariantRequest`,
// whose every field is an OPTIONAL POINTER. A body that echoes back fields the
// merchant did not touch is how a phone silently overwrites an edit someone
// made in the web admin thirty seconds earlier: the phone's copy of the
// variant is as old as the list it was rendered from, and the server cannot
// tell "unchanged" from "set it back to this".
//
// So "the body carries exactly one key" is not tidiness — it is the whole
// safety property of this endpoint, and it is asserted on the object that
// actually reaches the api client, not on the helper alone.
import { renderHook, waitFor } from "@testing-library/react-native";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import React from "react";

// Typed parameters, not a bare `jest.fn()`: the assertions below read
// `mock.calls[0][2]` — the BODY — and an untyped mock gives that call a
// zero-length tuple type, so the one assertion that holds the whole safety
// property of this endpoint would not compile.
const mockUpdateVariant = jest.fn(
  (_productId: string, _variantId: string, _body: Record<string, unknown>) =>
    Promise.resolve({}),
);
jest.mock("@repo/mobile-shared/api/products", () => ({
  createProductsApi: () => ({ updateVariant: mockUpdateVariant }),
}));
jest.mock("@/lib/api-client", () => ({ useApiClient: () => ({}) }));

import { useQuickEditVariant, variantPatchBody } from "@/lib/admin-api/variant-quick-edit";

let queryClient: QueryClient;
let invalidate: jest.SpyInstance;

function wrapper({ children }: { children: React.ReactNode }) {
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
}

function invalidatedKeys(): string[] {
  return invalidate.mock.calls.map((call) => JSON.stringify(call[0]?.queryKey));
}

beforeEach(() => {
  jest.clearAllMocks();
  queryClient = new QueryClient({
    // gcTime 0 so no cache-eviction timer outlives the test and holds the
    // jest worker open.
    defaultOptions: {
      queries: { retry: false, gcTime: 0 },
      mutations: { retry: false, gcTime: 0 },
    },
  });
  invalidate = jest.spyOn(queryClient, "invalidateQueries");
});

afterEach(() => {
  queryClient.clear();
});

describe("variantPatchBody", () => {
  it("carries the price and NOTHING else", () => {
    const body = variantPatchBody("price", 19.99);
    expect(body).toEqual({ price: 19.99 });
    expect(Object.keys(body)).toHaveLength(1);
  });

  it("carries the stock quantity under its real wire name, and nothing else", () => {
    const body = variantPatchBody("inventory_quantity", 12);
    // NOT `stock` — UpdateVariantRequest has no such field, and a body using
    // it is discarded with a cheerful 200.
    expect(body).toEqual({ inventory_quantity: 12 });
    expect(Object.keys(body)).toHaveLength(1);
  });

  // Zero is a real price (a free sample) and a real stock level (sold out),
  // and both are the values most likely to be dropped by a truthiness check
  // somewhere between here and the wire.
  it("sends a zero rather than omitting it", () => {
    expect(variantPatchBody("price", 0)).toEqual({ price: 0 });
    expect(variantPatchBody("inventory_quantity", 0)).toEqual({ inventory_quantity: 0 });
  });
});

describe("useQuickEditVariant", () => {
  it("PATCHes only the field being changed", async () => {
    const { result } = renderHook(() => useQuickEditVariant(), { wrapper });
    result.current.mutate({
      productId: "p1",
      variantId: "v1",
      field: "price",
      value: 24.5,
    });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(mockUpdateVariant).toHaveBeenCalledWith("p1", "v1", { price: 24.5 });
    // The assertion that actually holds the safety property: whatever else
    // the hook grows, the body reaching the wire has ONE key.
    expect(Object.keys(mockUpdateVariant.mock.calls[0]![2]!)).toEqual(["price"]);
  });

  it("PATCHes stock without touching the price", async () => {
    const { result } = renderHook(() => useQuickEditVariant(), { wrapper });
    result.current.mutate({
      productId: "p1",
      variantId: "v1",
      field: "inventory_quantity",
      value: 7,
    });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(mockUpdateVariant).toHaveBeenCalledWith("p1", "v1", { inventory_quantity: 7 });
    expect(Object.keys(mockUpdateVariant.mock.calls[0]![2]!)).toEqual(["inventory_quantity"]);
  });

  // The Products list is keyed ["products", status, search]; the ["products"]
  // PREFIX is what reaches it, and it is the entire reason this screen needs
  // no optimistic update — the refetch is the authority on the new price.
  // ["product", id] is NOT under that prefix, so the detail screen needs its
  // own line.
  it("invalidates the products list prefix and the product detail", async () => {
    const { result } = renderHook(() => useQuickEditVariant(), { wrapper });
    result.current.mutate({ productId: "p1", variantId: "v1", field: "price", value: 1 });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(invalidatedKeys()).toEqual(
      expect.arrayContaining(['["products"]', '["product","p1"]']),
    );
  });

  // Not ["dashboard"]: its only product-shaped blocks are `top_products`
  // (sales-derived) and `low_stock`, and neither is refreshed usefully by a
  // single variant PATCH the merchant is watching. A third invalidation here
  // would be a refetch of the heaviest payload in the app for nothing.
  it("invalidates exactly those two keys — no speculative third", async () => {
    const { result } = renderHook(() => useQuickEditVariant(), { wrapper });
    result.current.mutate({ productId: "p1", variantId: "v1", field: "price", value: 1 });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(invalidatedKeys()).toEqual(['["products"]', '["product","p1"]']);
  });
});
