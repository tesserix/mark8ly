import { render, fireEvent } from "@testing-library/react-native";

// `@/components/products/CategoryPicker` imports `Check` from
// lucide-react-native directly, and pulls in `@/components/ui`'s barrel
// (for `Text`) which re-exports BackHeader/SearchField — those import icons
// from lucide-react-native's ESM build, not covered by jest-expo's default
// transformIgnorePatterns, so requiring any of this unmocked throws
// "Unexpected token 'export'". Stub every icon export with a no-op
// component (same fix as __tests__/security.test.tsx and
// __tests__/TenantGate.test.tsx).
jest.mock("lucide-react-native", () => {
  const IconStub = () => null;
  return new Proxy({}, { get: () => IconStub });
});

import { CategoryPicker, sortCategoryTree } from "@/components/products/CategoryPicker";

const cat = (id: string, name: string, parent_id?: string) => ({
  id,
  store_id: "s1",
  name,
  slug: name.toLowerCase(),
  position: 0,
  is_active: true,
  featured: false,
  created_at: "2026-05-04T23:48:01Z",
  updated_at: "2026-05-04T23:48:01Z",
  ...(parent_id ? { parent_id } : {}),
});

describe("sortCategoryTree", () => {
  it("nests children under their parent with a depth", () => {
    const flat = [cat("c2", "Bikinis", "c1"), cat("c1", "Swimwear"), cat("c3", "Towels")];
    const tree = sortCategoryTree(flat as never);
    expect(tree.map((n) => [n.category.name, n.depth])).toEqual([
      ["Swimwear", 0],
      ["Bikinis", 1],
      ["Towels", 0],
    ]);
  });

  it("keeps an orphan (parent_id pointing nowhere) visible at root rather than dropping it", () => {
    const flat = [cat("c9", "Orphan", "missing-parent")];
    const tree = sortCategoryTree(flat as never);
    expect(tree).toHaveLength(1);
    expect(tree[0]!.depth).toBe(0);
  });

  it("does not infinitely recurse on a self-referencing parent", () => {
    const flat = [cat("c1", "Loop", "c1")];
    expect(sortCategoryTree(flat as never)).toHaveLength(1);
  });
});

describe("CategoryPicker", () => {
  const CATS = [cat("c1", "Swimwear"), cat("c2", "Bikinis", "c1")];

  it("emits the full desired id set when one is added", () => {
    const onChange = jest.fn();
    const { getByLabelText } = render(
      <CategoryPicker
        categories={CATS as never}
        selected={[{ id: "c1", name: "Swimwear", slug: "swimwear" }]}
        onChange={onChange}
      />,
    );
    fireEvent.press(getByLabelText("Bikinis"));
    // Replace semantics: send the whole set, not a delta.
    expect(onChange).toHaveBeenCalledWith(["c1", "c2"]);
  });

  it("emits the remaining set when one is removed", () => {
    const onChange = jest.fn();
    const { getByLabelText } = render(
      <CategoryPicker
        categories={CATS as never}
        selected={[
          { id: "c1", name: "Swimwear", slug: "swimwear" },
          { id: "c2", name: "Bikinis", slug: "bikinis" },
        ]}
        onChange={onChange}
      />,
    );
    fireEvent.press(getByLabelText("Bikinis"));
    expect(onChange).toHaveBeenCalledWith(["c1"]);
  });

  it("emits an empty array when the last category is removed", () => {
    const onChange = jest.fn();
    const { getByLabelText } = render(
      <CategoryPicker
        categories={CATS as never}
        selected={[{ id: "c1", name: "Swimwear", slug: "swimwear" }]}
        onChange={onChange}
      />,
    );
    fireEvent.press(getByLabelText("Swimwear"));
    expect(onChange).toHaveBeenCalledWith([]);
  });

  it("emits in tree render order even when the incoming selection is out of order", () => {
    // c1 is a root, c2 its child, c3 another root → tree order is [c1, c2, c3].
    const treeCats = [cat("c1", "Swimwear"), cat("c2", "Bikinis", "c1"), cat("c3", "Towels")];
    const onChange = jest.fn();
    const { getByLabelText } = render(
      <CategoryPicker
        categories={treeCats as never}
        // Deliberately reversed vs. tree order. A `[...next]` (Set-insertion)
        // impl would carry this order through and emit ["c2","c1","c3"].
        selected={[
          { id: "c2", name: "Bikinis", slug: "bikinis" },
          { id: "c1", name: "Swimwear", slug: "swimwear" },
        ]}
        onChange={onChange}
      />,
    );
    fireEvent.press(getByLabelText("Towels"));
    // Must be tree order, not the order ids entered the Set.
    expect(onChange).toHaveBeenCalledWith(["c1", "c2", "c3"]);
  });

  it("renders gracefully with an empty categories array", () => {
    const onChange = jest.fn();
    const { queryByRole } = render(
      <CategoryPicker categories={[]} selected={[]} onChange={onChange} />,
    );
    expect(queryByRole("checkbox")).toBeNull();
  });
});
