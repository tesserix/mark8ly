// `@/components/ui` barrel re-exports BackHeader/SearchField, which import
// icons from lucide-react-native's ESM build (`dist/esm/...mjs`) — not
// covered by jest-expo's default transformIgnorePatterns, so requiring it
// unmocked throws "Unexpected token 'export'". Stub every icon export with a
// no-op component (same fix as __tests__/security.test.tsx).
jest.mock("lucide-react-native", () => {
  const IconStub = () => null;
  return new Proxy({}, { get: () => IconStub });
});
// OptionsEditor now renders <OptionBuilderSheet>, which mounts a real
// @gorhom/bottom-sheet BottomSheetModal — that pulls in react-native-reanimated,
// which throws under jest without a full worklets/logger setup this project
// doesn't have. Stub the two pieces OptionsEditor's tree touches; the sheet's
// own behaviour is pinned in __tests__/option-builder-sheet.test.tsx against
// the pure buildOptionSubmission function instead (portal content is not
// practical to unit-test here — see that file's header comment).
jest.mock("@gorhom/bottom-sheet", () => {
  const React = require("react");
  return {
    __esModule: true,
    BottomSheetModal: React.forwardRef((_props: unknown, ref: React.Ref<unknown>) => {
      React.useImperativeHandle(ref, () => ({ present: () => {}, dismiss: () => {} }));
      return null;
    }),
    BottomSheetView: ({ children }: { children?: React.ReactNode }) => children ?? null,
    BottomSheetScrollView: ({ children }: { children?: React.ReactNode }) => children ?? null,
  };
});

import { render, fireEvent } from "@testing-library/react-native";
import { OptionsEditor, toOptionRequestBodies } from "@/components/products/OptionsEditor";

const RESPONSE_OPTIONS = [
  {
    id: "opt-1",
    name: "Size",
    position: 0,
    values: [
      { id: "v2", value: "M", position: 1 },
      { id: "v1", value: "S", position: 0 },
    ],
  },
];

describe("toOptionRequestBodies", () => {
  it("converts the RESPONSE shape [{id,value,position}] to the REQUEST shape string[]", () => {
    expect(toOptionRequestBodies(RESPONSE_OPTIONS as never)).toEqual([
      { name: "Size", values: ["S", "M"] },
    ]);
  });

  it("orders values by position — the wire does not guarantee order", () => {
    const [first] = toOptionRequestBodies(RESPONSE_OPTIONS as never);
    expect(first!.values).toEqual(["S", "M"]);
  });

  it("orders options by position — Color (pos 1) listed first still lands after Size (pos 0)", () => {
    const OUT_OF_ORDER = [
      { id: "o2", name: "Color", position: 1, values: [{ id: "v3", value: "Red", position: 0 }] },
      { id: "o1", name: "Size", position: 0, values: [{ id: "v1", value: "S", position: 0 }] },
    ];
    expect(toOptionRequestBodies(OUT_OF_ORDER as never)).toEqual([
      { name: "Size", values: ["S"] },
      { name: "Color", values: ["Red"] },
    ]);
  });

  it("returns [] for a product with no options", () => {
    expect(toOptionRequestBodies([])).toEqual([]);
  });
});

describe("OptionsEditor", () => {
  it("emits the COMPLETE desired option set when a value is added", () => {
    const onChange = jest.fn();
    const { getByLabelText } = render(
      <OptionsEditor options={RESPONSE_OPTIONS as never} onChange={onChange} onAddOption={() => {}} />,
    );
    const input = getByLabelText("Add a value to Size");
    fireEvent.changeText(input, "L");
    fireEvent(input, "submitEditing");
    expect(onChange).toHaveBeenCalledWith([{ name: "Size", values: ["S", "M", "L"] }]);
  });

  it("emits the set without the removed value", () => {
    const onChange = jest.fn();
    const { getByLabelText } = render(
      <OptionsEditor options={RESPONSE_OPTIONS as never} onChange={onChange} onAddOption={() => {}} />,
    );
    fireEvent.press(getByLabelText("Remove M from Size"));
    expect(onChange).toHaveBeenCalledWith([{ name: "Size", values: ["S"] }]);
  });

  it("ignores a blank value rather than sending an empty string", () => {
    const onChange = jest.fn();
    const { getByLabelText } = render(
      <OptionsEditor options={RESPONSE_OPTIONS as never} onChange={onChange} onAddOption={() => {}} />,
    );
    const input = getByLabelText("Add a value to Size");
    fireEvent.changeText(input, "   ");
    fireEvent(input, "submitEditing");
    expect(onChange).not.toHaveBeenCalled();
  });

  it("ignores a duplicate value — the backend keys variants off the tuple", () => {
    const onChange = jest.fn();
    const { getByLabelText } = render(
      <OptionsEditor options={RESPONSE_OPTIONS as never} onChange={onChange} onAddOption={() => {}} />,
    );
    const input = getByLabelText("Add a value to Size");
    fireEvent.changeText(input, "S");
    fireEvent(input, "submitEditing");
    expect(onChange).not.toHaveBeenCalled();
  });
});

describe("OptionsEditor empty state", () => {
  it("shows guidance and an add-option affordance when there are no options", () => {
    const { getByText, getByLabelText } = render(
      <OptionsEditor options={[]} onChange={() => {}} onAddOption={() => {}} />,
    );
    expect(getByText(/one variant/i)).toBeTruthy();
    expect(getByLabelText("Add an option")).toBeTruthy();
  });

  it("still shows the add-option affordance once an option already exists", () => {
    const { getByLabelText, queryByText } = render(
      <OptionsEditor options={RESPONSE_OPTIONS as never} onChange={() => {}} onAddOption={() => {}} />,
    );
    expect(getByLabelText("Add an option")).toBeTruthy();
    expect(queryByText(/one variant/i)).toBeNull();
  });
});
