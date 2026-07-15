import { z } from "zod";
import { money, pageMeta, paginated, dataOnly } from "@repo/mobile-shared/api/schema-helpers";

describe("money", () => {
  it.each([
    ["number", 84, 84],
    ["zero", 0, 0],
    ["float", 12.5, 12.5],
    ["decimal string", "84.00", 84],
    ["zero string", "0", 0],
    ["padded string", " 84.00 ", 84],
  ])("accepts %s", (_label, input, expected) => {
    expect(money.parse(input)).toBe(expected);
  });

  // The whole reason money is not z.coerce.number(): these must FAIL, not
  // silently become 0 or NaN. A wrong price is worse than a loud failure.
  // Each row is [label, value] so jest never has to pretty-print a bare
  // undefined/[]/{} as the test name.
  it.each([
    ["empty string", ""],
    ["whitespace only", "   "],
    ["non-numeric string", "abc"],
    ["null", null],
    ["undefined", undefined],
    ["boolean", true],
    ["array", []],
    ["object", {}],
  ])("rejects %s", (_label, input) => {
    expect(money.safeParse(input).success).toBe(false);
  });
});

describe("pageMeta", () => {
  it("accepts the real meta shape", () => {
    expect(
      pageMeta.parse({ page: 1, page_size: 20, total: 3, total_pages: 1 }),
    ).toEqual({ page: 1, page_size: 20, total: 3, total_pages: 1 });
  });
});

describe("paginated", () => {
  it("accepts {data, meta}", () => {
    const schema = paginated(z.object({ id: z.string() }));
    const parsed = schema.parse({
      data: [{ id: "a" }],
      meta: { page: 1, page_size: 20, total: 1, total_pages: 1 },
    });
    expect(parsed.data).toEqual([{ id: "a" }]);
  });

  it("rejects the fictional {items} shape", () => {
    const schema = paginated(z.object({ id: z.string() }));
    expect(schema.safeParse({ items: [{ id: "a" }], total: 1 }).success).toBe(false);
  });
});

describe("dataOnly", () => {
  it("accepts {data} with no meta", () => {
    const schema = dataOnly(z.object({ id: z.string() }));
    expect(schema.parse({ data: [{ id: "a" }] }).data).toEqual([{ id: "a" }]);
  });
});
