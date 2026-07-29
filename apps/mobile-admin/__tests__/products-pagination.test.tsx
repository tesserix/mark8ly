jest.mock("@/lib/api-client", () => ({ useApiClient: () => ({}) }));

import { nextProductPage } from "@/lib/hooks/use-products";
import type { ProductListResponse } from "@repo/mobile-shared/api/schemas/products";

function pageOf(page: number, total_pages: number): ProductListResponse {
  return { data: [], meta: { page, page_size: 20, total: total_pages * 20, total_pages } };
}

describe("nextProductPage", () => {
  it("returns the next page number while more pages remain", () => {
    expect(nextProductPage(pageOf(1, 3))).toBe(2);
    expect(nextProductPage(pageOf(2, 3))).toBe(3);
  });

  it("returns undefined on the last page (stops infinite scroll)", () => {
    expect(nextProductPage(pageOf(3, 3))).toBeUndefined();
  });

  it("returns undefined for a single-page result", () => {
    expect(nextProductPage(pageOf(1, 1))).toBeUndefined();
  });
});
