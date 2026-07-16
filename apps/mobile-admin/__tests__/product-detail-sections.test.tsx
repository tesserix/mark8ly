// The detail screen pulls the real api client (and Firebase auth) through its
// hook imports. Mock it the same way use-products.test.tsx does.
jest.mock("@/lib/api-client", () => ({
  useApiClient: () => ({}),
}));

// tsconfig scopes `types` to ["jest"] only, so Node's ambient globals aren't
// picked up automatically — declare the one this file's fs.readFileSync
// call below needs, rather than widening the project-wide tsconfig.
declare const __dirname: string;

const source = require("fs").readFileSync(
  require("path").join(__dirname, "../app/(tabs)/products/[id].tsx"),
  "utf8",
);

describe("product detail screen composition", () => {
  it("renders every new section", () => {
    expect(source).toContain("<OptionsEditor");
    expect(source).toContain("<CategoryPicker");
    expect(source).toContain("<VariantEditor");
    expect(source).toContain("<MediaGrid");
    expect(source).toContain("<ImageViewer");
  });

  it("never sends `variants` on a product PATCH — that matrix soft-deletes", () => {
    // UpdateAggregateRequest.Variants is a FULL DESIRED MATRIX; applyVariantsDiff
    // soft-deletes anything missing from it. Variant edits go through the
    // variant quick-PATCH instead.
    expect(source).not.toMatch(/updateProduct[\s\S]{0,200}variants:/);
  });

  it("stayed small after composing — it was 571 lines before extraction", () => {
    expect(source.split("\n").length).toBeLessThan(500);
  });
});
