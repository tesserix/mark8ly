jest.mock("@/lib/api-client", () => ({ useApiClient: () => ({}) }));

import { nextCustomerPage } from "@/lib/hooks/use-customers";
import type { CustomerListResponse } from "@repo/mobile-shared/api/schemas/customers";

function pageOf(page: number, total_pages: number): CustomerListResponse {
  return { data: [], meta: { page, page_size: 50, total: total_pages * 50, total_pages } };
}

describe("nextCustomerPage", () => {
  it("returns the next page number while more pages remain", () => {
    expect(nextCustomerPage(pageOf(1, 3))).toBe(2);
    expect(nextCustomerPage(pageOf(2, 3))).toBe(3);
  });

  it("returns undefined on the last page (stops infinite scroll)", () => {
    expect(nextCustomerPage(pageOf(3, 3))).toBeUndefined();
  });

  it("returns undefined for a single-page result", () => {
    expect(nextCustomerPage(pageOf(1, 1))).toBeUndefined();
  });
});
