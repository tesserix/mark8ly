import { describe, it, expect } from "vitest";
import { bulkActionSchema, MAX_BULK_IDS } from "./bulk-action";

describe("bulkActionSchema", () => {
  it("accepts 1 to 100 product ids", () => {
    const ids = Array.from({ length: 50 }, (_, i) => `p${i}`);
    const result = bulkActionSchema.safeParse({ product_ids: ids });
    expect(result.success).toBe(true);
  });

  it("rejects empty product_ids", () => {
    const result = bulkActionSchema.safeParse({ product_ids: [] });
    expect(result.success).toBe(false);
  });

  it("rejects more than 100 product ids", () => {
    const ids = Array.from({ length: MAX_BULK_IDS + 1 }, (_, i) => `p${i}`);
    const result = bulkActionSchema.safeParse({ product_ids: ids });
    expect(result.success).toBe(false);
    if (!result.success) {
      expect(result.error.issues[0]?.message).toContain("100");
    }
  });

  it("accepts exactly 100 product ids", () => {
    const ids = Array.from({ length: MAX_BULK_IDS }, (_, i) => `p${i}`);
    const result = bulkActionSchema.safeParse({ product_ids: ids });
    expect(result.success).toBe(true);
  });
});
