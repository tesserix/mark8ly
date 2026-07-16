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
    expect(source).toContain("<CategoryField");
    expect(source).toContain("<VariantRow");
    expect(source).toContain("<MediaGrid");
    expect(source).toContain("<ImageViewer");
  });

  it("renders the editorial header with the productThumb and a StatusBadge", () => {
    expect(source).toContain("productThumb(product)");
    expect(source).toContain("<StatusBadge");
  });

  it("never spends moss on the header status badge — success IS moss", () => {
    // 🔴 The one-accent rule this task exists to enforce: the header badge
    // must never resolve to the `success` tone (StatusBadge.TONE.success is
    // a solid moss fill). It maps active -> "neutral" (solid ink) instead.
    expect(source).not.toMatch(/tone=\{?["']?success/);
  });

  it("keeps the Photos and option-add affordances ink at rest, moss only on press", () => {
    // Both affordances read the Pressable `pressed` render-prop rather than
    // hardcoding `color="accent"` — moss is reserved for the header Save.
    expect(source).toMatch(/pressed\s*\?\s*"accent"\s*:\s*"text"/);
  });

  it("migrated the Details card's Title/Description inputs to FieldInput", () => {
    expect(source).toMatch(/<FieldInput\s*\n\s*label="Title"/);
    expect(source).toContain('label="Description"');
    expect(source).not.toContain("<TextInput");
  });

  it("uses ghost cards with a top Hairline for Details/Options/Categories", () => {
    expect(source.match(/variant="ghost"/g)?.length).toBe(3);
  });

  it("never sends `variants` on a product PATCH — that matrix soft-deletes", () => {
    // UpdateAggregateRequest.Variants is a FULL DESIRED MATRIX; applyVariantsDiff
    // soft-deletes anything missing from it. Variant edits go through the
    // variant quick-PATCH instead.
    //
    // Anchored on the REAL call token: this screen invokes the product PATCH as
    // `updateMutation.mutate(...)`, never the lowercase `updateProduct`. The old
    // regex anchored on `updateProduct`, which appears nowhere here, so it
    // matched nothing and guarded nothing — it would have passed even for
    // `updateMutation.mutate({ id, body: { variants: [] } })`.
    expect(source).not.toMatch(/updateMutation\.mutate\([\s\S]{0,200}variants:/);
  });

  it("use-add-option-handler builds `variants` ONLY from buildOptionMatrix, never by hand", () => {
    // The add-option PATCH (the ONE place that legitimately sends `variants`)
    // moved out of [id].tsx into this hook, so the [id].tsx guard above no
    // longer covers it. `variants` is a FULL DESIRED MATRIX — a hand-built
    // array literal here would soft-delete every existing variant it omits.
    const handlerSource = require("fs").readFileSync(
      require("path").join(__dirname, "../lib/hooks/use-add-option-handler.ts"),
      "utf8",
    );
    // Anchor: the safe producer must actually be present, or this guard is
    // vacuous. buildOptionMatrix is the only sanctioned source of `variants`.
    expect(handlerSource).toContain("buildOptionMatrix");
    // The only `variants` sent must be buildOptionMatrix's destructured output
    // (`const { options, variants } = buildOptionMatrix(...)`), never a
    // hand-built `variants: [ ... ]` literal in a PATCH body.
    expect(handlerSource).not.toMatch(/variants:\s*\[/);
  });

  it("stayed small after composing — it was 571 lines before extraction", () => {
    expect(source.split("\n").length).toBeLessThan(500);
  });
});
