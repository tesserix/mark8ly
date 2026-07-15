import { storeSchema, storesResponseSchema } from "@repo/mobile-shared/api/schemas/stores";

// Captured from prod 2026-07-15: GET /api/v1/mobile/admin/stores
const REAL_RESPONSE = {
  data: [
    {
      id: "8b69eea9-2537-4d36-9d99-bafcbad02dbc",
      name: "The Bondi Store",
      slug: "the-bondi-store",
      country_code: "AU",
      currency_code: "AUD",
      status: "active",
    },
  ],
};

describe("storesResponseSchema", () => {
  it("parses the real prod {data:[...]} response", () => {
    const parsed = storesResponseSchema.parse(REAL_RESPONSE);
    expect(parsed.data).toHaveLength(1);
    expect(parsed.data[0].name).toBe("The Bondi Store");
    expect(parsed.data[0].currency_code).toBe("AUD");
  });

  // Negative control. The old TS type claimed {items}. If this passed, the
  // schema would be permissive rather than wire-truthful, and the bug that
  // made the dashboard unreachable could come back unnoticed.
  it("REJECTS the fictional {items:[...]} shape", () => {
    expect(
      storesResponseSchema.safeParse({ items: REAL_RESPONSE.data }).success,
    ).toBe(false);
  });

  it("rejects a store missing the fields the old type omitted", () => {
    // The old `Store` interface was only {id,name,slug}.
    expect(
      storeSchema.safeParse({ id: "a", name: "b", slug: "c" }).success,
    ).toBe(false);
  });
});
