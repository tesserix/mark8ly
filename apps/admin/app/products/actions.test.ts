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
  copyProduct: vi.fn(),
  bulkProductAction: vi.fn(),
}));

import {
  updateProductAction,
  copyProductAction,
  bulkArchiveAction,
  bulkUnarchiveAction,
  bulkPublishAction,
  bulkUnpublishAction,
  bulkDeleteAction,
  bulkAssignCategoryAction,
} from "./actions";
import { updateProduct, copyProduct, bulkProductAction } from "@/lib/api/marketplace-api";

const mockedUpdate = updateProduct as unknown as ReturnType<typeof vi.fn>;
const mockedCopy = copyProduct as unknown as ReturnType<typeof vi.fn>;
const mockedBulk = bulkProductAction as unknown as ReturnType<typeof vi.fn>;

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

describe("copyProductAction", () => {
  beforeEach(() => {
    mockedCopy.mockReset();
  });

  it("sends target_store_id and copy_media to copyProduct", async () => {
    mockedCopy.mockResolvedValue({
      ok: true,
      data: { new_product_id: "p2", new_store_id: "s2" },
    });

    const result = await copyProductAction("s1", "p1", {
      targetStoreId: "s2",
      copyMedia: true,
    });

    expect(result.ok).toBe(true);
    expect(mockedCopy).toHaveBeenCalledTimes(1);
    const [storeId, productId, body] = mockedCopy.mock.calls[0]!;
    expect(storeId).toBe("s1");
    expect(productId).toBe("p1");
    expect(body).toEqual({ target_store_id: "s2", copy_media: true });
  });

  it("returns error on copy failure", async () => {
    mockedCopy.mockResolvedValue({
      ok: false,
      error: { code: "forbidden", message: "Not allowed" },
    });

    const result = await copyProductAction("s1", "p1", {
      targetStoreId: "s2",
      copyMedia: false,
    });
    expect(result.ok).toBe(false);
    expect(result.error?.code).toBe("forbidden");
  });
});

describe("bulk actions", () => {
  beforeEach(() => {
    mockedBulk.mockReset();
  });

  it("bulkArchiveAction sends 'archive' action", async () => {
    mockedBulk.mockResolvedValue({
      ok: true,
      data: { results: [{ id: "p1", status: "ok" }] },
    });

    const result = await bulkArchiveAction("s1", ["p1"]);
    expect(result.ok).toBe(true);
    const [storeId, body] = mockedBulk.mock.calls[0]!;
    expect(storeId).toBe("s1");
    expect(body.action).toBe("archive");
    expect(body.product_ids).toEqual(["p1"]);
  });

  it("bulkUnarchiveAction sends 'unarchive' action", async () => {
    mockedBulk.mockResolvedValue({
      ok: true,
      data: { results: [{ id: "p1", status: "ok" }] },
    });

    await bulkUnarchiveAction("s1", ["p1"]);
    expect(mockedBulk.mock.calls[0]![1].action).toBe("unarchive");
  });

  it("bulkPublishAction sends 'publish' action", async () => {
    mockedBulk.mockResolvedValue({
      ok: true,
      data: { results: [{ id: "p1", status: "ok" }] },
    });

    await bulkPublishAction("s1", ["p1"]);
    expect(mockedBulk.mock.calls[0]![1].action).toBe("publish");
  });

  it("bulkUnpublishAction sends 'unpublish' action", async () => {
    mockedBulk.mockResolvedValue({
      ok: true,
      data: { results: [{ id: "p1", status: "ok" }] },
    });

    await bulkUnpublishAction("s1", ["p1"]);
    expect(mockedBulk.mock.calls[0]![1].action).toBe("unpublish");
  });

  it("bulkDeleteAction sends 'delete' action", async () => {
    mockedBulk.mockResolvedValue({
      ok: true,
      data: { results: [{ id: "p1", status: "ok" }] },
    });

    await bulkDeleteAction("s1", ["p1"]);
    expect(mockedBulk.mock.calls[0]![1].action).toBe("delete");
  });

  it("bulkAssignCategoryAction sends 'assign_category' with category_ids param", async () => {
    mockedBulk.mockResolvedValue({
      ok: true,
      data: { results: [{ id: "p1", status: "ok" }] },
    });

    await bulkAssignCategoryAction("s1", ["p1"], ["c1", "c2"]);
    const body = mockedBulk.mock.calls[0]![1];
    expect(body.action).toBe("assign_category");
    expect(body.params).toEqual({ category_ids: ["c1", "c2"] });
  });

  it("rejects when product_ids exceeds 100", async () => {
    const ids = Array.from({ length: 101 }, (_, i) => `p${i}`);
    const result = await bulkArchiveAction("s1", ids);
    expect(result.ok).toBe(false);
    expect(result.error?.code).toBe("validation_failed");
    expect(mockedBulk).not.toHaveBeenCalled();
  });
});
