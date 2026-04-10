import { describe, it, expect, vi, beforeEach } from "vitest";

vi.mock("next/headers", () => ({
  headers: async () =>
    new Map<string, string>([
      ["x-session-user-id", "u1"],
      ["x-session-tenant-id", "t1"],
    ]),
}));

vi.mock("next/navigation", () => ({
  redirect: vi.fn(),
}));

vi.mock("next/cache", () => ({
  revalidatePath: vi.fn(),
}));

vi.mock("@/lib/api/marketplace-api", () => ({
  updateProduct: vi.fn(),
  createProduct: vi.fn(),
  deleteProduct: vi.fn(),
}));

import { updateProductAction } from "./actions";
import { updateProduct } from "@/lib/api/marketplace-api";

const mockedUpdate = updateProduct as unknown as ReturnType<typeof vi.fn>;

function baseInput() {
  return {
    title: "T",
    handle: "t",
    description: "",
    status: "draft" as const,
    price: "19.99",
    inventoryQuantity: "0",
    sku: "",
    categoryIds: [],
    options: [
      {
        name: "Size",
        values: [{ value: "M" }],
      },
    ],
    variants: [
      {
        key: "Size=M",
        price: "19.99",
        sku: "",
        stock: 0,
        weight: 0,
        optionValues: [{ optionName: "Size", value: "M" }],
      },
    ],
    media: [],
    removed_variant_ids: ["old-1"],
  };
}

describe("updateProductAction", () => {
  beforeEach(() => {
    mockedUpdate.mockReset();
  });

  it("forwards options, variants, media and removed_variant_ids to updateProduct", async () => {
    mockedUpdate.mockResolvedValue({
      ok: true,
      data: { id: "p1", title: "T" },
    });

    await updateProductAction("s1", "p1", "USD", baseInput());

    expect(mockedUpdate).toHaveBeenCalledTimes(1);
    const [storeId, productId, body] = mockedUpdate.mock.calls[0]!;
    expect(storeId).toBe("s1");
    expect(productId).toBe("p1");
    expect(body.options).toHaveLength(1);
    expect(body.variants).toHaveLength(1);
    expect(body.media).toEqual([]);
    expect(body.removed_variant_ids).toEqual(["old-1"]);
  });

  it("maps too_many_variants backend error to typed result", async () => {
    mockedUpdate.mockResolvedValue({
      ok: false,
      error: {
        code: "too_many_variants",
        message: "Too many variants",
      },
    });

    const result = await updateProductAction("s1", "p1", "USD", baseInput());
    expect(result.ok).toBe(false);
    expect(result.error?.code).toBe("too_many_variants");
  });

  it("maps option_value_in_use backend error to typed result", async () => {
    mockedUpdate.mockResolvedValue({
      ok: false,
      error: {
        code: "option_value_in_use",
        message: "Option value still referenced",
      },
    });

    const result = await updateProductAction("s1", "p1", "USD", baseInput());
    expect(result.ok).toBe(false);
    expect(result.error?.code).toBe("option_value_in_use");
  });
});
