// CategoryField renders <CategoryPickerSheet> internally, which mounts a real
// @gorhom/bottom-sheet BottomSheetModal — that pulls in react-native-reanimated,
// which throws under jest without a full worklets/logger setup this project
// doesn't have. Stub the pieces the sheet's tree touches (same fix as
// __tests__/options-editor.test.tsx): BottomSheetFlatList added on top since
// the sheet renders its rows through it.
//
// Unlike option-builder-sheet's stub, BottomSheetModal here renders its
// children directly instead of returning null: CategoryField now keeps the
// sheet permanently mounted (not just while "presented"), and its own
// `isLoading` spinner/empty-state branches need to be reachable from
// CategoryField-level render() calls, not just from the imperative
// present()/dismiss() handle.
jest.mock("@gorhom/bottom-sheet", () => {
  const React = require("react");
  return {
    __esModule: true,
    BottomSheetModal: React.forwardRef(
      ({ children }: { children?: React.ReactNode }, ref: React.Ref<unknown>) => {
        React.useImperativeHandle(ref, () => ({ present: () => {}, dismiss: () => {} }));
        return children ?? null;
      },
    ),
    BottomSheetView: ({ children }: { children?: React.ReactNode }) => children ?? null,
    BottomSheetScrollView: ({ children }: { children?: React.ReactNode }) => children ?? null,
    BottomSheetFlatList: ({ children }: { children?: React.ReactNode }) => children ?? null,
  };
});
// `@/components/ui`'s barrel re-exports SearchField/BackHeader, which import
// icons from lucide-react-native's ESM build — not covered by jest-expo's
// default transformIgnorePatterns, so requiring it unmocked throws
// "Unexpected token 'export'". Stub every icon export with a no-op component
// (same fix as __tests__/category-picker.test.tsx).
jest.mock("lucide-react-native", () => {
  const IconStub = () => null;
  return new Proxy({}, { get: () => IconStub });
});
jest.mock("expo-haptics", () => ({
  selectionAsync: jest.fn(),
}));

import { render } from "@testing-library/react-native";
import { sortCategoryTree } from "@/components/products/CategoryPicker";
import { filterTree } from "@/components/products/CategoryPickerSheet";
import { CategoryField } from "@/components/products/CategoryField";

const cat = (id: string, name: string, parent_id?: string, position = 0) => ({
  id,
  store_id: "s1",
  name,
  slug: name.toLowerCase(),
  position,
  is_active: true,
  featured: false,
  created_at: "2026-05-04T23:48:01Z",
  updated_at: "2026-05-04T23:48:01Z",
  ...(parent_id ? { parent_id } : {}),
});

describe("filterTree", () => {
  // Deliberately out of tree-render order: Bikinis (a match for "wear"? no —
  // for "bikini") sits nested under Swimwear, which itself matches "wear",
  // and Towels (no match for either) is a root sorted after Swimwear by
  // position. A no-op filter (`return nodes`) would still contain Towels —
  // this fixture forces the assertions to actually discriminate.
  const nodes = sortCategoryTree([
    cat("c3", "Towels", undefined, 1),
    cat("c1", "Swimwear", undefined, 0),
    cat("c2", "Bikinis", "c1"),
  ] as never);

  it("returns only name-matching nodes, case-insensitively, when a query is given", () => {
    expect(filterTree(nodes, "wear").map((n) => n.category.name)).toEqual(["Swimwear"]);
    expect(filterTree(nodes, "BIKINI").map((n) => n.category.name)).toEqual(["Bikinis"]);
  });

  it("returns the full tree, unfiltered, when the query is empty", () => {
    expect(filterTree(nodes, "")).toEqual(nodes);
  });

  it("returns no rows when nothing matches", () => {
    expect(filterTree(nodes, "zzz")).toEqual([]);
  });
});

describe("CategoryField", () => {
  const CATS = [cat("c1", "Swimwear"), cat("c2", "Bikinis", "c1")];

  it("shows a placeholder when nothing is selected", () => {
    const { getByText, getByLabelText } = render(
      <CategoryField categories={CATS as never} selected={[]} onChange={jest.fn()} />,
    );
    expect(getByText("Add categories")).toBeTruthy();
    expect(getByLabelText("Categories, 0 selected, edit")).toBeTruthy();
  });

  it("shows a chip per selected category", () => {
    const { getByText } = render(
      <CategoryField
        categories={CATS as never}
        selected={[
          { id: "c1", name: "Swimwear", slug: "swimwear" },
          { id: "c2", name: "Bikinis", slug: "bikinis" },
        ]}
        onChange={jest.fn()}
      />,
    );
    expect(getByText("Swimwear")).toBeTruthy();
    expect(getByText("Bikinis")).toBeTruthy();
  });

  it("caps visible chips at 4 and folds the remainder into a +N chip", () => {
    const many = Array.from({ length: 6 }, (_, i) => cat(`c${i}`, `Cat${i}`));
    const selected = many.map((c) => ({ id: c.id, name: c.name, slug: c.slug }));
    const { getByText, queryByText } = render(
      <CategoryField categories={many as never} selected={selected} onChange={jest.fn()} />,
    );
    expect(getByText("Cat0")).toBeTruthy();
    expect(getByText("Cat3")).toBeTruthy();
    expect(queryByText("Cat4")).toBeNull();
    expect(getByText("+2")).toBeTruthy();
  });

  it("labels the whole field as a button naming the selection count", () => {
    const { getByLabelText } = render(
      <CategoryField
        categories={CATS as never}
        selected={[{ id: "c1", name: "Swimwear", slug: "swimwear" }]}
        onChange={jest.fn()}
      />,
    );
    expect(getByLabelText("Categories, 1 selected, edit")).toBeTruthy();
  });

  it("shows a disabled loading label instead of the placeholder while categories load", () => {
    const { getByText, queryByText } = render(
      <CategoryField categories={[]} selected={[]} onChange={jest.fn()} isLoading />,
    );
    expect(getByText("Loading categories…")).toBeTruthy();
    expect(queryByText("Add categories")).toBeNull();
  });

  it("keeps the sheet mounted and shows its own spinner while categories load, so a refetch mid-edit can't unmount an open sheet", () => {
    const { getByTestId } = render(
      <CategoryField categories={[]} selected={[]} onChange={jest.fn()} isLoading />,
    );
    expect(getByTestId("category-picker-loading")).toBeTruthy();
  });

  it("shows a retry affordance when categories failed to load", () => {
    const { getByLabelText } = render(
      <CategoryField categories={[]} selected={[]} onChange={jest.fn()} error={new Error("network")} />,
    );
    expect(getByLabelText("Couldn't load categories, tap to retry")).toBeTruthy();
  });
});
