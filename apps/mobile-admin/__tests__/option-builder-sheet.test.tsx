// Importing OptionBuilderSheet.tsx (even just for the pure function it also
// exports) pulls in lucide-react-native icons and @gorhom/bottom-sheet's
// BottomSheetModal — the latter requires react-native-reanimated, which
// throws under jest without a full worklets/logger setup this project
// doesn't have. The sheet itself renders through a portal, which isn't
// practical to mount in this project's jest setup either way, so this file
// only exercises the pure `buildOptionSubmission` logic — see that
// function's doc comment for why it was extracted.
jest.mock("lucide-react-native", () => {
  const IconStub = () => null;
  return new Proxy({}, { get: () => IconStub });
});
jest.mock("@gorhom/bottom-sheet", () => {
  const React = require("react");
  return {
    __esModule: true,
    BottomSheetModal: React.forwardRef((_props: unknown, ref: React.Ref<unknown>) => {
      React.useImperativeHandle(ref, () => ({ present: () => {}, dismiss: () => {} }));
      return null;
    }),
    BottomSheetView: ({ children }: { children?: React.ReactNode }) => children ?? null,
  };
});

import { buildOptionSubmission } from "@/components/products/OptionBuilderSheet";

describe("buildOptionSubmission", () => {
  it("builds an option from a trimmed name and values", () => {
    expect(buildOptionSubmission("Size", ["S", "M", "L"])).toEqual({
      name: "Size",
      values: ["S", "M", "L"],
    });
  });

  it("trims the name", () => {
    expect(buildOptionSubmission("  Size  ", ["S"])).toEqual({ name: "Size", values: ["S"] });
  });

  it("trims each value and drops blanks", () => {
    expect(buildOptionSubmission("Size", [" S ", "  ", "M"])).toEqual({
      name: "Size",
      values: ["S", "M"],
    });
  });

  it("dedupes values, keeping the first occurrence's order", () => {
    expect(buildOptionSubmission("Size", ["S", "M", "S", "M", "L"])).toEqual({
      name: "Size",
      values: ["S", "M", "L"],
    });
  });

  it("returns null for a blank name", () => {
    expect(buildOptionSubmission("   ", ["S", "M"])).toBeNull();
  });

  it("returns null when there are no values", () => {
    expect(buildOptionSubmission("Size", [])).toBeNull();
  });

  it("returns null when every value is blank", () => {
    expect(buildOptionSubmission("Size", ["  ", " "])).toBeNull();
  });
});
